// Package skillcheck validates one skill package without a consuming
// project (Spec §15, curator skill check).
package skillcheck

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/relux-works/curator/internal/locale"
	"github.com/relux-works/curator/internal/scriptpolicy"
	"github.com/relux-works/curator/internal/skillspec"
	"github.com/relux-works/curator/internal/whitelist"
)

// Issue is one validation finding.
type Issue struct {
	Severity string `json:"severity"` // error | warning
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// Validate checks a skill directory: SKILL.md presence, manifest parsing
// with every Spec §5 rule, and locale consistency when a locale is given.
func Validate(skillDir, localeValue string) []Issue {
	var issues []Issue
	if info, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil || info.IsDir() {
		issues = append(issues, Issue{
			Severity: "error", Code: "skill.missing_skill_md",
			Path: "SKILL.md", Message: "required SKILL.md not found",
		})
	}
	spec, err := skillspec.Load(skillDir)
	if err != nil {
		issues = append(issues, Issue{
			Severity: "error", Code: "skill.manifest_invalid",
			Path: skillspec.ManifestSourcePath(skillDir), Message: err.Error(),
		})
	} else {
		issues = append(issues, executionPolicyIssues(spec)...)
		issues = append(issues, scriptAuditLabelIssues(spec)...)
		issues = append(issues, runtimeRootReferenceWarnings(skillDir, spec)...)
		issues = append(issues, buildRootReferenceWarnings(skillDir, spec)...)
		issues = append(issues, brokenRelativeReferenceIssues(skillDir, spec)...)
		issues = append(issues, skillMDSizeIssues(skillDir)...)
		issues = append(issues, unicodeAndSecretIssues(skillDir, spec)...)
		issues = append(issues, scriptExecutableAndShebangIssues(skillDir, spec)...)
	}
	analysis := locale.Analyze(skillDir, localeValue)
	for _, item := range analysis.Issues {
		issues = append(issues, Issue{Severity: item.Severity, Code: item.Code, Path: item.Path, Message: item.Message})
	}
	return issues
}

// executionPolicyIssues reports a manifest that selects an execution policy
// this manager does not implement. The manifest itself is a valid schema-8
// document — the parser accepted it — so the refusal is reported here, where
// the manager answers for what it will actually install, and it carries the
// closed §4.1.1 diagnostic rather than a `skill.`-namespaced one.
func executionPolicyIssues(spec *skillspec.Spec) []Issue {
	err := scriptpolicy.Admit(spec.Commands)
	if err == nil {
		return nil
	}
	var refusal *scriptpolicy.Error
	if !errors.As(err, &refusal) {
		return []Issue{{Severity: "error", Code: scriptpolicy.PolicyUnsupported, Message: err.Error()}}
	}
	return []Issue{{
		Severity: refusal.Severity,
		Code:     refusal.DiagnosticCode,
		Path:     refusal.Path,
		Message:  refusal.Detail,
	}}
}

// scriptAuditLabelIssues reports the script audit warning classes (manager
// profile §7) as validation warnings. Both classes are always warnings,
// never errors: a declared-only skill still validates and still installs,
// and an enforced skill with declared network hosts is admitted with its
// reporting-only posture stated. The issue code is the closed class name,
// pinned exactly as the conformance vector names it.
func scriptAuditLabelIssues(spec *skillspec.Spec) []Issue {
	var issues []Issue
	for _, entry := range scriptpolicy.AuditLabelsForCommands(spec.Commands, spec.Capabilities.Network) {
		for _, label := range entry.Labels {
			switch label {
			case scriptpolicy.LabelDeclaredOnly:
				issues = append(issues, Issue{
					Severity: "warning",
					Code:     label,
					Path:     "commands." + entry.Command + ".execution_policy",
					Message: fmt.Sprintf("command '%s' is declared-only (no execution_policy); its capabilities bound nothing at run time; adopt execution_policy %q with an interpreter to enforce containment",
						entry.Command, skillspec.ScriptExecutionPolicy),
				})
			case scriptpolicy.LabelUnfilteredDeclaredNetwork:
				issues = append(issues, Issue{
					Severity: "warning",
					Code:     label,
					Path:     "commands." + entry.Command,
					Message: fmt.Sprintf("command '%s' is enforced but its declared network hosts are reporting-only (no portable filtering is applied); declare only the hosts you need",
						entry.Command),
				})
			}
		}
	}
	return issues
}

func runtimeRootReferenceWarnings(skillDir string, spec *skillspec.Spec) []Issue {
	hasProviderRuntime := false
	for _, dependency := range spec.Dependencies {
		if dependency.Type == "skill" {
			hasProviderRuntime = true
			break
		}
	}
	if !hasProviderRuntime {
		for _, requirement := range spec.Requirements {
			if requirement.Mode == "full" || requirement.Mode == "runtime" {
				hasProviderRuntime = true
				break
			}
		}
	}
	if len(spec.RuntimeRoots) == 0 && !hasProviderRuntime {
		return nil
	}

	var issues []Issue
	excludeRoots := whitelist.ContextExcludedRoots(spec.RuntimeRoots, spec.BuildRoots)
	for _, path := range promptMarkdownFiles(skillDir, excludeRoots) {
		payload, err := os.ReadFile(path) // #nosec G304 -- path is below the validated skill directory
		if err != nil {
			continue
		}
		text := string(payload)
		for _, runtimeRoot := range spec.RuntimeRoots {
			windowsRoot := strings.ReplaceAll(runtimeRoot, "/", `\`)
			tokens := []string{runtimeRoot + "/", windowsRoot + `\`}
			for _, token := range tokens {
				if !strings.Contains(text, token) {
					continue
				}
				relative, _ := filepath.Rel(skillDir, path)
				issues = append(issues, Issue{
					Severity: "warning",
					Code:     "skill.runtime_root_in_prompt_context",
					Path:     filepath.ToSlash(relative),
					Message: fmt.Sprintf(
						"prompt-visible text references runtime-only path %q; Curator removes that root from installed skill context. Use exported command shims for installed execution and keep manifest-relative paths source-checkout-only",
						token,
					),
				})
				break
			}
		}
		if hasProviderRuntime && (strings.Contains(text, "scripts/") || strings.Contains(text, `scripts\`)) {
			relative, _ := filepath.Rel(skillDir, path)
			issues = append(issues, Issue{
				Severity: "warning",
				Code:     "skill.provider_runtime_path_in_prompt_context",
				Path:     filepath.ToSlash(relative),
				Message:  "prompt-visible text references a source scripts path while this skill consumes another skill's runtime. Resolve the provider's exported command shim instead of guessing its source layout",
			})
		}
	}
	return issues
}

func buildRootReferenceWarnings(skillDir string, spec *skillspec.Spec) []Issue {
	if len(spec.BuildRoots) == 0 {
		return nil
	}
	roots := append([]string(nil), spec.BuildRoots...)
	sort.Strings(roots)
	excludeRoots := whitelist.ContextExcludedRoots(spec.RuntimeRoots, spec.BuildRoots)
	var issues []Issue
	for _, path := range promptMarkdownFiles(skillDir, excludeRoots) {
		payload, err := os.ReadFile(path) // #nosec G304 -- path is below the validated skill directory
		if err != nil {
			continue
		}
		text := string(payload)
		for _, buildRoot := range roots {
			windowsRoot := strings.ReplaceAll(buildRoot, "/", `\`)
			tokens := []string{buildRoot + "/", windowsRoot + `\`}
			for _, token := range tokens {
				if !strings.Contains(text, token) {
					continue
				}
				relative, _ := filepath.Rel(skillDir, path)
				issues = append(issues, Issue{
					Severity: "warning",
					Code:     "skill.build_root_in_prompt_context",
					Path:     filepath.ToSlash(relative),
					Message: fmt.Sprintf(
						"prompt-visible text references build-only path %q; Curator excludes that build root from installed skill context and runtime storage. Use the exported compiled command shim and keep manifest-relative build source paths source-checkout-only",
						token,
					),
				})
				break
			}
		}
	}
	return issues
}

var (
	mdLinkPattern      = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	explicitRefPattern = regexp.MustCompile(`\b((?:references|scripts|examples|assets|templates)/[A-Za-z0-9_./-]+)`)
	secretPattern      = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----|ghp_[A-Za-z0-9]{30,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-proj-[A-Za-z0-9_-]{20,}`)
)

const (
	skillMDSizeWarningBytes  = 8192
	skillMDSizeExceededBytes = 24576
)

func specFileSelection(spec *skillspec.Spec) *whitelist.FileSelection {
	if spec == nil || spec.Files == nil {
		return nil
	}
	return &whitelist.FileSelection{Include: spec.Files.Include, Exclude: spec.Files.Exclude}
}

func brokenRelativeReferenceIssues(skillDir string, spec *skillspec.Spec) []Issue {
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	payload, err := os.ReadFile(skillMDPath) // #nosec G304 -- validated skill directory
	if err != nil {
		return nil
	}
	excludeRoots := whitelist.ContextExcludedRoots(spec.RuntimeRoots, spec.BuildRoots)
	effectiveFiles, err := whitelist.ContextFiles(skillDir, len(spec.Commands) == 0, excludeRoots, specFileSelection(spec))
	if err != nil {
		return nil
	}
	allowed := make(map[string]bool, len(effectiveFiles))
	allowedDirs := make(map[string]bool, len(effectiveFiles))
	for _, f := range effectiveFiles {
		allowed[f] = true
		for dir := path.Dir(f); dir != "." && dir != "/"; dir = path.Dir(dir) {
			allowedDirs[dir] = true
		}
	}
	hasProviderRuntime := false
	for _, dep := range spec.Dependencies {
		if dep.Type == "skill" {
			hasProviderRuntime = true
			break
		}
	}
	if !hasProviderRuntime {
		for _, req := range spec.Requirements {
			if req.Mode == "full" || req.Mode == "runtime" {
				hasProviderRuntime = true
				break
			}
		}
	}

	text := string(payload)
	var candidates []string
	for _, m := range mdLinkPattern.FindAllStringSubmatch(text, -1) {
		candidates = append(candidates, m[1])
	}
	for _, m := range explicitRefPattern.FindAllStringSubmatch(text, -1) {
		candidates = append(candidates, m[1])
	}
	seen := map[string]bool{}
	var issues []Issue
	for _, raw := range candidates {
		ref := raw
		if idx := strings.IndexByte(ref, '#'); idx >= 0 {
			ref = ref[:idx]
		}
		ref = strings.TrimRight(ref, ".,;:)'\"`")
		if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "mailto:") ||
			strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "/") ||
			strings.ContainsAny(ref, "<>*${}\\") || strings.HasSuffix(ref, "/") {
			continue
		}
		clean := path.Clean(strings.TrimPrefix(ref, "./"))
		if clean == "." || strings.HasPrefix(clean, "../") || seen[clean] {
			continue
		}
		seen[clean] = true
		coveredByRootWarning := false
		for _, root := range spec.RuntimeRoots {
			if clean == root || strings.HasPrefix(clean, root+"/") {
				coveredByRootWarning = true
				break
			}
		}
		for _, root := range spec.BuildRoots {
			if clean == root || strings.HasPrefix(clean, root+"/") {
				coveredByRootWarning = true
				break
			}
		}
		if hasProviderRuntime && (clean == "scripts" || strings.HasPrefix(clean, "scripts/")) {
			coveredByRootWarning = true
		}
		if coveredByRootWarning {
			continue
		}
		if !allowed[clean] && !allowedDirs[clean] {
			issues = append(issues, Issue{
				Severity: "warning",
				Code:     "skill.broken_relative_reference",
				Path:     "SKILL.md",
				Message:  fmt.Sprintf("SKILL.md references relative path %q which does not exist in the materialized skill context", clean),
			})
		}
	}
	return issues
}

func skillMDSizeIssues(skillDir string) []Issue {
	info, err := os.Stat(filepath.Join(skillDir, "SKILL.md"))
	if err != nil || info.IsDir() {
		return nil
	}
	size := info.Size()
	if size > skillMDSizeExceededBytes {
		return []Issue{{
			Severity: "error",
			Code:     "skill.skill_md_size_exceeded",
			Path:     "SKILL.md",
			Message:  fmt.Sprintf("SKILL.md is %d bytes (limit %d bytes / 24 KB); move detailed documentation into on-demand references/*.md", size, skillMDSizeExceededBytes),
		}}
	}
	if size > skillMDSizeWarningBytes {
		return []Issue{{
			Severity: "warning",
			Code:     "skill.skill_md_size_warning",
			Path:     "SKILL.md",
			Message:  fmt.Sprintf("SKILL.md is %d bytes (advisory budget %d bytes / 8 KB); move detailed documentation into on-demand references/*.md", size, skillMDSizeWarningBytes),
		}}
	}
	return nil
}

func unicodeAndSecretIssues(skillDir string, spec *skillspec.Spec) []Issue {
	excludeRoots := whitelist.ContextExcludedRoots(spec.RuntimeRoots, spec.BuildRoots)
	files, err := whitelist.ContextFiles(skillDir, len(spec.Commands) == 0, excludeRoots, specFileSelection(spec))
	if err != nil {
		for _, p := range promptMarkdownFiles(skillDir, excludeRoots) {
			if rel, relErr := filepath.Rel(skillDir, p); relErr == nil {
				files = append(files, filepath.ToSlash(rel))
			}
		}
	}
	var issues []Issue
	for _, rel := range files {
		payload, err := os.ReadFile(filepath.Join(skillDir, filepath.FromSlash(rel))) // #nosec G304 -- validated materialized skill path
		if err != nil {
			continue
		}
		text := string(payload)
		if hasHiddenUnicodeInjection(text) {
			issues = append(issues, Issue{
				Severity: "error",
				Code:     "skill.hidden_unicode_injection",
				Path:     rel,
				Message:  "file contains hidden zero-width, bidi-override, or Unicode tag characters",
			})
		}
		if secretPattern.Match(payload) {
			issues = append(issues, Issue{
				Severity: "error",
				Code:     "skill.secret_detected",
				Path:     rel,
				Message:  "file contains a hardcoded private key or API secret token",
			})
		}
	}
	return issues
}

func hasHiddenUnicodeInjection(text string) bool {
	for _, r := range text {
		if (r >= '\u200B' && r <= '\u200F') ||
			(r >= '\u202A' && r <= '\u202E') ||
			(r >= '\u2066' && r <= '\u2069') ||
			(r >= '\U000E0000' && r <= '\U000E007F') {
			return true
		}
	}
	return false
}

func scriptExecutableAndShebangIssues(skillDir string, spec *skillspec.Spec) []Issue {
	candidateSet := map[string]bool{}
	for _, command := range spec.Commands {
		if command.Type == "script" && command.UnixPath != "" {
			candidateSet[filepath.ToSlash(command.UnixPath)] = true
		}
	}
	scriptsDir := filepath.Join(skillDir, "scripts")
	if info, err := os.Stat(scriptsDir); err == nil && info.IsDir() {
		_ = filepath.WalkDir(scriptsDir, func(p string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(skillDir, p)
			if relErr != nil {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".sh" || ext == ".py" || ext == ".bash" || ext == "" {
				candidateSet[filepath.ToSlash(rel)] = true
			}
			return nil
		})
	}
	if len(candidateSet) == 0 {
		return nil
	}
	candidates := make([]string, 0, len(candidateSet))
	for rel := range candidateSet {
		candidates = append(candidates, rel)
	}
	sort.Strings(candidates)
	var issues []Issue
	for _, rel := range candidates {
		full := filepath.Join(skillDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		payload, err := os.ReadFile(full) // #nosec G304 -- validated script file below skillDir
		if err != nil {
			continue
		}
		if !strings.HasPrefix(string(payload), "#!") {
			issues = append(issues, Issue{
				Severity: "warning",
				Code:     "skill.script_missing_shebang",
				Path:     rel,
				Message:  fmt.Sprintf("script %q is missing a '#!' shebang line", rel),
			})
		}
		if info.Mode()&0o111 == 0 {
			issues = append(issues, Issue{
				Severity: "warning",
				Code:     "skill.script_not_executable",
				Path:     rel,
				Message:  fmt.Sprintf("script %q does not have executable permission bit set (mode %04o)", rel, info.Mode().Perm()),
			})
		}
	}
	return issues
}

func promptMarkdownFiles(skillDir string, excludeRoots []string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, root := range whitelist.IncludeRoots {
		candidate := filepath.Join(skillDir, root)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if whitelist.PathExcluded(filepath.ToSlash(root), excludeRoots) {
			continue
		}
		if !info.IsDir() {
			if strings.EqualFold(filepath.Ext(candidate), ".md") && !seen[candidate] {
				seen[candidate] = true
				paths = append(paths, candidate)
			}
			continue
		}
		_ = filepath.WalkDir(candidate, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			relative, relErr := filepath.Rel(skillDir, path)
			if relErr != nil {
				return nil
			}
			if path != candidate && whitelist.PathExcluded(filepath.ToSlash(relative), excludeRoots) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") || seen[path] {
				return nil
			}
			seen[path] = true
			paths = append(paths, path)
			return nil
		})
	}
	sort.Strings(paths)
	return paths
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

// Format renders one issue for terminal output.
func Format(issue Issue) string {
	location := ""
	if issue.Path != "" {
		location = " (" + issue.Path + ")"
	}
	return fmt.Sprintf("%s: %s%s: %s", issue.Severity, issue.Code, location, issue.Message)
}
