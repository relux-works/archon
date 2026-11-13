package envprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Explicit exact bindings win over both managed caller overrides and HOME;
// this proves selection only, without granting authority to mutate that path.
func TestNativeHomeBindingIgnoresCaller(t *testing.T) {
	manager := bindingDirectory(t)
	caller := bindingDirectory(t)
	t.Setenv("HOME", caller)
	for _, adapter := range Adapters {
		decoy := filepath.Join(caller, adapter.ID)
		if err := os.Mkdir(decoy, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(adapter.EnvVar, decoy)
	}
	for _, adapter := range Adapters {
		intended := bindingDirectory(t)
		got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{adapter.ID: intended}, adapter.ID)
		if err != nil || got != intended {
			t.Fatalf("explicit target selection %s = %q, %v; want %q", adapter.ID, got, err, intended)
		}
	}
}

// Missing, relative, unclean and root bindings refuse rather than guessing an
// alternate home. Unknown adapters retain the registered-adapter refusal.
func TestNativeHomeBindingRejectsMissingAndInvalid(t *testing.T) {
	manager := bindingDirectory(t)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "must-not-be-selected"))
	for _, home := range []string{"", "relative", filepath.Join(t.TempDir(), "nul") + string(rune(0)), filepath.Join(t.TempDir(), "x") + string(filepath.Separator) + "..", filepath.VolumeName(t.TempDir()) + string(filepath.Separator)} {
		got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": home}, "codex_cli")
		if got != "" || err == nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
			t.Errorf("invalid binding %q = %q, %v; want binding refusal", home, got, err)
		}
	}
	if got, err := ResolveNativeHomeBinding(manager, nil, "not_registered"); err == nil || got != "" {
		t.Fatalf("unregistered adapter = %q, %v", got, err)
	}
}

// All required bindings are checked before a resolver is returned; a caller
// cannot inspect the first home and only later discover a missing second one.
func TestNativeHomeBindingPreflightsCompleteSet(t *testing.T) {
	manager := bindingDirectory(t)
	first := bindingDirectory(t)
	if resolver, err := nativeBindingResolver(manager, NativeHomeBindings{"codex_cli": first}, []string{"codex_cli", "pi"}); err == nil || resolver != nil || !strings.Contains(err.Error(), DiagNativeHomeBindingRequired) {
		t.Fatalf("partial binding = %v, %v; want refusal before resolution", resolver != nil, err)
	}
	bindings := NativeHomeBindings{"codex_cli": first, "pi": bindingDirectory(t)}
	resolver, err := nativeBindingResolver(manager, bindings, []string{"codex_cli", "pi"})
	if err != nil {
		t.Fatal(err)
	}
	bindings["codex_cli"] = filepath.Join(t.TempDir(), "changed")
	if got, err := resolver("codex_cli"); err != nil || got != first {
		t.Fatalf("frozen explicit target = %q, %v; want %q", got, err, first)
	}
}

func bindingDirectory(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

// Resolution requires an existing directory and refuses duplicate adapter
// targets before a resolver can expose any target to marker inspection.
func TestNativeHomeBindingStrictResolution(t *testing.T) {
	manager := bindingDirectory(t)
	native := bindingDirectory(t)
	if got, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": native}, "codex_cli"); err != nil || got != native {
		t.Fatalf("valid binding = %q, %v", got, err)
	}
	missing := filepath.Join(native, "absent")
	if _, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": missing}, "codex_cli"); err == nil {
		t.Fatal("missing binding directory accepted")
	}
	file := filepath.Join(native, "file")
	if err := os.WriteFile(file, []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": file}, "codex_cli"); err == nil {
		t.Fatal("file accepted as native directory")
	}
	if _, err := ResolveNativeHomeBinding(manager, NativeHomeBindings{"codex_cli": native, "pi": native}, "codex_cli"); err == nil {
		t.Fatal("public resolver accepted duplicate target")
	}
	if resolver, err := nativeBindingResolver(manager, NativeHomeBindings{"codex_cli": native, "pi": native}, []string{"pi", "codex_cli"}); err == nil || resolver != nil {
		t.Fatal("duplicate native target accepted")
	}
}
