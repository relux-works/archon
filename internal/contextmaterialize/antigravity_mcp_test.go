package contextmaterialize

import (
	"testing"
)

func TestAntigravityMCPFile(t *testing.T) {
	if !SupportsMCPFile("antigravity") {
		t.Fatal("SupportsMCPFile(antigravity) = false, want true")
	}
	if got := MCPPath("antigravity"); got != "config/mcp_config.json" {
		t.Fatalf("MCPPath(antigravity) = %q, want config/mcp_config.json", got)
	}
	servers := []MCPServer{
		{Name: "alpha", Transport: MCPTransportStdio, Command: "npx", Args: []string{"-y", "@example/mcp"}, EnvNames: []string{"GITHUB_TOKEN"}},
		{Name: "beta", Transport: MCPTransportHTTP, URL: "https://mcp.example.com"},
	}
	path, doc, written, err := MCPFile("antigravity", servers)
	if err != nil {
		t.Fatalf("MCPFile(antigravity): %v", err)
	}
	if !written || path != "config/mcp_config.json" {
		t.Fatalf("written=%v path=%q, want true config/mcp_config.json", written, path)
	}
	want := "{\"mcpServers\":{\"alpha\":{\"args\":[\"-y\",\"@example/mcp\"],\"command\":\"npx\",\"type\":\"stdio\"},\"beta\":{\"type\":\"http\",\"url\":\"https://mcp.example.com\"}}}\n"
	if string(doc) != want {
		t.Fatalf("doc = %q, want %q", string(doc), want)
	}

	// Forbidden ambient auth and policy env names must be refused for antigravity (CIP-0033 X2).
	for _, forbidden := range []string{
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"AGY_ADC_AUTH",
		"GOOGLE_APPLICATION_CREDENTIALS",
		"GOOGLE_CLOUD_QUOTA_PROJECT",
		"AGY_CLI_FORCE_FILE_TOKEN_STORAGE",
		"AGY_CLI_DISABLE_AUTO_UPDATE",
		"agy_adc_auth",
	} {
		bad := []MCPServer{{
			Name:      "bad-server",
			Transport: MCPTransportStdio,
			Command:   "node",
			EnvNames:  []string{forbidden},
		}}
		if _, _, _, err := MCPFile("antigravity", bad); err == nil {
			t.Fatalf("MCPFile admitted forbidden antigravity env_name %q", forbidden)
		}
	}

	// Unsupported transport must fail in mcpServersJSON rather than silently drop.
	if _, err := mcpServersJSON([]MCPServer{{Name: "bogus", Transport: "websocket"}}); err == nil {
		t.Fatal("mcpServersJSON silently dropped unsupported transport")
	}
}
