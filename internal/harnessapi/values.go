package harnessapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/relux-works/curator/internal/harnessvalue"
)

// The following constants define the closed contract vocabulary and bounds.
const (
	UnknownID             = "environment_unknown"
	APIVersionUnsupported = "harness_adapter_api_version_unsupported"
	RegistryMismatch      = "harness_adapter_registry_mismatch"
	CapabilityUnsupported = "harness_capability_unsupported"
	ProbeSpecUnsupported  = "harness_probe_spec_unsupported"
	RuntimeMissing        = "adapter_runtime_missing"
)

// Refusal carries a stable code without rendering untrusted input values.
type Refusal struct{ code string }

// NewRefusal bounds a stable diagnostic code without formatting input values.
func NewRefusal(code string) Refusal {
	if code == "" || harnessvalue.Text(code) != nil {
		code = "harness_refusal_invalid"
	}
	return Refusal{code}
}

// Code returns the stable refusal code.
func (r Refusal) Code() string { return r.code }

// Error returns the refusal code without input values.
func (r Refusal) Error() string { return r.code }

// CapabilitySet is an immutable closed set of declared operations.
type CapabilitySet struct{ bits uint64 }

// NewCapabilitySet validates operation identifiers and builds an immutable set.
func NewCapabilitySet(values ...Capability) (CapabilitySet, error) {
	var set CapabilitySet
	for _, value := range values {
		if value < ManagedHome || value > ReleaseProbing {
			return CapabilitySet{}, NewRefusal(CapabilityUnsupported)
		}
		set.bits |= uint64(1) << value
	}
	return set, nil
}

// Has reports whether the operation is explicitly declared.
func (s CapabilitySet) Has(value Capability) bool {
	return value >= ManagedHome && value <= ReleaseProbing && s.bits&(uint64(1)<<value) != 0
}

// Values returns declared operations in stable numeric order.
func (s CapabilitySet) Values() []Capability {
	var values []Capability
	for value := ManagedHome; value <= ReleaseProbing; value++ {
		if s.Has(value) {
			values = append(values, value)
		}
	}
	return values
}

// ProbeSpec remains untrusted data: only the registry's exact tuple admits it.
// Invalid oversized values retain an invalid marker, never a repaired tuple.
type ProbeSpec struct {
	alias       string
	arguments   []string
	timeout     time.Duration
	environment map[string]string
	invalid     bool
}

// NewProbeSpec copies a proposed tuple without granting probe permission.
func NewProbeSpec(alias string, args []string, timeout time.Duration, env map[string]string) ProbeSpec {
	spec := ProbeSpec{alias: alias, timeout: timeout}
	if harnessvalue.Text(alias) != nil || len(args) > harnessvalue.MaxItems || len(env) > harnessvalue.MaxItems {
		return ProbeSpec{invalid: true}
	}
	for _, arg := range args {
		if harnessvalue.Text(arg) != nil {
			return ProbeSpec{invalid: true}
		}
	}
	for key, value := range env {
		if harnessvalue.Text(key) != nil || harnessvalue.Text(value) != nil {
			return ProbeSpec{invalid: true}
		}
	}
	spec.arguments = append([]string(nil), args...)
	spec.environment = copyMap(env)
	return spec
}

// ExecutableAlias returns the proposed bare executable alias.
func (p ProbeSpec) ExecutableAlias() string { return p.alias }

// Arguments returns a separate argument slice.
func (p ProbeSpec) Arguments() []string { return append([]string(nil), p.arguments...) }

// Timeout returns the proposed deadline.
func (p ProbeSpec) Timeout() time.Duration { return p.timeout }

// Environment returns a separate map of proposed overrides.
func (p ProbeSpec) Environment() map[string]string { return copyMap(p.environment) }

// ValidValue reports constructor size validation, independently of probe permission.
func (p ProbeSpec) ValidValue() bool { return !p.invalid }
func copyMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
}

// VerifiedRelease carries a bounded vendor release observation independently of the API version.
type VerifiedRelease struct{ version string }

// NewVerifiedRelease bounds a nonempty vendor release observation.
func NewVerifiedRelease(version string) (VerifiedRelease, error) {
	if version == "" || harnessvalue.Text(version) != nil {
		return VerifiedRelease{}, NewRefusal("harness_release_invalid")
	}
	return VerifiedRelease{version}, nil
}

// Version returns the vendor release observation.
func (v VerifiedRelease) Version() string { return v.version }

// ProbeOutput is an immutable harness contract value.
type ProbeOutput struct {
	stdout, stderr harnessvalue.Bytes
	exitCode       int
}

// NewProbeOutput bounds and copies process output and its observed exit code.
func NewProbeOutput(stdout, stderr []byte, exitCode int) (ProbeOutput, error) {
	out, err := harnessvalue.NewBytes(stdout)
	if err != nil {
		return ProbeOutput{}, err
	}
	errout, err := harnessvalue.NewBytes(stderr)
	if err != nil {
		return ProbeOutput{}, err
	}
	return ProbeOutput{out, errout, exitCode}, nil
}

// Stdout returns a separate copy of standard output.
func (p ProbeOutput) Stdout() []byte { return p.stdout.Copy() }

// Stderr returns a separate copy of standard error.
func (p ProbeOutput) Stderr() []byte { return p.stderr.Copy() }

// ExitCode returns the observed process exit code.
func (p ProbeOutput) ExitCode() int { return p.exitCode }

// Context carries explicit platform/scope inputs; it has no ambient lookup.
type Context struct {
	platform string
	scope    Scope
	release  VerifiedRelease
}

// NewContext validates the platform and scope and binds a release observation.
func NewContext(platform string, scope Scope, release VerifiedRelease) (Context, error) {
	if platform != "linux" && platform != "darwin" && platform != "windows" {
		return Context{}, NewRefusal("harness_platform_unsupported")
	}
	if scope != UserScope && scope != ProjectScope {
		return Context{}, NewRefusal("harness_scope_unsupported")
	}
	return Context{platform, scope, release}, nil
}

// Platform returns the explicit target platform.
func (c Context) Platform() string { return c.platform }

// Scope returns the explicit delivery scope.
func (c Context) Scope() Scope { return c.scope }

// Release returns the immutable vendor release observation.
func (c Context) Release() VerifiedRelease { return c.release }

// Property is approved nonsecret configuration, never a raw native file.
type Property struct{ name, value string }

// NewProperty validates a closed nonsecret observation name and value.
func NewProperty(name, value string) (Property, error) {
	// A closed vocabulary prevents an arbitrary secret-named field entering a snapshot.
	var allowed string
	switch name {
	case "instruction_form":
		allowed = "|monolithic|referenced|"
	case "settings_mode":
		allowed = "|managed|read-only|"
	case "seed_revision":
		allowed = "|A|B|C|"
	case "mcp_format":
		allowed = "|json|jsonc|toml|none|"
	case "credential_route":
		allowed = "|native|file-link|keyring-preferred|unsupported|"
	case "trust_observed", "owned", "present":
		allowed = "|true|false|unknown|"
	default:
		return Property{}, NewRefusal("harness_snapshot_field_unsupported")
	}
	if value == "" || strings.Contains(value, "|") || !strings.Contains(allowed, "|"+value+"|") {
		return Property{}, NewRefusal("harness_snapshot_value_invalid")
	}
	return Property{name, value}, nil
}

// Name returns the approved observation name.
func (p Property) Name() string { return p.name }

// Value returns the approved observation value.
func (p Property) Value() string { return p.value }

// Snapshot is an immutable harness contract value.
type Snapshot struct{ properties []Property }

// NewSnapshot bounds and copies unique approved observations.
func NewSnapshot(properties []Property) (Snapshot, error) {
	if len(properties) > harnessvalue.MaxItems {
		return Snapshot{}, NewRefusal("harness_value_too_large")
	}
	seen := map[string]bool{}
	for _, p := range properties {
		if _, err := NewProperty(p.name, p.value); err != nil {
			return Snapshot{}, err
		}
		if seen[p.name] {
			return Snapshot{}, NewRefusal("harness_snapshot_field_duplicate")
		}
		seen[p.name] = true
	}
	return Snapshot{append([]Property(nil), properties...)}, nil
}

// Properties returns a separate observation slice.
func (s Snapshot) Properties() []Property { return append([]Property(nil), s.properties...) }

// Request carries an explicit operation, instruction form and scope.
type Request struct {
	operation Capability
	form      Form
	scope     Scope
}

// NewRequest validates operation, form and scope without executing them.
func NewRequest(operation Capability, form Form, scope Scope) (Request, error) {
	if _, err := NewCapabilitySet(operation); err != nil {
		return Request{}, err
	}
	if form != Monolithic && form != Referenced {
		return Request{}, NewRefusal("environment_form_unsupported")
	}
	if scope != UserScope && scope != ProjectScope {
		return Request{}, NewRefusal("harness_scope_unsupported")
	}
	return Request{operation, form, scope}, nil
}

// Operation returns the requested operation.
func (r Request) Operation() Capability { return r.operation }

// Form returns the declared instruction form.
func (r Request) Form() Form { return r.form }

// Scope returns the explicit delivery scope.
func (r Request) Scope() Scope { return r.scope }

// NamedPaths describes an ordered native surface without allowing path effects.
type NamedPaths struct{ paths []harnessvalue.RelativePath }

// NewNamedPaths bounds and copies validated relative paths.
func NewNamedPaths(paths []harnessvalue.RelativePath) (NamedPaths, error) {
	if len(paths) > harnessvalue.MaxItems {
		return NamedPaths{}, NewRefusal("harness_value_too_large")
	}
	for _, p := range paths {
		if !p.Valid() {
			return NamedPaths{}, NewRefusal("harness_path_invalid")
		}
	}
	return NamedPaths{append([]harnessvalue.RelativePath(nil), paths...)}, nil
}

// Paths returns a separate ordered path slice.
func (p NamedPaths) Paths() []harnessvalue.RelativePath {
	return append([]harnessvalue.RelativePath(nil), p.paths...)
}

// HomeLayout describes environment selection and ordered relative roots.
type HomeLayout struct {
	environment string
	roots       NamedPaths
	parent      bool
}

// NewHomeLayout binds an environment selector to immutable relative roots.
func NewHomeLayout(environment string, roots NamedPaths, parent bool) (HomeLayout, error) {
	if environment == "" || harnessvalue.Text(environment) != nil {
		return HomeLayout{}, NewRefusal("harness_home_invalid")
	}
	return HomeLayout{environment, roots, parent}, nil
}

// EnvironmentVariable returns the native selector name.
func (h HomeLayout) EnvironmentVariable() string { return h.environment }

// Roots returns the immutable relative-root description.
func (h HomeLayout) Roots() NamedPaths { return h.roots }

// ParentValued reports whether the selector names a parent directory.
func (h HomeLayout) ParentValued() bool { return h.parent }

// InstructionSpec describes destinations, form and ownership notice bytes.
type InstructionSpec struct {
	paths  NamedPaths
	form   Form
	notice harnessvalue.Bytes
}

// NewInstructionSpec validates the form and copies ownership notice bytes.
func NewInstructionSpec(paths NamedPaths, form Form, notice []byte) (InstructionSpec, error) {
	if form != Monolithic && form != Referenced {
		return InstructionSpec{}, NewRefusal("environment_form_unsupported")
	}
	value, err := harnessvalue.NewBytes(notice)
	if err != nil {
		return InstructionSpec{}, err
	}
	return InstructionSpec{paths, form, value}, nil
}

// Paths returns the immutable path description.
func (s InstructionSpec) Paths() NamedPaths { return s.paths }

// Form returns the declared instruction form.
func (s InstructionSpec) Form() Form { return s.form }

// Notice returns a separate copy of ownership notice bytes.
func (s InstructionSpec) Notice() []byte { return s.notice.Copy() }

// SkillDiscovery describes native discovery and projection for an explicit scope.
type SkillDiscovery struct {
	paths, ignores NamedPaths
	scope          Scope
	native, mirror bool
}

// NewSkillDiscovery binds discovery paths and mirror policy to an explicit scope.
func NewSkillDiscovery(paths, ignores NamedPaths, scope Scope, native, mirror bool) (SkillDiscovery, error) {
	if scope != UserScope && scope != ProjectScope {
		return SkillDiscovery{}, NewRefusal("harness_scope_unsupported")
	}
	return SkillDiscovery{paths, ignores, scope, native, mirror}, nil
}

// Paths returns the immutable path description.
func (s SkillDiscovery) Paths() NamedPaths { return s.paths }

// IgnorePaths returns the generated-path ignore description.
func (s SkillDiscovery) IgnorePaths() NamedPaths { return s.ignores }

// Scope returns the explicit delivery scope.
func (s SkillDiscovery) Scope() Scope { return s.scope }

// NativeDiscovery reports native discovery support in this description.
func (s SkillDiscovery) NativeDiscovery() bool { return s.native }

// Mirror reports whether projection is requested.
func (s SkillDiscovery) Mirror() bool { return s.mirror }

// Format identifies an in-memory native serialization.
type Format string

// The following constants define the closed contract vocabulary and bounds.
const (
	JSON     Format = "json"
	TOML     Format = "toml"
	JSONC    Format = "jsonc"
	Markdown Format = "markdown"
)

func validFormat(f Format) bool { return f == JSON || f == TOML || f == JSONC || f == Markdown }

// SettingsSpec describes destinations, format and approved fields.
type SettingsSpec struct {
	paths  NamedPaths
	format Format
	fields []Property
}

// NewSettingsSpec binds a known format to approved fields and destinations.
func NewSettingsSpec(paths NamedPaths, format Format, fields []Property) (SettingsSpec, error) {
	if !validFormat(format) {
		return SettingsSpec{}, NewRefusal("harness_format_unsupported")
	}
	snapshot, err := NewSnapshot(fields)
	if err != nil {
		return SettingsSpec{}, err
	}
	return SettingsSpec{paths, format, snapshot.Properties()}, nil
}

// Paths returns the immutable path description.
func (s SettingsSpec) Paths() NamedPaths { return s.paths }

// Format returns the declared serialization format.
func (s SettingsSpec) Format() Format { return s.format }

// Fields returns a separate approved-field slice.
func (s SettingsSpec) Fields() []Property { return append([]Property(nil), s.fields...) }

// MCPChannel separates inspection, writing and project trust requirements.
type MCPChannel struct {
	paths                 NamedPaths
	format                Format
	inspect, write, trust bool
}

// NewMCPChannel validates inspection and write declarations independently of trust.
func NewMCPChannel(paths NamedPaths, format Format, inspect, write, trust bool) (MCPChannel, error) {
	if !validFormat(format) || write && !inspect {
		return MCPChannel{}, NewRefusal("harness_mcp_invalid")
	}
	return MCPChannel{paths, format, inspect, write, trust}, nil
}

// Paths returns the immutable path description.
func (m MCPChannel) Paths() NamedPaths { return m.paths }

// Format returns the declared serialization format.
func (m MCPChannel) Format() Format { return m.format }

// Inspect reports declared inspection support.
func (m MCPChannel) Inspect() bool { return m.inspect }

// Write reports declared native-write support.
func (m MCPChannel) Write() bool { return m.write }

// RequiresTrust reports whether project admission is required.
func (m MCPChannel) RequiresTrust() bool { return m.trust }

// CredentialRoute describes custody metadata only; it cannot request a copy.
type CredentialRoute struct {
	paths     NamedPaths
	strategy  string
	qualified bool
}

// NewCredentialRoute describes a closed metadata route without credential values or copy actions.
func NewCredentialRoute(paths NamedPaths, strategy string, qualified bool) (CredentialRoute, error) {
	switch strategy {
	case "native", "file-link", "keyring-preferred", "unsupported":
	default:
		return CredentialRoute{}, NewRefusal("environment_credential_unsupported")
	}
	return CredentialRoute{paths, strategy, qualified}, nil
}

// Paths returns the immutable path description.
func (c CredentialRoute) Paths() NamedPaths { return c.paths }

// Strategy returns the closed credential metadata route.
func (c CredentialRoute) Strategy() string { return c.strategy }

// Qualified reports the route's supplied qualification fact.
func (c CredentialRoute) Qualified() bool { return c.qualified }

// TrustPolicy describes required admission without granting it.
type TrustPolicy struct{ required, observationOnly bool }

// NewTrustPolicy describes an observation-only trust requirement.
func NewTrustPolicy(required bool) TrustPolicy { return TrustPolicy{required, true} }

// Required reports whether trust admission is required.
func (p TrustPolicy) Required() bool { return p.required }

// ObservationOnly reports that the policy grants no trust.
func (p TrustPolicy) ObservationOnly() bool { return p.observationOnly }

// FileAction is a proposed owned-file replacement, not an executable callback.
type FileAction struct {
	path     harnessvalue.RelativePath
	bytes    harnessvalue.Bytes
	preimage string
	mode     uint32
}

// NewFileAction bounds replacement bytes and validates path, mode and expected preimage.
func NewFileAction(path harnessvalue.RelativePath, bytes []byte, preimage string, mode uint32) (FileAction, error) {
	if !path.Valid() || mode & ^uint32(0o777) != 0 {
		return FileAction{}, NewRefusal("harness_plan_invalid")
	}
	if preimage != "absent" {
		if len(preimage) != 64 {
			return FileAction{}, NewRefusal("harness_preimage_invalid")
		}
		for _, c := range preimage {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return FileAction{}, NewRefusal("harness_preimage_invalid")
			}
		}
	}
	value, err := harnessvalue.NewBytes(bytes)
	if err != nil {
		return FileAction{}, err
	}
	return FileAction{path, value, preimage, mode}, nil
}

// Path returns the validated relative destination.
func (f FileAction) Path() harnessvalue.RelativePath { return f.path }

// Bytes returns a separate copy of proposed file bytes.
func (f FileAction) Bytes() []byte { return f.bytes.Copy() }

// Preimage returns the expected preceding identity.
func (f FileAction) Preimage() string { return f.preimage }

// Mode returns the proposed permission bits.
func (f FileAction) Mode() uint32 { return f.mode }

// SeedPlan contains bounded file actions or typed refusals.
type SeedPlan struct {
	actions  []FileAction
	refusals []Refusal
}

// NewSeedPlan rejects duplicate destinations and mixed writes and refusals.
func NewSeedPlan(actions []FileAction, refusals []Refusal) (SeedPlan, error) {
	if len(actions) > harnessvalue.MaxItems || len(refusals) > harnessvalue.MaxItems {
		return SeedPlan{}, NewRefusal("harness_value_too_large")
	}
	seen := map[string]bool{}
	total := 0
	for _, action := range actions {
		if _, err := NewFileAction(action.path, action.bytes.Copy(), action.preimage, action.mode); err != nil {
			return SeedPlan{}, err
		}
		if seen[action.path.String()] {
			return SeedPlan{}, NewRefusal("harness_plan_path_duplicate")
		}
		seen[action.path.String()] = true
		total += action.bytes.Len()
		if total > harnessvalue.MaxTextBytes {
			return SeedPlan{}, NewRefusal("harness_value_too_large")
		}
	}
	if len(actions) > 0 && len(refusals) > 0 {
		return SeedPlan{}, NewRefusal("harness_plan_refused_with_actions")
	}
	return SeedPlan{append([]FileAction(nil), actions...), append([]Refusal(nil), refusals...)}, nil
}

// Actions returns a separate file-action slice.
func (s SeedPlan) Actions() []FileAction { return append([]FileAction(nil), s.actions...) }

// Refusals returns a separate refusal slice.
func (s SeedPlan) Refusals() []Refusal { return append([]Refusal(nil), s.refusals...) }

// ProjectContextPlan binds a seed plan to its protected-field description.
type ProjectContextPlan struct {
	plan      SeedPlan
	protected NamedPaths
}

// NewProjectContextPlan binds immutable output and protected-field descriptions.
func NewProjectContextPlan(plan SeedPlan, protected NamedPaths) ProjectContextPlan {
	return ProjectContextPlan{plan, protected}
}

// Plan returns the immutable output description.
func (p ProjectContextPlan) Plan() SeedPlan { return p.plan }

// ProtectedFields returns the protected native-field description.
func (p ProjectContextPlan) ProtectedFields() NamedPaths { return p.protected }

// Status contains approved read-only observations.
type Status struct{ properties []Property }

// NewStatus copies approved status observations.
func NewStatus(properties []Property) (Status, error) {
	s, err := NewSnapshot(properties)
	return Status{s.Properties()}, err
}

// Properties returns a separate observation slice.
func (s Status) Properties() []Property { return append([]Property(nil), s.properties...) }

// DriftReport describes owned paths and typed refusals.
type DriftReport struct {
	paths    NamedPaths
	refusals []Refusal
}

// NewDriftReport bounds and copies drift refusals and path descriptions.
func NewDriftReport(paths NamedPaths, refusals []Refusal) (DriftReport, error) {
	if len(refusals) > harnessvalue.MaxItems {
		return DriftReport{}, fmt.Errorf("harness_value_too_large")
	}
	return DriftReport{paths, append([]Refusal(nil), refusals...)}, nil
}

// Paths returns the immutable path description.
func (d DriftReport) Paths() NamedPaths { return d.paths }

// Refusals returns a separate refusal slice.
func (d DriftReport) Refusals() []Refusal { return append([]Refusal(nil), d.refusals...) }
