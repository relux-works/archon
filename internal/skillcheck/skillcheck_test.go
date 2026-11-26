package skillcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMissingSkillAndInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "csk-skill.json"), []byte(`{"schema_version":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	issues := Validate(dir, "")
	if len(issues) != 2 || issues[0].Code != "skill.missing_skill_md" || issues[1].Code != "skill.manifest_invalid" {
		t.Fatalf("issues = %+v", issues)
	}
	if !HasErrors(issues) || !strings.Contains(Format(issues[0]), "SKILL.md") {
		t.Fatalf("error helpers rejected issues: %+v", issues)
	}
	// Closure provenance (skill name, resolved ref, requirement chain) is
	// added at the closure call site, not inside skillspec.Load, so a
	// standalone check keeps reporting the bare validation message.
	if issues[1].Path != "csk-skill.json" || strings.Contains(issues[1].Message, "invalid skill manifest for") {
		t.Fatalf("standalone manifest issue = %+v", issues[1])
	}
}

func TestValidateLocaleWarning(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("SKILL.md", "---\nname: skill\n---\n")
	write("locales/metadata.json", `{"locales":{"en":{"description":"English"}}}`)
	write(".skill_triggers/en.md", "- trigger\n")
	issues := Validate(dir, "ru")
	if len(issues) != 1 || issues[0].Severity != "warning" || HasErrors(issues) {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestValidateWarnsAboutRuntimePathInPromptContext(t *testing.T) {
	for _, runtimePath := range []string{"scripts/tool", `scripts\tool.cmd`} {
		t.Run(runtimePath, func(t *testing.T) {
			dir := t.TempDir()
			writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\n---\n\nRun "+runtimePath+".\n")
			writeSkillFile(t, dir, "scripts/tool", "#!/bin/sh\n")
			writeSkillFile(t, dir, "csk-skill.json", marshal(t, map[string]any{
				"schema_version": 2,
				"runtime_roots":  []string{"scripts"},
				"commands": map[string]any{
					"tool": map[string]any{"type": "script", "unix_path": "scripts/tool"},
				},
			}))
			issues := Validate(dir, "")
			if len(issues) != 2 ||
				issues[0].Code != "script-command-declared-only" ||
				issues[1].Code != "skill.runtime_root_in_prompt_context" {
				t.Fatalf("issues = %+v", issues)
			}
			if issues[0].Severity != "warning" || HasErrors(issues) {
				t.Fatalf("the declared-only label must warn, never error: %+v", issues)
			}
		})
	}
}

func TestValidateWarnsWhenConsumerGuessesProviderRuntimePath(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md", "---\nname: consumer\n---\n\nRun scripts/tool.\n")
	writeSkillFile(t, dir, "csk-skill.json", marshal(t, map[string]any{
		"schema_version": 2,
		"commands":       map[string]any{},
		"dependencies": map[string]any{"commands": map[string]any{
			"tool": map[string]any{
				"type": "skill", "skill": "provider", "command": "tool",
			},
		}},
	}))
	issues := Validate(dir, "")
	if len(issues) != 1 ||
		issues[0].Code != "skill.provider_runtime_path_in_prompt_context" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestValidateWarnsAboutBuildSourcePathInPromptContext(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		token     string
	}{
		{name: "posix", reference: "assets/build-tool/cmd/tool", token: "assets/build-tool/"},
		{name: "windows", reference: `assets\build-tool\cmd\tool`, token: `assets\build-tool\`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSkillFile(t, dir, "SKILL.md",
				"---\nname: skill\n---\n\nRun "+testCase.reference+".\n"+
					"Resolve .agents/bin/tool.cmd, then global/bin/tool, then command -v tool or Get-Command tool.\n")
			writeSkillFile(t, dir, "assets/build-tool/go.mod", "module example.com/tool\n")
			writeSkillFile(t, dir, "assets/build-tool/cmd/tool/main.go", "package main\nfunc main() {}\n")
			writeSkillFile(t, dir, "agent-skill.json", marshal(t, map[string]any{
				"schema_version": 6,
				"build_roots":    []string{"assets/build-tool"},
				"capabilities":   map[string]any{},
				"commands": map[string]any{
					"tool": map[string]any{
						"type": "build", "driver": "go-v1", "source_dir": "assets/build-tool/cmd/tool",
					},
				},
			}))
			issues := Validate(dir, "")
			if len(issues) != 1 {
				t.Fatalf("issues = %+v", issues)
			}
			issue := issues[0]
			wantMessage := fmt.Sprintf(
				"prompt-visible text references build-only path %q; Curator excludes that build root from installed skill context and runtime storage. Use the exported compiled command shim and keep manifest-relative build source paths source-checkout-only",
				testCase.token,
			)
			if issue.Severity != "warning" || issue.Code != "skill.build_root_in_prompt_context" ||
				issue.Path != "SKILL.md" || issue.Message != wantMessage {
				t.Fatalf("issue = %+v", issue)
			}
		})
	}
}

func TestValidateDoesNotScanMarkdownInsideExcludedBuildRoot(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md",
		"---\nname: skill\n---\n\nResolve .agents/bin/tool.cmd, then global/bin/tool, then command -v tool or Get-Command tool.\n")
	writeSkillFile(t, dir, "assets/build-tool/go.mod", "module example.com/tool\n")
	writeSkillFile(t, dir, "assets/build-tool/cmd/tool/main.go", "package main\nfunc main() {}\n")
	writeSkillFile(t, dir, "assets/build-tool/docs/source.md", "Source: assets/build-tool/cmd/tool.\n")
	writeSkillFile(t, dir, "agent-skill.json", marshal(t, map[string]any{
		"schema_version": 6,
		"build_roots":    []string{"assets/build-tool"},
		"capabilities":   map[string]any{},
		"commands": map[string]any{
			"tool": map[string]any{
				"type": "build", "driver": "go-v1", "source_dir": "assets/build-tool/cmd/tool",
			},
		},
	}))
	if issues := Validate(dir, ""); len(issues) != 0 {
		t.Fatalf("excluded build-root Markdown produced warnings: %+v", issues)
	}
}

func TestValidateBuildCommandDoesNotRequireShellNeutralResolverBoilerplate(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\n---\n")
	writeSkillFile(t, dir, "build/go.mod", "module example.com/tool\n")
	writeSkillFile(t, dir, "build/cmd/tool/main.go", "package main\nfunc main() {}\n")
	writeSkillFile(t, dir, "agent-skill.json", marshal(t, map[string]any{
		"schema_version": 6,
		"build_roots":    []string{"build"},
		"capabilities":   map[string]any{},
		"commands": map[string]any{
			"tool": map[string]any{"type": "build", "driver": "go-v1", "source_dir": "build/cmd/tool"},
		},
	}))
	if issues := Validate(dir, ""); len(issues) != 0 {
		t.Fatalf("unexpected boilerplate issues = %+v", issues)
	}
}

func TestValidateBrokenRelativeReferenceSizeUnicodeSecretsAndScripts(t *testing.T) {
	t.Run("broken_relative_reference", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\n---\nSee [guide](references/guide.md) and references/missing.md.\n")
		writeSkillFile(t, dir, "references/guide.md", "# Guide\n")
		issues := Validate(dir, "")
		if len(issues) != 1 || issues[0].Code != "skill.broken_relative_reference" || !strings.Contains(issues[0].Message, "references/missing.md") {
			t.Fatalf("broken relative reference issues = %+v", issues)
		}
	})
	t.Run("skill_md_size_budget", func(t *testing.T) {
		warnDir := t.TempDir()
		writeSkillFile(t, warnDir, "SKILL.md", "---\nname: skill\n---\n"+strings.Repeat("a", 9000))
		warnIssues := Validate(warnDir, "")
		if len(warnIssues) != 1 || warnIssues[0].Code != "skill.skill_md_size_warning" || warnIssues[0].Severity != "warning" {
			t.Fatalf("size warning issues = %+v", warnIssues)
		}
		errDir := t.TempDir()
		writeSkillFile(t, errDir, "SKILL.md", "---\nname: skill\n---\n"+strings.Repeat("a", 25000))
		errIssues := Validate(errDir, "")
		if len(errIssues) != 1 || errIssues[0].Code != "skill.skill_md_size_exceeded" || errIssues[0].Severity != "error" {
			t.Fatalf("size exceeded issues = %+v", errIssues)
		}
	})
	t.Run("hidden_unicode_and_secret_scanner", func(t *testing.T) {
		dir := t.TempDir()
		secret := "ghp_" + strings.Repeat("A", 36)
		writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\n---\nHidden \u202E bidi and token "+secret+"\n")
		issues := Validate(dir, "")
		if len(issues) != 2 || issues[0].Code != "skill.hidden_unicode_injection" || issues[1].Code != "skill.secret_detected" {
			t.Fatalf("unicode/secret issues = %+v", issues)
		}
	})
	t.Run("script_executable_and_shebang", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\n---\n")
		badScript := filepath.Join(dir, "scripts", "run.sh")
		if err := os.MkdirAll(filepath.Dir(badScript), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(badScript, []byte("echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		issues := Validate(dir, "")
		if len(issues) != 2 || issues[0].Code != "skill.script_missing_shebang" || issues[1].Code != "skill.script_not_executable" {
			t.Fatalf("script issues = %+v", issues)
		}
	})
}

func TestValidateAcceptsCrossPlatformShellNeutralResolver(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md",
		"---\nname: skill\n---\n\n"+
			"Resolve .agents/bin/tool (tool.cmd on Windows), then global/bin/tool, "+
			"then command -v tool or Get-Command tool.\n")
	writeSkillFile(t, dir, "README.md", "Development source: scripts/tool.\n")
	writeSkillFile(t, dir, "scripts/tool", "#!/bin/sh\n")
	writeSkillFile(t, dir, "scripts/tool.cmd", "@echo off\r\n")
	writeSkillFile(t, dir, "csk-skill.json", marshal(t, map[string]any{
		"schema_version": 2,
		"runtime_roots":  []string{"scripts"},
		"commands": map[string]any{
			"tool": map[string]any{
				"type": "script", "unix_path": "scripts/tool", "win_path": "scripts/tool.cmd",
			},
		},
	}))
	// The shell-neutral resolver silences the prompt-context warnings; the
	// script audit label remains as a warning, never an error.
	issues := Validate(dir, "")
	if len(issues) != 1 || issues[0].Code != "script-command-declared-only" {
		t.Fatalf("shell-neutral skill warnings = %+v", issues)
	}
	if issues[0].Severity != "warning" || HasErrors(issues) {
		t.Fatalf("the declared-only label must warn, never error: %+v", issues)
	}
}

func writeSkillFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if strings.HasPrefix(relative, "scripts/") && strings.HasPrefix(content, "#!") {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

// schema8ScriptSkill lays out a schema-8 skill with one script command,
// enforced or declared-only, and nothing else that could produce an error.
func schema8ScriptSkill(t *testing.T, enforced bool) string {
	t.Helper()
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md", "---\nname: skill\ndescription: d\n---\n"+
		"Resolve the command from project .agents/bin, then the manager global/bin fallback, "+
		"then a validated bare command with command -v or Get-Command; on Windows the shim carries .cmd.\n")
	writeSkillFile(t, dir, "scripts/tool", "#!/bin/sh\n")
	command := map[string]any{"type": "script", "unix_path": "scripts/tool"}
	if enforced {
		command["execution_policy"] = "script-worker-v1"
		command["interpreter"] = "python3-v1"
	}
	writeSkillFile(t, dir, "agent-skill.json", marshal(t, map[string]any{
		"schema_version": 8,
		"capabilities": map[string]any{
			"env_read": []string{}, "exec": "none", "filesystem": "repo",
			"network": "none", "secrets": "none",
		},
		"runtime_roots": []string{"scripts"},
		"commands":      map[string]any{"tool": command},
	}))
	return dir
}

// A schema-8 manifest that selects script-worker-v1 is a valid document,
// so it parses and validates: the control table is complete, so admission
// proceeds and only a host that cannot provide a mandatory control
// refuses, at install and at invocation.
func TestValidateAcceptsEnforcedScriptExecutionPolicy(t *testing.T) {
	issues := Validate(schema8ScriptSkill(t, true), "")
	if HasErrors(issues) {
		t.Fatalf("an enforced schema-8 skill reported errors: %+v", issues)
	}
	for _, issue := range issues {
		if issue.Code == "script_execution_policy_unsupported" || issue.Code == "script_execution_control_unavailable" {
			t.Fatalf("an enforced command was refused at check: %+v", issue)
		}
	}
}

// The declared-only schema-8 script command is the control: admitting schema 8
// must not have changed what an unenforced command does.
func TestValidateAcceptsDeclaredOnlySchema8ScriptCommand(t *testing.T) {
	issues := Validate(schema8ScriptSkill(t, false), "")
	if HasErrors(issues) {
		t.Fatalf("a declared-only schema-8 skill reported errors: %+v", issues)
	}
	for _, issue := range issues {
		if issue.Code == "script_execution_policy_unsupported" || issue.Code == "script_execution_control_unavailable" {
			t.Fatalf("a declared-only command was refused: %+v", issue)
		}
	}
}
