//go:build !windows

package auth

// CDPImportResult holds cookie + token extracted via CDP
type CDPImportResult struct {
	Cookie string
	Token  string
}

// CDPImport is a no-op on non-Windows platforms.
// macOS uses sweetcookie (Keychain) which handles decryption natively.
func CDPImport(targetWorkspace string) (*CDPImportResult, error) {
	return nil, nil
}
