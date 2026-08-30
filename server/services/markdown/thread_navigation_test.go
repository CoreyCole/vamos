package markdown

import (
	"os"
	"strings"
	"testing"
)

func TestThreadNavigationTemplatesUseOnlyPlainAnchors(t *testing.T) {
	for _, file := range []string{"thread_navigation.templ", "thread_navigation_templ.go"} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"preventDefault", "/select", "/artifact", "data-replace-url", "workbench-v2-url-sync"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s contains %q", file, forbidden)
			}
		}
	}
}

func TestThreadArtifactHrefUsesThoughtsRouteAndCanonicalPath(t *testing.T) {
	if got := ThreadArtifactHref(
		"thread_1",
		"thoughts/owner/plans/alpha/design.md",
	); got != "/thoughts/owner/plans/alpha/design.md" {
		t.Fatalf("href = %q", got)
	}
}

func TestThreadArtifactHrefRejectsInvalidDocument(t *testing.T) {
	if got := ThreadArtifactHref("thread_1", "../../etc/passwd"); got != "/thoughts/" {
		t.Fatalf("href = %q", got)
	}
}
