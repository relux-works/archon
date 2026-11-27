package envsnapshot

import (
	"os"
	"testing"

	"github.com/relux-works/archon/internal/testgitenv"
)

// Isolate all scratch Git repositories and native homes before fixture setup.
func TestMain(m *testing.M) {
	cleanup := testgitenv.Isolate()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
