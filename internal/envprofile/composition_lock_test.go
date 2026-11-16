package envprofile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/config"
	"github.com/relux-works/curator/internal/contextresolve"
	"github.com/relux-works/curator/internal/envregistry"
)

// Production entry points under test: Install, Resolve, UseWithPolicy,
// UpdateWithPolicy, Compose (environments §10.1, §12.1: composition and lock
// agreement, compose publication, the crash window).

// composedFixture writes a path root (acme) and a path overlay (personal) and
// returns the manager home, the root, the overlay, and two policies: one that
// declares the overlay for acme and the root-only one.
func composedFixture(t *testing.T) (home, root, overlay string, with, without Policy) {
	t.Helper()
	home = gitFixtureHome(t)
	pinHomes(t)
	root = filepath.Join(t.TempDir(), "root")
	writeManifestPackage(t, root,
		`{"schema_version": 1, "name": "acme", "version": "1.0.0",`+
			`"context": {"modules": [{"path": "a.md"}]}}`+"\n",
		map[string]string{"a.md": "root\n"})
	overlay = filepath.Join(t.TempDir(), "overlay")
	writeManifestPackage(t, overlay,
		`{"schema_version": 1, "name": "personal", "version": "0.3.0",`+
			`"context": {"modules": [{"path": "a.md"}]}}`+"\n",
		map[string]string{"a.md": "overlay\n"})
	without = Policy{OverlaysAllowed: true, OverlayDefaultWeight: 1000}
	with = Policy{
		OverlaysAllowed:      true,
		OverlayDefaultWeight: 1000,
		Overlays:             map[string][]OverlaySpec{"acme": {{Source: overlay}}},
	}
	return home, root, overlay, with, without
}

// resolveComposed drives the production Resolve for acme on claude_code with
// the given policy, repairing the managed home so a success emits a fragment.
func resolveComposed(t *testing.T, home string, policy Policy) (*ResolveResult, error) {
	t.Helper()
	return Resolve(ResolveRequest{
		Home: home, Profile: "acme", EnvID: "claude_code", Machine: envregistry.DefaultMachineConfig(),
		Policy: policy, LaunchDir: t.TempDir(), Repair: true, Format: "json",
		Detect:       func(envregistry.Adapter) string { return "unknown" },
		NativeHomeOf: func(id string) (string, error) { return filepath.Join(home, "native", id), nil },
		OperatorXDG:  t.TempDir(),
	})
}

func requireMismatch(t *testing.T, err error, member, direction string) {
	t.Helper()
	if err == nil {
		t.Fatalf("resolve admitted a lock that disagrees with the effective overlay declarations (%s %s)", member, direction)
	}
	if !strings.Contains(err.Error(), DiagCompositionLockMismatch) || !strings.Contains(err.Error(), `"acme"`) ||
		!strings.Contains(err.Error(), member) || !strings.Contains(err.Error(), direction) {
		t.Fatalf("err = %v, want %s naming profile acme, member %s, %s", err, DiagCompositionLockMismatch, member, direction)
	}
}

// This proves env resolve refuses with composition_lock_mismatch — naming the
// profile and the disagreeing overlay member as missing from or extra in the
// lock — in both directions, provisions no managed home while refusing, and
// emits a fragment when the declarations and the lock agree (positive
// control beside each refusal).
func TestResolveRefusesCompositionLockMismatch(t *testing.T) {
	t.Run("extra-in-lock", func(t *testing.T) {
		home, root, _, with, without := composedFixture(t)
		if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: with}); err != nil {
			t.Fatal(err)
		}
		managed := ManagedHomeDir(home, "acme", "claude_code")
		result, err := resolveComposed(t, home, without)
		requireMismatch(t, err, "personal", "not declared")
		if result != nil && len(result.Document) != 0 {
			t.Fatalf("refused resolve emitted a fragment: %s", result.Document)
		}
		if _, err := os.Lstat(managed); !os.IsNotExist(err) {
			t.Fatalf("refused resolve provisioned the managed home: %v", err)
		}
		result, err = resolveComposed(t, home, with)
		if err != nil || len(result.Document) == 0 {
			t.Fatalf("agreeing declarations did not emit a fragment: %v", err)
		}
	})
	t.Run("missing-from-lock", func(t *testing.T) {
		home, root, _, with, without := composedFixture(t)
		if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: without}); err != nil {
			t.Fatal(err)
		}
		managed := ManagedHomeDir(home, "acme", "claude_code")
		result, err := resolveComposed(t, home, with)
		requireMismatch(t, err, "personal", "missing from the lock")
		if result != nil && len(result.Document) != 0 {
			t.Fatalf("refused resolve emitted a fragment: %s", result.Document)
		}
		if _, err := os.Lstat(managed); !os.IsNotExist(err) {
			t.Fatalf("refused resolve provisioned the managed home: %v", err)
		}
		result, err = resolveComposed(t, home, without)
		if err != nil || len(result.Document) == 0 {
			t.Fatalf("agreeing root-only composition did not emit a fragment: %v", err)
		}
	})
}

// This proves D6: with overlays_allowed false the declarations are inert, so
// a lock without overlay members agrees with the declared-but-forbidden
// composition on resolve and use (control), while the same forbidding policy
// against a lock that does carry an overlay member is a mismatch.
func TestForbiddenOverlaysMatchRootOnlyLock(t *testing.T) {
	home, root, overlay, with, _ := composedFixture(t)
	forbidding := Policy{
		OverlaysAllowed:      false,
		OverlayDefaultWeight: 1000,
		Overlays:             map[string][]OverlaySpec{"acme": {{Source: overlay}}},
	}
	info, _, _, err := Install(home, InstallOptions{Operand: root, Policy: forbidding})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Lock.Members) != 1 {
		t.Fatalf("forbidding install lock members %+v, want the root alone", info.Lock.Members)
	}
	if result, err := resolveComposed(t, home, forbidding); err != nil || len(result.Document) == 0 {
		t.Fatalf("forbidden declarations against a root-only lock must resolve: %v", err)
	}
	if _, err := UseWithPolicy(home, "acme", "", "", false, forbidding); err != nil {
		t.Fatalf("forbidden declarations against a root-only lock must switch: %v", err)
	}
	// The lock built while overlays were allowed carries the member; the
	// effective root-only composition under the forbidding policy no longer
	// matches it.
	other, otherRoot, _, _, _ := composedFixture(t)
	if _, _, _, err := Install(other, InstallOptions{Operand: otherRoot, Policy: with}); err != nil {
		t.Fatal(err)
	}
	_, err = resolveComposed(t, other, forbidding)
	requireMismatch(t, err, "personal", "not declared")
}

// This proves profile use — machine scope and one environment scope — refuses
// a lock that disagrees with the effective declarations with
// composition_lock_mismatch before any adapter home or current marker
// changes, while the agreeing policy switches (positive control).
func TestUseRefusesCompositionLockMismatch(t *testing.T) {
	home, root, _, with, without := composedFixture(t)
	homes := map[string]string{
		"claude_code": os.Getenv("CLAUDE_CONFIG_DIR"),
		"codex_cli":   os.Getenv("CODEX_HOME"),
		"opencode":    filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode"),
		"pi":          os.Getenv("PI_CODING_AGENT_DIR"),
	}
	if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: with}); err != nil {
		t.Fatal(err)
	}
	beforeHomes := snapshotActivationHomes(t, homes)
	beforeCurrent := snapshotActivationTree(t, CurrentFile(home))
	beforeScoped := snapshotActivationTree(t, ScopedDir(home))
	for _, environment := range []string{"", "codex_cli"} {
		_, err := UseWithPolicy(home, "acme", environment, "", false, without)
		if err == nil || !strings.Contains(err.Error(), DiagCompositionLockMismatch) ||
			!strings.Contains(err.Error(), `"acme"`) || !strings.Contains(err.Error(), "personal") {
			t.Fatalf("use (env %q) with disagreeing declarations = %v, want %s naming acme and personal", environment, err, DiagCompositionLockMismatch)
		}
	}
	if got := snapshotActivationHomes(t, homes); !reflect.DeepEqual(got, beforeHomes) {
		t.Fatal("refused use changed an adapter home")
	}
	if got := snapshotActivationTree(t, CurrentFile(home)); !reflect.DeepEqual(got, beforeCurrent) {
		t.Fatal("refused use changed the machine current marker")
	}
	if got := snapshotActivationTree(t, ScopedDir(home)); !reflect.DeepEqual(got, beforeScoped) {
		t.Fatal("refused use changed scoped current markers")
	}
	results, err := UseWithPolicy(home, "acme", "", "", false, with)
	if err != nil {
		t.Fatalf("agreeing use failed: %v", err)
	}
	for _, result := range results {
		if !result.OK {
			t.Fatalf("%s: %s", result.Adapter, result.Detail)
		}
	}
	if current, err := Current(home); err != nil || current != "acme" {
		t.Fatalf("current = %q, %v; want acme", current, err)
	}
	if got := chapterOrder(t, filepath.Join(homes["claude_code"], "CLAUDE.md")); strings.Join(got, ",") != "acme,personal" {
		t.Fatalf("chapters %v, want the composed pair", got)
	}
}

// This proves install --use of an already installed source is the §9.1
// reinstall: it re-resolves the lock from the effective declarations as
// profile update does, so a disagreeing lock converges instead of refusing,
// and the activation that follows passes the same agreement seam.
func TestInstallUseReinstallConvergesCompositionLock(t *testing.T) {
	home, root, _, with, without := composedFixture(t)
	installed, _, _, err := Install(home, InstallOptions{Operand: root, Policy: with})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lockMember(installed.Lock, "personal"); !ok {
		t.Fatalf("composed install lock %+v carries no overlay", installed.Lock.Members)
	}
	info, activated, updated, err := Install(home, InstallOptions{Operand: root, Use: true, Policy: without})
	if err != nil || !activated || !updated {
		t.Fatalf("reinstall --use activated=%t updated=%t, err=%v", activated, updated, err)
	}
	if _, ok := lockMember(info.Lock, "personal"); ok {
		t.Fatalf("reinstall kept the undeclared overlay in the lock: %+v", info.Lock.Members)
	}
	if lock, _, err := readLock(home, "acme"); err != nil || len(lockedOverlayKeys(lock)) != 0 {
		t.Fatalf("published lock still carries overlay members: %v %v", lock, err)
	}
	if current, err := Current(home); err != nil || current != "acme" {
		t.Fatalf("current = %q, %v; want acme", current, err)
	}
	if got := chapterOrder(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "CLAUDE.md")); strings.Join(got, ",") != "acme" {
		t.Fatalf("chapters %v, want the root alone", got)
	}
}

// This proves the §12.1 crash window: a compose whose declaration write fails
// after its lock publication leaves a lock the declarations disagree with,
// env resolve refuses that state with composition_lock_mismatch, and rerunning
// the same compose converges without moving the lock again; the mirror state
// (declaration present, lock not rebuilt) refuses the same way and converges
// through profile update. compose remove drops the member again.
func TestComposeCrashWindowRefusesThenConverges(t *testing.T) {
	home, root, _, with, without := composedFixture(t)
	if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: without}); err != nil {
		t.Fatal(err)
	}
	// Lock written, declaration not.
	_, err := Compose(home, "acme", ComposeOptions{Policy: with, PublishDeclaration: func() error {
		return errors.New("simulated crash before the declaration write")
	}})
	if err == nil || !strings.Contains(err.Error(), DiagCompositionLockMismatch) || !strings.Contains(err.Error(), "simulated crash") {
		t.Fatalf("compose with a failed declaration write = %v, want the mismatch-naming error", err)
	}
	lock, crashedHash, err := readLock(home, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lockMember(lock, "personal"); !ok {
		t.Fatalf("compose did not publish the rebuilt lock before the declaration write: %+v", lock.Members)
	}
	_, err = resolveComposed(t, home, without)
	requireMismatch(t, err, "personal", "not declared")
	// Rerunning the same compose converges: the lock is already right and
	// stays put; the declaration is written this time.
	declared := false
	result, err := Compose(home, "acme", ComposeOptions{Policy: with, PublishDeclaration: func() error {
		declared = true
		return nil
	}})
	if err != nil || !declared || !result.LockRebuilt || result.LockMoved || result.Info.LockHash != crashedHash {
		t.Fatalf("rerun compose = %+v, declared=%t, err=%v; want the declaration written over an unmoved lock", result, declared, err)
	}
	if res, err := resolveComposed(t, home, with); err != nil || len(res.Document) == 0 {
		t.Fatalf("converged composition did not resolve: %v", err)
	}
	// Declaration present, lock not rebuilt: profile update converges.
	mirror, mirrorRoot, _, mirrorWith, mirrorWithout := composedFixture(t)
	if _, _, _, err := Install(mirror, InstallOptions{Operand: mirrorRoot, Policy: mirrorWithout}); err != nil {
		t.Fatal(err)
	}
	_, err = resolveComposed(t, mirror, mirrorWith)
	requireMismatch(t, err, "personal", "missing from the lock")
	if _, moved, err := UpdateWithPolicy(mirror, "acme", mirrorWith); err != nil || !moved {
		t.Fatalf("profile update moved=%t, err=%v; want the lock rebuilt from the declaration", moved, err)
	}
	if res, err := resolveComposed(t, mirror, mirrorWith); err != nil || len(res.Document) == 0 {
		t.Fatalf("update-converged composition did not resolve: %v", err)
	}
	// compose remove rebuilds the lock without the member and the root-only
	// declaration agrees with it again.
	result, err = Compose(home, "acme", ComposeOptions{Policy: without, PublishDeclaration: func() error { return nil }})
	if err != nil || !result.LockRebuilt || !result.LockMoved {
		t.Fatalf("compose remove = %+v, err=%v; want a moved lock", result, err)
	}
	if _, ok := lockMember(result.Info.Lock, "personal"); ok {
		t.Fatalf("compose remove kept the overlay: %+v", result.Info.Lock.Members)
	}
	if res, err := resolveComposed(t, home, without); err != nil || len(res.Document) == 0 {
		t.Fatalf("root-only composition did not resolve after remove: %v", err)
	}
}

// This proves D5: compose on a profile that has no lock (here: not installed)
// writes only the declaration and creates no profile record, and the first
// install afterwards resolves the overlay jointly with the root.
func TestComposeWithoutLockUpdatesOnlyDeclaration(t *testing.T) {
	home, root, _, with, _ := composedFixture(t)
	declared := false
	result, err := Compose(home, "acme", ComposeOptions{Policy: with, PublishDeclaration: func() error {
		declared = true
		return nil
	}})
	if err != nil || !declared || result.LockRebuilt || result.LockMoved {
		t.Fatalf("compose before a lock = %+v, declared=%t, err=%v; want the declaration alone", result, declared, err)
	}
	if _, err := readSource(home, "acme"); !isStateAbsent(err) {
		t.Fatalf("compose before a lock created a profile record: %v", err)
	}
	info, _, _, err := Install(home, InstallOptions{Operand: root, Policy: with})
	if err != nil {
		t.Fatal(err)
	}
	if member, ok := lockMember(info.Lock, "personal"); !ok || !member.Overlay {
		t.Fatalf("first install did not resolve the declared overlay jointly: %+v", info.Lock.Members)
	}
}

// This proves nothing changes when the complete resulting composition does
// not validate: a compose whose overlay repeats the root's package name is
// environment_composition_invalid, the lock keeps its hash, and the
// declaration write never runs. The converging compose in
// TestComposeCrashWindowRefusesThenConverges is the positive control.
func TestComposeRefusesInvalidCompositionWithoutWriting(t *testing.T) {
	home, root, _, _, without := composedFixture(t)
	if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: without}); err != nil {
		t.Fatal(err)
	}
	_, oldHash, err := readLock(home, "acme")
	if err != nil {
		t.Fatal(err)
	}
	duplicate := filepath.Join(t.TempDir(), "duplicate")
	writeManifestPackage(t, duplicate,
		`{"schema_version": 1, "name": "acme", "version": "9.9.9",`+
			`"context": {"modules": [{"path": "a.md"}]}}`+"\n",
		map[string]string{"a.md": "duplicate\n"})
	invalid := Policy{
		OverlaysAllowed:      true,
		OverlayDefaultWeight: 1000,
		Overlays:             map[string][]OverlaySpec{"acme": {{Source: duplicate}}},
	}
	declared := false
	_, err = Compose(home, "acme", ComposeOptions{Policy: invalid, PublishDeclaration: func() error {
		declared = true
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), contextresolve.DiagCompositionInvalid) {
		t.Fatalf("err = %v, want %s", err, contextresolve.DiagCompositionInvalid)
	}
	if declared {
		t.Fatal("an invalid composition must not write its declaration")
	}
	if _, hash, err := readLock(home, "acme"); err != nil || hash != oldHash {
		t.Fatalf("an invalid composition moved the lock: %s -> %s, %v", oldHash, hash, err)
	}
}

// This proves the hardened source-allowlist gate (manager §7.1, an
// install/update error) applies to compose exactly where compose resolves
// sources: on an installed profile with a lock the rebuild is refused with
// source_allowlist_empty before anything is written — the lock keeps its hash
// and the declaration write never runs — while the same policy on a profile
// without a lock (positive control) writes the declaration alone, since a
// declaration-only edit resolves nothing (environments §12.1). The gate still
// fires when that profile is first installed.
func TestComposeHardenedEmptyAllowlistGatesOnlyLockRebuild(t *testing.T) {
	home, root, _, with, without := composedFixture(t)
	hardened := with
	hardened.SecurityPosture = config.SecurityPostureHardened
	if _, _, _, err := Install(home, InstallOptions{Operand: root, Policy: without}); err != nil {
		t.Fatal(err)
	}
	_, oldHash, err := readLock(home, "acme")
	if err != nil {
		t.Fatal(err)
	}
	declared := false
	result, err := Compose(home, "acme", ComposeOptions{Policy: hardened, PublishDeclaration: func() error {
		declared = true
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), config.DiagSourceAllowlistEmpty) {
		t.Fatalf("hardened compose on a locked profile = %+v, %v; want %s", result, err, config.DiagSourceAllowlistEmpty)
	}
	if declared || result.LockRebuilt {
		t.Fatalf("a refused rebuild wrote: declared=%t, result=%+v", declared, result)
	}
	if _, hash, err := readLock(home, "acme"); err != nil || hash != oldHash {
		t.Fatalf("a refused rebuild moved the lock: %s -> %s, %v", oldHash, hash, err)
	}
	// Positive control: no lock, so nothing is resolved and the declaration
	// alone is written under the same hardened policy.
	other, otherRoot, _, _, _ := composedFixture(t)
	declared = false
	result, err = Compose(other, "acme", ComposeOptions{Policy: hardened, PublishDeclaration: func() error {
		declared = true
		return nil
	}})
	if err != nil || !declared || result.LockRebuilt {
		t.Fatalf("hardened compose before a lock = %+v, declared=%t, err=%v; want the declaration alone", result, declared, err)
	}
	if _, err := readSource(other, "acme"); !isStateAbsent(err) {
		t.Fatalf("hardened compose before a lock created a profile record: %v", err)
	}
	if _, _, _, err := Install(other, InstallOptions{Operand: otherRoot, Policy: hardened}); err == nil || !strings.Contains(err.Error(), config.DiagSourceAllowlistEmpty) {
		t.Fatalf("first install of the composed profile under hardened = %v; want %s", err, config.DiagSourceAllowlistEmpty)
	}
}
