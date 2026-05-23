package docs

import "testing"

func TestParseThoughtsDocPath(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		raw     string
		want    DocPath
		wantErr bool
	}{
		{name: "plain", raw: "foo/design.md", want: "foo/design.md"},
		{name: "strips thoughts", raw: "thoughts/foo/design.md", want: "foo/design.md"},
		{
			name:   "strips prefix",
			prefix: "agent-chat/thoughts",
			raw:    "/agent-chat/thoughts/foo/design.md",
			want:   "foo/design.md",
		},
		{name: "decodes echo wildcard", raw: "foo/a%20b.md", want: "foo/a b.md"},
		{name: "empty", raw: "", wantErr: true},
		{name: "escape", raw: "../secret.md", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseThoughtsDocPath(tt.prefix, tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseThoughtsDocPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseThoughtsDocPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseThoughtsDocPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDocHrefsEscapePathSegments(t *testing.T) {
	docPath := DocPath("foo/a b.md")
	if got := ThoughtsDocHref(docPath); got != "/thoughts/foo/a%20b.md" {
		t.Fatalf("ThoughtsDocHref() = %q", got)
	}
}
