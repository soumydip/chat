package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
	"unicode"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

func CreateNewUUIDV7() string {
	id := uuid.NewV7()

	return id.String()
}

func ValidatePasswordStrength(password string) error {
	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	switch {
	case !hasUpper:
		return errors.New("password must contain at least one uppercase letter")
	case !hasLower:
		return errors.New("password must contain at least one lowercase letter")
	case !hasDigit:
		return errors.New("password must contain at least one digit")
	case !hasSpecial:
		return errors.New("password must contain at least one special character")
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// raw token: cookie-তে যায়। DB-তে শুধু HashToken(raw) যাবে।
func GenerateRefreshToken() (string, error) {
	return randomHex(32)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func GenerateAccessToken(userID, username, sessionID string) (string, error) {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		return "", errors.New("JWT_SECRET environment variable is not set")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":    userID,
		"session_id": sessionID,
		"exp":        now.Add(15 * time.Minute).Unix(),
		"iat":        now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}
