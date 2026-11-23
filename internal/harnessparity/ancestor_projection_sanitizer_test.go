package harnessparity

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/relux-works/curator/internal/testtoolchain"
)

func TestFixtureAncestorAdjacentPrivatePathSanitizerRefuses(t *testing.T) {
	expected := []string{
		"/tmp/cap07/CLAUDE.md",
		"/tmp/cap07/.claude/CLAUDE.md",
		"/tmp/cap07/CLAUDE.local.md",
	}
	input := []byte("{\"claudeMdExcludes\":[\"/tmp/cap07/CLAUDE.md\",\"/tmp/cap07/.claude/CLAUDE.md\",\"/tmp/cap07/CLAUDE.local.md\",\"/Users/example/private/state\"]}")
	want := []byte(fmt.Sprintf("{\"claudeMdExcludes\":[\"%s\",\"/Users/example/private/state\"]}", fixtureAncestorProjectionToken))
	settingsPath := "/tmp/cap07/manager/environments/sample/claude_code/settings.json"
	binding := fixtureAncestorControlBindingAtPath(fixtureAncestorCaseClaudeEmpty, settingsPath, expected)
	projected, err := projectFixtureAncestorSettings(fixtureAncestorCaseClaudeEmpty, settingsPath, input, binding)
	if err != nil || !bytes.Equal(projected, want) {
		t.Fatalf("adjacent private path projection changed: %v", err)
	}
	if os.Getenv("CAPTURE_WORKFLOW_PATH") == "" {
		t.Skip("unchanged sanitizer control runs in the workflow-bound contract lane")
	}
	assertFixtureAncestorSanitizerRefusal(t, projected)
}

func assertFixtureAncestorSanitizerRefusal(t *testing.T, projected []byte) {
	t.Helper()
	workflow, err := readBoundNativeCaptureWorkflow()
	if err != nil {
		t.Fatalf("read unchanged native capture sanitizer: %v", err)
	}
	source, err := staticCaptureWorkflowGoProgram(string(workflow), `cat > "$sanitizer_source_dir/main.go" <<'GO'`)
	if err != nil {
		t.Fatalf("extract unchanged native capture sanitizer: %v", err)
	}
	root := moduleRoot(t)
	scratchParent := filepath.Join(root, ".temp")
	if err := os.MkdirAll(scratchParent, 0o700); err != nil {
		t.Fatal("fixture-ancestor-sanitizer/scratch-parent-failed")
	}
	scratch, err := os.MkdirTemp(scratchParent, "fixture-ancestor-sanitizer-")
	if err != nil {
		t.Fatal("fixture-ancestor-sanitizer/scratch-create-failed")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(scratch); err != nil {
			t.Error("fixture-ancestor-sanitizer/scratch-cleanup-failed")
		}
	})
	if err := os.WriteFile(filepath.Join(scratch, "main.go"), source, 0o600); err != nil {
		t.Fatal("fixture-ancestor-sanitizer/source-write-failed")
	}
	controlTest := `package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectedAdjacentPrivatePathIsRefusedBeforeExport(t *testing.T) {
	paths, err := prepareControl(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("control setup failed: %v", err)
	}
	payload := []byte(os.Getenv("FIXTURE_ANCESTOR_PROJECTED_SETTINGS"))
	casePath := filepath.Join(paths.caseDir, paths.caseFiles[0].name)
	if err := os.WriteFile(casePath, payload, 0600); err != nil {
		t.Fatalf("projected input setup failed: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runGate([]string{paths.caseDir, paths.repeatDir, paths.artifact, "false"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("c15_sanitizer_accepted_adjacent_private_path")
	}
	refusal := stderr.String()
	if code != 1 || stdout.Len() != 0 || (refusal != diagnostic("path-private-home") && refusal != diagnostic("path-absolute")) {
		t.Fatalf("C15 projected adjacent path verdict: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	entries, err := os.ReadDir(paths.artifact)
	if err != nil || len(entries) != 0 {
		t.Fatalf("sanitizer refusal published artifacts: entries=%d err=%v", len(entries), err)
	}
	projectedDirectory := filepath.Join(paths.artifact, "outputs")
	if _, err := os.Lstat(projectedDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sanitizer refusal created the projected artifact directory: %v", err)
	}
	projectedExport := filepath.Join(paths.artifact, "outputs", "case-00.json")
	if _, err := os.Lstat(projectedExport); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sanitizer refusal created projected export file: %v", err)
	}
}
`
	if err := os.WriteFile(filepath.Join(scratch, "main_test.go"), []byte(controlTest), 0o600); err != nil {
		t.Fatal("fixture-ancestor-sanitizer/control-write-failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	packagePath, err := filepath.Rel(root, scratch)
	if err != nil {
		t.Fatal("fixture-ancestor-sanitizer/package-path-failed")
	}
	cmd := exec.CommandContext(ctx, testtoolchain.Go(t), "test", "-count=1", "./"+filepath.ToSlash(packagePath))
	cmd.Dir = root
	testtoolchain.InheritHostGOROOTLock(t, cmd)
	cmd.Env = append(os.Environ(), "FIXTURE_ANCESTOR_PROJECTED_SETTINGS="+string(projected))
	output, err := cmd.CombinedOutput()
	if err != nil {
		if bytes.Contains(output, []byte("c15_sanitizer_accepted_adjacent_private_path")) {
			t.Fatal("unchanged sanitizer accepted the adjacent private path")
		}
		t.Fatalf("unchanged sanitizer failed the adjacent private-path refusal control: %v (%d diagnostic bytes)", err, len(output))
	}
}
