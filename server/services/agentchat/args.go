package agentchat

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	workspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/datastarui/utils"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

type AgentChatResponsiveLayoutArgs struct {
	HasSelectedThread bool
	Sidebar           templ.Component
	ChatHeader        templ.Component
	Messages          templ.Component
	Composer          templ.Component
	RightPane         templ.Component
}

type AttachedPath struct {
	Path     string `json:"path"`
	Basename string `json:"basename"`
}

type ThreadWorkspaceRole string

const (
	ThreadWorkspaceRolePrimary ThreadWorkspaceRole = "primary"
	ThreadWorkspaceRoleRelated ThreadWorkspaceRole = "related"
)

type ThreadWorkspaceAssociation struct {
	ThreadID    string
	WorkspaceID string
	IsPrimary   bool
	Role        ThreadWorkspaceRole
	AdoptedFrom string
	AdoptedAt   time.Time
}

type ThreadWorkspaceContext struct {
	Thread  db.AgentThread
	Primary *db.Workspace
	Related []db.Workspace
}

type NewThreadTargetKind string

const (
	NewThreadTargetPrimary  NewThreadTargetKind = "primary"
	NewThreadTargetRelated  NewThreadTargetKind = "related"
	NewThreadTargetFreeform NewThreadTargetKind = "freeform"
)

type ThreadMetadataView struct {
	ThreadID                string
	Title                   string
	URL                     string
	ProjectID               string
	ThreadCwd               string
	PiCwd                   string
	ImplementationWorkspace string
	Primary                 *ThreadWorkspaceView
	Related                 []ThreadWorkspaceView
	NewTargets              []ThreadNewTargetView
}

type ThreadWorkspaceView struct {
	WorkspaceID  string
	Label        string
	RootDocPath  string
	WorkflowType string
	Lifecycle    string
	Role         string
}

type ThreadExecutionTargetView struct {
	ThreadCwd               string
	PiCwd                   string
	ImplementationWorkspace string
	Source                  string
}

type ThreadNewTargetView struct {
	Kind        NewThreadTargetKind
	WorkspaceID string
	Label       string
	Selected    bool
}

type TranscriptMessage struct {
	DOMID                 string
	EntryID               string
	ToolCallID            string
	ToolResult            bool
	Variant               string
	Role                  string
	Title                 string
	HeaderCode            string
	HeaderSummary         string
	DetailHeader          string
	Content               string
	HTMLContent           string
	Attachments           []AttachedPath
	ShowForkForm          bool
	IsError               bool
	Collapsible           bool
	HideBodyWhenCollapsed bool
	WorkflowCard          *QRSPIWorkflowCard
	ChatSessionID         string
	ChatNodeID            string
	ChatEventSeq          int64
}

type SessionThreadSource string

const (
	SessionThreadSourceAgentChat SessionThreadSource = "agentchat"
	SessionThreadSourcePi        SessionThreadSource = "pi"
)

type SessionThreadRef struct {
	Key         string
	Source      SessionThreadSource
	ThreadID    string
	SessionPath string
	PlanDir     string
	Title       string
	Cwd         string
	UpdatedAt   time.Time
}

type ThreadSidebarThread struct {
	ID          string
	Href        string
	Title       string
	CwdLabel    string
	SourceLabel string
	IsActive    bool

	OpenPiSessionAction string
	SessionPath         string
	WorkspaceDir        string
}

type ThreadSidebarGroup struct {
	Key           string
	KindLabel     string
	Label         string
	Timestamp     string
	ThreadCount   int
	IsActive      bool
	WorkspaceHref string
	Threads       []ThreadSidebarThread
}

type ThreadSidebarArgs struct {
	Groups            []ThreadSidebarGroup
	HasSelectedThread bool
}

type PlanSidebarInput struct {
	UserEmail         string
	ProjectID         string
	ActiveWorkspaceID string
	ActiveThreadID    string
	ActivePlanDir     string
	IncludeArchived   bool
}

type PlanSidebarState struct {
	TargetID          string
	DrawerTitle       string
	ActivePlanDir     string
	ActiveWorkspaceID string
	HasSelection      bool
	Nodes             []PlanSidebarNode
}

type PlanSidebarNode struct {
	Key                  string
	PlanDir              string
	PlanDirRel           string
	Label                string
	Href                 string
	Depth                int
	Expanded             bool
	Active               bool
	PrimaryProject       string
	RelatedProjects      []string
	MatchedRole          string
	Bindings             []PlanSidebarBindingView
	DirectLatestAt       time.Time
	AggregateLatestAt    time.Time
	LatestThreadID       string
	LatestSessionID      string
	LatestSourceLabel    string
	LatestUserActivityAt time.Time
	DirectCount          int
	AggregateCount       int
	Children             []PlanSidebarNode
}

type PlanSidebarBindingView struct {
	ProjectID     string
	Role          string
	WorkspaceSlug string
	CheckoutPath  string
	URL           string
	Status        string
}

type PlanSidebarSource struct {
	PlanDir         string
	PlanDirRel      string
	ProjectID       string
	PrimaryProject  string
	RelatedProjects []string
	MatchedRole     string
	Bindings        []PlanSidebarBindingView
	WorkspaceID     string
	ThreadID        string
	SessionID       string
	Source          string
	Title           string
	UpdatedAt       time.Time
}

type AgentChatSidebarTargetArgs struct {
	ID                string
	Body              templ.Component
	DrawerTitle       string
	HasSelectedThread bool
}

type WorkspacePatchScope string

type StreamPatchScope = WorkspacePatchScope

const (
	PatchWorkspaceResource WorkspacePatchScope = WorkspacePatchScope(
		workspace.PatchResource,
	)

	// Narrow scopes kept while thread-query compatibility and the live transcript
	// fast lane remain compatible with workspace-wide patches.
	PatchWorkspaceHeader   WorkspacePatchScope = "workspace-header"
	PatchWorkspaceSidebar  WorkspacePatchScope = "workspace-sidebar"
	PatchWorkspaceMessages WorkspacePatchScope = "workspace-messages"
	PatchWorkspaceComposer WorkspacePatchScope = "workspace-composer"
	PatchStableTranscript  WorkspacePatchScope = "stable-transcript"
	PatchLiveTranscript    WorkspacePatchScope = WorkspacePatchScope(
		workspace.PatchLiveTranscript,
	)
	PatchRunSessionHeader       WorkspacePatchScope = "run-session-header"
	PatchDocTree                WorkspacePatchScope = "doc-tree"
	PatchDocContent             WorkspacePatchScope = "doc-content"
	PatchDocAnchorCallouts      WorkspacePatchScope = "doc-anchor-callouts"
	PatchWorkspaceDocComments   WorkspacePatchScope = "doc-comments"
	PatchArtifactTree                               = PatchDocTree
	PatchArtifactContent                            = PatchDocContent
	PatchArtifactAnchorCallouts                     = PatchDocAnchorCallouts
	PatchWorkspaceMinimap       WorkspacePatchScope = "workspace-minimap"
	PatchWorkflowPanel          WorkspacePatchScope = "workflow-panel"
	PatchWorkspacePageCatchup   WorkspacePatchScope = PatchWorkspaceResource

	PatchRunHeader    WorkspacePatchScope = PatchRunSessionHeader
	PatchDocPane      WorkspacePatchScope = PatchDocContent
	PatchArtifactPane                     = PatchDocPane
	PatchSidebar      WorkspacePatchScope = PatchWorkspaceSidebar
	PatchThreadPage   WorkspacePatchScope = PatchWorkspacePageCatchup
)

type WorkspaceStreamSignal struct {
	Cursor int64
	Scope  WorkspacePatchScope
}

type ThreadStreamSignal = WorkspaceStreamSignal

type PiSessionIndexRequest struct {
	UserEmail string
	Reason    string
	Force     bool
}

type PiSessionIndexResult struct {
	Indexed            int
	Imported           int
	Skipped            int
	Failed             int
	AffectedPlans      []string
	AffectedWorkspaces []string
}

type TranscriptRenderPolicy struct {
	HideThinking         bool
	HideToolCalls        bool
	HidePendingToolCalls bool
}

type LiveTranscriptView struct {
	Items []TranscriptMessage
}

type TranscriptPaneState struct {
	Cursor int64
	Stable []TranscriptMessage
	Live   LiveTranscriptView
	Policy TranscriptRenderPolicy
}

type SelectedChatAnnotation struct {
	ID       string
	Label    string
	NodeID   string
	EventSeq int64
}

type AgentChatComposerArgs struct {
	Action               string
	WorkspaceID          string
	ThreadID             string
	DocPath              string
	RunID                string
	Cwd                  string
	ModeLabel            string
	Placeholder          string
	HasThread            bool
	IncludeCwd           bool
	AttachedPaths        []AttachedPath
	SlashEndpointBase    string
	ShowCurrentDocToggle bool
	CurrentDocAttached   bool
	SelectedAnnotations  []SelectedChatAnnotation
	ThreadMetadata       ThreadMetadataView
}

type ChatMessageArgs struct {
	ID          string
	Role        string
	Content     string
	HTMLContent string
	Attachments []AttachedPath
}

type ChatMessageStreamingArgs struct {
	ID string
}

type ChatMessageDeltaArgs struct {
	ID   string
	Text string
}

type ChatMessageDeltaHTMLArgs struct {
	ID          string
	HTMLContent string
}

type ChatMessageCompleteArgs struct {
	ID          string
	Content     string
	HTMLContent string
}

type (
	ArtifactTreeNode            = DocTreeNode
	ArtifactRenderView          = DocRenderView
	ArtifactPaneState           = DocPaneState
	ArtifactSectionCommentsArgs = DocSectionCommentsArgs
)

type DocTreeNode struct {
	Path         string
	AbsolutePath string
	Name         string
	IsDir        bool
	IsExpanded   bool
	Selected     bool
	Children     []DocTreeNode
}

type DocRenderView struct {
	RootPath     string
	RelativePath string
	DisplayName  string
	HTML         string
	Sections     []markdown.Section
	Exists       bool
}

type WorkingDirCrumb struct {
	Label string
	Path  string
}

type WorkingDirectoryState struct {
	ProjectName      string
	ProjectPath      string
	AbsolutePath     string
	RelativePath     string
	CurrentTitle     string
	CurrentTimestamp string
	ResetPath        string
	Crumbs           []WorkingDirCrumb
}

type DocPaneState struct {
	WorkspaceID string
	ActiveRunID string
	RootDocPath string
	RootLabel   string
	WorkingDir  WorkingDirectoryState
	PlanLineage *PlanNode
	Tree        []DocTreeNode
	Selected    DocRenderView
	Comments    []WorkspaceDocCommentView
}

type WorkspaceDocPaneState = DocPaneState

type WorkspaceDocCommentAnchor struct {
	SelectedText string
	SectionHint  string
	HeadingHint  string
	StartLine    int
	StartColumn  int
	EndLine      int
	EndColumn    int
}

type WorkspaceDocCommentView struct {
	Comment     db.WorkspaceDocComment
	Replies     []db.WorkspaceDocCommentReply
	AnchorState string
	MatchIndex  int
}

type WorkspaceDocCommentTargetKind string

const (
	WorkspaceDocCommentTargetDocument WorkspaceDocCommentTargetKind = "document"
	WorkspaceDocCommentTargetSection  WorkspaceDocCommentTargetKind = "section"
)

type DocSectionCommentsArgs struct {
	WorkspaceID string
	DocRelPath  string
	DocPath     string
	SectionID   string
	HeadingHint string
	UserEmail   string
	Comments    []WorkspaceDocCommentView
}

type WorkspaceDocCommentFormArgs struct {
	ID           string
	WorkspaceID  string
	DocRelPath   string
	DocPath      string
	TargetKind   WorkspaceDocCommentTargetKind
	SectionHint  string
	HeadingHint  string
	SelectedText string
	StartLine    int
	StartColumn  int
	EndLine      int
	EndColumn    int
	Error        string
}

type WorkspaceDocCommentTargetRefresh struct {
	WorkspaceID string
	DocRelPath  string
	DocPath     string
	SectionHint string
	HeadingHint string
}

type CreateWorkspaceDocCommentInput struct {
	WorkspaceID string
	DocRelPath  string
	DocPath     string
	UserEmail   string
	CommentText string
	Anchor      WorkspaceDocCommentAnchor
}

type CommentActionRequest struct {
	WorkspaceID string
	CommentID   string
	ActorEmail  string
	ActorType   string
	RequestID   string
	Resolved    bool
	EventType   string
	PayloadJSON string
}

type ReplyWorkspaceDocCommentInput struct {
	WorkspaceID string
	CommentID   string
	UserEmail   string
	ActorType   string
	ReplyText   string
	RequestID   string
}

type AgentCommentActionInput struct {
	WorkspaceID      string
	CommentID        string
	UserEmail        string
	ReplyText        string
	Resolve          bool
	DocUpdated       bool
	ArtifactUpdated  bool
	NoChangeDecision bool
	RequestID        string
}

type WorkspaceHeaderState struct {
	WorkspaceID      string
	Title            string
	RootDocPath      string
	WorkflowLabel    string
	SelectedThreadID string
	HasActiveRun     bool
	ModelLabel       string
	ThinkingLabel    string
}

type WorkspaceSidebarState struct {
	Groups           []ThreadSidebarGroup
	WorkspaceID      string
	SelectedThreadID string
	ActiveGroupKey   string
	HasSelection     bool
}

type WorkspaceCommentSummary struct {
	Unresolved int64
	Resolved   int64
}

type WorkspaceMinimapEvent struct {
	ID         int64
	Type       string
	Category   string
	Label      string
	ThreadID   string
	SessionID  string
	RunID      string
	DocRelPath string
	DocPath    string
	CommentID  string
	CreatedAt  time.Time
}

type WorkspaceMinimapState struct {
	Events []WorkspaceMinimapEvent
}

type WorkspaceCwdProjection struct {
	Path        string
	Label       string
	Scope       string
	Blocked     bool
	BlockReason string
}

type WorkspaceWorkflowState struct {
	WorkspaceID       string
	ThreadID          string
	Type              WorkspaceWorkflowType
	CurrentStep       string
	Status            string
	ReviewGate        string
	Mermaid           string
	Metadata          json.RawMessage
	LastResultSummary string
	PrimaryArtifact   string
	PrimaryDoc        string
	NextDisplay       string
	WaitingHuman      bool
	HumanGateReason   string
	BypassedNodes     []string
	Policy            WorkspaceWorkflowPolicyProjection
	ActiveCwd         WorkspaceCwdProjection
	RuntimeNextStep   string
	LastResultCard    *QRSPIWorkflowCard
}

type WorkspaceSessionState struct {
	PlanDir            string
	PlanLabel          string
	IncludeDescendants bool
	History            []WorkspaceSessionHistoryItem
	Sessions           []db.AgentSession
}

type WorkspaceLogState struct {
	Events []WorkspaceLogItem
}

type WorkspaceLogItem struct {
	ID             int64
	Type           string
	Category       string
	Label          string
	Detail         string
	ThreadID       string
	ThreadHref     string
	SessionID      string
	RunID          string
	DocRelPath     string
	DocPath        string
	CreatedAtLabel string
}

type WorkspaceSessionHistoryItem struct {
	ID                 string
	ThreadID           string
	ThreadHref         string
	Title              string
	Status             string
	SourceLabel        string
	CwdLabel           string
	SessionPathLabel   string
	InferredPlanLabel  string
	FirstPromptExcerpt string
	FirstCommandLabel  string
	ImportStatsLabel   string
	ErrorLabel         string
	UpdatedAtLabel     string
	IsCurrentThread    bool
}

type WorkspaceProjection struct {
	Workspace            db.Workspace
	SelectedThread       *db.AgentThread
	ActiveRun            *db.AgentRun
	Header               WorkspaceHeaderState
	Sidebar              WorkspaceSidebarState
	PlanSidebar          PlanSidebarState
	SidebarProjection    WorkspaceSidebarProjection
	Transcript           TranscriptPaneState
	Docs                 WorkspaceDocPaneState
	Artifacts            WorkspaceDocPaneState
	Comments             WorkspaceCommentSummary
	Minimap              WorkspaceMinimapState
	Workflow             WorkspaceWorkflowState
	Sessions             WorkspaceSessionState
	Log                  WorkspaceLogState
	CurrentChatSessionID string
	ActiveChatPath       []db.ChatSession
	ChatTree             chatsession.ChatTreeProjection
}

type BuildWorkspacePageInput struct {
	UserEmail       string
	WorkspaceID     string
	ThreadID        string
	RunID           string
	DocRelPath      string
	DocPath         string
	ArtifactRelPath string
}

type WorkspacePageArgs struct {
	UserEmail          string
	CurrentTheme       string
	CurrentSyntaxTheme string
	WorkspaceID        string
	Cursor             int64
	Projection         WorkspaceProjection
	Workbench          workbench.WorkbenchState
	PendingAttachments []AttachedPath
}

type ThreadProjectionInput struct {
	UserEmail string
	ThreadID  string
	RunID     string
	DocPath   string
	Cwd       string
}

type ChatPageArgs struct {
	UserEmail          string
	CurrentTheme       string
	CurrentSyntaxTheme string
	Cursor             int64
	Sessions           []SessionThreadRef
	ThreadGroups       []ThreadSidebarGroup
	PlanSidebar        PlanSidebarState
	Cwd                string
	CurrentThread      *db.AgentThread
	ActiveRun          *db.AgentRun
	PrimaryWorkspace   *db.Workspace
	RelatedWorkspaces  []db.Workspace
	ThreadMetadata     ThreadMetadataView
	Workflow           WorkspaceWorkflowState
	Transcript         TranscriptPaneState
	DocPane            DocPaneState
	ArtifactPane       DocPaneState
	Workbench          workbench.WorkbenchState
	PendingAttachments []AttachedPath
}

func docPaneTitle(state DocPaneState) string {
	base := strings.TrimSpace(state.WorkingDir.CurrentTitle)
	if !state.Selected.Exists {
		return base
	}

	docTitle, _ := formatWorkingDirectoryDisplay(state.Selected.DisplayName)
	docTitle = strings.TrimSpace(docTitle)
	if docTitle == "" {
		docTitle = strings.TrimSpace(state.Selected.DisplayName)
	}
	if docTitle == "" {
		return base
	}
	if base == "" || strings.EqualFold(base, docTitle) {
		return docTitle
	}
	return base + " / " + docTitle
}

func transcriptPaneHasDetails(state TranscriptPaneState) bool {
	for _, msg := range state.Stable {
		if msg.Variant == "detail" {
			return true
		}
	}
	for _, msg := range state.Live.Items {
		if msg.Variant == "detail" {
			return true
		}
	}
	return false
}

type transcriptDetailState struct {
	Expanded bool `json:"expanded"`
}

func transcriptDetailSignalManager(msg TranscriptMessage) *utils.SignalManager {
	return utils.Signals(
		"agent-chat-detail-"+signalSafeID(msg.DOMID),
		transcriptDetailState{Expanded: false},
	)
}

func signalSafeID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "detail"
	}
	var builder strings.Builder
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	return builder.String()
}

func transcriptDetailSignals(msg TranscriptMessage) string {
	if !msg.Collapsible {
		return "{}"
	}
	return transcriptDetailSignalManager(msg).DataSignals
}

func transcriptDetailExpanded(msg TranscriptMessage) string {
	return transcriptDetailSignalManager(msg).Signal("expanded")
}

func transcriptDetailCollapsed(msg TranscriptMessage) string {
	return "!" + transcriptDetailExpanded(msg)
}

func transcriptDetailToggle(msg TranscriptMessage) string {
	return transcriptDetailSignalManager(msg).Toggle("expanded")
}

func transcriptDetailAriaExpanded(msg TranscriptMessage) string {
	expanded := transcriptDetailExpanded(msg)
	return expanded + " ? 'true' : 'false'"
}

func transcriptDetailCaretClasses(msg TranscriptMessage) string {
	if !msg.Collapsible {
		return "{}"
	}
	return "{'rotate-180': " + transcriptDetailExpanded(msg) + "}"
}

func transcriptDetailBodyClasses(msg TranscriptMessage) string {
	if !msg.Collapsible {
		return "{}"
	}
	expanded := transcriptDetailExpanded(msg)
	return "{'is-collapsed': !" + expanded + ", 'is-expanded': " + expanded + "}"
}

func transcriptDetailCollapsedBodyClasses(msg TranscriptMessage) string {
	if !msg.Collapsible {
		return "{}"
	}
	return "{'hidden': " + transcriptDetailCollapsed(msg) + "}"
}

func formatWorkspaceEventTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 2 15:04")
}

func getThreadID(thread *db.AgentThread) string {
	if thread == nil {
		return ""
	}
	return thread.ID
}

func getRunID(run *db.AgentRun) string {
	if run == nil {
		return ""
	}
	return run.ID
}

func docSelectAction(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "@post('/agent-chat/docs/select', {contentType: 'form'})"
	}
	return "@post('/agent-chat/" + workspaceID + "/docs/select', {contentType: 'form'})"
}

func planWorkspaceAction(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	return "@post('/agent-chat/" + workspaceID + "/plan-workspace-action', {contentType: 'form'})"
}

func hasWorkspaceAction(actions []WorkspaceAction, action WorkspaceAction) bool {
	for _, existing := range actions {
		if existing == action {
			return true
		}
	}
	return false
}
