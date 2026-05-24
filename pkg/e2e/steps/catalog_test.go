package steps

import (
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

func TestDefaultCatalogResolvesAllVerbs(t *testing.T) {
	catalog := DefaultCatalog()
	verbs := []story.StepVerb{
		"authenticated_as",
		"load_fixture",
		"visit",
		"wait_for_feature_ready",
		"expect_region_visible",
		"expect_region_reachable",
		"expect_tab_selected",
		"expect_text_absent",
	}
	for _, verb := range verbs {
		if err := catalog.ResolveStep(
			story.Step{
				Verb: verb,
				Args: map[string]string{
					"email":   "a",
					"name":    "f",
					"path":    "/",
					"feature": "x",
					"key":     "k",
					"text":    "t",
				},
			},
		); err != nil {
			t.Fatalf("ResolveStep(%s) error = %v", verb, err)
		}
	}
}

func TestCompileCallEmitsQuotedLiterals(t *testing.T) {
	catalog := DefaultCatalog()
	def, _, err := catalog.Resolve(
		story.Step{Verb: "visit", Args: map[string]string{"path": "/"}},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	call, err := def.Compile(story.Step{Args: map[string]string{"path": "/"}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got, want := call.Function, "Visit"; got != want {
		t.Fatalf("function=%q want %q", got, want)
	}
	if got, want := string(call.Args[0]), `"/"`; got != want {
		t.Fatalf("arg=%q want %q", got, want)
	}
}
