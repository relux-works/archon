package envsnapshot

import (
	"strings"
	"testing"
)

func TestSnapshotLaunchV2Antigravity(t *testing.T) {
	valid := Descriptor{
		Descriptor:  DescriptorVersionV2,
		Environment: "antigravity",
		Profile: Profile{
			Name:       "sample",
			LockSHA256: strings.Repeat("0", 64),
		},
		Precedence: Precedence{
			Winner:    "higher-weight",
			Placement: "winner-last",
		},
		Home: &HomeBinding{
			Path:      "home",
			Transport: "flag",
		},
		Env: map[string]string{},
		Permissions: Permissions{
			Mode:   "native",
			Locked: false,
			Source: "default",
		},
		MCP: &MCP{
			Path:     "home/config/mcp_config.json",
			EnvNames: []string{"GITHUB_TOKEN"},
			Channels: []Channel{{Kind: "file", Filename: "config/mcp_config.json"}},
		},
	}
	raw, err := EncodeDescriptorV2(valid)
	if err != nil {
		t.Fatalf("EncodeDescriptorV2 valid: %v", err)
	}
	decoded, err := DecodeDescriptorV2(raw)
	if err != nil {
		t.Fatalf("DecodeDescriptorV2 valid: %v", err)
	}
	if decoded.Home == nil || decoded.Home.Path != "home" || decoded.Home.Transport != "flag" || len(decoded.Env) != 0 {
		t.Fatalf("unexpected decoded v2 descriptor: %+v", decoded)
	}
	// Predecessor v1 decoder must refuse v2 descriptor.
	if _, err := DecodeDescriptor(raw); err == nil {
		t.Fatal("v1 DecodeDescriptor accepted snapshot-launch-v2")
	}

	// Non-empty env (e.g. credential policy leaked into portable snapshot) must fail.
	badEnv := valid
	badEnv.Env = map[string]string{"AGY_CLI_FORCE_FILE_TOKEN_STORAGE": "true"}
	if _, err := EncodeDescriptorV2(badEnv); err == nil {
		t.Fatal("snapshot-launch-v2 accepted non-empty env for antigravity")
	}

	// Missing home must fail.
	missingHome := valid
	missingHome.Home = nil
	if _, err := EncodeDescriptorV2(missingHome); err == nil {
		t.Fatal("snapshot-launch-v2 accepted missing home for antigravity")
	}

	// Absolute home path must fail.
	absHome := valid
	absHome.Home = &HomeBinding{Path: "/tmp/home", Transport: "flag"}
	if _, err := EncodeDescriptorV2(absHome); err == nil {
		t.Fatal("snapshot-launch-v2 accepted non-'home' path")
	}
}
