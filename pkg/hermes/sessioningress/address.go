package sessioningress

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	MaxSessionIDBytes  = 1_024
	MaxSocketPathBytes = 103
)

var ErrSurfaceUnsupported = errors.New("surface_unsupported")

func ValidateSessionID(sessionID string) ([]byte, error) {
	encoded := []byte(sessionID)
	if !validUTF8Size(sessionID, 1, MaxSessionIDBytes) {
		return nil, errors.New("session ID must contain 1 to 1024 UTF-8 bytes")
	}
	if hasControl(sessionID) {
		return nil, errors.New("session ID must not contain C0 or C1 control characters")
	}
	return encoded, nil
}

func DeriveSocketBasename(sessionID string) (string, error) {
	exact, err := ValidateSessionID(sessionID)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(exact)
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:])
	return "v1-" + strings.ToLower(token) + ".sock", nil
}

func SelectRuntimeDirectory(hermesHome, socketBasename string, euid int) (string, error) {
	if !filepath.IsAbs(hermesHome) {
		return "", errors.New("HERMES_HOME must be absolute")
	}
	if socketBasename == "" || filepath.Base(socketBasename) != socketBasename ||
		strings.ContainsRune(socketBasename, 0) {
		return "", errors.New("socket basename must be one path component")
	}
	preferred := filepath.Join(hermesHome, "run", "session-ingress-v1")
	if len([]byte(filepath.Join(preferred, socketBasename))) <= MaxSocketPathBytes {
		return preferred, nil
	}
	if euid < 0 {
		return "", errors.New("effective UID must be a non-negative integer")
	}
	return fmt.Sprintf("/tmp/hsi-%d", euid), nil
}

func DeriveSocketPath(sessionID, hermesHome string, euid int) (string, error) {
	basename, err := DeriveSocketBasename(sessionID)
	if err != nil {
		return "", err
	}
	directory, err := SelectRuntimeDirectory(hermesHome, basename, euid)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, basename), nil
}
