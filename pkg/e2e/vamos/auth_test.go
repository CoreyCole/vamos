package vamos

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
)

func TestWorkspaceVerifyConfigForAuthUsesRequestedEmail(t *testing.T) {
	cfg := duiruntime.Config{BaseURL: "https://feature.example.test"}
	primary := workspaceVerifyConfigForAuth(cfg, "primary@example.test")
	secondary := workspaceVerifyConfigForAuth(cfg, "secondary@example.test")
	if primary.BrowserEmail != "primary@example.test" ||
		secondary.BrowserEmail != "secondary@example.test" {
		t.Fatalf("requested actor emails were not preserved: %#v %#v", primary, secondary)
	}
	if primary.BrowserEmail == secondary.BrowserEmail {
		t.Fatal("primary and secondary auth claims overlap")
	}
}
