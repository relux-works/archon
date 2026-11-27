// Package kimi_code delegates the Kimi Code managed-home adapter to
// github.com/relux-works/archon-pantheon/pkg/agentic/systems/kimi.
package kimi_code

import (
	"github.com/relux-works/archon-pantheon/pkg/agentic/systems/kimi"
	"github.com/relux-works/archon/internal/harnessapi"
)

const ID harnessapi.ID = "kimi_code"

type Adapter = kimi.HarnessAdapter

func New() harnessapi.Adapter                     { return kimi.NewHarnessAdapter() }
func ManagedHomeSpec() harnessapi.ManagedHomeSpec { return kimi.ManagedHomeSpec() }
