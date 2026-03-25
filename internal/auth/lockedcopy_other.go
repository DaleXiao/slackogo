//go:build !windows

package auth

// prepareWindowsCookieSnapshot is a no-op on non-Windows platforms.
// macOS and Linux use advisory locks that don't prevent reading.
func prepareWindowsCookieSnapshot(cookiesDBPath string) (string, func(), error) {
	return "", nil, nil
}
