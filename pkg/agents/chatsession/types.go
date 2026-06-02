package chatsession

import "encoding/json"

type CommandType string
type CommandStatus string
type EventType string
type AgentSessionLinkMode string
type AgentSurfacePermissionMode string
type ChatSessionTopologyKind string

const (
	TopologyRoot ChatSessionTopologyKind = "root"
	TopologyFork ChatSessionTopologyKind = "fork"

	CommandSubmitted CommandStatus = "submitted"
	CommandAccepted  CommandStatus = "accepted"
	CommandRejected  CommandStatus = "rejected"
	CommandApplied   CommandStatus = "applied"
	CommandFailed    CommandStatus = "failed"

	EventCommandSubmitted    EventType = "command.submitted"
	EventCommandAccepted     EventType = "command.accepted"
	EventCommandRejected     EventType = "command.rejected"
	EventCommandApplied      EventType = "command.applied"
	EventCommandFailed       EventType = "command.failed"
	EventMessageCreated      EventType = "message.created"
	EventMessageStarted      EventType = "message.started"
	EventMessageCheckpointed EventType = "message.checkpointed"
	EventMessageCompleted    EventType = "message.completed"
	EventRunStarted          EventType = "run.started"
	EventRunProgress         EventType = "run.progress"
	EventRunOutputDelta      EventType = "run.output.delta"
	EventRunCompleted        EventType = "run.completed"
	EventRunFailed           EventType = "run.failed"
	EventToolStarted         EventType = "tool.started"
	EventToolUpdated         EventType = "tool.updated"
	EventToolCompleted       EventType = "tool.completed"
	EventToolFailed          EventType = "tool.failed"
	EventFileWritten         EventType = "file.written"
	EventFileEdited          EventType = "file.edited"
	EventQRSPIResult         EventType = "qrspi.result"
	EventQRSPINextStarted    EventType = "qrspi.next_started"
	EventQRSPIWaitingHuman   EventType = "qrspi.waiting_human"
	EventQRSPIBlocked        EventType = "qrspi.blocked"
	EventRunCancelRequest    EventType = "run.cancel_requested"
	EventForkCreated         EventType = "session.fork_created"
	EventBaselineCopied      EventType = "session.baseline_copied"
	EventPromoted            EventType = "session.promoted_to_active_path"
	EventSurfaceAttached     EventType = "surface.attached"
	EventSurfaceDetached     EventType = "surface.detached"
	EventControlTransferred  EventType = "control.transferred"
	EventPiSessionDiscovered EventType = "pi.session.discovered"
	EventPiSessionAdopted    EventType = "pi.session.adopted"

	LinkImported    AgentSessionLinkMode = "imported"
	LinkObserved    AgentSessionLinkMode = "observed"
	LinkInteractive AgentSessionLinkMode = "interactive"
	LinkControlled  AgentSessionLinkMode = "controlled"
	LinkHandoff     AgentSessionLinkMode = "handoff"

	PermissionObserve AgentSurfacePermissionMode = "observe"
	PermissionSubmit  AgentSurfacePermissionMode = "submit"
	PermissionControl AgentSurfacePermissionMode = "control"
	PermissionOwn     AgentSurfacePermissionMode = "own"
)

type CreateSessionInput struct {
	WorkspaceID     string
	ActorEmail      string
	BranchID        string
	ParentSessionID string
	WorkflowID      string
	WorkflowNodeID  string
	ForkedFromSeq   int64
	WorkflowAttempt int
	TopologyKind    ChatSessionTopologyKind
}

type SubmitCommandInput struct {
	WorkspaceID    string
	SessionID      string
	IdempotencyKey string
	ActorEmail     string
	Type           CommandType
	PayloadJSON    json.RawMessage
	AnnotationIDs  []string
}

type CommandOutcome struct {
	CommandID   string
	Status      CommandStatus
	Events      []ChatEvent
	OutcomeJSON json.RawMessage
}

type AppendEventInput struct {
	SessionID          string
	EventType          EventType
	ActorParticipantID string
	CommandID          string
	RunID              string
	PayloadJSON        json.RawMessage
}

type ChatEvent struct {
	SessionID          string
	Seq                int64
	EventType          EventType
	ActorParticipantID string
	CommandID          string
	RunID              string
	PayloadJSON        json.RawMessage
}

type ExternalAgentSessionLink struct {
	ChatSessionID          string
	ExternalAgentSessionID string
	Mode                   AgentSessionLinkMode
}

type AgentSurfaceAttachment struct {
	ChatSessionID string
	RunID         string
	SurfaceKind   string
	SurfaceID     string
	UserEmail     string
	Permission    AgentSurfacePermissionMode
}
