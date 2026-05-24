package story

type Feature struct {
	Slug          string
	Title         string
	UserStory     string
	BusinessRules []string
	Scenarios     []Scenario
	Properties    []Property
	SourcePath    string
}

type Scenario struct {
	Slug     string
	Title    string
	Viewport string
	Given    []Step
	When     []Step
	Then     []Step
}

type Step struct {
	Kind   StepKind
	Verb   StepVerb
	Args   map[string]string
	Source SourceRange
}

type (
	StepKind string
	StepVerb string
)

type SourceRange struct{ Line int }

type Property struct {
	Slug       string
	Title      string
	Dimensions []Dimension
	Then       []Step
}

type Dimension struct {
	Name   string
	Values []string
}

type ParseOptions struct{ Strict bool }

type (
	StepResolver    interface{ ResolveStep(step Step) error }
	FixtureResolver interface{ HasFixture(name string) bool }
)
