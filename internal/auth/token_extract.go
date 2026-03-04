package auth

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ExtractTokenFromBrowser uses AppleScript (macOS) or PowerShell (Windows)
// to execute JS in the active Slack browser tab.
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

	// JS that extracts ALL xoxc- tokens from localStorage, separated by newlines.
	// The caller will filter for workspace-level token (T-prefix team_id) via auth.test.
	//
	// Priority: boot_data script tags first, then localStorage.
	// In Enterprise Grid, localStorage may have both enterprise (E-prefix) and
	// workspace (T-prefix) tokens. We return all candidates and let the caller pick.
	js := `(function(){var r=[];` +
		`var s=document.querySelectorAll('script');` +
		`for(var i=0;i<s.length;i++){` +
		`var m=s[i].textContent.match(/\"api_token\"\\s*:\\s*\"(xoxc-[^\"]+)\"/);` +
		`if(m)r.push(m[1])}` +
		`if(typeof boot_data!=='undefined'&&boot_data.api_token&&boot_data.api_token.indexOf('xoxc-')===0)r.push(boot_data.api_token);` +
		`if(typeof TS!=='undefined'&&TS.boot_data&&TS.boot_data.api_token&&TS.boot_data.api_token.indexOf('xoxc-')===0)r.push(TS.boot_data.api_token);` +
		`for(var j=0;j<localStorage.length;j++){` +
		`var v=localStorage.getItem(localStorage.key(j));` +
		`if(v&&v.indexOf('xoxc-')===0)r.push(v);` +
		`try{var o=JSON.parse(v);if(o&&o.token&&o.token.indexOf('xoxc-')===0)r.push(o.token)}catch(e){}}` +
		`return[...new Set(r)].join('\\n')})()`

	script := fmt.Sprintf(`tell application "%s"
	set tokenResult to "NOT_FOUND"
	repeat with w in every window
		repeat with t in every tab of w
			if URL of t contains ".slack.com" then
				set tokenResult to (execute t javascript "%s")
				if tokenResult contains "xoxc-" then
					return tokenResult
				end if
			end if
		end repeat
	end repeat
	return tokenResult
end tell`, appName, strings.ReplaceAll(js, `"`, `\"`))

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("AppleScript failed: %s (%w). Make sure %s is running with a Slack tab open", errMsg, err, appName)
	}

	result := strings.TrimSpace(string(out))
	if !strings.Contains(result, "xoxc-") {
		return "", fmt.Errorf("no xoxc- token found in %s Slack tabs. Make sure your Slack workspace is fully loaded", appName)
	}

	// Return all tokens newline-separated — caller filters via auth.test
	return result, nil
}

func extractTokenWindows(browser string) (string, error) {
	var exeName string
	switch strings.ToLower(browser) {
	case "edge":
		exeName = "msedge"
	case "chrome":
		exeName = "chrome"
	default:
		return "", fmt.Errorf("PowerShell token extraction supports: chrome, edge (got %q)", browser)
	}

	js := `(function(){var r=[];` +
		`var s=document.querySelectorAll('script');` +
		`for(var i=0;i<s.length;i++){` +
		`var m=s[i].textContent.match(/\"api_token\"\\s*:\\s*\"(xoxc-[^\"]+)\"/);` +
		`if(m)r.push(m[1])}` +
		`if(typeof boot_data!=='undefined'&&boot_data.api_token&&boot_data.api_token.indexOf('xoxc-')===0)r.push(boot_data.api_token);` +
		`if(typeof TS!=='undefined'&&TS.boot_data&&TS.boot_data.api_token&&TS.boot_data.api_token.indexOf('xoxc-')===0)r.push(TS.boot_data.api_token);` +
		`for(var j=0;j<localStorage.length;j++){` +
		`var v=localStorage.getItem(localStorage.key(j));` +
		`if(v&&v.indexOf('xoxc-')===0)r.push(v);` +
		`try{var o=JSON.parse(v);if(o&&o.token&&o.token.indexOf('xoxc-')===0)r.push(o.token)}catch(e){}}` +
		`return[...new Set(r)].join('\\n')})()`

	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$proc = Get-Process -Name "%s" -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -ne "" } | Select-Object -First 1
if (-not $proc) { Write-Output "BROWSER_NOT_FOUND"; exit }
$sig = '[DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);'
$type = Add-Type -MemberDefinition $sig -Name Win32 -Namespace Native -PassThru
$type::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
Start-Sleep -Milliseconds 500
[System.Windows.Forms.SendKeys]::SendWait("^+j")
Start-Sleep -Milliseconds 1000
$escaped = '%s' -replace '[+^%%~(){}]', '{$0}'
[System.Windows.Forms.SendKeys]::SendWait("copy($escaped){ENTER}")
Start-Sleep -Milliseconds 500
[System.Windows.Forms.SendKeys]::SendWait("{F12}")
Start-Sleep -Milliseconds 300
$result = [System.Windows.Forms.Clipboard]::GetText()
Write-Output $result
`, exeName, strings.ReplaceAll(js, "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("PowerShell failed: %s (%w). Make sure %s is running with a Slack tab open", errMsg, err, exeName)
	}

	result := strings.TrimSpace(string(out))
	if result == "BROWSER_NOT_FOUND" {
		return "", fmt.Errorf("browser %s not found running. Open Slack in %s first", exeName, exeName)
	}
	if !strings.Contains(result, "xoxc-") {
		return "", fmt.Errorf("no xoxc- token found. Make sure the active tab in %s is your Slack workspace", exeName)
	}

	return result, nil
}
