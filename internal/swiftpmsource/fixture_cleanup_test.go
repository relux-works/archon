//go:build darwin || linux

package swiftpmsource

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func restoreSwiftFixtureDirectories(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Unix unlink needs writable directories, not writable files. In
		// particular, chmod on a tool-farm symlink would change the host tool.
		if !entry.IsDir() {
			return nil
		}
		return os.Chmod(path, 0o700)
	})
}

// Cleanup must make sealed directories removable without chmod following a
// tool-farm symlink or mutating a hard-linked executable outside the fixture.
// Real exec before/after is the positive control for the observed EACCES class.
func TestSwiftFixtureCleanupPreservesExternalExecutable(t *testing.T) {
	outside := t.TempDir()
	tool := filepath.Join(outside, "git-control")
	body := []byte("#!/bin/sh\nprintf 'external executable preserved\\n'\n")
	if err := os.WriteFile(tool, body, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(tool)
	if err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		output, err := exec.Command(tool).CombinedOutput()
		if err != nil || string(output) != "external executable preserved\n" {
			t.Errorf("external executable launch failed: %v: %s", err, output)
		}
	}
	run()
	root := t.TempDir()
	sealed := filepath.Join(root, "sealed")
	if err := os.Mkdir(sealed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sealed, "payload"), []byte("owned"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tool, filepath.Join(root, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(tool, filepath.Join(root, "git-hardlink")); err != nil {
		t.Fatal(err)
	}
	externalDir := t.TempDir()
	if err := os.Chmod(externalDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(externalDir, 0o700) })
	if err := os.Symlink(externalDir, filepath.Join(root, "external-directory")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := restoreSwiftFixtureDirectories(root); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(externalDir); err != nil || info.Mode().Perm() != 0o500 {
		t.Errorf("external directory mode changed: %v", err)
	}
	if info, err := os.Stat(filepath.Join(sealed, "payload")); err != nil || info.Mode().Perm() != 0o400 {
		t.Errorf("owned read-only file mode changed: %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("sealed fixture was not removable: %v", err)
	}
	after, err := os.Stat(tool)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		t.Errorf("external executable identity or mode changed: %v", err)
	}
	if got, err := os.ReadFile(tool); err != nil || string(got) != string(body) {
		t.Errorf("external executable bytes changed: %v", err)
	}
	run()
}
