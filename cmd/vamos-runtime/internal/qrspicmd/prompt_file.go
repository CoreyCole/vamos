package qrspicmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func resolveOrInitStartState(
	ctx context.Context,
	opts StartNextOptions,
	d deps,
) (ManagerState, string, error) {
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	if strings.TrimSpace(opts.StateFile) != "" {
		store := stateStore(d, "", clock)
		state, err := store.Load(opts.StateFile)
		if err != nil {
			return ManagerState{}, "", err
		}
		if piModel := strings.TrimSpace(opts.PiModel); piModel != "" {
			state.PiModel = piModel
			if err := store.Save(opts.StateFile, state); err != nil {
				return ManagerState{}, "", err
			}
		}
		return state, opts.StateFile, nil
	}
	if strings.TrimSpace(opts.PlanDir) == "" {
		return ManagerState{}, "", errors.New("plan-dir is required")
	}
	projectRoot := strings.TrimSpace(opts.ProjectRoot)
	if projectRoot == "" {
		projectRoot = "."
	}
	policy, err := initialPolicy(opts.PolicyFile, opts.PolicyPreset)
	if err != nil {
		return ManagerState{}, "", err
	}
	state, err := InitialManagerState(opts.PlanDir, projectRoot, policy)
	if err != nil {
		return ManagerState{}, "", err
	}
	if isFastPolicyPreset(opts.PolicyPreset) && strings.TrimSpace(opts.NodeID) == "" {
		opts.NodeID = string(qrspi.NodeOutline)
	}
	if err := ApplyInitOverrides(
		&state,
		InitOverrides{
			NodeID:            opts.NodeID,
			ImplementationCwd: opts.ImplementationCwd,
			PiModel:           opts.PiModel,
		},
	); err != nil {
		return ManagerState{}, "", err
	}
	state.ManagerPaneID = CaptureManagerPaneID(opts.ManagerPane)
	state.ManagerRunID = managerRunID(clock())
	root, err := stateRoot(d)
	if err != nil {
		return ManagerState{}, "", err
	}
	store := stateStore(d, root, clock)
	key := LockKey{RepoID: state.RepoID, CanonicalPlanDir: state.CanonicalPlanDir}
	if _, err := store.AcquireLock(ctx, key, state.ManagerRunID, lockTTL); err != nil {
		return ManagerState{}, "", err
	}
	stateFile := StatePath(root, key, state.ManagerRunID)
	if err := store.Save(stateFile, state); err != nil {
		return ManagerState{}, "", err
	}
	return state, stateFile, nil
}

func selectLaunchNode(state ManagerState, opts StartNextOptions) (wruntime.Node, error) {
	def, err := Definition()
	if err != nil {
		return wruntime.Node{}, err
	}
	nodeID := wruntime.NodeID(strings.TrimSpace(opts.NodeID))
	if nodeID == "" {
		nodeID = state.Workflow.CurrentNodeID
	}
	if nodeID == "" {
		nodeID = def.Start
	}
	node, ok := def.Nodes[nodeID]
	if !ok {
		return wruntime.Node{}, fmt.Errorf("node %q is not in QRSPI definition", nodeID)
	}
	return node, nil
}

func defaultChildCwd(
	state ManagerState,
	nodeID wruntime.NodeID,
	override string,
) (string, error) {
	if cwd := strings.TrimSpace(override); cwd != "" {
		return cwd, nil
	}
	switch nodeID {
	case "implement", "review-implementation", "verify":
		if cwd := strings.TrimSpace(state.ImplementationCwd); cwd != "" {
			return cwd, nil
		}
	}
	if cwd := strings.TrimSpace(state.SourceCwd); cwd != "" {
		return cwd, nil
	}
	return "", errors.New("child cwd is required")
}

func renderPromptForNode(
	state ManagerState,
	node wruntime.Node,
	planDir string,
) (string, error) {
	manifest, err := LoadManifest(state.SourceCwd)
	if err != nil {
		return "", err
	}
	return RenderStagePrompt(PromptContext{
		Node:       node,
		State:      state,
		PlanDir:    planDir,
		Manifest:   manifest,
		LastResult: state.Workflow.LastResult,
	})
}

func WriteStagePromptFile(
	ctx context.Context,
	state ManagerState,
	node wruntime.Node,
	opts PromptFileOptions,
) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	prompt, err := renderPromptForNode(state, node, state.CanonicalPlanDir)
	if err != nil {
		return "", err
	}
	when := opts.Timestamp
	if when.IsZero() {
		when = time.Now()
	}
	path := promptPathFor(opts.StateFile, node.ID, when)
	if err := writeFileAtomically(path, []byte(prompt), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func promptPathFor(stateFile string, nodeID wruntime.NodeID, timestamp time.Time) string {
	stamp := timestamp.UTC().Format("20060102T150405.000000000Z")
	return filepath.Join(
		filepath.Dir(stateFile),
		"prompts",
		fmt.Sprintf("%s-%s.md", nodeID, stamp),
	)
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
