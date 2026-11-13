package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envmarker"
	"github.com/relux-works/curator/internal/envprofile"
)

func nativeBindingCLIDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func nativeBindingCLIMarker(t *testing.T, home string) {
	t.Helper()
	copies := []envmarker.Copy{}
	marker := &envmarker.Marker{Version: envmarker.VersionV1, Profile: envmarker.Profile{Name: "sample", Root: "sample", Kind: "local", LockSHA256: strings.Repeat("a", 64)}, Members: []envmarker.Member{{Name: "sample", Version: "1.0.0", Commit: strings.Repeat("b", 40)}}, Precedence: envmarker.Precedence{Winner: "higher-weight", Placement: "winner-last"}, Mode: envmarker.ModeLinked, Surfaces: map[string]envmarker.Surface{envmarker.SurfaceRootContext: {Paths: []string{"AGENTS.md"}, Form: "monolithic", ContentSHA256: "sha256:" + strings.Repeat("c", 64), Copies: &copies}}}
	data, err := marker.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, envmarker.Name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("bound context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Persisted knobs alone target the admitted native directories. An explicit
// CLI value wins for just that adapter; the old knob target and the decoy
// caller marker remain unchanged. The selected config bytes remain unchanged.
func TestNativeHomeBindingKnobAndCLIPrecedence(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "knob-only", true: "cli-wins"}[override], func(t *testing.T) {
			source, manager := profileHome(t)
			original := []byte("{\"schema_version\":2,\"skills_root\":\"unused\",\"projects\":{}}\n")
			if err := os.WriteFile(source.path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			pkg := t.TempDir()
			writeContextPackage(t, pkg, "sample", "1.0.0", "sample\n")
			if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
				t.Fatalf("install=%d: %s", code, stderr)
			}
			if err := envprofile.SetCurrent(manager, "default"); err != nil {
				t.Fatal(err)
			}
			homes := map[string]string{}
			for _, id := range []string{"claude_code", "codex_cli", "opencode", "pi"} {
				homes[id] = nativeBindingCLIDirectory(t)
				if code, stdout, stderr := runProfile(t, source, "env", "config", "set", "native_home."+id, homes[id]); code != exitOK || !strings.Contains(stdout, envprofile.NativeHomeBindingsPath(source.path)) {
					t.Fatalf("knob set=%d: %s %s", code, stdout, stderr)
				}
			}
			nativeBindingCLIMarker(t, homes["codex_cli"])
			args := []string{"profile", "remove", "sample", "--purge"}
			selected := homes["codex_cli"]
			if override {
				selected = nativeBindingCLIDirectory(t)
				nativeBindingCLIMarker(t, selected)
				args = append(args, "--native-home", "codex_cli="+selected)
			}
			decoy := nativeBindingCLIDirectory(t)
			badMarker := []byte("deliberately invalid decoy marker")
			if err := os.WriteFile(filepath.Join(decoy, envmarker.Name), badMarker, 0o600); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_CONFIG_HOME", "PI_CODING_AGENT_DIR"} {
				t.Setenv(key, decoy)
			}
			if code, _, stderr := runProfile(t, source, args...); code != exitOK {
				t.Fatalf("bound purge=%d: %s", code, stderr)
			}
			if _, err := os.Lstat(filepath.Join(selected, "AGENTS.md")); !os.IsNotExist(err) {
				t.Fatalf("selected native target retained: %v", err)
			}
			if override {
				if _, err := os.Lstat(filepath.Join(homes["codex_cli"], "AGENTS.md")); err != nil {
					t.Fatalf("CLI override changed old knob target: %v", err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(decoy, envmarker.Name)); err != nil || !bytes.Equal(data, badMarker) {
				t.Fatalf("bound purge changed caller marker: %v", err)
			}
			if data, err := os.ReadFile(source.path); err != nil || !bytes.Equal(data, original) {
				t.Fatalf("knob changed old config carrier: %v", err)
			}
		})
	}
}

// Native knob show/unset reports its source, refuses unknown IDs without a
// partial update, and does not rewrite or promote the old config carrier.
func TestNativeHomeBindingKnobCommands(t *testing.T) {
	source, _ := profileHome(t)
	home := nativeBindingCLIDirectory(t)
	if code, stdout, stderr := runProfile(t, source, "env", "config", "show", "native_home.codex_cli"); code != exitOK || !strings.Contains(stdout, `"source": "none"`) {
		t.Fatalf("missing knob show=%d: %s %s", code, stdout, stderr)
	}
	if code, _, stderr := runProfile(t, source, "env", "config", "set", "native_home.codex_cli", home); code != exitOK {
		t.Fatalf("set=%d: %s", code, stderr)
	}
	before, err := os.ReadFile(envprofile.NativeHomeBindingsPath(source.path))
	if err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runProfile(t, source, "env", "config", "set", "native_home.unknown", home); code != exitUsage || !strings.Contains(stderr, envprofile.DiagNativeHomeBindingRequired) {
		t.Fatalf("unknown knob=%d: %s", code, stderr)
	}
	after, err := os.ReadFile(envprofile.NativeHomeBindingsPath(source.path))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unknown knob changed sidecar: %v", err)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "config", "show", "native_home"); code != exitOK || !strings.Contains(stdout, home) {
		t.Fatalf("knob map show=%d: %s %s", code, stdout, stderr)
	}
	if code, _, stderr := runProfile(t, source, "env", "config", "unset", "native_home.codex_cli"); code != exitOK {
		t.Fatalf("unset=%d: %s", code, stderr)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "config", "show", "native_home.codex_cli"); code != exitOK || !strings.Contains(stdout, `"value": null`) {
		t.Fatalf("unset knob show=%d: %s %s", code, stdout, stderr)
	}
}
