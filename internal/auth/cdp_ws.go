package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// evaluateInTab connects to a CDP tab via WebSocket and evaluates JS to extract the token.
func evaluateInTab(wsURL string) (string, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return "", fmt.Errorf("CDP WebSocket connect failed: %w", err)
	}
	defer conn.Close()

	// Multiple JS expressions to try — different Slack versions store the token differently
	expressions := []string{
		// Modern Slack client
		`(() => {
			// Try boot_data global
			if (typeof boot_data !== 'undefined' && boot_data.api_token) return boot_data.api_token;
			// Try TS.boot_data
			if (typeof TS !== 'undefined' && TS.boot_data && TS.boot_data.api_token) return TS.boot_data.api_token;
			// Try localStorage
			for (let i = 0; i < localStorage.length; i++) {
				const key = localStorage.key(i);
				const val = localStorage.getItem(key);
				if (val && val.startsWith('xoxc-')) return val;
				try {
					const obj = JSON.parse(val);
					if (obj && obj.token && obj.token.startsWith('xoxc-')) return obj.token;
				} catch(e) {}
			}
			// Scan script tags for embedded token
			const scripts = document.querySelectorAll('script');
			for (const s of scripts) {
				const m = s.textContent.match(/"api_token"\s*:\s*"(xoxc-[^"]+)"/);
				if (m) return m[1];
			}
			return '';
		})()`,
	}

	var mu sync.Mutex
	results := make(chan string, 1)
	errChan := make(chan error, 1)

	// Read responses
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			var resp struct {
				ID     int `json:"id"`
				Result struct {
					Result struct {
						Type  string `json:"type"`
						Value string `json:"value"`
					} `json:"result"`
				} `json:"result"`
			}
			if err := json.Unmarshal(message, &resp); err != nil {
				continue
			}
			if resp.Result.Result.Type == "string" && strings.HasPrefix(resp.Result.Result.Value, "xoxc-") {
				mu.Lock()
				select {
				case results <- resp.Result.Result.Value:
				default:
				}
				mu.Unlock()
				return
			}
		}
	}()

	// Send evaluation requests
	for i, expr := range expressions {
		cmd := map[string]interface{}{
			"id":     i + 1,
			"method": "Runtime.evaluate",
			"params": map[string]interface{}{
				"expression":    expr,
				"returnByValue": true,
			},
		}
		msg, _ := json.Marshal(cmd)
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return "", fmt.Errorf("CDP send failed: %w", err)
		}
	}

	// Wait for result
	select {
	case token := <-results:
		return token, nil
	case err := <-errChan:
		return "", fmt.Errorf("CDP evaluation error: %w", err)
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("CDP timeout — could not find xoxc- token in the Slack page. Make sure the Slack workspace is fully loaded")
	}
}
