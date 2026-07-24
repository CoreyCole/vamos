package qrspicmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ClaimOperation identifies the one shared mutation a manager is attempting.
// Manager registration is deliberately not a claim: pre-workspace managers may
// coexist for the same plan.
type ClaimOperation string

const (
	ClaimRecordConsume           ClaimOperation = "record-consume"
	ClaimManagerAttach           ClaimOperation = "manager-attach"
	ClaimActiveChildMutate       ClaimOperation = "active-child-mutate"
	ClaimGraphTransition         ClaimOperation = "graph-transition"
	ClaimImplementationWorkspace ClaimOperation = "implementation-workspace"
)

type ManagerClaim struct {
	ID                      string         `json:"id"`
	Key                     LockKey        `json:"key"`
	Operation               ClaimOperation `json:"operation"`
	HolderRunID             string         `json:"holderRunId"`
	ExpectedRecordID        string         `json:"expectedRecordId,omitempty"`
	ExpectedChildID         string         `json:"expectedChildId,omitempty"`
	ExpectedChildGeneration int            `json:"expectedChildGeneration,omitempty"`
	ExpectedTransitionEpoch int            `json:"expectedTransitionEpoch,omitempty"`
	AcquiredAt              time.Time      `json:"acquiredAt"`
	ExpiresAt               time.Time      `json:"expiresAt"`
}

type ClaimRequest struct {
	Key                     LockKey
	Operation               ClaimOperation
	HolderRunID             string
	ExpectedRecordID        string
	ExpectedChildID         string
	ExpectedChildGeneration int
	ExpectedTransitionEpoch int
	TTL                     time.Duration
}

type ClaimConflictError struct{ Existing ManagerClaim }

func (e ClaimConflictError) Error() string {
	return fmt.Sprintf(
		"q-manager %s claim held by %s",
		e.Existing.Operation,
		e.Existing.HolderRunID,
	)
}

func ClaimPath(root string, key LockKey, operation ClaimOperation) string {
	return filepath.Join(root, keyID(key), "claims", string(operation)+".json")
}

func ClaimAuditPath(root string, key LockKey) string {
	return filepath.Join(root, keyID(key), "recovery-audit.jsonl")
}

func FindClaim(root string, key LockKey, claimID string) (ManagerClaim, error) {
	if strings.TrimSpace(claimID) == "" {
		return ManagerClaim{}, errors.New("claim is required")
	}
	entries, err := os.ReadDir(filepath.Join(root, keyID(key), "claims"))
	if err != nil {
		return ManagerClaim{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, keyID(key), "claims", entry.Name()))
		if err != nil {
			return ManagerClaim{}, err
		}
		var claim ManagerClaim
		if len(strings.TrimSpace(string(data))) == 0 ||
			json.Unmarshal(data, &claim) != nil {
			continue
		}
		if claim.ID == claimID {
			return claim, nil
		}
	}
	return ManagerClaim{}, errors.New("claim was not found")
}

func (s FileStateStore) AcquireClaim(
	ctx context.Context,
	req ClaimRequest,
) (ManagerClaim, error) {
	select {
	case <-ctx.Done():
		return ManagerClaim{}, ctx.Err()
	default:
	}
	if strings.TrimSpace(req.HolderRunID) == "" {
		return ManagerClaim{}, errors.New("claim holder run ID is required")
	}
	if !validClaimOperation(req.Operation) {
		return ManagerClaim{}, fmt.Errorf("invalid claim operation %q", req.Operation)
	}
	if req.TTL <= 0 {
		req.TTL = lockTTL
	}
	path := ClaimPath(s.Root, req.Key, req.Operation)
	file, err := openClaimFile(path)
	if err != nil {
		return ManagerClaim{}, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return ManagerClaim{}, err
	}
	defer syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_UN,
	) //nolint:errcheck // unlock failure cannot be recovered usefully

	now := s.now()
	existing, err := readClaim(file)
	if err != nil {
		return ManagerClaim{}, err
	}
	if existing != nil && existing.ExpiresAt.After(now) &&
		existing.HolderRunID != req.HolderRunID {
		return ManagerClaim{}, ClaimConflictError{Existing: *existing}
	}
	id, err := newClaimID()
	if err != nil {
		return ManagerClaim{}, err
	}
	claim := ManagerClaim{
		ID:                      id,
		Key:                     req.Key,
		Operation:               req.Operation,
		HolderRunID:             req.HolderRunID,
		ExpectedRecordID:        req.ExpectedRecordID,
		ExpectedChildID:         req.ExpectedChildID,
		ExpectedChildGeneration: req.ExpectedChildGeneration,
		ExpectedTransitionEpoch: req.ExpectedTransitionEpoch,
		AcquiredAt:              now.UTC(),
		ExpiresAt:               now.Add(req.TTL).UTC(),
	}
	if err := writeLockedJSON(file, claim); err != nil {
		return ManagerClaim{}, err
	}
	return claim, nil
}

func (s FileStateStore) ReleaseClaim(ctx context.Context, claim ManagerClaim) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if claim.ID == "" || !validClaimOperation(claim.Operation) {
		return errors.New("valid claim ID and operation are required")
	}
	file, err := openClaimFile(ClaimPath(s.Root, claim.Key, claim.Operation))
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_UN,
	) //nolint:errcheck // unlock failure cannot be recovered usefully

	existing, err := readClaim(file)
	if err != nil {
		return err
	}
	if existing == nil || existing.ID != claim.ID ||
		existing.HolderRunID != claim.HolderRunID {
		return errors.New("claim is no longer held by this manager run")
	}
	return writeLockedJSON(file, map[string]any{})
}

// RecoverClaim only reclaims an expired claim. A current holder is deliberately
// preserved; callers must inspect/attach rather than guessing ownership.
func (s FileStateStore) RecoverClaim(
	ctx context.Context,
	key LockKey,
	operation ClaimOperation,
	claimID, newHolder string,
) (ManagerClaim, error) {
	if strings.TrimSpace(newHolder) == "" {
		return ManagerClaim{}, errors.New("recovery holder run ID is required")
	}
	if !validClaimOperation(operation) {
		return ManagerClaim{}, fmt.Errorf("invalid claim operation %q", operation)
	}
	path := ClaimPath(s.Root, key, operation)
	file, err := openClaimFile(path)
	if err != nil {
		return ManagerClaim{}, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return ManagerClaim{}, err
	}
	defer syscall.Flock(
		int(file.Fd()),
		syscall.LOCK_UN,
	) //nolint:errcheck // unlock failure cannot be recovered usefully

	existing, err := readClaim(file)
	if err != nil {
		return ManagerClaim{}, err
	}
	if existing == nil || existing.ID != claimID {
		return ManagerClaim{}, errors.New("claim was not found")
	}
	if existing.ExpiresAt.After(s.now()) {
		return ManagerClaim{}, ClaimConflictError{Existing: *existing}
	}
	request := ClaimRequest{
		Key:         key,
		Operation:   operation,
		HolderRunID: newHolder,
		TTL:         lockTTL,
	}
	id, err := newClaimID()
	if err != nil {
		return ManagerClaim{}, err
	}
	now := s.now().UTC()
	claim := ManagerClaim{
		ID:          id,
		Key:         key,
		Operation:   operation,
		HolderRunID: newHolder,
		AcquiredAt:  now,
		ExpiresAt:   now.Add(request.TTL),
	}
	if err := writeLockedJSON(file, claim); err != nil {
		return ManagerClaim{}, err
	}
	if err := appendClaimRecoveryAudit(
		ClaimAuditPath(s.Root, key),
		*existing,
		claim,
	); err != nil {
		return ManagerClaim{}, err
	}
	return claim, nil
}

func openClaimFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
}

func readClaim(file *os.File) (*ManagerClaim, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	var claim ManagerClaim
	if err := json.NewDecoder(file).Decode(&claim); err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	if claim.ID == "" {
		return nil, nil
	}
	return &claim, nil
}

func appendClaimRecoveryAudit(path string, previous, replacement ManagerClaim) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	entry := struct {
		RecoveredAt time.Time    `json:"recoveredAt"`
		Previous    ManagerClaim `json:"previous"`
		Replacement ManagerClaim `json:"replacement"`
	}{time.Now().UTC(), previous, replacement}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}

func validClaimOperation(operation ClaimOperation) bool {
	switch operation {
	case ClaimRecordConsume,
		ClaimManagerAttach,
		ClaimActiveChildMutate,
		ClaimGraphTransition,
		ClaimImplementationWorkspace:
		return true
	default:
		return false
	}
}

func newClaimID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return "claim_" + hex.EncodeToString(data[:]), nil
}
