package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/sweetcookie"
)

// Supported browsers
var supportedBrowsers = map[string]sweetcookie.Browser{
	"chrome":  sweetcookie.BrowserChrome,
	"edge":    sweetcookie.BrowserEdge,
	"brave":   sweetcookie.BrowserBrave,
	"firefox": sweetcookie.BrowserFirefox,
	"safari":  sweetcookie.BrowserSafari,
}

// ImportResult holds the result of importing credentials for one workspace
type ImportResult struct {
	Cookie    string
	Token     string
	Workspace string
	TeamName  string
	Error     string
	// CookieOnly indicates that only the cookie was saved (no token)
	CookieOnly bool
}

// ImportFromBrowser extracts the d cookie from a browser's cookie store.
//
// This function ONLY reads from the local browser database — it makes
// NO HTTP requests. Enterprise Grid security systems detect and invalidate
// sessions when non-browser TLS clients use the d cookie, so we avoid
// any network activity entirely.
//
// After importing, the user provides the xoxc- token manually:
//
//	slackogo auth import --browser edge -t myworkspace
//	slackogo auth manual --token xoxc-... myworkspace
func ImportFromBrowser(browser, browserProfile, workspace string) ([]ImportResult, error) {
	browser = strings.ToLower(browser)

	scBrowser, ok := supportedBrowsers[browser]
	if !ok {
		names := make([]string, 0, len(supportedBrowsers))
		for k := range supportedBrowsers {
			names = append(names, k)
		}
		return nil, fmt.Errorf("unsupported browser %q. Supported: %s", browser, strings.Join(names, ", "))
	}

	opts := sweetcookie.Options{
		URL:      "https://slack.com/",
		Names:    []string{"d"},
		Browsers: []sweetcookie.Browser{scBrowser},
		Mode:     sweetcookie.ModeFirst,
	}

	res, err := sweetcookie.Get(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookies from %s: %w", browser, err)
	}

	if len(res.Cookies) == 0 {
		return nil, fmt.Errorf("no 'd' cookie found for .slack.com in %s. Make sure you're logged into Slack in that browser", browser)
	}

	cookie := res.Cookies[0].Value

	return []ImportResult{{
		Cookie:     cookie,
		Workspace:  workspace,
		CookieOnly: true,
	}}, nil
}
