package comments

import "testing"

func TestCommentDisplayNameShortensAllowedDomains(t *testing.T) {
	s := (&Service{}).WithAllowedDomains([]string{"chestnutfi.com", "@example.org"})

	cases := []struct {
		email string
		want  string
	}{
		{"corey@chestnutfi.com", "Corey"},
		{"jane.doe@chestnutfi.com", "Jane Doe"},
		{"alex-smith+notes@example.org", "Alex Smith Notes"},
		{"outsider@gmail.com", "outsider@gmail.com"},
		{"not-an-email", "not-an-email"},
	}

	for _, tc := range cases {
		if got := s.commentDisplayName(tc.email); got != tc.want {
			t.Fatalf("commentDisplayName(%q) = %q, want %q", tc.email, got, tc.want)
		}
	}
}
