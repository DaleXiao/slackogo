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

// ImportFromBrowser extracts the d cookie from a browser's cookie store.
// Uses the sweetcookie library directly — no external CLI needed.
func ImportFromBrowser(browser, browserProfile string) (string, error) {
	browser = strings.ToLower(browser)

	scBrowser, ok := supportedBrowsers[browser]
	if !ok {
		names := make([]string, 0, len(supportedBrowsers))
		for k := range supportedBrowsers {
			names = append(names, k)
		}
		return "", fmt.Errorf("unsupported browser %q. Supported: %s", browser, strings.Join(names, ", "))
	}

	opts := sweetcookie.Options{
		URL:      "https://slack.com/",
		Names:    []string{"d"},
		Browsers: []sweetcookie.Browser{scBrowser},
		Mode:     sweetcookie.ModeFirst,
	}

	res, err := sweetcookie.Get(context.Background(), opts)
	if err != nil {
		return "", fmt.Errorf("failed to read cookies from %s: %w", browser, err)
	}

	if len(res.Cookies) == 0 {
		return "", fmt.Errorf("no 'd' cookie found for .slack.com in %s. Make sure you're logged into Slack in that browser", browser)
	}

	return res.Cookies[0].Value, nil
}
