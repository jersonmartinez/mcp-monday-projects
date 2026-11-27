# Capabilities and write policy

## Capability matrix

Each tool declares a capability (visible in `list_tool_catalog` and
[TOOLS.md](TOOLS.md)). monday API tokens are user-scoped: the token can do
whatever its user can do in the UI. Use a dedicated user and the server-side
write policy below to narrow what an MCP client can change.

| Capability | Covers | monday OAuth scope (apps) |
|---|---|---|
| `account.read` | `get_me`, `get_api_status`, `server_info`, catalog | `me:read`, `account:read` |
| `workspaces.read` / `.write` | workspaces and folders | `workspaces:read` / `workspaces:write` |
| `boards.read` / `.write` | boards, templates, subscribers | `boards:read` / `boards:write` |
| `groups.read` / `.write` | groups | `boards:read` / `boards:write` |
| `columns.read` / `.write` | columns, formats | `boards:read` / `boards:write` |
| `items.read` / `.write` | items, subitems, validation, bulk | `boards:read` / `boards:write` |
| `people.read` | users and teams | `users:read`, `teams:read` |
| `collaboration.read` / `.write` | updates, replies, likes, notifications | `updates:read` / `updates:write`, `notifications:write` |
| `tags.read` / `.write` | tags | `tags:read` / `boards:write` |
| `reports.read` | every report and flow metric | `boards:read` |
| `automation.read` / `.write` | native automation visibility/control and deterministic workflow orchestration | `boards:read` / `boards:write`; provider workflow APIs may require current platform access |

## Access levels

`MCP_ACCESS_LEVEL` decides which tools are **registered**. A tool hidden at a
level is invisible to the client; `server_info.access_level` and
`server_info.hidden_write_tools` report the effective surface.

`MCP_READ_ONLY` was removed in v1.0.0. Configurations must use
`MCP_ACCESS_LEVEL=read|write|full`; supplying the removed variable is a startup
error so an old deployment cannot silently select a different policy.

| Level | Tools | Use it for |
|---|---|---|
| `read` | 64 read tools: schema, search, reports, discovery, diagnostics | Analysts, dashboards, audits |
| `write` (default) | + 47 write/archive tools: create, update, move, archive, bulk | Day-to-day work; archived objects can be restored from monday at any time |
| `full` | + 8 permanent deletes (`delete_*`) | Cleanup and maintenance by a trusted operator |

### Tool profiles

`MCP_TOOL_PROFILE` narrows **discovery** for an operator role. It is additive
filtering only: `MCP_ACCESS_LEVEL`, `MONDAY_WORKSPACE_ID`, and the write
allowlists remain the security boundary. The default `full` preserves the
complete access-level catalog.

| Profile | Visible writes | Intended use |
|---|---|---|
| `full` | Every write allowed by `MCP_ACCESS_LEVEL`; permanent deletes still require `full` | Trusted operator/automation |
| `read_only` | None | Analysts, audits, reporting |
| `developer` | Item, bulk, collaboration, tag, and workflow writes | Day-to-day execution on existing boards |
| `project_manager` | Board/group/column structure, item, bulk, collaboration, tag, and workflow writes; no workspace administration | Planning, templates, coordination, reports |
| `devops` | Workspace, board, group, column, item, bulk, collaboration, tag, and workflow writes; no permanent deletes | Provisioning and operational administration |

Read tools remain visible in every non-`read_only` profile so the client can
inspect schema and confirm context before writing. A write hidden by the
profile cannot be re-exposed by `MCP_WRITE_TOOL_ALLOWLIST`; the allowlist only
narrows the already visible non-read catalog.

### Discovery and operational metrics

`search_tools` is the cheap discovery path for large-catalog clients: pass a
keyword and receive at most 25 compact `ToolSpec` rows (name, category,
capability, description, and read/destructive hints) without serializing every
input/output JSON schema.

`get_operational_metrics` returns in-process aggregate counters since server
start: calls, errors, upstream rate-limit/budget failures, average/max latency,
and per-minute rates per tool, plus totals and uptime. It is privacy-safe by
construction — arguments, results, tokens, user content, and raw error messages
are never stored. The registry is bounded by the registered tool catalog. The
counters reset when the container restarts and are operational diagnostics, not
a billing or audit log.

### Automations, workflows, and statistics

Monday exposes two complementary control planes:

1. **Native provider automations.** `list_automations` reads board automation configuration through monday's versioned automation API. `create_automation` exposes monday's preview creation mutation for template-based or expert-authored workflow JSON; it is dry-run-first, bounds definition size, rejects mixed creation modes, and refuses common embedded credential values. `manage_automation` can activate or deactivate a live workflow after proving that the automation belongs to the requested board. Permanent removal is isolated in `delete_automation`, which exists only with `MCP_ACCESS_LEVEL=full`, requires `confirm: true`, and performs the same ownership proof. Legacy automations can be provider read-only, and complex visual editing remains product-controlled because monday does not expose a stable general automation-editing mutation. `get_automation_statistics` and `list_automation_runs` expose success, failure, exhaustion, and execution-history signals when the account has the current workflow platform.
2. **Deterministic MCP workflows.** `run_board_automation`, `complete_item`, and `handoff_item` orchestrate validated item transitions through the normal MCP write boundary. They are server-side workflows rather than stored monday recipes: they are dry-run by default, evaluate bounded item sets, validate every column value before writing, enforce workspace scope and write allowlists, execute with bounded concurrency, and report row-level outcomes.

`board_flow_metrics` adds execution statistics: WIP by status and group, bottleneck status, completion, workload, overdue/stale counts, created/updated throughput, and aging/cycle-time approximations. Completion proxies are explicitly labelled because monday item history is not a complete immutable status-transition event stream.

### Model-facing instructions and write-tool allowlist

The server advertises built-in instructions in the MCP initialize result. They tell clients to use tools as the source of truth, follow pagination metadata, cite returned IDs/URLs, prefer reads, and confirm the requested change before writing. `MCP_SERVER_INSTRUCTIONS` replaces the complete built-in text when non-empty and is limited to 4000 characters; it is not appended to the defaults.

`MCP_WRITE_TOOL_ALLOWLIST` is an optional comma-separated list of exact tool names. When non-empty, only listed tools in the non-read tier are registered; read tools remain registered, and `MCP_ACCESS_LEVEL` still gates writes first (`read` hides all writes and `full` is required for permanent deletes). Empty or unset preserves the current catalog. Unknown names fail startup with the offending name, so typos cannot silently reduce or widen the intended surface.
Permanent deletes (`internal/application/deletes.go`):

- Exist only at `full`, carry `destructiveHint: true`, and are listed in
  their own `deletes` category of the catalog.
- Require `confirm: true`; without it the call is refused locally and the
  error names the recoverable `archive_*` alternative when one exists.
- Pass the same guards as every other mutation: `MONDAY_WORKSPACE_ID`, the
  board/workspace allowlists, and `ErrReadOnly`.
- `delete_update` needs the owning `item_id` and checks that the update
  belongs to it; `delete_folder` checks that the folder is in the workspace;
  `delete_workspace` is refused whenever a scope or allowlist is set; the
  `name` column cannot be deleted.

Archived objects remain recoverable through monday; the retention period is
controlled by monday and is not promised by this server.

`MCP_READ_ONLY=true` was removed in v1.0.0. Use `MCP_ACCESS_LEVEL=read` for a
read-only deployment.

## Profiles

A profile pins one monday target — account token, workspace, access level and
allowlists — in `profiles/<name>.env`, selected with `MCP_PROFILE=<name>`. It
mirrors `GH_PROJECT_PROFILE` in mcp-github-projects.

```bash
cp profiles/example.env profiles/devops.env   # git-ignored
docker run --rm -i --env-file .env \
  -e MCP_PROFILE=devops \
  -v "$PWD/profiles:/profiles:ro" \
  mcp-monday-projects:local
```

| Rule | Behaviour |
|---|---|
| Precedence | Keys set in the profile win over the environment, so a stray variable cannot widen a pinned target; keys the profile omits fall back to the environment and then to defaults. |
| Tokens | Never in the file: `MONDAY_API_TOKEN` is rejected. `MONDAY_API_TOKEN_ENV=MONDAY_TOKEN_DEVOPS` names the variable that holds this account's token; without it `MONDAY_API_TOKEN` is used. |
| Allowed keys | `MONDAY_API_TOKEN_ENV`, `MONDAY_WORKSPACE_ID`, `MCP_ACCESS_LEVEL`, `MCP_TOOL_PROFILE`, `MCP_SERVER_INSTRUCTIONS`, `MCP_WRITE_TOOL_ALLOWLIST`, the two write allowlists, `MCP_REPORT_MAX_ITEMS`, `MCP_BULK_MAX_CONCURRENCY`, `MONDAY_API_VERSION`, `MONDAY_API_URL`, and the runtime limits. Any other key fails at startup. |
| Location | `MCP_PROFILES_DIR` (default `/profiles` in the image, `profiles` otherwise). Names are lowercase `a-z 0-9 - _`. |
| Visibility | `server_info.profile` reports the active profile; the token is never returned. |

Errors name the profile and the offending line, and stop the server before
it accepts any request.

## Workspace scope

`MONDAY_WORKSPACE_ID=<id>` confines the server to one workspace, for **reads
and writes**. It is enforced by a decorator on the application port
(`internal/application/scope.go`), so every tool goes through it.

It takes a numeric ID, not a name: monday does not guarantee unique workspace
names. Find the ID with `list_workspaces` on an unscoped server.

| Situation | Behaviour |
|---|---|
| `workspace_id` omitted | Defaults to the scope (`get_workspace`, `list_folders`, `create_folder`, `create_board`, `duplicate_board`, `provision_board_from_template`, `workspace_overview`). |
| `list_workspaces` | Returns only the scoped workspace. |
| `list_boards` | Filter forced to the scope; a different `workspace_ids` is refused. |
| Board-level tools | The board's workspace is resolved (and cached) first; foreign boards are refused. |
| Item-level tools | The item's board is resolved first; returned item pages are re-checked. |
| `create_workspace`, `notify` with `target_type=Post` | Refused: the target cannot be verified against the scope. |
| Users, teams, tags, `get_me`, `get_api_status` | Account-level; not scoped. |

Refusals are typed (`ScopeError`), name `MONDAY_WORKSPACE_ID`, and happen
before any mutation reaches monday. `server_info.workspace_scope` reports the
ID and resolved name, and the MCP server instructions tell the client which
workspace is in force.

Without a scope the server stays account-wide, and its instructions tell the
client to resolve the workspace from the request, or to call
`list_workspaces` and ask the user before writing.

The scope composes with the access level and the write policy below: they
allowlists still apply inside the scoped workspace.

## Write policy

The policy is enforced in the application layer (`internal/application/guard.go`)
before any GraphQL mutation is built.

| Variable | Effect |
|---|---|
| `MCP_ACCESS_LEVEL=read` | Write tools are **not registered** — clients cannot even see them. `server_info.hidden_write_tools` reports how many were hidden. |
| `MONDAY_WRITE_BOARD_ALLOWLIST=1,2` | Mutations are allowed only on these boards. Item-level writes resolve the item's board first. |
| `MONDAY_WRITE_WORKSPACE_ALLOWLIST=7` | Board/folder creation, duplication, and template provisioning are allowed only in these workspaces. |

When any allowlist is set:

- `create_workspace` is refused.
- Boards created by this server process (`create_board`, `duplicate_board`,
  `provision_board_from_template`) become writable for the rest of the process,
  so a freshly provisioned board can be populated without widening the list.
- Refusals are typed (`GuardError`) and name the variable to change.

`server_info.write_policy` shows the effective policy at runtime.

## Safety properties

- Below `full`, no tool deletes anything. `archive_*` tools archive, and
  monday keeps archived boards, groups and items until someone restores them
  (no time limit); `archive_board` and `archive_group` require
  `confirm: true`.
- At `full`, every `delete_*` tool requires `confirm: true` and is guarded
  like any other mutation. The server offers no way back: per monday's
  documentation, deleted objects sit in the account's Trash for 30 days,
  where only an admin or the user who deleted them can restore them from the
  monday UI, and are then gone for good.
- Bulk tools are capped at 50 rows and default to `dry_run: true`.
- `create_labels_if_missing` is always `false`.
- Every tool advertises MCP annotations (`readOnlyHint`, `destructiveHint`)
  so clients can require confirmation for destructive calls.

## Recommended profiles

| Profile | Settings |
|---|---|
| Analyst / reporting | `MCP_ACCESS_LEVEL=read` |
| Single-team workspace | `MONDAY_WORKSPACE_ID=<team workspace>` |
| Team sandbox | `MONDAY_WORKSPACE_ID=<sandbox workspace>`, `MONDAY_WRITE_WORKSPACE_ALLOWLIST=<sandbox workspace>` and `MONDAY_WRITE_BOARD_ALLOWLIST=<sandbox boards>` |
| Trusted automation | `MCP_ACCESS_LEVEL=write`, dedicated monday user with minimal board access |
| Several accounts or teams | one profile per target, e.g. `MCP_PROFILE=devops` (read) and `MCP_PROFILE=devops-admin` (full) |
| Maintenance / cleanup | `MCP_ACCESS_LEVEL=full` with `MONDAY_WORKSPACE_ID` and a board allowlist |
