// Package opencode delegates the OpenCode harness adapter to
// github.com/relux-works/archon-pantheon/pkg/agentic/systems/opencode.
package opencode

import (
	"github.com/relux-works/archon-pantheon/pkg/agentic/systems/opencode"
	"github.com/relux-works/archon/internal/harnessapi"
)

const ID harnessapi.ID = "opencode"

type Adapter = opencode.HarnessAdapter

func New() harnessapi.Adapter { return opencode.NewHarnessAdapter() }
