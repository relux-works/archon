// Package testruntimes installs zero-exit stand-ins for the adapter runtime
// probes (claude, codex, opencode, pi) on PATH, so a test can drive the
// explicit activation path — profile install --use, profile use, import
// --use — without the real agent binaries. Registration without --use never
// needs them (environments §9.2: only explicit intent activates), so a test
// that pins nothing proves the adapter_runtime_missing preflight instead.
// Production code never imports this package.
package testruntimes

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// All lists the runtime probe of every adapter the machine-scope activation
// preflight checks.
func All() []string { return []string{"claude", "codex", "opencode", "pi"} }

// Pin writes a zero-exit executable for each named runtime into a fresh
// temporary directory and sets PATH to that directory, followed by fallback
// when it is not empty, for the rest of the test. On Windows the stand-ins
// are .cmd scripts and PATHEXT admits them. It returns the directory.
func Pin(t testing.TB, fallback string, runtimes ...string) string {
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
	if fallback != "" {
		path += string(os.PathListSeparator) + fallback
	}
	t.Setenv("PATH", path)
	return bin
}
