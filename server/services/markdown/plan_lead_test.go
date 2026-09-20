package markdown

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func TestRosterDocsBandSelectedDocsDeskRoomID(t *testing.T) {
	t.Parallel()
	sel := agenthome.RosterSelection{Kind: agenthome.KindPlan, ID: "docs--vamos"}
	if !agenthome.RosterDocsBandSelected(sel, "vamos") {
		t.Fatal("docs--vamos should select band id vamos")
	}
	if agenthome.RosterDocsBandSelected(sel, "chestnut") {
		t.Fatal("docs--vamos must not select chestnut")
	}
}

func TestPlanLeadRoomID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"owner/plans/alpha/design.md", "alpha"},
		{"thoughts/owner/plans/alpha/design.md", "alpha"},
		{"/thoughts/owner/plans/alpha/", "alpha"},
		{"owner/plans/alpha", "alpha"},
		{"thoughts/workbench-v2/root.md", ""},
		{"", ""},
		{"notes.md", ""},
	}
	for _, tc := range cases {
		if got := planLeadRoomID(tc.in); got != tc.want {
			t.Fatalf("planLeadRoomID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlanLeadChatHref(t *testing.T) {
	t.Parallel()
	got := planLeadChatHref("owner/plans/alpha/design.md")
	want := "/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"
	if got != want {
		t.Fatalf("planLeadChatHref() = %q, want %q", got, want)
	}
	if got := planLeadChatHref("thoughts/workbench-v2/root.md"); got != "" {
		t.Fatalf("non-plan href = %q", got)
	}
}

func TestThoughtsChatHrefNearestAgents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "vamos", "shots")
	mustMkdirAll(t, docs)
	mustWriteFile(t, filepath.Join(root, "docs", "vamos", "AGENTS.md"), []byte("# desk"))
	mustWriteFile(t, filepath.Join(docs, "index.html"), []byte("<html></html>"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "message-threads.md"),
		[]byte("# mt"),
	)

	planDir := filepath.Join(root, "owner", "plans", "alpha")
	mustMkdirAll(t, planDir)
	mustWriteFile(t, filepath.Join(planDir, "AGENTS.md"), []byte("# plan"))
	mustWriteFile(t, filepath.Join(planDir, "design.md"), []byte("# d"))

	orphanDir := filepath.Join(root, "orphan")
	mustMkdirAll(t, orphanDir)
	mustWriteFile(t, filepath.Join(orphanDir, "note.md"), []byte("# n"))

	cases := []struct {
		doc      string
		wantID   string
		wantArt  string
		wantHide bool
	}{
		{
			doc:     "thoughts/docs/vamos/shots/index.html",
			wantID:  "docs--vamos",
			wantArt: "thoughts/docs/vamos/shots/index.html",
		},
		{
			doc:     "docs/vamos/message-threads.md",
			wantID:  "docs--vamos",
			wantArt: "thoughts/docs/vamos/message-threads.md",
		},
		{
			doc:     "thoughts/docs/vamos/AGENTS.md",
			wantID:  "docs--vamos",
			wantArt: "thoughts/docs/vamos/AGENTS.md",
		},
		{
			doc:     "owner/plans/alpha/design.md",
			wantID:  "alpha",
			wantArt: "thoughts/owner/plans/alpha/design.md",
		},
		{
			doc:      "orphan/note.md",
			wantHide: true,
		},
	}
	for _, tc := range cases {
		got := thoughtsChatHref(root, tc.doc)
		if tc.wantHide {
			if got != "" {
				t.Fatalf("orphan thoughtsChatHref(%q)=%q, want empty", tc.doc, got)
			}
			continue
		}
		if !strings.HasPrefix(got, "/rooms/plan/"+url.PathEscape(tc.wantID)) {
			t.Fatalf("thoughtsChatHref(%q)=%q, want room id %q", tc.doc, got, tc.wantID)
		}
		if strings.Contains(got, "/rooms/plan/?") ||
			strings.HasSuffix(got, "/rooms/plan/") {
			t.Fatalf("dead plan room href: %q", got)
		}
		if !strings.Contains(got, "artifact="+url.QueryEscape(tc.wantArt)) {
			t.Fatalf(
				"thoughtsChatHref(%q)=%q missing artifact %q",
				tc.doc,
				got,
				tc.wantArt,
			)
		}
	}
}

func TestThoughtsPlanArtifactPathFallsBackToPlanMd(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "owner", "plans", "alpha")
	beta := filepath.Join(root, "owner", "plans", "beta")
	gamma := filepath.Join(root, "owner", "plans", "gamma")
	delta := filepath.Join(root, "owner", "plans", "delta")
	epsilon := filepath.Join(root, "owner", "plans", "epsilon")
	zeta := filepath.Join(root, "owner", "plans", "zeta")
	mustMkdirAll(t, alpha)
	mustMkdirAll(t, beta)
	mustMkdirAll(t, gamma)
	mustMkdirAll(t, delta)
	mustMkdirAll(t, epsilon)
	mustMkdirAll(t, zeta)
	mustWriteFile(t, filepath.Join(alpha, "design.md"), []byte("# d"))
	mustWriteFile(t, filepath.Join(alpha, "plan.md"), []byte("# p"))
	mustWriteFile(t, filepath.Join(beta, "AGENTS.md"), []byte("# a"))
	mustWriteFile(t, filepath.Join(beta, "plan.md"), []byte("# p"))
	mustWriteFile(t, filepath.Join(gamma, "plan.md"), []byte("# p"))
	mustWriteFile(t, filepath.Join(epsilon, "README.md"), []byte("# r"))
	mustWriteFile(t, filepath.Join(epsilon, "plan.md"), []byte("# p"))
	mustWriteFile(t, filepath.Join(zeta, "README.md"), []byte("# r"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		dir, want string
	}{
		{"owner/plans/alpha", "thoughts/owner/plans/alpha/design.md"},
		{"owner/plans/beta", "thoughts/owner/plans/beta/AGENTS.md"},
		{"owner/plans/gamma", "thoughts/owner/plans/gamma/plan.md"},
		{"owner/plans/delta", "thoughts/owner/plans/delta/design.md"},
		{"owner/plans/epsilon", "thoughts/owner/plans/epsilon/README.md"},
		{"owner/plans/zeta", "thoughts/owner/plans/zeta/README.md"},
	}
	for _, tc := range cases {
		if got := svc.thoughtsPlanArtifactPath(tc.dir); got != tc.want {
			t.Fatalf("thoughtsPlanArtifactPath(%q)=%q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestGlobRosterPlansIncludesReadmeOnlyDirs(t *testing.T) {
	root := t.TempDir()
	readmeOnly := filepath.Join(root, "owner", "plans", "solo")
	mustMkdirAll(t, readmeOnly)
	mustWriteFile(t, filepath.Join(readmeOnly, "README.md"), []byte("# r"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := svc.globRosterPlans()
	if len(rows) != 1 {
		t.Fatalf("globRosterPlans() = %#v, want 1 README.md row", rows)
	}
	if rows[0].ID != "solo" {
		t.Fatalf("id = %q", rows[0].ID)
	}
	if !strings.Contains(rows[0].Href, "README.md") {
		t.Fatalf("href = %q, want README.md artifact", rows[0].Href)
	}
}

func TestGlobRosterPlansIncludesPlanMdOnlyDirs(t *testing.T) {
	root := t.TempDir()
	planOnly := filepath.Join(root, "owner", "plans", "solo")
	mustMkdirAll(t, planOnly)
	mustWriteFile(t, filepath.Join(planOnly, "plan.md"), []byte("# p"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := svc.globRosterPlans()
	if len(rows) != 1 {
		t.Fatalf("globRosterPlans() = %#v, want 1 plan.md row", rows)
	}
	if rows[0].ID != "solo" {
		t.Fatalf("id = %q", rows[0].ID)
	}
	if !strings.Contains(rows[0].Href, "plan.md") {
		t.Fatalf("href = %q, want plan.md artifact", rows[0].Href)
	}
}
