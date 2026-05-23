package runtime

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type parserStub struct{}

func (parserStub) Parse(string, ParseContext) (any, error) { return nil, nil }
func (parserStub) CorrectionPrompt(error, int) string      { return "" }

type converterStub struct{}

func (converterStub) ToWorkflowResult(any, ParseContext) (WorkflowResult, error) {
	return WorkflowResult{}, nil
}

func validBuilder() *Builder[struct{}] {
	return New[struct{}]("test").
		Start("start").
		Agent("start", PromptSpec{Static: "run"}).
		Done("done").
		Edge("start", "done").
		ResultParser(parserStub{}).
		ResultConverter(converterStub{})
}

func TestValidateDefinition(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (Definition, error)
		wantErr string
	}{
		{
			name:  "valid definition",
			build: validBuilder().Build,
		},
		{
			name: "duplicate node",
			build: func() (Definition, error) {
				return validBuilder().Agent("start", PromptSpec{Static: "duplicate"}).
					Build()
			},
			wantErr: "duplicate node",
		},
		{
			name: "missing start",
			build: func() (Definition, error) {
				return New[struct{}]("test").Agent("start", PromptSpec{Static: "run"}).
					Done("done").
					ResultParser(parserStub{}).
					ResultConverter(converterStub{}).
					Build()
			},
			wantErr: "start node is required",
		},
		{
			name: "missing edge endpoint",
			build: func() (Definition, error) {
				return New[struct{}]("test").Start("start").
					Agent("start", PromptSpec{Static: "run"}).
					Done("done").
					Edge("start", "missing").
					ResultParser(parserStub{}).
					ResultConverter(converterStub{}).
					Build()
			},
			wantErr: "edge to",
		},
		{
			name: "missing parser",
			build: func() (Definition, error) {
				return New[struct{}]("test").Start("start").
					Agent("start", PromptSpec{Static: "run"}).
					Done("done").
					Edge("start", "done").
					ResultConverter(converterStub{}).
					Build()
			},
			wantErr: "result parser is required",
		},
		{
			name: "missing converter",
			build: func() (Definition, error) {
				return New[struct{}]("test").Start("start").
					Agent("start", PromptSpec{Static: "run"}).
					Done("done").
					Edge("start", "done").
					ResultParser(parserStub{}).
					Build()
			},
			wantErr: "result converter is required",
		},
		{
			name: "no terminal",
			build: func() (Definition, error) {
				return New[struct{}]("test").Start("start").
					Agent("start", PromptSpec{Static: "run"}).
					ResultParser(parserStub{}).
					ResultConverter(converterStub{}).
					Build()
			},
			wantErr: "definition has no terminal node",
		},
		{
			name: "agent prompt required",
			build: func() (Definition, error) {
				return New[struct{}]("test").Start("start").
					Agent("start", PromptSpec{}).
					Done("done").
					Edge("start", "done").
					ResultParser(parserStub{}).
					ResultConverter(converterStub{}).
					Build()
			},
			wantErr: "has no prompt",
		},
		{
			name: "invalid default policy",
			build: func() (Definition, error) {
				return validBuilder().PolicySpec(PolicySpec{Defaults: []byte(`{"bad": true}`), Validate: func(json.RawMessage) error { return errors.New("bad policy") }}).
					Build()
			},
			wantErr: "default policy invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Build() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Build() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	def, err := validBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(def); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got, ok := registry.Get("test"); !ok || got.ID != "test" {
		t.Fatalf("Get() = (%v, %v), want definition", got.ID, ok)
	}
	if err := registry.Register(
		def,
	); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("second Register() error = %v, want duplicate", err)
	}
}
