package agentchat

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// OpaqueSettlementAdmissionStore holds the server-owned discovery authority.
// Plan-local Pi artifacts are evidence only and cannot manufacture admission.
type OpaqueSettlementAdmissionStore interface {
	Admit(context.Context, OpaqueSettlementDeliveryRequest, []byte) error
	Require(context.Context, OpaqueSettlementDeliveryRequest, []byte) error
}

type SQLOpaqueSettlementAdmissionStore struct{ DB *sql.DB }

func (s SQLOpaqueSettlementAdmissionStore) Admit(
	ctx context.Context,
	request OpaqueSettlementDeliveryRequest,
	raw []byte,
) error {
	if s.DB == nil {
		return errors.New("opaque settlement admission store is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO opaque_settlement_admissions (
	delivery_id, plan, manager_thread, session, final_entry_id, settlement_bytes
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (delivery_id) DO NOTHING`,
		request.DeliveryID, request.Plan, request.ManagerThread,
		request.Session, request.FinalEntryID, raw,
	)
	if err != nil {
		return fmt.Errorf("record opaque settlement admission: %w", err)
	}
	return s.Require(ctx, request, raw)
}

func (s SQLOpaqueSettlementAdmissionStore) Require(
	ctx context.Context,
	request OpaqueSettlementDeliveryRequest,
	raw []byte,
) error {
	if s.DB == nil {
		return errors.New("opaque settlement admission store is nil")
	}
	var storedPlan, storedThread, storedSession, storedEntry string
	var storedRaw []byte
	err := s.DB.QueryRowContext(ctx, `
SELECT plan, manager_thread, session, final_entry_id, settlement_bytes
FROM opaque_settlement_admissions
WHERE delivery_id = ?`, request.DeliveryID).Scan(
		&storedPlan, &storedThread, &storedSession, &storedEntry, &storedRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("server-owned settlement discovery admission is required")
	}
	if err != nil {
		return fmt.Errorf("read opaque settlement admission: %w", err)
	}
	if storedPlan != request.Plan || storedThread != request.ManagerThread ||
		storedSession != request.Session || storedEntry != request.FinalEntryID ||
		!bytes.Equal(storedRaw, raw) {
		return errors.New("immutable opaque settlement identity conflict")
	}
	return nil
}

func (s *Service) requireOpaqueSettlementAdmission(
	ctx context.Context,
	request OpaqueSettlementDeliveryRequest,
	raw []byte,
) error {
	store := s.opaqueSettlementAdmissions
	if store == nil {
		store = SQLOpaqueSettlementAdmissionStore{DB: s.db}
	}
	return store.Require(ctx, request, raw)
}

func NewOpaqueSettlementDeliveryActivities(
	thoughtsRoot string,
	database *sql.DB,
	receiver OpaqueSettlementReceiver,
) *OpaqueSettlementDeliveryActivities {
	return &OpaqueSettlementDeliveryActivities{
		ThoughtsRoot: thoughtsRoot,
		PlanSource:   SQLBoundedOpaqueSettlementPlanSource{DB: database},
		Admissions:   SQLOpaqueSettlementAdmissionStore{DB: database},
		Receiver:     receiver,
	}
}

type SQLBoundedOpaqueSettlementPlanSource struct{ DB *sql.DB }

func (s SQLBoundedOpaqueSettlementPlanSource) Scan(
	ctx context.Context,
) ([]DiscoveredPlanWorkspace, error) {
	if s.DB == nil {
		return nil, errors.New("opaque settlement plan projection is nil")
	}
	// Fetch at most the cap plus one sentinel, before resolving any filesystem
	// path. The authoritative workspace projection is server-owned.
	rows, err := s.DB.QueryContext(ctx, `
SELECT plan_dir, plan_dir_rel
FROM plan_workspaces
WHERE archived_at IS NULL AND qrspi_lifecycle NOT IN ('merged', 'closed')
ORDER BY plan_dir_rel
LIMIT ?`, opaqueSettlementScanLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list opaque settlement projected plans: %w", err)
	}
	defer rows.Close()
	plans := make([]DiscoveredPlanWorkspace, 0, opaqueSettlementScanLimit+1)
	for rows.Next() {
		var plan DiscoveredPlanWorkspace
		if err := rows.Scan(&plan.PlanDir, &plan.PlanDirRel); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(plans) > opaqueSettlementScanLimit {
		return nil, fmt.Errorf(
			"opaque settlement plan discovery exceeds limit %d",
			opaqueSettlementScanLimit,
		)
	}
	return plans, nil
}
