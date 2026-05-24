package fixtures

import (
	"context"
	"fmt"
)

func BuildThoughtsWorkbenchBasic(
	ctx context.Context,
	db DBTX,
	input Input,
) (State, error) {
	if input.Workspace.Slug == "" || input.Workspace.Slug == "main" {
		return State{}, fmt.Errorf("fixture requires non-main workspace slug")
	}
	if input.Workspace.CheckoutPath == "" || input.Workspace.DBPath == "" {
		return State{}, fmt.Errorf("fixture requires workspace checkout and DB path")
	}
	if err := db.QueryRowContext(ctx, "select 1").Scan(new(int)); err != nil {
		return State{}, fmt.Errorf("workspace DB ping: %w", err)
	}
	return State{
		Name: "thoughts-workbench.basic",
		Data: map[string]any{"workspace_slug": input.Workspace.Slug},
	}, nil
}
