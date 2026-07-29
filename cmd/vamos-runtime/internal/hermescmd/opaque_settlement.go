package hermescmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const opaqueSettlementKind = "pi_assistant_settlement"

// OpaqueSettlementEnvelope is the versioned, structural envelope written by
// Pi. Its response and fences are evidence only: Hermes never parses them.
type OpaqueSettlementEnvelope struct {
	Version          int                      `json:"version"`
	Kind             string                   `json:"kind"`
	Session          string                   `json:"session"`
	Plan             string                   `json:"plan"`
	ManagerThread    string                   `json:"manager_thread"`
	AssistantEntryID string                   `json:"assistant_entry_id"`
	SettledAt        time.Time                `json:"settled_at"`
	RawResponse      string                   `json:"raw_response"`
	FencedYAMLBlocks *[]OpaqueSettlementFence `json:"fenced_yaml_blocks,omitempty"`
}

// OpaqueSettlementPending retains the exact published bytes for recovery.
// BytesBase64 is deliberately the original file bytes encoded as base64, not a
// Go re-serialization of Envelope.
type OpaqueSettlementPending struct {
	Envelope    OpaqueSettlementEnvelope
	BytesBase64 string
}

// DecodeOpaqueSettlementEnvelope checks the versioned envelope without
// interpreting the opaque fence contents.
func DecodeOpaqueSettlementEnvelope(data []byte) (OpaqueSettlementEnvelope, error) {
	var envelope OpaqueSettlementEnvelope
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return OpaqueSettlementEnvelope{}, fmt.Errorf("decode opaque settlement: %w", err)
	}
	if raw, ok := fields["raw_response"]; !ok || len(raw) == 0 || raw[0] != '"' {
		return OpaqueSettlementEnvelope{}, errors.New(
			"opaque settlement raw_response is required",
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return OpaqueSettlementEnvelope{}, fmt.Errorf("decode opaque settlement: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return OpaqueSettlementEnvelope{}, errors.New(
				"opaque settlement has multiple JSON values",
			)
		}
		return OpaqueSettlementEnvelope{}, fmt.Errorf(
			"decode opaque settlement trailing data: %w",
			err,
		)
	}
	if envelope.Version != 1 {
		return OpaqueSettlementEnvelope{}, fmt.Errorf(
			"unsupported opaque settlement version %d",
			envelope.Version,
		)
	}
	if envelope.Kind != opaqueSettlementKind {
		return OpaqueSettlementEnvelope{}, fmt.Errorf(
			"unsupported opaque settlement kind %q",
			envelope.Kind,
		)
	}
	for label, value := range map[string]string{
		"session": envelope.Session, "manager thread": envelope.ManagerThread, "assistant entry": envelope.AssistantEntryID,
	} {
		if err := ValidateSafeComponent(value); err != nil {
			return OpaqueSettlementEnvelope{}, fmt.Errorf(
				"opaque settlement %s: %w",
				label,
				err,
			)
		}
	}
	if err := validateThoughtsRelativePlan(envelope.Plan); err != nil {
		return OpaqueSettlementEnvelope{}, err
	}
	if envelope.SettledAt.IsZero() {
		return OpaqueSettlementEnvelope{}, errors.New(
			"opaque settlement settled_at is required",
		)
	}
	if envelope.FencedYAMLBlocks != nil {
		for _, block := range *envelope.FencedYAMLBlocks {
			if block.Language == "" {
				return OpaqueSettlementEnvelope{}, errors.New(
					"opaque settlement fence language is required",
				)
			}
		}
	}
	return envelope, nil
}

// WriteOpaqueSettlement publishes Pi-produced JSON without re-encoding it.
// Equal-byte retries are idempotent; different bytes for the same immutable
// identity are rejected.
func WriteOpaqueSettlement(
	planDir, sessionID, finalEntryID string,
	data []byte,
) (string, error) {
	path, err := SettlementPath(planDir, sessionID, finalEntryID)
	if err != nil {
		return "", err
	}
	envelope, err := DecodeOpaqueSettlementEnvelope(data)
	if err != nil {
		return "", err
	}
	if envelope.Session != sessionID || envelope.AssistantEntryID != finalEntryID ||
		envelope.Plan != thoughtsRelative(planDir) {
		return "", errors.New(
			"opaque settlement payload does not match its immutable path identity",
		)
	}
	return writeOpaqueSettlementBytes(planDir, path, data)
}

// ReadOpaqueSettlement validates a contained settlement and returns its exact
// bytes as base64 for recovery callers that must not reconstruct JSON bytes.
func ReadOpaqueSettlement(
	planDir, sessionID, finalEntryID string,
) (OpaqueSettlementPending, error) {
	path, err := SettlementPath(planDir, sessionID, finalEntryID)
	if err != nil {
		return OpaqueSettlementPending{}, err
	}
	data, err := readContainedOpaqueSettlement(planDir, path)
	if err != nil {
		return OpaqueSettlementPending{}, err
	}
	envelope, err := DecodeOpaqueSettlementEnvelope(data)
	if err != nil {
		return OpaqueSettlementPending{}, err
	}
	if envelope.Session != sessionID || envelope.AssistantEntryID != finalEntryID ||
		envelope.Plan != thoughtsRelative(planDir) {
		return OpaqueSettlementPending{}, errors.New(
			"opaque settlement payload does not match its immutable path identity",
		)
	}
	return OpaqueSettlementPending{
		Envelope:    envelope,
		BytesBase64: base64.StdEncoding.EncodeToString(data),
	}, nil
}

func readContainedOpaqueSettlement(planDir, path string) ([]byte, error) {
	resolvedPlan, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return nil, err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !pathWithinPlan(resolvedPath, resolvedPlan) {
		return nil, errors.New("opaque settlement read escapes plan directory")
	}
	return os.ReadFile(resolvedPath)
}

func writeOpaqueSettlementBytes(planDir, path string, data []byte) (string, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	resolvedPlan, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return "", err
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	if !pathWithinPlan(resolvedDirectory, resolvedPlan) {
		return "", errors.New("opaque settlement path escapes plan directory")
	}
	temp, err := os.CreateTemp(directory, ".settlement-*")
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Link(tempName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return "", statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return "", fmt.Errorf(
					"opaque settlement identity is a symlink at %q",
					path,
				)
			}
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return "", readErr
			}
			if bytes.Equal(existing, data) {
				return path, nil
			}
			return "", fmt.Errorf(
				"immutable opaque settlement identity conflict at %q",
				path,
			)
		}
		return "", err
	}
	if err := syncDirectory(directory); err != nil {
		_ = os.Remove(path)
		_ = syncDirectory(directory)
		return "", err
	}
	if err := os.Remove(tempName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	_ = syncDirectory(directory)
	return path, nil
}
