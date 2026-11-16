package testtoolchain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The run matcher admits a selected parent for a slash-qualified child pattern while preserving the selected tree boundary.
func TestHostGOROOTRunPatternMatchesSelectedParentScope(t *testing.T) {
	pattern := `^TestParent$/^child$`
	for _, name := range []string{"TestParent", "TestParent/child", "TestParent/child/grandchild"} {
		if !hostGOROOTRunPatternMatches(pattern, name) {
			t.Errorf("selected name %q was refused by %q", name, pattern)
		}
	}
	for _, name := range []string{"Other/child", "TestParentElse/child", "TestParent/other"} {
		if hostGOROOTRunPatternMatches(pattern, name) {
			t.Errorf("out-of-scope name %q was admitted by %q", name, pattern)
		}
	}
}

// Reuse accepts only an exact, live holder that still owns the OS lock; this fixture uses a private temporary lock root.
func TestHostGOROOTInheritedLeaseStillHeldChecksAllWitnesses(t *testing.T) {
	root := t.TempDir()
	lock, err := acquireHostGOROOT(context.Background(), root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	owner := lock.owner
	if !hostGOROOTInheritedLeaseStillHeld(root, owner) {
		t.Fatal("live exact holder and OS lock were not accepted")
	}

	wrong := owner
	wrong.Name += "/changed"
	if err := writeHostGOROOTOwner(filepath.Join(root, "holder.json"), wrong); err != nil {
		t.Fatal(err)
	}
	if hostGOROOTInheritedLeaseStillHeld(root, owner) {
		t.Fatal("mismatched holder metadata was accepted")
	}
	if err := writeHostGOROOTOwner(filepath.Join(root, "holder.json"), owner); err != nil {
		t.Fatal(err)
	}
	if !hostGOROOTInheritedLeaseStillHeld(root, owner) {
		t.Fatal("restored exact holder metadata was not accepted while the lock remained held")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeHostGOROOTOwner(filepath.Join(root, "holder.json"), owner); err != nil {
		t.Fatal(err)
	}
	if hostGOROOTInheritedLeaseStillHeld(root, owner) {
		t.Fatal("available OS lock was accepted as still held")
	}
}

// This test process acquires the private fixture lock so the controller can verify inheritance before terminating it.
func TestR11HostGOROOTInheritanceParent(t *testing.T) {
	root := os.Getenv("CURATOR_R11_HOST_GOROOT_ROOT")
	if root == "" {
		return
	}
	lock, err := acquireHostGOROOT(context.Background(), root, t.Name(), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "R11_PARENT_READY"); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

// The child first proves live inheritance, then must drop the cached lease and acquire normally after its owner exits.
func TestR11HostGOROOTInheritanceChild(t *testing.T) {
	root := os.Getenv("CURATOR_R11_HOST_GOROOT_ROOT")
	if root == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	first, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	cancel()
	if err != nil {
		t.Fatalf("R11_LIVE_PARENT_INHERIT_FAILED: %v", err)
	}
	if !first.borrowed {
		t.Fatal("R11_LIVE_PARENT_CONTROL_NOT_BORROWED")
	}
	t.Cleanup(func() {
		_ = first.Close()
		releaseInheritedHostGOROOTLease(root, first.owner)
	})
	if _, err := fmt.Fprintln(os.Stdout, "R11_CHILD_FIRST_BORROWED"); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatalf("R11_CHILD_RETRY_SIGNAL_FAILED: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	second, err := acquireHostGOROOT(ctx, root, t.Name(), t.Logf)
	cancel()
	if err != nil {
		t.Fatalf("R11_SECOND_CHILD_ACQUIRE_FAILED: %v", err)
	}
	if second.borrowed {
		t.Fatal("R11_STALE_INHERITED_LEASE_REUSED")
	}
	if second.owner.PID != os.Getpid() {
		t.Fatalf("second acquisition owner PID=%d, want child PID=%d", second.owner.PID, os.Getpid())
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "R11_CHILD_SECOND_ACQUIRED"); err != nil {
		t.Fatal(err)
	}
}

// A second child acquisition after owner death drops the inherited cache and obtains its own lock.
func TestHostGOROOTInheritedLeaseIsDroppedAfterParentDeath(t *testing.T) {
	root := t.TempDir()
	parentPID, releaseParent := startHostGOROOTHolder(t, root)
	owner := readHostGOROOTOwner(filepath.Join(root, "holder.json"))
	if owner == nil || owner.PID != parentPID {
		t.Fatalf("fixture holder does not identify its owned parent PID %d", parentPID)
	}
	if !hostGOROOTInheritedLeaseStillHeld(root, *owner) {
		t.Fatal("live fixture parent did not hold the lock")
	}
	marker, err := json.Marshal(hostGOROOTLockMarkerValue{Owner: *owner, Tests: []string{`^TestR11HostGOROOTInheritanceChild$`}})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestR11HostGOROOTInheritanceChild$")
	command.Env = append(withoutHostGOROOTMarker(os.Environ()), hostGOROOTLockMarker+"="+string(marker), "CURATOR_R11_HOST_GOROOT_ROOT="+root)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		_ = input.Close()
		if !waited && command.ProcessState == nil {
			_ = command.Process.Kill()
		}
		if !waited {
			_ = command.Wait()
		}
	})
	lines := make(chan string, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	await := func(want string) {
		t.Helper()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case line := <-lines:
				if line == want {
					return
				}
				if strings.HasPrefix(line, "R11_") {
					t.Fatalf("child emitted %q before %q; stderr: %s", line, want, stderr.String())
				}
			case <-done:
				t.Fatalf("child exited before %q; stderr: %s", want, stderr.String())
			case <-timer.C:
				t.Fatalf("child did not emit %q; stderr: %s", want, stderr.String())
			}
		}
	}
	await("R11_CHILD_FIRST_BORROWED")

	// releaseParent waits only for the exact fixture process returned by startHostGOROOTHolder.
	releaseParent()
	if hostGOROOTProcessAlive(parentPID) {
		t.Fatalf("owned fixture parent PID %d remains alive after release", parentPID)
	}
	if _, err := fmt.Fprintln(input, "retry"); err != nil {
		t.Fatal(err)
	}
	await("R11_CHILD_SECOND_ACQUIRED")
	_ = input.Close()
	if err := command.Wait(); err != nil {
		waited = true
		t.Fatalf("child exit %v; stderr: %s", err, stderr.String())
	}
	waited = true
}
