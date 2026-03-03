<div align="center">

# 🔧 slacko

**Slack, but make it terminal.**

Power CLI using web cookies. Read channels, send messages, search, and script with JSON/plain output.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

---

Your already logged-in Slack workspace — the agent can use it directly. No admin permissions, no OAuth app registration, no rate limits.

```
AI Agent (Claude, GPT, etc.)
       │ CLI commands
       ▼
   slacko CLI ──HTTP──▶ Slack Web API
                           │
                     xoxc- token + d cookie
                     (from your browser session)
```

## Why Not a Slack App?

| | Slack App / Bot Token | slacko |
|---|---|---|
| Setup | Register app, get admin approval, configure OAuth | Import cookies from Chrome |
| Permissions | Scoped, limited by admin | Full access — anything you can do in Slack |
| Rate limits | Strict (tier 1-4) | Web client limits (generous) |
| Internal channels | Need explicit permission | If you can see it, slacko can too |
| Enterprise Grid | Complex multi-workspace auth | Just import cookies per workspace |

## Install

### From source (recommended)

```bash
git clone https://github.com/DaleXiao/slacko.git
cd slacko
go build ./cmd/slacko/
```

### Go install

```bash
go install github.com/DaleXiao/slacko/cmd/slacko@latest
```

## Quick Start

### 1. Get your credentials

**Option A: Import from Chrome** (easiest)

```bash
slacko auth import --browser chrome
```

**Option B: Manual setup**

Open Chrome → your Slack workspace → F12:
- **d cookie**: Application → Cookies → `.slack.com` → `d`
- **xoxc- token**: Network → filter `api/` → any request's form data → `token`

```bash
slacko auth manual --token xoxc-YOUR-TOKEN --cookie "YOUR-D-COOKIE" --workspace your-team
```

### 2. Verify

```bash
slacko auth status
```

### 3. Go

```bash
slacko channel list                    # List channels
slacko channel read general --limit 10 # Read messages
slacko channel send general "Hello!"   # Send message
slacko dm send @alice "Hi there"       # Send DM
slacko search "quarterly report"       # Search messages
```

## Commands

### Auth

```bash
slacko auth import --browser chrome     # Import cookies from Chrome
slacko auth manual --token T --cookie C # Set credentials manually
slacko auth status                      # Check auth status
```

### Channels

```bash
slacko channel list                     # List all channels
slacko channel read CHANNEL [--limit N] # Read messages
slacko channel send CHANNEL "message"   # Send a message
```

### Direct Messages

```bash
slacko dm list                          # List DM conversations
slacko dm read USER [--limit N]         # Read DMs with a user
slacko dm send USER "message"           # Send a DM
```

### Search & Info

```bash
slacko search "query" [--limit N]       # Search messages
slacko user list [--limit N]            # List workspace users
slacko user info USER                   # User details
slacko workspace list                   # List workspaces
slacko status                           # Connection status
```

## Output Modes

```bash
slacko channel list                     # Human-readable (colorized)
slacko channel list --plain             # Tab-separated (for scripts)
slacko channel list --json              # Structured JSON (for agents)
```

Respects `NO_COLOR` and `TERM=dumb`.

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | JSON output |
| `--plain` | Tab-separated output |
| `--no-color` | Disable colors |
| `-w, --workspace` | Select workspace |
| `--timeout` | Request timeout (default 10s) |
| `-q` / `-v` / `-d` | Quiet / Verbose / Debug |
| `--version` | Show version |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid usage |
| 3 | Auth failure |
| 4 | Network error |

## How It Works

Slack's web client authenticates with an `xoxc-` token paired with a `d` cookie. slacko reuses these credentials to call the same Web API endpoints that `app.slack.com` uses — no OAuth, no bot tokens, no admin approval.

Credentials are stored locally in `~/.config/slacko/`.

## Security

- Credentials stay on your machine (`~/.config/slacko/`)
- No data sent to third parties
- You're using your own Slack session
- Use responsibly and in accordance with your organization's policies

## Inspired By

[spogo](https://github.com/steipete/spogo) — Spotify power CLI using web cookies. Same philosophy: if the web client can do it, so can the terminal.

## License

MIT
