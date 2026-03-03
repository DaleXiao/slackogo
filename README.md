# slacko

Slack CLI tool using browser cookies — no Slack App installation required.

像 [spogo](https://github.com/steipete/spogo) 用浏览器 cookie 绕过 Spotify 限制一样，slacko 用浏览器 cookie（`xoxc-` token + `d` cookie）调用 Slack 内部 Web API。

## Install / 安装

```bash
go install github.com/openclaw/slacko/cmd/slacko@latest
```

Or build from source:

```bash
git clone https://github.com/openclaw/slacko.git
cd slacko
go build ./cmd/slacko/
```

## Quick Start / 快速开始

### 1. Get credentials / 获取凭证

Open Chrome → navigate to your Slack workspace → F12 → Application → Cookies → find `d` cookie for `.slack.com`.

For the `xoxc-` token: F12 → Network → filter `api/` → check any request's form data for `token`.

在 Chrome 中打开 Slack 工作区 → F12 → Application → Cookies → 找到 `.slack.com` 的 `d` cookie。
Token：F12 → Network → 过滤 `api/` → 查看任意请求的 form data 中的 `token`。

### 2. Configure / 配置

```bash
slacko auth manual --token xoxc-YOUR-TOKEN --cookie "YOUR-D-COOKIE" --workspace your-team
```

### 3. Use / 使用

```bash
slacko status                          # Check connection
slacko channel list                    # List channels
slacko channel read general --limit 10 # Read messages
slacko channel send general "Hello!"   # Send message
slacko dm send @alice "Hi there"       # Send DM
slacko search "quarterly report"       # Search
slacko user list                       # List users
```

## Commands / 命令

| Command | Description / 说明 |
|---------|-------------------|
| `auth import --browser chrome` | Import cookies from Chrome / 从 Chrome 导入 cookie |
| `auth manual` | Set token & cookie manually / 手动设置 |
| `auth status` | Check auth status / 检查认证状态 |
| `workspace list` | List workspaces / 列出工作区 |
| `channel list` | List channels / 列出频道 |
| `channel read CHANNEL` | Read messages / 读消息 |
| `channel send CHANNEL MSG` | Send message / 发消息 |
| `dm list` | List DMs / 列出私聊 |
| `dm read USER` | Read DMs / 读私聊 |
| `dm send USER MSG` | Send DM / 发私聊 |
| `search QUERY` | Search messages / 搜索消息 |
| `status` | Show status / 显示状态 |
| `user list` | List users / 列出用户 |
| `user info USER` | User details / 用户详情 |

## Global Flags / 全局选项

| Flag | Description / 说明 |
|------|-------------------|
| `--json` | JSON output / JSON 输出 |
| `--plain` | Tab-separated output / Tab 分隔输出 |
| `--no-color` | Disable colors / 禁用颜色 |
| `-w, --workspace` | Select workspace / 选择工作区 |
| `--timeout` | Request timeout (default 10s) / 超时 |
| `-q` | Quiet mode / 安静模式 |
| `-v` | Verbose / 详细输出 |
| `-d` | Debug / 调试模式 |

## How it works / 原理

Slack's web client uses `xoxc-` tokens with a `d` cookie for authentication. slacko reuses these credentials to call Slack's internal Web API directly, bypassing the need for OAuth apps or admin permissions.

Slack 网页端用 `xoxc-` token 配合 `d` cookie 认证。slacko 复用这些凭证直接调用 Slack 内部 Web API，无需 OAuth 或管理员权限。

## License

MIT
