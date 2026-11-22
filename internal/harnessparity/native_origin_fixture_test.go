//go:build harness_parity

package harnessparity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/contextlock"
	"github.com/relux-works/curator/internal/contextstore"
	"github.com/relux-works/curator/internal/envprofile"
	"github.com/relux-works/curator/internal/envregistry"
	"github.com/relux-works/curator/internal/privatedir"
)

// Resolve must reject native origins that overlap manager-owned environments before publication.
func TestNativeParentFixtureOriginBoundaries(t *testing.T) {
	for _, row := range []struct {
		name  string
		cause string
	}{
		{"manager-ancestor", "native origin contains managed environments"},
		{"managed-root", "inside managed environments"},
		{"managed-child", "inside managed environments"},
		{"aliased-ancestor", "native origin contains managed environments"},
		{"external-near-prefix", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			root := t.TempDir()
			manager := filepath.Join(root, "manager")
			native := filepath.Join(root, "native")
			project := filepath.Join(root, "project")
			xdg := filepath.Join(root, "xdg")
			for _, dir := range []string{manager, native, project, xdg} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := privatedir.Protect(manager); err != nil {
				t.Fatal(err)
			}
			sourceDir := filepath.Join(root, "context-source")
			if err := os.MkdirAll(filepath.Join(sourceDir, "context"), 0o700); err != nil {
				t.Fatal(err)
			}
			manifest := []byte("{\"schema_version\":1,\"name\":\"sample\",\"version\":\"1.0.0\",\"context\":{\"modules\":[{\"path\":\"a.md\"}]}}\n")
			if err := os.WriteFile(filepath.Join(sourceDir, "agent-context.json"), manifest, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sourceDir, "context", "a.md"), []byte("Fixture root context.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, stateHash, err := contextstore.EnsureState(manager, "context", "sample", sourceDir)
			if err != nil {
				t.Fatal(err)
			}
			profileDir := envprofile.ProfileDir(manager, "sample")
			if err := privatedir.MakeAll(profileDir); err != nil {
				t.Fatal(err)
			}
			lock := &contextlock.Lock{Root: "sample", Members: []contextlock.Member{{Kind: "context", Name: "sample", Version: "1.0.0", StateHash: stateHash, Weight: 100}}}
			lock.Sort()
			if _, err := contextlock.Write(filepath.Join(profileDir, "lock.json"), lock); err != nil {
				t.Fatal(err)
			}
			source, err := json.Marshal(envprofile.Source{Kind: envprofile.KindPath, Path: sourceDir})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profileDir, "source.json"), append(source, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := envprofile.SetCurrent(manager, "sample"); err != nil {
				t.Fatal(err)
			}
			origin := filepath.Join(native, "codex_cli-native-parent")
			if err := os.MkdirAll(origin, 0o700); err != nil {
				t.Fatal(err)
			}
			switch row.name {
			case "manager-ancestor":
				origin = manager
			case "managed-root":
				origin = envprofile.EnvRoot(manager)
			case "managed-child":
				origin = filepath.Join(envprofile.EnvRoot(manager), "sample", "codex_cli")
			case "aliased-ancestor":
				origin = filepath.Join(native, "manager-alias")
				if err := os.Symlink(manager, origin); err != nil {
					t.Fatal(err)
				}
			case "external-near-prefix":
				origin = manager + "-other"
				if err := os.Mkdir(origin, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", root)
			t.Setenv("CODEX_HOME", filepath.Join(root, "unrelated-selector"))
			request := envprofile.ResolveRequest{
				Home: manager, Profile: "sample", EnvID: "codex_cli", LaunchDir: project,
				Machine: envregistry.DefaultMachineConfig(), Repair: true,
				Detect:       func(envregistry.Adapter) string { return "unknown" },
				NativeHomeOf: func(string) (string, error) { return origin, nil }, OperatorXDG: xdg,
			}
			result, resolveErr := envprofile.Resolve(request)
			managedHome := envprofile.ManagedHomeDir(manager, "sample", "codex_cli")
			renderedHome := filepath.Join(profileDir, "rendered", "codex_cli")
			if row.cause == "" {
				if resolveErr != nil || result == nil || !result.Provisioned || len(result.Document) == 0 {
					t.Fatalf("external sibling must provision: result=%+v error=%v", result, resolveErr)
				}
				if info, err := os.Stat(managedHome); err != nil || !info.IsDir() {
					t.Fatalf("managed tree missing: info=%v error=%v", info, err)
				}
				if info, err := os.Stat(renderedHome); err != nil || !info.IsDir() {
					t.Fatalf("rendered tree missing: info=%v error=%v", info, err)
				}
				if _, err := os.Stat(filepath.Join(renderedHome, "AGENTS.md")); err != nil {
					t.Fatalf("rendered root context missing: %v", err)
				}
				treeEntries := 0
				if err := filepath.WalkDir(managedHome, func(_ string, _ os.DirEntry, err error) error {
					if err == nil {
						treeEntries++
					}
					return err
				}); err != nil || treeEntries <= 1 {
					t.Fatalf("managed tree missing entries: count=%d error=%v", treeEntries, err)
				}
				t.Logf("minimal root-context managed tree entries: %d", treeEntries)
				return
			}
			if resolveErr == nil || !strings.HasPrefix(resolveErr.Error(), envregistry.DiagCredentialConflict+":") || !strings.Contains(resolveErr.Error(), row.cause) {
				t.Fatalf("wrong native-origin refusal: %v", resolveErr)
			}
			if result != nil && len(result.Document) != 0 {
				t.Fatalf("refused origin returned a document: %d bytes", len(result.Document))
			}
			for _, path := range []string{managedHome, renderedHome} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("refused origin published %s: %v", path, err)
				}
			}
		})
	}
}
