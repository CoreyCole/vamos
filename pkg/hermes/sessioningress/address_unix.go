//go:build !windows

package sessioningress

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func SurfaceSupported() bool { return true }

func CurrentEUID() (int, error) { return os.Geteuid(), nil }

func PrepareRuntimeDirectory(path string, euid int) error {
	if euid < 0 {
		return errors.New("effective UID must be a non-negative integer")
	}
	info, err := os.Lstat(path)
	switch {
	case err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()):
		return errors.New("runtime path must be a real directory")
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect runtime directory: %w", err)
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create runtime directory: %w", err)
		}
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set runtime directory permissions: %w", err)
	}
	return ValidateRuntimeDirectory(path, euid)
}

func ValidateRuntimeDirectory(path string, euid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("runtime path must be a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		return errors.New("runtime directory permissions must be 0700")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("runtime directory owner is unavailable")
	}
	if uint64(stat.Uid) != uint64(euid) {
		return errors.New("runtime directory owner does not match effective UID")
	}
	return nil
}
