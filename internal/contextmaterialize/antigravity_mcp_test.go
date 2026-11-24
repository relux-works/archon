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
}
