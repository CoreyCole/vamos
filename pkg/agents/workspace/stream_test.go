package workspace

import "testing"

func TestScopeForPersistedEventAlwaysRefreshesResource(t *testing.T) {
	for _, eventType := range []string{"thread_created", "run_checkpointed", "workflow_stage_changed", "future"} {
		if got := ScopeForPersistedEvent(eventType); got != PatchResource {
			t.Fatalf(
				"ScopeForPersistedEvent(%q) = %q, want %q",
				eventType,
				got,
				PatchResource,
			)
		}
	}
}

func TestNeedsCatchup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		since         int64
		currentCursor int64
		want          bool
	}{
		{name: "equal cursors", since: 3, currentCursor: 3, want: false},
		{name: "client behind", since: 2, currentCursor: 3, want: true},
		{name: "client ahead", since: 4, currentCursor: 3, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NeedsCatchup(tt.since, tt.currentCursor); got != tt.want {
				t.Fatalf(
					"NeedsCatchup(%d, %d) = %t, want %t",
					tt.since,
					tt.currentCursor,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestSidebarVisibleByDefault(t *testing.T) {
	t.Parallel()

	if !SidebarVisibleByDefault(false) {
		t.Fatal("SidebarVisibleByDefault(false) = false, want true")
	}
	if SidebarVisibleByDefault(true) {
		t.Fatal("SidebarVisibleByDefault(true) = true, want false")
	}
}

func TestIsLiveConversationEvent(t *testing.T) {
	for _, eventType := range []string{"message_start", "message_update", "message_end", "tool_execution_start", "tool_execution_update", "tool_execution_end"} {
		if !IsLiveConversationEvent(eventType) {
			t.Fatalf("IsLiveConversationEvent(%q) = false, want true", eventType)
		}
	}
	for _, eventType := range []string{"checkpoint", "run_completed", "workflow_stage_changed", ""} {
		if IsLiveConversationEvent(eventType) {
			t.Fatalf("IsLiveConversationEvent(%q) = true, want false", eventType)
		}
	}
}
