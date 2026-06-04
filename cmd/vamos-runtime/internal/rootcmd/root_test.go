package rootcmd

import "testing"

func TestRootCommandContainsAuthCtlAndE2E(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"auth", "ctl", "e2e"} {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing command %s", name)
		}
	}
}
