package auth

import (
	"testing"
	"time"
)

func TestJWT_CreateAndValidate(t *testing.T) {
	svc := NewJWTService("test-secret-key", time.Hour)

	token, err := svc.Create("admin")
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
	if claims.IssuedAt <= 0 {
		t.Errorf("expected positive IssuedAt, got %d", claims.IssuedAt)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Error("expected ExpiresAt > IssuedAt")
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	svc := NewJWTService("test-secret-key", -time.Hour)

	token, err := svc.Create("admin")
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

	token, err := creator.Create("admin")
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
