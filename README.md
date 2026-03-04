<div align="center">

# 🔧 slackogo

**Slack, but make it terminal.**

Power CLI using web cookies. Read channels, send messages, search, and script with JSON/plain output.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

---

Your already logged-in Slack workspace — the agent can use it directly. No admin permissions, no OAuth app registration, no rate limits.

```
AI Agent (Claude, GPT, etc.)
       │ CLI commands
       ▼
   slackogo CLI ──HTTP──▶ Slack Web API
                              │
                        xoxc- token + d cookie
                        (from your browser session)
```

## Why Not a Slack App?

| | Slack App / Bot Token | slackogo |
|---|---|---|
| Setup | Register app, get admin approval, configure OAuth | Import cookies from Chrome |
| Permissions | Scoped, limited by admin | Full access — anything you can do in Slack |
| Rate limits | Strict (tier 1-4) | Web client limits (generous) |
| Internal channels | Need explicit permission | If you can see it, slackogo can too |
| Enterprise Grid | Complex multi-workspace auth | Just import cookies per workspace |

## Install

### From source

```bash
git clone https://github.com/DaleXiao/slackogo.git
cd slackogo
go build ./cmd/slackogo/
```

### Go install

```bash
go install github.com/DaleXiao/slackogo/cmd/slackogo@latest
```

### Download binary

Grab a pre-built binary from [Releases](https://github.com/DaleXiao/slackogo/releases) (macOS arm64, Windows amd64/arm64).

## Quick Start

### 1. Get your credentials

**Option A: Import from browser** (recommended)

```bash
# Step 1: Start Edge with CDP enabled
# Windows:
msedge.exe --remote-debugging-port=9222
# macOS:
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge --remote-debugging-port=9222

# Step 2: Open your Slack workspace in that browser window

# Step 3: Import (extracts cookie locally + token via CDP — no extra HTTP requests)
slackogo auth import --browser edge -t your-workspace
```

For non-Enterprise workspaces, CDP is optional:
```bash
slackogo auth import --browser chrome
slackogo auth import --browser edge --no-cdp
```

Supported browsers for cookie extraction: Chrome, Edge, Brave, Firefox, Safari.

**Option B: Manual setup**

If automatic import doesn't work, set credentials manually:

Open your browser → Slack workspace → F12:
- **d cookie**: Application → Cookies → `.slack.com` → `d`
- **xoxc- token**: Network → filter `api/` → any request's form data → `token`

```bash
slackogo auth manual --token xoxc-YOUR-TOKEN --cookie "YOUR-D-COOKIE" your-team
```

### 2. Verify

```bash
slackogo auth status
```

### 3. Go

```bash
slackogo channel list                    # List channels
slackogo channel read general --limit 10 # Read messages
slackogo channel send general "Hello!"   # Send message
slackogo dm send @alice "Hi there"       # Send DM
slackogo search "quarterly report"       # Search messages
```

## Commands

### Auth

```bash
slackogo auth import --browser chrome     # Import cookies from Chrome
slackogo auth manual --token T --cookie C # Set credentials manually
slackogo auth status                      # Check auth status
```

### Channels

```bash
slackogo channel list                     # List all channels
slackogo channel read CHANNEL [--limit N] # Read messages
slackogo channel send CHANNEL "message"   # Send a message
```

### Direct Messages

```bash
slackogo dm list                          # List DM conversations
slackogo dm read USER [--limit N]         # Read DMs with a user
slackogo dm send USER "message"           # Send a DM
```

### Search & Info

```bash
slackogo search "query" [--limit N]       # Search messages
slackogo user list [--limit N]            # List workspace users
slackogo user info USER                   # User details
slackogo workspace list                   # List workspaces
slackogo status                           # Connection status
```

## Output Modes

```bash
slackogo channel list                     # Human-readable (colorized)
slackogo channel list --plain             # Tab-separated (for scripts)
slackogo channel list --json              # Structured JSON (for agents)
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

Slack's web client authenticates with an `xoxc-` token paired with a `d` cookie. slackogo reuses these credentials to call the same Web API endpoints that `app.slack.com` uses — no OAuth, no bot tokens, no admin approval.

Credentials are stored locally in `~/.config/slackogo/`.

## Features

- **One-command setup** — cookie extraction + CDP token discovery, zero extra HTTP requests
- **Enterprise Grid safe** — uses CDP to read tokens from your already-open browser tab, never triggers security detection
- **Browser fingerprint** — API requests mimic real Edge browser headers
- **Cookie rotation** — automatically captures and persists Slack's rotated `d` cookie

## Project Structure

```
slackogo/
├── cmd/slackogo/        # CLI entry point
├── internal/            # Go packages (api, auth, output, app)
├── .github/workflows/   # Release automation
├── .gitignore
├── LICENSE
└── README.md
```

## Security

- Credentials stay on your machine (`~/.config/slackogo/`)
- No data sent to third parties
- You're using your own Slack session
- Use responsibly and in accordance with your organization's policies

## Inspired By

[spogo](https://github.com/steipete/spogo) — Spotify power CLI using web cookies. Same philosophy: if the web client can do it, so can the terminal.

## License

MIT
