// Package harnessapi delegates the versioned value-only harness contract to
// github.com/relux-works/archon-pantheon/pkg/harnessapi.
package harnessapi

import upstream "github.com/relux-works/archon-pantheon/pkg/harnessapi"

type ID = upstream.ID
type APIVersion = upstream.APIVersion
type Form = upstream.Form
type Scope = upstream.Scope
type Capability = upstream.Capability
type Adapter = upstream.Adapter

const (
	AdapterAPIVersionV1 = upstream.AdapterAPIVersionV1
	ProbeTimeoutV1      = upstream.ProbeTimeoutV1

	Monolithic = upstream.Monolithic
	Referenced = upstream.Referenced

	UserScope    = upstream.UserScope
	ProjectScope = upstream.ProjectScope

	ManagedHome          = upstream.ManagedHome
	InstructionOutput    = upstream.InstructionOutput
	SkillProjection      = upstream.SkillProjection
	NativeSkillDiscovery = upstream.NativeSkillDiscovery
	MCPInspection        = upstream.MCPInspection
	NativeMCPWrite       = upstream.NativeMCPWrite
	ProjectContextOutput = upstream.ProjectContextOutput
	HomeLease            = upstream.HomeLease
	CredentialMetadata   = upstream.CredentialMetadata
	TrustObservation     = upstream.TrustObservation
	ReleaseProbing       = upstream.ReleaseProbing
)
