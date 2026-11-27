// Package antigravity delegates the Antigravity managed-home adapter to
// github.com/relux-works/archon-pantheon/pkg/agentic/systems/agy.
package antigravity

import (
	"github.com/relux-works/archon-pantheon/pkg/agentic/systems/agy"
	"github.com/relux-works/archon/internal/harnessapi"
)

const ID harnessapi.ID = "antigravity"

type Adapter = agy.HarnessAdapter

func New() harnessapi.Adapter                     { return agy.NewHarnessAdapter() }
func ManagedHomeSpec() harnessapi.ManagedHomeSpec { return agy.ManagedHomeSpec() }
