package agentchat

import (
	"context"
	"errors"
	"log"
	"time"
)

const (
	SyncPhaseWorkspace        = "workspace"
	SyncPhaseTerminalMetadata = "terminal_metadata"
	SyncPhaseQRSPIProjection  = "qrspi_projection"

	SyncPhaseStatusOK      = "ok"
	SyncPhaseStatusFailed  = "failed"
	SyncPhaseStatusSkipped = "skipped"
)

type SyncCoordinatorInput struct {
	Workspace SyncWorkspacesInput
	Terminal  TerminalMetadataIndexInput
	QRSPI     QRSPIProjectionApplyInput
}

type SyncCoordinatorResult struct {
	Workspace   SyncWorkspacesResult
	Terminal    TerminalMetadataIndexResult
	QRSPI       QRSPIProjectionApplyResult
	Diagnostics []SyncPhaseDiagnostic
	Changed     bool
}

type SyncPhaseDiagnostic struct {
	Phase       string
	Status      string
	StartedAt   time.Time
	CompletedAt time.Time
	Error       string
	Changed     bool
}

type TerminalMetadataIndexInput struct {
	EventLogPath string
	MaxEvents    int
	MaxBytes     int64
	MaxDuration  time.Duration
}

type TerminalMetadataIndexResult struct {
	EventsRead       int
	SessionsUpserted int
	QRSPIProjected   int
	Failed           int
	CursorAdvanced   bool
	Changed          bool
}

type QRSPIProjectionApplyInput struct {
	MaxResults  int
	MaxDuration time.Duration
}

type QRSPIProjectionApplyResult struct {
	Applied int
	Skipped int
	Failed  int
	Changed bool
}

type TerminalMetadataIndexer interface {
	IndexTerminalMetadata(context.Context, TerminalMetadataIndexInput) (TerminalMetadataIndexResult, error)
}

type QRSPIProjectionApplier interface {
	ApplyQRSPIProjections(context.Context, QRSPIProjectionApplyInput) (QRSPIProjectionApplyResult, error)
}

type WorkspaceSyncerInterface interface {
	Sync(context.Context, SyncWorkspacesInput) (SyncWorkspacesResult, error)
}

type SyncCoordinatorOptions struct {
	WorkspaceSync       WorkspaceSyncerInterface
	TerminalIndex       TerminalMetadataIndexer
	QRSPIApply          QRSPIProjectionApplier
	OnWorkspaceComplete func(context.Context, SyncWorkspacesResult, error)
}

type SyncCoordinator struct {
	workspaceSync       WorkspaceSyncerInterface
	terminalIndex       TerminalMetadataIndexer
	qrspiApply          QRSPIProjectionApplier
	onWorkspaceComplete func(context.Context, SyncWorkspacesResult, error)
}

func NewSyncCoordinator(opts SyncCoordinatorOptions) *SyncCoordinator {
	return &SyncCoordinator{
		workspaceSync:       opts.WorkspaceSync,
		terminalIndex:       opts.TerminalIndex,
		qrspiApply:          opts.QRSPIApply,
		onWorkspaceComplete: opts.OnWorkspaceComplete,
	}
}

func DefaultSyncCoordinatorInput(workspace SyncWorkspacesInput) SyncCoordinatorInput {
	workspace.RunCompletionHook = true
	return SyncCoordinatorInput{
		Workspace: workspace,
		Terminal: TerminalMetadataIndexInput{
			MaxEvents:   500,
			MaxBytes:    1 << 20,
			MaxDuration: 10 * time.Second,
		},
		QRSPI: QRSPIProjectionApplyInput{
			MaxResults:  50,
			MaxDuration: 10 * time.Second,
		},
	}
}

func (c *SyncCoordinator) Run(
	ctx context.Context,
	input SyncCoordinatorInput,
) (SyncCoordinatorResult, error) {
	if c == nil || c.workspaceSync == nil {
		return SyncCoordinatorResult{}, errors.New("sync coordinator requires workspace syncer")
	}

	var result SyncCoordinatorResult
	workspaceStarted := time.Now()
	workspace, workspaceErr := c.workspaceSync.Sync(ctx, input.Workspace)
	result.Workspace = workspace
	result.Diagnostics = append(result.Diagnostics, phaseDiagnostic(
		SyncPhaseWorkspace,
		workspaceStarted,
		workspace.Changed,
		workspaceErr,
	))
	if c.onWorkspaceComplete != nil {
		c.onWorkspaceComplete(ctx, workspace, workspaceErr)
	}
	if workspaceErr != nil {
		result.Changed = workspace.Changed
		return result, workspaceErr
	}

	if c.terminalIndex == nil {
		result.Diagnostics = append(result.Diagnostics, skippedPhaseDiagnostic(SyncPhaseTerminalMetadata))
	} else {
		started := time.Now()
		terminal, err := c.terminalIndex.IndexTerminalMetadata(ctx, input.Terminal)
		result.Terminal = terminal
		result.Diagnostics = append(result.Diagnostics, phaseDiagnostic(
			SyncPhaseTerminalMetadata,
			started,
			terminal.Changed,
			err,
		))
		if err != nil {
			log.Printf("sync_coordinator_terminal_metadata_failed: %v", err)
		}
	}

	if c.qrspiApply == nil {
		result.Diagnostics = append(result.Diagnostics, skippedPhaseDiagnostic(SyncPhaseQRSPIProjection))
	} else {
		started := time.Now()
		qrspi, err := c.qrspiApply.ApplyQRSPIProjections(ctx, input.QRSPI)
		result.QRSPI = qrspi
		result.Diagnostics = append(result.Diagnostics, phaseDiagnostic(
			SyncPhaseQRSPIProjection,
			started,
			qrspi.Changed,
			err,
		))
		if err != nil {
			log.Printf("sync_coordinator_qrspi_projection_failed: %v", err)
		}
	}

	result.Changed = result.Workspace.Changed || result.Terminal.Changed || result.QRSPI.Changed
	return result, nil
}

func phaseDiagnostic(
	phase string,
	started time.Time,
	changed bool,
	err error,
) SyncPhaseDiagnostic {
	status := SyncPhaseStatusOK
	errText := ""
	if err != nil {
		status = SyncPhaseStatusFailed
		errText = err.Error()
	}
	return SyncPhaseDiagnostic{
		Phase:       phase,
		Status:      status,
		StartedAt:   started,
		CompletedAt: time.Now(),
		Error:       errText,
		Changed:     changed,
	}
}

func skippedPhaseDiagnostic(phase string) SyncPhaseDiagnostic {
	now := time.Now()
	return SyncPhaseDiagnostic{
		Phase:       phase,
		Status:      SyncPhaseStatusSkipped,
		StartedAt:   now,
		CompletedAt: now,
	}
}
