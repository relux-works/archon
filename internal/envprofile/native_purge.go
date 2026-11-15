package envprofile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/relux-works/curator/internal/envmarker"
	"github.com/relux-works/curator/internal/envregistry"
	"github.com/relux-works/curator/internal/pathboundary"
	"github.com/relux-works/curator/internal/stateread"
)

const (
	DiagProfilePurgePreflightRefused = "profile_purge_preflight_refused"
	DiagProfilePurgeIncomplete       = "profile_purge_incomplete"
)

// purgeHomesWithBindings preflights the complete native target set before any
// deletion. Manager authority and profile-in-use checks belong to Remove.
func purgeHomesWithBindings(home, profile string, bindings NativeHomeBindings) error {
	if !validProfileName(profile) {
		return fmt.Errorf("%s: invalid profile name", DiagProfileUnknown)
	}
	ids := make([]string, 0, len(Adapters))
	for _, adapter := range Adapters {
		ids = append(ids, adapter.ID)
	}
	sort.Strings(ids)
	resolve, err := nativeBindingResolver(home, bindings, ids)
	if err != nil {
		return err
	}
	type target struct {
		adapter string
		root    *os.Root
		marker  *envmarker.Marker
	}
	var targets []target
	var roots []*os.Root
	defer func() {
		for _, root := range roots {
			_ = root.Close()
		}
	}()
	for _, id := range ids {
		binding, err := resolve(id)
		if err != nil {
			return err
		}
		if beforeNativePurgeRootOpen != nil {
			if err := beforeNativePurgeRootOpen(binding.path); err != nil {
				return err
			}
		}
		currentState, err := stateread.Lstat(binding.path)
		if err != nil {
			return fmt.Errorf("%s: bound native directory is stale: %w", DiagNativeHomeBindingRequired, err)
		}
		if currentState.Kind == stateread.KindAbsent {
			return fmt.Errorf("%s: bound native directory is stale: %w", DiagNativeHomeBindingRequired, stateread.AbsentError(binding.path))
		}
		if currentState.Kind != stateread.KindPresent || currentState.Info == nil {
			return fmt.Errorf("%s: bound native directory is stale: %s", DiagNativeHomeBindingRequired, binding.path)
		}
		current := currentState.Info
		if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(binding.info, current) {
			return fmt.Errorf("%s: bound native directory changed after admission", DiagNativeHomeBindingRequired)
		}
		root, err := os.OpenRoot(binding.path)
		if err != nil {
			return fmt.Errorf("%s: cannot open the admitted native directory: %w", DiagNativeHomeBindingRequired, err)
		}
		roots = append(roots, root)
		openedState, err := stateread.RootStat(root, ".")
		if err != nil {
			return fmt.Errorf("%s: cannot confirm opened native identity: %w", DiagNativeHomeBindingRequired, err)
		}
		if openedState.Kind != stateread.KindPresent || openedState.Info == nil {
			return fmt.Errorf("%s: cannot confirm opened native identity", DiagNativeHomeBindingRequired)
		}
		opened := openedState.Info
		if !os.SameFile(binding.info, opened) {
			return fmt.Errorf("%s: bound native directory changed after admission", DiagNativeHomeBindingRequired)
		}
		markerState, err := stateread.RootLstat(root, envmarker.Name)
		if err != nil {
			return fmt.Errorf("%s: inspect native marker before purge: %w", DiagProfilePurgePreflightRefused, err)
		}
		if markerState.Kind == stateread.KindAbsent {
			continue
		}
		if markerState.Kind != stateread.KindPresent || markerState.Info == nil {
			return fmt.Errorf("%s: inspect native marker before purge", DiagProfilePurgePreflightRefused)
		}
		markerInfo := markerState.Info
		markerPath := filepath.Join(binding.path, envmarker.Name)
		if err := pathboundary.ValidateLeafWithOwner(binding.path, markerPath, pathboundary.DefaultOwnerLookup()); err != nil {
			return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
		}
		if beforeNativePurgeMarkerRead != nil {
			if err := beforeNativePurgeMarkerRead(binding.path); err != nil {
				return err
			}
		}
		marker, err := readNativePurgeMarker(binding.path, markerInfo)
		if err != nil {
			return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
		}
		if marker == nil || marker.Profile.Name != profile {
			continue
		}
		if err := pathboundary.CheckRemoveAccess(markerPath); err != nil {
			return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
		}
		if marker.Mode == envmarker.ModeManagedHome {
			return fmt.Errorf("%s: a managed marker cannot admit a native target", DiagNativeHomeBindingRequired)
		}
		for _, key := range marker.SortedSurfaceKeys() {
			surface := marker.Surfaces[key]
			for _, path := range surface.Paths {
				if credentialPath(id, path) {
					return fmt.Errorf("%s: native credential path cannot be purged as a context surface", envregistry.DiagCredentialConflict)
				}
				if err := validateNativePurgePath(binding.path, path); err != nil {
					return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
				}
				if err := preflightNativeSurfacePath(root, binding.path, path); err != nil {
					return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
				}
			}
		}
		if err := preflightNativeBackup(root, binding.path); err != nil {
			return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
		}
		targets = append(targets, target{adapter: id, root: root, marker: marker})
	}
	managed, err := preflightManagedPurge(home, profile)
	if err != nil {
		return fmt.Errorf("%s: %w", DiagProfilePurgePreflightRefused, err)
	}
	if beforeNativePurgeDelete != nil {
		beforeNativePurgeDelete()
	}

	for _, item := range targets {
		for _, key := range item.marker.SortedSurfaceKeys() {
			surface := item.marker.Surfaces[key]
			for _, path := range surface.Paths {
				if err := item.root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return purgeIncomplete("native adapter "+item.adapter+" surface "+path, err)
				}
			}
		}
		if err := item.root.Remove(envmarker.Name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return purgeIncomplete("native adapter "+item.adapter+" marker", err)
		}
		if err := item.root.RemoveAll(".agent-environment-backup"); err != nil {
			return purgeIncomplete("native adapter "+item.adapter+" backup directory", err)
		}
	}
	if err := os.RemoveAll(managed); err != nil {
		return purgeIncomplete("managed home", err)
	}
	return nil
}

func preflightNativeSurfacePath(root *os.Root, native, path string) error {
	relative := filepath.FromSlash(path)
	state, err := stateread.RootLstat(root, relative)
	if err != nil {
		return fmt.Errorf("inspect surface %q: %w", path, err)
	}
	if state.Kind == stateread.KindAbsent {
		return nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fmt.Errorf("inspect surface %q: metadata unavailable", path)
	}
	info := state.Info
	if info.Mode()&os.ModeSymlink != 0 {
		return pathboundary.CheckRemoveAccess(filepath.Join(native, relative)) // Unlinks do not follow recorded surface links.
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("surface %q is neither a regular file nor a directory", path)
	}
	if err := pathboundary.ValidateLeafWithOwner(native, filepath.Join(native, relative), pathboundary.DefaultOwnerLookup()); err != nil {
		return err
	}
	if err := pathboundary.CheckRemoveAccess(filepath.Join(native, relative)); err != nil {
		return err
	}
	if info.IsDir() {
		dir, err := root.Open(relative)
		if err != nil {
			return fmt.Errorf("inspect directory surface %q: %w", path, err)
		}
		entries, readErr := dir.ReadDir(-1)
		closeErr := dir.Close()
		if readErr != nil {
			return fmt.Errorf("inspect directory surface %q: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close directory surface %q: %w", path, closeErr)
		}
		if len(entries) != 0 {
			return fmt.Errorf("surface directory %q is not empty", path)
		}
	}
	return nil
}

func preflightNativeBackup(root *os.Root, native string) error {
	const name = ".agent-environment-backup"
	state, err := stateread.RootLstat(root, name)
	if err != nil {
		return fmt.Errorf("inspect backup directory: %w", err)
	}
	if state.Kind == stateread.KindAbsent {
		return nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fmt.Errorf("inspect backup directory: metadata unavailable")
	}
	info := state.Info
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("backup path is not a direct directory")
	}
	path := filepath.Join(native, name)
	if err := pathboundary.ValidateRouteWithOwner(native, path, pathboundary.DefaultOwnerLookup()); err != nil {
		return err
	}
	return preflightNativeRemoveTree(root, native, name)
}

func preflightRemoveTree(root, target string) error {
	state, err := stateread.Lstat(target)
	if err != nil {
		return fmt.Errorf("inspect removal target: %w", err)
	}
	if state.Kind == stateread.KindAbsent {
		return nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fmt.Errorf("inspect removal target: metadata unavailable")
	}
	info := state.Info
	if info.Mode()&os.ModeSymlink != 0 {
		return pathboundary.CheckRemoveAccess(target)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("removal target is neither a regular file nor a directory")
	}
	if err := pathboundary.ValidateLeafWithOwner(root, target, pathboundary.DefaultOwnerLookup()); err != nil {
		return err
	}
	if err := pathboundary.CheckRemoveAccess(target); err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return fmt.Errorf("inspect removal directory: %w", err)
		}
		for _, entry := range entries {
			if err := preflightRemoveTree(root, filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func preflightNativeRemoveTree(root *os.Root, native, relative string) error {
	state, err := stateread.RootLstat(root, relative)
	if err != nil {
		return fmt.Errorf("inspect removal target: %w", err)
	}
	if state.Kind == stateread.KindAbsent {
		return nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fmt.Errorf("inspect removal target: metadata unavailable")
	}
	info := state.Info
	target := filepath.Join(native, relative)
	if info.Mode()&os.ModeSymlink != 0 {
		if err := pathboundary.ValidateRouteWithOwner(native, filepath.Dir(target), pathboundary.DefaultOwnerLookup()); err != nil {
			return err
		}
		return pathboundary.CheckRemoveAccess(target)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("removal target is neither a regular file nor a directory")
	}
	if err := pathboundary.ValidateLeafWithOwner(native, target, pathboundary.DefaultOwnerLookup()); err != nil {
		return err
	}
	if err := pathboundary.CheckRemoveAccess(target); err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	directory, err := root.Open(relative)
	if err != nil {
		return fmt.Errorf("inspect removal directory: %w", err)
	}
	opened, statErr := directory.Stat()
	if statErr != nil {
		_ = directory.Close()
		return fmt.Errorf("inspect removal directory: %w", statErr)
	}
	if !opened.IsDir() || !os.SameFile(info, opened) {
		_ = directory.Close()
		return fmt.Errorf("removal directory changed during preflight")
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return fmt.Errorf("inspect removal directory: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close removal directory: %w", closeErr)
	}
	for _, entry := range entries {
		if err := preflightNativeRemoveTree(root, native, filepath.Join(relative, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func purgeIncomplete(stage string, cause error) error {
	return fmt.Errorf("%s: purge stopped during %s; selected targets may now be partially removed and no rollback was attempted: %w", DiagProfilePurgeIncomplete, stage, cause)
}

// Check parent routes without reading payloads. A surface leaf may be a
// symlink: confined Remove unlinks it, never follows it into another home.
func validateNativePurgePath(native, path string) error {
	parent := filepath.Dir(filepath.Join(native, filepath.FromSlash(path)))
	if err := pathboundary.ValidateRouteWithOwner(native, parent, pathboundary.DefaultOwnerLookup()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Managed homes are selected by manager/profile identity, never native binding.
func preflightManagedPurge(home, profile string) (string, error) {
	if !validProfileName(profile) {
		return "", fmt.Errorf("%s: invalid profile name", DiagProfileUnknown)
	}
	managed := filepath.Join(EnvRoot(home), profile)
	state, err := stateread.Lstat(managed)
	if err != nil {
		return "", err
	}
	if state.Kind != stateread.KindAbsent {
		if err := pathboundary.ValidateRouteWithOwner(EnvRoot(home), managed, pathboundary.DefaultOwnerLookup()); err != nil {
			return "", err
		}
		if err := preflightRemoveTree(EnvRoot(home), managed); err != nil {
			return "", err
		}
	}
	return managed, nil
}

func preflightProfileRemoval(home, profile string) error {
	path := ProfileDir(home, profile)
	state, err := stateread.Lstat(path)
	if err != nil {
		return err
	}
	if state.Kind == stateread.KindAbsent {
		return nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fmt.Errorf("profile metadata unavailable")
	}
	info := state.Info
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("profile path is not a direct directory")
	}
	if err := pathboundary.ValidateRouteWithOwner(filepath.Dir(path), path, pathboundary.DefaultOwnerLookup()); err != nil {
		return err
	}
	return preflightRemoveTree(filepath.Dir(path), path)
}

func purgeManagedHomes(home, profile string) error {
	managed, err := preflightManagedPurge(home, profile)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(managed); err != nil {
		return purgeIncomplete("managed home", err)
	}
	return nil
}

// Test-only fault seam between metadata admission and the actual marker open.
// Production leaves it nil; an error stops before marker bytes are read.
var beforeNativePurgeMarkerRead func(string) error

// Test-only fault seams. Production leaves them nil.
var beforeNativePurgeRootOpen func(string) error
var beforeNativePurgeDelete func()

// Read only the admitted regular marker inode. A link or replacement after
// metadata admission refuses before reading any credential or session payload.
func readNativePurgeMarker(native string, expected os.FileInfo) (*envmarker.Marker, error) {
	return envmarker.ReadWith(native, func(path string) (stateread.File, error) {
		unreadable := func(err error) (stateread.File, error) { return stateread.File{}, stateread.UnusableError(path, err) }
		file, err := pathboundary.OpenReadNoFollow(path)
		if err != nil {
			return unreadable(err)
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			return unreadable(err)
		}
		if expected == nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) {
			return unreadable(fmt.Errorf("native marker changed after admission"))
		}
		if err := pathboundary.CheckPrivateFile(path, info); err != nil {
			return unreadable(err)
		}
		payload, err := io.ReadAll(file)
		if err != nil {
			return unreadable(err)
		}
		return stateread.File{Kind: stateread.KindPresent, Bytes: payload}, nil
	})
}
