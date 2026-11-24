package harnessregistry

import (
	"github.com/relux-works/curator/internal/harness/antigravity"
	"github.com/relux-works/curator/internal/harnessapi"
)

// ManagedHomeSpecs returns the pure adapter-owned managed-home specifications
// admitted beyond the frozen legacy parity set.
func ManagedHomeSpecs() []harnessapi.ManagedHomeSpec {
	return []harnessapi.ManagedHomeSpec{
		antigravity.ManagedHomeSpec(),
	}
}
