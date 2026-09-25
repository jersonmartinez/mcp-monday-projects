package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadUsesMondayDefaults(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "test-token")
	t.Setenv("MONDAY_API_URL", "")
	t.Setenv("MONDAY_API_VERSION", "")
	t.Setenv("MCP_HTTP_TIMEOUT", "")
	t.Setenv("MCP_MAX_RESPONSE_BYTES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIVersion != defaultAPIVersion {
		t.Fatalf("APIVersion = %q, want %q", cfg.APIVersion, defaultAPIVersion)
	}
	if cfg.APIURL != defaultAPIURL {
		t.Fatalf("APIURL = %q, want %q", cfg.APIURL, defaultAPIURL)
	}
	if cfg.HTTPTimeout != 15*time.Second {
		t.Fatalf("HTTPTimeout = %s, want 15s", cfg.HTTPTimeout)
	}
}

func TestLoadRequiresToken(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing-token error")
	}
}

func TestLoadRejectsNonHTTPSURL(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "test-token")
	t.Setenv("MONDAY_API_URL", "http://localhost:8080")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want URL validation error")
	}
	_ = os.Unsetenv("MONDAY_API_URL")
}
