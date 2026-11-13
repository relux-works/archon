package main

import (
	"os"
	"path/filepath"

	"github.com/relux-works/curator/internal/envprofile"
	"strings"
	"testing"
)

// Explicit binding syntax accepts exact absolute paths (including '=') and
// rejects absent IDs, missing paths, relative paths, unknown IDs and duplicate
// bindings without modifying earlier accepted values.
func TestNativeHomeBindingFlags(t *testing.T) {
	home := filepath.Join(t.TempDir(), "native=codex")
	var f nativeHomeFlags
	if err := f.Set("codex_cli=" + home); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"codex_cli", "=", "codex_cli=relative", "codex_cli=", "unknown=" + home, "codex_cli=" + filepath.Join(t.TempDir(), "other")} {
		if err := f.Set(value); err == nil {
			t.Errorf("invalid binding accepted: %q", value)
		}
		if len(f.homes) != 1 || f.homes["codex_cli"] != home {
			t.Fatalf("refused flag changed existing bindings: %v", f.homes)
		}
	}
	if err := f.Set("pi=" + filepath.Join(t.TempDir(), "pi")); err != nil {
		t.Fatal(err)
	}
	if len(f.homes) != 2 {
		t.Fatal("valid second binding missing")
	}
	if strings.Contains(f.String(), home) {
		t.Fatal("default flag display discloses explicit path")
	}
}

// Production CLI removal refuses an unbound native purge, accepts managed-only
// purge without bindings, and accepts the same removable profile with every
// native adapter explicitly bound. The caller's overrides remain irrelevant.
func TestNativeHomeBindingCLI(t *testing.T) {
	for _, mode := range []string{"unbound", "managed-only", "bound"} {
		t.Run(mode, func(t *testing.T) {
			source, manager := profileHome(t)
			pkg := t.TempDir()
			writeContextPackage(t, pkg, "sample", "1.0.0", "sample\n")
			if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
				t.Fatalf("install = %d: %s", code, stderr)
			}
			if err := envprofile.SetCurrent(manager, "default"); err != nil {
				t.Fatal(err)
			}
			args := []string{"profile", "remove", "sample", "--purge"}
			if mode == "managed-only" {
				args = append(args, "--managed-only")
			}
			if mode == "bound" {
				for _, id := range []string{"claude_code", "codex_cli", "opencode", "pi"} {
					home, err := filepath.EvalSymlinks(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "--native-home", id+"="+home)
				}
			}
			code, _, stderr := runProfile(t, source, args...)
			if mode == "unbound" {
				if code != exitFail || !strings.Contains(stderr, envprofile.DiagNativeHomeBindingRequired) {
					t.Fatalf("unbound purge = %d: %s", code, stderr)
				}
				if _, err := os.Stat(envprofile.ProfileDir(manager, "sample")); err != nil {
					t.Fatalf("unbound purge removed profile: %v", err)
				}
			} else if code != exitOK {
				t.Fatalf("%s purge = %d: %s", mode, code, stderr)
			}
		})
	}
}
