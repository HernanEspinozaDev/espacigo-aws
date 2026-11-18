// Package s3storage implements the private blob, media, and privacy replay
// storage ports using AWS SDK for Go v2 against S3 / aws-emulator.
package s3storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	defaultEndpointURL = "https://127.0.0.1:9297"
	defaultRegion      = "us-east-1"
)

// NewClient uses regional AWS S3 in production and aws-emulator otherwise.
// For the emulator, it honors AWS endpoint, region, CA and credentials overrides
// and falls back to ~/.aws/emulator/9297 defaults with path-style addressing.
func NewClient(ctx context.Context) (*s3.Client, error) {
	if os.Getenv("APP_PROFILE") == "production" {
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithSharedConfigFiles([]string{}),
			config.WithSharedCredentialsFiles([]string{}),
		)
		if err != nil {
			return nil, err
		}
		if cfg.Region == "" {
			return nil, errors.New("production S3 requires an AWS region")
		}
		return s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = nil
		}), nil
	}
	endpoint := strings.TrimSpace(os.Getenv("AWS_ENDPOINT_URL"))
	if endpoint == "" {
		endpoint = defaultEndpointURL
	}
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = defaultRegion
	}

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	// Supply custom CA bundle if found
	if caPath := resolveCABundlePath(); caPath != "" {
		pemBytes, readErr := os.ReadFile(caPath)
		if readErr != nil {
			return nil, readErr
		}
		opts = append(opts, config.WithCustomCABundle(bytes.NewReader(pemBytes)))
	}

	// Avoid loading ~/.aws/emulator/9297/config unless explicitly requested,
	// because container-generated config files may reference container-internal paths.
	if strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE")) == "" {
		opts = append(opts, config.WithSharedConfigFiles([]string{}))
	}

	if strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")) == "" && strings.TrimSpace(os.Getenv("AWS_SHARED_CREDENTIALS_FILE")) == "" {
		if credsFile := defaultEmulatorCredentialsPath(); credsFile != "" {
			opts = append(opts, config.WithSharedCredentialsFiles([]string{credsFile}))
		} else {
			opts = append(opts, config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("EMULATOR000000000000", "emulator-local-secret", ""),
			))
		}
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}

func resolveCABundlePath() string {
	if envCA := strings.TrimSpace(os.Getenv("AWS_CA_BUNDLE")); envCA != "" {
		if info, err := os.Stat(envCA); err == nil && !info.IsDir() {
			return envCA
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	candidate := filepath.Join(home, ".aws", "emulator", "9297", "ca.pem")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

func defaultEmulatorCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	candidate := filepath.Join(home, ".aws", "emulator", "9297", "emulator_credentials")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

func isPreconditionFailedError(err error) bool {
	if err == nil {
		return false
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && (respErr.HTTPStatusCode() == http.StatusPreconditionFailed || respErr.HTTPStatusCode() == http.StatusConflict) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict", "412", "409":
			return true
		}
	}
	return false
}
