package workflows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func applyQRSPIWorkspaceResult(
	state wruntime.State,
	result wruntime.WorkflowResult,
	planningCwd string,
) (wruntime.State, error) {
	if result.SourceNodeID != qrspi.NodeWorkspace {
		return state, nil
	}
	workspacePath := strings.TrimSpace(result.Workspace)
	if workspacePath == "" {
		return state, errors.New("q-workspace result must include absolute <workspace>")
	}
	if !filepath.IsAbs(workspacePath) {
		return state, fmt.Errorf(
			"q-workspace result workspace must be absolute: %q",
			workspacePath,
		)
	}
	planningCwd = strings.TrimSpace(planningCwd)
	if planningCwd != "" && filepath.Clean(workspacePath) == filepath.Clean(planningCwd) {
		return state, fmt.Errorf(
			"q-workspace result workspace must differ from planning checkout cwd: %q",
			workspacePath,
		)
	}
	info, err := os.Stat(workspacePath)
	if err != nil {
		return state, fmt.Errorf("stat q-workspace workspace: %w", err)
	}
	if !info.IsDir() {
		return state, fmt.Errorf(
			"q-workspace workspace is not a directory: %q",
			workspacePath,
		)
	}
	state.ExecutionCwd = workspacePath
	return state, nil
}

func effectiveNodeCwd(state wruntime.State, nodeID wruntime.NodeID) string {
	switch nodeID {
	case qrspi.NodeImplement,
		qrspi.NodeReviewImplementation,
		qrspi.NodeHumanReviewImplementation,
		qrspi.NodeDone:
		return strings.TrimSpace(state.ExecutionCwd)
	default:
		return ""
	}
}
