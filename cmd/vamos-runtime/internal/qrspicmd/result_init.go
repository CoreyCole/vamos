package qrspicmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi/semantic"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type ResultInitOptions struct {
	StateFile string
	State     string
	Outcome   string
	Artifact  string
}

type ResultInitRequest struct {
	State    ManagerState
	StateID  string
	Outcome  string
	Artifact string
	Now      time.Time
}

// InitResultRecord validates a selected lifecycle state before publishing one
// stable, child-bound template. The child fills only summary and artifact text
// after this call; graph controls are never taken from the edited file.
func InitResultRecord(
	ctx context.Context,
	req ResultInitRequest,
) (ResultRecordRef, ResultRecord, error) {
	if req.State.ActiveChild == nil {
		return ResultRecordRef{}, ResultRecord{}, errors.New("no active child")
	}
	child := req.State.ActiveChild
	if strings.TrimSpace(req.State.ManagerRunID) == "" {
		return ResultRecordRef{}, ResultRecord{}, errors.New("manager run ID is required")
	}
	if strings.TrimSpace(req.StateID) == "" {
		return ResultRecordRef{}, ResultRecord{}, errors.New("result state is required")
	}
	sessionPath := strings.TrimSpace(child.SessionPath)
	if sessionPath == "" {
		var err error
		sessionPath, err = ResolveSessionPath(
			child.SessionDir,
			child.SessionID,
			child.Cwd,
		)
		if err != nil {
			return ResultRecordRef{}, ResultRecord{}, fmt.Errorf(
				"resolve active child session: %w",
				err,
			)
		}
		child.SessionPath = sessionPath
	}
	sessionRef, err := ThoughtsRelativePath(req.State.CanonicalPlanDir, sessionPath)
	if err != nil {
		return ResultRecordRef{}, ResultRecord{}, err
	}
	artifact := strings.TrimSpace(req.Artifact)
	artifacts := []qrspi.Artifact(nil)
	if artifact != "" {
		if _, err := ResolveThoughtsReference(
			req.State.CanonicalPlanDir,
			artifact,
		); err != nil {
			return ResultRecordRef{}, ResultRecord{}, fmt.Errorf(
				"invalid artifact: %w",
				err,
			)
		}
		artifacts = []qrspi.Artifact{{Role: "primary", Path: artifact}}
	}
	id, err := newResultRecordID()
	if err != nil {
		return ResultRecordRef{}, ResultRecord{}, err
	}
	if req.Now.IsZero() {
		req.Now = time.Now()
	}
	record := ResultRecord{
		Version:               resultRecordVersion,
		ID:                    id,
		ManagerRunID:          req.State.ManagerRunID,
		SourceChildID:         child.ID,
		SourceChildGeneration: child.Generation,
		Node:                  string(req.State.Workflow.CurrentNodeID),
		State:                 strings.TrimSpace(req.StateID),
		Outcome:               strings.TrimSpace(req.Outcome),
		CreatedAt:             req.Now.UTC(),
		Session:               SessionReference{ID: child.SessionID, Path: sessionRef},
		Artifacts:             artifacts,
	}
	if _, err := ApplyResultRecord(
		req.State,
		ResultRecordRef{ID: id},
		record,
	); err != nil {
		return ResultRecordRef{}, ResultRecord{}, fmt.Errorf(
			"invalid result selection: %w",
			err,
		)
	}
	filename := fmt.Sprintf(
		"%s-%s-%s.yaml",
		req.Now.UTC().Format("20060102T150405.000000000Z"),
		record.Node,
		id,
	)
	ref := ResultRecordRef{
		ID:   id,
		Path: filepath.Join(PlanResultDir(req.State.CanonicalPlanDir), filename),
	}
	if err := WriteResultRecord(ref.Path, record); err != nil {
		return ResultRecordRef{}, ResultRecord{}, err
	}
	return ref, record, nil
}

func newResultRecordID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "qrspi_result_" + hex.EncodeToString(random[:]), nil
}

// ApplyResultRecord is the sole compatibility boundary from the minimal durable
// record into the existing QRSPI semantic and transition authority.
func ApplyResultRecord(
	state ManagerState,
	ref ResultRecordRef,
	record ResultRecord,
) (ParsedDecision, error) {
	policy := qrspi.ParsePolicy(state.Workflow.Policy)
	parsed := qrspi.Result{
		Stage:   record.Node,
		Status:  record.State,
		Outcome: record.Outcome,
		Policy: qrspi.PolicyYAML{
			AdvanceMode:             policy.AdvanceMode,
			AutoMode:                policy.AutoMode,
			EnablePlanReviews:       policy.EnablePlanReviews,
			InvalidResultRetryLimit: policy.InvalidResultRetryLimit,
		},
		Summary:   record.Summary,
		Artifacts: record.Artifacts,
	}
	if len(record.Artifacts) > 0 {
		parsed.Artifact = record.Artifacts[0].Path
	}
	def, err := Definition()
	if err != nil {
		return ParsedDecision{}, err
	}
	applied, err := semantic.Apply(nil, semantic.ApplyInput{
		Definition:   def,
		ParsedResult: &parsed,
		ParseContext: wruntime.ParseContext{
			WorkflowType:   string(qrspi.AgentChatWorkflowType),
			ExpectedNodeID: state.Workflow.CurrentNodeID,
			RunID:          record.SourceChildID,
			SessionID:      record.Session.ID,
			SessionPath:    record.Session.Path,
		},
		Context: semantic.Context{
			WorkflowType:      qrspi.AgentChatWorkflowType,
			State:             state.Workflow,
			ExpectedNodeID:    state.Workflow.CurrentNodeID,
			Source:            semantic.SourceRunCompletion,
			PlanDir:           state.CanonicalPlanDir,
			ImplementationCwd: state.ImplementationCwd,
			PlanningCwd:       state.SourceCwd,
			RunID:             record.SourceChildID,
		},
	})
	if err != nil {
		return ParsedDecision{}, GraphContractError(err)
	}
	return ParsedDecision{
		Result:         applied.WorkflowResult,
		Decision:       applied.Decision,
		Normalizations: resultNormalizations(applied.Normalizations),
		RecordRef:      &ref,
	}, nil
}

func ReadValidatedActiveResultRecord(state ManagerState) (ParsedDecision, error) {
	if state.ActiveChild == nil || strings.TrimSpace(state.ActiveChild.ResultPath) == "" {
		return ParsedDecision{}, ResultRecordNotFoundError{
			Path: "active child has no result record",
		}
	}
	ref := ResultRecordRef{
		ID:   state.ActiveChild.ResultID,
		Path: state.ActiveChild.ResultPath,
	}
	record, err := ReadResultRecord(ref.Path)
	if err != nil {
		return ParsedDecision{}, err
	}
	if err := ValidateResultRecordAt(state, ref, record); err != nil {
		return ParsedDecision{}, err
	}
	return ApplyResultRecord(state, ref, record)
}

func RunResultInit(
	ctx context.Context,
	opts ResultInitOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.StateFile) == "" {
		return errors.New("state-file is required")
	}
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, "", clock)
	lock, err := store.AcquireOperationLock(ctx, opts.StateFile)
	if err != nil {
		return err
	}
	defer lock.Release()
	state, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if state.ActiveChild == nil {
		return errors.New("no active child")
	}
	claimStore := store
	if d.StateStore == nil {
		claimStore = stateStore(d, filepath.Dir(filepath.Dir(opts.StateFile)), clock)
	}
	claim, err := claimStore.AcquireClaim(ctx, ClaimRequest{
		Key: LockKey{
			RepoID:           state.RepoID,
			CanonicalPlanDir: state.CanonicalPlanDir,
		},
		Operation:               ClaimActiveChildMutate,
		HolderRunID:             state.ManagerRunID,
		ExpectedChildID:         state.ActiveChild.ID,
		ExpectedChildGeneration: state.ActiveChild.Generation,
		ExpectedTransitionEpoch: state.TransitionEpoch,
	})
	if err != nil {
		return err
	}
	defer claimStore.ReleaseClaim(
		context.Background(),
		claim,
	) // best-effort narrow-claim cleanup
	ref, _, err := InitResultRecord(ctx, ResultInitRequest{
		State:    state,
		StateID:  opts.State,
		Outcome:  opts.Outcome,
		Artifact: opts.Artifact,
		Now:      clock(),
	})
	if err != nil {
		return err
	}
	latest, err := store.Load(opts.StateFile)
	if err != nil {
		return err
	}
	if latest.ActiveChild == nil || latest.ActiveChild.ID != state.ActiveChild.ID ||
		latest.ActiveChild.Generation != state.ActiveChild.Generation || latest.TransitionEpoch != state.TransitionEpoch {
		return fmt.Errorf(
			"result record %s was created but active child changed; repair with explicit manager attachment",
			ref.Path,
		)
	}
	latest.ActiveChild.ResultID = ref.ID
	latest.ActiveChild.ResultPath = ref.Path
	if latest.ActiveChild.SessionPath == "" {
		latest.ActiveChild.SessionPath = state.ActiveChild.SessionPath
	}
	if err := store.Save(opts.StateFile, latest); err != nil {
		return fmt.Errorf(
			"result record %s was created but binding was not saved: %w",
			ref.Path,
			err,
		)
	}
	_, err = fmt.Fprintf(
		ensureWriter(out),
		"result record: %s\nresult id: %s\n",
		ref.Path,
		ref.ID,
	)
	return err
}
