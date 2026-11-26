// Package pi delegates the Pi harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/pi.
package pi

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/pi"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "pi"

type Adapter = pi.HarnessAdapter

func New() harnessapi.Adapter { return pi.NewHarnessAdapter() }
