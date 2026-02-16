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

- `/session new <name>`
- `/session list`
- `/session use <id>`
- `/session add-repo <abs-path> [label]`
- `/session repos`
- `/session repo use <repo-id>`
- `/provider use <codex|cursor>`
- `/provider status`
- `/help`

## Wrapper protocol

Codex is now built in (no wrapper required).  
For wrapper-based providers (such as Cursor), see `/Users/thwoodle/Desktop/open-pilot/internal/providers/wrappers/README.md` for the NDJSON contract.
