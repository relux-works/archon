//go:build darwin

package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/privatedir"
)

// The physical /private/var path is an accepted direct binding while its /var
// alias and a symlink below the physical root are refused. This exercises the
// host alias and does not model other macOS filesystem layouts.
func TestNativeHomeBindingVarAliasRequiresPhysicalPath(t *testing.T) {
	varInfo, err := os.Lstat("/var")
	if err != nil || varInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("/var is not the expected symlinked ancestor: %v", err)
	}
	root, err := os.MkdirTemp("/private/var/tmp", "curator-native-binding-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := privatedir.Protect(root); err != nil {
		t.Fatal(err)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(physicalRoot, "/private/var/") {
		t.Fatalf("test root is not under the physical /private/var path: %q", physicalRoot)
	}
	manager := filepath.Join(physicalRoot, "manager")
	native := filepath.Join(physicalRoot, "native")
	if err := privatedir.Make(manager); err != nil {
		t.Fatal(err)
	}
	if err := privatedir.Make(native); err != nil {
		t.Fatal(err)
	}
	physicalChild := filepath.Join(native, "child")
	if err := privatedir.Make(physicalChild); err != nil {
		t.Fatal(err)
	}

	if got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": native}, "codex_cli"); err != nil || got != native {
		t.Fatalf("physical native path = %q, %v; want %q", got, err, native)
	}
	alias := strings.TrimPrefix(native, "/private")
	if !strings.HasPrefix(alias, "/var/") {
		t.Fatalf("alias fixture did not retain /var spelling: %q", alias)
	}
	if got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": alias}, "codex_cli"); err == nil || got != "" || !strings.Contains(err.Error(), "/var") || !strings.Contains(err.Error(), "physical/resolved path") {
		t.Fatalf("/var alias = %q, %v; want named ancestor refusal and physical-path guidance", got, err)
	}
	link := filepath.Join(physicalRoot, "native-link")
	if err := os.Symlink(native, link); err != nil {
		t.Fatal(err)
	}
	linkedHome := filepath.Join(link, "child")
	if got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": linkedHome}, "codex_cli"); err == nil || got != "" || !strings.Contains(err.Error(), link) || !strings.Contains(err.Error(), "physical/resolved path") {
		t.Fatalf("symlink below physical root = %q, %v; want named ancestor refusal", got, err)
	}
}
