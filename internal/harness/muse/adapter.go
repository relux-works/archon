// Package muse delegates the Muse harness adapter to
// github.com/relux-works/curator-runtimes/pkg/agentic/systems/muse.
package muse

import (
	"github.com/relux-works/curator-runtimes/pkg/agentic/systems/muse"
	"github.com/relux-works/curator/internal/harnessapi"
)

const ID harnessapi.ID = "muse"

type Adapter = muse.HarnessAdapter

func New() harnessapi.Adapter { return muse.NewHarnessAdapter() }
