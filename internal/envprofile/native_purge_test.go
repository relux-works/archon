package envprofile

import (
	"errors"
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

// A replacement root is refused against the resolver-admitted identity before
// marker inspection or deletion; the paired no-swap fixture still purges.
// This synthetic witness does not prove a universal filesystem race guarantee.
func TestNativeHomeBindingPurgePinsAdmittedRootIdentity(t *testing.T) {
	for _, swap := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-swap", true: "root-replaced"}[swap], func(t *testing.T) {
			manager := bindingDirectory(t)
			bindings := bindingPurgeHomes(t)
			native := bindings["claude_code"]
			bindingPurgeMarker(t, native, "sample", "AGENTS.md")
			managed := filepath.Join(EnvRoot(manager), "sample")
			if err := os.MkdirAll(managed, 0o700); err != nil {
				t.Fatal(err)
			}
			if !swap {
				if err := purgeHomesWithBindings(manager, "sample", bindings); err != nil {
					t.Fatalf("no-swap purge refused: %v", err)
				}
				for _, path := range []string{filepath.Join(native, "AGENTS.md"), filepath.Join(native, envmarker.Name), managed} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("no-swap purge retained %s: %v", path, err)
					}
				}
				return
			}

			replacement := bindingDirectory(t)
			bindingPurgeMarker(t, replacement, "sample", "AGENTS.md")
			admittedPath := native + ".admitted"
			previous := beforeNativePurgeRootOpen
			defer func() { beforeNativePurgeRootOpen = previous }()
			planted := false
			beforeNativePurgeRootOpen = func(path string) error {
				if path != native || planted {
					return nil
				}
				if err := os.Rename(native, admittedPath); err != nil {
					return err
				}
				if err := os.Symlink(replacement, native); err != nil {
					return err
				}
				planted = true
				return nil
			}
			err := purgeHomesWithBindings(manager, "sample", bindings)
			if !planted || err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
				t.Fatalf("root replacement applied=%v, purge=%v; want admitted-identity refusal", planted, err)
			}
			for _, path := range []string{filepath.Join(admittedPath, "AGENTS.md"), filepath.Join(admittedPath, envmarker.Name), filepath.Join(replacement, "AGENTS.md"), filepath.Join(replacement, envmarker.Name), managed} {
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf("root replacement refusal changed %s: %v", path, err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(admittedPath, "AGENTS.md")); err != nil || string(data) != "recorded context\n" {
				t.Fatalf("admitted fixture content changed: %v", err)
			}
			if data, err := os.ReadFile(filepath.Join(replacement, "AGENTS.md")); err != nil || string(data) != "recorded context\n" {
				t.Fatalf("replacement fixture content changed: %v", err)
			}
		})
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

// A non-empty final surface is rejected before earlier targets change; an
// empty-directory neighbor remains removable. The failure is deterministic
// across privilege levels because os.Root.Remove cannot remove non-empty dirs.
func TestNativeHomeBindingPurgePreflightsRemovability(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-positive", true: "nonempty-refusal"}[nonempty], func(t *testing.T) {
			manager := bindingDirectory(t)
			bindings := bindingPurgeHomes(t)
			first := bindings["claude_code"]
			bindingPurgeMarker(t, first, "sample", "CLAUDE.md")
			last := bindings["pi"]
			bindingPurgeMarker(t, last, "sample", "AGENTS.md")
			marker, err := envmarker.Read(last)
			if err != nil || marker == nil {
				t.Fatalf("read later fixture marker: %v", err)
			}
			surface := marker.Surfaces[envmarker.SurfaceRootContext]
			surface.Paths = []string{"occupied"}
			marker.Surfaces[envmarker.SurfaceRootContext] = surface
			data, err := marker.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(last, envmarker.Name), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(last, "AGENTS.md")); err != nil {
				t.Fatal(err)
			}
			occupied := filepath.Join(last, "occupied")
			if err := os.Mkdir(occupied, 0o700); err != nil {
				t.Fatal(err)
			}
			if nonempty {
				if err := os.WriteFile(filepath.Join(occupied, "keep.txt"), []byte("keep\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			managed := filepath.Join(EnvRoot(manager), "sample")
			if err := os.MkdirAll(managed, 0o700); err != nil {
				t.Fatal(err)
			}
			firstContext := filepath.Join(first, "CLAUDE.md")
			firstMarker := filepath.Join(first, envmarker.Name)
			lastMarker := filepath.Join(last, envmarker.Name)
			firstInfo, err := os.Lstat(firstContext)
			if err != nil {
				t.Fatal(err)
			}
			firstBytes, err := os.ReadFile(firstContext)
			if err != nil {
				t.Fatal(err)
			}
			firstMarkerBytes, err := os.ReadFile(firstMarker)
			if err != nil {
				t.Fatal(err)
			}
			lastMarkerBytes, err := os.ReadFile(lastMarker)
			if err != nil {
				t.Fatal(err)
			}

			err = purgeHomesWithBindings(manager, "sample", bindings)
			if nonempty {
				if err == nil || !strings.Contains(err.Error(), DiagProfilePurgePreflightRefused) || !strings.Contains(err.Error(), "not empty") {
					t.Fatalf("non-empty final surface = %v; want coded preflight refusal", err)
				}
				for _, path := range []string{firstContext, firstMarker, lastMarker, filepath.Join(occupied, "keep.txt"), managed} {
					if _, err := os.Lstat(path); err != nil {
						t.Fatalf("preflight refusal changed %s: %v", path, err)
					}
				}
				afterInfo, err := os.Lstat(firstContext)
				if err != nil || !os.SameFile(firstInfo, afterInfo) || !firstInfo.ModTime().Equal(afterInfo.ModTime()) {
					t.Fatalf("preflight refusal changed earlier target identity or metadata: %v", err)
				}
				for path, want := range map[string]string{firstContext: string(firstBytes), firstMarker: string(firstMarkerBytes), lastMarker: string(lastMarkerBytes), filepath.Join(occupied, "keep.txt"): "keep\n"} {
					got, err := os.ReadFile(path)
					if err != nil || string(got) != want {
						t.Fatalf("preflight refusal changed %s bytes: %v", path, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("empty-directory positive control refused: %v", err)
			}
			for _, path := range []string{filepath.Join(first, "CLAUDE.md"), filepath.Join(first, envmarker.Name), filepath.Join(last, "occupied"), filepath.Join(last, envmarker.Name), managed} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("positive purge retained %s: %v", path, err)
				}
			}
		})
	}
}

// The injected late failure follows a successful earlier-target removal;
// the failing target and managed home remain. This fixture does not claim
// rollback or multi-path atomicity.
func TestNativeHomeBindingPurgeReportsLateRemovalFailure(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	first := bindings["claude_code"]
	bindingPurgeMarker(t, first, "sample", "CLAUDE.md")
	later := bindings["codex_cli"]
	bindingPurgeMarker(t, later, "sample", "AGENTS.md")
	laterMarker := filepath.Join(later, envmarker.Name)
	laterMarkerBytes, err := os.ReadFile(laterMarker)
	if err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(EnvRoot(manager), "sample")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	previous := beforeNativePurgeDelete
	defer func() { beforeNativePurgeDelete = previous }()
	var plantErr error
	planted := false
	beforeNativePurgeDelete = func() {
		if err := os.Remove(filepath.Join(later, "AGENTS.md")); err != nil {
			plantErr = err
			return
		}
		if err := os.Mkdir(filepath.Join(later, "AGENTS.md"), 0o700); err != nil {
			plantErr = err
			return
		}
		if err := os.WriteFile(filepath.Join(later, "AGENTS.md", "keep.txt"), []byte("keep\n"), 0o600); err != nil {
			plantErr = err
			return
		}
		planted = true
	}
	err = purgeHomesWithBindings(manager, "sample", bindings)
	if plantErr != nil || !planted {
		t.Fatalf("late failure fixture applied=%v, err=%v", planted, plantErr)
	}
	if err == nil || !strings.Contains(err.Error(), DiagProfilePurgeIncomplete) || !strings.Contains(err.Error(), "no rollback was attempted") {
		t.Fatalf("late remove failure = %v; want truthful incomplete-purge code", err)
	}
	for _, path := range []string{filepath.Join(first, "CLAUDE.md"), filepath.Join(first, envmarker.Name)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("completed earlier target retained %s: %v", path, err)
		}
	}
	for _, path := range []string{filepath.Join(later, "AGENTS.md", "keep.txt"), laterMarker, managed} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("late failure state missing %s: %v", path, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(later, "AGENTS.md")); err != nil {
		t.Fatalf("late target directory missing: %v", err)
	}
	keepBytes, err := os.ReadFile(filepath.Join(later, "AGENTS.md", "keep.txt"))
	if err != nil || string(keepBytes) != "keep\n" {
		t.Fatalf("late target contents changed: %v", err)
	}
	markerBytes, err := os.ReadFile(laterMarker)
	if err != nil || string(markerBytes) != string(laterMarkerBytes) {
		t.Fatalf("late target marker changed: %v", err)
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

// Each case proves backup purge unlinks a leaf symlink without following an
// existing outside target or a broken target. Intermediate surface links stay
// covered by the separate escaping-parent refusal test.
func TestNativeHomeBindingPurgeUnlinksBackupSymlinkLeaves(t *testing.T) {
	for _, scenario := range []string{"outside-target", "broken-target"} {
		t.Run(scenario, func(t *testing.T) {
			manager := bindingDirectory(t)
			bindings := bindingPurgeHomes(t)
			native := bindings["codex_cli"]
			bindingPurgeMarker(t, native, "sample", "AGENTS.md")
			backupEntry := filepath.Join(native, ".agent-environment-backup", "1")
			if err := os.MkdirAll(backupEntry, 0o700); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "outside.md")
			var before os.FileInfo
			if scenario == "outside-target" {
				if err := os.WriteFile(outside, []byte("foreign context\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				before, err = os.Lstat(outside)
				if err != nil {
					t.Fatal(err)
				}
			}
			link := filepath.Join(backupEntry, "AGENTS.md")
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}

			if err := purgeHomesWithBindings(manager, "sample", bindings); err != nil {
				t.Fatalf("purge backup symlink: %v", err)
			}
			if _, err := os.Lstat(link); !os.IsNotExist(err) {
				t.Fatalf("purge retained backup symlink: %v", err)
			}
			if scenario == "outside-target" {
				after, err := os.Lstat(outside)
				if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
					t.Fatalf("purge changed outside target metadata: %v", err)
				}
				data, err := os.ReadFile(outside)
				if err != nil || string(data) != "foreign context\n" {
					t.Fatalf("purge changed outside target bytes: %v", err)
				}
			} else if _, err := os.Lstat(outside); !os.IsNotExist(err) {
				t.Fatalf("purge changed broken-link target state: %v", err)
			}
		})
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

// A marker replaced by a different file after admission is never deleted by
// name: the purge refuses before its first deletion and every target,
// including the replacement and the managed home, stays in place. Without
// the identity pin the replacement would be removed, so this also witnesses
// that pin. A same-bytes replacement keeps the content check from passing it.
func TestNativeHomeBindingPurgeRefusesMarkerSwappedAfterAdmission(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	first := bindings["claude_code"]
	bindingPurgeMarker(t, first, "sample", "CLAUDE.md")
	later := bindings["codex_cli"]
	bindingPurgeMarker(t, later, "sample", "AGENTS.md")
	laterMarker := filepath.Join(later, envmarker.Name)
	markerBytes, err := os.ReadFile(laterMarker)
	if err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(EnvRoot(manager), "sample")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	previous := beforeNativePurgeDelete
	defer func() { beforeNativePurgeDelete = previous }()
	var plantErr error
	var replacement os.FileInfo
	beforeNativePurgeDelete = func() {
		staged := filepath.Join(later, "marker.replacement")
		if plantErr = os.WriteFile(staged, markerBytes, 0o600); plantErr != nil {
			return
		}
		if plantErr = os.Rename(staged, laterMarker); plantErr != nil {
			return
		}
		replacement, plantErr = os.Lstat(laterMarker)
	}
	err = purgeHomesWithBindings(manager, "sample", bindings)
	if plantErr != nil || replacement == nil {
		t.Fatalf("marker swap fixture applied=%v, err=%v", replacement != nil, plantErr)
	}
	if err == nil || !strings.Contains(err.Error(), DiagProfilePurgePreflightRefused) || !strings.Contains(err.Error(), "native marker changed after admission") {
		t.Fatalf("swapped marker purge = %v; want preflight refusal naming the changed marker", err)
	}
	for _, path := range []string{filepath.Join(first, "CLAUDE.md"), filepath.Join(first, envmarker.Name), filepath.Join(later, "AGENTS.md"), managed} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("refused purge removed %s: %v", path, err)
		}
	}
	now, err := os.Lstat(laterMarker)
	if err != nil || !os.SameFile(replacement, now) {
		t.Fatalf("replacement marker was removed or changed: %v", err)
	}
}

// A bound native directory that is missing at admission reports the binding
// diagnostic code instead of a raw filesystem error; the underlying
// not-exist cause stays inspectable.
func TestNativeHomeBindingAdmissionCodesMissingDirectory(t *testing.T) {
	manager := bindingDirectory(t)
	missing := filepath.Join(bindingDirectory(t), "missing")
	_, err := admitNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": missing}, "codex_cli")
	if err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
		t.Fatalf("missing bound directory = %v; want %s", err, DiagNativeHomeBindingRequired)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing bound directory lost its not-exist cause: %v", err)
	}
}
