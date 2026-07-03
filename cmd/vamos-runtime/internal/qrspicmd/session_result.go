package qrspicmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type sessionEntry struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Cwd     string          `json:"cwd,omitempty"`
	Message *sessionMessage `json:"message,omitempty"`
}

type sessionMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stopReason,omitempty"`
}

type sessionContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func ResolveSessionPath(sessionDir, sessionID, cwd string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" || strings.TrimSpace(sessionID) == "" {
		return "", errors.New("session dir and session id are required")
	}

	if _, err := os.Stat(sessionDir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session %q not found in %s", sessionID, sessionDir)
		}
		return "", err
	}

	var matches []string
	walkErr := filepath.WalkDir(
		sessionDir,
		func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				return nil
			}
			header, err := readSessionHeader(path)
			if err != nil {
				return nil
			}
			if header.Type == "session" && header.ID == sessionID &&
				(strings.TrimSpace(cwd) == "" || header.Cwd == cwd) {
				matches = append(matches, path)
			}
			return nil
		},
	)
	if walkErr != nil {
		return "", walkErr
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("session %q not found in %s", sessionID, sessionDir)
	default:
		return "", fmt.Errorf(
			"session %q matched multiple files in %s",
			sessionID,
			sessionDir,
		)
	}
}

func readSessionHeader(path string) (sessionEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionEntry{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return sessionEntry{}, err
		}
		return sessionEntry{}, errors.New("empty session file")
	}
	var header sessionEntry
	if err := json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &header); err != nil {
		return sessionEntry{}, err
	}
	return header, nil
}

type ChildProviderError struct {
	SessionPath string
}

func (e ChildProviderError) Error() string {
	return fmt.Sprintf(
		"session %s ended with provider error before qrspi_result",
		e.SessionPath,
	)
}

func IsChildProviderError(err error) bool {
	var providerErr ChildProviderError
	return errors.As(err, &providerErr)
}

func ExtractFinalAssistantTextFromSession(path string) (string, error) {
	last, err := extractLastAssistantTextFromSession(path, true)
	if err != nil {
		return "", err
	}
	if last == "" {
		return "", fmt.Errorf(
			"session %s has no assistant text containing qrspi_result",
			path,
		)
	}
	return last, nil
}

func ExtractLastAssistantTextFromSession(path string) (string, error) {
	last, err := extractLastAssistantTextFromSession(path, false)
	if err != nil {
		return "", err
	}
	if last == "" {
		return "", fmt.Errorf("session %s has no assistant text", path)
	}
	return last, nil
}

func extractLastAssistantTextFromSession(
	path string,
	requireQRSPIResult bool,
) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var last string
	var sawProviderError bool
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry sessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != "message" || entry.Message == nil ||
			entry.Message.Role != "assistant" {
			continue
		}
		if requireQRSPIResult && entry.Message.StopReason == "error" {
			sawProviderError = true
			continue
		}
		if requireQRSPIResult && entry.Message.StopReason == "aborted" {
			continue
		}
		text := textBlocksFromAssistantMessage(*entry.Message)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !requireQRSPIResult || strings.Contains(text, "qrspi_result") {
			last = text
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if last == "" && sawProviderError {
		return "", ChildProviderError{SessionPath: path}
	}
	return last, nil
}

func textBlocksFromAssistantMessage(msg sessionMessage) string {
	var blocks []sessionContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil {
		return text
	}
	return ""
}
