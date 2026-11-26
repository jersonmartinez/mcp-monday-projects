package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jersonmartinez/mcp-monday-projects/internal/oauth"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cfg, err := oauth.ConfigFromEnv(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "login":
		token, err := oauth.Login(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("OAuth authorization completed. Token file: %s\n", cfg.TokenFile)
		fmt.Printf("Expires at: %s\n", token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	case "refresh":
		token, err := oauth.RefreshFile(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("OAuth token refreshed. Expires at: %s\n", token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	case "status":
		token, err := oauth.LoadToken(cfg.TokenFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Authorized: yes\nExpires at: %s\nRegion: %s\nScopes: %s\n", token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"), token.Region, token.Scope)
	case "logout":
		token, err := oauth.LoadToken(cfg.TokenFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := oauth.Revoke(ctx, cfg, token); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.Remove(cfg.TokenFile); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("OAuth authorization revoked and local token file removed.")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mcp-monday-oauth login|refresh|status|logout")
}
