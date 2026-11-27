package harnessregistry

import (
	upstream "github.com/relux-works/archon-pantheon/pkg/harnessregistry"
	"github.com/relux-works/archon/internal/harnessapi"
)

func ManagedHomeSpecs() []harnessapi.ManagedHomeSpec { return upstream.ManagedHomeSpecs() }
