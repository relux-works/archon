package windsurf

import (
	"errors"
	"reflect"
	"testing"

	"github.com/relux-works/archon/internal/harnessapi"
)

func code(err error) string {
	var refusal harnessapi.Refusal
	if errors.As(err, &refusal) {
		return refusal.Code()
	}
	return ""
}

func stringsFrom(t *testing.T, paths harnessapi.NamedPaths) []string {
	t.Helper()
	values := paths.Paths()
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !value.Valid() {
			t.Fatalf("invalid returned path %q", value.String())
		}
		result = append(result, value.String())
	}
	return result
}

func TestRegisteredSurfaceAndSkillScopes(t *testing.T) {
	adapter := New()
	if adapter.ID() != ID || adapter.APIVersion() != harnessapi.AdapterAPIVersionV1 {
		t.Fatalf("adapter metadata: %s v%d", adapter.ID(), adapter.APIVersion())
	}
	wantCapabilities, err := harnessapi.NewCapabilitySet(harnessapi.NativeSkillDiscovery, harnessapi.MCPInspection)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Capabilities() != wantCapabilities {
		t.Fatalf("capabilities: %v", adapter.Capabilities().Values())
	}

	for _, row := range []struct {
		scope  harnessapi.Scope
		mirror bool
	}{
		{harnessapi.UserScope, true},
		{harnessapi.ProjectScope, false},
	} {
		discovery, err := adapter.Skills(harnessapi.Context{}, row.scope)
		if err != nil {
			t.Fatalf("skills %s: %v", row.scope, err)
		}
		if got := stringsFrom(t, discovery.Paths()); !reflect.DeepEqual(got, []string{".agents/skills"}) {
			t.Fatalf("paths %s: %v", row.scope, got)
		}
		if got := stringsFrom(t, discovery.IgnorePaths()); !reflect.DeepEqual(got, []string{".agents"}) {
			t.Fatalf("ignore paths %s: %v", row.scope, got)
		}
		if !discovery.NativeDiscovery() || discovery.Mirror() != row.mirror || discovery.Scope() != row.scope {
			t.Fatalf("discovery policy %s: native=%t mirror=%t", row.scope, discovery.NativeDiscovery(), discovery.Mirror())
		}
	}
	if _, err := (Adapter{}).Skills(harnessapi.Context{}, "unsupported"); code(err) != "harness_scope_unsupported" {
		t.Fatalf("invalid skill scope refusal: %v", err)
	}
}

func TestMCPIsHomeInspectionOnly(t *testing.T) {
	adapter := Adapter{}
	home, err := adapter.MCP(harnessapi.Context{}, harnessapi.UserScope)
	if err != nil {
		t.Fatal(err)
	}
	if got := stringsFrom(t, home.Paths()); !reflect.DeepEqual(got, []string{".codeium/windsurf/mcp_config.json"}) {
		t.Fatalf("home MCP path: %v", got)
	}
	if home.Format() != harnessapi.JSON || !home.Inspect() || home.Write() || home.RequiresTrust() {
		t.Fatalf("home MCP permissions: format=%s inspect=%t write=%t trust=%t", home.Format(), home.Inspect(), home.Write(), home.RequiresTrust())
	}

	project, err := adapter.MCP(harnessapi.Context{}, harnessapi.ProjectScope)
	if err != nil {
		t.Fatal(err)
	}
	if paths := stringsFrom(t, project.Paths()); len(paths) != 0 || project.Inspect() || project.Write() || project.RequiresTrust() {
		t.Fatalf("unexpected project MCP channel: paths=%v inspect=%t write=%t trust=%t", paths, project.Inspect(), project.Write(), project.RequiresTrust())
	}
	if _, err := adapter.MCP(harnessapi.Context{}, "unsupported"); code(err) != "harness_scope_unsupported" {
		t.Fatalf("invalid MCP scope refusal: %v", err)
	}
	if _, err := adapter.ReleaseProbe(harnessapi.Context{}); code(err) != harnessapi.CapabilityUnsupported {
		t.Fatalf("release probe refusal: %v", err)
	}
}
