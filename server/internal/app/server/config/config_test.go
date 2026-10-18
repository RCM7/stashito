package config

import (
	"context"
	"testing"

	"github.com/sethvargo/go-envconfig"
)

func processConfig(t *testing.T, extra map[string]string) (Config, error) {
	t.Helper()
	env := map[string]string{
		"PORT":         "8080",
		"STORAGE_PATH": t.TempDir(),
		"LOG_LEVEL":    "info",
		"LOG_FORMAT":   "text",
		"TAG_TTL":      "60s",
	}
	for k, v := range extra {
		env[k] = v
	}
	var c Config
	err := envconfig.ProcessWith(context.Background(), &envconfig.Config{Target: &c, Lookuper: envconfig.MapLookuper(env)})
	return c, err
}

func TestMaxStorageUnsetMeansUnlimited(t *testing.T) {
	c, err := processConfig(t, nil)
	if err != nil {
		t.Fatalf("Process without MAX_STORAGE: %v", err)
	}
	if c.MaxStorage != 0 {
		t.Errorf("MaxStorage = %d, want 0", c.MaxStorage)
	}
}

func TestMaxStorageParsed(t *testing.T) {
	c, err := processConfig(t, map[string]string{"MAX_STORAGE": "50GB"})
	if err != nil {
		t.Fatalf("Process with MAX_STORAGE: %v", err)
	}
	if int64(c.MaxStorage) != 50<<30 {
		t.Errorf("MaxStorage = %d, want %d", c.MaxStorage, 50<<30)
	}
}

func TestMaxStorageInvalidFailsStartup(t *testing.T) {
	if _, err := processConfig(t, map[string]string{"MAX_STORAGE": "lots"}); err == nil {
		t.Fatal("Process with invalid MAX_STORAGE = nil error, want error")
	}
}
