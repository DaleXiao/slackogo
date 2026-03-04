package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/DaleXiao/slackogo/internal/transport"
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
	// CookieOnly indicates that only the cookie was saved (no token extraction attempted)
	CookieOnly bool
}

// ImportFromBrowser extracts the d cookie and optionally xoxc- tokens.
// If workspace is provided, it targets that specific workspace (for Enterprise Grid).
// Token extraction makes HTTP requests that may trigger security alerts on
// Enterprise Grid — if that's a concern, use --workspace to save cookie-only
// and then provide the token manually.
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

	// If workspace is specified, try to get token from that specific workspace
	var lastErr string

	if workspace != "" {
		// Try multiple URL patterns for enterprise grid compatibility
		urls := []string{
			fmt.Sprintf("https://%s.slack.com/", workspace),
			fmt.Sprintf("https://%s.enterprise.slack.com/", workspace),
		}
		for _, u := range urls {
			result := tryExtractToken(cookie, u, workspace)
			if result != nil && result.Token != "" {
				return []ImportResult{*result}, nil
			}
			if result != nil && result.Error != "" {
				lastErr = result.Error
			}
		}
		// Token extraction failed — save cookie-only
		msg := fmt.Sprintf("cookie saved for %s. Add token manually: slackogo auth manual --token <TOKEN> --cookie '<COOKIE>' %s", workspace, workspace)
		if lastErr != "" {
			msg = fmt.Sprintf("cookie saved for %s. Token auto-extract failed: %s", workspace, lastErr)
		}
		return []ImportResult{{
			Cookie:     cookie,
			Workspace:  workspace,
			CookieOnly: true,
			Error:      msg,
		}}, nil
	}

	// No workspace specified — try the standard signin flow
	result := tryExtractToken(cookie, "https://slack.com/signin", "")
	if result != nil && result.Token != "" {
		return []ImportResult{*result}, nil
	}

	errMsg := "cookie extracted but could not auto-discover workspace. Use --target flag or add credentials manually"
	if result != nil && result.Error != "" {
		errMsg = result.Error
	}

	return []ImportResult{{
		Cookie: cookie,
		Error:  errMsg,
	}}, nil
}

var (
	tokenRegex      = regexp.MustCompile(`"api_token"\s*:\s*"(xoxc-[^"]+)"`)
	teamDomainRegex = regexp.MustCompile(`"team_url"\s*:\s*"https://([^.]+)\.slack\.com/"`)
	teamNameRegex   = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
)

func setEdgeHeaders(req *http.Request, cookie string) {
	req.Header.Set("Cookie", "d="+cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
}

func setNavigationHeaders(req *http.Request) {
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")
}

func tryExtractToken(cookie, pageURL, workspace string) *ImportResult {
	client := &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport.NewEdgeTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			setEdgeHeaders(req, cookie)
			setNavigationHeaders(req)
			return nil
		},
	}

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil
	}
	setEdgeHeaders(req, cookie)
	setNavigationHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return &ImportResult{
			Cookie: cookie,
			Workspace: workspace,
			Error: fmt.Sprintf("HTTP request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ImportResult{
			Cookie: cookie,
			Workspace: workspace,
			Error: fmt.Sprintf("failed to read response: %v", err),
		}
	}

	html := string(body)
	finalURL := resp.Request.URL.String()

	// Extract token
	tokenMatch := tokenRegex.FindStringSubmatch(html)
	if tokenMatch == nil {
		// Return diagnostic info instead of silent nil
		snippet := html
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return &ImportResult{
			Cookie:    cookie,
			Workspace: workspace,
			Error:     fmt.Sprintf("no xoxc- token found at %s (HTTP %d, final URL: %s, body preview: %s...)", pageURL, resp.StatusCode, finalURL, snippet),
		}
	}

	token := tokenMatch[1]

	// Extract workspace domain from final URL or boot data
	if workspace == "" || workspace == "app" {
		// Try from redirect final URL
		finalHost := resp.Request.URL.Hostname()
		if strings.HasSuffix(finalHost, ".slack.com") {
			parts := strings.Split(finalHost, ".")
			if len(parts) >= 3 {
				candidate := strings.Join(parts[:len(parts)-2], ".")
				if candidate != "app" && candidate != "www" {
					workspace = candidate
				}
			}
		}
	}
	if workspace == "" || workspace == "app" {
		if m := teamDomainRegex.FindStringSubmatch(html); m != nil {
			workspace = m[1]
		}
	}

	teamName := ""
	if m := teamNameRegex.FindStringSubmatch(html); m != nil {
		teamName = m[1]
	}

	return &ImportResult{
		Cookie:    cookie,
		Token:     token,
		Workspace: workspace,
		TeamName:  teamName,
	}
}
