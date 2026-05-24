package selectors

import "testing"

func TestDefaultCatalogResolvesKnownKeys(t *testing.T) {
	catalog := DefaultCatalog()
	entry, err := catalog.Resolve("thoughts.workbench.sidebar")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if entry.CSS == "" {
		t.Fatal("Resolve() CSS empty")
	}
}

func TestDefaultCatalogRejectsUnknownKeys(t *testing.T) {
	catalog := DefaultCatalog()
	if _, err := catalog.Resolve("missing"); err == nil {
		t.Fatal("Resolve() error = nil, want unknown key")
	}
}
