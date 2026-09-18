package roster

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestListGetActive(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: zebra
    name: Zebra
  - slug: alpha
    name: Alpha
  - slug: beta
    name: alpha
    archived: true
`)
	store := &Store{Path: path}

	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List len = %d, want 2", len(got))
	}
	if got[0].Slug != "alpha" || got[1].Slug != "zebra" {
		t.Fatalf("List order = %v", slugs(got))
	}

	bot, err := store.Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if bot.Name != "Alpha" {
		t.Fatalf("Get name = %q", bot.Name)
	}
}

func TestListOmitsArchivedGetArchived(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: live
    name: Live
  - slug: old
    name: Old
    archived: true
`)
	store := &Store{Path: path}

	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slug != "live" {
		t.Fatalf("List = %v", slugs(got))
	}

	_, err = store.Get("old")
	if !errors.Is(err, ErrArchived) {
		t.Fatalf("Get archived = %v, want ErrArchived", err)
	}

	_, err = store.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestMissingFileEmpty(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "agents.yml")}
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing file List = %v", got)
	}
	_, err = store.Get("any")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing file = %v", err)
	}
}

func TestEmptyBots(t *testing.T) {
	path := writeRoster(t, "bots: []\n")
	store := &Store{Path: path}
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty bots List = %v", got)
	}
}

func TestBadYAML(t *testing.T) {
	path := writeRoster(t, "bots: [\n")
	store := &Store{Path: path}
	_, err := store.List()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad yaml = %v, want ErrInvalid", err)
	}
}

func TestDuplicateSlugInvalid(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: dup
    name: One
  - slug: dup
    name: Two
`)
	store := &Store{Path: path}
	_, err := store.List()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate = %v, want ErrInvalid", err)
	}
}

func TestReservedSlugInFileInvalid(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: a2a
    name: Reserved
`)
	store := &Store{Path: path}
	_, err := store.List()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("reserved in file = %v, want ErrInvalid", err)
	}
}

func TestListOrderLowerNameThenSlug(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: b
    name: Beta
  - slug: a
    name: beta
  - slug: c
    name: Alpha
`)
	store := &Store{Path: path}
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "a", "b"}
	if slugs(got)[0] != want[0] || slugs(got)[1] != want[1] || slugs(got)[2] != want[2] {
		t.Fatalf("order = %v, want %v", slugs(got), want)
	}
}

func TestCreateVisibleToGet(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "agents.yml")}
	got, err := store.Create(Bot{Slug: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "nova" {
		t.Fatalf("empty name default = %q, want slug", got.Name)
	}
	loaded, err := store.Get("nova")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Slug != "nova" || loaded.Name != "nova" {
		t.Fatalf("Get after Create = %+v", loaded)
	}
}

func TestCreateDuplicateConflict(t *testing.T) {
	path := writeRoster(t, `
bots:
  - slug: live
    name: Live
  - slug: old
    name: Old
    archived: true
`)
	store := &Store{Path: path}
	_, err := store.Create(Bot{Slug: "live", Name: "Again"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("active duplicate = %v, want ErrConflict", err)
	}
	_, err = store.Create(Bot{Slug: "old", Name: "Again"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("archived duplicate = %v, want ErrConflict", err)
	}
}

func TestCreateReserved(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "agents.yml")}
	_, err := store.Create(Bot{Slug: "a2a"})
	if !errors.Is(err, ErrReserved) {
		t.Fatalf("a2a = %v, want ErrReserved", err)
	}
	_, err = store.Create(Bot{Slug: "_hidden"})
	if !errors.Is(err, ErrReserved) {
		t.Fatalf("_prefix = %v, want ErrReserved", err)
	}
}

func TestCreateConcurrentDifferentSlugs(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "agents.yml")}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = store.Create(Bot{Slug: "one", Name: "One"})
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = store.Create(Bot{Slug: "two", Name: "Two"})
	}()
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	one, err := store.Get("one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Get("two")
	if err != nil {
		t.Fatal(err)
	}
	if one.Slug != "one" || two.Slug != "two" {
		t.Fatalf("lost bot: %+v %+v", one, two)
	}
}

func writeRoster(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agents.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func slugs(bots []Bot) []string {
	out := make([]string, len(bots))
	for i, b := range bots {
		out[i] = b.Slug
	}
	return out
}
