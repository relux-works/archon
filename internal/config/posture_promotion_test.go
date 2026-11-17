package config

import (
	"encoding/json"
	"testing"
)

// Claim: PromoteSchemaPreservingPosture writes permissive only for a schema-1 object without a posture, and leaves every stated posture untouched.
func TestPromoteSchemaPreservingPosture(t *testing.T) {
	for _, tc := range []struct {
		name        string
		object      map[string]any
		target      int
		wantPosture string
		wantPresent bool
	}{
		{"schema1 without posture gains permissive", map[string]any{"schema_version": float64(1)}, SchemaVersion2, SecurityPosturePermissive, true},
		{"schema1 as json.Number gains permissive", map[string]any{"schema_version": json.Number("1")}, StartupVerificationSchemaVersion, SecurityPosturePermissive, true},
		{"schema1 with explicit hardened keeps it", map[string]any{"schema_version": float64(1), "security_posture": "hardened"}, SchemaVersion2, "hardened", true},
		{"schema1 with explicit permissive keeps it", map[string]any{"schema_version": float64(1), "security_posture": "permissive"}, SchemaVersion2, "permissive", true},
		{"schema2 without posture gains none", map[string]any{"schema_version": float64(2)}, SchemaVersion5, "", false},
		{"schema2 with explicit hardened keeps it", map[string]any{"schema_version": float64(2), "security_posture": "hardened"}, SchemaVersion5, "hardened", true},
		{"schema2 with explicit permissive keeps it", map[string]any{"schema_version": float64(2), "security_posture": "permissive"}, SchemaVersion7, "permissive", true},
		{"schema5 without posture gains none", map[string]any{"schema_version": float64(5)}, SchemaVersion5, "", false},
		{"missing schema gains no posture", map[string]any{}, SchemaVersion2, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			PromoteSchemaPreservingPosture(tc.object, tc.target)
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
	PromoteSchemaPreservingPosture(nil, SchemaVersion2)
}
