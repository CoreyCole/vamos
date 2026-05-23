package chatsession

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/google/uuid"
)

func (s *Service) SubmitCommand(ctx context.Context, input SubmitCommandInput) (CommandOutcome, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommandOutcome{}, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)

	command, err := q.CreateChatCommand(ctx, db.CreateChatCommandParams{
		ID:             uuid.NewString(),
		SessionID:      strings.TrimSpace(input.SessionID),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		CommandType:    string(input.Type),
		Status:         string(CommandSubmitted),
		ActorEmail:     strings.TrimSpace(input.ActorEmail),
		PayloadJson:    string(defaultJSON(input.PayloadJSON)),
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			existing, getErr := q.GetChatCommandByIdempotencyKey(ctx, db.GetChatCommandByIdempotencyKeyParams{
				SessionID:      strings.TrimSpace(input.SessionID),
				IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
			})
			if getErr != nil {
				return CommandOutcome{}, getErr
			}
			return outcomeFromCommand(existing), nil
		}
		return CommandOutcome{}, err
	}

	submitted, err := appendEventTx(ctx, q, AppendEventInput{
		SessionID:   input.SessionID,
		EventType:   EventCommandSubmitted,
		CommandID:   command.ID,
		PayloadJSON: commandPayload(command),
	})
	if err != nil {
		return CommandOutcome{}, err
	}
	acceptedCommand, err := q.UpdateChatCommandStatus(ctx, db.UpdateChatCommandStatusParams{
		ID:          command.ID,
		Status:      string(CommandAccepted),
		OutcomeJson: nullString(`{"status":"accepted"}`),
	})
	if err != nil {
		return CommandOutcome{}, err
	}
	accepted, err := appendEventTx(ctx, q, AppendEventInput{
		SessionID:   input.SessionID,
		EventType:   EventCommandAccepted,
		CommandID:   command.ID,
		PayloadJSON: commandPayload(acceptedCommand),
	})
	if err != nil {
		return CommandOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommandOutcome{}, err
	}
	return CommandOutcome{
		CommandID:   command.ID,
		Status:      CommandAccepted,
		Events:      []ChatEvent{submitted, accepted},
		OutcomeJSON: json.RawMessage(`{"status":"accepted"}`),
	}, nil
}

func (s *Service) AcceptCommand(ctx context.Context, id string, outcomeJSON json.RawMessage) (CommandOutcome, error) {
	return s.transitionCommand(ctx, id, CommandAccepted, EventCommandAccepted, outcomeJSON)
}

func (s *Service) RejectCommand(ctx context.Context, id string, outcomeJSON json.RawMessage) (CommandOutcome, error) {
	return s.transitionCommand(ctx, id, CommandRejected, EventCommandRejected, outcomeJSON)
}

func (s *Service) ApplyCommand(ctx context.Context, id string, outcomeJSON json.RawMessage) (CommandOutcome, error) {
	return s.transitionCommand(ctx, id, CommandApplied, EventCommandApplied, outcomeJSON)
}

func (s *Service) FailCommand(ctx context.Context, id string, outcomeJSON json.RawMessage) (CommandOutcome, error) {
	return s.transitionCommand(ctx, id, CommandFailed, EventCommandFailed, outcomeJSON)
}

func (s *Service) transitionCommand(ctx context.Context, id string, status CommandStatus, eventType EventType, outcomeJSON json.RawMessage) (CommandOutcome, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommandOutcome{}, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	command, err := q.UpdateChatCommandStatus(ctx, db.UpdateChatCommandStatusParams{
		ID:          strings.TrimSpace(id),
		Status:      string(status),
		OutcomeJson: nullString(string(defaultJSON(outcomeJSON))),
	})
	if err != nil {
		return CommandOutcome{}, err
	}
	event, err := appendEventTx(ctx, q, AppendEventInput{
		SessionID:   command.SessionID,
		EventType:   eventType,
		CommandID:   command.ID,
		PayloadJSON: commandPayload(command),
	})
	if err != nil {
		return CommandOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommandOutcome{}, err
	}
	return CommandOutcome{CommandID: command.ID, Status: status, Events: []ChatEvent{event}, OutcomeJSON: defaultJSON(outcomeJSON)}, nil
}

func commandPayload(command db.ChatSessionCommand) json.RawMessage {
	return marshalPayload(map[string]any{
		"command_id":      command.ID,
		"command_type":    command.CommandType,
		"status":          command.Status,
		"actor_email":     command.ActorEmail,
		"payload":         json.RawMessage(command.PayloadJson),
		"outcome":         nullableJSON(command.OutcomeJson.String, command.OutcomeJson.Valid),
		"idempotency_key": command.IdempotencyKey,
	})
}

func nullableJSON(value string, valid bool) any {
	if !valid || strings.TrimSpace(value) == "" {
		return nil
	}
	return json.RawMessage(value)
}

func outcomeFromCommand(command db.ChatSessionCommand) CommandOutcome {
	return CommandOutcome{
		CommandID:   command.ID,
		Status:      CommandStatus(command.Status),
		OutcomeJSON: json.RawMessage(command.OutcomeJson.String),
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint failed")
}
