package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// Known hash values generated with openssl passwd:
//
//	$1$testsalt$y987TrgtxSula470KDCCr1                                                             (MD5, password: testpass)
//	$5$testsalt$GR6PqdknD2fHavVjM//Q.4Qni8EXZKnxS838p5GC9r5                                       (SHA-256, password: testpass)
//	$6$testsalt$zcc0po6c786cz9LdMIli0E4Zox6uXK6Khb536rxCF/JO..UDVYHeg9zCKnpkm0FyMFumVno4DCKiS8pQLicRP. (SHA-512, password: testpass)

const (
	md5Hash    = "$1$testsalt$y987TrgtxSula470KDCCr1"
	sha256Hash = "$5$testsalt$GR6PqdknD2fHavVjM//Q.4Qni8EXZKnxS838p5GC9r5"
	sha512Hash = "$6$testsalt$zcc0po6c786cz9LdMIli0E4Zox6uXK6Khb536rxCF/JO..UDVYHeg9zCKnpkm0FyMFumVno4DCKiS8pQLicRP."
)

func writeShadowFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "shadow")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerify_SHA256_ValidPassword(t *testing.T) {
	path := writeShadowFile(t, "admin:"+sha256Hash+":19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to return true for correct password")
	}
}

func TestVerify_SHA512_ValidPassword(t *testing.T) {
	path := writeShadowFile(t, "admin:"+sha512Hash+":19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to return true for correct password")
	}
}

func TestVerify_MD5_ValidPassword(t *testing.T) {
	path := writeShadowFile(t, "admin:"+md5Hash+":19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to return true for correct password")
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	path := writeShadowFile(t, "admin:"+sha256Hash+":19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "wrongpassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected Verify to return false for wrong password")
	}
}

func TestVerify_UserNotFound(t *testing.T) {
	path := writeShadowFile(t, "admin:"+sha256Hash+":19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("nobody", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected Verify to return false for nonexistent user")
	}
}

func TestVerify_LockedAccount(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"exclamation", "!"},
		{"asterisk", "*"},
		{"double exclamation", "!!"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeShadowFile(t, "admin:"+tt.hash+":19000:0:99999:7:::\n")
			sa := NewShadowAuth(path)

			ok, err := sa.Verify("admin", "testpass")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatal("expected Verify to return false for locked account")
			}
		})
	}
}

func TestVerify_EmptyShadowFile(t *testing.T) {
	path := writeShadowFile(t, "")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected Verify to return false for empty shadow file")
	}
}

func TestVerify_FileNotFound(t *testing.T) {
	sa := NewShadowAuth("/nonexistent/shadow")

	ok, err := sa.Verify("admin", "testpass")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if ok {
		t.Fatal("expected Verify to return false for nonexistent file")
	}
}

func TestVerify_MultipleUsers(t *testing.T) {
	content := "root:" + sha512Hash + ":19000:0:99999:7:::\n" +
		"nobody:*:19000:0:99999:7:::\n" +
		"admin:" + sha256Hash + ":19000:0:99999:7:::\n"
	path := writeShadowFile(t, content)
	sa := NewShadowAuth(path)

	// admin should work with SHA-256 hash
	ok, err := sa.Verify("admin", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to return true for admin")
	}

	// root should work with SHA-512 hash
	ok, err = sa.Verify("root", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Verify to return true for root")
	}

	// nobody is locked
	ok, err = sa.Verify("nobody", "testpass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected Verify to return false for locked nobody")
	}
}

func TestVerify_UnsupportedHashFormat(t *testing.T) {
	// $2b$ is bcrypt, not supported by our implementation
	path := writeShadowFile(t, "admin:$2b$12$somesaltandhashvalue:19000:0:99999:7:::\n")
	sa := NewShadowAuth(path)

	ok, err := sa.Verify("admin", "testpass")
	if err == nil {
		t.Fatal("expected error for unsupported hash format")
	}
	if ok {
		t.Fatal("expected Verify to return false for unsupported hash format")
	}
}

func TestFindEntry_ParsesFieldsCorrectly(t *testing.T) {
	content := "admin:" + sha256Hash + ":19000:0:99999:7:::\n"
	path := writeShadowFile(t, content)
	sa := NewShadowAuth(path)

	entry, err := sa.findEntry("admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.username != "admin" {
		t.Errorf("expected username 'admin', got %q", entry.username)
	}
	if entry.hash != sha256Hash {
		t.Errorf("expected hash %q, got %q", sha256Hash, entry.hash)
	}
}

func TestFindEntry_ReturnsNilForMissingUser(t *testing.T) {
	content := "admin:" + sha256Hash + ":19000:0:99999:7:::\n"
	path := writeShadowFile(t, content)
	sa := NewShadowAuth(path)

	entry, err := sa.findEntry("ghost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil entry for missing user")
	}
}
