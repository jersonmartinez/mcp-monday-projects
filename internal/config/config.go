package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAPIURL = "https://api.monday.com/v2"
const defaultAPIVersion = "2026-07"

// Config contains validated runtime configuration for the MCP server.
type Config struct {
	APIToken         string
	APIVersion       string
	APIURL           string
	LogLevel         string
	HTTPTimeout      time.Duration
	MaxResponseBytes int64
	MaxRetries       int
}

// Load reads and validates configuration from environment variables.
func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv("MONDAY_API_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("MONDAY_API_TOKEN is required")
	}

	apiURL := valueOrDefault("MONDAY_API_URL", defaultAPIURL)
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return Config{}, fmt.Errorf("MONDAY_API_URL must be a valid HTTPS URL")
	}

	timeout, err := parseDuration("MCP_HTTP_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxResponseBytes, err := parseInt64("MCP_MAX_RESPONSE_BYTES", 4*1024*1024)
	if err != nil || maxResponseBytes < 1024 {
		return Config{}, fmt.Errorf("MCP_MAX_RESPONSE_BYTES must be at least 1024 bytes")
	}
	retries, err := parseInt64("MCP_MAX_RETRIES", 2)
	if err != nil || retries < 0 || retries > 5 {
		return Config{}, fmt.Errorf("MCP_MAX_RETRIES must be between 0 and 5")
	}

	return Config{
		APIToken:         token,
		APIVersion:       valueOrDefault("MONDAY_API_VERSION", defaultAPIVersion),
		APIURL:           apiURL,
		LogLevel:         valueOrDefault("MCP_LOG_LEVEL", "info"),
		HTTPTimeout:      timeout,
		MaxResponseBytes: maxResponseBytes,
		MaxRetries:       int(retries),
	}, nil
}

func valueOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func parseDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := valueOrDefault(name, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func parseInt64(name string, fallback int64) (int64, error) {
	value := valueOrDefault(name, strconv.FormatInt(fallback, 10))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}
