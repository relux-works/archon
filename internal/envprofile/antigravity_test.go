package envprofile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/archon/internal/contextpkg"
	"github.com/relux-works/archon/internal/envfragment"
	"github.com/relux-works/archon/internal/envregistry"
)

func TestAntigravityManagedHomeLifecycle(t *testing.T) {
	requireLinkCapability(t)
	fx := writeManagedFixture(t, "acme")
	nativeDir := t.TempDir()
	fx.native[envregistry.Antigravity] = nativeDir

	// Before provisioning, antigravity must stay informational in env status --check.
	statusBefore, err := StatusOf(StatusRequest{
		Home:         fx.home,
		Machine:      envregistry.DefaultMachineConfig(),
		NativeHomeOf: fx.request(envregistry.Antigravity).NativeHomeOf,
		LaunchDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StatusOf before provision: %v", err)
	}
	foundNote := false
	for _, note := range statusBefore.Notes {
		if strings.Contains(note, "Antigravity CLI (agy 1.5.16)") {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("StatusOf missing standing Antigravity note: %v", statusBefore.Notes)
	}
	foundRow := false
	for _, row := range statusBefore.Homes {
		if row.Environment == envregistry.Antigravity {
			foundRow = true
			if row.Provisioned {
				t.Fatal("unprovisioned antigravity reported Provisioned=true")
			}
		}
	}
	if !foundRow {
		t.Fatal("StatusOf missing antigravity row")
	}

	// Shared isolation fails closed when native ~/.gemini/antigravity-cli/antigravity-oauth-token is absent.
	req := fx.request(envregistry.Antigravity)
	req.Repair = true
	if _, err := Resolve(req); err == nil || !strings.Contains(err.Error(), envregistry.DiagCredentialConflict) {
		t.Fatalf("shared resolve without native token = %v, want %s", err, envregistry.DiagCredentialConflict)
	}

	// Explicit keychain credential_store refuses with environment_credential_store_unsupported.
	keychainReq := fx.request(envregistry.Antigravity)
	keychainReq.Repair = true
	keychainReq.Machine.CredentialStore = map[string]map[string]envregistry.CredentialStoreSelection{
		fx.profile: {envregistry.Antigravity: envregistry.CredentialStoreKeychain},
	}
	if _, err := Resolve(keychainReq); err == nil || !strings.Contains(err.Error(), envregistry.DiagCredentialStoreUnsupported) {
		t.Fatalf("keychain selector = %v, want %s", err, envregistry.DiagCredentialStoreUnsupported)
	}

	// Provision under isolated mode without requiring native token.
	isoReq := fx.request(envregistry.Antigravity)
	isoReq.Repair = true
	isoReq.Machine.Isolation[fx.profile] = map[string]string{envregistry.Antigravity: envregistry.IsolationIsolated}
	isoRes, err := Resolve(isoReq)
	if err != nil {
		t.Fatalf("isolated Resolve --repair: %v", err)
	}
	if !isoRes.Provisioned {
		t.Fatal("expected Provisioned=true on first resolve")
	}
	managedHome := ManagedHomeDir(fx.home, fx.profile, envregistry.Antigravity)
	if filepath.Base(managedHome) != "antigravity" {
		t.Fatalf("managed home basename = %q, want antigravity (must not be .gemini)", filepath.Base(managedHome))
	}
	if _, err := os.Stat(filepath.Join(managedHome, "config", "AGENTS.md")); err != nil {
		t.Fatalf("config/AGENTS.md not materialized: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(managedHome, ".agent-context", "system-prompt.md")); !os.IsNotExist(err) {
		t.Fatalf("system-prompt.md must not be materialized for antigravity: %v", err)
	}

	// Verify emitted fragment is launch-env-fragment-v7.
	var frag map[string]any
	if err := json.Unmarshal(isoRes.Document, &frag); err != nil {
		t.Fatalf("unmarshal fragment: %v", err)
	}
	if frag["fragment"] != envfragment.VersionV7 {
		t.Fatalf("fragment version = %v, want %s", frag["fragment"], envfragment.VersionV7)
	}
	homeObj, _ := frag["home"].(map[string]any)
	if homeObj["path"] != managedHome || homeObj["transport"] != "flag" {
		t.Fatalf("unexpected fragment home: %v", frag["home"])
	}
	envObj, _ := frag["env"].(map[string]any)
	if len(envObj) != 1 || envObj["AGY_CLI_FORCE_FILE_TOKEN_STORAGE"] != "true" {
		t.Fatalf("unexpected fragment env: %v", envObj)
	}
	credObj, _ := frag["credential_store"].(map[string]any)
	if credObj["selection"] != "file" {
		t.Fatalf("unexpected fragment credential_store: %v", credObj)
	}

	// Fail-closed --format env and --format shell produce zero stdout bytes.
	for _, format := range []string{"env", "shell"} {
		fmtReq := isoReq
		fmtReq.Repair = false
		fmtReq.Format = format
		res, err := Resolve(fmtReq)
		if err == nil || !strings.Contains(err.Error(), envregistry.DiagFlagHomeUnsupported) {
			t.Fatalf("--format %s err = %v, want %s", format, err, envregistry.DiagFlagHomeUnsupported)
		}
		if res != nil && len(res.Document) != 0 {
			t.Fatalf("--format %s emitted %d bytes, want 0", format, len(res.Document))
		}
	}

	// Shadowing path makes status non-current until acknowledged.
	shadowPath := filepath.Join(managedHome, "config", "GEMINI.md")
	if err := os.WriteFile(shadowPath, []byte("# shadow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shadowMachine := envregistry.DefaultMachineConfig()
	shadowMachine.Isolation[fx.profile] = map[string]string{envregistry.Antigravity: envregistry.IsolationIsolated}
	st, err := StatusOf(StatusRequest{Home: fx.home, Machine: shadowMachine, NativeHomeOf: fx.request(envregistry.Antigravity).NativeHomeOf, LaunchDir: t.TempDir()})
	if err != nil {
		t.Fatalf("StatusOf shadow: %v", err)
	}
	var agyRow *HomeState
	for i := range st.Homes {
		if st.Homes[i].Environment == envregistry.Antigravity {
			agyRow = &st.Homes[i]
			break
		}
	}
	if agyRow == nil || agyRow.Current {
		t.Fatalf("expected non-current row with unacknowledged config/GEMINI.md shadow: %+v", agyRow)
	}
	if err := os.Remove(shadowPath); err != nil {
		t.Fatal(err)
	}

	// Shared mode with live native token creates and verifies symlink.
	fxShared := writeManagedFixture(t, "acme")
	nativeShared := t.TempDir()
	fxShared.native[envregistry.Antigravity] = nativeShared
	nativeToken := filepath.Join(nativeShared, "antigravity-cli", "antigravity-oauth-token")
	if err := os.MkdirAll(filepath.Dir(nativeToken), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativeToken, []byte(`{"token":"synthetic"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedReq := fxShared.request(envregistry.Antigravity)
	sharedReq.Repair = true
	if _, err := Resolve(sharedReq); err != nil {
		t.Fatalf("shared Resolve --repair with live native token: %v", err)
	}
	managedToken := filepath.Join(ManagedHomeDir(fxShared.home, fxShared.profile, envregistry.Antigravity), "antigravity-cli", "antigravity-oauth-token")
	gotTarget, err := os.Readlink(managedToken)
	if err != nil || filepath.Clean(gotTarget) != filepath.Clean(nativeToken) {
		t.Fatalf("shared auth link = (%q, %v), want %q", gotTarget, err, nativeToken)
	}
	// Bare resolve stays current lock-free.
	sharedBare := fxShared.request(envregistry.Antigravity)
	if _, err := Resolve(sharedBare); err != nil {
		t.Fatalf("shared bare Resolve: %v", err)
	}
	// Replacing symlink with regular file triggers environment_credential_conflict.
	if err := os.Remove(managedToken); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedToken, []byte(`{"fork":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(sharedReq); err == nil || !strings.Contains(err.Error(), envregistry.DiagCredentialConflict) {
		t.Fatalf("forked token repair = %v, want %s", err, envregistry.DiagCredentialConflict)
	}

	// Context packages carrying antigravity-cli/antigravity-oauth-token are refused.
	pkgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pkgDir, "antigravity-cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "antigravity-cli", "antigravity-oauth-token"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := contextpkg.LoadManifest(pkgDir); err == nil || !strings.Contains(err.Error(), "must not be profile content") {
		t.Fatalf("LoadManifest with antigravity token = %v, want refusal", err)
	}
}
