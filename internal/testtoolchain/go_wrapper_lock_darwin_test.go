//go:build darwin

package testtoolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type hostGOROOTGoResult struct {
	path string
	err  error
}

type hostGOROOTCaptureTB struct {
	testing.TB
	name  string
	waits chan string
}

func (tb *hostGOROOTCaptureTB) Name() string { return tb.name }

func (tb *hostGOROOTCaptureTB) Logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if strings.HasPrefix(line, "host GOROOT test lock waiting:") {
		select {
		case tb.waits <- line:
		default:
		}
	}
	tb.TB.Logf(format, args...)
}

func (tb *hostGOROOTCaptureTB) Fatalf(format string, args ...any) {
	panic(fmt.Errorf(format, args...))
}

// The public Go helper makes an unrelated same-process test wait while the
// synthetic fingerprint test owns the lock. The log proves the wait path ran;
// a returned Go path or a timeout alone cannot satisfy this control.
func TestGoWrapperLocksUnrelatedSameProcessTest(t *testing.T) {
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	root := filepath.Join(base, "curator-host-goroot-test-lock-v1")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	holderName := t.Name() + "/fingerprint"
	holder, err := acquireHostGOROOT(ctx, root, holderName, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := holder.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()

	callerName := t.Name() + "/go-calling"
	captured := &hostGOROOTCaptureTB{TB: t, name: callerName, waits: make(chan string, 1)}
	result := make(chan hostGOROOTGoResult, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- hostGOROOTGoResult{err: fmt.Errorf("testtoolchain.Go panicked: %v", recovered)}
			}
		}()
		result <- hostGOROOTGoResult{path: Go(captured)}
	}()

	select {
	case line := <-captured.waits:
		if !strings.Contains(line, fmt.Sprintf("owner=%q", callerName)) || !strings.Contains(line, fmt.Sprintf("pid=%d", os.Getpid())) {
			_ = holder.Close()
			t.Fatalf("GO_WRAPPER_LOCK_WAIT_UNATTRIBUTED: wait log = %q", line)
		}
	case got := <-result:
		_ = holder.Close()
		t.Fatalf("GO_WRAPPER_LOCK_DROPPED: unrelated Go caller returned before the fingerprint lock was released: path=%q err=%v", got.path, got.err)
	case <-time.After(5 * time.Second):
		_ = holder.Close()
		select {
		case got := <-result:
			t.Fatalf("GO_WRAPPER_LOCK_WAIT_MISSING: caller returned without reporting a wait: path=%q err=%v", got.path, got.err)
		case <-time.After(10 * time.Second):
			t.Fatal("GO_WRAPPER_LOCK_CONTROL_STALLED: caller neither reported its wait nor returned after holder release")
		}
	}

	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil || got.path == "" {
			t.Fatalf("GO_WRAPPER_LOCK_RELEASE_REACQUIRE_FAILED: path=%q err=%v", got.path, got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GO_WRAPPER_LOCK_RELEASE_REACQUIRE_STALLED: caller did not acquire after holder release")
	}
}

// Removing the lock call from Go must make the same-process control fail at
// its intended assertion, proving the control detects this regression class.
func TestGoWrapperLockDropNarrowingControl(t *testing.T) {
	goPath := Go(t)
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source")
	}
	packageDir := filepath.Dir(sourceFile)
	executablePath := filepath.Join(packageDir, "executable.go")
	original, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	const lockCall = "\tLockHostGOROOT(t)\n"
	if strings.Count(string(original), lockCall) != 1 {
		t.Fatalf("Go lock-call plant anchor count = %d, want 1", strings.Count(string(original), lockCall))
	}
	replacement := filepath.Join(t.TempDir(), "executable.go")
	if err := os.WriteFile(replacement, []byte(strings.Replace(string(original), lockCall, "", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{executablePath: replacement}})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(t.TempDir(), "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(goPath, "test", "-overlay", overlayPath, "-count=1", "-timeout=20s", "-v", "-run", "^TestGoWrapperLocksUnrelatedSameProcessTest$", ".")
	command.Dir = packageDir
	InheritHostGOROOTLock(t, command)
	output, err := command.CombinedOutput()
	exit, isExit := err.(*exec.ExitError)
	if !isExit || exit.ExitCode() != 1 || !strings.Contains(string(output), "GO_WRAPPER_LOCK_DROPPED") || strings.Contains(string(output), "panic:") {
		t.Fatalf("lock-drop mutant did not fail at the intended assertion: err=%v\n%s", err, output)
	}
}

// Go retains a lease under the lock root selected by the TMPDIR in effect at call time, not a root resolved by an earlier call or a different spelling of it.
func TestGoRetainsLeaseUnderTMPDIRInEffect(t *testing.T) {
	controlRoot := filepath.Join(os.TempDir(), "curator-host-goroot-test-lock-v1")
	if path := Go(t); path == "" {
		t.Fatal("control Go call returned no path")
	}
	if hostGOROOTLeaseFor(controlRoot, t.Name()) == nil {
		t.Fatal("Go did not retain the current test's lease under the lock root in effect before the TMPDIR change")
	}
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	changedRoot := filepath.Join(base, "curator-host-goroot-test-lock-v1")
	if hostGOROOTLeaseFor(changedRoot, t.Name()) != nil {
		t.Fatal("a lease under the changed lock root existed before Go was called under it")
	}
	if path := Go(t); path == "" {
		t.Fatal("Go call after the TMPDIR change returned no path")
	}
	if hostGOROOTLeaseFor(changedRoot, t.Name()) == nil {
		t.Fatal("Go did not retain the current test's lease under the lock root selected after the TMPDIR change")
	}
	if hostGOROOTLeaseFor(changedRoot, t.Name()+"x") != nil {
		t.Fatal("the retained lease admitted a lexically-prefixed unrelated test")
	}
}
