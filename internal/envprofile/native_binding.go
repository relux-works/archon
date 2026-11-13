package envprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/relux-works/curator/internal/envregistry"
	"github.com/relux-works/curator/internal/pathboundary"
)

// DiagNativeHomeBindingRequired refuses an absent or inadmissible native target.
const DiagNativeHomeBindingRequired = "native_home_binding_required"

// NativeHomeBindings names exact native adapter directories admitted by the
// caller. Opencode binds its actual directory, not the XDG parent. Bindings
// grant no takeover or credential authority; operation gates still apply.
type NativeHomeBindings map[string]string

// ValidateNativeHomeBindingValue checks an explicitly supplied binding's syntax
// without inspecting a home. Resolution performs the filesystem checks later.
func ValidateNativeHomeBindingValue(id, home string) error {
	if _, err := envregistry.ByID(id); err != nil {
		return fmt.Errorf("%s: %w", DiagNativeHomeBindingRequired, err)
	}
	if home == "" {
		return fmt.Errorf("%s: bind the native home for %s explicitly", DiagNativeHomeBindingRequired, id)
	}
	if strings.IndexByte(home, 0) >= 0 || !filepath.IsAbs(home) || filepath.Clean(home) != home || filepath.Dir(home) == home {
		return fmt.Errorf("%s: %s needs a clean absolute directory other than a filesystem root", DiagNativeHomeBindingRequired, id)
	}
	return nil
}

// ResolveNativeHomeBinding selects and validates the exact bound directory
// before any marker inspection. Neither the environment nor HOME supplies a
// binding. The manager argument identifies state that cannot be a native home.
func ResolveNativeHomeBinding(manager string, bindings NativeHomeBindings, id string) (string, error) {
	if err := ValidateNativeHomeBindingValue(id, bindings[id]); err != nil {
		return "", err
	}
	var infos []os.FileInfo
	selected := ""
	for _, key := range bindingKeys(bindings) {
		home, err := resolveNativeHomeBindingWithOwner(manager, bindings, key, nil)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(home)
		if err != nil {
			return "", err
		}
		for _, prior := range infos {
			if os.SameFile(prior, info) {
				return "", fmt.Errorf("%s: two adapters name one native directory", DiagNativeHomeBindingRequired)
			}
		}
		infos = append(infos, info)
		if key == id {
			selected = home
		}
	}
	return selected, nil
}

func resolveNativeHomeBindingWithOwner(manager string, bindings NativeHomeBindings, id string, owner pathboundary.OwnerLookup) (string, error) {
	home := bindings[id]
	if err := ValidateNativeHomeBindingValue(id, home); err != nil {
		return "", err
	}
	if err := validateNativeBindingTarget(manager, home, owner); err != nil {
		return "", fmt.Errorf("%s: %w", DiagNativeHomeBindingRequired, err)
	}
	return home, nil
}

// Freeze and validate the complete set before returning any target. Sorted
// adapter order makes both plans and refusals deterministic.
func nativeBindingResolver(manager string, bindings NativeHomeBindings, ids []string) (func(string) (string, error), error) {
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	frozen := make(NativeHomeBindings, len(ids))
	// Missing/syntactically invalid bindings precede all filesystem inspection.
	for _, id := range ids {
		if err := ValidateNativeHomeBindingValue(id, bindings[id]); err != nil {
			return nil, err
		}
	}
	keys := make([]string, 0, len(bindings))
	for id := range bindings {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if err := ValidateNativeHomeBindingValue(id, bindings[id]); err != nil {
			return nil, err
		}
	}
	var infos []os.FileInfo
	for _, id := range ids {
		home, err := resolveNativeHomeBindingWithOwner(manager, bindings, id, nil)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(home)
		if err != nil {
			return nil, err
		}
		for _, prior := range infos {
			if os.SameFile(prior, info) {
				return nil, fmt.Errorf("%s: two adapters name one native directory", DiagNativeHomeBindingRequired)
			}
		}
		infos = append(infos, info)
		frozen[id] = home
	}
	return func(id string) (string, error) {
		home, ok := frozen[id]
		if !ok {
			return "", fmt.Errorf("%s: adapter is outside the admitted target set", DiagNativeHomeBindingRequired)
		}
		return home, nil
	}, nil
}
