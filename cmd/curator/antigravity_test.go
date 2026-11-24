package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envfragment"
	"github.com/relux-works/curator/internal/envregistry"
)

func TestAntigravityCLIResolveAliasAndFormatBoundaries(t *testing.T) {
	source, _ := profileHome(t)
	writeNativeCredentials(t)
	pkg := t.TempDir()
	writeContextPackage(t, pkg, "acme", "1.0.0", "hello\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
		t.Fatalf("install stderr:\n%s", stderr)
	}
	setEnvTestCurrent(t, source, "acme")

	// Resolve via the agy alias normalizes to canonical antigravity and emits v7.
	code, stdout, stderr := runProfile(t, source, "env", "resolve", "agy", "--repair")
	if code != exitOK {
		t.Fatalf("resolve agy --repair = %d\nstderr:\n%s", code, stderr)
	}
	var frag map[string]any
	if err := json.Unmarshal([]byte(stdout), &frag); err != nil {
		t.Fatalf("fragment is not JSON: %v\nstdout:\n%s", err, stdout)
	}
	if frag["fragment"] != envfragment.VersionV7 {
		t.Fatalf("fragment version = %v, want %s", frag["fragment"], envfragment.VersionV7)
	}
	if frag["environment"] != envregistry.Antigravity {
		t.Fatalf("environment = %v, want %s", frag["environment"], envregistry.Antigravity)
	}
	homeObj, _ := frag["home"].(map[string]any)
	homePath, _ := homeObj["path"].(string)
	if homeObj["transport"] != "flag" || filepath.Base(homePath) != envregistry.Antigravity {
		t.Fatalf("unexpected home object: %v", homeObj)
	}
	if _, err := os.Stat(filepath.Join(homePath, "config", "AGENTS.md")); err != nil {
		t.Fatalf("config/AGENTS.md missing: %v", err)
	}

	// Canonical bare resolve matches alias bytes.
	code2, canonicalOut, stderr2 := runProfile(t, source, "env", "resolve", envregistry.Antigravity)
	if code2 != exitOK {
		t.Fatalf("resolve antigravity = %d\nstderr:\n%s", code2, stderr2)
	}
	if canonicalOut != stdout {
		t.Fatalf("canonical fragment differs from alias output:\ncanonical:\n%s\nalias:\n%s", canonicalOut, stdout)
	}

	// --format env and --format shell must fail closed with 0 stdout bytes.
	for _, format := range []string{"env", "shell"} {
		fCode, fOut, fErr := runProfile(t, source, "env", "resolve", "agy", "--format", format)
		if fCode == exitOK {
			t.Fatalf("--format %s unexpectedly succeeded\nstdout:\n%s", format, fOut)
		}
		if len(fOut) != 0 {
			t.Fatalf("--format %s wrote %d stdout bytes, want 0:\n%s", format, len(fOut), fOut)
		}
		if !strings.Contains(fErr, envregistry.DiagFlagHomeUnsupported) {
			t.Fatalf("--format %s stderr = %q, want %s", format, fErr, envregistry.DiagFlagHomeUnsupported)
		}
	}
}
