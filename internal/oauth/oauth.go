// Package oauth implements Monday OAuth 2.1 + PKCE for per-user credentials.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultAuthorizeURL = "https://auth.monday.com/oauth2/authorize"
	defaultTokenURL     = "https://auth.monday.com/oauth_ms/oauth/token"
	defaultRevokeURL    = "https://auth.monday.com/oauth_ms/oauth/revoke"
	defaultRedirectURI  = "http://127.0.0.1:8765/oauth/callback"
)

// Config contains public OAuth client settings and the local token-file path.
// ClientSecret is never written to the token file.
type Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	Scopes        string
	Subdomain     string
	TokenFile     string
	AuthorizeURL  string
	TokenURL      string
	RevocationURL string
	HTTPClient    *http.Client
}

// Token is the OAuth 2.1 token pair stored for one user/account authorization.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope,omitempty"`
	Region       string    `json:"region,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ConfigFromEnv loads the OAuth client configuration without printing secrets.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		ClientID:      strings.TrimSpace(getenv("MCP_OAUTH_CLIENT_ID")),
		ClientSecret:  strings.TrimSpace(getenv("MCP_OAUTH_CLIENT_SECRET")),
		RedirectURI:   strings.TrimSpace(getenv("MCP_OAUTH_REDIRECT_URI")),
		Scopes:        strings.TrimSpace(getenv("MCP_OAUTH_SCOPES")),
		Subdomain:     strings.TrimSpace(getenv("MCP_OAUTH_SUBDOMAIN")),
		TokenFile:     strings.TrimSpace(getenv("MCP_OAUTH_TOKEN_FILE")),
		AuthorizeURL:  strings.TrimSpace(getenv("MCP_OAUTH_AUTHORIZE_URL")),
		TokenURL:      strings.TrimSpace(getenv("MCP_OAUTH_TOKEN_URL")),
		RevocationURL: strings.TrimSpace(getenv("MCP_OAUTH_REVOCATION_URL")),
		HTTPClient:    &http.Client{Timeout: 30 * time.Second},
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return Config{}, errors.New("MCP_OAUTH_CLIENT_ID and MCP_OAUTH_CLIENT_SECRET are required")
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = defaultRedirectURI
	}
	if cfg.TokenFile == "" {
		return Config{}, errors.New("MCP_OAUTH_TOKEN_FILE is required")
	}
	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = defaultAuthorizeURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = defaultTokenURL
	}
	if cfg.RevocationURL == "" {
		cfg.RevocationURL = defaultRevokeURL
	}
	return cfg, nil
}

// LoadToken reads a token file and refuses group/world-readable permissions.
func LoadToken(path string) (Token, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Token{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Token{}, fmt.Errorf("OAuth token file %s must not be group/world-readable", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Token{}, err
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, fmt.Errorf("invalid OAuth token file: %w", err)
	}
	if token.AccessToken == "" {
		return Token{}, errors.New("OAuth token file has no access_token")
	}
	return token, nil
}

// LoadAccessTokenFile is used by stdio configuration to load one user's token.
func LoadAccessTokenFile(path string) (string, error) {
	token, err := LoadToken(path)
	if err != nil {
		return "", err
	}
	if !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt) {
		return "", errors.New("OAuth access token is expired; run the oauth refresh command")
	}
	return token.AccessToken, nil
}

func saveToken(path string, token Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".oauth-token-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c Config) authorizationURL(state, verifier string) string {
	u, _ := url.Parse(c.AuthorizeURL)
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("state", state)
	q.Set("code_challenge", challenge(verifier))
	q.Set("code_challenge_method", "S256")
	if c.Scopes != "" {
		q.Set("scope", c.Scopes)
	}
	if c.Subdomain != "" {
		q.Set("subdomain", c.Subdomain)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func exchange(ctx context.Context, c Config, values url.Values) (Token, error) {
	values.Set("client_id", c.ClientID)
	values.Set("client_secret", c.ClientSecret)
	body, err := json.Marshal(values)
	if err != nil {
		return Token{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(string(body)))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return Token{}, fmt.Errorf("OAuth token endpoint returned HTTP %d", resp.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		Region       string `json:"region"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Token{}, fmt.Errorf("invalid OAuth token response: %w", err)
	}
	if raw.AccessToken == "" || raw.RefreshToken == "" {
		return Token{}, errors.New("OAuth token response did not include access_token and refresh_token")
	}
	return Token{AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken, TokenType: raw.TokenType, Scope: raw.Scope, Region: raw.Region, ExpiresAt: time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)}, nil
}

// Exchange exchanges a one-time authorization code using its PKCE verifier.
func Exchange(ctx context.Context, c Config, code, verifier string) (Token, error) {
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.RedirectURI}, "code_verifier": {verifier}}
	return exchange(ctx, c, values)
}

// Refresh rotates and persists a refresh-token grant.
func Refresh(ctx context.Context, c Config, current Token) (Token, error) {
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {current.RefreshToken}}
	return exchange(ctx, c, values)
}

// Revoke revokes the refresh token for this authorization.
func Revoke(ctx context.Context, c Config, token Token) error {
	body, _ := json.Marshal(map[string]string{"token": token.RefreshToken, "token_type_hint": "refresh_token", "client_id": c.ClientID, "client_secret": c.ClientSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RevocationURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("OAuth revocation endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Login opens a browser, handles the localhost callback, exchanges the code,
// and stores the token pair with mode 0600.
func Login(ctx context.Context, c Config) (Token, error) {
	state, err := randomURLSafe(24)
	if err != nil {
		return Token{}, err
	}
	verifier, err := randomURLSafe(48)
	if err != nil {
		return Token{}, err
	}
	u, err := url.Parse(c.RedirectURI)
	if err != nil || u.Hostname() == "" {
		return Token{}, errors.New("MCP_OAUTH_REDIRECT_URI must be a valid URL")
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		return Token{}, errors.New("oauth login only listens on localhost")
	}
	server := &http.Server{}
	result := make(chan Token, 1)
	errs := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errs <- errors.New("OAuth state mismatch")
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("error") != "" {
			errs <- fmt.Errorf("OAuth authorization denied: %s", r.URL.Query().Get("error"))
			http.Error(w, "authorization denied", http.StatusBadRequest)
			return
		}
		token, exchangeErr := Exchange(r.Context(), c, r.URL.Query().Get("code"), verifier)
		if exchangeErr != nil {
			errs <- exchangeErr
			http.Error(w, "token exchange failed", http.StatusBadGateway)
			return
		}
		if saveErr := saveToken(c.TokenFile, token); saveErr != nil {
			errs <- saveErr
			http.Error(w, "token storage failed", http.StatusInternalServerError)
			return
		}
		result <- token
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(template.HTMLEscapeString("Authorization complete. You can close this tab.")))
	})
	listener, err := netListen(u.Host)
	if err != nil {
		return Token{}, err
	}
	server.Handler = mux
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())
	if err := openBrowser(c.authorizationURL(state, verifier)); err != nil {
		fmt.Printf("Open this URL in your browser:\n%s\n", c.authorizationURL(state, verifier))
	}
	select {
	case token := <-result:
		return token, nil
	case err := <-errs:
		return Token{}, err
	case <-ctx.Done():
		return Token{}, ctx.Err()
	case <-time.After(5 * time.Minute):
		return Token{}, errors.New("OAuth callback timed out")
	}
}

// RefreshFile refreshes the token file if it is expired or expires soon.
func RefreshFile(ctx context.Context, c Config) (Token, error) {
	current, err := LoadToken(c.TokenFile)
	if err != nil {
		return Token{}, err
	}
	if current.ExpiresAt.After(time.Now().Add(2 * time.Minute)) {
		return current, nil
	}
	fresh, err := Refresh(ctx, c, current)
	if err != nil {
		return Token{}, err
	}
	if err := saveToken(c.TokenFile, fresh); err != nil {
		return Token{}, err
	}
	return fresh, nil
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

// netListen is a variable to keep Login unit-testable without binding a port.
var netListen = func(address string) (net.Listener, error) { return net.Listen("tcp", address) }
