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

### canvas (SPEC-050 v5 / SPEC-056 v4 / SPEC-058 v3)

Wraps the Slack canvas web APIs. All requests reuse the existing
cookie + `xoxc-` token auth; no new scopes required.

- `canvas list [--channel C123] [--limit N]` — `files.list?types=canvas`
  (Slack does not expose `canvases.list`; the canonical method is
  `files.list` filtered by canvas filetype, per
  https://docs.slack.dev/surfaces/canvases/. Note: `types=canvas` singular).
- `canvas get <canvas_id> [-o md|json|raw]` — `files.info` for metadata, then
  GET `file.url_private_download` with `Authorization: Bearer <xoxc>` and
  `Cookie: d=<d>` to retrieve the canvas body. Slack canvases are stored as
  markdown (per the Slack canvas "Formatting canvas content with the Slack
  API" docs); the download endpoint returns either plain markdown bytes or a
  thin JSON envelope. The CLI sniffs the payload and renders the appropriate
  output for each `-o` mode.
- `canvas create --title T [--channel C] [--from-file f|--body "..."]`
  — `canvases.create` (standalone) or `conversations.canvases.create` when
  `--channel` is set.
- `canvas edit <canvas_id> --op OP [--section S] [--from-file f|--body "..."]`
  — `canvases.edit`; OP ∈ {insert_at_start, insert_at_end, insert_before,
  insert_after, replace, delete}.
- `canvas delete <canvas_id>` — `canvases.delete`.
- `canvas access set <canvas_id> --user U... --level read|write`
  — `canvases.access.set`.
- `canvas access delete <canvas_id> --user U...` — `canvases.access.delete`.

`canvas_id` must be an F-prefix file ID (e.g. `F0ASWF3SRST`); the wrappers
reject empty or Q-prefix legacy Quip IDs client-side via `validateCanvasID`.

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
