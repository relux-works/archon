package envprofile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/pathboundary"
)

// An existing bound external directory passes the target
// boundary. Managed descendants, manager ancestors, foreign ownership and
// symlink aliases refuse without changing either directory's contents.
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
		alias := filepath.Join(t.TempDir(), "alias")
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
	}
}
