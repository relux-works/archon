package config

import (
	"encoding/json"
	"testing"
)

// Claim: PromoteSchemaPreservingPosture writes the loaded effective posture only for a schema-1 object without a posture, and leaves every stated posture untouched.
func TestPromoteSchemaPreservingPosture(t *testing.T) {
	for _, tc := range []struct {
		name        string
		object      map[string]any
		target      int
		effective   string
		wantPosture string
		wantPresent bool
	}{
		{"schema1 without posture gains permissive when loaded permissive", map[string]any{"schema_version": float64(1)}, SchemaVersion2, SecurityPosturePermissive, SecurityPosturePermissive, true},
		{"schema1 without posture gains hardened when loaded hardened", map[string]any{"schema_version": float64(1)}, SchemaVersion2, SecurityPostureHardened, SecurityPostureHardened, true},
		{"schema1 as json.Number gains hardened when loaded hardened", map[string]any{"schema_version": json.Number("1")}, StartupVerificationSchemaVersion, SecurityPostureHardened, SecurityPostureHardened, true},
		{"schema1 as json.Number gains permissive when loaded permissive", map[string]any{"schema_version": json.Number("1")}, StartupVerificationSchemaVersion, SecurityPosturePermissive, SecurityPosturePermissive, true},
		{"schema1 with explicit hardened keeps it over permissive loaded posture", map[string]any{"schema_version": float64(1), "security_posture": "hardened"}, SchemaVersion2, SecurityPosturePermissive, "hardened", true},
		{"schema1 with explicit permissive keeps it over hardened loaded posture", map[string]any{"schema_version": float64(1), "security_posture": "permissive"}, SchemaVersion2, SecurityPostureHardened, "permissive", true},
		{"schema1 without posture and empty loaded posture gains permissive", map[string]any{"schema_version": float64(1)}, SchemaVersion2, "", SecurityPosturePermissive, true},
		{"schema2 without posture gains none when loaded hardened", map[string]any{"schema_version": float64(2)}, SchemaVersion5, SecurityPostureHardened, "", false},
		{"schema2 without posture gains none when loaded permissive", map[string]any{"schema_version": float64(2)}, SchemaVersion5, SecurityPosturePermissive, "", false},
		{"schema2 with explicit hardened keeps it", map[string]any{"schema_version": float64(2), "security_posture": "hardened"}, SchemaVersion5, SecurityPostureHardened, "hardened", true},
		{"schema2 with explicit permissive keeps it", map[string]any{"schema_version": float64(2), "security_posture": "permissive"}, SchemaVersion7, SecurityPosturePermissive, "permissive", true},
		{"schema5 without posture gains none", map[string]any{"schema_version": float64(5)}, SchemaVersion5, SecurityPostureHardened, "", false},
		{"missing schema gains no posture", map[string]any{}, SchemaVersion2, SecurityPostureHardened, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			PromoteSchemaPreservingPosture(tc.object, tc.target, tc.effective)
			if tc.object["schema_version"] != float64(tc.target) {
				t.Fatalf("schema_version = %v, want %d", tc.object["schema_version"], tc.target)
			}
			posture, present := tc.object["security_posture"]
			if present != tc.wantPresent {
				t.Fatalf("security_posture present = %v, want %v", present, tc.wantPresent)
			}
			if tc.wantPresent && posture != tc.wantPosture {
				t.Fatalf("security_posture = %v, want %q", posture, tc.wantPosture)
			}
		})
	}
	PromoteSchemaPreservingPosture(nil, SchemaVersion2, SecurityPostureHardened)
}
