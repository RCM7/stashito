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

func TestMaxStorageHeadroomDefaultsToTenPercent(t *testing.T) {
	c, err := processConfig(t, map[string]string{"MAX_STORAGE": "1000"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if c.MaxStorageHeadroom != 10 {
		t.Errorf("MaxStorageHeadroom = %d, want 10", c.MaxStorageHeadroom)
	}
	if got := c.MaxStorageHeadroom.Of(int64(c.MaxStorage)); got != 100 {
		t.Errorf("headroom of 1000 = %d, want 100", got)
	}
}

func TestMaxStorageHeadroomParsed(t *testing.T) {
	for _, in := range []string{"25%", "25", " 25 % "} {
		c, err := processConfig(t, map[string]string{"MAX_STORAGE_HEADROOM": in})
		if err != nil {
			t.Fatalf("Process with MAX_STORAGE_HEADROOM=%q: %v", in, err)
		}
		if c.MaxStorageHeadroom != 25 {
			t.Errorf("MaxStorageHeadroom(%q) = %d, want 25", in, c.MaxStorageHeadroom)
		}
	}
}

func TestMaxStorageHeadroomInvalidFailsStartup(t *testing.T) {
	for _, in := range []string{"abc", "100%", "-1", "2.5%"} {
		if _, err := processConfig(t, map[string]string{"MAX_STORAGE_HEADROOM": in}); err == nil {
			t.Errorf("Process with MAX_STORAGE_HEADROOM=%q = nil error, want error", in)
		}
	}
}
