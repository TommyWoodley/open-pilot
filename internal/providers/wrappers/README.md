# Wrapper Protocol (NDJSON)

`open-pilot` uses wrapper commands for providers (codex, cursor, etc.).

## stdin messages

One JSON object per line:

- `{"type":"prompt","id":"<request-id>","text":"...","repo_path":"/abs/path","session_id":"<session-id>"}`
- `{"type":"interrupt","id":"<request-id>"}`
- `{"type":"shutdown"}`

## stdout events

One JSON object per line:

- `{"type":"ready"}`
- `{"type":"chunk","id":"<request-id>","text":"..."}`
- `{"type":"final","id":"<request-id>","text":"..."}`
- `{"type":"error","id":"<request-id>","message":"..."}`
- `{"type":"status","message":"..."}`

Wrappers must flush output after every line.
