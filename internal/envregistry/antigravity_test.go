package envregistry

import (
	"strings"
	"testing"
)

func TestAntigravityAdapterContract(t *testing.T) {
	adapter := mustAdapter(t, Antigravity)
	if !adapter.IsFlagHome() || adapter.EffectiveHomeTransport() != HomeTransportFlag {
		t.Fatalf("antigravity home transport = %q, want flag", adapter.EffectiveHomeTransport())
	}
	if !adapter.ManagedOnly || adapter.NativeDefaultDir != ".gemini" || !adapter.NativeHomeIgnoresEnv || adapter.ForbiddenHomeBasename != ".gemini" {
		t.Fatalf("unexpected antigravity managed-only/native settings: %+v", adapter)
	}
	if adapter.RootTarget != "config/AGENTS.md" || adapter.SkillsDir != "config/skills" {
		t.Fatalf("unexpected surfaces: root=%q skills=%q", adapter.RootTarget, adapter.SkillsDir)
	}
	if adapter.MCP == nil || adapter.MCP.Kind != KindFile || adapter.MCP.Filename != "config/mcp_config.json" || adapter.MCPFormat != MCPFormatServersJSON {
		t.Fatalf("unexpected MCP descriptor: %+v format=%q", adapter.MCP, adapter.MCPFormat)
	}
	if adapter.VerifiedRelease != "1.5.16" || adapter.SizeAdvisoryBytes != 24000 {
		t.Fatalf("unexpected release/advisory: release=%q advisory=%d", adapter.VerifiedRelease, adapter.SizeAdvisoryBytes)
	}
	if len(adapter.SystemPrompt) != 0 {
		t.Fatalf("antigravity must declare no system_prompt channels, got %+v", adapter.SystemPrompt)
	}
	if form, err := adapter.ResolveForm(""); err != nil || form != FormMonolithic {
		t.Fatalf("default form = %q (%v), want monolithic", form, err)
	}
	if _, err := adapter.ResolveForm(FormReferenced); err == nil || !strings.Contains(err.Error(), DiagFormUnsupported) {
		t.Fatalf("referenced form must fail with %s, got %v", DiagFormUnsupported, err)
	}
	passthrough := adapter.PassthroughFor("darwin")
	if len(passthrough) != 1 || passthrough[0].Path != "antigravity-cli/antigravity-oauth-token" || passthrough[0].Strategy != StrategyFileLink || passthrough[0].FileLinkTarget != "antigravity-cli/antigravity-oauth-token" {
		t.Fatalf("unexpected passthrough: %+v", passthrough)
	}
	if len(adapter.Shadows) != 4 {
		t.Fatalf("expected 4 shadows, got %+v", adapter.Shadows)
	}
	if NormalizeEnvID("agy") != Antigravity {
		t.Fatalf("agy alias did not normalize to %s", Antigravity)
	}
}
