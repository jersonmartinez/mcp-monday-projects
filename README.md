# mcp-monday-projects

[![CI](https://github.com/jersonmartinez/mcp-monday-projects/actions/workflows/ci.yaml/badge.svg)](https://github.com/jersonmartinez/mcp-monday-projects/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker--first-2496ED.svg?logo=docker&logoColor=white)](Dockerfile)

A high-performance, public, Docker-first [Model Context Protocol](https://modelcontextprotocol.io/) server for managing monday.com. It is written in Go and works with any MCP client that supports stdio transport.

## Current status

The repository is being built incrementally under [Project #12](https://github.com/users/jersonmartinez/projects/12). The first foundation release provides the official Go MCP SDK, validated configuration, a bounded GraphQL transport boundary, Docker Compose, and the `server_info` diagnostic tool.

## Quick start

```bash
cp .env.example .env
# Set MONDAY_API_TOKEN in .env
make validate
make run
```

Only Docker and Docker Compose are required on the host. Go, tests, lint, and runtime execution are containerized through the Makefile.

## Configuration

| Variable | Required | Default | Purpose |
|---|---:|---|---|
| `MONDAY_API_TOKEN` | yes | — | monday.com API token |
| `MONDAY_API_VERSION` | no | `2026-07` | Value sent in the `API-Version` header |
| `MONDAY_API_URL` | no | `https://api.monday.com/v2` | Monday GraphQL endpoint |
| `MCP_HTTP_TIMEOUT` | no | `15s` | Outbound request timeout |
| `MCP_MAX_RESPONSE_BYTES` | no | `4194304` | Response size limit |
| `MCP_MAX_RETRIES` | no | `2` | Bounded retries for transient HTTP failures |

See [docs/SETUP.md](docs/SETUP.md) for token handling and API versioning. The current stable version must be confirmed against Monday's [versioning documentation](https://developer.monday.com/api-reference/docs/api-versioning) before upgrades.

## Architecture

```text
MCP tools → application services → domain models → monday port → GraphQL transport
```

The protocol layer does not build GraphQL queries directly. The Monday adapter owns authentication headers, connection reuse, response limits, and API error normalization.

## Make targets

```text
make help        Show targets
make build       Build the runtime image
make run         Run the stdio MCP server
make test        Run Go tests in Docker
make fmt-check   Verify gofmt in Docker
make vet         Run go vet in Docker
make validate    Run the full local validation path
make down        Stop Compose resources
```

## Collaboration

- Start work from an issue in the public Project.
- Create an epic when work spans more than one issue.
- Every new tool requires tests and documentation in the same PR.
- Use Conventional Commits and open PRs ready for review.
- `AGENTS.md` is the canonical instruction file. This repository intentionally does not use `CLAUDE.md`.

See [CONTRIBUTING.md](CONTRIBUTING.md), [GOVERNANCE.md](GOVERNANCE.md), and [SECURITY.md](SECURITY.md) for project policies.

## License

MIT. See [LICENSE](LICENSE).

## Initial tool catalog

| Tool | Purpose |
|---|---|
| `server_info` | Return safe runtime metadata. |
| `list_workspaces` | Discover visible monday.com workspaces. |
| `list_boards` | Discover visible monday.com boards. |
| `get_board` | Retrieve one board by ID. |

Tools return typed JSON objects and propagate sanitized, actionable errors. More
board, group, column, item, update, and reporting tools are being added through
issues linked to the project epic.
