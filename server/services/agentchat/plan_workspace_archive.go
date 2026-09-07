package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	planArchiveReasonManual          = "manual"
	planArchiveReasonMissingFromDisk = "missing_from_disk"
	planListStatusCurrent            = "current"
	planListStatusArchived           = "archived"
)

var (
	ErrPlanWorkspaceNotFound               = errors.New("plan workspace not found")
	ErrPlanWorkspaceNotManuallyArchived    = errors.New("plan workspace is not manually archived")
	ErrPlanWorkspaceMissingFromDiskArchive = errors.New(
		"cannot unarchive plan archived as missing_from_disk",
	)
	ErrPlanWorkspaceArchiveConflict = errors.New(
		"plan workspace cannot be manually archived in its current state",
	)
	ErrPlanWorkspaceInvalidListStatus = errors.New("status must be current or archived")
)

// PlanWorkspaceArchiveView is the JSON shape returned by archive/list APIs so
// SSR/signals can flip Archived chrome without a full plan-home redesign.
type PlanWorkspaceArchiveView struct {
	PlanDirRel              string     `json:"plan_dir_rel"`
	ProjectID               string     `json:"project_id"`
	PlanDir                 string     `json:"plan_dir"`
	Label                   string     `json:"label"`
	ArtifactUpdatedAt       time.Time  `json:"artifact_updated_at"`
	QrspiLifecycle          string     `json:"qrspi_lifecycle"`
	QrspiLifecycleUpdatedAt *time.Time `json:"qrspi_lifecycle_updated_at,omitempty"`
	QrspiClosedReason       string     `json:"qrspi_closed_reason,omitempty"`
	DiscoveredAt            time.Time  `json:"discovered_at"`
	LastDiscoveredAt        time.Time  `json:"last_discovered_at"`
	ArchivedAt              *time.Time `json:"archived_at"`
	ArchiveReason           string     `json:"archive_reason"`
	ArchivedByEmail         string     `json:"archived_by_email"`
}

type PlanWorkspaceListResult struct {
	Status string                     `json:"status"`
	Plans  []PlanWorkspaceArchiveView `json:"plans"`
}

func planWorkspaceArchiveView(row db.PlanWorkspace) PlanWorkspaceArchiveView {
	view := PlanWorkspaceArchiveView{
		PlanDirRel:        row.PlanDirRel,
		ProjectID:         row.ProjectID,
		PlanDir:           row.PlanDir,
		Label:             row.Label,
		ArtifactUpdatedAt: row.ArtifactUpdatedAt,
		QrspiLifecycle:    row.QrspiLifecycle,
		QrspiClosedReason: row.QrspiClosedReason,
		DiscoveredAt:      row.DiscoveredAt,
		LastDiscoveredAt:  row.LastDiscoveredAt,
		ArchiveReason:     row.ArchiveReason,
		ArchivedByEmail:   row.ArchivedByEmail,
	}
	if row.QrspiLifecycleUpdatedAt.Valid {
		t := row.QrspiLifecycleUpdatedAt.Time
		view.QrspiLifecycleUpdatedAt = &t
	}
	if row.ArchivedAt.Valid {
		t := row.ArchivedAt.Time
		view.ArchivedAt = &t
	}
	return view
}

func (s *Service) ManualArchivePlanWorkspace(
	ctx context.Context,
	planDirRel string,
	actorEmail string,
) (PlanWorkspaceArchiveView, error) {
	planDirRel = strings.TrimSpace(planDirRel)
	actorEmail = strings.TrimSpace(actorEmail)
	if planDirRel == "" {
		return PlanWorkspaceArchiveView{}, fmt.Errorf("plan_dir_rel is required")
	}
	if actorEmail == "" {
		return PlanWorkspaceArchiveView{}, fmt.Errorf("actor email is required")
	}

	row, err := s.queries.ManualArchivePlanWorkspace(ctx, db.ManualArchivePlanWorkspaceParams{
		PlanDirRel:      planDirRel,
		ArchivedByEmail: actorEmail,
	})
	if err == nil {
		return planWorkspaceArchiveView(row), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PlanWorkspaceArchiveView{}, err
	}

	existing, getErr := s.queries.GetPlanWorkspace(ctx, planDirRel)
	if errors.Is(getErr, sql.ErrNoRows) {
		return PlanWorkspaceArchiveView{}, ErrPlanWorkspaceNotFound
	}
	if getErr != nil {
		return PlanWorkspaceArchiveView{}, getErr
	}
	if existing.ArchiveReason == planArchiveReasonManual && existing.ArchivedAt.Valid {
		// Idempotent path should have matched the UPDATE; treat as success.
		return planWorkspaceArchiveView(existing), nil
	}
	return PlanWorkspaceArchiveView{}, ErrPlanWorkspaceArchiveConflict
}

func (s *Service) UnarchiveManualPlanWorkspace(
	ctx context.Context,
	planDirRel string,
) (PlanWorkspaceArchiveView, error) {
	planDirRel = strings.TrimSpace(planDirRel)
	if planDirRel == "" {
		return PlanWorkspaceArchiveView{}, fmt.Errorf("plan_dir_rel is required")
	}

	row, err := s.queries.UnarchiveManualPlanWorkspace(ctx, planDirRel)
	if err == nil {
		return planWorkspaceArchiveView(row), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PlanWorkspaceArchiveView{}, err
	}

	existing, getErr := s.queries.GetPlanWorkspace(ctx, planDirRel)
	if errors.Is(getErr, sql.ErrNoRows) {
		return PlanWorkspaceArchiveView{}, ErrPlanWorkspaceNotFound
	}
	if getErr != nil {
		return PlanWorkspaceArchiveView{}, getErr
	}
	if existing.ArchiveReason == planArchiveReasonMissingFromDisk {
		return PlanWorkspaceArchiveView{}, ErrPlanWorkspaceMissingFromDiskArchive
	}
	return PlanWorkspaceArchiveView{}, ErrPlanWorkspaceNotManuallyArchived
}

func (s *Service) ListPlanWorkspacesByStatus(
	ctx context.Context,
	status string,
	projectID string,
) (PlanWorkspaceListResult, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = planListStatusCurrent
	}
	projectID = strings.TrimSpace(projectID)

	var (
		rows []db.PlanWorkspace
		err  error
	)
	switch status {
	case planListStatusCurrent:
		rows, err = s.queries.ListCurrentPlanWorkspaces(ctx, projectID)
	case planListStatusArchived:
		rows, err = s.queries.ListManualArchivedPlanWorkspaces(ctx, projectID)
	default:
		return PlanWorkspaceListResult{}, ErrPlanWorkspaceInvalidListStatus
	}
	if err != nil {
		return PlanWorkspaceListResult{}, err
	}

	plans := make([]PlanWorkspaceArchiveView, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, planWorkspaceArchiveView(row))
	}
	return PlanWorkspaceListResult{Status: status, Plans: plans}, nil
}
