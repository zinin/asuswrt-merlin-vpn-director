// Package auth provides authentication against /etc/shadow password hashes.
// It supports MD5 ($1$), SHA-256 ($5$), and SHA-512 ($6$) MCF hash formats.
// Pure Go implementation — no cgo dependency, cross-compiles cleanly for ARM.
package auth

import (
	"bufio"
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

	// Locked or disabled accounts.
	hash := entry.hash
	if hash == "" || hash == "!" || hash == "*" || hash == "!!" {
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
