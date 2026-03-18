package auth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

// JS to extract all xoxc- tokens. Uses regex match across all localStorage values.
// Returns pipe-separated tokens (newlines get eaten by AppleScript).
// Priority: script tag boot_data first, then globals, then localStorage regex scan.
const extractJS = `(function(){
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

	// Write JS to a temp file to completely avoid AppleScript quote escaping issues
	tmpDir := os.TempDir()
	jsFile := filepath.Join(tmpDir, "slackogo_extract.js")
	if err := os.WriteFile(jsFile, []byte(extractJS), 0600); err != nil {
		return "", fmt.Errorf("failed to write temp JS file: %w", err)
	}
	defer os.Remove(jsFile)

	// AppleScript reads JS from the temp file — no embedded quotes at all
	// Uses 'with timeout' to prevent hanging on tabs that are loading/stuck.
	// Returns as soon as the first tab yields a valid xoxc- token.
	script := fmt.Sprintf(`set jsCode to read POSIX file "%s"
tell application "%s"
	set tokenResult to "NOT_FOUND"
	repeat with w in every window
		repeat with t in every tab of w
			try
				with timeout of 5 seconds
					if URL of t contains ".slack.com" then
						set tokenResult to (execute t javascript jsCode)
						if tokenResult contains "xoxc-" then
							return tokenResult
						end if
					end if
				end timeout
			on error
				-- Tab timed out or failed, skip to next
			end try
		end repeat
	end repeat
	return tokenResult
end tell`, jsFile, appName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("AppleScript failed: %s (%w). Make sure %s is running with a Slack tab open", errMsg, err, appName)
	}

	result := strings.TrimSpace(string(out))
	if !strings.Contains(result, "xoxc-") {
		return "", fmt.Errorf("no xoxc- token found in %s Slack tabs. Make sure your Slack workspace is fully loaded", appName)
	}

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

	// Write JS to temp file
	tmpDir := os.TempDir()
	jsFile := filepath.Join(tmpDir, "slackogo_extract.js")
	if err := os.WriteFile(jsFile, []byte(extractJS), 0600); err != nil {
		return "", fmt.Errorf("failed to write temp JS file: %w", err)
	}
	defer os.Remove(jsFile)

	// PowerShell: activate browser, open console, run JS via file, read clipboard.
	// Uses exec.CommandContext for timeout protection against hanging.
	psScript := fmt.Sprintf("\n"+
		"Add-Type -AssemblyName System.Windows.Forms\n"+
		"$proc = Get-Process -Name \"%s\" -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -ne \"\" } | Select-Object -First 1\n"+
		"if (-not $proc) { Write-Output \"BROWSER_NOT_FOUND\"; exit }\n"+
		"$sig = '[DllImport(\"user32.dll\")] public static extern bool SetForegroundWindow(IntPtr hWnd);'\n"+
		"$type = Add-Type -MemberDefinition $sig -Name Win32 -Namespace Native -PassThru\n"+
		"$type::SetForegroundWindow($proc.MainWindowHandle) | Out-Null\n"+
		"Start-Sleep -Milliseconds 500\n"+
		"[System.Windows.Forms.SendKeys]::SendWait(\"^+j\")\n"+
		"Start-Sleep -Milliseconds 1000\n"+
		"$js = Get-Content -Raw \"%s\"\n"+
		"$js = $js -replace \"`n\", \" \"\n"+
		"$escaped = \"copy(\" + $js + \")\"\n"+
		"[System.Windows.Forms.SendKeys]::SendWait($escaped + \"{ENTER}\")\n"+
		"Start-Sleep -Milliseconds 500\n"+
		"[System.Windows.Forms.SendKeys]::SendWait(\"{F12}\")\n"+
		"Start-Sleep -Milliseconds 300\n"+
		"$result = [System.Windows.Forms.Clipboard]::GetText()\n"+
		"Write-Output $result\n",
		exeName, strings.ReplaceAll(jsFile, `\`, `\\`))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
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
