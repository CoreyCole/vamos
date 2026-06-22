package appfiles

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSafeOpenPathRejectsTraversalAndAllowsRootPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "players.csv"), []byte("name\nAda\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := SafeOpenPath(root, "players.csv")
	if err != nil {
		t.Fatalf("safe path: %v", err)
	}
	if got != filepath.Join(root, "players.csv") {
		t.Fatalf("path = %q", got)
	}

	for _, rel := range []string{"../secret.txt", "/../../secret.txt"} {
		if _, err := SafeOpenPath(root, rel); err == nil {
			t.Fatalf("SafeOpenPath(%q) succeeded, want error", rel)
		}
	}
}

func TestSafeOpenPathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeOpenPath(root, "secret-link"); err == nil {
		t.Fatal("symlink escape succeeded, want error")
	}

	if err := os.Symlink(outside, filepath.Join(root, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeOpenPath(root, "outside-dir/secret.txt"); err == nil {
		t.Fatal("parent symlink escape succeeded, want error")
	}
}

func TestIsHiddenMatchesPathSubtrees(t *testing.T) {
	hidden := []string{"apps/iterations", "logs", "temp", "bin"}
	for _, rel := range []string{"apps/iterations", "apps/iterations/2026", "logs/app.log", "temp/build", "bin/server"} {
		if !IsHidden(rel, hidden) {
			t.Fatalf("IsHidden(%q) = false, want true", rel)
		}
	}
	for _, rel := range []string{"apps/current", "players.csv", "app.log"} {
		if IsHidden(rel, hidden) {
			t.Fatalf("IsHidden(%q) = true, want false", rel)
		}
	}
}
