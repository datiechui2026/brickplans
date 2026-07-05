package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"brickplans/internal/config"
)

// ErrInvalidToken is returned when a token fails signature/format/expiry checks.
var ErrInvalidToken = errors.New("invalid token")

// Claims extends the standard JWT claims with a token type ("access"/"refresh")
// and a TokenVersion used for stateless revocation.
type Claims struct {
	Type string `json:"type"`
	Ver  int    `json:"ver"`
	jwt.RegisteredClaims
}

// CreateAccessToken signs a short-lived access token bound to the user's current
// TokenVersion. Bumping TokenVersion (e.g. on password change) invalidates it.
func CreateAccessToken(cfg *config.Config, sub string, ver int) (string, error) {
	now := time.Now()
	c := Claims{
		Type: "access",
		Ver:  ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(cfg.JWTAccessMin) * time.Minute)),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString([]byte(cfg.SecretKey))
}

// CreateRefreshToken signs a long-lived refresh token, also bound to TokenVersion.
func CreateRefreshToken(cfg *config.Config, sub string, ver int) (string, error) {
	now := time.Now()
	c := Claims{
		Type: "refresh",
		Ver:  ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(cfg.JWTRefreshDays) * 24 * time.Hour)),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString([]byte(cfg.SecretKey))
}

// CreateEmailVerifyToken signs a 24h token for email verification. type=email_verify.
func CreateEmailVerifyToken(cfg *config.Config, sub string) (string, error) {
	now := time.Now()
	c := Claims{
		Type: "email_verify",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString([]byte(cfg.SecretKey))
}

// Parse verifies the signature, algorithm (HS256) and returns claims.
func Parse(cfg *config.Config, tokenStr string) (*Claims, error) {
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(cfg.SecretKey), nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return c, nil
}
