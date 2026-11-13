package envprofile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/pathboundary"
)

// Sidecar set/show/unset preserves the selected config's bytes and inode,
// retains a 0600 sidecar, and records the exact source. Missing reads do no IO
// mutation. Duplicate target rejection preserves the old sidecar bytes.
func TestNativeHomeBindingStore(t *testing.T) {
	manager := bindingDirectory(t)
	configPath := filepath.Join(manager, "config.json")
	original := []byte("{\"schema_version\":2,\"skills_root\":\"unused\",\"projects\":{}}\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	bindings, source, err := ReadNativeHomeBindings(configPath)
	if err != nil || len(bindings) != 0 || source != "" {
		t.Fatalf("missing sidecar = %v %q %v", bindings, source, err)
	}
	entries, err := os.ReadDir(manager)
	if err != nil || len(entries) != 1 {
		t.Fatalf("missing read wrote state: %v %v", entries, err)
	}
	native := bindingDirectory(t)
	if err := SetNativeHomeBinding(configPath, manager, "codex_cli", native); err != nil {
		t.Fatal(err)
	}
	bindings, source, err = ReadNativeHomeBindings(configPath)
	if err != nil || bindings["codex_cli"] != native || source != NativeHomeBindingsPath(configPath) {
		t.Fatalf("stored binding = %v %q %v", bindings, source, err)
	}
	sidecarBefore, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetNativeHomeBinding(configPath, manager, "pi", native); err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
		t.Fatalf("duplicate target = %v", err)
	}
	sidecarAfter, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(sidecarBefore, sidecarAfter) {
		t.Fatalf("rejected set changed sidecar: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(source)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("sidecar mode = %v %v", info, err)
		}
	}
	if err := UnsetNativeHomeBinding(configPath, "codex_cli"); err != nil {
		t.Fatal(err)
	}
	bindings, _, err = ReadNativeHomeBindings(configPath)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("unset = %v %v", bindings, err)
	}
	after, err := os.Lstat(configPath)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("binding operation changed selected config metadata: %v", err)
	}
	current, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(original, current) {
		t.Fatalf("binding operation changed old config bytes: %v", err)
	}
}

// Unknown fields/adapters, unsupported versions, duplicate keys and unsafe
// sidecar ownership/modes refuse before replacement; no permission repair.
func TestNativeHomeBindingStoreRefusals(t *testing.T) {
	for _, payload := range []string{
		`{"version":1,"bindings":{},"extra":true}`,
		`{"version":2,"bindings":{}}`,
		`{"version":1}`,
		`{"version":1,"bindings":{},"version":1}`,
		`{"version":1,"bindings":{"unknown":"/synthetic/native"}}`,
		`{"version":1,"bindings":{"codex_cli":"relative"}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			manager := bindingDirectory(t)
			cfg := filepath.Join(manager, "config.json")
			sidecar := NativeHomeBindingsPath(cfg)
			if err := os.WriteFile(sidecar, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadNativeHomeBindings(cfg); err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
				t.Fatalf("invalid sidecar = %v", err)
			}
			if err := SetNativeHomeBinding(cfg, manager, "codex_cli", bindingDirectory(t)); err == nil {
				t.Fatal("invalid sidecar replaced")
			}
			data, err := os.ReadFile(sidecar)
			if err != nil || string(data) != payload {
				t.Fatalf("refusal rewrote invalid sidecar: %v", err)
			}
		})
	}
	if runtime.GOOS != "windows" {
		for _, mode := range []os.FileMode{0o644, 0o660} {
			t.Run(mode.String(), func(t *testing.T) {
				manager := bindingDirectory(t)
				cfg := filepath.Join(manager, "config.json")
				sidecar := NativeHomeBindingsPath(cfg)
				if err := os.WriteFile(sidecar, []byte(`{"version":1,"bindings":{}}`), mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(sidecar, mode); err != nil {
					t.Fatal(err)
				}
				if _, _, err := ReadNativeHomeBindings(cfg); err == nil {
					t.Fatal("unsafe sidecar mode accepted")
				}
				info, err := os.Lstat(sidecar)
				if err != nil || info.Mode().Perm() != mode {
					t.Fatalf("read repaired sidecar mode: %v", err)
				}
			})
		}
	}
	t.Run("foreign-owner", func(t *testing.T) {
		manager := bindingDirectory(t)
		cfg := filepath.Join(manager, "config.json")
		if err := os.WriteFile(NativeHomeBindingsPath(cfg), []byte(`{"version":1,"bindings":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		lookup := func(string, os.FileInfo) (pathboundary.OwnerIdentity, error) {
			return pathboundary.OwnerIdentity("foreign-fixture-owner"), nil
		}
		if _, _, err := readNativeHomeBindingsWithOwner(cfg, lookup); err == nil || !strings.Contains(err.Error(), pathboundary.CheckOwnership) {
			t.Fatalf("foreign sidecar = %v", err)
		}
	})
}

// The sidecar path itself cannot be a link. The unrelated target remains
// unchanged when both read and mutation refuse that link.
func TestNativeHomeBindingStoreRejectsLink(t *testing.T) {
	manager := bindingDirectory(t)
	cfg := filepath.Join(manager, "config.json")
	foreign := filepath.Join(bindingDirectory(t), "other.json")
	data := []byte(`{"version":1,"bindings":{}}`)
	if err := os.WriteFile(foreign, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, NativeHomeBindingsPath(cfg)); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if _, _, err := ReadNativeHomeBindings(cfg); err == nil {
		t.Fatal("linked sidecar read accepted")
	}
	if err := SetNativeHomeBinding(cfg, manager, "codex_cli", bindingDirectory(t)); err == nil {
		t.Fatal("linked sidecar mutation accepted")
	}
	after, err := os.ReadFile(foreign)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatalf("linked sidecar changed foreign bytes: %v", err)
	}
}

// A relative selected config is resolved using that explicit path's current
// directory, without deriving any native target from HOME or adapter variables.
func TestNativeHomeBindingStoreRelativeConfig(t *testing.T) {
	manager := bindingDirectory(t)
	t.Chdir(manager)
	native := bindingDirectory(t)
	if err := SetNativeHomeBinding("config.json", ".", "codex_cli", native); err != nil {
		t.Fatal(err)
	}
	bindings, source, err := ReadNativeHomeBindings("config.json")
	if err != nil || bindings["codex_cli"] != native || source != NativeHomeBindingsPath(filepath.Join(manager, "config.json")) {
		t.Fatalf("relative selected config = %v %q %v", bindings, source, err)
	}
}
