package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the decoded JWT claims returned by Validate.
type Claims struct {
	Subject   string
	IssuedAt  int64
	ExpiresAt int64
	// PasswordHash is the pwh claim: ShadowAuth.Fingerprint of the user's
	// password hash at the time the token was issued. Empty for tokens minted
	// before the claim existed.
	PasswordHash string
}

// JWTService creates and validates HS256-signed JWT tokens.
type JWTService struct {
	secret   []byte
	duration time.Duration
}

// NewJWTService returns a JWTService that signs tokens with the given secret
// and sets expiration to now + duration.
func NewJWTService(secret string, duration time.Duration) *JWTService {
	return &JWTService{
		secret:   []byte(secret),
		duration: duration,
	}
}

// Create returns a signed JWT with sub, pwh, iat and exp claims. fingerprint
// binds the token to the password it was issued under; see
// ShadowAuth.Fingerprint.
func (s *JWTService) Create(subject, fingerprint string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject,
		"pwh": fingerprint,
		"iat": jwt.NewNumericDate(now),
		"exp": jwt.NewNumericDate(now.Add(s.duration)),
	})
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Validate parses the token string, verifies its HMAC-SHA256 signature and
// expiration, and returns the decoded claims.
func (s *JWTService) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	sub, err := mapClaims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", err)
	}

	iat, err := mapClaims.GetIssuedAt()
	if err != nil {
		return nil, fmt.Errorf("get issued at: %w", err)
	}

	exp, err := mapClaims.GetExpirationTime()
	if err != nil {
		return nil, fmt.Errorf("get expiration: %w", err)
	}

	// A token issued before the pwh claim existed simply has none; the empty
	// string never matches a real fingerprint, so the caller rejects it.
	pwh, _ := mapClaims["pwh"].(string)

	return &Claims{
		Subject:      sub,
		IssuedAt:     iat.Unix(),
		ExpiresAt:    exp.Unix(),
		PasswordHash: pwh,
	}, nil
}
