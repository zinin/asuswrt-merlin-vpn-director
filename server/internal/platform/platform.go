// Package platform names the router firmware a daemon runs on and the one
// fact it needs without a shell: the file passwords are verified against.
// Everything else - tunnels, interfaces, the WAN - comes from
// `vpn-director.sh platform` through service.VPNDirector.Platform, so that
// knowledge stays in the shell platform layer (lib/platform/<name>.sh).
package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// Platform names, as router/files.manifest tags them.
const (
	Merlin   = "merlin"
	Keenetic = "keenetic"
)

// EnvVar overrides detection, like VPD_PLATFORM does for the shell.
const EnvVar = "VPD_PLATFORM"

// probeRootEnv prefixes every probed path, like VPD_PROBE_ROOT for the shell.
const probeRootEnv = "VPD_PROBE_ROOT"

// Platform is a firmware and the password file it keeps.
type Platform struct {
	Name         string // Merlin or Keenetic
	PasswordFile string // /etc/shadow or /opt/etc/passwd
}

// ForName returns the platform called name.
func ForName(name string) (Platform, error) {
	switch name {
	case Merlin:
		return Platform{Name: Merlin, PasswordFile: "/etc/shadow"}, nil
	case Keenetic:
		return Platform{Name: Keenetic, PasswordFile: "/opt/etc/passwd"}, nil
	default:
		return Platform{}, fmt.Errorf("unsupported platform: %q", name)
	}
}

// Detect applies the rules of lib/platform.sh: VPD_PLATFORM when it is set,
// else the file system (under VPD_PROBE_ROOT when that is set).
func Detect() (Platform, error) {
	if name := os.Getenv(EnvVar); name != "" {
		return ForName(name)
	}
	return DetectAt(os.Getenv(probeRootEnv))
}

// DetectAt probes the file system under root ("" is /): /opt/etc/ndm with an
// executable /bin/ndmc is Keenetic, /jffs with an executable /bin/nvram is
// Merlin, in that order.
func DetectAt(root string) (Platform, error) {
	if root == "" {
		root = "/"
	}
	if isDir(filepath.Join(root, "opt/etc/ndm")) && isExecutable(filepath.Join(root, "bin/ndmc")) {
		return ForName(Keenetic)
	}
	if isDir(filepath.Join(root, "jffs")) && isExecutable(filepath.Join(root, "bin/nvram")) {
		return ForName(Merlin)
	}
	return Platform{}, fmt.Errorf("unsupported platform: neither Asuswrt-Merlin nor Keenetic detected")
}

// Resolve is what the daemons call at startup: the --platform flag when
// given, Merlin in dev mode (a development machine is neither router), else
// detection.
func Resolve(flagValue string, dev bool) (Platform, error) {
	switch {
	case flagValue != "":
		return ForName(flagValue)
	case dev:
		return ForName(Merlin)
	default:
		return Detect()
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}
