package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/envmarker"
)

// The legacy purge entry fails before creating locks, journals or profiles and
// never guesses the managed caller's native home. Ordinary non-purge removal
// remains covered by the existing profile removal controls.
func TestNativeHomeBindingLegacyPurgeNoWrites(t *testing.T) {
	manager := bindingDirectory(t)
	caller := bindingDirectory(t)
	bindingPurgeMarker(t, caller, "sample", "AGENTS.md")
	for _, adapter := range Adapters {
		t.Setenv(adapter.EnvVar, caller)
	}
	before, err := os.Lstat(manager)
	if err != nil {
		t.Fatal(err)
	}
	err = Remove(manager, "sample", true)
	if err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
		t.Fatalf("legacy missing binding = %v", err)
	}
	entries, err := os.ReadDir(manager)
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing binding wrote manager entries: %v %v", entries, err)
	}
	after, err := os.Lstat(manager)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("missing binding changed manager metadata: %v", err)
	}
	if marker, err := envmarker.Read(caller); err != nil || marker == nil || marker.Profile.Name != "sample" {
		t.Fatalf("missing binding changed caller marker: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(caller, "AGENTS.md")); err != nil || string(data) != "recorded context\n" {
		t.Fatalf("missing binding changed caller context: %v", err)
	}
}

// A real removable profile succeeds with exact native bindings, while managed
// enumeration can purge only that profile's managed homes with no binding.
func TestNativeHomeBindingRemovalPositiveModes(t *testing.T) {
	for _, managedOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "native-and-managed", true: "managed-only"}[managedOnly], func(t *testing.T) {
			fx := writeManagedFixture(t, "sample")
			if err := SetCurrent(fx.home, "default"); err != nil {
				t.Fatal(err)
			}
			native := bindingPurgeHomes(t)
			caller := bindingDirectory(t)
			bindingPurgeMarker(t, caller, "sample", "AGENTS.md")
			for _, adapter := range Adapters {
				t.Setenv(adapter.EnvVar, caller)
			}
			bindingPurgeMarker(t, native["codex_cli"], "sample", "AGENTS.md")
			managed := ManagedHomeDir(fx.home, fx.profile, "codex_cli")
			if err := os.MkdirAll(managed, 0o700); err != nil {
				t.Fatal(err)
			}
			opts := RemoveOptions{Purge: true, ManagedOnly: managedOnly}
			if !managedOnly {
				opts.NativeHomes = native
			}
			if err := RemoveWithOptions(fx.home, fx.profile, opts); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{ProfileDir(fx.home, fx.profile), filepath.Join(EnvRoot(fx.home), fx.profile)} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("removal retained selected profile target: %v", err)
				}
			}
			_, err := os.Lstat(filepath.Join(native["codex_cli"], "AGENTS.md"))
			if managedOnly && err != nil {
				t.Fatalf("managed-only changed native target: %v", err)
			}
			if !managedOnly && !os.IsNotExist(err) {
				t.Fatalf("native purge retained bound target: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(caller, "AGENTS.md")); err != nil {
				t.Fatalf("removal changed managed caller: %v", err)
			}
		})
	}
}

// The new binding API preserves builtin refusal independently of the legacy
// wrapper, including managed-only and non-purge calls, before manager writes.
func TestNativeHomeBindingRemovalPreservesBuiltin(t *testing.T) {
	for _, name := range []string{"default", "./default", "child/../default", "../profiles/default", "default/"} {
		for _, opts := range []RemoveOptions{{}, {Purge: true}, {Purge: true, ManagedOnly: true}} {
			manager := bindingDirectory(t)
			err := RemoveWithOptions(manager, name, opts)
			if err == nil || !strings.Contains(err.Error(), "builtin_profile_remove_forbidden") {
				t.Fatalf("builtin removal %q = %v", name, err)
			}
			entries, readErr := os.ReadDir(manager)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("builtin refusal changed manager: %v %v", entries, readErr)
			}
		}
	}
}

// Nonportable removal names refuse before manager creation. Distinct Default
// remains removable on a case-sensitive filesystem; an actual case alias
// instead receives builtin refusal. Only the host's branch is exercised.
func TestNativeHomeBindingRemovalNameBoundary(t *testing.T) {
	for _, name := range []string{".", "..", "../other", "nested/profile", ""} {
		manager := bindingDirectory(t)
		err := RemoveWithOptions(manager, name, RemoveOptions{})
		if err == nil || !strings.Contains(err.Error(), DiagSourceInvalid) {
			t.Fatalf("invalid removal name %q = %v", name, err)
		}
		entries, err := os.ReadDir(manager)
		if err != nil || len(entries) != 0 {
			t.Fatalf("invalid removal wrote manager: %v %v", entries, err)
		}
	}
	fx := writeManagedFixture(t, "Default")
	if err := SetCurrent(fx.home, DefaultProfile); err != nil {
		t.Fatal(err)
	}
	builtin := ProfileDir(fx.home, DefaultProfile)
	if err := os.MkdirAll(builtin, 0o700); err != nil {
		t.Fatal(err)
	}
	selectedInfo, err := os.Lstat(ProfileDir(fx.home, "Default"))
	if err != nil {
		t.Fatal(err)
	}
	builtinInfo, err := os.Lstat(builtin)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(selectedInfo, builtinInfo) {
		if err := RemoveWithOptions(fx.home, "Default", RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "builtin_profile_remove_forbidden") {
			t.Fatalf("actual case alias = %v", err)
		}
		return
	}
	if err := RemoveWithOptions(fx.home, "Default", RemoveOptions{}); err != nil {
		t.Fatalf("distinct Default removal: %v", err)
	}
	if _, err := os.Lstat(ProfileDir(fx.home, "Default")); !os.IsNotExist(err) {
		t.Fatalf("distinct Default retained: %v", err)
	}
	after, err := os.Lstat(builtin)
	if err != nil || !os.SameFile(builtinInfo, after) {
		t.Fatalf("builtin changed: %v", err)
	}
}

// A normalized builtin alias refuses before mutation on initialized state;
// the builtin directory, retained bytes and complete manager-tree metadata stay.
func TestNativeHomeBindingRemovalBuiltinAliasState(t *testing.T) {
	fx := writeManagedFixture(t, "sample")
	if err := EnsureDefault(fx.home); err != nil {
		t.Fatal(err)
	}
	builtin := ProfileDir(fx.home, DefaultProfile)
	if err := os.MkdirAll(builtin, 0o700); err != nil {
		t.Fatal(err)
	}
	retained := filepath.Join(builtin, "retain.md")
	if err := os.WriteFile(retained, []byte("retain builtin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := map[string]os.FileInfo{}
	if err := filepath.Walk(fx.home, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			before[path] = info
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"./default", "child/../default", "../profiles/default"} {
		for _, opts := range []RemoveOptions{{}, {Purge: true}, {Purge: true, ManagedOnly: true}} {
			if err := RemoveWithOptions(fx.home, name, opts); err == nil || !strings.Contains(err.Error(), "builtin_profile_remove_forbidden") {
				t.Fatalf("builtin alias %q = %v", name, err)
			}
		}
	}
	count := 0
	if err := filepath.Walk(fx.home, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		count++
		old := before[path]
		if old == nil || !os.SameFile(old, info) || old.Mode() != info.Mode() || !old.ModTime().Equal(info.ModTime()) || old.Size() != info.Size() {
			t.Errorf("builtin refusal changed manager path %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != len(before) {
		t.Fatalf("manager path count = %d; want %d", count, len(before))
	}
	if data, err := os.ReadFile(retained); err != nil || string(data) != "retain builtin\n" {
		t.Fatalf("builtin retained bytes = %q, %v", data, err)
	}
}
