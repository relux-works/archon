package harnessboundary

import (
	"errors"
	"reflect"
	"testing"

	"github.com/relux-works/curator/internal/harnessapi"
	"github.com/relux-works/curator/internal/harnessregistry"
)

func code(err error) string {
	var refusal harnessapi.Refusal
	if errors.As(err, &refusal) {
		return refusal.Code()
	}
	return ""
}

// Unsupported registration versions refuse before construction; a reported
// version mismatch refuses after metadata only. Correct metadata passes, while
// ID/capability mismatches never return an adapter for an operational call.

// effectSpy counts a successful operation result or a nonzero result paired
// with refusal. Staged refusals must produce neither; the source effect gate
// separately rejects direct filesystem, process, and network effects.
type effectSpy struct{ count int }

func (s *effectSpy) observe(value any, err error) error {
	if err == nil || !reflect.ValueOf(value).IsZero() {
		s.count++
	}
	return err
}

// Every unported method is observed through the same effect spy.
func notPortedResults(adapter harnessapi.Adapter, effects *effectSpy) []error {
	c, s, r := harnessapi.Context{}, harnessapi.Snapshot{}, harnessapi.Request{}
	var errors []error
	v, err := adapter.ParseReleaseProbe(c, harnessapi.ProbeOutput{})
	errors = append(errors, effects.observe(v, err))
	vh, err := adapter.Home(c)
	errors = append(errors, effects.observe(vh, err))
	vi, err := adapter.Instructions(c, harnessapi.Monolithic)
	errors = append(errors, effects.observe(vi, err))
	vs, err := adapter.Skills(c, harnessapi.ProjectScope)
	errors = append(errors, effects.observe(vs, err))
	vset, err := adapter.Settings(c)
	errors = append(errors, effects.observe(vset, err))
	vmcp, err := adapter.MCP(c, harnessapi.UserScope)
	errors = append(errors, effects.observe(vmcp, err))
	vc, err := adapter.Credentials(c)
	errors = append(errors, effects.observe(vc, err))
	vt, err := adapter.Trust(c)
	errors = append(errors, effects.observe(vt, err))
	vr, err := adapter.Preflight(c, s, r)
	errors = append(errors, effects.observe(vr, err))
	vp, err := adapter.PlanSeed(c, s, r)
	errors = append(errors, effects.observe(vp, err))
	vpc, err := adapter.ProjectContext(c, s, r)
	errors = append(errors, effects.observe(vpc, err))
	vstatus, err := adapter.Status(c, s)
	errors = append(errors, effects.observe(vstatus, err))
	vd, err := adapter.Drift(c, s)
	errors = append(errors, effects.observe(vd, err))
	return errors
}

// Staged capabilities describe implementations, rather than future native
// support. A false operational declaration fails this same conformance check.
func stagedCapabilities(adapter harnessapi.Adapter) error {
	for _, capability := range adapter.Capabilities().Values() {
		if capability != harnessapi.ReleaseProbing {
			return harnessapi.NewRefusal(harnessapi.RegistryMismatch)
		}
	}
	effects := effectSpy{}
	for _, err := range notPortedResults(adapter, &effects) {
		if code(err) != harnessapi.OperationNotPorted {
			return harnessapi.NewRefusal(harnessapi.RegistryMismatch)
		}
	}
	if effects.count != 0 {
		return harnessapi.NewRefusal(harnessapi.RegistryMismatch)
	}
	_, err := adapter.ReleaseProbe(harnessapi.Context{})
	if adapter.Capabilities().Has(harnessapi.ReleaseProbing) {
		if err != nil {
			return harnessapi.NewRefusal(harnessapi.RegistryMismatch)
		}
	} else if code(err) != harnessapi.CapabilityUnsupported {
		return harnessapi.NewRefusal(harnessapi.RegistryMismatch)
	}
	return nil
}

type falseCapability struct{ harnessapi.Adapter }

func (falseCapability) Capabilities() harnessapi.CapabilitySet {
	set, _ := harnessapi.NewCapabilitySet(harnessapi.ManagedHome)
	return set
}

func TestStagedOperationsAndCapabilities(t *testing.T) {
	var positive effectSpy
	if err := positive.observe(harnessapi.HomeLayout{}, nil); err != nil {
		t.Fatalf("positive effect observation failed: %v", err)
	}
	if positive.count != 1 {
		t.Fatal("effect spy positive control is disconnected")
	}
	for _, id := range harnessregistry.IDs() {
		t.Run(string(id), func(t *testing.T) {
			adapter, err := harnessregistry.Lookup(id)
			if err != nil {
				t.Fatal(err)
			}
			if err := stagedCapabilities(adapter); err != nil {
				t.Fatal(err)
			}
			if err := stagedCapabilities(falseCapability{adapter}); code(err) != harnessapi.RegistryMismatch {
				t.Fatalf("false claim escaped: %v", err)
			}
		})
	}
}
