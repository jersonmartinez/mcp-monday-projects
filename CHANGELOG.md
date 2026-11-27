# Changelog

All notable changes are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Bulk creation and table imports:** `bulk_create_items` validates up to 50 rows before any write, detects duplicate batch names, supports group IDs or exact titles, and preserves input order. `import_board_table` imports CSV or Markdown tables through the same validation, dry-run-first safety, bounded concurrency, and row-level outcome contract.
- **Asset visibility:** `list_item_assets` and `list_update_assets` expose file metadata for items, file columns, gallery entries, and update attachments. Expiring provider URLs stay redacted unless `include_urls=true`.
- **Native automation lifecycle control and observability:** `list_automations`, `create_automation`, `manage_automation`, `delete_automation`, `get_automation_statistics`, and `list_automation_runs` expose monday workflow configuration, dry-run-first template/direct creation through the preview workflow API, guarded activation/deactivation/deletion, outcome totals, and execution history where the provider API supports them. Deletion requires full access and confirmation; creation, activation, and deactivation verify board ownership/scope first and never require embedded credential secrets.
- **Deterministic MCP workflows:** `run_board_automation` evaluates declarative rules against a bounded board snapshot and applies validated transitions; `complete_item` and `handoff_item` orchestrate common multi-step completion and ownership-transfer flows. All three are dry-run by default and enforce the normal workspace scope, write allowlists, concurrency limits, and row-level audit output.
- **Execution statistics:** `board_flow_metrics` reports WIP by status/group, bottleneck status, workload, completion, overdue/stale work, throughput, open-age, and cycle-time approximations with explicit proxy/truncation warnings.
- **Guided workflow reviews:** new `automation_health_review` and `flow_metrics_review` MCP prompts orchestrate safe evidence gathering, proxy interpretation, and dry-run-first next actions.

### Contract note

The stable tool schema digest changes intentionally because this feature set adds fourteen tools. Existing tool schemas remain compatible; role profiles now expose workflow writes where appropriate.

## [1.7.0] - 2026-11-26

### 🚀 Features

- **Add Monday OAuth 2.1 PKCE login** by @Jerson-Martinez-JEM0925 in [#82](https://github.com/jersonmartinez/mcp-monday-projects/pull/82).

### 📦 Release data

- **Version:** `v1.7.0` (`minor` bump from `v1.6.0` — new user-facing capability).
- **Compatibility:** ✅ backwards-compatible feature release.
- **Source:** pull request [#82](https://github.com/jersonmartinez/mcp-monday-projects/pull/82); merge commit `eb6079049acc`.
- **Compare:** [`v1.6.0...v1.7.0`](https://github.com/jersonmartinez/mcp-monday-projects/compare/v1.6.0...v1.7.0).
- **Container image:** `ghcr.io/jersonmartinez/mcp-monday-projects:1.7.0` (also `ghcr.io/jersonmartinez/mcp-monday-projects:1.7` and `ghcr.io/jersonmartinez/mcp-monday-projects:latest`).
- **Upgrade:** `docker pull ghcr.io/jersonmartinez/mcp-monday-projects:1.7.0`.

## [1.6.0] - 2026-11-26

### 🚀 Features

- **Automate merge-to-main release pipelines** by @jersonmartinez in [#75](https://github.com/jersonmartinez/mcp-monday-projects/pull/75).

### Added

- **Compact discovery and operational metrics:** new `search_tools` and
  `get_operational_metrics` tools. Metrics are aggregate latency, error, and rate-limit counters
  only; arguments, content, tokens, and raw error messages are never stored.
- **Role-oriented discovery profiles:** `MCP_TOOL_PROFILE=full|read_only|developer|devops|project_manager`
  narrows the visible catalog while `MCP_ACCESS_LEVEL`, workspace scope, and
  write allowlists remain the enforcement boundary.
- **HTTP hardening controls:** bounded request bodies, per-client rate limits,
  optional TLS enforcement, cached bearer validation, security headers, and
  complete HTTP server timeouts for the Streamable HTTP transport.
- **Per-user Monday OAuth 2.1 + PKCE login:** `mcp-monday-oauth` opens the browser, handles the localhost callback, exchanges and refreshes tokens, revokes authorizations, and stores token pairs with restrictive permissions.

### Changed

- **Mutation retry safety:** monday GraphQL mutations are no longer retried
  automatically because a lost response can duplicate creates or repeat
  irreversible operations. Reads keep bounded retries with `Retry-After`,
  exponential backoff, and jitter.
- **Resilience:** repeated temporary upstream failures open a per-credential
  circuit breaker keyed by a SHA-256 fingerprint, never by the raw token.
- **Reproducible builds:** committed `go.mod` now contains the complete module
  graph, Docker builds use `-mod=readonly` instead of mutating dependencies
  with `go mod tidy`, builder/runtime base images are pinned by digest, and
  `.gitattributes` keeps text files LF-terminated on Windows checkouts so
  Docker bind-mounted validation behaves like CI.
- **Supply-chain validation:** GitHub Actions are pinned by commit SHA, CI
  gains govulncheck, Trivy image scanning, and CodeQL, and release images
  include SBOM/provenance attestations.
- **Patched toolchain:** the module and Docker builders move to Go 1.27
  (1.27.4 in the pinned builder digest) because govulncheck/Trivy reported
  reachable high-severity `net/http`, `crypto/tls`, and `crypto/x509` standard
  library vulnerabilities in the previous Go 1.25 runtime.

### Contract note

The stable tool schema digest changes intentionally because the additive
`search_tools` and `get_operational_metrics` tools expand the catalog.
Existing tool schemas are unchanged.

### 📦 Release data

- **Version:** `v1.6.0` (`minor` bump from `v1.5.0` — new user-facing capability).
- **Compatibility:** ✅ backwards-compatible feature release.
- **Source:** pull request [#75](https://github.com/jersonmartinez/mcp-monday-projects/pull/75); merge commit `b9cec480dd2a`.
- **Scope:** includes curated changes that were accumulated under `[Unreleased]` before automation.
- **Compare:** [`v1.0.0...v1.6.0`](https://github.com/jersonmartinez/mcp-monday-projects/compare/v1.0.0...v1.6.0).
  The previous declared version `v1.5.0` has no git tag; the comparison starts at the last published tag `v1.0.0`.
- **Container image:** `ghcr.io/jersonmartinez/mcp-monday-projects:1.6.0` (also `ghcr.io/jersonmartinez/mcp-monday-projects:1.6` and `ghcr.io/jersonmartinez/mcp-monday-projects:latest`).
- **Upgrade:** `docker pull ghcr.io/jersonmartinez/mcp-monday-projects:1.6.0`.

## [1.0.0] — Stable contract

### Changed

- **Stable v1 contract:** tool names, schemas, access levels, structured errors,
  and documented environment variables are now protected by a schema digest
  regression test. Breaking changes require a major version.
- **Access policy:** removed the ambiguous `MCP_READ_ONLY` compatibility
  variable. Use `MCP_ACCESS_LEVEL=read|write|full` explicitly.
- **Documentation:** clarified that archive retention is controlled by monday,
  and recorded that live smoke validation used an administrator token. Member
  permissions still require a separate member-owned token and have not been
  validated.

- **Program-scale reconciliation tools.** Added bounded text search across visible column values, persisted-value verification, effort progress rollups, dependency read/write helpers with same-board validation, timeline cascade previews, subitem status rollups, bulk update publishing, and bulk people/date setters. Writes report unreadable columns and provider-adjusted date/timeline values as warnings.
- **Subitem safety.** `create_subitem` now rejects a subitem as the parent because monday keeps subitems flat rather than nesting them. `list_subitems` can project requested column values in the same read.
- **Read calculated rollup column values.** Item reads request monday's `CALCULATED` column-value capability, so `get_item`, `get_items`, item lists, searches, and subitem reads include values from rollup-capable number, date, timeline, and status columns. Status rollups remain represented through monday's normal `text`, `value`, and `type` fields.
- **Read dependency, connected-board and mirror columns.** Current monday API versions return `value: null` for dependency and board-relation columns, so their links were invisible to every read tool. Item reads now request the typed `DependencyValue`, `BoardRelationValue` and `MirrorValue` fragments: each column value gains `linked_item_ids` (connected item IDs) and `text` falls back to monday's `display_value`. The stable tool schema digest changes because column values gain the optional `linked_item_ids` field.
- **Column writes validate against the item's real board.** `update_item_column_values`, `set_item_status`, `set_item_date` and the other single-column helpers now resolve the board the item actually lives on before validating and writing. monday accepts a column mutation against the wrong board and silently discards it (a no-op that still returns success); this happened most often with subitems, whose board has a different set of columns than the parent board. An unknown column on the item's board is now rejected with a clear `unknown column ID for this board` error instead of a false success.
- **Attach files to updates and columns.** New `add_file_to_update` and `add_file_to_column` tools upload a file to an update (comment) or to a file-type column, via monday's GraphQL multipart file endpoint. The file contents are passed base64-encoded (`content_base64`), so the server never resolves a client path; uploads are bounded and write-guarded.

## [1.5.0] — Markdown-to-HTML update formatting

- **Monday update formatting:** `create_update` and `reply_to_update` now render Markdown-like input as sanitized HTML, preserving commands, tables, links, backticks and placeholders; `edit_update` replaces an existing body in place without duplicating comments.
- **Item mutation response validation**: item mutations now reject provider responses missing `item_id` instead of returning zero-value items; troubleshooting documents reconciliation before retrying.

## [1.4.0] — Governed HTTP and model-directed tools

### Added

- **Stateless Streamable HTTP transport** with `/mcp`, `/healthz` and
  `/readyz`, while stdio remains the default transport.
- **Per-request bearer credentials** for HTTP requests. Credentials are
  isolated per request, fail closed when absent, and are never copied into
  process-global state or logs.
- **Response-quality and integration tools:** bounded all-item/search views,
  compact item context, due-soon/unassigned/blocked reports, board risk
  scoring, item/board activity summaries, stable integration payloads and
  catalog diagnostics. Results expose `truncated`, `warnings`, timestamps,
  correlation IDs and idempotency keys where applicable.
- **Server instructions and write-tool allowlist:** the MCP initialize result
  tells clients to use Monday tools as the source of truth and surface
  pagination/write safety. `MCP_SERVER_INSTRUCTIONS` replaces that text, while
  `MCP_WRITE_TOOL_ALLOWLIST` narrows exposed non-read tools without affecting
  reads; unknown names fail startup.

### Fixed

- **Unreadable-column warnings:** restricted columns are now reported through
  an optional `warnings` list instead of appearing identical to empty columns.
  This covers `create_item`, `update_item_column_values`, `set_item_status`,
  `set_item_date`, `assign_item_people`, `column_distribution` and
  `board_summary`.

### Notes

- Cursor pagination was audited across item pages, column-value pages, bounded
  reports and workspace-scoped pages. `has_more` and `truncated` continue to
  derive from provider cursors rather than post-filter counts.

## [0.3.0] — Operational parity

### Added

- **Usage and troubleshooting guides (#27):** `docs/USAGE.md` walks through
  the tools by task (connection, discovery, reads, validated writes, bulk
  plan → apply, archive vs. delete, collaboration, reports, templates);
  `docs/TROUBLESHOOTING.md` lists every startup, policy, validation and
  rate-limit message the server emits, captured from the real binary, with
  its fix.
- **Release workflow (#28):** a `vX.Y.Z` tag checks that the tag matches the
  code `Version` and a CHANGELOG section (`scripts/release_notes.sh`), builds
  `linux/amd64` + `linux/arm64`, pushes `ghcr.io/jersonmartinez/mcp-monday-projects`
  (`X.Y.Z`, `X.Y`, `latest`) and publishes the GitHub release. See
  `docs/RELEASING.md`.
- **Repository automation (#26):** `pr-checks.yaml` (Conventional Commits PR
  title, branch name, yamllint, shellcheck, markdownlint, relative doc links),
  `labels-sync.yaml` (labels declared in `.github/labels.yaml`, never deleted)
  and `docs-wiki-sync.yaml` (the Wiki is rebuilt from `docs/`). `make lint`
  and `make wiki-preview` run the same steps locally; `make validate` now
  includes `lint`.
- **Access levels (#23):** `MCP_ACCESS_LEVEL=read|write|full` (default
  `write`) decides which tools are registered. `full` adds seven permanent
  deletes (`delete_item`, `delete_group`, `delete_board`, `delete_column`,
  `delete_update`, `delete_folder`, `delete_workspace`) that require
  `confirm: true` and obey the workspace scope and allowlists.
  `server_info.access_level` reports the level; the smoke suite checks all
  three levels and gains an opt-in `--allow-delete` phase.
- **Workspace scope (#20):** `MONDAY_WORKSPACE_ID` confines every read and
  write to one workspace. An omitted `workspace_id` defaults to it, foreign
  boards and items are refused with a typed `ScopeError`, and
  `server_info.workspace_scope` reports the ID and name. Unscoped servers now
  instruct the client to pick a workspace (or ask the user) before writing.
- **Profiles (#24):** `MCP_PROFILE=<name>` loads `profiles/<name>.env`, a
  pinned target (token reference via `MONDAY_API_TOKEN_ENV`, workspace,
  access level, allowlists) that wins over the environment. Tokens are
  rejected in profile files and unknown keys fail at startup, as in
  mcp-github-projects. `server_info.profile` reports the active profile; the
  image mounts profiles at `/profiles` (`MCP_PROFILES_DIR`).
- **Structured `.env.example`** with sections and read / write / full presets.

- **Coverage gate (#25):** `internal/application` 58.7% → 81.5% and
  `internal/monday` 53.9% → 84.5% with table-driven adapter contract tests
  (exact variables, omitted nulls, reply mapping, not-found and GraphQL error
  paths) and a use-case suite run unscoped, scoped, and allowlisted.
  `scripts/coverage_gate.sh` fails CI and `make validate` below 80%.

### Changed

- **Dependencies (#26):** Go builder image 1.25.0 -> 1.27.1 (the `go.mod`
  minimum stays 1.25), `actions/checkout` v4 -> v7, `gitleaks-action` v2 -> v3.
  Supersedes Dependabot PRs #9, #10 and #11.
- `MCP_READ_ONLY` is a deprecated alias of `MCP_ACCESS_LEVEL=read`.

### Fixed

- **`MCP_LOG_LEVEL` was read but ignored** (logs were always INFO). It is now
  validated (`debug`, `info`, `warn`, `error`; anything else fails at startup)
  and applied to the stderr logger.
- **Docs audit (#27)** against the code: CAPABILITIES said archived objects
  are restorable for 30 days — monday keeps archives with no time limit (the
  30 days apply to deleted objects in monday's Trash), now stated precisely;
  README gains `MCP_LOG_LEVEL`, `MONDAY_API_TOKEN_ENV`, the full list of Make
  targets and the new guides; SETUP describes every workflow (coverage, race,
  PR Checks, labels, Wiki) instead of the original two; SMOKE warns that the
  `--report` file contains account URLs.

## [0.2.0] — Operational MVP

Epic #14. Closes #15, #16, #17, #18.

### Added

- **Board schema, lookup, search, pagination (#15):** `get_board_schema`,
  `get_workspace`, `list_folders`, `list_group_items`, `search_items`,
  `find_items_by_column_values`, `get_item`, `get_items`, `list_subitems`;
  cursor pagination (`cursor`, `has_more`) and filter rules on `list_items`;
  pagination and filters on workspace/board listings.
- **Users, teams, updates, collaboration (#16):** `list_users`,
  `search_users`, `get_user`, `list_teams`, `list_item_updates`,
  `list_board_updates`, `create_update`, `reply_to_update`, `like_update`,
  `notify_user`, `add_board_subscribers`, `list_tags`, `create_or_get_tag`.
- **Typed column-value validation and safe writes (#17):** domain validator
  for 23 writable column types with explicit rejection of computed types;
  `validate_column_values`, `describe_column_formats`, `create_subitem`,
  `set_item_status`, `set_item_date`, `assign_item_people`, `rename_item`,
  `move_item_to_board`, `duplicate_item`, `bulk_update_items`,
  `bulk_move_items`, `bulk_archive_items`; board/group/column/workspace/folder
  management (`create_board`, `update_board`, `archive_board`,
  `duplicate_board`, `create_group`, `update_group`, `duplicate_group`,
  `archive_group`, `create_column`, `update_column`, `create_workspace`,
  `create_folder`).
- **Operational reports and smoke suite (#18):** `board_summary`,
  `column_distribution`, `workload_report`, `overdue_items`, `stale_items`,
  `daily_standup`, `board_health_report`, `export_board_markdown`,
  `export_board_csv`, `workspace_overview`; `scripts/smoke.py` real-account
  suite with read, guard, sandbox, and provision phases.
- Write policy: `MCP_READ_ONLY`, `MONDAY_WRITE_BOARD_ALLOWLIST`,
  `MONDAY_WRITE_WORKSPACE_ALLOWLIST`; `MCP_REPORT_MAX_ITEMS`.
- Board templates (`list_board_templates`, `provision_board_from_template`).
- Diagnostics: `list_tool_catalog`, `get_me`, `get_api_status`; richer
  `server_info` (tool count, write policy).
- MCP prompts `board_health_review` and `standup_digest`; MCP tool
  annotations on every tool.
- Application and domain layers matching the documented architecture.
- Docs: TOOLS, COLUMN_VALUES, CAPABILITIES, REPORTS, SMOKE, ADR 0002,
  SECURITY. Make targets `fmt`, `probe`, `tools`, `smoke`,
  `smoke-provision`, `docs-tools`.

### Fixed

- `list_board_columns` queried `settings_str`, which API `2026-07` no longer
  exposes; columns now read `settings` (object or encoded string).
- `create_item` / `update_item_column_values` sent column values as a JSON
  object; monday requires a JSON-encoded string.
- `users(page: null)` and similar explicit nulls caused monday internal
  errors; nil variables are now omitted.
- GraphQL errors are typed (`RequestError` with monday's code and path) and
  complexity/rate-limit errors are retried within the configured bound.

## [0.1.0] — Foundation

- Go MCP server, validated configuration, bounded GraphQL transport, Docker
  Compose, `server_info`, workspace/board/item foundation tools.
