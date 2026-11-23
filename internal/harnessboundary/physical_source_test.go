package harnessboundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourceScanBoundaries(t *testing.T) {
	t.Run("external-protocol-checkout-is-outside-project-scan", func(t *testing.T) {
		root := t.TempDir()
		plant(t, root, "internal/shared/clean.go", "package shared\nfunc ordinary() {}\n")
		plant(t, root, "protocol-spec/tools/generate.go", "package main\nfunc selected(id string) bool { return id == \"claude_code\" }\n")
		if findings := scan(t, root); len(findings) != 0 {
			t.Fatalf("external authoritative checkout entered the Curator source scan: %v", findings)
		}
		owned := "internal/shared/plant.go"
		plant(t, root, owned, "package shared\nfunc selected(id string) bool { return id == \"claude_code\" }\n")
		requireFinding(t, scan(t, root), owned, 2)
	})
	t.Run("agent-skill-links-are-outside-project-source-scan", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "agents", "skills", "skill-go-testing-tools")
		plant(t, target, "hidden/adapter.go", "package hidden\nfunc selected(id string) bool { return id == \"claude_code\" }\n")
		for _, toolDir := range []string{".claude", ".codex"} {
			link := filepath.Join(root, toolDir, "skills", "skill-go-testing-tools")
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			relativeTarget, err := filepath.Rel(filepath.Dir(link), target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(relativeTarget, link); err != nil {
				t.Skipf("host cannot create symlink controls: %v", err)
			}
		}
		if files, err := productionFiles(root); err != nil || len(files) != 0 {
			t.Fatalf("agent skill link entered product source scan: files=%v err=%v", files, err)
		}
	})
	t.Run("production-source-symlink-is-refused", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(t.TempDir(), "target.go")
		if err := os.WriteFile(target, []byte("package shared\nfunc selected(id string) bool { return id == \"claude_code\" }\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "internal", "shared", "alias.go")
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("host cannot create symlink controls: %v", err)
		}
		if _, err := productionFiles(root); err == nil || !strings.Contains(err.Error(), "source_symlink_unsupported") {
			t.Fatalf("production source symlink verdict: %v", err)
		}
	})
	t.Run("directory-symlink-is-refused", func(t *testing.T) {
		root := t.TempDir()
		target := t.TempDir()
		plant(t, target, "hidden/plant.go", "package shared\nfunc selected(id string) bool { return id == \"claude_code\" }\n")
		link := filepath.Join(root, "internal", "shared", "hidden")
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("host cannot create symlink controls: %v", err)
		}
		if _, err := productionFiles(root); err == nil || !strings.Contains(err.Error(), "source_directory_symlink_unsupported") {
			t.Fatalf("production directory symlink verdict: %v", err)
		}
	})
	t.Run("production-path-cannot-alias-declared-skill-submodule", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "agents", "skills", "skill-go-testing-tools")
		plant(t, target, "hidden/adapter.go", "package hidden\nfunc selected(id string) bool { return id == \"claude_code\" }\n")
		link := filepath.Join(root, "internal", "harnessvalue")
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		relativeTarget, err := filepath.Rel(filepath.Dir(link), target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(relativeTarget, link); err != nil {
			t.Skipf("host cannot create symlink controls: %v", err)
		}
		if _, err := productionFiles(root); err == nil || !strings.Contains(err.Error(), "source_directory_symlink_unsupported") {
			t.Fatalf("production path aliasing the skill submodule escaped the scan: %v", err)
		}
	})
}

// Source directives may change compiler diagnostics, never the physical path,
// line or line hash used by this source boundary. The generic control remains
// accepted; each planted violation reports its actual source line.
func testPhysicalSourceLocations(t *testing.T) {
	t.Run("forged-line-allowance", testForgedLineAllowance)
	const prefix = "package shared\n//line synthetic.go:200000000\n"
	root := t.TempDir()
	rel := "internal/shared/plant.go"
	plant(t, root, rel, prefix+"func same(a,b string)bool{return a==b}\n")
	if len(scan(t, root)) != 0 {
		t.Fatal("generic source directive refused")
	}
	for _, row := range []struct {
		name, body string
		lines      []int
	}{
		{"computed-ID", "const selected=\"cla\"+\"ude_code\"\nfunc choose(id string)bool{return id==selected}\n", []int{3, 4}},
		{"concrete-import", "import _ \"github.com/relux-works/curator/internal/harness/codex_cli\"\n", []int{3}},
		{"generated-directive", "//go:generate fixture\n", []int{3}},
	} {
		t.Run(row.name, func(t *testing.T) {
			plant(t, root, rel, prefix+row.body)
			findings := scan(t, root)
			for _, line := range row.lines {
				requireFinding(t, findings, rel, line)
			}
		})
	}
}

// LF/CRLF delimiters are excluded from the same independently pinned content
// hash. Counts still reject a copied occurrence, and changed trailing spaces
// cannot reuse the original allowance. A lone CR is content, not a delimiter.
func testCanonicalSourceLines(t *testing.T) {
	const line = "const nativePath = \".codex/skills\""
	const expected = "0be2cc7d28583daa8c3c173b6e8a760eb7d29e745dae85a1e36ec03d9b8a80f5"
	const rel = "internal/shared/plant.go"
	allowance := ExceptionSet{SourceHead: baselineHead, Exceptions: []Exception{{rel, "codex_cli", expected}}}
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run("delimiter-"+strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			root := t.TempDir()
			plant(t, root, rel, strings.Join([]string{"package shared", line, ""}, eol))
			findings := scan(t, root)
			if len(findings) != 1 || findings[0].Line != 2 || findings[0].LineSHA256 != expected || len(ApplyExceptions(findings, allowance)) != 0 {
				t.Fatalf("line-content allowance mismatch: %v", findings)
			}
			plant(t, root, rel, strings.Join([]string{"package shared", line, line, ""}, eol))
			unmatched := ApplyExceptions(scan(t, root), allowance)
			if len(unmatched) != 1 {
				t.Fatalf("copied occurrence verdict: %v", unmatched)
			}
			requireFinding(t, unmatched, rel, 3)
			for _, changed := range []string{line + " ", line + "\r"} {
				// No final LF: the CR in this variant is actual content.
				plant(t, root, rel, "package shared"+eol+changed)
				unmatched = ApplyExceptions(scan(t, root), allowance)
				if len(unmatched) != 1 {
					t.Fatal("content change reused an allowance")
				}
				requireFinding(t, unmatched, rel, 2)
			}
		})
	}
}

// Aliased forbidden imports and calls retain physical diagnostics even with an
// adjusted logical line. A pure strings helper with the same directive passes.
func testPhysicalEffectLocations(t *testing.T) {
	root := t.TempDir()
	rel := "internal/harness/fixture/adapter.go"
	const prefix = "package fixture\n//line synthetic.go:200000000\n"
	plant(t, root, rel, prefix+"import memory \"strings\"\nfunc pure(s string)string{return memory.TrimSpace(s)}\n")
	findings, err := ScanEffects(root)
	if err != nil || len(findings) != 0 {
		t.Fatalf("pure directive control: %v %v", findings, err)
	}
	plant(t, root, rel, prefix+"import blocked \"os\"\nfunc effect(){_,_=blocked.ReadFile(\"fixture.json\")}\n")
	findings, err = ScanEffects(root)
	if err != nil {
		t.Fatal(err)
	}
	requireFinding(t, findings, rel, 3)
	requireFinding(t, findings, rel, 4)
}

// One fixture allowance admits only its actual old occurrence. Forged logical
// coordinates must not make new computed policy consume that same allowance.
func testForgedLineAllowance(t *testing.T) {
	const rel = "internal/shared/plant.go"
	const old = `func original(id string)bool{return id=="claude_code"}`
	const expected = "bf1a3b451414654aef748770b96227b2cacff39a109cb7cc35e3cc7833e49941"
	root := t.TempDir()
	allowance := ExceptionSet{SourceHead: baselineHead, Exceptions: []Exception{{rel, "claude_code", expected}}}
	plant(t, root, rel, "package shared\n"+old+"\n")
	original := scan(t, root)
	if len(original) != 1 || len(ApplyExceptions(original, allowance)) != 0 {
		t.Fatal("original fixture allowance refused")
	}
	plant(t, root, rel, "package shared\n"+old+"\n//line synthetic.go:2\nconst selected=\"cla\"+\"ude_code\"\n//line synthetic.go:2\nfunc added(id string)bool{return id==selected}\n")
	unmatched := ApplyExceptions(scan(t, root), allowance)
	if len(unmatched) != 2 {
		t.Fatalf("forged logical position reused an allowance: %v", unmatched)
	}
	requireFinding(t, unmatched, rel, 4)
	requireFinding(t, unmatched, rel, 6)
}
