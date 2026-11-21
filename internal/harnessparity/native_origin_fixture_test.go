//go:build harness_parity

package harnessparity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envprofile"
	"github.com/relux-works/curator/internal/envregistry"
)

// The capture's fixture-owned external origin must reach successful Resolve
// with its own directory and link target, including when the caller supplies
// an unrelated selector. These assertions do not establish native Mac parity.
func TestNativeParentFixtureExternalOrigin(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "unrelated-selector"))
	c := fixtureCase{Name: "codex_cli-native-parent", ID: "codex_cli", Operation: "managed", State: "native-parent"}
	actual := runLegacyFixture(t, c)
	if !actual.Executed || actual.Refusal != "" || len(actual.Tree) != 9 || len(actual.Rendered) == 0 || len(actual.Document) == 0 {
		t.Fatalf("external-origin fixture must provision: executed=%v refusal=%q managed=%d rendered=%d", actual.Executed, actual.Refusal, len(actual.Tree), len(actual.Rendered))
	}
	wantOrigin := c.ID + "-native-parent"
	foundDirectory, foundLink := false, false
	for _, node := range actual.NativeBefore {
		if node.Path == wantOrigin && node.Kind == "directory" {
			foundDirectory = true
		}
	}
	for _, node := range actual.Tree {
		if node.Path == "auth.json" && node.Kind == "link" && node.Link == "${FIXTURE_ROOT}/native/"+wantOrigin+"/auth.json" {
			foundLink = true
		}
	}
	if !foundDirectory || !foundLink || !actual.ProjectPreserved {
		t.Fatalf("external origin lost: directory=%v link=%v project-preserved=%v", foundDirectory, foundLink, actual.ProjectPreserved)
	}
}

// Ancestor, contained and aliased manager origins must retain the specific
// credential-conflict refusal without publishing managed/rendered files or
// changing native/project trees. A nearby external sibling must still succeed.
func TestNativeParentFixtureOriginBoundaries(t *testing.T) {
	for _, row := range []struct {
		name   string
		origin func(t *testing.T, manager, native string) string
		cause  string
	}{
		{"manager-ancestor", func(_ *testing.T, manager, _ string) string { return manager }, "native origin contains managed environments"},
		{"managed-root", func(_ *testing.T, manager, _ string) string { return envprofile.EnvRoot(manager) }, "inside managed environments"},
		{"managed-child", func(_ *testing.T, manager, _ string) string {
			return filepath.Join(envprofile.EnvRoot(manager), "sample", "codex_cli")
		}, "inside managed environments"},
		{"aliased-ancestor", func(t *testing.T, manager, native string) string {
			alias := filepath.Join(native, "manager-alias")
			if err := os.Symlink(manager, alias); err != nil {
				t.Fatal(err)
			}
			return alias
		}, "native origin contains managed environments"},
		{"external-near-prefix", func(t *testing.T, manager, _ string) string {
			sibling := manager + "-other"
			if err := os.Mkdir(sibling, 0o700); err != nil {
				t.Fatal(err)
			}
			return sibling
		}, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			c := fixtureCase{Name: "codex_cli-native-parent", ID: "codex_cli", Operation: "managed", State: "native-parent"}
			actual := runLegacyFixtureWithNativeHome(t, c, func(manager, native string) string {
				return row.origin(t, manager, native)
			})
			if !actual.Executed {
				t.Fatal("Resolve was not exercised")
			}
			if row.cause == "" {
				if actual.Refusal != "" || len(actual.Tree) != 9 || len(actual.Rendered) == 0 || len(actual.Document) == 0 {
					t.Fatalf("external sibling must provision: refusal=%q managed=%d", actual.Refusal, len(actual.Tree))
				}
				return
			}
			if !strings.HasPrefix(actual.Refusal, envregistry.DiagCredentialConflict+":") || !strings.Contains(actual.Refusal, row.cause) {
				t.Fatalf("wrong native-origin refusal: %q", actual.Refusal)
			}
			if len(actual.Tree) != 0 || len(actual.Rendered) != 0 || len(actual.Document) != 0 || !actual.NativePreserved || !actual.ProjectPreserved {
				t.Fatalf("refused origin published output or changed native/project trees: managed=%d rendered=%d native-preserved=%v project-preserved=%v", len(actual.Tree), len(actual.Rendered), actual.NativePreserved, actual.ProjectPreserved)
			}
		})
	}
}
