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
	credInfo, err := os.Lstat(filepath.Join(homePath, "credentials"))
	if err != nil {
		t.Fatalf("lstat credentials dir: %v", err)
	}
	if credInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("credentials entry is not a symlink; atomic token refresh would break it")
	}

	// Simulate kimi 2.8.1 refreshing credentials/kimi-code.json via atomic temp-file rename.
	tmpCred := filepath.Join(homePath, "credentials", "kimi-code.json.tmp")
	if err := os.WriteFile(tmpCred, []byte(`{"token":"refreshed"}`), 0o600); err != nil {
		t.Fatalf("write refreshed cred temp: %v", err)
	}
	if err := os.Rename(tmpCred, filepath.Join(homePath, "credentials", "kimi-code.json")); err != nil {
		t.Fatalf("atomic rename refreshed cred: %v", err)
	}
	nativeBytes, err := os.ReadFile(filepath.Join(nativeKimi, "credentials", "kimi-code.json"))
	if err != nil || string(nativeBytes) != `{"token":"refreshed"}` {
		t.Fatalf("native credentials not updated through dir symlink: bytes=%q err=%v", string(nativeBytes), err)
	}

	// Second resolve must remain clean with zero environment_credential_conflict.
	if code, _, stderr := runProfile(t, source, "env", "resolve", "kimi"); code != exitOK {
		t.Fatalf("second resolve kimi after atomic credential refresh = %d\nstderr:\n%s", code, stderr)
	}
	if code, _, stderr := runProfile(t, source, "env", "resolve", "kimi", "--repair"); code != exitOK {
		t.Fatalf("second resolve kimi --repair after atomic credential refresh = %d\nstderr:\n%s", code, stderr)
	}
}
