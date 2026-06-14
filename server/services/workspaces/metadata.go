package workspaces

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type WorkspaceMetadata struct {
	Slug         string
	ProjectID    string
	CheckoutPath string
	ManagerURL   string
	RestartToken string `json:"-"`
	DatabasePath string
	PID          int
	Port         int
}

func WorkspaceMetadataPath(checkoutPath string, metadataDirName ...string) string {
	return RuntimePaths(checkoutPath, metadataDirName...).WorkspaceEnv
}

func WriteMetadata(path string, meta WorkspaceMetadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	databasePath := strings.TrimSpace(meta.DatabasePath)
	if databasePath == "" && strings.TrimSpace(meta.CheckoutPath) != "" {
		databasePath = RuntimePaths(meta.CheckoutPath).AgentsDB
	}
	content := fmt.Sprintf(
		"VAMOS_WORKSPACE_SLUG=%s\nVAMOS_WORKSPACE_PROJECT_ID=%s\nVAMOS_WORKSPACE_CHECKOUT=%s\nVAMOS_WORKSPACE_MANAGER_URL=%s\nVAMOS_WORKSPACE_RESTART_TOKEN=%s\nVAMOS_DATABASE_PATH=%s\nVAMOS_WORKSPACE_PID=%d\nVAMOS_WORKSPACE_PORT=%d\n",
		shellValue(meta.Slug),
		shellValue(meta.ProjectID),
		shellValue(meta.CheckoutPath),
		shellValue(meta.ManagerURL),
		shellValue(meta.RestartToken),
		shellValue(databasePath),
		meta.PID,
		meta.Port,
	)
	return os.WriteFile(path, []byte(content), 0o600)
}

func ReadMetadata(path string) (WorkspaceMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return WorkspaceMetadata{}, err
	}
	defer f.Close()

	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(key)] = unshellValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return WorkspaceMetadata{}, err
	}

	pid, _ := strconv.Atoi(vals["VAMOS_WORKSPACE_PID"])
	port, _ := strconv.Atoi(vals["VAMOS_WORKSPACE_PORT"])
	return WorkspaceMetadata{
		Slug:         vals["VAMOS_WORKSPACE_SLUG"],
		ProjectID:    vals["VAMOS_WORKSPACE_PROJECT_ID"],
		CheckoutPath: vals["VAMOS_WORKSPACE_CHECKOUT"],
		ManagerURL:   vals["VAMOS_WORKSPACE_MANAGER_URL"],
		RestartToken: vals["VAMOS_WORKSPACE_RESTART_TOKEN"],
		DatabasePath: vals["VAMOS_DATABASE_PATH"],
		PID:          pid,
		Port:         port,
	}, nil
}

func shellValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func unshellValue(value string) string {
	if len(value) < 2 || !strings.HasPrefix(value, "'") ||
		!strings.HasSuffix(value, "'") {
		return value
	}
	inner := strings.TrimPrefix(strings.TrimSuffix(value, "'"), "'")
	return strings.ReplaceAll(inner, "'\\''", "'")
}
