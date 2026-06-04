package ctlcmd

import "testing"

func TestCtlCommandContainsOperationalSubcommands(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"workspace", "verify", "project-metadata"} {
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
