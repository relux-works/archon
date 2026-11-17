package testtoolchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const hostGOROOTOverlayMode = "control"

// Reentrant acquisition shares one test and its nested subtests, while sibling
// tests in the same process still wait for the owning test's OS lock.
func TestHostGOROOTLockReentrancyAndSiblingExclusion(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parent, err := acquireHostGOROOT(ctx, root, t.Name()+"/fingerprint", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := parent.Close(); err != nil {
			t.Error(err)
		}
	}()

	for _, name := range []string{t.Name() + "/fingerprint", t.Name() + "/fingerprint/nested"} {
		borrowed, err := acquireHostGOROOT(ctx, root, name, t.Logf)
		if err != nil {
			t.Fatalf("same test scope %q did not reenter: %v", name, err)
		}
		if !borrowed.borrowed {
			t.Fatalf("same test scope %q opened a second OS lock", name)
		}
		if err := borrowed.Close(); err != nil {
			t.Fatalf("borrowed cleanup for %q: %v", name, err)
		}
	}

	before, err := os.ReadFile(filepath.Join(root, "holder.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{t.Name() + "/fingerprinting", t.Name() + "/go-calling"} {
		blockedCtx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
		started := time.Now()
		waiter, err := acquireHostGOROOT(blockedCtx, root, name, func(string, ...any) {})
		stop()
		if waiter != nil {
			_ = waiter.Close()
			t.Fatalf("unrelated same-process test %q bypassed fingerprint lock", name)
		}
		var waitErr *hostGOROOTWaitError
		if !errors.As(err, &waitErr) || waitErr.holder == nil || waitErr.holder.Name != t.Name()+"/fingerprint" {
			t.Fatalf("unrelated test %q did not receive the named refusal: %v", name, err)
		}
		if time.Since(started) < 50*time.Millisecond {
			t.Fatalf("unrelated test %q did not wait for the lock", name)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "holder.json"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("refused sibling changed holder metadata: %v", err)
	}
	if sameTestOrSubtest("TestFoo", "TestFoobar") || !sameTestOrSubtest("TestFoo", "TestFoo/child") {
		t.Fatal("test scope prefix matching confused a lexical prefix or missed a subtest")
	}
}

// A valid marker admits only the selected child test while its parent process
// still owns the recorded OS lock; an unmarked child observes a bounded,
// named refusal. The overlay control must reach its intended mutant assertion.
func TestHostGOROOTLockChildMarkerControls(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("host GOROOT lock is inactive outside macOS; Linux exercises the portable lock fixture")
	}
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	root := filepath.Join(base, "curator-host-goroot-test-lock-v1")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parent, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := parent.Close(); err != nil {
			t.Error(err)
		}
	}()

	goPath := Go(t)
	if runtime.GOOS == "darwin" && hostGOROOTLeaseFor(root, t.Name()) == nil {
		t.Fatal("Go did not retain the current test's host GOROOT lock")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate overlay control source")
	}
	source, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	const original = `const hostGOROOTOverlayMode = "control"`
	const mutant = `const hostGOROOTOverlayMode = "mutant"`
	if !strings.Contains(string(source), original) {
		t.Fatal("overlay control anchor was not present")
	}
	mutated := strings.Replace(string(source), original, mutant, 1)
	mutatedPath := filepath.Join(t.TempDir(), "lock_coverage_mutant_test.go")
	if err := os.WriteFile(mutatedPath, []byte(mutated), 0600); err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(t.TempDir(), "overlay.json")
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{sourceFile: mutatedPath}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, overlay, 0600); err != nil {
		t.Fatal(err)
	}

	markedCtx, stopMarked := context.WithTimeout(context.Background(), 30*time.Second)
	marked := exec.CommandContext(markedCtx, goPath, "test", "-overlay", overlayPath, "-count=1", "-timeout=8s", "-v", "-run", "^TestHostGOROOTMarkerChild$", ".")
	marked.Env = append(os.Environ(), "CURATOR_TEST_GOROOT_CONTROL_ROOT="+root, "CURATOR_TEST_GOROOT_CONTROL_MODE=mutant")
	if !inheritHostGOROOTLockAt(t, marked, root) {
		stopMarked()
		t.Fatal("selected overlay child did not receive its parent lock marker")
	}
	markedOutput, markedErr := marked.CombinedOutput()
	stopMarked()
	if markedErr == nil || !strings.Contains(string(markedOutput), "HOST_GOROOT_OVERLAY_MUTANT_REACHED") || strings.Contains(string(markedOutput), "context deadline exceeded") {
		t.Fatalf("marked overlay child did not reach its intended assertion: err=%v output=%s", markedErr, markedOutput)
	}

	unmarkedCtx, stopUnmarked := context.WithTimeout(context.Background(), 30*time.Second)
	unmarked := exec.CommandContext(unmarkedCtx, goPath, "test", "-count=1", "-timeout=8s", "-v", "-run", "^TestHostGOROOTMarkerChild$", ".")
	unmarked.Env = withoutHostGOROOTMarker(os.Environ())
	unmarked.Env = append(unmarked.Env, "CURATOR_TEST_GOROOT_CONTROL_ROOT="+root, "CURATOR_TEST_GOROOT_CONTROL_MODE=refuse", "CURATOR_TEST_GOROOT_PARENT_PID="+strconv.Itoa(os.Getpid()), "CURATOR_TEST_GOROOT_PARENT_NAME="+t.Name())
	unmarkedOutput, unmarkedErr := unmarked.CombinedOutput()
	stopUnmarked()
	if unmarkedErr != nil || !strings.Contains(string(unmarkedOutput), "HOST_GOROOT_NO_MARKER_REFUSED") {
		t.Fatalf("unmarked child did not complete the expected bounded refusal: err=%v output=%s", unmarkedErr, unmarkedOutput)
	}
}

// The child control either reaches the overlaid sentinel after valid marker
// admission or reports the expected bounded refusal without a marker.
func TestHostGOROOTMarkerChild(t *testing.T) {
	root := os.Getenv("CURATOR_TEST_GOROOT_CONTROL_ROOT")
	if root == "" {
		return
	}
	mode := os.Getenv("CURATOR_TEST_GOROOT_CONTROL_MODE")
	switch mode {
	case "mutant":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		lock, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
		if err != nil {
			t.Fatalf("admitted marker did not share parent lock: %v", err)
		}
		if lock != nil {
			defer func() { _ = lock.Close() }()
		}
		if hostGOROOTOverlayMode != "mutant" {
			t.Fatal("overlay mutation did not apply")
		}
		t.Fatal("HOST_GOROOT_OVERLAY_MUTANT_REACHED")
	case "refuse":
		parentPID, err := strconv.Atoi(os.Getenv("CURATOR_TEST_GOROOT_PARENT_PID"))
		if err != nil {
			t.Fatalf("bad expected parent PID: %v", err)
		}
		parentName := os.Getenv("CURATOR_TEST_GOROOT_PARENT_NAME")
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer cancel()
		started := time.Now()
		lock, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
		if lock != nil {
			_ = lock.Close()
			t.Fatal("unmarked child bypassed the held parent lock")
		}
		var waitErr *hostGOROOTWaitError
		if !errors.As(err, &waitErr) || waitErr.holder == nil || waitErr.holder.PID != parentPID || waitErr.holder.Name != parentName || time.Since(started) < 60*time.Millisecond {
			t.Fatalf("unmarked child did not get the intended bounded holder refusal: %v", err)
		}
		fmt.Println("HOST_GOROOT_NO_MARKER_REFUSED")
	default:
		t.Fatalf("unknown lock control mode %q", mode)
	}
}

// Forged, stale, mismatched, and out-of-scope markers cannot bypass a held
// OS lock; a stale record after release permits a fresh real acquisition.
func TestHostGOROOTMarkerInvalidScenarios(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("host GOROOT lock is inactive outside macOS; Linux exercises the portable marker fixture")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	parent, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := parent.Close(); err != nil {
			t.Error(err)
		}
	}()
	base := hostGOROOTLockMarkerValue{Owner: parent.owner, Tests: []string{"^TestHostGOROOTMarkerNegativeChild$"}}
	makeMarker := func(edit func(*hostGOROOTLockMarkerValue)) string {
		t.Helper()
		value := base
		edit(&value)
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	rows := []struct {
		name   string
		marker string
	}{
		{name: "malformed", marker: "not-json"},
		{name: "wrong-owner", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) { value.Owner.Name = "other-test" })},
		{name: "wrong-pid", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) { value.Owner.PID++ })},
		{name: "stale-pid", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) { value.Owner.PID = 1 << 30 })},
		{name: "stale-owner-time", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) {
			value.Owner.AcquiredAt = value.Owner.AcquiredAt.Add(-time.Second)
		})},
		{name: "wrong-test-scope", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) { value.Tests = []string{"^TestDifferentChild$"} })},
		{name: "lexical-prefix-is-not-child", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) {
			value.Tests = []string{"^TestHostGOROOTMarkerNegativeChildExtra$"}
		})},
		{name: "wrong-process", marker: makeMarker(func(value *hostGOROOTLockMarkerValue) { value.Owner.Process = "different-process" })},
	}
	before, err := os.ReadFile(filepath.Join(root, "holder.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHostGOROOTMarkerNegativeChild$")
			command.Env = withoutHostGOROOTMarker(os.Environ())
			command.Env = append(command.Env, "CURATOR_TEST_GOROOT_NEGATIVE_ROOT="+root, "CURATOR_TEST_GOROOT_NEGATIVE_MODE=reject", "CURATOR_TEST_GOROOT_PARENT_PID="+strconv.Itoa(parent.owner.PID), "CURATOR_TEST_GOROOT_PARENT_NAME="+parent.owner.Name, hostGOROOTLockMarker+"="+row.marker)
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "HOST_GOROOT_MARKER_REJECTED") {
				t.Fatalf("invalid marker bypassed the live holder or failed outside the refusal: %v\n%s", err, output)
			}
			after, err := os.ReadFile(filepath.Join(root, "holder.json"))
			if err != nil || string(after) != string(before) {
				t.Fatalf("rejected marker changed holder metadata: %v", err)
			}
		})
	}

	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeHostGOROOTOwner(filepath.Join(root, "holder.json"), parent.owner); err != nil {
		t.Fatal(err)
	}
	validStale := base
	marker, err := json.Marshal(validStale)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHostGOROOTMarkerNegativeChild$")
	command.Env = withoutHostGOROOTMarker(os.Environ())
	command.Env = append(command.Env, "CURATOR_TEST_GOROOT_NEGATIVE_ROOT="+root, "CURATOR_TEST_GOROOT_NEGATIVE_MODE=free", "CURATOR_TEST_GOROOT_PARENT_PID="+strconv.Itoa(parent.owner.PID), "CURATOR_TEST_GOROOT_PARENT_NAME="+parent.owner.Name, hostGOROOTLockMarker+"="+string(marker))
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "HOST_GOROOT_STALE_MARKER_REJECTED") {
		t.Fatalf("released OS lock with stale matching metadata was inherited: %v\n%s", err, output)
	}
}

// Inheritance removes stale environment markers from non-test and unselected
// commands, and only restores a marker for a bounded go test selection.
func TestInheritHostGOROOTLockRequiresSelectedChild(t *testing.T) {
	root := t.TempDir()
	goPath := Go(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lock, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, command := range []*exec.Cmd{
		exec.Command("echo", "child"),
		exec.Command(goPath, "test", "-count=1", "."),
		exec.Command(goPath, "test", "-run", "", "."),
	} {
		command.Env = append(os.Environ(), hostGOROOTLockMarker+"=forged")
		if inheritHostGOROOTLockAt(t, command, root) {
			t.Fatalf("unselected command %q received a host-lock marker", command.Args)
		}
		for _, entry := range command.Env {
			if strings.HasPrefix(entry, hostGOROOTLockMarker+"=") {
				t.Fatalf("unselected command %q retained a forged marker", command.Args)
			}
		}
	}
	selected := exec.Command(goPath, "test", "-run", "^TestChildControl$", ".")
	if !inheritHostGOROOTLockAt(t, selected, root) {
		t.Fatal("bounded go test child did not receive the held lock marker")
	}
	count := 0
	for _, entry := range selected.Env {
		if strings.HasPrefix(entry, hostGOROOTLockMarker+"=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("selected child has %d marker values, want one", count)
	}
}

// The negative child accepts only the intended wait/refusal for an active
// holder, or a new non-borrowed acquisition after that holder has released.
func TestHostGOROOTMarkerNegativeChild(t *testing.T) {
	root := os.Getenv("CURATOR_TEST_GOROOT_NEGATIVE_ROOT")
	if root == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	lock, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	if os.Getenv("CURATOR_TEST_GOROOT_NEGATIVE_MODE") == "free" {
		if err != nil || lock == nil || lock.borrowed || lock.owner.PID != os.Getpid() {
			if lock != nil {
				_ = lock.Close()
			}
			t.Fatalf("stale marker was not rejected in favor of a fresh acquisition: lock=%+v err=%v", lock, err)
		}
		if err := lock.Close(); err != nil {
			t.Fatal(err)
		}
		fmt.Println("HOST_GOROOT_STALE_MARKER_REJECTED")
		return
	}
	if lock != nil {
		_ = lock.Close()
		t.Fatal("invalid marker borrowed the active parent's OS lock")
	}
	var waitErr *hostGOROOTWaitError
	parentPID, _ := strconv.Atoi(os.Getenv("CURATOR_TEST_GOROOT_PARENT_PID"))
	if !errors.As(err, &waitErr) || waitErr.holder == nil || waitErr.holder.PID != parentPID || waitErr.holder.Name != os.Getenv("CURATOR_TEST_GOROOT_PARENT_NAME") {
		t.Fatalf("invalid marker did not preserve the named holder refusal: %v", err)
	}
	fmt.Println("HOST_GOROOT_MARKER_REJECTED")
}

// A lease registered under a resolved lock root is found through another spelling of that same root, and not through a different root or for an unrelated test.
func TestHostGOROOTLeaseForResolvesRootAlias(t *testing.T) {
	realRoot := filepath.Join(t.TempDir(), "lock-root")
	if err := os.MkdirAll(realRoot, 0700); err != nil {
		t.Fatal(err)
	}
	var aliasRoot string
	if runtime.GOOS == "windows" {
		aliasRoot = strings.ToUpper(realRoot)
		if aliasRoot == realRoot {
			aliasRoot = strings.ToLower(realRoot)
		}
		realInfo, realErr := os.Stat(realRoot)
		aliasInfo, aliasErr := os.Stat(aliasRoot)
		if realErr != nil || aliasErr != nil || !os.SameFile(realInfo, aliasInfo) {
			// A case-sensitive directory makes the recased spelling a
			// different root by design; the alias proof then stays on
			// the acquiring spelling and Unix covers the alias case.
			aliasRoot = realRoot
		}
	} else {
		aliasRoot = filepath.Join(t.TempDir(), "lock-root-alias")
		if err := os.Symlink(realRoot, aliasRoot); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lock, err := acquireHostGOROOT(ctx, realRoot, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	if hostGOROOTLeaseFor(realRoot, t.Name()) == nil {
		t.Fatal("lease was not found through the root used to acquire it")
	}
	if hostGOROOTLeaseFor(aliasRoot, t.Name()) == nil {
		t.Fatal("lease was not found through an alias of its lock root")
	}
	if hostGOROOTLeaseFor(t.TempDir(), t.Name()) != nil {
		t.Fatal("lease was found through a different lock root")
	}
	if hostGOROOTLeaseFor(aliasRoot, t.Name()+"x") != nil {
		t.Fatal("lease admitted a lexically-prefixed unrelated test")
	}
}

// A cleanup release line does not replace a skipped test's recorded reason, while a passing control still reports the release.
func TestHostGOROOTLockReleaseDoesNotReplaceSkipReason(t *testing.T) {
	const skipReason = "release control skip reason"
	attributed := regexp.MustCompile(`[^\s]+\.go:[0-9]+: (.*)$`)
	runChild := func(mode string) (string, []string) {
		t.Helper()
		root := t.TempDir()
		command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHostGOROOTLockSkipReasonChild$", "-test.v")
		command.Env = append(withoutHostGOROOTMarker(os.Environ()), "CURATOR_TEST_GOROOT_SKIP_ROOT="+root, "CURATOR_TEST_GOROOT_SKIP_MODE="+mode)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("skip-reason child mode %q failed: %v\n%s", mode, err, output)
		}
		var reasons []string
		for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(line, "--- SKIP:") || strings.HasPrefix(line, "--- PASS:") {
				break
			}
			if match := attributed.FindStringSubmatch(line); match != nil {
				reasons = append(reasons, match[1])
			}
		}
		return string(output), reasons
	}
	skippedOutput, skippedReasons := runChild("skip")
	if len(skippedReasons) == 0 || skippedReasons[len(skippedReasons)-1] != skipReason {
		t.Fatalf("last attributed line before the skip result = %q, want the skip reason; output:\n%s", skippedReasons, skippedOutput)
	}
	if !strings.Contains(skippedOutput, "host GOROOT test lock released:") {
		t.Fatalf("release diagnostic is missing from the skipped child output:\n%s", skippedOutput)
	}
	for _, line := range strings.Split(skippedOutput, "\n") {
		if strings.Contains(line, "host GOROOT test lock released:") && attributed.MatchString(line) {
			t.Fatalf("release diagnostic is file-attributed and can replace a skip reason: %q", line)
		}
	}
	passedOutput, _ := runChild("pass")
	if !strings.Contains(passedOutput, "--- PASS: TestHostGOROOTLockSkipReasonChild") || !strings.Contains(passedOutput, "host GOROOT test lock released:") {
		t.Fatalf("passing control did not pass with its release diagnostic:\n%s", passedOutput)
	}
}

// The skip-reason child holds the lock under a cleanup, then skips or passes as directed by its parent.
func TestHostGOROOTLockSkipReasonChild(t *testing.T) {
	root := os.Getenv("CURATOR_TEST_GOROOT_SKIP_ROOT")
	if root == "" {
		return
	}
	lock, err := acquireHostGOROOT(context.Background(), root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	switch os.Getenv("CURATOR_TEST_GOROOT_SKIP_MODE") {
	case "skip":
		t.Skip("release control skip reason")
	case "pass":
	default:
		t.Fatalf("unknown skip-reason child mode %q", os.Getenv("CURATOR_TEST_GOROOT_SKIP_MODE"))
	}
}
