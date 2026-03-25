//go:build windows

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// cdpCookieResult represents a cookie from CDP Network.getCookies
type cdpCookieResult struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// CDPImportResult holds cookie + token extracted via CDP
type CDPImportResult struct {
	Cookie string
	Token  string
}

// CDPImport is the primary Windows auth import path.
// It launches Edge with --remote-debugging-port, restores the user's session,
// then extracts the d cookie via Network.getCookies and the xoxc- token via
// Runtime.evaluate on the Slack page context.
//
// Flow:
// 1. Check if Edge is running → prompt user to close it (or kill)
// 2. Launch Edge --remote-debugging-port=<port> --restore-last-session
// 3. Wait for CDP ready (poll /json/version)
// 4. Find Slack tab in /json/list targets, or navigate to Slack
// 5. Network.getCookies → extract d cookie
// 6. Runtime.evaluate → extract xoxc- token from page JS context
// 7. Return results (Edge continues running for user)
func CDPImport(targetWorkspace string) (*CDPImportResult, error) {
	edgePath := findEdgePath()
	if edgePath == "" {
		return nil, fmt.Errorf("Microsoft Edge not found")
	}

	// Check if Edge is already running
	if isEdgeRunning() {
		return nil, fmt.Errorf("Edge is currently running. Please close Edge first, then retry.\n" +
			"  (slackogo will relaunch Edge with debugging enabled and restore your tabs)")
	}

	// Pick a fixed debug port
	debugPort := "9333"

	// Launch Edge with remote debugging + restore last session
	userDataDir := edgeUserDataDir()
	args := []string{
		"--remote-debugging-port=" + debugPort,
		"--restore-last-session",
		"--no-first-run",
		"--no-default-browser-check",
		"--user-data-dir=" + userDataDir,
	}

	cmd := exec.Command(edgePath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Edge: %w", err)
	}
	// Don't kill Edge on return — let user keep using it
	go func() { _ = cmd.Wait() }()

	// Wait for CDP to become ready
	if err := waitForCDP(debugPort, 15*time.Second); err != nil {
		return nil, fmt.Errorf("CDP not ready: %w", err)
	}

	// Find or navigate to Slack tab
	wsURL, err := findOrCreateSlackTab(debugPort, targetWorkspace)
	if err != nil {
		return nil, fmt.Errorf("find Slack tab: %w", err)
	}

	// Extract cookie
	cookie, err := cdpGetCookie(wsURL, "d", "https://app.slack.com/")
	if err != nil {
		// Also try the workspace-specific URL
		cookie, err = cdpGetCookie(wsURL, "d", fmt.Sprintf("https://%s.slack.com/", targetWorkspace))
		if err != nil {
			return nil, fmt.Errorf("extract cookie: %w", err)
		}
	}

	// Extract token via Runtime.evaluate
	token, _ := cdpEvalToken(wsURL)

	return &CDPImportResult{
		Cookie: cookie,
		Token:  token,
	}, nil
}

func isEdgeRunning() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq msedge.exe", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "msedge.exe")
}

func findEdgePath() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		return p
	}
	return ""
}

func edgeUserDataDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data")
}

func waitForCDP(port string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + port + "/json/version")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %v", timeout)
}

type cdpTarget struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	URL                string `json:"url"`
	Type               string `json:"type"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func findOrCreateSlackTab(port, workspace string) (string, error) {
	targets, err := listTargets(port)
	if err != nil {
		return "", err
	}

	// Look for existing Slack tab
	for _, t := range targets {
		if t.Type == "page" && strings.Contains(t.URL, ".slack.com") {
			if t.WebSocketDebuggerURL != "" {
				return t.WebSocketDebuggerURL, nil
			}
		}
	}

	// No Slack tab found — navigate browser to Slack
	slackURL := "https://app.slack.com/"
	if workspace != "" {
		slackURL = fmt.Sprintf("https://%s.slack.com/", workspace)
	}

	// Open new tab by hitting the /json/new endpoint
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/json/new?%s", port, slackURL))
	if err != nil {
		return "", fmt.Errorf("create new tab: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var newTarget cdpTarget
	if err := json.Unmarshal(body, &newTarget); err != nil {
		return "", fmt.Errorf("parse new tab response: %w", err)
	}

	if newTarget.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("new tab has no WebSocket URL")
	}

	// Wait for page to load
	time.Sleep(3 * time.Second)

	return newTarget.WebSocketDebuggerURL, nil
}

func listTargets(port string) ([]cdpTarget, error) {
	resp, err := http.Get("http://127.0.0.1:" + port + "/json/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var targets []cdpTarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func cdpGetCookie(wsURL, cookieName, targetURL string) (string, error) {
	ws, err := websocket.Dial(wsURL, "", "http://localhost")
	if err != nil {
		return "", fmt.Errorf("websocket dial: %w", err)
	}
	defer ws.Close()

	reqID := 1
	request := map[string]interface{}{
		"id":     reqID,
		"method": "Network.getCookies",
		"params": map[string]interface{}{
			"urls": []string{targetURL},
		},
	}

	reqBytes, _ := json.Marshal(request)
	if _, err := ws.Write(reqBytes); err != nil {
		return "", fmt.Errorf("send getCookies: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for CDP response")
		default:
		}

		var msg []byte
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}

		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Cookies []cdpCookieResult `json:"cookies"`
			} `json:"result"`
		}
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}
		if resp.ID != reqID {
			continue
		}

		for _, c := range resp.Result.Cookies {
			if c.Name == cookieName {
				return c.Value, nil
			}
		}
		return "", fmt.Errorf("cookie %q not found (%d cookies returned)", cookieName, len(resp.Result.Cookies))
	}
}

// cdpEvalToken uses Runtime.evaluate to extract xoxc- token from the Slack page JS context
func cdpEvalToken(wsURL string) (string, error) {
	ws, err := websocket.Dial(wsURL, "", "http://localhost")
	if err != nil {
		return "", fmt.Errorf("websocket dial: %w", err)
	}
	defer ws.Close()

	// JS to extract xoxc- token from page context
	js := `(function(){
		var r=[];
		var s=document.querySelectorAll('script');
		for(var i=0;i<s.length;i++){
			var m=s[i].textContent.match(/xoxc-[a-zA-Z0-9_-]+/g);
			if(m) r.push.apply(r,m);
		}
		if(typeof boot_data!=='undefined'&&boot_data.api_token) r.push(boot_data.api_token);
		if(typeof TS!=='undefined'&&TS.boot_data&&TS.boot_data.api_token) r.push(TS.boot_data.api_token);
		for(var j=0;j<localStorage.length;j++){
			var v=localStorage.getItem(localStorage.key(j));
			var lm=v?v.match(/xoxc-[a-zA-Z0-9_-]+/g):null;
			if(lm) r.push.apply(r,lm);
		}
		return[...new Set(r)].join('|');
	})()`

	reqID := 2
	request := map[string]interface{}{
		"id":     reqID,
		"method": "Runtime.evaluate",
		"params": map[string]interface{}{
			"expression":    js,
			"returnByValue": true,
		},
	}

	reqBytes, _ := json.Marshal(request)
	if _, err := ws.Write(reqBytes); err != nil {
		return "", fmt.Errorf("send evaluate: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout")
		default:
		}

		var msg []byte
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return "", fmt.Errorf("read: %w", err)
		}

		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}
		if resp.ID != reqID {
			continue
		}

		value := resp.Result.Result.Value
		if strings.Contains(value, "xoxc-") {
			return value, nil
		}
		return "", fmt.Errorf("no xoxc- token found in page context")
	}
}
