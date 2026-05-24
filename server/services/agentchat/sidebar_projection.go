package agentchat

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkspaceSidebarProjection struct {
	Workspaces []WorkspaceTree
}

type WorkspaceTree struct {
	WorkspaceID string
	Title       string
	Lifecycle   WorkspaceLifecycleBadge
	Expanded    bool
	Nodes       []chatsession.WorkspaceTreeNode
}

type WorkspaceLifecycleBadge struct {
	Stage      WorkspaceLifecycleStage
	Label      string
	Progress   int
	Tone       string
	NeedsHuman bool
	Blocked    bool
	Href       string
}

type WorkspaceLifecycleStage string

const (
	WorkspaceLifecycleQuestion             WorkspaceLifecycleStage = "question"
	WorkspaceLifecycleResearch             WorkspaceLifecycleStage = "research"
	WorkspaceLifecycleDesign               WorkspaceLifecycleStage = "design"
	WorkspaceLifecycleOutline              WorkspaceLifecycleStage = "outline"
	WorkspaceLifecycleReviewOutline        WorkspaceLifecycleStage = "review_outline"
	WorkspaceLifecyclePlan                 WorkspaceLifecycleStage = "plan"
	WorkspaceLifecycleReviewPlan           WorkspaceLifecycleStage = "review_plan"
	WorkspaceLifecycleWorkspace            WorkspaceLifecycleStage = "workspace"
	WorkspaceLifecycleImplement            WorkspaceLifecycleStage = "implement"
	WorkspaceLifecycleReview               WorkspaceLifecycleStage = "review"
	WorkspaceLifecycleReviewImplementation WorkspaceLifecycleStage = "review_implementation"
	WorkspaceLifecycleVerify               WorkspaceLifecycleStage = "verify"
	WorkspaceLifecycleClosed               WorkspaceLifecycleStage = "closed"
	WorkspaceLifecyclePRDraft              WorkspaceLifecycleStage = "pr_draft"
	WorkspaceLifecyclePRReady              WorkspaceLifecycleStage = "pr_ready"
	WorkspaceLifecyclePRChanges            WorkspaceLifecycleStage = "pr_changes"
	WorkspaceLifecyclePRApproved           WorkspaceLifecycleStage = "pr_approved"
	WorkspaceLifecycleMerged               WorkspaceLifecycleStage = "merged"
	WorkspaceLifecycleBlocked              WorkspaceLifecycleStage = "blocked"
	WorkspaceLifecycleFailed               WorkspaceLifecycleStage = "failed"
)

type SidebarInput struct {
	UserEmail         string
	ActiveWorkspaceID string
	SelectedNodeID    string
}

type PRState struct {
	Stage WorkspaceLifecycleStage
	Href  string
}

func (s *Service) BuildWorkspaceSidebarProjection(
	ctx context.Context,
	input SidebarInput,
) (WorkspaceSidebarProjection, error) {
	workspaces, err := s.queries.ListWorkspaces(ctx, workspaceSidebarWorkspacesLimit)
	if err != nil {
		return WorkspaceSidebarProjection{}, err
	}
	projection := WorkspaceSidebarProjection{
		Workspaces: make([]WorkspaceTree, 0, len(workspaces)),
	}
	for _, workspace := range workspaces {
		workflow, _ := s.BuildWorkspaceWorkflowState(ctx, workspace)
		badge := DeriveWorkspaceLifecycle(
			workspace,
			workflowRuntimeState(workspace),
			PRState{},
		)
		if badge.Href == "" {
			badge.Href = thoughtsWorkspaceHref(workspace.RootDocPath, workspace.ID)
		}
		tree := WorkspaceTree{
			WorkspaceID: workspace.ID,
			Title:       workspace.Title,
			Lifecycle:   badge,
			Expanded:    workspace.ID == strings.TrimSpace(input.ActiveWorkspaceID),
		}
		if workflow.CurrentStep != "" && badge.Label == "" {
			tree.Lifecycle.Label = workflow.CurrentStep
		}
		if tree.Expanded && workspace.CurrentSessionID.Valid {
			chatProjection, err := s.chatSessions.Snapshot(
				ctx,
				workspace.CurrentSessionID.String,
			)
			if err == nil {
				annotations, _ := s.queries.ListChatAnnotationsBySession(
					ctx,
					workspace.CurrentSessionID.String,
				)
				chatProjection.Tree.SelectedNodeID = strings.TrimSpace(
					input.SelectedNodeID,
				)
				tree.Nodes = BuildWorkspaceTree(chatProjection, annotations)
			}
		}
		projection.Workspaces = append(projection.Workspaces, tree)
	}
	return projection, nil
}

func DeriveWorkspaceLifecycle(
	workspace db.Workspace,
	state wruntime.State,
	pr PRState,
) WorkspaceLifecycleBadge {
	if pr.Stage != "" {
		return WorkspaceLifecycleBadge{
			Stage: pr.Stage,
			Label: lifecycleLabel(pr.Stage),
			Tone:  lifecycleTone(pr.Stage),
			Href:  pr.Href,
		}
	}
	stage := lifecycleStageFromNode(string(state.CurrentNodeID))
	if stage == "" {
		stage = lifecycleStageFromWorkspace(workspace)
	}
	if state.Status == wruntime.WorkspaceStatusBlocked {
		stage = WorkspaceLifecycleBlocked
	}
	if state.Status == wruntime.WorkspaceStatusError {
		stage = WorkspaceLifecycleFailed
	}
	badge := WorkspaceLifecycleBadge{
		Stage:      stage,
		Label:      lifecycleLabel(stage),
		Progress:   lifecycleProgress(stage),
		Tone:       lifecycleTone(stage),
		NeedsHuman: state.Status == wruntime.WorkspaceStatusWaitingHuman,
		Blocked: state.Status == wruntime.WorkspaceStatusBlocked ||
			state.Status == wruntime.WorkspaceStatusError,
	}
	if badge.NeedsHuman && badge.Label != "" {
		badge.Label += " · needs human"
	}
	return badge
}

func BuildWorkspaceTree(
	proj chatsession.ChatProjection,
	annotations []db.ChatAnnotation,
) []chatsession.WorkspaceTreeNode {
	withCounts := chatsession.ApplyAnnotationCounts(proj, annotations)
	nodes := append([]chatsession.WorkspaceTreeNode(nil), withCounts.Tree.Nodes...)
	active := map[string]bool{}
	for _, id := range withCounts.Tree.ActivePathNodeIDs {
		active[id] = true
	}
	for i := range nodes {
		if nodes[i].WorkspaceID == "" {
			nodes[i].WorkspaceID = withCounts.Tree.WorkspaceID
		}
		if active[nodes[i].ID] {
			nodes[i].ActivePath = true
		}
		if withCounts.Tree.SelectedNodeID != "" &&
			nodes[i].ID == withCounts.Tree.SelectedNodeID {
			nodes[i].Selected = true
		}
		if nodes[i].ChatHref == "" && nodes[i].SessionID != "" {
			nodes[i].ChatHref = chatNodeHref(nodes[i])
		}
		if nodes[i].DocumentHref == "" && nodes[i].WorkspaceID != "" {
			nodes[i].DocumentHref = chatNodeHref(nodes[i])
		}
	}
	return nodes
}

func workflowRuntimeState(workspace db.Workspace) wruntime.State {
	if !workspace.WorkflowStateJson.Valid ||
		strings.TrimSpace(workspace.WorkflowStateJson.String) == "" {
		return wruntime.State{}
	}
	var state wruntime.State
	_ = jsonUnmarshal([]byte(workspace.WorkflowStateJson.String), &state)
	return state
}

func lifecycleStageFromWorkspace(workspace db.Workspace) WorkspaceLifecycleStage {
	if strings.TrimSpace(workspace.WorkflowType) == string(WorkspaceWorkflowQRSPI) {
		return WorkspaceLifecycleQuestion
	}
	return WorkspaceLifecyclePlan
}

func lifecycleStageFromNode(node string) WorkspaceLifecycleStage {
	switch {
	case strings.Contains(node, "question"):
		return WorkspaceLifecycleQuestion
	case strings.Contains(node, "research"):
		return WorkspaceLifecycleResearch
	case strings.Contains(node, "design"):
		return WorkspaceLifecycleDesign
	case strings.Contains(node, "review_implementation"), strings.Contains(node, "review-implementation"):
		return WorkspaceLifecycleReviewImplementation
	case strings.Contains(node, "review_outline"), strings.Contains(node, "review-outline"):
		return WorkspaceLifecycleReviewOutline
	case strings.Contains(node, "review_plan"), strings.Contains(node, "review-plan"):
		return WorkspaceLifecycleReviewPlan
	case strings.Contains(node, "verify"):
		return WorkspaceLifecycleVerify
	case strings.Contains(node, "outline"):
		return WorkspaceLifecycleOutline
	case strings.Contains(node, "plan"):
		return WorkspaceLifecyclePlan
	case strings.Contains(node, "workspace"):
		return WorkspaceLifecycleWorkspace
	case strings.Contains(node, "implement"):
		return WorkspaceLifecycleImplement
	case strings.Contains(node, "review"):
		return WorkspaceLifecycleReview
	case strings.Contains(node, "done"), strings.Contains(node, "merge"):
		return WorkspaceLifecycleMerged
	default:
		return ""
	}
}

func lifecycleLabel(stage WorkspaceLifecycleStage) string {
	switch stage {
	case WorkspaceLifecycleQuestion:
		return "Question"
	case WorkspaceLifecycleResearch:
		return "Research"
	case WorkspaceLifecycleDesign:
		return "Design"
	case WorkspaceLifecycleOutline:
		return "Outline"
	case WorkspaceLifecycleReviewOutline:
		return "Review outline"
	case WorkspaceLifecyclePlan:
		return "Plan"
	case WorkspaceLifecycleReviewPlan:
		return "Review plan"
	case WorkspaceLifecycleWorkspace:
		return "Workspace"
	case WorkspaceLifecycleImplement:
		return "Implement"
	case WorkspaceLifecycleReview:
		return "Review"
	case WorkspaceLifecycleReviewImplementation:
		return "Review implementation"
	case WorkspaceLifecycleVerify:
		return "Verify"
	case WorkspaceLifecycleClosed:
		return "Closed"
	case WorkspaceLifecyclePRDraft:
		return "PR draft"
	case WorkspaceLifecyclePRReady:
		return "PR ready"
	case WorkspaceLifecyclePRChanges:
		return "PR changes"
	case WorkspaceLifecyclePRApproved:
		return "PR approved"
	case WorkspaceLifecycleMerged:
		return "Merged"
	case WorkspaceLifecycleBlocked:
		return "Blocked"
	case WorkspaceLifecycleFailed:
		return "Failed"
	default:
		return "Active"
	}
}

const (
	lifecycleProgressNone          = 0
	lifecycleProgressQuestion      = 10
	lifecycleProgressResearch      = 20
	lifecycleProgressDesign        = 35
	lifecycleProgressOutline       = 50
	lifecycleProgressReviewOutline = 55
	lifecycleProgressPlan          = 60
	lifecycleProgressReviewPlan    = 62
	lifecycleProgressWorkspace     = 65
	lifecycleProgressImplement     = 75
	lifecycleProgressReview        = 85
	lifecycleProgressVerify        = 92
	lifecycleProgressPR            = 90
	lifecycleProgressApproved      = 95
	lifecycleProgressMerged        = 100
)

func lifecycleProgress(stage WorkspaceLifecycleStage) int {
	switch stage {
	case WorkspaceLifecycleQuestion:
		return lifecycleProgressQuestion
	case WorkspaceLifecycleResearch:
		return lifecycleProgressResearch
	case WorkspaceLifecycleDesign:
		return lifecycleProgressDesign
	case WorkspaceLifecycleOutline:
		return lifecycleProgressOutline
	case WorkspaceLifecycleReviewOutline:
		return lifecycleProgressReviewOutline
	case WorkspaceLifecyclePlan:
		return lifecycleProgressPlan
	case WorkspaceLifecycleReviewPlan:
		return lifecycleProgressReviewPlan
	case WorkspaceLifecycleWorkspace:
		return lifecycleProgressWorkspace
	case WorkspaceLifecycleImplement:
		return lifecycleProgressImplement
	case WorkspaceLifecycleReview, WorkspaceLifecycleReviewImplementation:
		return lifecycleProgressReview
	case WorkspaceLifecycleVerify:
		return lifecycleProgressVerify
	case WorkspaceLifecyclePRDraft,
		WorkspaceLifecyclePRReady,
		WorkspaceLifecyclePRChanges:
		return lifecycleProgressPR
	case WorkspaceLifecyclePRApproved:
		return lifecycleProgressApproved
	case WorkspaceLifecycleMerged:
		return lifecycleProgressMerged
	case WorkspaceLifecycleBlocked, WorkspaceLifecycleFailed:
		return lifecycleProgressNone
	default:
		return lifecycleProgressNone
	}
}

func lifecycleTone(stage WorkspaceLifecycleStage) string {
	switch stage {
	case WorkspaceLifecycleBlocked, WorkspaceLifecycleFailed, WorkspaceLifecyclePRChanges:
		return "destructive"
	case WorkspaceLifecycleMerged, WorkspaceLifecycleClosed, WorkspaceLifecyclePRApproved:
		return "success"
	case WorkspaceLifecycleReview,
		WorkspaceLifecycleReviewOutline,
		WorkspaceLifecycleReviewPlan,
		WorkspaceLifecycleReviewImplementation,
		WorkspaceLifecyclePRDraft,
		WorkspaceLifecyclePRReady:
		return "accent"
	case WorkspaceLifecycleQuestion,
		WorkspaceLifecycleResearch,
		WorkspaceLifecycleDesign,
		WorkspaceLifecycleOutline,
		WorkspaceLifecyclePlan,
		WorkspaceLifecycleWorkspace,
		WorkspaceLifecycleImplement,
		WorkspaceLifecycleVerify:
		return "muted"
	default:
		return "muted"
	}
}

func workspaceLifecycleBadgeClass(badge WorkspaceLifecycleBadge) string {
	switch badge.Tone {
	case "destructive":
		return "border-destructive/30 bg-destructive/10 text-destructive"
	case "success":
		return "border-emerald-500/30 bg-emerald-500/10 text-emerald-600"
	case "accent":
		return "border-primary/30 bg-primary/10 text-primary"
	default:
		return "border-border bg-muted text-muted-foreground"
	}
}

func chatNodeHref(node chatsession.WorkspaceTreeNode) string {
	values := url.Values{}
	if node.SessionID != "" {
		values.Set("chat_session", node.SessionID)
	}
	if node.ID != "" {
		values.Set("chat_node", node.ID)
	}
	if node.EventSeq > 0 {
		values.Set("chat_seq", strconvFormatInt(node.EventSeq))
	}
	if values.Encode() == "" {
		return ""
	}
	return "/agent-chat/workspaces/" + url.PathEscape(
		node.WorkspaceID,
	) + "?" + values.Encode()
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func strconvFormatInt(v int64) string        { return strconv.FormatInt(v, 10) }
