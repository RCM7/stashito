package upstream

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

const ecrRefreshMargin = 30 * time.Minute

type ecrAPI interface {
	GetAuthorizationToken(ctx context.Context, params *ecr.GetAuthorizationTokenInput, optFns ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

type ecrEntry struct {
	username  string
	password  string
	expiresAt time.Time
}

// ecrCredentials resolves and caches rotating ECR authorization tokens per
// upstream host, refreshing them before expiry.
type ecrCredentials struct {
	mu        sync.Mutex
	cache     map[string]ecrEntry
	newClient func(ctx context.Context, up entity.Upstream) (ecrAPI, error)
}

func newECRCredentials() *ecrCredentials {
	return &ecrCredentials{
		cache:     make(map[string]ecrEntry),
		newClient: newECRClient,
	}
}

func newECRClient(ctx context.Context, up entity.Upstream) (ecrAPI, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(up.ECRRegion()),
	}
	if up.HasCredentials() {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(up.Username, up.Password, ""),
		))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for %s: %w", up.Host, err)
	}
	return ecr.NewFromConfig(cfg), nil
}

// get returns a valid basic-auth pair for the ECR upstream, fetching a fresh
// authorization token when the cached one is missing or close to expiry.
func (e *ecrCredentials) get(ctx context.Context, up entity.Upstream) (string, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if entry, ok := e.cache[up.Host]; ok && time.Now().Before(entry.expiresAt.Add(-ecrRefreshMargin)) {
		return entry.username, entry.password, nil
	}

	client, err := e.newClient(ctx, up)
	if err != nil {
		return "", "", err
	}

	out, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", "", fmt.Errorf("getting ECR authorization token for %s: %w", up.Host, err)
	}
	if len(out.AuthorizationData) == 0 || out.AuthorizationData[0].AuthorizationToken == nil {
		return "", "", fmt.Errorf("ECR authorization token response for %s contained no token", up.Host)
	}

	decoded, err := base64.StdEncoding.DecodeString(*out.AuthorizationData[0].AuthorizationToken)
	if err != nil {
		return "", "", fmt.Errorf("decoding ECR authorization token for %s: %w", up.Host, err)
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", fmt.Errorf("malformed ECR authorization token for %s", up.Host)
	}

	expiresAt := time.Now().Add(12 * time.Hour)
	if out.AuthorizationData[0].ExpiresAt != nil {
		expiresAt = *out.AuthorizationData[0].ExpiresAt
	}
	e.cache[up.Host] = ecrEntry{username: username, password: password, expiresAt: expiresAt}

	slog.Info("refreshed ECR authorization token",
		"registry", up.Host,
		"expiresAt", expiresAt,
	)

	return username, password, nil
}
