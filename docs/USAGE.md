# Usage

Task-oriented guide for the monday.com MCP server. Every tool, its mode and
capability is listed in [TOOLS.md](TOOLS.md) (generated from the server);
this page shows how the tools fit together. Examples are the `arguments`
object an MCP client sends in `tools/call`; IDs are placeholders.

## Calling conventions

- **IDs are numeric strings**: `"board_id": "1234567890"`. Non-numeric IDs are
  rejected before any API call.
- **Column values are friendly and validated**: pass `"Done"`, `"2026-10-01"`
  or `["12345678"]` keyed by column ID; the server checks them against the live
  board schema and converts them to monday JSON. Formats per column type:
  [COLUMN_VALUES.md](COLUMN_VALUES.md).
- **Pagination**: list tools take `limit` and `page`; item tools return a
  `cursor` (pass it back to continue) and `has_more`.
- **Errors come back as tool results** with `isError: true` and one
  actionable sentence; see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
- **What you can call depends on configuration**: `MCP_ACCESS_LEVEL`,
  `MONDAY_WORKSPACE_ID`, the write allowlists and `MCP_PROFILE`
  ([CAPABILITIES.md](CAPABILITIES.md)). `server_info` reports the effective
  policy; a tool that is not registered answers `unknown tool`.

## 1. Check the connection and the policy

```json
{"name": "get_me", "arguments": {}}
{"name": "server_info", "arguments": {}}
{"name": "get_api_status", "arguments": {}}
```

`get_me` shows whose token is in use — every action is attributed to that
user in monday. `server_info` shows the access level, the workspace scope
(with its resolved name) and the allowlists. `get_api_status` returns the
remaining per-minute complexity budget.

## 2. Find the workspace, board and columns

```json
{"name": "list_workspaces", "arguments": {"limit": 25}}
{"name": "list_boards", "arguments": {"workspace_ids": ["14216815"], "limit": 25}}
{"name": "get_board_schema", "arguments": {"board_id": "1234567890"}}
```

With `MONDAY_WORKSPACE_ID` set, `list_workspaces` returns only that workspace
and `list_boards` only its boards. `get_board_schema` returns columns with
their status/dropdown labels, groups, owners and the status/date/people
columns the reports will use — read it before writing.

## 3. Read and search items

```json
{"name": "list_items", "arguments": {"board_id": "1234567890", "limit": 50}}
{"name": "search_items", "arguments": {"board_id": "1234567890", "text": "pipeline"}}
{"name": "find_items_by_column_values", "arguments": {"board_id": "1234567890",
  "columns": [{"column_id": "status", "column_values": ["Stuck"]}]}}
{"name": "get_item", "arguments": {"item_id": "9876543210"}}
```

`search_items` also takes a monday `filter` (rules with `column_id`,
`compare_value`, `operator`), see the example in [TOOLS.md](TOOLS.md#examples).
`find_items_by_column_values` matches values exactly.

## 4. Write safely

Validate first — this never mutates:

```json
{"name": "validate_column_values", "arguments": {"board_id": "1234567890",
  "column_values": {"status": "Done", "due_date": "2026-10-01", "owner": ["12345678"]}}}
```

Then write:

```json
{"name": "create_item", "arguments": {"board_id": "1234567890", "group_id": "topics",
  "name": "Rotate API keys", "column_values": {"status": "Working on it", "priority": "High"}}}
{"name": "set_item_status", "arguments": {"board_id": "1234567890", "item_id": "9876543210", "label": "Done"}}
{"name": "set_item_date", "arguments": {"board_id": "1234567890", "item_id": "9876543210", "date": "2026-10-15"}}
{"name": "assign_item_people", "arguments": {"board_id": "1234567890", "item_id": "9876543210", "user_ids": ["12345678"]}}
{"name": "move_item", "arguments": {"item_id": "9876543210", "group_id": "done"}}
```

A label that does not exist is rejected with the list of valid ones. When
`column_id` is omitted, the single-purpose setters auto-detect the column the
same way the reports do (for dates, a due/deadline column is preferred); a
board without such a column asks for `column_id`. `set_item_date` with an
empty `date` clears it, and `assign_item_people` with an empty list clears
the column.

## 5. Bulk changes: plan, then apply

Bulk tools accept at most 50 rows, **default to `dry_run: true`** and write
nothing if any row is invalid:

```json
{"name": "bulk_update_items", "arguments": {"board_id": "1234567890",
  "updates": [{"item_id": "9876543210", "column_values": {"priority": "High"}}]}}
{"name": "bulk_update_items", "arguments": {"board_id": "1234567890", "dry_run": false,
  "updates": [{"item_id": "9876543210", "column_values": {"priority": "High"}}]}}
{"name": "bulk_move_items", "arguments": {"item_ids": ["9876543210", "9876543211"], "group_id": "done", "dry_run": false}}
```

### Create or import many rows

`bulk_create_items` validates the complete batch before the first write. Rows
accept a `group_id` or exact group title and friendly `column_values`; duplicate
names inside the batch are rejected so re-running an import is less likely to
create accidental doubles.

```json
{"name": "bulk_create_items", "arguments": {"board_id": "1234567890",
  "rows": [{"name": "Prepare release", "group_id": "todo",
    "column_values": {"status": "Working on it", "due": "2026-11-30"}}]}}
```

`import_board_table` accepts CSV or Markdown. A `name` column is required;
`group` is optional and every other header resolves as a column ID or exact
column title. The default call is a plan. Apply only after reviewing it:

```json
{"name": "import_board_table", "arguments": {"board_id": "1234567890", "format": "markdown",
  "text": "| name | group | status |\n|---|---|---|\n| Alpha | To Do | Done |",
  "dry_run": false}}
```

## 6. Archive and delete

Archiving is the default destructive verb and is restorable from monday's
archive:

```json
{"name": "archive_item", "arguments": {"item_id": "9876543210"}}
{"name": "archive_board", "arguments": {"board_id": "1234567890", "confirm": true}}
```

Permanent `delete_*` tools exist only at `MCP_ACCESS_LEVEL=full`, require
`confirm: true`, and still respect the workspace scope and allowlists:

```json
{"name": "delete_item", "arguments": {"item_id": "9876543210", "confirm": true}}
```

## 7. Inspect file assets

`list_item_assets` returns file metadata for an item, its file columns and
gallery; `list_update_assets` does the same for one update. Expiring provider
URLs are redacted unless `include_urls` is true. Pass `item_id` with an update
whenever possible so ownership is verified before reading metadata.

```json
{"name": "list_item_assets", "arguments": {"item_id": "9876543210"}}
{"name": "list_update_assets", "arguments": {"update_id": "5555555555", "item_id": "9876543210"}}
```

## 8. Collaborate

```json
{"name": "create_update", "arguments": {"item_id": "9876543210", "body": "Deployed to staging."}}
{"name": "notify_user", "arguments": {"user_id": "12345678", "target_id": "9876543210",
  "target_type": "Project", "text": "Ready for review"}}
```

With `MONDAY_WORKSPACE_ID` set, notify on the item (`target_type: "Project"`);
`Post` targets are refused because an update's workspace cannot be verified.

## 9. Automations, workflows, and flow statistics

Native monday automations are inspected separately from deterministic MCP
workflow execution:

```json
{"name": "list_automations", "arguments": {"board_id": "1234567890"}}
{"name": "get_automation_statistics", "arguments": {"board_id": "1234567890"}}
{"name": "list_automation_runs", "arguments": {"board_id": "1234567890", "states": ["failure"], "offset": 0}}
```

`manage_automation` verifies that the automation belongs to the board before
activating or deactivating it. Permanent removal is isolated in
`delete_automation`, which exists only at `MCP_ACCESS_LEVEL=full`, requires
`confirm: true`, and performs the same ownership proof; prefer `deactivate`
unless removal is intentional.

```json
{"name": "manage_automation", "arguments": {"board_id": "1234567890",
  "automation_id": "777777777", "action": "deactivate"}}
{"name": "delete_automation", "arguments": {"board_id": "1234567890",
  "automation_id": "777777777", "confirm": true}}
```

`create_automation` completes the native lifecycle for operators who have a
template reference or expert-authored workflow JSON. It uses monday's preview
workflow API, so the call is intentionally dry-run-first and the provider remains
the authoritative validator of block definitions. Pass either
`template_reference_id` (plus optional `workflow_variables_values`) or both
`workflow_blocks` and `workflow_variables`; never embed credential secrets in the
JSON. The default call validates the mode, board, size, and input combinations
without creating anything:

```json
{"name": "create_automation", "arguments": {"board_id": "1234567890",
  "title": "Close completed work", "template_reference_id": "777777777",
  "workflow_variables_values": {"status": "Done"}}}
```

Set `dry_run: false` only after reviewing the plan, then verify the result with
`list_automations`. For deterministic one-shot transitions, patterns that do not
map cleanly to provider blocks, or environments where the preview creation API is
unavailable, use `run_board_automation`. Rules are evaluated in order, the first
matching rule wins, and the default call only plans matching rows:

```json
{"name": "run_board_automation", "arguments": {"board_id": "1234567890",
  "rules": [{"name": "finish-ready", "column_id": "status", "operator": "equals",
    "value": "Ready", "set_column_values": {"status": "Done"},
    "update_body": "Closed by reviewed MCP automation."}]}}
```

`complete_item` and `handoff_item` package common multi-step flows. Both are
dry-run by default, validate values against the live schema before writing,
and report exactly which steps were planned or applied:

```json
{"name": "complete_item", "arguments": {"item_id": "9876543210", "done_value": "Done",
  "move_group_id": "done", "update_body": "Completed and verified."}}
{"name": "handoff_item", "arguments": {"item_id": "9876543210", "assignees": [12345678],
  "status_column_id": "status", "status_value": "Working on it",
  "update_body": "Handoff after triage."}}
```

`board_flow_metrics` reports throughput, WIP, bottleneck, workload, aging,
overdue and completion statistics. Completion and cycle-time fields are
explicit proxies derived from current status plus update timestamps, not an
immutable transition-event stream.

```json
{"name": "board_flow_metrics", "arguments": {"board_id": "1234567890", "window_days": 30}}
```

## 10. Reports

Reports detect the status, date and people columns automatically; override
with `status_column_id`, `date_column_id`, `people_column_id`. They load at
most `MCP_REPORT_MAX_ITEMS` items and say so (`truncated`) when the board is
larger. Semantics and the health score: [REPORTS.md](REPORTS.md).

```json
{"name": "board_summary", "arguments": {"board_id": "1234567890"}}
{"name": "workload_report", "arguments": {"board_id": "1234567890"}}
{"name": "overdue_items", "arguments": {"board_id": "1234567890"}}
{"name": "stale_items", "arguments": {"board_id": "1234567890", "days": 14}}
{"name": "daily_standup", "arguments": {"board_id": "1234567890", "hours": 24}}
{"name": "board_health_report", "arguments": {"board_id": "1234567890"}}
{"name": "export_board_markdown", "arguments": {"board_id": "1234567890"}}
```

The prompts `board_health_review`, `standup_digest`, `automation_health_review`,
and `flow_metrics_review` chain these reports and automation signals for an MCP
client ([TOOLS.md](TOOLS.md#prompts)).

## 11. Boards from templates

```json
{"name": "list_board_templates", "arguments": {}}
{"name": "provision_board_from_template", "arguments": {"template": "devops",
  "workspace_id": "14216815", "board_kind": "private",
  "seed_items": [{"group": "In Progress", "name": "CI pipeline", "values": {"status": "Working on it"}}]}}
```

Templates: `devops`, `incident`, `release`, `project`. Provisioning is
additive (nothing is deleted) and validates seed values against the new
board. A board created in the session stays writable for that session even
under a board allowlist.

## 12. Try it from the command line

`scripts/mcp_probe.py` runs the image with your `.env` and calls one tool:

```bash
make probe TOOL=get_me
make probe TOOL=list_boards ARGS='{"limit": 5}'
python3 scripts/mcp_probe.py server_info '{}' -e MCP_ACCESS_LEVEL=read
python3 scripts/mcp_probe.py --list -e MCP_ACCESS_LEVEL=full
```

## 13. Format updates for Monday

`create_update` and `reply_to_update` convert Markdown-like input to sanitized HTML before sending it to Monday. Monday does not render GitHub Markdown in update bodies, so headings, lists, links, tables, inline backticks and fenced commands must pass through this formatter. Raw HTML is escaped so placeholders and technical content remain visible instead of being removed by sanitization.

See [UPDATE_FORMATTING.md](UPDATE_FORMATTING.md) for the supported syntax, security rules and a read-back probe using an existing item.

### Editar un update existente sin duplicarlo

El tool `edit_update` usa la mutation oficial `edit_update(id, body)` de Monday. Recibe `item_id`, `update_id` y el nuevo body Markdown-like; verifica que el update pertenezca al item, aplica el mismo renderer HTML seguro y reemplaza el body en el mismo update.

```json
{"name":"edit_update","arguments":{"item_id":"1234567890","update_id":"9876543210","body":"## Validación actualizada\\n\\n```bash\\nkubectl get pods -A\\n```"}}
```

Use `edit_update` para corregir formato o migrar comentarios existentes. No cree un nuevo `create_update` si la intención es preservar el historial y el enlace del comentario.

## 14. Trazabilidad y visibilidad por proyecto

Prácticas para que un proyecto sea rastreable y fácil de reportar con las herramientas del servidor.

### Vincular un item con un sistema de seguimiento externo

Cuando un item también se rastrea en un sistema externo, publique la URL del registro externo en el item con un
update formateado, de modo que el enlace quede **visible y clicable** (no implícito):

```json
{"name":"create_update","arguments":{"item_id":"<item_id>","body":"🔗 **Enlace externo:** [registro](https://<tracker>/<path>)"}}
```

El body admite Markdown y se renderiza como HTML (ver sección 11). Use `edit_update` para corregir el enlace sin
duplicar el comentario. Escriba el enlace recíproco (la URL del item de Monday) desde el sistema externo, para que
la trazabilidad funcione en ambos sentidos.

### Clasificar por proyecto (visibilidad y reportes)

- Aplique una etiqueta de la columna *Project* (status) con el nombre del proyecto a todos sus items con
  `update_item_column_values` / `change_column_value` (`create_labels_if_missing` crea la etiqueta la primera
  vez). Esto agrupa los tickets del proyecto y habilita reportes por proyecto.
- Use `board_summary`, `workload_report` y `column_distribution` para generar reportes por proyecto (avance,
  carga por asignado, distribución por estado) apoyándose en esa etiqueta y en el campo de estimación.

### Asignar a todos los responsables

- `assign_item_people` acepta **varios** `user_ids`: asigne a **todas** las personas responsables de un item en
  una sola llamada (no reparta un item entre asignados distintos si el trabajo es conjunto).

## 15. Reconcile a large program safely

Use `list_subitems` with `column_ids` to fetch only the subitem cells needed by a synchronization pass. `find_item_by_text` searches item names and visible text/link/long-text cells when an external reference is not in the item name.

For idempotent synchronization, call `verify_item_columns` with an `expected` map after writes. The result separates correct rows, differences, and columns that are unreadable to the token. Date and timeline writes also return warnings when the persisted value differs from the requested value, which can happen when dependencies move dates.

`board_progress_report` groups planned and spent numeric effort by status and optional phase, while `subitems_status_rollup` summarizes several parents at once. Use `get_item_dependencies` before changing a dependency graph, `set_item_dependencies` with `mode` `add`, `remove`, or `replace` to make the intended operation explicit, and `timeline_dependency_preview` to inspect direct date shifts before a write.

For bounded batch operations, use `bulk_create_updates`, `bulk_set_people`, and `bulk_set_dates` with their default dry run first. Each write-capable bulk tool returns per-item outcomes; do not treat a successful batch call as proof that every row persisted.
