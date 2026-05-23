package rootcmd

import "testing"

func TestRootCommandContainsCtlAndE2E(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"ctl", "e2e"} {
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
