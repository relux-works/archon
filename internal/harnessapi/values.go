package harnessapi

import (
	"time"

	upstream "github.com/relux-works/archon-pantheon/pkg/harnessapi"
	"github.com/relux-works/archon/internal/harnessvalue"
)

const (
	UnknownID             = upstream.UnknownID
	APIVersionUnsupported = upstream.APIVersionUnsupported
	RegistryMismatch      = upstream.RegistryMismatch
	CapabilityUnsupported = upstream.CapabilityUnsupported
	ProbeSpecUnsupported  = upstream.ProbeSpecUnsupported
	RuntimeMissing        = upstream.RuntimeMissing
)

type Refusal = upstream.Refusal
type CapabilitySet = upstream.CapabilitySet
type ProbeSpec = upstream.ProbeSpec
type VerifiedRelease = upstream.VerifiedRelease
type ProbeOutput = upstream.ProbeOutput
type Context = upstream.Context
type Property = upstream.Property
type Snapshot = upstream.Snapshot
type Request = upstream.Request
type NamedPaths = upstream.NamedPaths
type HomeLayout = upstream.HomeLayout
type InstructionSpec = upstream.InstructionSpec
type SkillDiscovery = upstream.SkillDiscovery
type Format = upstream.Format
type SettingsSpec = upstream.SettingsSpec
type MCPChannel = upstream.MCPChannel
type CredentialRoute = upstream.CredentialRoute
type TrustPolicy = upstream.TrustPolicy
type FileAction = upstream.FileAction
type SeedPlan = upstream.SeedPlan
type ProjectContextPlan = upstream.ProjectContextPlan
type Status = upstream.Status
type DriftReport = upstream.DriftReport

const (
	JSON     = upstream.JSON
	TOML     = upstream.TOML
	JSONC    = upstream.JSONC
	Markdown = upstream.Markdown
)

func NewRefusal(code string) Refusal { return upstream.NewRefusal(code) }
func NewCapabilitySet(values ...Capability) (CapabilitySet, error) {
	return upstream.NewCapabilitySet(values...)
}
func NewProbeSpec(alias string, args []string, timeout time.Duration, env map[string]string) ProbeSpec {
	return upstream.NewProbeSpec(alias, args, timeout, env)
}
func NewVerifiedRelease(version string) (VerifiedRelease, error) {
	return upstream.NewVerifiedRelease(version)
}
func NewProbeOutput(stdout, stderr []byte, exitCode int) (ProbeOutput, error) {
	return upstream.NewProbeOutput(stdout, stderr, exitCode)
}
func NewContext(platform string, scope Scope, release VerifiedRelease) (Context, error) {
	return upstream.NewContext(platform, scope, release)
}
func NewProperty(name, value string) (Property, error) {
	return upstream.NewProperty(name, value)
}
func NewSnapshot(properties []Property) (Snapshot, error) {
	return upstream.NewSnapshot(properties)
}
func NewRequest(operation Capability, form Form, scope Scope) (Request, error) {
	return upstream.NewRequest(operation, form, scope)
}
func NewNamedPaths(paths []harnessvalue.RelativePath) (NamedPaths, error) {
	return upstream.NewNamedPaths(paths)
}
func NewHomeLayout(environment string, roots NamedPaths, parent bool) (HomeLayout, error) {
	return upstream.NewHomeLayout(environment, roots, parent)
}
func NewInstructionSpec(paths NamedPaths, form Form, notice []byte) (InstructionSpec, error) {
	return upstream.NewInstructionSpec(paths, form, notice)
}
func NewSkillDiscovery(paths, ignores NamedPaths, scope Scope, native, mirror bool) (SkillDiscovery, error) {
	return upstream.NewSkillDiscovery(paths, ignores, scope, native, mirror)
}
func NewSettingsSpec(paths NamedPaths, format Format, fields []Property) (SettingsSpec, error) {
	return upstream.NewSettingsSpec(paths, format, fields)
}
func NewMCPChannel(paths NamedPaths, format Format, inspect, write, trust bool) (MCPChannel, error) {
	return upstream.NewMCPChannel(paths, format, inspect, write, trust)
}
func NewCredentialRoute(paths NamedPaths, strategy string, qualified bool) (CredentialRoute, error) {
	return upstream.NewCredentialRoute(paths, strategy, qualified)
}
func NewTrustPolicy(required bool) TrustPolicy { return upstream.NewTrustPolicy(required) }
func NewFileAction(path harnessvalue.RelativePath, bytes []byte, preimage string, mode uint32) (FileAction, error) {
	return upstream.NewFileAction(path, bytes, preimage, mode)
}
func NewSeedPlan(actions []FileAction, refusals []Refusal) (SeedPlan, error) {
	return upstream.NewSeedPlan(actions, refusals)
}
func NewProjectContextPlan(plan SeedPlan, protected NamedPaths) ProjectContextPlan {
	return upstream.NewProjectContextPlan(plan, protected)
}
func NewStatus(properties []Property) (Status, error) { return upstream.NewStatus(properties) }
func NewDriftReport(paths NamedPaths, refusals []Refusal) (DriftReport, error) {
	return upstream.NewDriftReport(paths, refusals)
}
