package build

import (
	"path/filepath"
	"testing"
)

func TestHashStableIndependentOfCreationOrder(t *testing.T) {
	t.Parallel()

	first := t.TempDir()
	writeFile(t, filepath.Join(first, "b.txt"), "b")
	writeFile(t, filepath.Join(first, "a.txt"), "a")

	second := t.TempDir()
	writeFile(t, filepath.Join(second, "a.txt"), "a")
	writeFile(t, filepath.Join(second, "b.txt"), "b")

	spec := HashSpec{Roots: []string{"."}}
	firstHash, err := NewTreeHasher(first).Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash first: %v", err)
	}
	secondHash, err := NewTreeHasher(second).Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash second: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes differ for same tree: %s != %s", firstHash, secondHash)
	}
}

func TestHashChangedFileContentChangesHash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	writeFile(t, path, "before")
	hasher := NewTreeHasher(dir)
	spec := HashSpec{Roots: []string{"file.txt"}}
	before, err := hasher.Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash before: %v", err)
	}
	writeFile(t, path, "after")
	after, err := hasher.Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash after: %v", err)
	}
	if before == after {
		t.Fatal("hash did not change after file contents changed")
	}
}

func TestHashExcludesBuildAgents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tracked.txt"), "same")
	writeFile(t, filepath.Join(dir, ".build-agents", "state.json"), "before")
	hasher := NewTreeHasher(dir)
	spec := HashSpec{Roots: []string{"."}, Excludes: []string{".build-agents/**"}}
	before, err := hasher.Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash before: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".build-agents", "state.json"), "after")
	after, err := hasher.Hash(t.Context(), spec)
	if err != nil {
		t.Fatalf("hash after: %v", err)
	}
	if before != after {
		t.Fatalf("excluded .build-agents changed hash: %s != %s", before, after)
	}
}
