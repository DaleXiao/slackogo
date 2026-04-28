# slacko CLI Specification

## Overview

slacko is a CLI tool for interacting with Slack workspaces using browser cookies (`xoxc-` token + `d` cookie), inspired by [spogo](https://github.com/steipete/spogo).

## Authentication

### Cookie-based auth
- `d` cookie from `.slack.com` domain
- `xoxc-` token extracted from Slack web client
- Stored in `~/.config/slacko/credentials.json`

### Credential format
```json
[
  {
    "token": "xoxc-...",
    "cookie": "...",
    "workspace": "myteam"
  }
]
```

## API Communication

All requests use `POST https://<workspace>.slack.com/api/<method>` with:
- `Content-Type: application/x-www-form-urlencoded`
- `Cookie: d=<cookie_value>`
- Form data includes `token=<xoxc_token>`

## Commands

### auth
- `auth import --browser chrome` — Extract `d` cookie from Chrome
- `auth manual --token T --cookie C --workspace W` — Manual credential entry
- `auth status` — List configured credentials

### workspace
- `workspace list` — Show current workspace info via `team.info`

### channel
- `channel list` — List channels via `conversations.list`
- `channel read <channel> [--limit N]` — Read messages via `conversations.history`
- `channel send <channel> <message>` — Send via `chat.postMessage`

### dm
- `dm list` — List IM conversations via `conversations.list` (type=im)
- `dm read <user> [--limit N]` — Open DM then read history
- `dm send <user> <message>` — Open DM then send

### search
- `search <query> [--limit N]` — Search via `search.messages`

### status
- `status` — Show current user and presence via `auth.test` + `users.getPresence`

### user
- `user list [--limit N]` — List via `users.list`
- `user info <user>` — Detail via `users.info`

### canvas (SPEC-050 v5)

Wraps the Slack `canvases.*` web API family. All requests reuse the existing
cookie + `xoxc-` token auth; no new scopes required.

- `canvas list [--channel C123] [--limit N]` — `files.list?types=canvas` (Slack has no `canvases.list`; canvases are surfaced as files of type `canvas` per https://docs.slack.dev/surfaces/canvases/)
- `canvas get <F-id>` — `files.info?file=<F-id>` (Slack has no `canvases.get`; canvas IDs are F-prefix file IDs, e.g. `F0ASWF3SRST`)
- `canvas get <canvas_id> [-o md|json|raw]` — `canvases.get`; `md` returns the
  markdown body, `json`/`raw` return the full API response
- `canvas create --title T [--channel C] [--from-file f|--body "..."]`
  — `canvases.create` (standalone) or `conversations.canvases.create` when
  `--channel` is set
- `canvas edit <canvas_id> --op OP [--section S] [--from-file f|--body "..."]`
  — `canvases.edit`; OP ∈ {insert_at_start, insert_at_end, insert_before,
  insert_after, replace, delete}
- `canvas delete <canvas_id>` — `canvases.delete`
- `canvas access set <canvas_id> --user U... --level read|write`
  — `canvases.access.set`
- `canvas access delete <canvas_id> --user U...` — `canvases.access.delete`

The canvas wrappers live in `internal/api/canvas.go` (a sibling file to
`client.go`, NOT a modification of it). The CLI subtree lives in
`cmd/slackogo/canvas.go`.

## Output Formats

| Format | Flag | Description |
|--------|------|-------------|
| Human | (default) | Colored, readable |
| Plain | `--plain` | Tab-separated |
| JSON | `--json` | Structured JSON |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage/argument error |
| 3 | Authentication failure |
| 4 | Network error |

## Dependencies

- `github.com/alecthomas/kong` — CLI framework
- `github.com/fatih/color` — Terminal colors
