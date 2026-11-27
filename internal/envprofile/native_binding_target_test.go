package envprofile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/pathboundary"
)

// A canonical external directory passes while managed overlap, foreign
// ownership, and symlinked native routes refuse at the target boundary.
func TestNativeHomeBindingTargetBoundary(t *testing.T) {
	manager := bindingDirectory(t)
	native := bindingDirectory(t)
	if err := validateNativeBindingTarget(manager, native, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeBindingTarget(manager, filepath.Join(native, "absent"), nil); err == nil {
		t.Fatal("missing native directory accepted")
	}
	for _, target := range []string{manager, filepath.Join(manager, "environments", "sample", "codex_cli"), filepath.Dir(manager)} {
		if err := validateNativeBindingTarget(manager, target, nil); err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
			t.Fatalf("managed overlap %q = %v; want binding refusal", target, err)
		}
	}
	lookup := func(string, os.FileInfo) (pathboundary.OwnerIdentity, error) {
		return pathboundary.OwnerIdentity("foreign-fixture-owner"), nil
	}
	if err := validateNativeBindingTarget(manager, native, lookup); err == nil || !strings.Contains(err.Error(), pathboundary.CheckOwnership) {
		t.Fatalf("foreign owner = %v; want ownership refusal", err)
	}
	if runtime.GOOS != "windows" {
		alias := filepath.Join(bindingDirectory(t), "alias")
		if err := os.Symlink(manager, alias); err != nil {
			t.Fatal(err)
		}
		if err := validateNativeBindingTarget(manager, alias, nil); err == nil {
			t.Fatal("managed alias accepted")
		}
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(native, alias); err != nil {
			t.Fatal(err)
		}
		if err := validateNativeBindingTarget(manager, alias, nil); err == nil {
			t.Fatal("symlink native root accepted")
		}

		// The canonical root is a positive control for a route through the
		// symlinked parent below.
		root := bindingDirectory(t)
		realParent := filepath.Join(root, "real-parent")
		if err := os.Mkdir(realParent, 0o700); err != nil {
			t.Fatal(err)
		}
		nativeHome := filepath.Join(realParent, "native")
		if err := os.Mkdir(nativeHome, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := validateNativeBindingTarget(manager, nativeHome, nil); err != nil {
			t.Fatalf("canonical native home refused: %v", err)
		}
		aliasParent := filepath.Join(root, "alias-parent")
		if err := os.Symlink(realParent, aliasParent); err != nil {
			t.Fatal(err)
		}
		aliasedHome := filepath.Join(aliasParent, "native")
		if err := validateNativeBindingTarget(manager, aliasedHome, nil); err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) || !strings.Contains(err.Error(), aliasParent) || !strings.Contains(err.Error(), "physical/resolved path") {
			t.Fatalf("symlinked native ancestor refusal = %v; want named ancestor and physical/resolved-path guidance", err)
		}
	}
}
