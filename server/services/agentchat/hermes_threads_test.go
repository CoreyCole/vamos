package agentchat

import (
	"testing"
	"time"
)

func TestCanPromptThreadIsOwnerOnlyAndCaseInsensitive(t *testing.T) {
	s := &Service{}
	thread := HermesThread{OwnerEmail: "Owner@Example.com"}
	if !s.CanPromptThread("owner@example.com", thread) {
		t.Fatal("owner should be allowed to prompt")
	}
	if s.CanPromptThread("observer@example.com", thread) {
		t.Fatal("observer must remain read-only")
	}
}

func TestGroupHermesThreadsGroupsByPlan(t *testing.T) {
	groups := GroupHermesThreads([]HermesThread{
		{ID: "a", PlanDir: "thoughts/a", UpdatedAt: time.Now()},
		{ID: "b", PlanDir: "thoughts/a"},
		{ID: "c", PlanDir: "thoughts/b"},
	})
	if len(groups) != 2 || groups[0].PlanDir != "thoughts/a" ||
		len(groups[0].Threads) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
}
