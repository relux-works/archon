package envprofile

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/relux-works/archon/internal/envregistry"
)

func machineScopeAdapters(home string) ([]Adapter, error) {
	scoped, err := ScopedCurrents(home)
	if err != nil {
		return nil, err
	}
	var adapters []Adapter
	for _, adapter := range Adapters {
		skip := false
		for scope := range scoped {
			environment, _ := splitScope(scope)
			if environment == adapter.ID {
				skip = true
				break
			}
		}
		if !skip {
			adapters = append(adapters, adapter)
		}
	}
	return adapters, nil
}

func scopedAdapters(home, environment, target string) ([]Adapter, error) {
	if environment != "" {
		adapter, ok := adapterByID(environment)
		if !ok {
			return nil, fmt.Errorf("%s: explicit operand names the unregistered environment %q", envregistry.DiagUnknown, environment)
		}
		return []Adapter{adapter}, nil
	}
	if target != "" {
		// Secondary fixed-home targets do not write surfaces in this stage.
		return nil, nil
	}
	return machineScopeAdapters(home)
}

func missingRuntimeRows(adapters []Adapter) ([]string, error) {
	var missing []string
	for _, adapter := range adapters {
		registered, err := envregistry.ByID(adapter.ID)
		if err != nil {
			return nil, err
		}
		if len(registered.Probe) == 0 || registered.Probe[0] == "" {
			return nil, fmt.Errorf("%s: adapter %s has no registered runtime probe", DiagSourceInvalid, adapter.ID)
		}
		runtime := registered.Probe[0]
		if _, err := exec.LookPath(runtime); err != nil {
			missing = append(missing, fmt.Sprintf("adapter %s requires runtime %s", adapter.ID, runtime))
		}
	}
	return missing, nil
}

func requireAdapterRuntimes(home, environment, target string) error {
	adapters, err := scopedAdapters(home, environment, target)
	if err != nil {
		return err
	}
	missing, err := missingRuntimeRows(adapters)
	if err != nil || len(missing) == 0 {
		return err
	}
	return fmt.Errorf("%s: %s runtime not found on PATH", DiagAdapterRuntimeMissing, strings.Join(missing, "; "))
}

func withNoUseRuntimeWarnings(home string, info Info, sink io.Writer) Info {
	adapters, err := machineScopeAdapters(home)
	if err != nil {
		return info
	}
	var verifiedAdapters []Adapter
	for _, adapter := range adapters {
		if registered, err := envregistry.ByID(adapter.ID); err == nil && registered.VerifiedRelease == "" {
			continue
		}
		verifiedAdapters = append(verifiedAdapters, adapter)
	}
	missing, err := missingRuntimeRows(verifiedAdapters)
	if err != nil || len(missing) == 0 {
		return info
	}
	warnings := make([]string, 0, len(missing))
	for _, row := range missing {
		warnings = append(warnings, fmt.Sprintf("%s: %s; profile registration remains and no adapter home was changed", DiagAdapterRuntimeMissing, row))
	}
	info.Warnings = append(info.Warnings, warnings...)
	writeUpdateWarnings(sink, "", warnings)
	return info
}
