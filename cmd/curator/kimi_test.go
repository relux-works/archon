package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKimiCodeCLIResolveAliasAndManagedHome(t *testing.T) {
	source, _ := profileHome(t)
	nativeKimi := filepath.Join(os.Getenv("HOME"), ".kimi-code")
	if err := os.MkdirAll(filepath.Join(nativeKimi, "credentials"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeKimi, "credentials", "kimi-code.json"), []byte(`{"token":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeKimi, "config.toml"), []byte("default_model = \"kimi-code/kimi-for-coding\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	pkg := t.TempDir()
	writeContextPackage(t, pkg, "kimi-smoke", "1.0.0", "hello kimi\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", pkg); code != exitOK {
		t.Fatalf("install stderr:\n%s", stderr)
	}
	setEnvTestCurrent(t, source, "kimi-smoke")

	code, stdout, stderr := runProfile(t, source, "env", "resolve", "kimi", "--repair")
	if code != exitOK {
		t.Fatalf("resolve kimi --repair = %d\nstderr:\n%s", code, stderr)
	}
	var frag map[string]any
	if err := json.Unmarshal([]byte(stdout), &frag); err != nil {
		t.Fatalf("fragment is not JSON: %v\nstdout:\n%s", err, stdout)
	}
	if frag["environment"] != "kimi_code" {
		t.Fatalf("environment = %v, want kimi_code", frag["environment"])
	}
	envObj, _ := frag["env"].(map[string]any)
	homePath, _ := envObj["KIMI_CODE_HOME"].(string)
	if homePath == "" {
		t.Fatalf("missing KIMI_CODE_HOME in env: %v", envObj)
	}
	if _, err := os.Stat(filepath.Join(homePath, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md missing in managed kimi home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homePath, "credentials", "kimi-code.json")); err != nil {
		t.Fatalf("credentials/kimi-code.json passthrough missing: %v", err)
	}
}
