package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/config"
	"github.com/relux-works/archon/internal/envprofile"
)

// These tests drive the production run() entry point for the profile compose
// row against an installed profile (cli/curator.md, environments §10.1 and
// §12.1): the lock follows the declaration, env resolve and profile use refuse
// a disagreement, and the crash window converges.

// composedCLIFixture installs the path root acme without --use under a schema
// 2 machine file and returns its config source, the overlay directory, and the
// manager home.
func composedCLIFixture(t *testing.T) (stubConfigSource, string, string) {
	t.Helper()
	source, home := profileHome(t)
	writeNativeCredentials(t)
	if err := os.WriteFile(source.path, []byte(`{"schema_version": 2, "security_posture": "permissive", "skills_root": "x", "projects": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source = reloadSource(t, source)
	root := t.TempDir()
	writeContextPackage(t, root, "acme", "1.0.0", "root\n")
	overlay := t.TempDir()
	writeContextPackage(t, overlay, "personal", "0.3.0", "overlay\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", root); code != exitOK {
		t.Fatalf("install = %d\nstderr:\n%s", code, stderr)
	}
	return source, overlay, home
}

// installedLock returns the installed profile's lock through the production
// listing.
func installedLock(t *testing.T, home, profile string) envprofile.Info {
	t.Helper()
	profiles, err := envprofile.ListWithPolicy(home, envprofile.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range profiles {
		if info.Name == profile {
			return info
		}
	}
	t.Fatalf("profile %s is not installed", profile)
	return envprofile.Info{}
}

// lockOverlayNames returns the overlay member names of the installed lock.
func lockOverlayNames(t *testing.T, home, profile string) []string {
	t.Helper()
	var names []string
	for _, member := range installedLock(t, home, profile).Lock.Members {
		if member.Overlay {
			names = append(names, member.Name)
		}
	}
	return names
}

// setDeclaredOverlays rewrites overlays.<profile> in the machine file directly,
// bypassing compose, to manufacture a declaration/lock disagreement.
func setDeclaredOverlays(t *testing.T, source stubConfigSource, profile string, sources ...string) stubConfigSource {
	t.Helper()
	object, err := config.ReadRaw(source.path)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := object["environments"].(map[string]any)
	if env == nil {
		env = map[string]any{}
		object["environments"] = env
	}
	overlays, _ := env["overlays"].(map[string]any)
	if overlays == nil {
		overlays = map[string]any{}
		env["overlays"] = overlays
	}
	list := make([]any, 0, len(sources))
	for _, overlay := range sources {
		list = append(list, map[string]any{"source": overlay})
	}
	overlays[profile] = list
	if _, err := config.WriteRaw(source.path, object); err != nil {
		t.Fatal(err)
	}
	return reloadSource(t, source)
}

// This proves compose add and remove on an installed profile rebuild its lock
// from the complete resulting declaration — the overlay member appears and
// disappears with the declaration — and env resolve emits a fragment after
// each, while the uninstalled-profile row still changes only the declaration.
func TestProfileComposeRebuildsInstalledProfileLock(t *testing.T) {
	source, overlay, home := composedCLIFixture(t)
	if names := lockOverlayNames(t, home, "acme"); len(names) != 0 {
		t.Fatalf("fresh root-only lock carries overlays %v", names)
	}
	code, stdout, stderr := runProfile(t, source, "profile", "compose", "acme", "add", overlay)
	if code != exitOK || !strings.Contains(stdout, "added overlay") || !strings.Contains(stdout, "; lock sha256:") {
		t.Fatalf("compose add = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	source = reloadSource(t, source)
	if names := lockOverlayNames(t, home, "acme"); strings.Join(names, ",") != "personal" {
		t.Fatalf("lock overlays after add = %v, want personal", names)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme", "--repair"); code != exitOK {
		t.Fatalf("resolve after add = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	code, stdout, stderr = runProfile(t, source, "profile", "compose", "acme", "remove", overlay)
	if code != exitOK || !strings.Contains(stdout, "removed overlay") || !strings.Contains(stdout, "; lock sha256:") {
		t.Fatalf("compose remove = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	source = reloadSource(t, source)
	if names := lockOverlayNames(t, home, "acme"); len(names) != 0 {
		t.Fatalf("lock overlays after remove = %v, want none", names)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme", "--repair"); code != exitOK {
		t.Fatalf("resolve after remove = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	// An uninstalled profile has no lock: only the declaration changes.
	code, stdout, stderr = runProfile(t, source, "profile", "compose", "other", "add", overlay)
	if code != exitOK || !strings.Contains(stdout, "the profile has no lock") {
		t.Fatalf("compose add on an uninstalled profile = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	source = reloadSource(t, source)
	if decls := source.cfg.Env.Overlays["other"]; len(decls) != 1 || decls[0].Source != overlay {
		t.Fatalf("declaration for the uninstalled profile: %+v", source.cfg.Env.Overlays)
	}
	if _, err := os.Stat(envprofile.ProfileDir(home, "other")); !os.IsNotExist(err) {
		t.Fatalf("compose on an uninstalled profile created a profile record: %v", err)
	}
}

// This proves the crash window through run(): after the lock is rebuilt but
// the declaration is lost, env resolve refuses with composition_lock_mismatch
// naming the profile and the extra member and emits no fragment; rerunning
// the same compose add converges with the lock unmoved and the declaration
// written once. The mirror state — a declaration written while the lock still
// lacks the member — refuses naming the missing member, and profile update
// converges it.
func TestProfileComposeCrashWindowRefusesAndConverges(t *testing.T) {
	source, overlay, home := composedCLIFixture(t)
	if code, _, stderr := runProfile(t, source, "profile", "compose", "acme", "add", overlay); code != exitOK {
		t.Fatalf("compose add = %d\nstderr:\n%s", code, stderr)
	}
	// Lock written, declaration not: drop the declaration behind compose's back.
	source = setDeclaredOverlays(t, source, "acme")
	code, stdout, stderr := runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme", "--repair")
	if code != exitFail || stdout != "" {
		t.Fatalf("resolve on a disagreeing lock = %d, stdout %q; want %d and no fragment", code, stdout, exitFail)
	}
	if !strings.Contains(stderr, envprofile.DiagCompositionLockMismatch) || !strings.Contains(stderr, `"acme"`) ||
		!strings.Contains(stderr, "personal") || !strings.Contains(stderr, "not declared") {
		t.Fatalf("stderr must name composition_lock_mismatch, acme, and the extra member personal:\n%s", stderr)
	}
	lockBefore := installedLock(t, home, "acme").LockHash
	code, stdout, stderr = runProfile(t, source, "profile", "compose", "acme", "add", overlay)
	if code != exitOK || !strings.Contains(stdout, "lock unchanged") {
		t.Fatalf("rerun compose add = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	source = reloadSource(t, source)
	if installedLock(t, home, "acme").LockHash != lockBefore {
		t.Fatal("rerun compose add moved an already converged lock")
	}
	if decls := source.cfg.Env.Overlays["acme"]; len(decls) != 1 {
		t.Fatalf("rerun compose add must declare the overlay exactly once: %+v", decls)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme", "--repair"); code != exitOK {
		t.Fatalf("resolve after convergence = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	// Declaration written, lock not: remove through compose, then put the
	// declaration back behind compose's back.
	if code, _, stderr := runProfile(t, source, "profile", "compose", "acme", "remove", overlay); code != exitOK {
		t.Fatalf("compose remove = %d\nstderr:\n%s", code, stderr)
	}
	source = setDeclaredOverlays(t, source, "acme", overlay)
	code, stdout, stderr = runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme")
	if code != exitFail || stdout != "" || !strings.Contains(stderr, envprofile.DiagCompositionLockMismatch) ||
		!strings.Contains(stderr, "personal") || !strings.Contains(stderr, "missing from the lock") {
		t.Fatalf("resolve on a lock lacking the declared member = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if code, stdout, stderr := runProfile(t, source, "profile", "update", "acme"); code != exitOK {
		t.Fatalf("profile update = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if names := lockOverlayNames(t, home, "acme"); strings.Join(names, ",") != "personal" {
		t.Fatalf("lock overlays after update = %v, want personal", names)
	}
	if code, stdout, stderr := runProfile(t, source, "env", "resolve", "codex_cli", "--profile", "acme", "--repair"); code != exitOK {
		t.Fatalf("resolve after update = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// This proves through run() that the hardened source-allowlist gate (manager
// §7.1, an install/update error) refuses compose add only where it rebuilds a
// lock: on the installed, locked profile the row fails with
// source_allowlist_empty and leaves the machine file and lock.json
// byte-identical, while the same row on a profile without a lock under the
// same hardened configuration writes the declaration (positive control;
// TestProfileComposeAddListRemove and TestProfileComposeListDirectoryRoundTrip
// are the same control under the default posture).
func TestProfileComposeHardenedEmptyAllowlistRefusesOnlyLockedProfile(t *testing.T) {
	t.Setenv("CURATOR_SYSTEM_CONFIG", "")
	source, overlay, home := composedCLIFixture(t)
	object, err := config.ReadRaw(source.path)
	if err != nil {
		t.Fatal(err)
	}
	object["security_posture"] = config.SecurityPostureHardened
	object["allowed_sources"] = []any{}
	if _, err := config.WriteRaw(source.path, object); err != nil {
		t.Fatal(err)
	}
	source = reloadSource(t, source)
	lockPath := filepath.Join(envprofile.ProfileDir(home, "acme"), "lock.json")
	declarationBefore, err := os.ReadFile(source.path)
	if err != nil {
		t.Fatal(err)
	}
	lockBefore, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runProfile(t, source, "profile", "compose", "acme", "add", overlay)
	if code != exitFail || stdout != "" || !strings.Contains(stderr, config.DiagSourceAllowlistEmpty) {
		t.Fatalf("hardened compose add on a locked profile = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if after, err := os.ReadFile(source.path); err != nil || !bytes.Equal(after, declarationBefore) {
		t.Fatalf("refused compose changed the declaration (%v):\n%s", err, after)
	}
	if after, err := os.ReadFile(lockPath); err != nil || !bytes.Equal(after, lockBefore) {
		t.Fatalf("refused compose changed the lock (%v):\n%s", err, after)
	}
	if names := lockOverlayNames(t, home, "acme"); len(names) != 0 {
		t.Fatalf("refused compose rebuilt the lock with overlays %v", names)
	}
	// Positive control: the same edit on a profile without a lock resolves no
	// source, so the hardened gate does not apply and only the declaration
	// changes.
	code, stdout, stderr = runProfile(t, source, "profile", "compose", "other", "add", overlay)
	if code != exitOK || !strings.Contains(stdout, "the profile has no lock") {
		t.Fatalf("hardened compose add on a profile without a lock = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	source = reloadSource(t, source)
	if decls := source.cfg.Env.Overlays["other"]; len(decls) != 1 || decls[0].Source != overlay {
		t.Fatalf("declaration for the profile without a lock: %+v", source.cfg.Env.Overlays)
	}
	if _, err := os.Stat(envprofile.ProfileDir(home, "other")); !os.IsNotExist(err) {
		t.Fatalf("compose on a profile without a lock created a profile record: %v", err)
	}
	if code, _, stderr := runProfile(t, source, "profile", "compose", "other", "remove", overlay); code != exitOK {
		t.Fatalf("hardened compose remove on a profile without a lock = %d\nstderr:\n%s", code, stderr)
	}
}

// This proves profile use through run() refuses a disagreeing lock with
// composition_lock_mismatch, recording no current, and switches once the
// declaration is converged (positive control).
func TestProfileUseRefusesCompositionLockMismatchRow(t *testing.T) {
	source, overlay, home := composedCLIFixture(t)
	if code, _, stderr := runProfile(t, source, "profile", "compose", "acme", "add", overlay); code != exitOK {
		t.Fatalf("compose add = %d\nstderr:\n%s", code, stderr)
	}
	source = setDeclaredOverlays(t, source, "acme")
	code, stdout, stderr := runProfile(t, source, "profile", "use", "acme")
	if code != exitFail || !strings.Contains(stderr, envprofile.DiagCompositionLockMismatch) || strings.Contains(stdout, "switched") {
		t.Fatalf("profile use on a disagreeing lock = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if current, err := envprofile.Current(home); err != nil || current != "" {
		t.Fatalf("refused use recorded current %q, %v", current, err)
	}
	if code, stdout, stderr := runProfile(t, source, "profile", "update", "acme"); code != exitOK {
		t.Fatalf("profile update = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if code, stdout, stderr := runProfile(t, source, "profile", "use", "acme"); code != exitOK {
		t.Fatalf("profile use after convergence = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if current, err := envprofile.Current(home); err != nil || current != "acme" {
		t.Fatalf("current = %q, %v; want acme", current, err)
	}
}
