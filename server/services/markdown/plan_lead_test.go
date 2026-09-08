package markdown

import "testing"

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
