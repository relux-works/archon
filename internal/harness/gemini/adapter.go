// Package gemini declares the staged harness surface; native operations remain
// in the legacy implementation until their separately qualified migration.
package gemini

import "github.com/relux-works/curator/internal/harnessapi"

// ID is this module's stable canonical identifier.
const ID harnessapi.ID = "gemini"

// Adapter exposes the staged harness surface with explicit operational refusals.
type Adapter struct{ harnessapi.UnportedOperations }

// New constructs the module's staged adapter.
func New() harnessapi.Adapter { return Adapter{} }

// ID returns the module's canonical identifier.
func (Adapter) ID() harnessapi.ID { return ID }

// APIVersion returns the implemented internal contract revision.
func (Adapter) APIVersion() harnessapi.APIVersion { return harnessapi.AdapterAPIVersionV1 }

// Capabilities returns only actually ported operations.
func (Adapter) Capabilities() harnessapi.CapabilitySet { return harnessapi.CapabilitySet{} }

// ReleaseProbe returns the real baseline tuple or an unsupported-capability refusal.
func (Adapter) ReleaseProbe(harnessapi.Context) (harnessapi.ProbeSpec, error) {
	return harnessapi.ProbeSpec{}, harnessapi.NewRefusal(harnessapi.CapabilityUnsupported)
}
