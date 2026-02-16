# open-pilot

`open-pilot` is a Go-based TUI for interacting with coding-agent CLIs through provider wrappers.

## Prerequisites

- Go 1.24+
- Provider wrapper commands available on `PATH`:
  - `open-pilot-codex-wrapper`
  - `open-pilot-cursor-wrapper` (or override with your own command)

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

See `/Users/thwoodle/Desktop/open-pilot/internal/providers/wrappers/README.md` for NDJSON contract details.
