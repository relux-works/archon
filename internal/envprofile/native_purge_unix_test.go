//go:build unix

package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envmarker"
)

// A read-only parent on the final native adapter is rejected before the
// earlier adapter changes. The neighboring empty-directory control is in
// native_purge_test.go; this exercises Unix owner-mode removability.
func TestNativeHomeBindingPurgePreflightsUnwritableLaterParent(t *testing.T) {
	manager := bindingDirectory(t)
	bindings := bindingPurgeHomes(t)
	first := bindings["claude_code"]
	bindingPurgeMarker(t, first, "sample", "CLAUDE.md")
	later := bindings["pi"]
	bindingPurgeMarker(t, later, "sample", "AGENTS.md")
	marker, err := envmarker.Read(later)
	if err != nil || marker == nil {
		t.Fatalf("read later fixture marker: %v", err)
	}
	surface := marker.Surfaces[envmarker.SurfaceRootContext]
	surface.Paths = []string{"locked/AGENTS.md"}
	marker.Surfaces[envmarker.SurfaceRootContext] = surface
	data, err := marker.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(later, envmarker.Name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(later, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(later, "locked")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "AGENTS.md"), []byte("recorded context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	managed := filepath.Join(EnvRoot(manager), "sample")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	firstContext := filepath.Join(first, "CLAUDE.md")
	firstMarker := filepath.Join(first, envmarker.Name)
	firstBytes, err := os.ReadFile(firstContext)
	if err != nil {
		t.Fatal(err)
	}
	firstMarkerBytes, err := os.ReadFile(firstMarker)
	if err != nil {
		t.Fatal(err)
	}
	laterMarkerBytes, err := os.ReadFile(filepath.Join(later, envmarker.Name))
	if err != nil {
		t.Fatal(err)
	}
	err = purgeHomesWithBindings(manager, "sample", bindings)
	if chmodErr := os.Chmod(locked, 0o700); chmodErr != nil {
		t.Fatalf("restore fixture permissions: %v", chmodErr)
	}
	if err == nil || !strings.Contains(err.Error(), DiagProfilePurgePreflightRefused) || !strings.Contains(err.Error(), "write and search permission") {
		t.Fatalf("unwritable later parent = %v; want coded no-write preflight refusal", err)
	}
	for path, want := range map[string]string{
		firstContext:                         string(firstBytes),
		firstMarker:                          string(firstMarkerBytes),
		filepath.Join(locked, "AGENTS.md"):   "recorded context\n",
		filepath.Join(later, envmarker.Name): string(laterMarkerBytes),
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("preflight refusal changed %s: %v", path, err)
		}
	}
	if _, err := os.Lstat(managed); err != nil {
		t.Fatalf("preflight refusal removed managed home: %v", err)
	}
}
