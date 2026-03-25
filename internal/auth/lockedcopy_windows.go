//go:build windows

package auth

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// copyLockedFile copies a file that is held open with an exclusive lock
// by another process (e.g. Edge's Cookies database on Windows).
// It uses CreateFileW with FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE
// to open the source, bypassing the standard Go os.Open which fails when
// the file is exclusively locked.
func copyLockedFile(src, dst string) error {
	srcW, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return fmt.Errorf("utf16 convert src: %w", err)
	}

	const (
		GENERIC_READ          = 0x80000000
		FILE_SHARE_READ       = 0x00000001
		FILE_SHARE_WRITE      = 0x00000002
		FILE_SHARE_DELETE     = 0x00000004
		OPEN_EXISTING         = 3
		FILE_ATTRIBUTE_NORMAL = 0x00000080
	)

	handle, err := syscall.CreateFile(
		srcW,
		GENERIC_READ,
		FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE,
		nil,
		OPEN_EXISTING,
		FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return fmt.Errorf("CreateFile %s: %w", src, err)
	}
	defer syscall.CloseHandle(handle)

	// Wrap the Windows handle in an os.File for convenient io.Copy
	srcFile := os.NewFile(uintptr(handle), src)
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return dstFile.Sync()
}

// _ is a compile-time check that unsafe is imported (needed for handle conversion).
var _ = unsafe.Sizeof(0)

// prepareWindowsCookieSnapshot copies the browser's Cookies database
// (and WAL/SHM sidecars if present) to a temporary directory using
// shared-mode file access. Returns the path to the copied Cookies file
// and a cleanup function that removes the temp dir.
//
// The returned path can be passed directly to sweetcookie as a profile
// override (it accepts a Cookies DB file path).
func prepareWindowsCookieSnapshot(cookiesDBPath string) (snapshotPath string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "slackogo-cookies-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(dir) }

	// Recreate the directory structure sweetcookie expects:
	//   <temp>/Network/Cookies  (if original is in Network/)
	//   <temp>/Cookies          (otherwise)
	base := filepath.Base(cookiesDBPath)
	parentDir := filepath.Base(filepath.Dir(cookiesDBPath))

	var targetDir string
	if parentDir == "Network" {
		targetDir = filepath.Join(dir, "Network")
	} else {
		targetDir = dir
	}
	os.MkdirAll(targetDir, 0755)

	target := filepath.Join(targetDir, base)

	if err := copyLockedFile(cookiesDBPath, target); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy cookies DB: %w", err)
	}

	// Copy WAL/SHM sidecars if they exist (best effort)
	for _, suffix := range []string{"-wal", "-shm"} {
		src := cookiesDBPath + suffix
		if _, statErr := os.Stat(src); statErr == nil {
			_ = copyLockedFile(src, target+suffix)
		}
	}

	return target, cleanup, nil
}
