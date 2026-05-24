package fixtures

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

const DurableFreeformFixture = "freeform-chat.durable"

type State struct {
	Name string
	Data map[string]any
}

type WorkspaceIdentity struct {
	Slug         string
	CheckoutPath string
	DBPath       string
	ManagerURL   string
}

type Input struct {
	Workspace WorkspaceIdentity
	Params    map[string]string
}

type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Registry interface {
	Resolve(name string) (Builder, error)
	HasFixture(name string) bool
	Names() []string
}

type (
	Builder  func(context.Context, DBTX, Input) (State, error)
	registry map[string]Builder
)

func DefaultRegistry() Registry {
	return registry{
		"thoughts-workbench.basic": stubFixture("thoughts-workbench.basic"),
		DurableFreeformFixture:     stubFixture(DurableFreeformFixture),
	}
}

func Load(
	ctx context.Context,
	db DBTX,
	workspace WorkspaceIdentity,
	name string,
) (State, error) {
	builder, err := DefaultRegistry().Resolve(name)
	if err != nil {
		return State{}, err
	}
	return builder(ctx, db, Input{Workspace: workspace, Params: map[string]string{}})
}

func (r registry) Resolve(name string) (Builder, error) {
	b, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("unknown fixture %q", name)
	}
	return b, nil
}

func (r registry) HasFixture(name string) bool { _, ok := r[name]; return ok }

func (r registry) Names() []string {
	out := make([]string, 0, len(r))
	for name := range r {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func stubFixture(name string) Builder {
	return func(context.Context, DBTX, Input) (State, error) {
		return State{Name: name, Data: map[string]any{}}, nil
	}
}
