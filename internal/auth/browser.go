package auth

import (
	"context"
	"fmt"
	"runtime"
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
// On Windows with Chromium browsers, this uses CDP (Chrome DevTools Protocol)
// to bypass file locks and v20 App-Bound Encryption entirely.
// Edge decrypts cookies internally; we read plaintext via the debug protocol.
// CDP also extracts the xoxc- token from the Slack page JS context.
//
// On macOS, this uses sweetcookie (Keychain-based decryption).
func ImportFromBrowser(browser, browserProfile, workspace string) ([]ImportResult, error) {
	browser = strings.ToLower(browser)

	if _, ok := supportedBrowsers[browser]; !ok {
		names := make([]string, 0, len(supportedBrowsers))
		for k := range supportedBrowsers {
			names = append(names, k)
		}
		return nil, fmt.Errorf("unsupported browser %q. Supported: %s", browser, strings.Join(names, ", "))
	}

	// Windows + Chromium: use CDP as primary path
	if runtime.GOOS == "windows" && isChromiumBrowser(browser) {
		cdpResult, err := CDPImport(workspace)
		if err != nil {
			return nil, fmt.Errorf("CDP import failed: %w", err)
		}
		if cdpResult != nil && cdpResult.Cookie != "" {
			return []ImportResult{{
				Cookie:     cdpResult.Cookie,
				Token:      cdpResult.Token,
				Workspace:  workspace,
				CookieOnly: cdpResult.Token == "",
			}}, nil
		}
		// If CDPImport returned nil (shouldn't happen on Windows), fall through
	}

	// macOS / Linux / non-Chromium: use sweetcookie
	return importViaSweetcookie(browser, workspace)
}

// importViaSweetcookie is the original sweetcookie-based import path.
// Used on macOS (and as fallback on other platforms).
func importViaSweetcookie(browser, workspace string) ([]ImportResult, error) {
	scBrowser := supportedBrowsers[browser]

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

// isChromiumBrowser returns true for browsers that use Chromium's SQLite cookie store.
func isChromiumBrowser(browser string) bool {
	switch browser {
	case "chrome", "edge", "brave", "chromium", "vivaldi", "opera":
		return true
	}
	return false
}
