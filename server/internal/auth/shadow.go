// Package auth provides authentication against /etc/shadow password hashes.
// It supports MD5 ($1$), SHA-256 ($5$), and SHA-512 ($6$) MCF hash formats.
// Pure Go implementation — no cgo dependency, cross-compiles cleanly for ARM.
package auth

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tredoe/osutil/user/crypt"
	_ "github.com/tredoe/osutil/user/crypt/md5_crypt"
	_ "github.com/tredoe/osutil/user/crypt/sha256_crypt"
	_ "github.com/tredoe/osutil/user/crypt/sha512_crypt"
)

// shadowEntry holds parsed fields from a single /etc/shadow line.
type shadowEntry struct {
	username string
	hash     string
}

// ShadowAuth verifies passwords against /etc/shadow hashes.
type ShadowAuth struct {
	path string
}

// ErrUserNotFound reports that the shadow file has no usable password hash for
// the username: the account is absent, locked ("!"), disabled ("*") or has an
// empty hash. It is distinct from a read error on purpose — a caller answers
// 401 for this and 500 when the file itself cannot be read.
var ErrUserNotFound = errors.New("no usable password hash for user")

// NewShadowAuth creates a ShadowAuth that reads from the given shadow file path.
func NewShadowAuth(path string) *ShadowAuth {
	return &ShadowAuth{path: path}
}

// Verify checks whether the given password matches the hash stored for the
// username in the shadow file. Returns (false, nil) if the user is not found
// or the account is locked. Returns an error if the file cannot be read or
// the hash format is unsupported.
func (sa *ShadowAuth) Verify(username, password string) (bool, error) {
	entry, err := sa.findEntry(username)
	if err != nil {
		return false, err
	}
	if entry == nil {
		return false, nil
	}

	// Locked or no-password accounts.
	hash := entry.hash
	if isUnusableHash(hash) {
		return false, nil
	}

	// Detect hash algorithm by prefix.
	crypter, err := newCrypter(hash)
	if err != nil {
		return false, err
	}

	if err := crypter.Verify(hash, []byte(password)); err != nil {
		if err == crypt.ErrKeyMismatch {
			return false, nil
		}
		return false, fmt.Errorf("verify password: %w", err)
	}

	return true, nil
}

// Fingerprint returns the first 8 bytes of the SHA-256 of the user's stored
// password hash, hex encoded. The Web UI puts it in the token's pwh claim and
// re-checks it on every request, so changing the router password invalidates
// every session issued under the old one. It is a fingerprint of the hash, not
// the hash: 16 hex characters are enough to notice a change and carry nothing
// worth attacking.
func (sa *ShadowAuth) Fingerprint(username string) (string, error) {
	entry, err := sa.findEntry(username)
	if err != nil {
		return "", err
	}
	if entry == nil || isUnusableHash(entry.hash) {
		return "", ErrUserNotFound
	}
	sum := sha256.Sum256([]byte(entry.hash))
	return hex.EncodeToString(sum[:8]), nil
}

// isUnusableHash reports whether the shadow field carries no password that can
// be verified: empty, "!" (locked) or "*" (login disabled).
func isUnusableHash(hash string) bool {
	return hash == "" || strings.HasPrefix(hash, "!") || strings.HasPrefix(hash, "*")
}

// findEntry scans the shadow file for a matching username and returns the
// parsed entry, or nil if the user is not found.
func (sa *ShadowAuth) findEntry(username string) (*shadowEntry, error) {
	f, err := os.Open(sa.path)
	if err != nil {
		return nil, fmt.Errorf("open shadow file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.SplitN(line, ":", 3)
		if len(fields) < 2 {
			continue
		}

		if fields[0] == username {
			return &shadowEntry{
				username: fields[0],
				hash:     fields[1],
			}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read shadow file: %w", err)
	}

	return nil, nil
}

// newCrypter returns the appropriate Crypter for the given hash prefix.
// Returns an error for unsupported hash formats instead of panicking.
func newCrypter(hash string) (crypt.Crypter, error) {
	switch {
	case strings.HasPrefix(hash, "$6$"):
		return crypt.New(crypt.SHA512), nil
	case strings.HasPrefix(hash, "$5$"):
		return crypt.New(crypt.SHA256), nil
	case strings.HasPrefix(hash, "$1$"):
		return crypt.New(crypt.MD5), nil
	default:
		return nil, fmt.Errorf("unsupported hash format: %s", extractPrefix(hash))
	}
}

// extractPrefix returns the MCF prefix (e.g. "$6$") from a hash string,
// or the first 10 characters if no standard prefix is found.
func extractPrefix(hash string) string {
	if len(hash) > 3 && hash[0] == '$' {
		idx := strings.Index(hash[1:], "$")
		if idx >= 0 {
			return hash[:idx+2]
		}
	}
	if len(hash) > 10 {
		return hash[:10] + "..."
	}
	return hash
}
