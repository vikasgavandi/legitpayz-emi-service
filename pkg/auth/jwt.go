package auth

import (
	"errors"
	"os"
	"time"

	"github.com/vikasgavandi/legitpayz-emi-service/pkg/secrets"

	"github.com/golang-jwt/jwt/v5"
)

func Secret() []byte {
	// AWS Secrets Manager (Floci: cc/jwt) -> JWT_SECRET env -> dev default
	if s := secrets.GetKey("cc/jwt", "JWT_SECRET", "JWT_SECRET"); s != "" {
		return []byte(s)
	}
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("dev-secret-change-me")
}

func Issue(customerID string) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": customerID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})
	return t.SignedString(Secret())
}

func Parse(token string) (string, error) {
	t, err := jwt.Parse(token, func(t *jwt.Token) (any, error) { return Secret(), nil })
	if err != nil || !t.Valid {
		return "", errors.New("invalid token")
	}
	sub, _ := t.Claims.(jwt.MapClaims)["sub"].(string)
	return sub, nil
}
