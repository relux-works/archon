package envfragment

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/envregistry"
)

func TestAntigravityFragmentV7(t *testing.T) {
	adapter, err := envregistry.ByID(envregistry.Antigravity)
	if err != nil {
		t.Fatal(err)
	}
	envRoot := filepath.Join(t.TempDir(), "environments")
	home := filepath.Join(envRoot, "team", "antigravity")
	mcpPath := filepath.Join(home, "config", "mcp_config.json")
	f := &Fragment{
		Environment: envregistry.Antigravity,
		Profile:     "team",
		LockSHA256:  strings.Repeat("a", 64),
		Permissions: Permissions{Mode: "native", Locked: false, Source: "default"},
		Winner:      "higher-weight",
		Placement:   "winner-last",
		Home:        &HomeBinding{Path: home, Transport: envregistry.HomeTransportFlag},
		Env:         map[string]string{"AGY_CLI_FORCE_FILE_TOKEN_STORAGE": "true"},
		CredentialStore: &CredentialStore{
			Selection: envregistry.CredentialStoreFile,
		},
		MCP: &MCP{
			Path:     mcpPath,
			EnvNames: []string{"GITHUB_TOKEN"},
			Channels: []envregistry.Channel{*adapter.MCP},
		},
	}
	if err := CheckBoundary(adapter, envRoot, f); err != nil {
		t.Fatalf("CheckBoundary valid v7: %v", err)
	}
	raw, err := f.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["fragment"] != VersionV7 {
		t.Fatalf("fragment = %v, want %s", doc["fragment"], VersionV7)
	}
	homeObj, ok := doc["home"].(map[string]any)
	if !ok || homeObj["path"] != home || homeObj["transport"] != "flag" {
		t.Fatalf("unexpected home object: %v", doc["home"])
	}

	// Fail closed for env and shell formats with zero bytes.
	if out, err := f.RenderEnvFormat(adapter); err == nil || len(out) != 0 || !strings.Contains(err.Error(), envregistry.DiagFlagHomeUnsupported) {
		t.Fatalf("RenderEnvFormat = (%q, %v), want %s with 0 bytes", string(out), err, envregistry.DiagFlagHomeUnsupported)
	}
	if out, err := f.RenderShellFormat(adapter); err == nil || len(out) != 0 || !strings.Contains(err.Error(), envregistry.DiagFlagHomeUnsupported) {
		t.Fatalf("RenderShellFormat = (%q, %v), want %s with 0 bytes", string(out), err, envregistry.DiagFlagHomeUnsupported)
	}

	// Negative boundary cases.
	badDotGemini := *f
	badDotGemini.Home = &HomeBinding{Path: filepath.Join(envRoot, "team", ".gemini"), Transport: envregistry.HomeTransportFlag}
	badDotGemini.MCP = nil
	if err := CheckBoundary(adapter, envRoot, &badDotGemini); err == nil {
		t.Fatal("accepted .gemini managed home basename")
	}

	badEnv := *f
	badEnv.Env = map[string]string{"AGY_CLI_FORCE_FILE_TOKEN_STORAGE": "false"}
	if err := CheckBoundary(adapter, envRoot, &badEnv); err == nil {
		t.Fatal("accepted false force-file env")
	}

	badStore := *f
	badStore.CredentialStore = &CredentialStore{Selection: envregistry.CredentialStoreKeychain}
	if err := CheckBoundary(adapter, envRoot, &badStore); err == nil {
		t.Fatal("accepted keychain credential store")
	}

	badPrompt := *f
	badPrompt.SystemPrompt = &SystemPrompt{Path: filepath.Join(home, ".agent-context", "system-prompt.md")}
	if err := CheckBoundary(adapter, envRoot, &badPrompt); err == nil {
		t.Fatal("accepted system_prompt on antigravity")
	}

	badMCP := *f
	badMCP.MCP = &MCP{Path: filepath.Join(home, "wrong.json"), EnvNames: []string{}, Channels: []envregistry.Channel{*adapter.MCP}}
	if err := CheckBoundary(adapter, envRoot, &badMCP); err == nil {
		t.Fatal("accepted mismatched mcp.path")
	}
}
