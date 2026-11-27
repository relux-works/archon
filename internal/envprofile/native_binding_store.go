package envprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/relux-works/archon/internal/envregistry"
	"github.com/relux-works/archon/internal/pathboundary"
	"github.com/relux-works/archon/internal/privatedir"
	"github.com/relux-works/archon/internal/protocoljson"
	"github.com/relux-works/archon/internal/stateread"
)

// NativeHomeBindingsPath selects the native-home-bindings/1 sidecar for one
// explicit machine configuration. It does not alter manager-config carriers.
func NativeHomeBindingsPath(configPath string) string {
	return configPath + ".native-home-bindings.json"
}

type nativeBindingsCarrier struct {
	Version  int                `json:"version"`
	Bindings NativeHomeBindings `json:"bindings"`
}

// ReadNativeHomeBindings reads only the selected private sidecar. Its absence
// means no bindings. Source is empty when absent, otherwise the exact sidecar.
func ReadNativeHomeBindings(configPath string) (bindings NativeHomeBindings, source string, err error) {
	return readNativeHomeBindingsWithOwner(configPath, nil)
}
func readNativeHomeBindingsWithOwner(configPath string, owner pathboundary.OwnerLookup) (NativeHomeBindings, string, error) {
	fail := func(err error) (NativeHomeBindings, string, error) {
		return nil, "", fmt.Errorf("%s: native binding sidecar: %w", DiagNativeHomeBindingRequired, err)
	}
	if configPath == "" {
		return fail(fmt.Errorf("selected config path is required"))
	}
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return fail(err)
	}
	path := NativeHomeBindingsPath(configPath)
	state, err := stateread.Lstat(path)
	if err != nil {
		return fail(err)
	}
	if state.Kind == stateread.KindAbsent {
		return NativeHomeBindings{}, "", nil
	}
	if state.Kind != stateread.KindPresent || state.Info == nil {
		return fail(fmt.Errorf("sidecar metadata is unavailable"))
	}
	info := state.Info
	if err := pathboundary.ValidateRootWithOwner(filepath.Dir(path), boundaryOwner(owner)); err != nil {
		return fail(err)
	}
	if err := pathboundary.ValidateLeafWithOwner(filepath.Dir(path), path, boundaryOwner(owner)); err != nil {
		return fail(err)
	}
	if err := privatedir.ValidateFile(path); err != nil {
		return fail(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return fail(fmt.Errorf("sidecar mode must be 0600"))
	}
	file, err := pathboundary.OpenReadNoFollow(path)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(info, opened) {
		return fail(fmt.Errorf("sidecar changed while opening"))
	}
	payload, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return fail(err)
	}
	if len(payload) > 64<<10 {
		return fail(fmt.Errorf("sidecar exceeds 64 KiB"))
	}
	if err := protocoljson.Validate(payload); err != nil {
		return fail(err)
	}
	var carrier nativeBindingsCarrier
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&carrier); err != nil {
		return fail(err)
	}
	if carrier.Version != 1 || carrier.Bindings == nil {
		return fail(fmt.Errorf("expected native-home-bindings/1 version and bindings"))
	}
	keys := bindingKeys(carrier.Bindings)
	for _, id := range keys {
		if err := ValidateNativeHomeBindingValue(id, carrier.Bindings[id]); err != nil {
			return fail(err)
		}
	}
	return carrier.Bindings, path, nil
}

func bindingKeys(bindings NativeHomeBindings) []string {
	keys := make([]string, 0, len(bindings))
	for id := range bindings {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return keys
}

// SetNativeHomeBinding persists one explicit knob with no change to the machine
// config. Existing bindings must remain admissible and distinct before writing.
func SetNativeHomeBinding(configPath, manager, id, home string) error {
	if _, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{id: home}, id); err != nil {
		return err
	}
	bindings, _, err := ReadNativeHomeBindings(configPath)
	if err != nil {
		return err
	}
	bindings[id] = home
	if _, err := nativeBindingResolver(manager, bindings, bindingKeys(bindings)); err != nil {
		return err
	}
	return writeNativeHomeBindings(configPath, bindings)
}

// UnsetNativeHomeBinding removes only a named persisted selection. Unsetting a
// stale target does not inspect the native home, and never reads its payloads.
func UnsetNativeHomeBinding(configPath, id string) error {
	if _, err := envregistry.ByID(id); err != nil {
		return fmt.Errorf("%s: %w", DiagNativeHomeBindingRequired, err)
	}
	bindings, _, err := ReadNativeHomeBindings(configPath)
	if err != nil {
		return err
	}
	if _, present := bindings[id]; !present {
		return fmt.Errorf("%s: native_home.%s is not set", DiagNativeHomeBindingRequired, id)
	}
	delete(bindings, id)
	return writeNativeHomeBindings(configPath, bindings)
}

// Replace atomically from an owner-only staged file. Never unlink the previous
// sidecar as a rename fallback, and never rewrite the selected machine config.
func writeNativeHomeBindings(configPath string, bindings NativeHomeBindings) error {
	if configPath == "" {
		return fmt.Errorf("%s: selected config path is required", DiagNativeHomeBindingRequired)
	}
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	path := NativeHomeBindingsPath(configPath)
	dir := filepath.Dir(path)
	if err := pathboundary.ValidateRootWithOwner(dir, pathboundary.DefaultOwnerLookup()); err != nil {
		return fmt.Errorf("%s: %w", DiagNativeHomeBindingRequired, err)
	}
	if _, _, err := ReadNativeHomeBindings(configPath); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(nativeBindingsCarrier{Version: 1, Bindings: bindings}, "", "  ")
	if err != nil {
		return err
	}
	staged, err := privatedir.CreateTemp(dir, ".native-home-bindings-*.tmp")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer func() { _ = staged.Close(); _ = os.Remove(stagedPath) }()
	if _, err := staged.Write(append(payload, '\n')); err != nil {
		return err
	}
	if err := staged.Sync(); err != nil {
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if _, _, err := ReadNativeHomeBindings(configPath); err != nil {
		return err
	}
	return os.Rename(stagedPath, path)
}

// MergeNativeHomeBindings applies the explicit CLI selection over the persisted
// knob for each adapter. Validation of the effective target set follows merging.
func MergeNativeHomeBindings(persisted, explicit NativeHomeBindings) NativeHomeBindings {
	merged := NativeHomeBindings{}
	for id, home := range persisted {
		merged[id] = home
	}
	for id, home := range explicit {
		merged[id] = home
	}
	return merged
}
