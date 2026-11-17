package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator/internal/config"
)

// These tests drive the production run() entry point for every command that
// promotes the machine file's schema_version, plus the hardened controls
// beside them.

// installPostureFixture installs the path root acme under the given machine
// file text and returns the config source and a path overlay package.
func installPostureFixture(t *testing.T, text string) (stubConfigSource, string) {
	t.Helper()
	t.Setenv("CURATOR_SYSTEM_CONFIG", "")
	source, _ := profileHome(t)
	writeNativeCredentials(t)
	if err := os.WriteFile(source.path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	source = reloadSource(t, source)
	root := t.TempDir()
	writeContextPackage(t, root, "acme", "1.0.0", "root\n")
	overlay := t.TempDir()
	writeContextPackage(t, overlay, "personal", "0.3.0", "overlay\n")
	if code, _, stderr := runProfile(t, source, "profile", "install", root); code != exitOK {
		t.Fatalf("install = %d\nstderr:\n%s", code, stderr)
	}
	return source, overlay
}

// requireRawSchemaPosture asserts the machine file on disk carries the given
// schema_version and, when wantPresent, exactly the given security_posture.
func requireRawSchemaPosture(t *testing.T, source stubConfigSource, wantSchema float64, wantPosture string, wantPresent bool) {
	t.Helper()
	payload, err := os.ReadFile(source.path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	if object["schema_version"] != wantSchema {
		t.Fatalf("schema_version = %v, want %v\n%s", object["schema_version"], wantSchema, payload)
	}
	posture, present := object["security_posture"]
	if present != wantPresent {
		t.Fatalf("security_posture present = %v, want %v\n%s", present, wantPresent, payload)
	}
	if wantPresent && posture != wantPosture {
		t.Fatalf("security_posture = %v, want %q\n%s", posture, wantPosture, payload)
	}
}

// requireProfileUpdateAccepted asserts a following profile update is not
// refused by the hardened source-allowlist gate.
func requireProfileUpdateAccepted(t *testing.T, source stubConfigSource) {
	t.Helper()
	code, stdout, stderr := runProfile(t, source, "profile", "update", "acme")
	if code != exitOK || strings.Contains(stderr, config.DiagSourceAllowlistEmpty) {
		t.Fatalf("profile update after promotion = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

// writeMachineConfigWithSystemEnvDefault loads a schema-1 machine file
// under a system file that supplies only an unlocked environments default.
// The system merge raises the effective schema to 2, so the loaded machine
// is already hardened before any command runs; the helper asserts that
// premise or the promotion assertions beside it would prove nothing.
func writeMachineConfigWithSystemEnvDefault(t *testing.T, machineText string) stubConfigSource {
	t.Helper()
	systemPath := filepath.Join(t.TempDir(), "system.json")
	systemText := `{"schema_version": 2, "locked": [], "environments": {"overlays_allowed": true}}`
	if err := os.WriteFile(systemPath, []byte(systemText), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CURATOR_SYSTEM_CONFIG", systemPath)
	source := writeMachineConfig(t, machineText)
	if source.cfg.Schema != config.SchemaVersion2 || source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
		t.Fatalf("loaded premise = schema %d posture %q, want schema 2 hardened under the system environments default",
			source.cfg.Schema, source.cfg.EffectiveSecurityPosture())
	}
	return source
}

// Claim: compose add and env config set promote a schema-1 machine with an explicit permissive posture, and compose remove leaves that written posture untouched.
func TestSchemaPromotionKeepsEffectivePosture(t *testing.T) {
	t.Run("compose add and remove", func(t *testing.T) {
		source, overlay := installPostureFixture(t, `{"schema_version": 1, "skills_root": "x", "projects": {}}`)
		if code, _, stderr := runProfile(t, source, "profile", "compose", "acme", "add", overlay); code != exitOK {
			t.Fatalf("compose add = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPosturePermissive, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPosturePermissive {
			t.Fatalf("effective posture after compose add = %q, want permissive", source.cfg.EffectiveSecurityPosture())
		}
		requireProfileUpdateAccepted(t, source)
		if code, _, stderr := runProfile(t, source, "profile", "compose", "acme", "remove", overlay); code != exitOK {
			t.Fatalf("compose remove = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPosturePermissive, true)
		source = reloadSource(t, source)
		requireProfileUpdateAccepted(t, source)
	})
	t.Run("env config set generic knob", func(t *testing.T) {
		source, _ := installPostureFixture(t, `{"schema_version": 1, "skills_root": "x", "projects": {}}`)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "precedence.winner", "lower-weight"); code != exitOK {
			t.Fatalf("env config set = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPosturePermissive, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPosturePermissive {
			t.Fatalf("effective posture after env config set = %q, want permissive", source.cfg.EffectiveSecurityPosture())
		}
		requireProfileUpdateAccepted(t, source)
	})
	t.Run("env config set credential store knob", func(t *testing.T) {
		source, _ := installPostureFixture(t, `{"schema_version": 1, "skills_root": "x", "projects": {}}`)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "credential_store.acme.codex", "native"); code != exitOK {
			t.Fatalf("env config set credential_store = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion5), config.SecurityPosturePermissive, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPosturePermissive {
			t.Fatalf("effective posture after credential-store set = %q, want permissive", source.cfg.EffectiveSecurityPosture())
		}
		requireProfileUpdateAccepted(t, source)
	})
}

// Claim: compose add, compose remove and env config set leave a hardened machine hardened and never write a permissive posture over it.
func TestSchemaPromotionLeavesHardenedMachinesUntouched(t *testing.T) {
	t.Setenv("CURATOR_SYSTEM_CONFIG", "")
	hardened := `{"schema_version": 2, "security_posture": "hardened", "skills_root": "x", "projects": {}}`
	t.Run("compose add and remove keep explicit hardened", func(t *testing.T) {
		source := writeMachineConfig(t, hardened)
		code, _, stderr := runProfile(t, source, "profile", "compose", "other", "add",
			"https://example.com/personal", "--range", "^1")
		if code != exitOK {
			t.Fatalf("compose add = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPostureHardened, true)
		if code, _, stderr := runProfile(t, source, "profile", "compose", "other", "remove",
			"https://example.com/personal"); code != exitOK {
			t.Fatalf("compose remove = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPostureHardened, true)
	})
	t.Run("env config set generic knob keeps explicit hardened", func(t *testing.T) {
		source := writeMachineConfig(t, hardened)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "precedence.winner", "lower-weight"); code != exitOK {
			t.Fatalf("env config set = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPostureHardened, true)
	})
	t.Run("env config set credential store knob keeps explicit hardened", func(t *testing.T) {
		source := writeMachineConfig(t, hardened)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "credential_store.acme.codex", "native"); code != exitOK {
			t.Fatalf("env config set credential_store = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion5), config.SecurityPostureHardened, true)
	})
	t.Run("schema 2 without a posture stays hardened by default", func(t *testing.T) {
		source := writeMachineConfig(t, `{"schema_version": 2, "skills_root": "x", "projects": {}}`)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "credential_store.acme.codex", "native"); code != exitOK {
			t.Fatalf("env config set credential_store = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion5), "", false)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
			t.Fatalf("effective posture = %q, want the schema-2 hardened default", source.cfg.EffectiveSecurityPosture())
		}
	})
}

// Claim: under a system environments default the loaded schema-1 machine is already hardened, and each promoting command keeps that hardened posture explicit instead of weakening it to permissive.
func TestSchemaPromotionUnderSystemEnvDefaultKeepsHardened(t *testing.T) {
	schema1 := `{"schema_version": 1, "skills_root": "x", "projects": {}}`
	t.Run("compose add keeps hardened", func(t *testing.T) {
		source := writeMachineConfigWithSystemEnvDefault(t, schema1)
		code, _, stderr := runProfile(t, source, "profile", "compose", "other", "add",
			"https://example.com/personal", "--range", "^1")
		if code != exitOK {
			t.Fatalf("compose add = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPostureHardened, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
			t.Fatalf("effective posture after compose add = %q, want hardened", source.cfg.EffectiveSecurityPosture())
		}
	})
	t.Run("env config set generic knob keeps hardened", func(t *testing.T) {
		source := writeMachineConfigWithSystemEnvDefault(t, schema1)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "precedence.winner", "lower-weight"); code != exitOK {
			t.Fatalf("env config set = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion2), config.SecurityPostureHardened, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
			t.Fatalf("effective posture after env config set = %q, want hardened", source.cfg.EffectiveSecurityPosture())
		}
	})
	t.Run("env config set credential store knob keeps hardened", func(t *testing.T) {
		source := writeMachineConfigWithSystemEnvDefault(t, schema1)
		if code, _, stderr := runProfile(t, source, "env", "config", "set", "credential_store.acme.codex", "native"); code != exitOK {
			t.Fatalf("env config set credential_store = %d\nstderr:\n%s", code, stderr)
		}
		requireRawSchemaPosture(t, source, float64(config.SchemaVersion5), config.SecurityPostureHardened, true)
		source = reloadSource(t, source)
		if source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
			t.Fatalf("effective posture after credential-store set = %q, want hardened", source.cfg.EffectiveSecurityPosture())
		}
	})
}

// Claim: the installed fixture that profile update accepts while permissive is refused with source_allowlist_empty once the same machine states hardened, so the accepted-update assertions above can go red.
func TestProfileUpdateRefusedUnderExplicitHardened(t *testing.T) {
	source, _ := installPostureFixture(t, `{"schema_version": 1, "skills_root": "x", "projects": {}}`)
	requireProfileUpdateAccepted(t, source)
	object, err := config.ReadRaw(source.path)
	if err != nil {
		t.Fatal(err)
	}
	object["schema_version"] = float64(config.SchemaVersion2)
	object["security_posture"] = config.SecurityPostureHardened
	object["allowed_sources"] = []any{}
	if _, err := config.WriteRaw(source.path, object); err != nil {
		t.Fatal(err)
	}
	source = reloadSource(t, source)
	if source.cfg.EffectiveSecurityPosture() != config.SecurityPostureHardened {
		t.Fatalf("effective posture = %q, want hardened", source.cfg.EffectiveSecurityPosture())
	}
	code, stdout, stderr := runProfile(t, source, "profile", "update", "acme")
	if code != exitFail || !strings.Contains(stderr, config.DiagSourceAllowlistEmpty) {
		t.Fatalf("profile update under explicit hardened = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}
