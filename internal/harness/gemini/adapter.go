// Package gemini delegates the Gemini harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/gemini.
package gemini

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/gemini"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "gemini"

type Adapter = gemini.HarnessAdapter

func Capabilities() harnessapi.CapabilitySet { return gemini.HarnessCapabilities() }
func New() harnessapi.Adapter                { return gemini.NewHarnessAdapter() }
