package auth

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ImportFromBrowser attempts to extract the d cookie from Chrome's cookie store.
// This is a simplified version - full implementation would use sweetcookie or
// platform-specific cookie extraction.
func ImportFromBrowser(browser string) (string, error) {
	if browser != "chrome" {
		return "", fmt.Errorf("only 'chrome' browser is supported")
	}

	switch runtime.GOOS {
	case "darwin":
		return importChromeMAC()
	case "linux":
		return importChromeLinux()
	default:
		return "", fmt.Errorf("browser cookie import not supported on %s. Use 'slacko auth manual' instead", runtime.GOOS)
	}
}

func importChromeMAC() (string, error) {
	// Try using sweetcookie if available
	path, err := exec.LookPath("sweetcookie")
	if err == nil {
		out, err := exec.Command(path, "get", "--browser", "chrome", "--domain", ".slack.com", "--name", "d").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", fmt.Errorf("cookie import requires sweetcookie. Install: go install github.com/nicois/sweetcookie@latest\nOr use 'slacko auth manual --token TOKEN --cookie COOKIE'")
}

func importChromeLinux() (string, error) {
	return "", fmt.Errorf("automatic Chrome cookie import on Linux is not yet supported.\nUse 'slacko auth manual --token TOKEN --cookie COOKIE' instead.\n\nTo get your cookie:\n1. Open Chrome → slack.com → F12 → Application → Cookies\n2. Find the 'd' cookie for .slack.com domain\n3. Copy its value")
}
