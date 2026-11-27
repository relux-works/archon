//go:build darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/relux-works/archon/internal/testtoolchain"
)

const unrelatedGOROOTChildMarker = "CURATOR_TEST_UNRELATED_GOROOT_CHILD_MARKER"

// TestUnrelatedPackageTestDoesNotWaitForHostGOROOTLock proves that an unrelated
// cmd/curator test process reaches its body while another process holds the
// host-GOROOT lock. This Darwin-only control does not establish hosted race-lane behavior.
func TestUnrelatedPackageTestDoesNotWaitForHostGOROOTLock(t *testing.T) {
	if marker := os.Getenv(unrelatedGOROOTChildMarker); marker != "" {
		if err := os.WriteFile(marker, []byte("started\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	lockCtx, cancelLock := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelLock()
	lock, err := testtoolchain.AcquireHostGOROOT(lockCtx, t.Name()+" holder")
	if err != nil {
		t.Fatalf("acquire isolated host-GOROOT control lock within 5s: %v", err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Errorf("release isolated host-GOROOT control lock: %v", err)
		}
	})

	marker := filepath.Join(root, "unrelated-started")
	childRoot := t.TempDir()
	childCtx, cancelChild := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelChild()
	child := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestUnrelatedPackageTestDoesNotWaitForHostGOROOTLock$")
	child.Env = []string{
		unrelatedGOROOTChildMarker + "=" + marker,
		"HOME=" + childRoot,
		"TMPDIR=" + root,
		"PATH=" + os.Getenv("PATH"),
		"GOTOOLCHAIN=local",
	}
	output, err := child.CombinedOutput()
	if childCtx.Err() != nil {
		t.Fatalf("unrelated child did not finish while the host-GOROOT lock was held within 5s: %v; output=%q", err, output)
	}
	if err != nil {
		t.Fatalf("unrelated child test failed while the host-GOROOT lock was held: %v; output=%q", err, output)
	}
	if started, err := os.ReadFile(marker); err != nil || string(started) != "started\n" {
		t.Fatalf("unrelated child test marker = %q, err = %v; want started marker", started, err)
	}
}
