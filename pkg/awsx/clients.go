// Package awsx builds endpoint-aware AWS clients.
// When AWS_ENDPOINT_URL (or FLOCI_ENDPOINT) is set, clients talk to local
// Floci (http://localhost:4566 on host, http://floci:4566 in compose net).
// Otherwise callers fall back to log stubs.
package awsx

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func Endpoint() string {
	if v := os.Getenv("AWS_ENDPOINT_URL"); v != "" {
		return v
	}
	return os.Getenv("FLOCI_ENDPOINT")
}

func Region() string {
	if v := os.Getenv("AWS_REGION"); v != "" {
		return v
	}
	return "us-east-1"
}

func Enabled() bool { return Endpoint() != "" }

func baseCfg(ctx context.Context) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(Region()),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			firstEnv("AWS_ACCESS_KEY_ID", "test"),
			firstEnv("AWS_SECRET_ACCESS_KEY", "test"), "")),
	}
	if ep := Endpoint(); ep != "" {
		opts = append(opts, config.WithBaseEndpoint(ep))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

func firstEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func SQS(ctx context.Context) (*sqs.Client, error) {
	cfg, err := baseCfg(ctx)
	if err != nil {
		return nil, err
	}
	return sqs.NewFromConfig(cfg), nil
}

func SNS(ctx context.Context) (*sns.Client, error) {
	cfg, err := baseCfg(ctx)
	if err != nil {
		return nil, err
	}
	return sns.NewFromConfig(cfg), nil
}

func S3(ctx context.Context) (*s3.Client, error) {
	cfg, err := baseCfg(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true // required for Floci/LocalStack S3
	}), nil
}

func Secrets(ctx context.Context) (*secretsmanager.Client, error) {
	cfg, err := baseCfg(ctx)
	if err != nil {
		return nil, err
	}
	return secretsmanager.NewFromConfig(cfg), nil
}
