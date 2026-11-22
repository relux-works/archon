package harnessparity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

const onlyHomeAndPathEnvironmentSHA256 = "cad800194f71650a36aa53e4751adc947e5decf894a931a88503cd5bcfb6f397"

// Source: testdata/originalcapture/capture_homes_test.go, blob f77056788ebb66069533d99e25d3ffe14fbb17c6, lines 44-55, capture head 44ff5d16c7dc6d2382981b83e14b0efd9ffddb85.
func onlyHomeAndPathEnvironment() bool {
	entries := os.Environ()
	if len(entries) != 2 {
		return false
	}
	names := map[string]bool{}
	for _, entry := range entries {
		name, _, _ := strings.Cut(entry, "=")
		names[name] = true
	}
	return len(names) == 2 && names["HOME"] && names["PATH"]
}

func TestOnlyHomeAndPathEnvironmentSourcePin(t *testing.T) {
	source, err := os.ReadFile("capture_helpers_test.go")
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(source, []byte("func onlyHomeAndPathEnvironment() bool {\n"))
	if start < 0 {
		t.Fatal("capture helper declaration missing")
	}
	const endMarker = "\n}\n"
	relativeEnd := bytes.Index(source[start:], []byte(endMarker))
	if relativeEnd < 0 {
		t.Fatal("capture helper end missing")
	}
	function := source[start : start+relativeEnd+len(endMarker)]
	digest := sha256.Sum256(function)
	if hex.EncodeToString(digest[:]) != onlyHomeAndPathEnvironmentSHA256 {
		t.Fatal("capture helper source digest mismatch")
	}
	mutant := bytes.Clone(function)
	mutant[0] ^= 1
	mutantDigest := sha256.Sum256(mutant)
	if hex.EncodeToString(mutantDigest[:]) == onlyHomeAndPathEnvironmentSHA256 {
		t.Fatal("one-byte capture helper mutation passed the pinned source check")
	}
}
