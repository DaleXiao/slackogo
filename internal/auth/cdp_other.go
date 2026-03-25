//go:build !windows

package auth

// cdpExtractCookie is a no-op on non-Windows platforms.
// macOS doesn't use App-Bound Encryption.
func cdpExtractCookie(cookieName, targetURL string) (string, error) {
	return "", nil
}
