// Package codex_cli delegates the Codex CLI harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/codex.
package codex_cli

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/codex"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "codex_cli"

type Adapter = codex.HarnessAdapter

func New() harnessapi.Adapter { return codex.NewHarnessAdapter() }
