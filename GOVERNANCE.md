# Governance

## Maintainer model

`jersonmartinez` is the initial maintainer and controls repository settings,
release publication, and collaborator access. Contributors may propose changes
through issues and pull requests. Maintainer access is granted explicitly when
someone is trusted to manage the project board and repository settings.

## Planning

The public [Project #12](https://github.com/users/jersonmartinez/projects/12) is
the source of truth for planned work. Any initiative with more than one issue
must have an epic titled `🏔️ [Epic] ...`; child issues must be linked to it.

## Pull requests

PRs must be ready for review, linked to an issue, documented, tested, and
validated with Docker. Maintainers merge only after CI and review are green.

## Agents

`AGENTS.md` is the canonical instruction set for LLM-based contributors. Agent
changes follow the same issue, PR, test, and documentation requirements as human
changes. No agent-specific bypass exists for security or review.

## Releases

Releases are created by maintainers after the default branch is stable and the
release checklist is complete. Tokens and Monday customer data are never part of
release artifacts.
