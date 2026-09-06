package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWT_CreateAndValidate(t *testing.T) {
	svc := NewJWTService("test-secret-key", time.Hour)

	token, err := svc.Create("admin", "deadbeefdeadbeef")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("Create: expected non-empty token")
	}

	claims, err := svc.Validate(token)
	if err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
	if claims.Subject != "admin" {
		t.Errorf("expected subject %q, got %q", "admin", claims.Subject)
	}
	if claims.PasswordHash != "deadbeefdeadbeef" {
		t.Errorf("expected pwh %q, got %q", "deadbeefdeadbeef", claims.PasswordHash)
	}
	if claims.IssuedAt <= 0 {
		t.Errorf("expected positive IssuedAt, got %d", claims.IssuedAt)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("expected ExpiresAt > IssuedAt")
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	svc := NewJWTService("test-secret-key", -time.Hour)

	token, err := svc.Create("admin", "deadbeefdeadbeef")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	_, err = svc.Validate(token)
	if err == nil {
		t.Fatal("Validate: expected error for expired token")
	}
}

func TestJWT_WrongSecret(t *testing.T) {
	creator := NewJWTService("secret-one", time.Hour)
	validator := NewJWTService("secret-two", time.Hour)

	token, err := creator.Create("admin", "deadbeefdeadbeef")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	_, err = validator.Validate(token)
	if err == nil {
		t.Fatal("Validate: expected error for wrong secret")
	}
}

func TestJWT_InvalidTokenFormat(t *testing.T) {
	svc := NewJWTService("test-secret-key", time.Hour)

	_, err := svc.Validate("this-is-not-a-jwt")
	if err == nil {
		t.Fatal("Validate: expected error for invalid token format")
	}
}

func TestJWT_EmptyToken(t *testing.T) {
	svc := NewJWTService("test-secret-key", time.Hour)

	_, err := svc.Validate("")
	if err == nil {
		t.Fatal("Validate: expected error for empty token")
	}
}

// TestJWT_TokenWithoutPwhValidatesWithAnEmptyFingerprint pins what happens to a
// token issued before this claim existed: Validate still parses it, and the
// empty PasswordHash never equals a real fingerprint, so the middleware turns
// it into a 401 without a special case.
func TestJWT_TokenWithoutPwhValidatesWithAnEmptyFingerprint(t *testing.T) {
	const secret = "test-secret-key"
	svc := NewJWTService(secret, time.Hour)

	now := time.Now()
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "admin",
		"iat": jwt.NewNumericDate(now),
		"exp": jwt.NewNumericDate(now.Add(time.Hour)),
	})
	signed, err := legacy.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}

	claims, err := svc.Validate(signed)
	if err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
	if claims.PasswordHash != "" {
		t.Errorf("expected an empty pwh for a legacy token, got %q", claims.PasswordHash)
	}
}
