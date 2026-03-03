package app

import (
	"fmt"
	"time"

	"github.com/openclaw/slacko/internal/api"
	"github.com/openclaw/slacko/internal/auth"
	"github.com/openclaw/slacko/internal/output"
)

type Context struct {
	Printer   *output.Printer
	Workspace string
	Timeout   time.Duration
	Verbose   bool
	Debug     bool
	Quiet     bool
}

func (ctx *Context) NewClient() (*api.Client, error) {
	creds, err := auth.FindCredentials(ctx.Workspace)
	if err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}
	return api.NewClient(creds, ctx.Timeout), nil
}
