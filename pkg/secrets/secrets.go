// Package secrets reads from AWS Secrets Manager (Floci locally)
// with env fallback. Secret values may be plain or JSON like {"JWT_SECRET":"..."}.
package secrets

import (
	"context"
	"encoding/json"
	"os"

	"github.com/vikasgavandi/legitpayz-emi-service/pkg/awsx"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

func Get(secretID string) (string, error) {
	ctx := context.Background()
	c, err := awsx.Secrets(ctx)
	if err != nil {
		return "", err
	}
	out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(secretID)})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.SecretString), nil
}

// GetKey fetches secretID then extracts key from JSON, else returns raw value.
func GetKey(secretID, key, fallbackEnv string) string {
	if v := os.Getenv(fallbackEnv); v != "" {
		return v
	}
	if !awsx.Enabled() {
		return ""
	}
	raw, err := Get(secretID)
	if err != nil || raw == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return raw
}
