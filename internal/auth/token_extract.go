package auth

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// JS expression to extract xoxc- token from Slack page
const extractTokenJS = `
(function() {
	if (typeof boot_data !== 'undefined' && boot_data.api_token) return boot_data.api_token;
	if (typeof TS !== 'undefined' && TS.boot_data && TS.boot_data.api_token) return TS.boot_data.api_token;
	var scripts = document.querySelectorAll('script');
	for (var i = 0; i < scripts.length; i++) {
		var m = scripts[i].textContent.match(/"api_token"\s*:\s*"(xoxc-[^"]+)"/);
		if (m) return m[1];
	}
	return 'NOT_FOUND';
})()
`

// ExtractTokenFromBrowser uses AppleScript (macOS) or PowerShell (Windows)
// to execute JS in the active Slack browser tab. No CDP, no extra HTTP
// requests — reads directly from the already-loaded page.
func ExtractTokenFromBrowser(browser string) (token string, err error) {
	switch runtime.GOOS {
	case "darwin":
		return extractTokenMacOS(browser)
	case "windows":
		return extractTokenWindows(browser)
	default:
		return "", fmt.Errorf("automatic token extraction not supported on %s. Use auth manual instead", runtime.GOOS)
	}
}

func extractTokenMacOS(browser string) (string, error) {
	// Map browser name to AppleScript application name
	appName := ""
	switch strings.ToLower(browser) {
	case "edge":
		appName = "Microsoft Edge"
	case "chrome":
		appName = "Google Chrome"
	case "brave":
		appName = "Brave Browser"
	default:
		return "", fmt.Errorf("AppleScript token extraction supports: chrome, edge, brave (got %q)", browser)
	}

	// AppleScript: find Slack tab and execute JS
	script := fmt.Sprintf(`
tell application "%s"
	set foundToken to "NOT_FOUND"
	repeat with w in windows
		repeat with t in tabs of w
			if URL of t contains ".slack.com" then
				set jsResult to execute t javascript "%s"
				if jsResult starts with "xoxc-" then
					set foundToken to jsResult
					exit repeat
				end if
			end if
		end repeat
		if foundToken starts with "xoxc-" then exit repeat
	end repeat
	return foundToken
end tell`, appName, strings.ReplaceAll(extractTokenJS, `"`, `\""`))

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("AppleScript failed: %w. Make sure %s is running with a Slack tab open", err, appName)
	}

	result := strings.TrimSpace(string(out))
	if !strings.HasPrefix(result, "xoxc-") {
		return "", fmt.Errorf("no xoxc- token found in %s Slack tabs. Make sure your Slack workspace is fully loaded", appName)
	}

	return result, nil
}

func extractTokenWindows(browser string) (string, error) {
	// PowerShell: use UI Automation or browser COM to run JS
	// Edge and Chrome expose DevTools via their COM interface, but the simplest
	// cross-browser approach on Windows is to read the page via PowerShell + COM

	var exeName string
	switch strings.ToLower(browser) {
	case "edge":
		exeName = "msedge"
	case "chrome":
		exeName = "chrome"
	default:
		return "", fmt.Errorf("PowerShell token extraction supports: chrome, edge (got %q)", browser)
	}

	// Strategy: Use the browser's --dump-dom on a named pipe? No — that makes a new request.
	// Instead, use PowerShell to access Edge via COMObject (InternetExplorer.Application won't work for Edge).
	// The most reliable Windows approach without CDP: read Slack's localStorage via PowerShell + Selenium.
	// But that's heavy. Better approach: clipboard injection.
	//
	// Simplest reliable approach: ask the user to copy from browser console.
	// But we want automation. Let's use PowerShell + SendKeys to:
	// 1. Activate the browser window
	// 2. Open DevTools console (F12, then Ctrl+Shift+J for console)
	// 3. Type JS expression
	// 4. Copy result to clipboard
	// 5. Read clipboard

	_ = exeName

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$js = '%s'

# Find browser window
$proc = Get-Process -Name "%s" -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -ne "" } | Select-Object -First 1
if (-not $proc) { Write-Output "BROWSER_NOT_FOUND"; exit }

# Activate window
$sig = '[DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);'
$type = Add-Type -MemberDefinition $sig -Name Win32 -Namespace Native -PassThru
$type::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
Start-Sleep -Milliseconds 500

# Open console: Ctrl+Shift+J
[System.Windows.Forms.SendKeys]::SendWait("^+j")
Start-Sleep -Milliseconds 1000

# Type JS and execute
[System.Windows.Forms.SendKeys]::SendWait("copy($($js)){ENTER}")
Start-Sleep -Milliseconds 500

# Close console
[System.Windows.Forms.SendKeys]::SendWait("{F12}")
Start-Sleep -Milliseconds 300

# Read clipboard
$result = [System.Windows.Forms.Clipboard]::GetText()
Write-Output $result
`, strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(extractTokenJS), "'", "''"), "\n", " "), exeName)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("PowerShell failed: %w. Make sure %s is running with a Slack tab open", err, exeName)
	}

	result := strings.TrimSpace(string(out))
	if result == "BROWSER_NOT_FOUND" {
		return "", fmt.Errorf("browser %s not found running. Open Slack in %s first", exeName, exeName)
	}
	if !strings.HasPrefix(result, "xoxc-") {
		return "", fmt.Errorf("no xoxc- token found. Make sure the active tab in %s is your Slack workspace", exeName)
	}

	return result, nil
}
