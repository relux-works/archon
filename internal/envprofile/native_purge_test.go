package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envmarker"
	"github.com/relux-works/curator/internal/envregistry"
)

func bindingPurgeMarker(t *testing.T, home, profile, target string) {
	t.Helper()
	copies := []envmarker.Copy{}
	marker := &envmarker.Marker{
		Version:    envmarker.VersionV1,
		Profile:    envmarker.Profile{Name: profile, Root: profile, Kind: "local", LockSHA256: strings.Repeat("a", 64)},
		Members:    []envmarker.Member{{Name: profile, Version: "1.0.0", Commit: strings.Repeat("b", 40), Weight: 0}},
		Precedence: envmarker.Precedence{Winner: "higher-weight", Placement: "winner-last"},
		Mode:       envmarker.ModeLinked,
		Surfaces:   map[string]envmarker.Surface{envmarker.SurfaceRootContext: {Paths: []string{target}, Form: "monolithic", ContentSHA256: "sha256:" + strings.Repeat("c", 64), Copies: &copies}},
	}
	data, err := marker.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, envmarker.Name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, target), []byte("recorded context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func bindingPurgeHomes(t *testing.T) NativeHomeBindings {
	t.Helper()
	bindings := NativeHomeBindings{}
	for _, adapter := range Adapters {
		bindings[adapter.ID] = bindingDirectory(t)
	}
	return bindings
}

// Purge follows all explicit bindings despite managed caller overrides, removes
// only the requested profile's native surfaces and enumerated managed homes,
// and retains foreign markers and unrelated files. No payload is read.
func TestNativeHomeBindingPurgeSelectsIntendedTargets(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	caller := t.TempDir()
	bindingPurgeMarker(t, caller, "sample", "AGENTS.md")
	for _, adapter := range Adapters {
		t.Setenv(adapter.EnvVar, caller)
	}
	bindingPurgeMarker(t, bindings["codex_cli"], "sample", "AGENTS.md")
	bindingPurgeMarker(t, bindings["claude_code"], "foreign", "CLAUDE.md")
	untouched := filepath.Join(bindings["codex_cli"], "unrelated.txt")
	if err := os.WriteFile(untouched, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(EnvRoot(manager), "sample")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := purgeHomesWithBindings(manager, "sample", bindings); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(bindings["codex_cli"], "AGENTS.md"), filepath.Join(bindings["codex_cli"], envmarker.Name), managed} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("bound purge retained %s: %v", path, err)
		}
	}
	for _, path := range []string{filepath.Join(caller, "AGENTS.md"), filepath.Join(caller, envmarker.Name), filepath.Join(bindings["claude_code"], "CLAUDE.md"), filepath.Join(bindings["claude_code"], envmarker.Name), untouched} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("purge changed unselected surface %s: %v", path, err)
		}
	}
}

// Missing/partial bindings refuse before even parsing an earlier corrupt bound
// marker. Both the managed profile and every bound/caller surface remain.
func TestNativeHomeBindingPurgeMissingFailsBeforeInspection(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "none", true: "partial"}[partial], func(t *testing.T) {
			manager := bindingDirectory(t)
			bindings := bindingPurgeHomes(t)
			first := bindings[Adapters[0].ID]
			if err := os.WriteFile(filepath.Join(first, envmarker.Name), []byte("deliberately invalid marker"), 0o600); err != nil {
				t.Fatal(err)
			}
			if partial {
				delete(bindings, Adapters[len(Adapters)-1].ID)
			} else {
				bindings = nil
			}
			managed := filepath.Join(EnvRoot(manager), "sample")
			if err := os.MkdirAll(managed, 0o700); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(managed)
			if err != nil {
				t.Fatal(err)
			}
			err = purgeHomesWithBindings(manager, "sample", bindings)
			if err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
				t.Fatalf("missing binding = %v; want binding refusal before marker inspection", err)
			}
			after, err := os.Lstat(managed)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("refusal changed managed target: %v", err)
			}
		})
	}
}

// A later malformed marker, a managed marker, or an escaping surface parent
// prevents all earlier native deletions. The expected typed refusals are
// separate from the valid purge control above.
func TestNativeHomeBindingPurgePreflightsAllTargets(t *testing.T) {
	for _, scenario := range []string{"late-corrupt-marker", "managed-marker", "escaping-parent"} {
		t.Run(scenario, func(t *testing.T) {
			manager := bindingDirectory(t)
			bindings := bindingPurgeHomes(t)
			first := bindings["claude_code"]
			bindingPurgeMarker(t, first, "sample", "CLAUDE.md")
			later := bindings["codex_cli"]
			bindingPurgeMarker(t, later, "sample", "AGENTS.md")
			want := ""
			switch scenario {
			case "late-corrupt-marker":
				if err := os.WriteFile(filepath.Join(later, envmarker.Name), []byte("invalid marker"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = envmarker.DiagMarkerUnreadable
			case "managed-marker":
				marker, err := envmarker.Read(later)
				if err != nil {
					t.Fatal(err)
				}
				marker.Mode = envmarker.ModeManagedHome
				passthrough := []envmarker.Passthrough{}
				seeds := []string{}
				marker.Passthrough, marker.Seeds = &passthrough, &seeds
				data, err := marker.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(later, envmarker.Name), data, 0o600); err != nil {
					t.Fatal(err)
				}
				want = DiagNativeHomeBindingRequired
			case "escaping-parent":
				foreign := t.TempDir()
				if err := os.WriteFile(filepath.Join(foreign, "outside.md"), []byte("keep\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, filepath.Join(later, "redirect")); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
				marker, err := envmarker.Read(later)
				if err != nil {
					t.Fatal(err)
				}
				surface := marker.Surfaces[envmarker.SurfaceRootContext]
				surface.Paths = []string{"redirect/outside.md"}
				marker.Surfaces[envmarker.SurfaceRootContext] = surface
				data, err := marker.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(later, envmarker.Name), data, 0o600); err != nil {
					t.Fatal(err)
				}
				want = "boundary check failed"
			}
			context := filepath.Join(first, "CLAUDE.md")
			before, err := os.Lstat(context)
			if err != nil {
				t.Fatal(err)
			}
			err = purgeHomesWithBindings(manager, "sample", bindings)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s = %v; want %s", scenario, err, want)
			}
			after, err := os.Lstat(context)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("refusal changed earlier surface: %v", err)
			}
			data, err := os.ReadFile(context)
			if err != nil || string(data) != "recorded context\n" {
				t.Fatalf("refusal changed context bytes: %v", err)
			}
		})
	}
}

// A recorded surface symlink is unlinked within its admitted native root;
// the file it points to outside that root keeps its bytes and identity.
func TestNativeHomeBindingPurgeUnlinksSurfaceWithoutFollowing(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	native := bindings["codex_cli"]
	bindingPurgeMarker(t, native, "sample", "AGENTS.md")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("foreign context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(native, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(native, "AGENTS.md")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if err := purgeHomesWithBindings(manager, "sample", bindings); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(outside)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("purge changed linked foreign target metadata: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "foreign context\n" {
		t.Fatalf("purge changed linked foreign target bytes: %v", err)
	}
}

// A marker cannot promote a native credential path into a context surface.
// Refusal preserves both the synthetic payload and an earlier valid target.
func TestNativeHomeBindingPurgeRejectsCredentialSurface(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	bindingPurgeMarker(t, bindings["claude_code"], "sample", "CLAUDE.md")
	bindingPurgeMarker(t, bindings["codex_cli"], "sample", "auth.json")
	err := purgeHomesWithBindings(manager, "sample", bindings)
	if err == nil || !strings.Contains(err.Error(), envregistry.DiagCredentialConflict) {
		t.Fatalf("credential surface = %v", err)
	}
	for _, path := range []string{filepath.Join(bindings["claude_code"], "CLAUDE.md"), filepath.Join(bindings["codex_cli"], "auth.json")} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "recorded context\n" {
			t.Fatalf("credential refusal changed a target: %v", err)
		}
	}
}
