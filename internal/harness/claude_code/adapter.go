// Package claude_code delegates the Claude Code harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/claude.
package claude_code

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/claude"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "claude_code"

type Adapter = claude.HarnessAdapter

func New() harnessapi.Adapter { return claude.NewHarnessAdapter() }
