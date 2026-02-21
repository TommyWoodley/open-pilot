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
  - `repo.added` (runs on `/session add-repo`)
  - `provider.codex.selected` (runs on `/provider use codex` and on `/session new` when codex is default)
- `install-builtin-skills-on-codex-select` pulls superpowers skills from GitHub (`TommyWoodley/pilot-superpowers@main`) and installs them into `~/.codex/skills`.

## Wrapper protocol

Codex is now built in (no wrapper required).  
For wrapper-based providers (such as Cursor), see `/Users/thwoodle/Desktop/open-pilot/internal/providers/wrappers/README.md` for the NDJSON contract.

## Codex behavior

- Uses built-in Codex adapter (no separate codex wrapper binary).
- Reuses the same Codex thread per open-pilot session/provider/repo context.
- Transcript shows assistant content only; Codex CLI metadata/log noise is suppressed.
