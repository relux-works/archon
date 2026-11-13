package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envmarker"
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
	for _, opts := range []RemoveOptions{{}, {Purge: true}, {Purge: true, ManagedOnly: true}} {
		manager := bindingDirectory(t)
		err := RemoveWithOptions(manager, "default", opts)
		if err == nil || !strings.Contains(err.Error(), "builtin_profile_remove_forbidden") {
			t.Fatalf("builtin removal = %v", err)
		}
		entries, readErr := os.ReadDir(manager)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("builtin refusal changed manager: %v %v", entries, readErr)
		}
	}
}
