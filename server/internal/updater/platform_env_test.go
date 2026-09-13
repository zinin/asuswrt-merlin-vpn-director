package updater

import (
	"os"
	"testing"

	"github.com/zinin/vpn-director/server/internal/platform"
)

// TestMain pins the platform for the whole package: getPlatform detects it
// from the file system by default, and a development machine is no router.
// A test about detection itself sets platform.EnvVar on its own.
func TestMain(m *testing.M) {
	if os.Getenv(platform.EnvVar) == "" {
		os.Setenv(platform.EnvVar, platform.Merlin)
	}
	os.Exit(m.Run())
}
