package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/fatih/color"

	"github.com/DaleXiao/slackogo/internal/app"
	"github.com/DaleXiao/slackogo/internal/auth"
	"github.com/DaleXiao/slackogo/internal/output"
)

const version = "0.1.0"

// Exit codes
const (
	ExitOK        = 0
	ExitError     = 1
	ExitUsage     = 2
	ExitAuth      = 3
	ExitNetwork   = 4
)

type Globals struct {
	JSON      bool          `help:"Output as JSON" xor:"format"`
	Plain     bool          `help:"Output as plain tab-separated text" xor:"format"`
	NoColor   bool          `help:"Disable color output"`
	Workspace string        `help:"Select workspace" short:"w"`
	Timeout   time.Duration `help:"Request timeout" default:"10s"`
	Quiet     bool          `help:"Suppress non-essential output" short:"q"`
	Verbose   bool          `help:"Verbose output" short:"v"`
	Debug     bool          `help:"Debug output" short:"d"`
	Version   kong.VersionFlag `help:"Show version"`
}

type CLI struct {
	Globals
	Auth      AuthCmd      `cmd:"" help:"Manage authentication"`
	Workspace WorkspaceCmd `cmd:"" name:"workspace" help:"Workspace operations"`
	Channel   ChannelCmd   `cmd:"" help:"Channel operations"`
	Dm        DmCmd        `cmd:"" name:"dm" help:"Direct message operations"`
	Search    SearchCmd    `cmd:"" help:"Search messages"`
	Status    StatusCmd    `cmd:"" help:"Show current status"`
	User      UserCmd      `cmd:"" help:"User operations"`
}

// === Auth ===

type AuthCmd struct {
	Import AuthImportCmd `cmd:"" help:"Import cookies from browser"`
	Manual AuthManualCmd `cmd:"" help:"Manually set token and cookie"`
	Status AuthStatusCmd `cmd:"" help:"Check authentication status"`
}

type AuthImportCmd struct {
	Browser        string `help:"Browser to import from (chrome,edge,brave,firefox,safari)" default:"chrome" enum:"chrome,edge,brave,firefox,safari"`
	BrowserProfile string `help:"Browser profile name" optional:""`
	Workspace      string `help:"Workspace domain (e.g. myteam) for Enterprise Grid" optional:"" short:"W"`
}

type AuthManualCmd struct {
	Token         string `help:"xoxc- token" required:""`
	Cookie        string `help:"d cookie value" required:""`
	WorkspaceName string `arg:"" help:"Workspace name"`
}

type AuthStatusCmd struct{}

// === Workspace ===

type WorkspaceCmd struct {
	List WorkspaceListCmd `cmd:"" help:"List workspaces"`
}

type WorkspaceListCmd struct{}

// === Channel ===

type ChannelCmd struct {
	List ChannelListCmd `cmd:"" help:"List channels"`
	Read ChannelReadCmd `cmd:"" help:"Read channel messages"`
	Send ChannelSendCmd `cmd:"" help:"Send a message to a channel"`
}

type ChannelListCmd struct{}

type ChannelReadCmd struct {
	Channel string `arg:"" help:"Channel name or ID"`
	Limit   int    `help:"Number of messages" default:"20"`
}

type ChannelSendCmd struct {
	Channel string `arg:"" help:"Channel name or ID"`
	Message string `arg:"" help:"Message text"`
}

// === DM ===

type DmCmd struct {
	List DmListCmd `cmd:"" help:"List DM conversations"`
	Read DmReadCmd `cmd:"" help:"Read DM messages"`
	Send DmSendCmd `cmd:"" help:"Send a DM"`
}

type DmListCmd struct{}

type DmReadCmd struct {
	User  string `arg:"" help:"Username or user ID"`
	Limit int    `help:"Number of messages" default:"20"`
}

type DmSendCmd struct {
	User    string `arg:"" help:"Username or user ID"`
	Message string `arg:"" help:"Message text"`
}

// === Search ===

type SearchCmd struct {
	Query string `arg:"" help:"Search query"`
	Limit int    `help:"Number of results" default:"20"`
}

// === Status ===

type StatusCmd struct{}

// === User ===

type UserCmd struct {
	List UserListCmd `cmd:"" help:"List users"`
	Info UserInfoCmd `cmd:"" help:"Show user info"`
}

type UserListCmd struct {
	Limit int `help:"Number of users" default:"100"`
}

type UserInfoCmd struct {
	User string `arg:"" help:"Username or user ID"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Name("slackogo"),
		kong.Description("Slack CLI tool using browser cookies"),
		kong.Vars{"version": version},
		kong.UsageOnError(),
	)

	if cli.Globals.NoColor {
		color.NoColor = true
	}

	format := output.FormatHuman
	if cli.Globals.JSON {
		format = output.FormatJSON
	} else if cli.Globals.Plain {
		format = output.FormatPlain
	}

	appCtx := &app.Context{
		Printer:   output.NewPrinter(format),
		Workspace: cli.Globals.Workspace,
		Timeout:   cli.Globals.Timeout,
		Verbose:   cli.Globals.Verbose,
		Debug:     cli.Globals.Debug,
		Quiet:     cli.Globals.Quiet,
	}

	var err error
	switch ctx.Command() {
	case "auth import":
		err = runAuthImport(appCtx, &cli.Auth.Import)
	case "auth manual <workspace-name>":
		err = runAuthManual(appCtx, &cli.Auth.Manual)
	case "auth status":
		err = runAuthStatus(appCtx)
	case "workspace list":
		err = runWorkspaceList(appCtx)
	case "channel list":
		err = runChannelList(appCtx)
	case "channel read <channel>":
		err = runChannelRead(appCtx, &cli.Channel.Read)
	case "channel send <channel> <message>":
		err = runChannelSend(appCtx, &cli.Channel.Send)
	case "dm list":
		err = runDmList(appCtx)
	case "dm read <user>":
		err = runDmRead(appCtx, &cli.Dm.Read)
	case "dm send <user> <message>":
		err = runDmSend(appCtx, &cli.Dm.Send)
	case "search <query>":
		err = runSearch(appCtx, &cli.Search)
	case "status":
		err = runStatus(appCtx)
	case "user list":
		err = runUserList(appCtx, &cli.User.List)
	case "user info <user>":
		err = runUserInfo(appCtx, &cli.User.Info)
	default:
		err = fmt.Errorf("unknown command: %s", ctx.Command())
	}

	if err != nil {
		p := appCtx.Printer
		p.Error("Error: %v", err)
		if strings.Contains(err.Error(), "auth error") || strings.Contains(err.Error(), "no credentials") {
			os.Exit(ExitAuth)
		}
		if strings.Contains(err.Error(), "network error") {
			os.Exit(ExitNetwork)
		}
		os.Exit(ExitError)
	}
}

// === Command Implementations ===

func runAuthImport(ctx *app.Context, cmd *AuthImportCmd) error {
	p := ctx.Printer
	p.Human("Importing credentials from %s...", cmd.Browser)
	results, err := auth.ImportFromBrowser(cmd.Browser, cmd.BrowserProfile, cmd.Workspace)
	if err != nil {
		return err
	}

	saved := 0
	for _, r := range results {
		// Always show cookie in verbose mode
		if ctx.Verbose && r.Cookie != "" {
			p.Human("  Cookie: %s", r.Cookie)
		}

		// CookieOnly: save the cookie without token, user adds token manually
		if r.CookieOnly && r.Workspace != "" {
			cred := auth.Credentials{
				Cookie:    r.Cookie,
				Workspace: r.Workspace,
			}
			_ = auth.AddOrUpdateCredentials(cred)
			p.Success("✓ Cookie saved for %s", r.Workspace)
			if r.Error != "" {
				p.Human("  %s", r.Error)
			}
			if !ctx.Verbose && r.Cookie != "" {
				p.Human("  Cookie (first 30 chars): %s...", r.Cookie[:min(30, len(r.Cookie))])
			}
			saved++
			continue
		}

		if r.Error != "" {
			p.Error("  %s", r.Error)
			if !ctx.Verbose && r.Cookie != "" {
				p.Human("  Cookie (first 30 chars): %s...", r.Cookie[:min(30, len(r.Cookie))])
				p.Human("  Use: slackogo auth manual --token <TOKEN> --cookie '<COOKIE>' <WORKSPACE>")
			}
			continue
		}
		if r.Token == "" {
			p.Human("  Cookie found but could not extract token automatically.")
			if !ctx.Verbose && r.Cookie != "" {
				p.Human("  Cookie (first 30 chars): %s...", r.Cookie[:min(30, len(r.Cookie))])
			}
			p.Human("  Use: slackogo auth manual --token <TOKEN> --cookie '<COOKIE>' <WORKSPACE>")
			continue
		}

		if ctx.Verbose {
			p.Human("  Token: %s...%s", r.Token[:15], r.Token[len(r.Token)-4:])
		}

		cred := auth.Credentials{
			Token:     r.Token,
			Cookie:    r.Cookie,
			Workspace: r.Workspace,
		}
		if err := auth.AddOrUpdateCredentials(cred); err != nil {
			p.Error("  Failed to save credentials for %s: %v", r.Workspace, err)
			continue
		}

		name := r.Workspace
		if r.TeamName != "" {
			name = fmt.Sprintf("%s (%s)", r.TeamName, r.Workspace)
		}
		p.Success("✓ Imported: %s", name)
		saved++
	}

	if saved > 0 {
		p.Human("\nVerify with: slackogo auth status")
	}
	return nil
}

func runAuthManual(_ *app.Context, cmd *AuthManualCmd) error {
	if !strings.HasPrefix(cmd.Token, "xoxc-") {
		return fmt.Errorf("token must start with 'xoxc-'")
	}
	cred := auth.Credentials{
		Token:     cmd.Token,
		Cookie:    cmd.Cookie,
		Workspace: cmd.WorkspaceName,
	}
	if err := auth.AddOrUpdateCredentials(cred); err != nil {
		return err
	}
	fmt.Printf("✓ Credentials saved for workspace %q\n", cmd.WorkspaceName)
	return nil
}

func runAuthStatus(ctx *app.Context) error {
	p := ctx.Printer
	creds, err := auth.LoadCredentials()
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		p.Human("No credentials configured.")
		p.Human("Run 'slacko auth manual' or 'slacko auth import' to get started.")
		return nil
	}

	type statusEntry struct {
		Workspace string `json:"workspace"`
		Token     string `json:"token"`
		HasCookie bool   `json:"has_cookie"`
	}
	var entries []statusEntry
	var plainRows [][]string

	for _, c := range creds {
		tokenPreview := c.Token[:min(15, len(c.Token))] + "..."
		entries = append(entries, statusEntry{c.Workspace, tokenPreview, c.Cookie != ""})
		plainRows = append(plainRows, []string{c.Workspace, tokenPreview, fmt.Sprintf("%v", c.Cookie != "")})
	}

	return p.Auto(entries, plainRows, func() {
		p.Header("Configured Workspaces")
		for _, c := range creds {
			tokenPreview := c.Token[:min(15, len(c.Token))] + "..."
			p.Human("  %s: token=%s cookie=%v", c.Workspace, tokenPreview, c.Cookie != "")
		}
	})
}

func runWorkspaceList(ctx *app.Context) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.TeamInfo()
	if err != nil {
		return err
	}

	type wsInfo struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	info := wsInfo{resp.Team.ID, resp.Team.Name, resp.Team.Domain}

	return ctx.Printer.Auto(info, [][]string{{info.ID, info.Name, info.Domain}}, func() {
		ctx.Printer.Header("Workspace")
		ctx.Printer.Human("  %s (%s) — %s.slack.com", info.Name, info.ID, info.Domain)
	})
}

func runChannelList(ctx *app.Context) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.ConversationsList("public_channel,private_channel", 200)
	if err != nil {
		return err
	}

	var plainRows [][]string
	for _, ch := range resp.Channels {
		plainRows = append(plainRows, []string{ch.ID, ch.Name, strconv.Itoa(ch.NumMembers), ch.Topic.Value})
	}

	return ctx.Printer.Auto(resp.Channels, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("Channels (%d)", len(resp.Channels)))
		for _, ch := range resp.Channels {
			prefix := "#"
			if ch.IsPrivate {
				prefix = "🔒"
			}
			ctx.Printer.Human("  %s%-20s  %3d members  %s", prefix, ch.Name, ch.NumMembers, ch.Topic.Value)
		}
	})
}

func runChannelRead(ctx *app.Context, cmd *ChannelReadCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	chID, err := client.ResolveChannelID(cmd.Channel)
	if err != nil {
		return err
	}
	resp, err := client.ConversationsHistory(chID, cmd.Limit)
	if err != nil {
		return err
	}

	var plainRows [][]string
	for _, m := range resp.Messages {
		plainRows = append(plainRows, []string{m.TS, m.User, m.Text})
	}

	return ctx.Printer.Auto(resp.Messages, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("Messages in %s (latest %d)", cmd.Channel, len(resp.Messages)))
		// Print in reverse (oldest first)
		for i := len(resp.Messages) - 1; i >= 0; i-- {
			m := resp.Messages[i]
			user := m.User
			if user == "" {
				user = "bot"
			}
			ctx.Printer.Human("  [%s] %s: %s", formatTS(m.TS), color.CyanString(user), m.Text)
		}
	})
}

func runChannelSend(ctx *app.Context, cmd *ChannelSendCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	chID, err := client.ResolveChannelID(cmd.Channel)
	if err != nil {
		return err
	}
	if err := client.ChatPostMessage(chID, cmd.Message); err != nil {
		return err
	}
	if !ctx.Quiet {
		ctx.Printer.Success("Message sent to %s", cmd.Channel)
	}
	return nil
}

func runDmList(ctx *app.Context) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.ConversationsList("im", 200)
	if err != nil {
		return err
	}

	var plainRows [][]string
	for _, ch := range resp.Channels {
		plainRows = append(plainRows, []string{ch.ID, ch.User})
	}

	return ctx.Printer.Auto(resp.Channels, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("DM Conversations (%d)", len(resp.Channels)))
		for _, ch := range resp.Channels {
			ctx.Printer.Human("  %s → %s", ch.ID, ch.User)
		}
	})
}

func runDmRead(ctx *app.Context, cmd *DmReadCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	userID, err := client.ResolveUserID(cmd.User)
	if err != nil {
		return err
	}
	dmID, err := client.OpenDM(userID)
	if err != nil {
		return err
	}
	resp, err := client.ConversationsHistory(dmID, cmd.Limit)
	if err != nil {
		return err
	}

	var plainRows [][]string
	for _, m := range resp.Messages {
		plainRows = append(plainRows, []string{m.TS, m.User, m.Text})
	}

	return ctx.Printer.Auto(resp.Messages, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("DM with %s (latest %d)", cmd.User, len(resp.Messages)))
		for i := len(resp.Messages) - 1; i >= 0; i-- {
			m := resp.Messages[i]
			ctx.Printer.Human("  [%s] %s: %s", formatTS(m.TS), color.CyanString(m.User), m.Text)
		}
	})
}

func runDmSend(ctx *app.Context, cmd *DmSendCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	userID, err := client.ResolveUserID(cmd.User)
	if err != nil {
		return err
	}
	dmID, err := client.OpenDM(userID)
	if err != nil {
		return err
	}
	if err := client.ChatPostMessage(dmID, cmd.Message); err != nil {
		return err
	}
	if !ctx.Quiet {
		ctx.Printer.Success("DM sent to %s", cmd.User)
	}
	return nil
}

func runSearch(ctx *app.Context, cmd *SearchCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.SearchMessages(cmd.Query, cmd.Limit)
	if err != nil {
		return err
	}

	var plainRows [][]string
	for _, m := range resp.Messages.Matches {
		plainRows = append(plainRows, []string{m.TS, m.Channel.Name, m.Username, m.Text})
	}

	return ctx.Printer.Auto(resp.Messages, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("Search results for %q (%d total)", cmd.Query, resp.Messages.Total))
		for _, m := range resp.Messages.Matches {
			ctx.Printer.Human("  [#%s] %s: %s", color.YellowString(m.Channel.Name), color.CyanString(m.Username), m.Text)
		}
	})
}

func runStatus(ctx *app.Context) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	authResp, err := client.AuthTest()
	if err != nil {
		return err
	}
	presResp, err := client.UsersGetPresence(authResp.UserID)
	if err != nil {
		return err
	}

	type statusInfo struct {
		User     string `json:"user"`
		UserID   string `json:"user_id"`
		Team     string `json:"team"`
		TeamID   string `json:"team_id"`
		Presence string `json:"presence"`
	}
	info := statusInfo{authResp.User, authResp.UserID, authResp.Team, authResp.TeamID, presResp.Presence}

	return ctx.Printer.Auto(info, [][]string{{info.User, info.UserID, info.Team, info.Presence}}, func() {
		ctx.Printer.Header("Status")
		ctx.Printer.Human("  User:      %s (%s)", info.User, info.UserID)
		ctx.Printer.Human("  Team:      %s (%s)", info.Team, info.TeamID)
		presColor := color.GreenString(info.Presence)
		if info.Presence != "active" {
			presColor = color.YellowString(info.Presence)
		}
		ctx.Printer.Human("  Presence:  %s", presColor)
	})
}

func runUserList(ctx *app.Context, cmd *UserListCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.UsersList(cmd.Limit)
	if err != nil {
		return err
	}

	// Filter out deleted and bots for human output
	var plainRows [][]string
	for _, u := range resp.Members {
		plainRows = append(plainRows, []string{u.ID, u.Name, u.RealName, fmt.Sprintf("bot=%v", u.IsBot)})
	}

	return ctx.Printer.Auto(resp.Members, plainRows, func() {
		ctx.Printer.Header(fmt.Sprintf("Users (%d)", len(resp.Members)))
		for _, u := range resp.Members {
			if u.Deleted {
				continue
			}
			status := ""
			if u.IsBot {
				status = " 🤖"
			}
			ctx.Printer.Human("  %-20s %-25s %s%s", u.Name, u.RealName, u.Profile.Title, status)
		}
	})
}

func runUserInfo(ctx *app.Context, cmd *UserInfoCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	userID, err := client.ResolveUserID(cmd.User)
	if err != nil {
		return err
	}
	resp, err := client.UsersInfo(userID)
	if err != nil {
		return err
	}
	u := resp.User

	return ctx.Printer.Auto(u, [][]string{{u.ID, u.Name, u.RealName, u.Profile.Email, u.Profile.Title}}, func() {
		ctx.Printer.Header("User Info")
		ctx.Printer.Human("  ID:      %s", u.ID)
		ctx.Printer.Human("  Name:    %s", u.Name)
		ctx.Printer.Human("  Real:    %s", u.RealName)
		if u.Profile.Title != "" {
			ctx.Printer.Human("  Title:   %s", u.Profile.Title)
		}
		if u.Profile.Email != "" {
			ctx.Printer.Human("  Email:   %s", u.Profile.Email)
		}
		if u.Profile.StatusText != "" {
			ctx.Printer.Human("  Status:  %s %s", u.Profile.StatusEmoji, u.Profile.StatusText)
		}
	})
}

// formatTS converts a Slack timestamp to a readable format
func formatTS(ts string) string {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return ts
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	t := time.Unix(sec, 0)
	return t.Format("15:04")
}
