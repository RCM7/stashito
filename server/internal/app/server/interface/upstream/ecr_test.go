package upstream

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

type fakeECR struct {
	calls     int
	token     string
	expiresAt time.Time
	err       error
}

func (f *fakeECR) GetAuthorizationToken(ctx context.Context, params *ecr.GetAuthorizationTokenInput, optFns ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &ecr.GetAuthorizationTokenOutput{
		AuthorizationData: []types.AuthorizationData{{
			AuthorizationToken: aws.String(f.token),
			ExpiresAt:          aws.Time(f.expiresAt),
		}},
	}, nil
}

func ecrUpstream() entity.Upstream {
	return entity.Upstream{
		Alias: "ecr",
		Host:  "123456789012.dkr.ecr.eu-west-1.amazonaws.com",
	}
}

func newFakeECRCredentials(fake *fakeECR) *ecrCredentials {
	c := newECRCredentials()
	c.newClient = func(ctx context.Context, up entity.Upstream) (ecrAPI, error) {
		return fake, nil
	}
	return c
}

func TestECRCredentialsGet(t *testing.T) {
	fake := &fakeECR{
		token:     base64.StdEncoding.EncodeToString([]byte("AWS:secrettoken")),
		expiresAt: time.Now().Add(12 * time.Hour),
	}
	c := newFakeECRCredentials(fake)

	user, pass, err := c.get(context.Background(), ecrUpstream())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if user != "AWS" || pass != "secrettoken" {
		t.Errorf("got %q/%q, want AWS/secrettoken", user, pass)
	}

	if _, _, err := c.get(context.Background(), ecrUpstream()); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("GetAuthorizationToken calls = %d, want 1 (cached)", fake.calls)
	}
}

func TestECRCredentialsRefreshNearExpiry(t *testing.T) {
	fake := &fakeECR{
		token:     base64.StdEncoding.EncodeToString([]byte("AWS:secrettoken")),
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	c := newFakeECRCredentials(fake)

	if _, _, err := c.get(context.Background(), ecrUpstream()); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, _, err := c.get(context.Background(), ecrUpstream()); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("GetAuthorizationToken calls = %d, want 2 (refresh inside margin)", fake.calls)
	}
}

func TestECRCredentialsMalformedToken(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"not base64", "%%%"},
		{"no colon", base64.StdEncoding.EncodeToString([]byte("nocolon"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeECR{token: tc.token, expiresAt: time.Now().Add(time.Hour)}
			c := newFakeECRCredentials(fake)
			if _, _, err := c.get(context.Background(), ecrUpstream()); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestECRCredentialsEmptyResponse(t *testing.T) {
	c := newECRCredentials()
	c.newClient = func(ctx context.Context, up entity.Upstream) (ecrAPI, error) {
		return &emptyECR{}, nil
	}
	if _, _, err := c.get(context.Background(), ecrUpstream()); err == nil {
		t.Error("expected error")
	}
}

type emptyECR struct{}

func (emptyECR) GetAuthorizationToken(ctx context.Context, params *ecr.GetAuthorizationTokenInput, optFns ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error) {
	return &ecr.GetAuthorizationTokenOutput{}, nil
}
