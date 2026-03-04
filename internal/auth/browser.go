package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

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
}

// ImportFromBrowser extracts the d cookie and xoxc- tokens from a browser's cookie store.
func ImportFromBrowser(browser, browserProfile string) ([]ImportResult, error) {
	browser = strings.ToLower(browser)

	scBrowser, ok := supportedBrowsers[browser]
	if !ok {
		names := make([]string, 0, len(supportedBrowsers))
		for k := range supportedBrowsers {
			names = append(names, k)
		}
		return nil, fmt.Errorf("unsupported browser %q. Supported: %s", browser, strings.Join(names, ", "))
	}

	// Also try to get the lc cookie (last workspace) for context
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

	// Extract tokens — single page load, mimics normal browser navigation
	results := discoverWorkspaces(cookie)

	if len(results) == 0 {
		return []ImportResult{{
			Cookie: cookie,
			Error:  "cookie extracted but no workspaces discovered. Use 'slackogo auth manual' with this cookie",
		}}, nil
	}

	return results, nil
}

var (
	tokenRegex      = regexp.MustCompile(`"api_token"\s*:\s*"(xoxc-[^"]+)"`)
	teamDomainRegex = regexp.MustCompile(`"team_url"\s*:\s*"https://([^.]+)\.slack\.com/"`)
	teamNameRegex   = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	// Match multiple teams in boot data (enterprise grid users have multiple)
	allTokensRegex  = regexp.MustCompile(`"api_token"\s*:\s*"(xoxc-[^"]+)"`)
	allDomainsRegex = regexp.MustCompile(`"domain"\s*:\s*"([^"]+)"`)
)

func setEdgeHeaders(req *http.Request, cookie string) {
	req.Header.Set("Cookie", "d="+cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
}

// discoverWorkspaces uses a single page navigation to slack.com (just like
// opening Slack in a browser tab) to extract workspace info and tokens.
// This avoids calling any API endpoints directly which could trigger
// Enterprise Grid security alerts.
func discoverWorkspaces(cookie string) []ImportResult {
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Carry cookie + headers through redirects like a real browser
			setEdgeHeaders(req, cookie)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
			req.Header.Set("Sec-Fetch-Dest", "document")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Site", "none")
			req.Header.Set("Sec-Fetch-User", "?1")
			return nil
		},
	}

	// Single navigation to slack.com/signin — this is exactly what happens
	// when a user clicks their Slack bookmark. The server redirects to the
	// correct workspace and serves boot data with the token embedded.
	req, err := http.NewRequest("GET", "https://slack.com/signin", nil)
	if err != nil {
		return nil
	}
	setEdgeHeaders(req, cookie)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	html := string(body)

	// The final URL tells us which workspace we landed on
	finalURL := resp.Request.URL.String()

	// Extract token
	tokenMatch := tokenRegex.FindStringSubmatch(html)
	if tokenMatch == nil {
		return nil
	}

	token := tokenMatch[1]

	// Extract workspace domain — try from final URL first, then from boot data
	workspace := ""
	if strings.Contains(finalURL, ".slack.com") {
		// Parse domain from final redirect URL (e.g. https://myteam.slack.com/...)
		parts := strings.Split(resp.Request.URL.Hostname(), ".")
		if len(parts) >= 3 && parts[len(parts)-2] == "slack" {
			workspace = strings.Join(parts[:len(parts)-2], ".")
		}
	}
	if workspace == "" || workspace == "app" {
		if m := teamDomainRegex.FindStringSubmatch(html); m != nil {
			workspace = m[1]
		}
	}

	// Extract team name
	teamName := ""
	if m := teamNameRegex.FindStringSubmatch(html); m != nil {
		teamName = m[1]
	}

	return []ImportResult{{
		Cookie:    cookie,
		Token:     token,
		Workspace: workspace,
		TeamName:  teamName,
	}}
}
