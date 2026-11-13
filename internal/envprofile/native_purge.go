package envprofile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/relux-works/curator/internal/envmarker"
	"github.com/relux-works/curator/internal/envregistry"
	"github.com/relux-works/curator/internal/pathboundary"
	"github.com/relux-works/curator/internal/stateread"
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
		root   *os.Root
		marker *envmarker.Marker
	}
	var targets []target
	defer func() {
		for _, item := range targets {
			_ = item.root.Close()
		}
	}()
	for _, id := range ids {
		native, err := resolve(id)
		if err != nil {
			return err
		}
		state, err := stateread.Lstat(filepath.Join(native, envmarker.Name))
		if err != nil {
			return err
		}
		if state.Kind == stateread.KindAbsent {
			continue
		}
		if err := pathboundary.ValidateLeafWithOwner(native, filepath.Join(native, envmarker.Name), pathboundary.DefaultOwnerLookup()); err != nil {
			return err
		}
		marker, err := envmarker.Read(native)
		if err != nil {
			return err
		}
		if marker == nil || marker.Profile.Name != profile {
			continue
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
				if err := validateNativePurgePath(native, path); err != nil {
					return err
				}
			}
		}
		if err := validateNativePurgePath(native, ".agent-environment-backup"); err != nil {
			return err
		}
		before, err := os.Stat(native)
		if err != nil {
			return err
		}
		root, err := os.OpenRoot(native)
		if err != nil {
			return err
		}
		after, err := root.Stat(".")
		if err != nil || !os.SameFile(before, after) {
			_ = root.Close()
			return fmt.Errorf("%s: bound native directory changed", DiagNativeHomeBindingRequired)
		}
		targets = append(targets, target{root: root, marker: marker})
	}
	managed, err := preflightManagedPurge(home, profile)
	if err != nil {
		return err
	}

	for _, item := range targets {
		for _, key := range item.marker.SortedSurfaceKeys() {
			surface := item.marker.Surfaces[key]
			for _, path := range surface.Paths {
				if err := item.root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
		if err := item.root.Remove(envmarker.Name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := item.root.RemoveAll(".agent-environment-backup"); err != nil {
			return err
		}
	}
	return os.RemoveAll(managed)
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
	}
	return managed, nil
}
func purgeManagedHomes(home, profile string) error {
	managed, err := preflightManagedPurge(home, profile)
	if err != nil {
		return err
	}
	return os.RemoveAll(managed)
}
