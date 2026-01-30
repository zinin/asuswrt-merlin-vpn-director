package updater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version (vX.Y.Z format).
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string // Original string for display (e.g., "v1.2.3")
}

// validVersionChars defines allowed characters for shell-safe version strings.
// Only alphanumeric, dots, 'v' prefix, and dashes are allowed.
var validVersionChars = regexp.MustCompile(`^[a-zA-Z0-9.v-]+$`)

// semverPattern matches strict semantic versioning: optional v prefix, three numeric parts.
var semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// ParseVersion parses a version string in "vX.Y.Z" or "X.Y.Z" format.
// Returns an error for:
// - "dev" or other non-semver strings
// - Pre-release versions like "v1.2.3-rc1"
// - Incomplete versions like "v1.2"
// - Invalid formats
func ParseVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	// Check for whitespace
	if strings.TrimSpace(s) != s {
		return Version{}, fmt.Errorf("version contains whitespace: %q", s)
	}

	// Check for pre-release suffix (contains dash after version numbers)
	if strings.Contains(s, "-") {
		return Version{}, fmt.Errorf("pre-release versions not supported: %q", s)
	}

	matches := semverPattern.FindStringSubmatch(s)
	if matches == nil {
		return Version{}, fmt.Errorf("invalid version format: %q (expected vX.Y.Z or X.Y.Z)", s)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %v", err)
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %v", err)
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %v", err)
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   s,
	}, nil
}

// Compare compares v with other and returns:
// -1 if v < other
//
//	0 if v == other
//	1 if v > other
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}

	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}

	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	return 0
}

// IsOlderThan returns true if v is strictly older than other.
func (v Version) IsOlderThan(other Version) bool {
	return v.Compare(other) < 0
}

// IsValidVersion validates that a version string contains only safe characters
// for shell embedding. Allowed: [a-zA-Z0-9.v-], length 1-50.
// This is a security check to prevent shell injection attacks.
func IsValidVersion(s string) bool {
	if s == "" {
		return false
	}

	if len(s) > 50 {
		return false
	}

	return validVersionChars.MatchString(s)
}
