//go:build windows

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envprofile"
	"github.com/relux-works/curator/internal/privatedir"
)

// TestNativeHomeBindingCLIRejectsForeignMutationDACL proves a foreign-write
// DACL refuses purge without changing manager or native-home contents.
func TestNativeHomeBindingCLIRejectsForeignMutationDACL(t *testing.T) {
	source, manager := profileHome(t)
	pkg := t.TempDir()
	writeContextPackage(t, pkg, "sample", "1.0.0", "sample\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
		t.Fatalf("install = %d: %s", code, stderr)
	}
	if err := envprofile.SetCurrent(manager, "default"); err != nil {
		t.Fatal(err)
	}

	ids := []string{"claude_code", "codex_cli", "opencode", "pi"}
	homes := make(map[string]string, len(ids))
	beforeHomes := make(map[string]map[string][]byte, len(ids))
	for _, id := range ids {
		home := nativeBindingCLIDirectory(t)
		homes[id] = home
		nativeBindingCLIMarker(t, home)
		beforeHomes[id] = nativeBindingTreeSnapshot(t, home)
	}
	beforeManager := nativeBindingTreeSnapshot(t, manager)
	// The resolver checks sorted adapter IDs, so pi is last; an incremental
	// purge would leave observable writes in the earlier homes.
	breakCacheProtection(t, homes["pi"])

	args := []string{"profile", "remove", "sample", "--purge"}
	for _, id := range ids {
		args = append(args, "--native-home", id+"="+homes[id])
	}
	if code, _, stderr := runProfile(t, source, args...); code != exitFail || !strings.Contains(stderr, "DACL grants mutation rights to another identity") {
		t.Fatalf("foreign-mutation DACL purge = %d: %s", code, stderr)
	}
	if after := nativeBindingTreeSnapshot(t, manager); !reflect.DeepEqual(beforeManager, after) {
		t.Fatal("refused purge changed manager state")
	}
	for _, id := range ids {
		if after := nativeBindingTreeSnapshot(t, homes[id]); !reflect.DeepEqual(beforeHomes[id], after) {
			t.Fatalf("refused purge changed %s home contents", id)
		}
	}
}

// The real CLI refuses a foreign-mutable backup sidecar DACL before changing
// any earlier native root or manager state. The sidecar is a synthetic tree.
func TestNativeHomeBindingCLIRejectsForeignMutationSidecarDACL(t *testing.T) {
	source, manager := profileHome(t)
	pkg := t.TempDir()
	writeContextPackage(t, pkg, "sample", "1.0.0", "sample\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
		t.Fatalf("install = %d: %s", code, stderr)
	}
	if err := envprofile.SetCurrent(manager, "default"); err != nil {
		t.Fatal(err)
	}
	ids := []string{"claude_code", "codex_cli", "opencode", "pi"}
	homes := make(map[string]string, len(ids))
	beforeHomes := make(map[string]map[string][]byte, len(ids))
	for _, id := range ids {
		home := nativeBindingCLIDirectory(t)
		homes[id] = home
		nativeBindingCLIMarker(t, home)
		beforeHomes[id] = nativeBindingTreeSnapshot(t, home)
	}
	sidecar := nativeBindingCLISidecar(t, homes["pi"])
	beforeHomes["pi"] = nativeBindingTreeSnapshot(t, homes["pi"])
	beforeManager := nativeBindingTreeSnapshot(t, manager)
	breakCacheProtection(t, sidecar)

	args := []string{"profile", "remove", "sample", "--purge"}
	for _, id := range ids {
		args = append(args, "--native-home", id+"="+homes[id])
	}
	if code, _, stderr := runProfile(t, source, args...); code != exitFail || !strings.Contains(stderr, "DACL grants mutation rights to another identity") {
		t.Fatalf("foreign-mutation sidecar DACL purge = %d: %s", code, stderr)
	}
	if after := nativeBindingTreeSnapshot(t, manager); !reflect.DeepEqual(beforeManager, after) {
		t.Fatal("sidecar DACL refusal changed manager state")
	}
	for _, id := range ids {
		if after := nativeBindingTreeSnapshot(t, homes[id]); !reflect.DeepEqual(beforeHomes[id], after) {
			t.Fatalf("sidecar DACL refusal changed %s home contents", id)
		}
	}
}

// A private root and private sidecar pass the same real CLI preflight and are
// removed; this is the valid DACL control for the two refusal tests.
func TestNativeHomeBindingCLIAcceptsPrivateRootAndSidecarDACL(t *testing.T) {
	source, manager := profileHome(t)
	pkg := t.TempDir()
	writeContextPackage(t, pkg, "sample", "1.0.0", "sample\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
		t.Fatalf("install = %d: %s", code, stderr)
	}
	if err := envprofile.SetCurrent(manager, "default"); err != nil {
		t.Fatal(err)
	}
	ids := []string{"claude_code", "codex_cli", "opencode", "pi"}
	homes := make(map[string]string, len(ids))
	for _, id := range ids {
		home := nativeBindingCLIDirectory(t)
		homes[id] = home
		nativeBindingCLIMarker(t, home)
	}
	sidecar := nativeBindingCLISidecar(t, homes["pi"])
	args := []string{"profile", "remove", "sample", "--purge"}
	for _, id := range ids {
		args = append(args, "--native-home", id+"="+homes[id])
	}
	if code, _, stderr := runProfile(t, source, args...); code != exitOK {
		t.Fatalf("private root/sidecar purge = %d: %s", code, stderr)
	}
	for _, id := range ids {
		for _, path := range []string{filepath.Join(homes[id], "AGENTS.md"), filepath.Join(homes[id], ".agent-environment.json")} {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("private purge retained %s: %v", path, err)
			}
		}
	}
	if _, err := os.Lstat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("private purge retained sidecar: %v", err)
	}
}

func nativeBindingCLISidecar(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, ".agent-environment-backup")
	if err := privatedir.Make(root); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(root, "1")
	if err := privatedir.Make(generation); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generation, "AGENTS.md"), []byte("backup fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func nativeBindingTreeSnapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[relative] = []byte("directory")
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			snapshot[relative] = []byte("symlink:" + target)
			return nil
		}
		if !entry.Type().IsRegular() {
			return os.ErrInvalid
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[relative] = data
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot fixture tree: %v", err)
	}
	return snapshot
}
