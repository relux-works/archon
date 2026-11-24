// Package windsurf owns native skill discovery and MCP inspection policy.
package windsurf

import (
	"github.com/relux-works/curator/internal/harnessapi"
	"github.com/relux-works/curator/internal/harnessvalue"
)

// ID is this module's stable canonical identifier.
const ID harnessapi.ID = "windsurf"

// Adapter exposes the staged harness surface with explicit operational refusals.
type Adapter struct{ harnessapi.UnportedOperations }

// New constructs the module's staged adapter.
func New() harnessapi.Adapter { return Adapter{} }

// ID returns the module's canonical identifier.
func (Adapter) ID() harnessapi.ID { return ID }

// APIVersion returns the implemented internal contract revision.
func (Adapter) APIVersion() harnessapi.APIVersion { return harnessapi.AdapterAPIVersionV1 }

// Capabilities returns only the native discovery and MCP inspection surfaces.
func (Adapter) Capabilities() harnessapi.CapabilitySet {
	capabilities, _ := harnessapi.NewCapabilitySet(harnessapi.NativeSkillDiscovery, harnessapi.MCPInspection)
	return capabilities
}

// Skills describes the native discovery root and the scope-specific mirror.
func (Adapter) Skills(_ harnessapi.Context, scope harnessapi.Scope) (harnessapi.SkillDiscovery, error) {
	paths, err := namedPaths(".agents/skills")
	if err != nil {
		return harnessapi.SkillDiscovery{}, err
	}
	ignores, err := namedPaths(".agents")
	if err != nil {
		return harnessapi.SkillDiscovery{}, err
	}
	return harnessapi.NewSkillDiscovery(paths, ignores, scope, true, scope == harnessapi.UserScope)
}

// MCP describes the read-only home config; project scope has no channel.
func (Adapter) MCP(_ harnessapi.Context, scope harnessapi.Scope) (harnessapi.MCPChannel, error) {
	switch scope {
	case harnessapi.UserScope:
		paths, err := namedPaths(".codeium/windsurf/mcp_config.json")
		if err != nil {
			return harnessapi.MCPChannel{}, err
		}
		return harnessapi.NewMCPChannel(paths, harnessapi.JSON, true, false, false)
	case harnessapi.ProjectScope:
		paths, err := namedPaths()
		if err != nil {
			return harnessapi.MCPChannel{}, err
		}
		return harnessapi.NewMCPChannel(paths, harnessapi.JSON, false, false, false)
	default:
		return harnessapi.MCPChannel{}, harnessapi.NewRefusal("harness_scope_unsupported")
	}
}

func namedPaths(values ...string) (harnessapi.NamedPaths, error) {
	paths := make([]harnessvalue.RelativePath, 0, len(values))
	for _, value := range values {
		path, err := harnessvalue.NewRelativePath(value)
		if err != nil {
			return harnessapi.NamedPaths{}, err
		}
		paths = append(paths, path)
	}
	return harnessapi.NewNamedPaths(paths)
}

// ReleaseProbe returns the real baseline tuple or an unsupported-capability refusal.
func (Adapter) ReleaseProbe(harnessapi.Context) (harnessapi.ProbeSpec, error) {
	return harnessapi.ProbeSpec{}, harnessapi.NewRefusal(harnessapi.CapabilityUnsupported)
}
