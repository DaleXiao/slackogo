package main

// Canvas subcommand for slackogo.
//
// Per SPEC-050 v5 acceptance #11, this is a NEW file under cmd/slackogo/.
// The only edit to cmd/slackogo/main.go is a single CLI struct field plus
// the dispatch cases in the runtime switch (kong's manual switch pattern,
// matching all existing subcommands).
//
// CLI surface (per spec):
//
//   slackogo canvas list   [--channel C123]
//   slackogo canvas get    <canvas_id> [-o json|md|raw]
//   slackogo canvas create --title "..." [--channel C123] [--from-file body.md]
//   slackogo canvas edit   <canvas_id> --op <op> [--section <id>] [--from-file ...]
//   slackogo canvas delete <canvas_id>
//   slackogo canvas access set <canvas_id> --user U123 [--user U456...] --level read|write

import (
	"fmt"
	"os"
	"strings"

	"github.com/DaleXiao/slackogo/internal/api"
	"github.com/DaleXiao/slackogo/internal/app"
)

// === Kong structs ===

type CanvasCmd struct {
	List   CanvasListCmd   `cmd:"" help:"List canvases"`
	Get    CanvasGetCmd    `cmd:"" help:"Get a canvas"`
	Create CanvasCreateCmd `cmd:"" help:"Create a canvas"`
	Edit   CanvasEditCmd   `cmd:"" help:"Edit a canvas section"`
	Delete CanvasDeleteCmd `cmd:"" help:"Delete a canvas"`
	Access CanvasAccessCmd `cmd:"" help:"Manage canvas access"`
}

type CanvasListCmd struct {
	Channel string `help:"Limit to a channel id"`
	Limit   int    `help:"Max canvases to return" default:"100"`
}

type CanvasGetCmd struct {
	CanvasID string `arg:"" help:"Canvas ID"`
	Format   string `name:"output" short:"o" help:"Output: json|md|raw" default:"md" enum:"json,md,raw"`
}

type CanvasCreateCmd struct {
	Title    string `help:"Canvas title" required:""`
	Channel  string `help:"Optional channel id (creates channel canvas)"`
	FromFile string `name:"from-file" help:"Markdown body file (use - for stdin)"`
	Body     string `help:"Inline markdown body (alternative to --from-file)"`
}

type CanvasEditCmd struct {
	CanvasID string `arg:"" help:"Canvas ID"`
	Op       string `help:"Edit op: insert_at_start|insert_at_end|insert_before|insert_after|replace|delete" required:"" enum:"insert_at_start,insert_at_end,insert_before,insert_after,replace,delete"`
	Section  string `help:"Section ID (required for non-insert_at_* ops)"`
	FromFile string `name:"from-file" help:"Markdown body file (use - for stdin)"`
	Body     string `help:"Inline markdown body"`
}

type CanvasDeleteCmd struct {
	CanvasID string `arg:"" help:"Canvas ID"`
}

type CanvasAccessCmd struct {
	Set    CanvasAccessSetCmd    `cmd:"" help:"Grant users canvas access"`
	Delete CanvasAccessDeleteCmd `cmd:"" help:"Revoke users canvas access"`
}

type CanvasAccessSetCmd struct {
	CanvasID string   `arg:"" help:"Canvas ID"`
	User     []string `help:"User ID (repeatable)" required:""`
	Level    string   `help:"Access level: read|write" required:"" enum:"read,write"`
}

type CanvasAccessDeleteCmd struct {
	CanvasID string   `arg:"" help:"Canvas ID"`
	User     []string `help:"User ID (repeatable)" required:""`
}

// === Run handlers ===

func runCanvasList(ctx *app.Context, cmd *CanvasListCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.CanvasesList(cmd.Channel, cmd.Limit)
	if err != nil {
		return err
	}

	var rows [][]string
	for _, cv := range resp.Canvases {
		rows = append(rows, []string{cv.ID, cv.Title, cv.OwnerID, cv.URL})
	}
	return ctx.Printer.Auto(resp.Canvases, rows, func() {
		ctx.Printer.Header(fmt.Sprintf("Canvases (%d)", len(resp.Canvases)))
		for _, cv := range resp.Canvases {
			title := cv.Title
			if title == "" {
				title = "(untitled)"
			}
			ctx.Printer.Human("  %s  %s  %s", cv.ID, title, cv.URL)
		}
	})
}

func runCanvasGet(ctx *app.Context, cmd *CanvasGetCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}

	// Slack API "format" values: empty (default), "markdown", "raw".
	// CLI exposes md/json/raw — md|raw map to API format, json prints raw API response.
	apiFormat := ""
	switch cmd.Format {
	case "md":
		apiFormat = "markdown"
	case "raw":
		apiFormat = "raw"
	case "json":
		apiFormat = "" // default, return full canvas object
	}

	resp, err := client.CanvasesGet(cmd.CanvasID, apiFormat)
	if err != nil {
		return err
	}

	switch cmd.Format {
	case "json":
		// Print the entire raw API response untouched.
		fmt.Println(string(resp.Raw))
		return nil
	case "md":
		if resp.Markdown != "" {
			fmt.Println(resp.Markdown)
		} else {
			ctx.Printer.Human("(no markdown body returned)")
		}
		return nil
	case "raw":
		fmt.Println(string(resp.Raw))
		return nil
	}
	return nil
}

func runCanvasCreate(ctx *app.Context, cmd *CanvasCreateCmd) error {
	body, err := readBody(cmd.Body, cmd.FromFile)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	resp, err := client.CanvasesCreate(cmd.Title, body, cmd.Channel)
	if err != nil {
		return err
	}

	return ctx.Printer.Auto(resp, [][]string{{resp.CanvasID}}, func() {
		ctx.Printer.Success("canvas created: %s", resp.CanvasID)
	})
}

func runCanvasEdit(ctx *app.Context, cmd *CanvasEditCmd) error {
	change := api.CanvasChange{Operation: cmd.Op, SectionID: cmd.Section}

	if cmd.Op != "delete" {
		body, err := readBody(cmd.Body, cmd.FromFile)
		if err != nil {
			return err
		}
		if body == "" {
			return fmt.Errorf("canvas edit op %q requires --body or --from-file", cmd.Op)
		}
		change.DocumentContent = &api.CanvasDocumentContent{Type: "markdown", Markdown: body}
	}

	// Validate section requirement.
	switch cmd.Op {
	case "insert_at_start", "insert_at_end":
		// section optional / unused
	default:
		if cmd.Section == "" {
			return fmt.Errorf("canvas edit op %q requires --section", cmd.Op)
		}
	}

	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	if err := client.CanvasesEdit(cmd.CanvasID, []api.CanvasChange{change}); err != nil {
		return err
	}
	ctx.Printer.Success("canvas %s edited (%s)", cmd.CanvasID, cmd.Op)
	return nil
}

func runCanvasDelete(ctx *app.Context, cmd *CanvasDeleteCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	if err := client.CanvasesDelete(cmd.CanvasID); err != nil {
		return err
	}
	ctx.Printer.Success("canvas %s deleted", cmd.CanvasID)
	return nil
}

func runCanvasAccessSet(ctx *app.Context, cmd *CanvasAccessSetCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	if err := client.CanvasesAccessSet(cmd.CanvasID, cmd.User, cmd.Level); err != nil {
		return err
	}
	ctx.Printer.Success("granted %s access on %s to %d user(s)", cmd.Level, cmd.CanvasID, len(cmd.User))
	return nil
}

func runCanvasAccessDelete(ctx *app.Context, cmd *CanvasAccessDeleteCmd) error {
	client, err := ctx.NewClient()
	if err != nil {
		return err
	}
	if err := client.CanvasesAccessDelete(cmd.CanvasID, cmd.User); err != nil {
		return err
	}
	ctx.Printer.Success("revoked access on %s from %d user(s)", cmd.CanvasID, len(cmd.User))
	return nil
}

// === Helpers ===

// readBody returns markdown body content from --body or --from-file.
// If both are empty, returns "" without error (caller decides whether that's allowed).
// If --from-file is "-", reads from stdin.
func readBody(inline, file string) (string, error) {
	if inline != "" && file != "" {
		return "", fmt.Errorf("--body and --from-file are mutually exclusive")
	}
	if inline != "" {
		return inline, nil
	}
	if file == "" {
		return "", nil
	}
	if file == "-" {
		data, err := readAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// readAll is io.ReadAll without pulling another import in if not present.
func readAll(r interface {
	Read(p []byte) (int, error)
}) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
