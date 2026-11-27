// Package cursor delegates the Cursor harness adapter to
// github.com/relux-works/archon-pantheon/pkg/agentic/systems/cursor.
package cursor

import (
	"github.com/relux-works/archon-pantheon/pkg/agentic/systems/cursor"
	"github.com/relux-works/archon/internal/harnessapi"
)

const ID harnessapi.ID = "cursor"

type Adapter = cursor.HarnessAdapter

func New() harnessapi.Adapter { return cursor.NewHarnessAdapter() }
