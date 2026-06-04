package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/auth/agentbrowser"
	db "github.com/CoreyCole/vamos/pkg/db"
)

type SQLMachineCredentialStore struct {
	queries *db.Queries
	now     func() time.Time
}

func NewSQLMachineCredentialStore(queries *db.Queries) *SQLMachineCredentialStore {
	return &SQLMachineCredentialStore{queries: queries, now: time.Now}
}

func (s *SQLMachineCredentialStore) Create(ctx context.Context, input CreateMachineCredentialInput) (CreatedMachineCredential, error) {
	if s == nil || s.queries == nil {
		return CreatedMachineCredential{}, errors.New("machine credential queries are required")
	}
	secret, err := NewMachineSecret()
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	hash, err := HashMachineSecret(secret)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	id, err := newMachineCredentialID()
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	actorEmails, err := encodeMachineCredentialStrings(input.AllowedActorEmails)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	slugs, err := encodeMachineCredentialStrings(input.AllowedSlugs)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	purposes, err := encodeMachineCredentialPurposes(input.AllowedPurposes)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	now := s.nowTime()
	row, err := s.queries.CreateMachineCredential(ctx, db.CreateMachineCredentialParams{
		ID:                     id,
		Name:                   strings.TrimSpace(input.Name),
		SecretHash:             hash,
		DefaultActorEmail:      strings.TrimSpace(input.DefaultActorEmail),
		AllowedActorEmailsJson: actorEmails,
		AllowedSlugsJson:       slugs,
		AllowedPurposesJson:    purposes,
		ExpiresAt:              timePtrToNullTime(input.ExpiresAt),
		CreatedAt:              now,
	})
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	credential, err := machineCredentialFromRow(row)
	if err != nil {
		return CreatedMachineCredential{}, err
	}
	return CreatedMachineCredential{Credential: credential, Secret: secret}, nil
}

func (s *SQLMachineCredentialStore) Authenticate(ctx context.Context, keyID, secret string) (MachineCredential, error) {
	if s == nil || s.queries == nil {
		return MachineCredential{}, errors.New("machine credential queries are required")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" || strings.TrimSpace(secret) == "" {
		return MachineCredential{}, errors.New("machine credential id and secret are required")
	}
	row, err := s.queries.GetMachineCredential(ctx, keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MachineCredential{}, errors.New("invalid machine credential")
		}
		return MachineCredential{}, err
	}
	credential, err := machineCredentialFromRow(row)
	if err != nil {
		return MachineCredential{}, err
	}
	if !VerifyMachineSecret(secret, credential.SecretHash) {
		return MachineCredential{}, errors.New("invalid machine credential")
	}
	if credential.RevokedAt != nil {
		return MachineCredential{}, errors.New("machine credential revoked")
	}
	if credential.ExpiresAt != nil && !credential.ExpiresAt.After(s.nowTime()) {
		return MachineCredential{}, errors.New("machine credential expired")
	}
	now := s.nowTime()
	if err := s.queries.UpdateMachineCredentialLastUsed(ctx, db.UpdateMachineCredentialLastUsedParams{LastUsedAt: sql.NullTime{Time: now, Valid: true}, ID: keyID}); err != nil {
		return MachineCredential{}, err
	}
	credential.LastUsedAt = &now
	return cloneMachineCredential(credential), nil
}

func (s *SQLMachineCredentialStore) Revoke(ctx context.Context, keyID string) error {
	if s == nil || s.queries == nil {
		return errors.New("machine credential queries are required")
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return errors.New("machine credential id is required")
	}
	rows, err := s.queries.RevokeMachineCredential(ctx, db.RevokeMachineCredentialParams{RevokedAt: sql.NullTime{Time: s.nowTime(), Valid: true}, ID: keyID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("machine credential not found")
	}
	return nil
}

func (s *SQLMachineCredentialStore) List(ctx context.Context) ([]MachineCredential, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("machine credential queries are required")
	}
	rows, err := s.queries.ListMachineCredentials(ctx)
	if err != nil {
		return nil, err
	}
	credentials := make([]MachineCredential, 0, len(rows))
	for _, row := range rows {
		credential, err := machineCredentialFromRow(row)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (s *SQLMachineCredentialStore) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func machineCredentialFromRow(row db.MachineCredential) (MachineCredential, error) {
	actorEmails, err := decodeMachineCredentialStrings(row.AllowedActorEmailsJson)
	if err != nil {
		return MachineCredential{}, fmt.Errorf("decode allowed actor emails: %w", err)
	}
	slugs, err := decodeMachineCredentialStrings(row.AllowedSlugsJson)
	if err != nil {
		return MachineCredential{}, fmt.Errorf("decode allowed slugs: %w", err)
	}
	purposes, err := decodeMachineCredentialPurposes(row.AllowedPurposesJson)
	if err != nil {
		return MachineCredential{}, fmt.Errorf("decode allowed purposes: %w", err)
	}
	return MachineCredential{
		ID:                 row.ID,
		Name:               row.Name,
		SecretHash:         append([]byte(nil), row.SecretHash...),
		DefaultActorEmail:  row.DefaultActorEmail,
		AllowedActorEmails: actorEmails,
		AllowedSlugs:       slugs,
		AllowedPurposes:    purposes,
		ExpiresAt:          nullTimePtr(row.ExpiresAt),
		RevokedAt:          nullTimePtr(row.RevokedAt),
		CreatedAt:          row.CreatedAt,
		LastUsedAt:         nullTimePtr(row.LastUsedAt),
	}, nil
}

func encodeMachineCredentialStrings(values []string) (string, error) {
	normalized := normalizeStringList(values)
	if normalized == nil {
		normalized = []string{}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeMachineCredentialStrings(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return normalizeStringList(values), nil
}

func encodeMachineCredentialPurposes(values []agentbrowser.Purpose) (string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		purpose := strings.TrimSpace(string(value))
		if purpose != "" {
			normalized = append(normalized, purpose)
		}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeMachineCredentialPurposes(raw string) ([]agentbrowser.Purpose, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	purposes := make([]agentbrowser.Purpose, 0, len(values))
	for _, value := range values {
		purpose := agentbrowser.Purpose(strings.TrimSpace(value))
		if purpose != "" {
			purposes = append(purposes, purpose)
		}
	}
	return purposes, nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func timePtrToNullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}
