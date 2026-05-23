package workspaces

import "testing"

func TestWorkspaceCardTitleFormatsTimestampedSlug(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "2026-05-11-06-13-26-workspace-subdomains-https-caddy-review"}
	if got, want := workspaceCardTitle(
		ws,
	), "Workspace Subdomains HTTPS Caddy Review"; got != want {
		t.Fatalf("workspaceCardTitle() = %q, want %q", got, want)
	}
}

func TestWorkspaceCardTimestampFormatsSlugTimestamp(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "2026-05-11-06-13-26-workspace-subdomains-https-caddy-review"}
	if got, want := workspaceCardTimestamp(ws), "May 11, 2026 · 6:13 AM"; got != want {
		t.Fatalf("workspaceCardTimestamp() = %q, want %q", got, want)
	}
}

func TestWorkspaceCardTitleFormatsMain(t *testing.T) {
	t.Parallel()

	ws := Workspace{Slug: "main", IsMain: true}
	if got, want := workspaceCardTitle(ws), "Main"; got != want {
		t.Fatalf("workspaceCardTitle() = %q, want %q", got, want)
	}
	if got := workspaceCardTimestamp(ws); got != "" {
		t.Fatalf("workspaceCardTimestamp() = %q, want empty", got)
	}
}

func TestWorkspaceActionIndicatorIsScopedBySlugAndAction(t *testing.T) {
	t.Parallel()

	start := workspaceActionIndicator("feature", "start")
	if start == "" || start[0] != '_' {
		t.Fatalf("workspaceActionIndicator() = %q, want private signal name", start)
	}
	if got := workspaceActionIndicatorSignal("feature", "start"); got != "$"+start {
		t.Fatalf("workspaceActionIndicatorSignal() = %q", got)
	}
	if got := workspaceActionIndicator("other", "start"); got == start {
		t.Fatalf("different workspace reused indicator %q", got)
	}
	if got := workspaceActionIndicator("feature", "stop"); got == start {
		t.Fatalf("different action reused indicator %q", got)
	}
}

func TestWorkspaceActionHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		observed          WorkspaceObservedState
		wantStart         bool
		wantStop          bool
		wantRestart       bool
		wantTransitioning bool
	}{
		{name: "starting", observed: WorkspaceObservedStarting, wantTransitioning: true},
		{name: "stopping", observed: WorkspaceObservedStopping, wantTransitioning: true},
		{name: "stopped", observed: WorkspaceObservedStopped, wantStart: true},
		{name: "failed", observed: WorkspaceObservedFailed, wantStart: true},
		{name: "crashed", observed: WorkspaceObservedCrashed, wantStart: true},
		{
			name:        "running",
			observed:    WorkspaceObservedRunning,
			wantStop:    true,
			wantRestart: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snap := WorkspaceLifecycleSnapshot{
				Workspace:     Workspace{Slug: "feature"},
				ObservedState: tc.observed,
			}
			if got := workspaceCanStart(snap); got != tc.wantStart {
				t.Fatalf("workspaceCanStart() = %v, want %v", got, tc.wantStart)
			}
			if got := workspaceCanStop(snap); got != tc.wantStop {
				t.Fatalf("workspaceCanStop() = %v, want %v", got, tc.wantStop)
			}
			if got := workspaceCanRestart(snap); got != tc.wantRestart {
				t.Fatalf("workspaceCanRestart() = %v, want %v", got, tc.wantRestart)
			}
			if got := workspaceTransitioning(snap); got != tc.wantTransitioning {
				t.Fatalf(
					"workspaceTransitioning() = %v, want %v",
					got,
					tc.wantTransitioning,
				)
			}
		})
	}
}

func TestWorkspaceActionHelpersDisallowMainWorkspace(t *testing.T) {
	t.Parallel()

	snap := WorkspaceLifecycleSnapshot{
		Workspace:     Workspace{Slug: "main", IsMain: true},
		ObservedState: WorkspaceObservedRunning,
	}
	if workspaceCanStart(snap) || workspaceCanStop(snap) || workspaceCanRestart(snap) {
		t.Fatalf("main workspace should not allow lifecycle actions")
	}
}

func TestWorkspaceTransitionLabelFallsBackToWorkspaceStatus(t *testing.T) {
	t.Parallel()

	snap := WorkspaceLifecycleSnapshot{Workspace: Workspace{Status: StatusStopped}}
	if got := workspaceTransitionLabel(snap); got != "stopped" {
		t.Fatalf("workspaceTransitionLabel() = %q, want stopped", got)
	}
}
