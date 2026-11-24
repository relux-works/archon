package envregistry

import (
	"strings"

	"github.com/relux-works/curator/internal/harnessapi"
	"github.com/relux-works/curator/internal/harnessregistry"
)

func init() {
	for _, spec := range harnessregistry.ManagedHomeSpecs() {
		Registry = append(Registry, adapterFromManagedSpec(spec))
		if spec.Alias != "" {
			EnvAliases[spec.Alias] = spec.ID
		}
	}
}

func adapterFromManagedSpec(spec harnessapi.ManagedHomeSpec) Adapter {
	adapter := Adapter{
		ID:                                spec.ID,
		HomeTransport:                     spec.HomeTransport,
		ManagedOnly:                       spec.ManagedOnly,
		NativeDefaultDir:                  spec.NativeDefaultDir,
		NativeHomeIgnoresEnv:              spec.NativeHomeIgnoresEnv,
		ForbiddenHomeBasename:             spec.ForbiddenHomeBasename,
		RootTarget:                        spec.RootTarget,
		SkillsDir:                         spec.SkillsDir,
		Forms:                             append([]string(nil), spec.Forms...),
		DefaultForm:                       spec.DefaultForm,
		MCPFormat:                         spec.MCPFormat,
		ForbiddenMCPEnvNames:              append([]string(nil), spec.ForbiddenMCPEnvNames...),
		CredentialFileGuardPath:           spec.CredentialFileGuardPath,
		ForcedCredentialStore:             CredentialStoreSelection(spec.ForcedCredentialStore),
		ForcedCredentialEnvVar:            spec.ForcedCredentialEnvVar,
		ForcedCredentialEnvValue:          spec.ForcedCredentialEnvValue,
		RequireLiveNativeCredentialTarget: spec.RequireLiveNativeCredentialTarget,
		VerifiedRelease:                   spec.VerifiedRelease,
		Probe:                             append([]string(nil), spec.Probe...),
		SizeAdvisoryBytes:                 spec.SizeAdvisoryBytes,
		CredentialScope:                   spec.CredentialScope,
		GlobalContextCap:                  spec.GlobalContextCap,
		FirstRunSteps:                     spec.FirstRunSteps,
		StatusNote:                        spec.StatusNote,
		NativeLoginHint:                   spec.NativeLoginHint,
		ForcedStoreGuidance:               spec.ForcedStoreGuidance,
		CredentialPolicyNote:              spec.CredentialPolicyNote,
		SeedWritten:                       map[string]bool{},
		Passthrough:                       map[string][]Passthrough{},
	}
	if spec.MCPFilename != "" {
		adapter.MCP = &Channel{Kind: KindFile, Filename: spec.MCPFilename}
	}
	for platform, entries := range spec.Passthrough {
		out := make([]Passthrough, 0, len(entries))
		for _, entry := range entries {
			out = append(out, Passthrough{
				Path:           entry.Path,
				Strategy:       entry.Strategy,
				FileLinkTarget: entry.FileLinkTarget,
			})
		}
		adapter.Passthrough[platform] = out
	}
	for _, shadow := range spec.Shadows {
		adapter.Shadows = append(adapter.Shadows, Shadow{
			Path:    shadow.Path,
			Surface: shadow.Surface,
		})
	}
	return adapter
}

// EffectiveHomeTransport returns the adapter's closed home transport: "flag"
// or "env" (default for legacy env-variable adapters).
func (a Adapter) EffectiveHomeTransport() string {
	if a.HomeTransport == HomeTransportFlag {
		return HomeTransportFlag
	}
	return HomeTransportEnv
}

// IsFlagHome reports whether the adapter relocates its managed home via a CLI
// flag rather than an environment variable.
func (a Adapter) IsFlagHome() bool {
	return a.EffectiveHomeTransport() == HomeTransportFlag
}

// ForbidsMCPEnvName reports whether name is forbidden in this adapter's MCP
// env_names declarations (case-insensitive), preventing ambient auth or
// managed-home policy variables from bypassing credential isolation.
func (a Adapter) ForbidsMCPEnvName(name string) bool {
	for _, forbidden := range a.ForbiddenMCPEnvNames {
		if strings.EqualFold(name, forbidden) {
			return true
		}
	}
	return false
}

// CredentialFileGuardPaths returns the home-relative credential file paths
// registered by managed-home specifications that must never appear in profile
// package content.
func CredentialFileGuardPaths() []string {
	var paths []string
	for _, adapter := range Registry {
		if adapter.CredentialFileGuardPath != "" {
			paths = append(paths, adapter.CredentialFileGuardPath)
		}
	}
	return paths
}

// StatusNotes returns the standing status notes contributed by registered
// adapter specifications.
func StatusNotes() []string {
	var notes []string
	for _, adapter := range Registry {
		if adapter.StatusNote != "" {
			notes = append(notes, adapter.StatusNote)
		}
	}
	return notes
}
