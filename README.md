# open-pilot

`open-pilot` is a Go-based TUI for interacting with coding-agent CLIs.

## Prerequisites

- Go 1.24+
- `codex` CLI available on `PATH` and authenticated (`codex login`)
- Optional: provider wrapper commands for non-Codex providers (for example `open-pilot-cursor-wrapper`)

## Run

```bash
go run ./cmd/open-pilot
```

## Build

```bash
go build ./cmd/open-pilot
```

## Local CI checks

Run the same checks used in CI:

```bash
go test ./...
go vet ./...
```

## GitHub Actions CI

CI runs on pushes to `master` and pull requests targeting `master`.
Checks executed:
- `go test ./...`
- `go vet ./...`

Linting can be added later after current lint debt is cleaned up.

## Slash commands

- `/session new <name>` (auto-sets provider to `codex` and pre-fills repo setup)
- `/session list`
- `/session use <id>`
- `/session add-repo [path] [label]` (empty path defaults to current working directory)
- `/session repos`
- `/session repo use <repo-id>`
- `/provider use <codex|cursor>`
- `/provider status`
- `/hooks run` (rerun startup hooks for active session)
- `/help`

## Built-in hooks (MVP)

- Hook files are auto-discovered from `hooks/builtin/*.yaml`.
- One YAML file defines one hook.
- Supported triggers right now:
  - `session.started` (runs on `/session new` and can be rerun with `/hooks run`)
  - `repo.selected` (runs on `/session add-repo`, `/session repo use`, and `/session use` when an active repo exists)
  - `provider.codex.selected` (runs on `/provider use codex` and on `/session new` when codex is default)
  - `development.work.complete` (runs each time assistant output includes `[DEVELOPMENT_WORK_COMPLETE]`, `[<DEVELOPMENT_WORK_COMPLETE>]`, or `<DEVELOPMENT_WORK_COMPLETE>`)
    - Also triggers automatic Codex review cycles in chat engine: review current branch changes vs base (`origin/main` fallback `origin/master`), ask Codex to address review comments, and repeat up to 5 cycles until approved/no comments.
    - Automatic review publishes state headings as system messages (for example `Automatic Review`, `Cycle: N/5`, and `State: ...`).
- `install-builtin-skills-on-codex-select` pulls superpowers skills from GitHub (`TommyWoodley/pilot-superpowers@main`) and installs them into `~/.codex/skills`.

## Wrapper protocol

Codex is now built in (no wrapper required).  
For wrapper-based providers (such as Cursor), see `/Users/thwoodle/Desktop/open-pilot/internal/providers/wrappers/README.md` for the NDJSON contract.

## Codex behavior

- Uses built-in Codex adapter (no separate codex wrapper binary).
- Reuses the same Codex thread per open-pilot session/provider/repo context.
- Transcript shows assistant content only; Codex CLI metadata/log noise is suppressed.
