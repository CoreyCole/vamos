package agentchat

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func NewWorkspaceID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("new workspace uuidv7: %w", err)
	}
	return id.String(), nil
}

func validateWorkspaceTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "New Workspace"
	}
	return truncateTitle(title)
}
