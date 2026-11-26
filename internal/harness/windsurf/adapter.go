// Package windsurf delegates the Windsurf harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/windsurf.
package windsurf

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/windsurf"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "windsurf"

type Adapter = windsurf.HarnessAdapter

func New() harnessapi.Adapter { return windsurf.NewHarnessAdapter() }
