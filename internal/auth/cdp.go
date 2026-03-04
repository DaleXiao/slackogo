package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultCDPPort = 9222

// CDPTab represents a browser tab from CDP /json/list
type CDPTab struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	URL                string `json:"url"`
	Type               string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// ExtractTokenViaCDP connects to Edge/Chrome's CDP remote debugging port,
// finds an open Slack tab, and extracts the xoxc- token via JS evaluation.
// This makes ZERO additional HTTP requests to Slack — it reads from the
// already-loaded page in the browser.
func ExtractTokenViaCDP(port int) (token string, workspace string, err error) {
	if port == 0 {
		port = defaultCDPPort
	}

	// Step 1: List all tabs via CDP HTTP endpoint (localhost only)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if err != nil {
		return "", "", fmt.Errorf("cannot connect to CDP on port %d. Start Edge with:\n"+
			"  Windows: msedge.exe --remote-debugging-port=%d\n"+
			"  macOS:   /Applications/Microsoft\\ Edge.app/Contents/MacOS/Microsoft\\ Edge --remote-debugging-port=%d\n"+
			"Then open your Slack workspace in that browser window", port, port, port)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read CDP response: %w", err)
	}

	var tabs []CDPTab
	if err := json.Unmarshal(body, &tabs); err != nil {
		return "", "", fmt.Errorf("failed to parse CDP tabs: %w", err)
	}

	// Step 2: Find a Slack tab
	var slackTab *CDPTab
	for i := range tabs {
		if tabs[i].Type == "page" && strings.Contains(tabs[i].URL, ".slack.com") {
			slackTab = &tabs[i]
			break
		}
	}
	if slackTab == nil {
		return "", "", fmt.Errorf("no Slack tab found in browser. Open your Slack workspace in the browser with CDP enabled")
	}

	// Extract workspace from tab URL
	workspace = extractWorkspaceFromURL(slackTab.URL)

	// Step 3: Connect via WebSocket and evaluate JS
	if slackTab.WebSocketDebuggerURL == "" {
		return "", "", fmt.Errorf("CDP WebSocket URL not available for Slack tab. Try closing and reopening the tab")
	}

	token, err = evaluateInTab(slackTab.WebSocketDebuggerURL)
	if err != nil {
		return "", "", err
	}

	return token, workspace, nil
}

func extractWorkspaceFromURL(u string) string {
	// https://myteam.slack.com/... → myteam
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	parts := strings.Split(u, ".")
	if len(parts) >= 3 && parts[len(parts)-2] == "slack" {
		candidate := strings.Join(parts[:len(parts)-2], ".")
		if candidate != "app" && candidate != "www" {
			return candidate
		}
	}
	return ""
}
