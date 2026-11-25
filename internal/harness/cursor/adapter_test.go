package cursor

import (
	"errors"
	"reflect"
	"testing"

	"github.com/relux-works/curator/internal/harnessapi"
)

func refusalCode(err error) string {
	var refusal harnessapi.Refusal
	if errors.As(err, &refusal) {
		return refusal.Code()
	}
	return ""
}

func pathStrings(paths harnessapi.NamedPaths) []string {
	values := paths.Paths()
	got := make([]string, 0, len(values))
	for _, path := range values {
		got = append(got, path.String())
	}
	return got
}

func TestCursorAdapterDeclaresOnlyMigratedSurfaces(t *testing.T) {
	adapter := New()
	if adapter.ID() != ID {
		t.Fatalf("ID() = %q, want %q", adapter.ID(), ID)
	}
	if adapter.APIVersion() != harnessapi.AdapterAPIVersionV1 {
		t.Fatalf("APIVersion() = %d, want %d", adapter.APIVersion(), harnessapi.AdapterAPIVersionV1)
	}
	wantCapabilities, err := harnessapi.NewCapabilitySet(harnessapi.SkillProjection, harnessapi.MCPInspection)
	if err != nil {
		t.Fatal(err)
	}
	if got := adapter.Capabilities().Values(); !reflect.DeepEqual(got, wantCapabilities.Values()) {
		t.Fatalf("Capabilities() = %v, want %v", got, wantCapabilities.Values())
	}
	if probe, err := adapter.ReleaseProbe(harnessapi.Context{}); err == nil || refusalCode(err) != harnessapi.CapabilityUnsupported || probe.ExecutableAlias() != "" {
		t.Fatalf("ReleaseProbe() = %#v, %v; want capability refusal", probe, err)
	}
	for _, scope := range []harnessapi.Scope{harnessapi.UserScope, harnessapi.ProjectScope} {
		skills, err := adapter.Skills(harnessapi.Context{}, scope)
		if err != nil {
			t.Fatalf("Skills(%q): %v", scope, err)
		}
		if got := pathStrings(skills.Paths()); !reflect.DeepEqual(got, []string{".cursor/rules"}) {
			t.Fatalf("Skills(%q).Paths() = %v", scope, got)
		}
		if got, want := pathStrings(skills.IgnorePaths()), []string(nil); scope == harnessapi.ProjectScope {
			want = []string{".cursor/rules"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Skills(%q).IgnorePaths() = %v, want %v", scope, got, want)
			}
		} else if len(got) != 0 {
			t.Fatalf("Skills(%q).IgnorePaths() = %v, want empty", scope, got)
		}
		if skills.Scope() != scope || skills.NativeDiscovery() || !skills.Mirror() {
			t.Fatalf("Skills(%q) scope/native/mirror = %q/%t/%t", scope, skills.Scope(), skills.NativeDiscovery(), skills.Mirror())
		}
	}
	for _, scope := range []harnessapi.Scope{harnessapi.UserScope, harnessapi.ProjectScope} {
		mcp, err := adapter.MCP(harnessapi.Context{}, scope)
		if err != nil {
			t.Fatalf("MCP(%q): %v", scope, err)
		}
		if got := pathStrings(mcp.Paths()); !reflect.DeepEqual(got, []string{".cursor/mcp.json"}) {
			t.Fatalf("MCP(%q).Paths() = %v", scope, got)
		}
		if mcp.Format() != harnessapi.JSON || !mcp.Inspect() || mcp.Write() || mcp.RequiresTrust() != (scope == harnessapi.ProjectScope) {
			t.Fatalf("MCP(%q) format/inspect/write/trust = %q/%t/%t/%t", scope, mcp.Format(), mcp.Inspect(), mcp.Write(), mcp.RequiresTrust())
		}
	}
	if _, err := adapter.Skills(harnessapi.Context{}, "invalid"); refusalCode(err) != "harness_scope_unsupported" {
		t.Fatalf("Skills(invalid) error = %v, want scope refusal", err)
	}
	if _, err := adapter.MCP(harnessapi.Context{}, "invalid"); refusalCode(err) != "harness_scope_unsupported" {
		t.Fatalf("MCP(invalid) error = %v, want scope refusal", err)
	}
}
