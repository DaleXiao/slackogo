package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// Discover workspaces and extract tokens
	results := discoverWorkspaces(cookie)

	// If nothing found at all, still return the cookie so user can use it manually
	if len(results) == 0 {
		return []ImportResult{{
			Cookie: cookie,
			Error:  "cookie extracted but no workspaces discovered. Use 'slackogo auth manual' with this cookie",
		}}, nil
	}

	return results, nil
}

var (
	tokenRegex = regexp.MustCompile(`"api_token"\s*:\s*"(xoxc-[^"]+)"`)
)

type teamsResponse struct {
	OK    bool `json:"ok"`
	Teams []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Domain string `json:"domain"`
		URL    string `json:"url"`
	} `json:"teams"`
	Error string `json:"error,omitempty"`
}

func newHTTPClient(cookie string) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			req.Header.Set("Cookie", "d="+cookie)
			return nil
		},
	}
}

func setEdgeHeaders(req *http.Request, cookie string) {
	req.Header.Set("Cookie", "d="+cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
}

// discoverWorkspaces finds all workspaces the user belongs to and extracts tokens
func discoverWorkspaces(cookie string) []ImportResult {
	client := newHTTPClient(cookie)

	// Step 1: Call auth.teams to discover all workspaces dynamically
	teams, err := fetchTeams(client, cookie)
	if err != nil || len(teams) == 0 {
		// Fallback: try slack.com directly and parse boot data
		result := tryExtractFromPage(client, cookie, "https://slack.com/ssb/redirect", "", "")
		if result != nil {
			return []ImportResult{*result}
		}
		return nil
	}

	// Step 2: For each workspace, load its page to extract the xoxc- token
	var results []ImportResult
	for _, team := range teams {
		pageURL := fmt.Sprintf("https://%s.slack.com/", team.Domain)
		result := tryExtractFromPage(client, cookie, pageURL, team.Domain, team.Name)
		if result != nil {
			results = append(results, *result)
		} else {
			results = append(results, ImportResult{
				Cookie:    cookie,
				Workspace: team.Domain,
				TeamName:  team.Name,
				Error:     fmt.Sprintf("cookie valid but could not extract token for %s", team.Domain),
			})
		}
	}

	return results
}

func fetchTeams(client *http.Client, cookie string) ([]struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	URL    string `json:"url"`
}, error) {
	params := url.Values{}
	// auth.teams doesn't need a token, just the cookie

	req, err := http.NewRequest("POST", "https://slack.com/api/auth.teams", strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setEdgeHeaders(req, cookie)
	req.Header.Set("Origin", "https://slack.com")
	req.Header.Set("Referer", "https://slack.com/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tr teamsResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}
	if !tr.OK {
		return nil, fmt.Errorf("auth.teams failed: %s", tr.Error)
	}

	return tr.Teams, nil
}

func tryExtractFromPage(client *http.Client, cookie, pageURL, workspace, teamName string) *ImportResult {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil
	}
	setEdgeHeaders(req, cookie)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")

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
	tokenMatch := tokenRegex.FindStringSubmatch(html)
	if tokenMatch == nil {
		return nil
	}

	// Try to extract workspace from page if not provided
	if workspace == "" {
		teamDomainRegex := regexp.MustCompile(`"team_url"\s*:\s*"https://([^.]+)\.slack\.com/"`)
		if m := teamDomainRegex.FindStringSubmatch(html); m != nil {
			workspace = m[1]
		}
	}
	if teamName == "" {
		teamNameRegex := regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
		if m := teamNameRegex.FindStringSubmatch(html); m != nil {
			teamName = m[1]
		}
	}

	return &ImportResult{
		Cookie:    cookie,
		Token:     tokenMatch[1],
		Workspace: workspace,
		TeamName:  teamName,
	}
}
