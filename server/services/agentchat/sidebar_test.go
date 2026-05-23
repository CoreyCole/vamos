//go:build !integration || unit

package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestDeriveWorkspaceLifecycleQRSPIStages(t *testing.T) {
	t.Parallel()

	workspace := db.Workspace{WorkflowType: string(WorkspaceWorkflowQRSPI)}
	tests := []struct {
		name  string
		state wruntime.State
		want  WorkspaceLifecycleStage
	}{
		{
			name:  "question",
			state: wruntime.State{CurrentNodeID: "question"},
			want:  WorkspaceLifecycleQuestion,
		},
		{
			name:  "research",
			state: wruntime.State{CurrentNodeID: "research"},
			want:  WorkspaceLifecycleResearch,
		},
		{
			name:  "design",
			state: wruntime.State{CurrentNodeID: "design"},
			want:  WorkspaceLifecycleDesign,
		},
		{
			name:  "outline",
			state: wruntime.State{CurrentNodeID: "review-outline"},
			want:  WorkspaceLifecycleOutline,
		},
		{
			name:  "plan",
			state: wruntime.State{CurrentNodeID: "review-plan"},
			want:  WorkspaceLifecyclePlan,
		},
		{
			name:  "workspace",
			state: wruntime.State{CurrentNodeID: "workspace"},
			want:  WorkspaceLifecycleWorkspace,
		},
		{
			name:  "implement",
			state: wruntime.State{CurrentNodeID: "implement"},
			want:  WorkspaceLifecycleImplement,
		},
		{
			name:  "review",
			state: wruntime.State{CurrentNodeID: "review-implementation"},
			want:  WorkspaceLifecycleReview,
		},
		{
			name: "blocked",
			state: wruntime.State{
				CurrentNodeID: "implement",
				Status:        wruntime.WorkspaceStatusBlocked,
			},
			want: WorkspaceLifecycleBlocked,
		},
		{
			name: "failed",
			state: wruntime.State{
				CurrentNodeID: "implement",
				Status:        wruntime.WorkspaceStatusError,
			},
			want: WorkspaceLifecycleFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			badge := DeriveWorkspaceLifecycle(workspace, tt.state, PRState{})
			if badge.Stage != tt.want {
				t.Fatalf("stage = %q, want %q", badge.Stage, tt.want)
			}
		})
	}
}

func TestBuildWorkspaceSidebarProjectionShowsSharedWorkspaces(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")

	projection, err := service.BuildWorkspaceSidebarProjection(t.Context(), SidebarInput{
		UserEmail:         "coworker@example.com",
		ActiveWorkspaceID: workspace.ID,
	})
	if err != nil {
		t.Fatalf("BuildWorkspaceSidebarProjection() error = %v", err)
	}
	if len(projection.Workspaces) == 0 {
		t.Fatal("projection has no workspaces, want shared workspace visible")
	}
	if projection.Workspaces[0].WorkspaceID != workspace.ID ||
		!projection.Workspaces[0].Expanded {
		t.Fatalf(
			"first workspace = %+v, want active shared workspace",
			projection.Workspaces[0],
		)
	}
}

func TestBuildWorkspaceTreeMarksActivePathAndBranchNodes(t *testing.T) {
	t.Parallel()

	projection := chatsession.ChatProjection{Tree: chatsession.ChatTreeProjection{
		WorkspaceID:       "workspace-1",
		SelectedNodeID:    "node-2",
		ActivePathNodeIDs: []string{"node-1"},
		Nodes: []chatsession.WorkspaceTreeNode{
			{
				ID:        "node-1",
				SessionID: "session-1",
				EventSeq:  1,
				Label:     "user",
				CanFork:   true,
			},
			{
				ID:         "node-2",
				SessionID:  "session-2",
				EventSeq:   2,
				Label:      "fork",
				Branch:     true,
				CanPromote: true,
			},
		},
	}}

	nodes := BuildWorkspaceTree(projection, nil)
	if len(nodes) != 2 {
		t.Fatalf("nodes len = %d, want 2", len(nodes))
	}
	if !nodes[0].ActivePath || nodes[0].WorkspaceID != "workspace-1" ||
		nodes[0].ChatHref == "" {
		t.Fatalf("node 1 = %+v, want active path with href", nodes[0])
	}
	if !nodes[1].Selected || !nodes[1].Branch {
		t.Fatalf("node 2 = %+v, want selected branch", nodes[1])
	}
}

func TestWorkspaceSidebarRendersLifecycleAndAnnotationBadges(t *testing.T) {
	t.Parallel()

	body := renderTemplToString(
		t,
		WorkspaceTopologySidebarBody(WorkspaceSidebarProjection{
			Workspaces: []WorkspaceTree{{
				WorkspaceID: "workspace-1",
				Title:       "Durable chat plan",
				Expanded:    true,
				Lifecycle: WorkspaceLifecycleBadge{
					Stage:      WorkspaceLifecycleImplement,
					Label:      "Implement",
					Progress:   lifecycleProgressImplement,
					Tone:       "accent",
					NeedsHuman: true,
					Href:       "/agent-chat/workspaces/workspace-1",
				},
				Nodes: []chatsession.WorkspaceTreeNode{
					{
						ID:              "node-1",
						Label:           "assistant",
						Summary:         "Implemented topology sidebar",
						ChatHref:        "/agent-chat/workspaces/workspace-1?chat_node=node-1",
						ActivePath:      true,
						UnresolvedCount: 2,
					},
				},
			}},
		}),
	)

	for _, want := range []string{"Durable chat plan", "Implement", "Needs human", "assistant", "2 unresolved"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered sidebar missing %q:\n%s", want, body)
		}
	}
}
