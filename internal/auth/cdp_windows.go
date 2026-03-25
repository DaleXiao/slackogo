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

// cdpExtractCookie launches a headless Edge instance with remote debugging,
// connects via CDP, and extracts the named cookie for the given URL.
// This bypasses v20 App-Bound Encryption entirely — Edge decrypts cookies
// internally, and we read the plaintext via the debugging protocol.
func cdpExtractCookie(cookieName, targetURL string) (string, error) {
	edgePath := findEdgePath()
	if edgePath == "" {
		return "", fmt.Errorf("Microsoft Edge not found")
	}

	// Create a temporary user data dir to avoid conflicts with running Edge
	tmpDir, err := os.MkdirTemp("", "slackogo-cdp-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Find the real user data dir to copy profile from
	userDataDir := edgeUserDataDir()

	// Launch Edge headless with remote debugging
	debugPort := "0" // let OS pick a free port
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-port=" + debugPort,
		"--user-data-dir=" + userDataDir,
		"--profile-directory=Default",
		targetURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, edgePath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start Edge: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Read the debug port from DevToolsActivePort file
	port, err := waitForDebugPort(userDataDir, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait for debug port: %w", err)
	}

	// Get WebSocket URL from /json/version
	wsURL, err := getDebugWSURL(port)
	if err != nil {
		return "", fmt.Errorf("get debug WS URL: %w", err)
	}

	// Connect and extract cookies
	value, err := cdpGetCookie(wsURL, cookieName, targetURL)
	if err != nil {
		return "", fmt.Errorf("CDP getCookies: %w", err)
	}

	return value, nil
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
	// Try PATH
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		return p
	}
	return ""
}

func edgeUserDataDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data")
}

func waitForDebugPort(userDataDir string, timeout time.Duration) (string, error) {
	portFile := filepath.Join(userDataDir, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(portFile)
		if err == nil && len(data) > 0 {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 1 && lines[0] != "" {
				return strings.TrimSpace(lines[0]), nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for DevToolsActivePort")
}

func getDebugWSURL(port string) (string, error) {
	resp, err := http.Get("http://127.0.0.1:" + port + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", err
	}
	if info.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("no webSocketDebuggerUrl in response")
	}
	return info.WebSocketDebuggerURL, nil
}

func cdpGetCookie(wsURL, cookieName, targetURL string) (string, error) {
	ws, err := websocket.Dial(wsURL, "", "http://localhost")
	if err != nil {
		return "", fmt.Errorf("websocket dial: %w", err)
	}
	defer ws.Close()

	// Send Network.getCookies
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

	// Read response
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
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
		return "", fmt.Errorf("cookie %q not found in CDP response (%d cookies returned)", cookieName, len(resp.Result.Cookies))
	}
	return "", fmt.Errorf("timeout waiting for CDP response")
}
