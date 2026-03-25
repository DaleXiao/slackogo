package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	// On Windows, Chromium browsers hold exclusive locks on the Cookies DB.
	// Try the normal path first; if it fails on Windows, fall back to a
	// shared-mode file copy snapshot.
	res, err := sweetcookie.Get(context.Background(), opts)

	if err != nil && runtime.GOOS == "windows" && isChromiumBrowser(browser) {
		snapshotPath, cleanup, snapErr := snapshotWindowsCookies(scBrowser, browserProfile)
		if snapErr == nil && snapshotPath != "" {
			defer cleanup()
			// Pass the snapshot Cookies file path as profile override
			opts.Profiles = map[sweetcookie.Browser]string{scBrowser: snapshotPath}
			res, err = sweetcookie.Get(context.Background(), opts)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read cookies from %s: %w", browser, err)
	}

	if len(res.Cookies) == 0 {
		return nil, fmt.Errorf("no 'd' cookie found for .slack.com in %s. Make sure you're logged into Slack in that browser", browser)
	}

	cookie := res.Cookies[0].Value

	// On Windows, check if the cookie value is empty (v20 App-Bound Encryption
	// means sweetcookie can read the row but cannot decrypt the value).
	// Fall back to CDP (Chrome DevTools Protocol) to let Edge decrypt it.
	if cookie == "" && runtime.GOOS == "windows" && isChromiumBrowser(browser) {
		cdpValue, cdpErr := cdpExtractCookie("d", "https://app.slack.com/")
		if cdpErr == nil && cdpValue != "" {
			cookie = cdpValue
		} else {
			hint := "v20 App-Bound Encryption detected and CDP fallback failed"
			if cdpErr != nil {
				hint += ": " + cdpErr.Error()
			}
			return nil, fmt.Errorf("%s. Try: slackogo auth manual --cookie <value> %s\n"+
				"  To get the cookie: Edge → F12 → Application → Cookies → https://app.slack.com → copy 'd' value",
				hint, workspace)
		}
	}

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

// snapshotWindowsCookies finds the Cookies DB for the given browser and
// creates a shared-mode copy in a temp directory. Returns the path to the
// copied Cookies file that can be used as sweetcookie's Profile override.
func snapshotWindowsCookies(browser sweetcookie.Browser, profileOverride string) (string, func(), error) {
	// Determine the Cookies DB path
	cookiesPath, err := findChromiumCookiesDB(browser, profileOverride)
	if err != nil {
		return "", nil, err
	}

	return prepareWindowsCookieSnapshot(cookiesPath)
}

// findChromiumCookiesDB locates the Cookies database file for a Chromium browser.
func findChromiumCookiesDB(browser sweetcookie.Browser, profileOverride string) (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")

	var userDataDir string
	switch browser {
	case sweetcookie.BrowserChrome:
		userDataDir = filepath.Join(localAppData, "Google", "Chrome", "User Data")
	case sweetcookie.BrowserEdge:
		userDataDir = filepath.Join(localAppData, "Microsoft", "Edge", "User Data")
	case sweetcookie.BrowserBrave:
		userDataDir = filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data")
	default:
		// Opera, etc
		if appData != "" {
			userDataDir = filepath.Join(appData, "Opera Software", "Opera Stable")
		}
	}

	if userDataDir == "" {
		return "", fmt.Errorf("cannot determine user data dir for %s", browser)
	}

	profile := "Default"
	if profileOverride != "" {
		profile = profileOverride
	}

	// Try Network/Cookies first (newer Chromium), then Cookies
	candidates := []string{
		filepath.Join(userDataDir, profile, "Network", "Cookies"),
		filepath.Join(userDataDir, profile, "Cookies"),
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	return "", fmt.Errorf("cookies DB not found for %s (profile=%s)", browser, profile)
}
