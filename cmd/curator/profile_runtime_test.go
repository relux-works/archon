package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/envprofile"
)

func pinProfileAdapterRuntimes(t *testing.T, runtimes ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range runtimes {
		filename, contents := name, "#!/bin/sh\nexit 0\n"
		mode := os.FileMode(0o755)
		if runtime.GOOS == "windows" {
			filename += ".cmd"
			contents = "@echo off\r\nexit /b 0\r\n"
			mode = 0o644
			t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
		}
		if err := os.WriteFile(filepath.Join(bin, filename), []byte(contents), mode); err != nil {
			t.Fatalf("write adapter runtime fixture %s: %v", name, err)
		}
	}
	path := bin
	if current := os.Getenv("PATH"); current != "" {
		path += string(os.PathListSeparator) + current
	}
	t.Setenv("PATH", path)
	return bin
}

func allProfileAdapterRuntimes() []string { return []string{"claude", "codex", "opencode", "pi"} }

// This proves registration-only install warns and writes its record/lock
// with a missing synthetic runtime, while explicit --use refuses before any
// further manager or adapter-home writes.
func TestProfileInstallWithoutUseWarnsAndUsePreflightIsReadOnly(t *testing.T) {
	source, home := profileHome(t)
	runtimeBin := pinProfileAdapterRuntimes(t, "claude", "opencode", "pi")
	t.Setenv("PATH", runtimeBin)
	pkg := t.TempDir()
	writeContextPackage(t, pkg, "beta", "1.0.0", "beta\n")

	code, _, stderr := runProfile(t, source, "profile", "install", pkg)
	if code != exitOK || !strings.Contains(stderr, envprofile.DiagAdapterRuntimeMissing) ||
		!strings.Contains(stderr, "codex_cli") || !strings.Contains(stderr, "codex") {
		t.Fatalf("no-use install = %d; missing runtime warning should name codex_cli/codex\nstderr:\n%s", code, stderr)
	}
	profileDir := envprofile.ProfileDir(home, "beta")
	sourcePath, lockPath := filepath.Join(profileDir, "source.json"), filepath.Join(profileDir, "lock.json")
	beforeSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("registration was not written: %v", err)
	}
	beforeLock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock was not written: %v", err)
	}
	homes := []string{
		os.Getenv("CLAUDE_CONFIG_DIR"), os.Getenv("CODEX_HOME"),
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode"), os.Getenv("PI_CODING_AGENT_DIR"),
	}
	for _, home := range homes {
		if _, err := os.Lstat(home); !os.IsNotExist(err) {
			t.Fatalf("no-use install touched adapter home %s: %v", filepath.Base(home), err)
		}
	}

	code, _, stderr = runProfile(t, source, "profile", "install", pkg, "--use")
	if code != exitFail || !strings.Contains(stderr, envprofile.DiagAdapterRuntimeMissing) ||
		!strings.Contains(stderr, "codex_cli") || !strings.Contains(stderr, "codex") {
		t.Fatalf("explicit install --use = %d; wanted named preflight refusal\nstderr:\n%s", code, stderr)
	}
	afterSource, err := os.ReadFile(sourcePath)
	if err != nil || string(afterSource) != string(beforeSource) {
		t.Fatalf("runtime refusal changed registration: %v", err)
	}
	afterLock, err := os.ReadFile(lockPath)
	if err != nil || string(afterLock) != string(beforeLock) {
		t.Fatalf("runtime refusal changed lock: %v", err)
	}
	if current, err := envprofile.Current(home); err != nil || current != "" {
		t.Fatalf("runtime refusal wrote current=%q, err=%v", current, err)
	}
	for _, home := range homes {
		if _, err := os.Lstat(home); !os.IsNotExist(err) {
			t.Fatalf("runtime refusal touched adapter home %s: %v", filepath.Base(home), err)
		}
	}
}
