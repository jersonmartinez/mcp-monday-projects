# Contributing

Thanks for contributing to `mcp-monday-projects`.

## Workflow

1. Open or select an issue in [Project #12](https://github.com/users/jersonmartinez/projects/12).
2. Create an epic when a change spans more than one issue.
3. Create a branch using `feat/`, `fix/`, `docs/`, `chore/`, or `test/`.
4. Implement the smallest coherent change.
5. Add tests and documentation in the same PR.
6. Run `make validate`.
7. Open a PR ready for review and include `Closes #N`.

## Development rules

- Go and Docker Compose are the supported development stack.
- Do not run Go commands directly on the host; use Makefile targets.
- Never commit `.env`, API tokens, or real monday.com data.
- Keep MCP stdout protocol-clean; write diagnostics to stderr.
- Use contexts, bounded I/O, typed errors, and explicit validation.
- Do not create `CLAUDE.md`; `AGENTS.md` is canonical.

## Commit style

Use Conventional Commits, for example `feat: add board discovery tool`.

## Review checklist

Reviewers verify behavior, tests, documentation, security, Docker reproducibility,
and that the change remains provider-aware rather than leaking GraphQL details
into MCP handlers.
