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

// Create returns a signed JWT with sub, iat, and exp claims.
func (s *JWTService) Create(subject string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": subject,
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

	return &Claims{
		Subject:   sub,
		IssuedAt:  iat.Unix(),
		ExpiresAt: exp.Unix(),
	}, nil
}
