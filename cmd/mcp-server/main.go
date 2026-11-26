package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
	mcpserver "github.com/jersonmartinez/mcp-monday-projects/internal/mcpserver"
	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// MCP_LOG_LEVEL is validated by config.Load (debug|info|warn|error), so
	// UnmarshalText cannot fail here; logs always go to stderr.
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	logger.Info("starting mcp-monday-projects", "api_version", cfg.APIVersion, "profile", cfg.Profile, "access_level", cfg.AccessLevel)
	server, err := mcpserver.New(cfg)
	if err != nil {
		logger.Error("invalid server configuration", "error", err)
		os.Exit(1)
	}
	if cfg.Transport == "stdio" {
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			logger.Error("MCP server stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	var tokenValidator func(context.Context, string) error
	if cfg.AuthMode == "request" && cfg.ValidateBearer {
		validationClient := monday.NewClient(cfg)
		tokenValidator = func(ctx context.Context, token string) error {
			_, err := validationClient.Me(monday.WithRequestToken(ctx, token))
			return err
		}
	}
	address := fmt.Sprintf("%s:%d", cfg.HTTPHost, cfg.HTTPPort)
	httpServer := &http.Server{
		Addr: address,
		Handler: mcpserver.NewHTTPHandler(server, cfg.HTTPPath, mcpserver.HTTPAuthOptions{
			Mode:                 cfg.AuthMode,
			AllowSharedToken:     cfg.AllowSharedToken,
			ClientKey:            cfg.ClientKey,
			AllowedTokenPrefixes: cfg.AllowedTokenPrefixes,
			MaxRequestBytes:      cfg.MaxRequestBytes,
			RateLimitRPM:         cfg.HTTPRateLimitRPM,
			RequireTLS:           cfg.RequireTLS,
			ValidateBearer:       cfg.ValidateBearer,
			TokenValidator:       tokenValidator,
		}),
		ReadHeaderTimeout: cfg.HTTPTimeout,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	logger.Info("starting streamable HTTP transport", "address", address, "path", cfg.HTTPPath)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("MCP server stopped", "error", err)
		os.Exit(1)
	}
}
