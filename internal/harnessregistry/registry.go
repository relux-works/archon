// Package harnessregistry delegates the closed compile-time harness catalog to
// github.com/relux-works/archon-pantheon/pkg/harnessregistry.
package harnessregistry

import (
	upstream "github.com/relux-works/archon-pantheon/pkg/harnessregistry"
	"github.com/relux-works/archon/internal/harnessapi"
)

const DefaultID harnessapi.ID = upstream.DefaultID

func CanonicalID(id harnessapi.ID) harnessapi.ID { return upstream.CanonicalID(id) }
func IDs() []harnessapi.ID                       { return upstream.IDs() }
func IDsWithCapability(capability harnessapi.Capability) []harnessapi.ID {
	return upstream.IDsWithCapability(capability)
}
func Lookup(id harnessapi.ID) (harnessapi.Adapter, error) { return upstream.Lookup(id) }
func Verify(version harnessapi.APIVersion, id harnessapi.ID, capabilities harnessapi.CapabilitySet, constructor func() harnessapi.Adapter) (harnessapi.Adapter, error) {
	return upstream.Verify(version, id, capabilities, constructor)
}
func ProbePolicy(id harnessapi.ID) (harnessapi.ProbeSpec, error) { return upstream.ProbePolicy(id) }
