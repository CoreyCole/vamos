package agentchat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHermesThreadsExplicitPlanAuthorityCreatorAndSharedVisibility(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	service := &Service{thoughtsRoot: root}
	thread, err := service.CreateHermesThread(context.Background(), CreateHermesThreadInput{
		PlanDir: "owner/plans/alpha", CreatorEmail: " Creator@Example.COM ", Title: "Alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if thread.CreatorEmail != "Creator@Example.COM" || thread.PromptAuthority.PrincipalValue != "creator@example.com" {
		t.Fatalf("thread = %#v", thread)
	}
	if !service.CanPromptThread("CREATOR@example.com", thread) {
		t.Fatal("authority principal could not prompt")
	}
	thread.CreatorEmail = "someone@example.com"
	if !service.CanPromptThread("creator@example.com", thread) || service.CanPromptThread("someone@example.com", thread) {
		t.Fatal("creator provenance affected prompt authority")
	}
	threads, err := service.ListHermesThreads(context.Background(), ThreadQuery{PlanDir: "owner/plans/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Title != "Alpha" {
		t.Fatalf("threads = %#v", threads)
	}
	empty, err := service.ListHermesThreads(context.Background(), ThreadQuery{})
	if err != nil || len(empty) != 0 {
		t.Fatalf("blank-plan threads = %#v, %v", empty, err)
	}
}

func TestHermesThreadsServerCollisionRetryAndStableUpdatedOrder(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	service := &Service{thoughtsRoot: root}
	original := newHermesThreadID
	ids := []string{"same", "same", "different"}
	newHermesThreadID = func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}
	defer func() { newHermesThreadID = original }()
	first, err := service.CreateHermesThread(context.Background(), CreateHermesThreadInput{
		PlanDir: "owner/plans/alpha", CreatorEmail: "owner@example.com", Title: "First",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPath, err := HermesTranscriptPath(plan, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(firstPath, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateHermesThread(context.Background(), CreateHermesThreadInput{
		PlanDir: "owner/plans/alpha", CreatorEmail: "owner@example.com", Title: "Second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "same" || second.ID != "different" {
		t.Fatalf("IDs = %q, %q", first.ID, second.ID)
	}
	threads, err := service.ListHermesThreads(context.Background(), ThreadQuery{PlanDir: "owner/plans/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 || threads[0].ID != "different" {
		t.Fatalf("order = %#v", threads)
	}
}

func TestHermesThreadsEqualIDDifferentPlanIsolation(t *testing.T) {
	root := t.TempDir()
	service := &Service{thoughtsRoot: root}
	for _, identity := range []HermesPlanIdentity{"owner/plans/alpha", "owner/plans/beta"} {
		plan := filepath.Join(root, filepath.FromSlash(string(identity)))
		if err := os.MkdirAll(plan, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := AppendHermesTranscript(plan, hermesMetadataFixture(identity, "same")); err != nil {
			t.Fatal(err)
		}
	}
	for _, identity := range []string{"owner/plans/alpha", "owner/plans/beta"} {
		threads, err := service.ListHermesThreads(context.Background(), ThreadQuery{PlanDir: identity})
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) != 1 || threads[0].ID != "same" || threads[0].PlanDir != identity {
			t.Fatalf("%s threads = %#v", identity, threads)
		}
	}
}
