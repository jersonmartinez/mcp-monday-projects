# ADR 0001: Go modular server with the official MCP SDK

- Status: Accepted
- Date: 2026-09-24

## Context

The project needs a performant, provider-specific MCP server that remains easy
to run from any MCP client and easy to validate in CI.

## Decision

Use Go with the official `github.com/modelcontextprotocol/go-sdk/mcp` package,
a single modular binary, a central Monday GraphQL transport, Docker multi-stage
builds, and Docker Compose for all local execution. Use `AGENTS.md` as the only
canonical LLM instruction file; do not create `CLAUDE.md`.

## Consequences

The binary has low runtime overhead and the protocol boundary remains isolated
from Monday domain services. Go module upgrades and Monday API version changes
must be tested explicitly.
