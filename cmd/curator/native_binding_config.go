package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/relux-works/curator/internal/config"
	"github.com/relux-works/curator/internal/envprofile"
	"github.com/relux-works/curator/internal/envregistry"
)

// native_home is a local sidecar knob, not an extension of an old machine
// configuration carrier. Empty id selects the whole map for show only.
func nativeHomeKnob(knob string) (id string, handled bool, err error) {
	if knob == "native_home" {
		return "", true, nil
	}
	if !strings.HasPrefix(knob, "native_home.") {
		return "", false, nil
	}
	id = strings.TrimPrefix(knob, "native_home.")
	if _, err := envregistry.ByID(id); err != nil {
		return "", true, fmt.Errorf("%s: %w", envprofile.DiagNativeHomeBindingRequired, err)
	}
	return id, true, nil
}
func (c cli) showNativeHomeKnob(cfg *config.Config, id string) int {
	bindings, source, err := envprofile.ReadNativeHomeBindings(cfg.Path)
	if err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	if source == "" {
		source = "none"
	}
	if id == "" {
		return c.printJSON(map[string]any{"bindings": bindings, "source": source})
	}
	var value any
	if home, ok := bindings[id]; ok {
		value = home
	}
	return c.printJSON(map[string]any{"value": value, "source": source})
}
func (c cli) setNativeHomeKnob(cfg *config.Config, id, raw string) int {
	if id == "" {
		_, _ = fmt.Fprintln(c.stderr, "curator: env config set native_home.<adapter> <absolute-directory>")
		return exitUsage
	}
	home := raw
	if json.Valid([]byte(raw)) {
		if err := json.Unmarshal([]byte(raw), &home); err != nil {
			_, _ = fmt.Fprintln(c.stderr, "curator: native_home needs a string path")
			return exitUsage
		}
	}
	if err := envprofile.SetNativeHomeBinding(cfg.Path, cfg.Home(), id, home); err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	return c.showNativeHomeKnob(cfg, id)
}
func (c cli) unsetNativeHomeKnob(cfg *config.Config, id string) int {
	if id == "" {
		_, _ = fmt.Fprintln(c.stderr, "curator: env config unset native_home.<adapter>")
		return exitUsage
	}
	if err := envprofile.UnsetNativeHomeBinding(cfg.Path, id); err != nil {
		_, _ = fmt.Fprintln(c.stderr, "curator:", err)
		return exitFail
	}
	_, _ = fmt.Fprintf(c.stdout, "unset native_home.%s\n", id)
	return exitOK
}
func addNativeHomeKnobs(cfg *config.Config, object map[string]any) error {
	bindings, source, err := envprofile.ReadNativeHomeBindings(cfg.Path)
	if err != nil {
		return err
	}
	if source == "" {
		source = "none"
	}
	object["native_home"] = bindings
	object["native_home_source"] = source
	return nil
}
