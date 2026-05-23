package auth

import "testing"

func TestNormalizeRedirectURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "plain page", raw: "/thoughts/foo?bar=baz", want: "/thoughts/foo?bar=baz"},
		{name: "stream page", raw: "/system/stream", want: "/system"},
		{
			name: "agent chat stream page",
			raw:  "/agent-chat/workspace-1/stream?thread=thread-1",
			want: defaultThoughtsRedirectPath,
		},
		{
			name: "agent chat thread page",
			raw:  "/agent-chat?thread=thread-1",
			want: defaultThoughtsRedirectPath,
		},
		{name: "root stream", raw: "/stream", want: "/"},
		{name: "protocol relative rejected", raw: "//evil.com/path", want: ""},
		{name: "absolute url rejected", raw: "https://evil.com/path", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRedirectURL(tt.raw); got != tt.want {
				t.Fatalf("NormalizeRedirectURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
