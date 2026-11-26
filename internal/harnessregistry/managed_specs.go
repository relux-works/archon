package harnessregistry

import (
	upstream "github.com/relux-works/curator-runtimes/pkg/harnessregistry"
	"github.com/relux-works/curator/internal/harnessapi"
)

func ManagedHomeSpecs() []harnessapi.ManagedHomeSpec { return upstream.ManagedHomeSpecs() }
