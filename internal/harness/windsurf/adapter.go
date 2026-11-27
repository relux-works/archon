// Package windsurf delegates the Windsurf harness adapter to
// github.com/relux-works/archon-pantheon/pkg/agentic/systems/windsurf.
package windsurf

import (
	"github.com/relux-works/archon-pantheon/pkg/agentic/systems/windsurf"
	"github.com/relux-works/archon/internal/harnessapi"
)

const ID harnessapi.ID = "windsurf"

type Adapter = windsurf.HarnessAdapter

func New() harnessapi.Adapter { return windsurf.NewHarnessAdapter() }
