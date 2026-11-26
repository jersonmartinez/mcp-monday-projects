package oauth

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigFromEnvDefaultsAndRequirements(t *testing.T) {
	get := func(key string) string {
		values := map[string]string{
			"MCP_OAUTH_CLIENT_ID":     "client",
			"MCP_OAUTH_CLIENT_SECRET": "secret",
			"MCP_OAUTH_TOKEN_FILE":    "/tmp/oauth.json",
		}
		return values[key]
	}
	cfg, err := ConfigFromEnv(get)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthorizeURL != defaultAuthorizeURL || cfg.TokenURL != defaultTokenURL || cfg.RevocationURL != defaultRevokeURL {
		t.Fatalf("unexpected OAuth endpoints: %+v", cfg)
	}
	if cfg.RedirectURI != defaultRedirectURI {
		t.Fatalf("redirect URI = %q", cfg.RedirectURI)
	}
}

func TestAuthorizationURLUsesPKCEAndState(t *testing.T) {
	cfg := Config{ClientID: "client", RedirectURI: "http://127.0.0.1:8765/oauth/callback", AuthorizeURL: defaultAuthorizeURL, Scopes: "me:read boards:read"}
	got := cfg.authorizationURL("state-value", "verifier-value")
	for _, part := range []string{"client_id=client", "redirect_uri=", "state=state-value", "code_challenge=", "code_challenge_method=S256", "scope=me%3Aread+boards%3Aread"} {
		if !strings.Contains(got, part) {
			t.Fatalf("authorization URL %q does not contain %q", got, part)
		}
	}
}

func TestTokenFileIsPrivateAndLoadsAccessToken(t *testing.T) {
	path := t.TempDir() + "/nested/oauth.json"
	token := Token{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := saveToken(path, token); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token permissions = %o, want 600", info.Mode().Perm())
	}
	access, err := LoadAccessTokenFile(path)
	if err != nil || access != "access" {
		t.Fatalf("access=%q err=%v", access, err)
	}
}
