package e2ecmd

import "testing"

func TestE2ECommandShape(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"check", "generate", "run", "review", "fix", "goldens"} {
		found := false
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing e2e command %s", name)
		}
	}
}
