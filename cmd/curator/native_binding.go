package main

import (
	"fmt"
	"strings"

	"github.com/relux-works/archon/internal/envprofile"
)

// nativeHomeFlags accepts only explicitly named exact native directories. It
// never fills omissions from the caller's environment or user-home callback.
type nativeHomeFlags struct{ homes envprofile.NativeHomeBindings }

func (f *nativeHomeFlags) String() string { return "" }

func (f *nativeHomeFlags) Set(value string) error {
	id, home, ok := strings.Cut(value, "=")
	if !ok {
		return fmt.Errorf("native-home needs <env-id>=<absolute-directory>")
	}
	if err := envprofile.ValidateNativeHomeBindingValue(id, home); err != nil {
		return err
	}
	if _, exists := f.homes[id]; exists {
		return fmt.Errorf("native-home repeats environment %q", id)
	}
	if f.homes == nil {
		f.homes = envprofile.NativeHomeBindings{}
	}
	f.homes[id] = home
	return nil
}
