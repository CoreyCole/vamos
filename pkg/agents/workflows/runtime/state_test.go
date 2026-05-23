package runtime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInitialState(t *testing.T) {
	def, err := validBuilder().PolicySpec(PolicySpec{
		Defaults: json.RawMessage(`{"enabled":true}`),
		Validate: func(raw json.RawMessage) error {
			if string(raw) == `{"bad":true}` {
				return errBadPolicy{}
			}
			return nil
		},
	}).Build()
	if err != nil {
		t.Fatal(err)
	}

	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatalf("InitialState() error = %v", err)
	}
	if state.Type != "test" || state.Version != "v1" || state.CurrentNodeID != "start" ||
		state.Status != WorkspaceStatusIdle {
		t.Fatalf("state = %+v", state)
	}
	if string(state.Policy) != `{"enabled":true}` {
		t.Fatalf("policy = %s", state.Policy)
	}
	if len(state.Nodes) != len(def.Nodes) {
		t.Fatalf("nodes len = %d, want %d", len(state.Nodes), len(def.Nodes))
	}
	for id, node := range state.Nodes {
		if node.Status != NodeStatusPending {
			t.Fatalf("node %q status = %q, want pending", id, node.Status)
		}
	}
}

func TestInitialStateInvalidPolicy(t *testing.T) {
	def, err := validBuilder().PolicySpec(PolicySpec{
		Validate: func(raw json.RawMessage) error {
			if string(raw) == `{"bad":true}` {
				return errBadPolicy{}
			}
			return nil
		},
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	_, err = InitialState(def, json.RawMessage(`{"bad":true}`))
	if err == nil || !strings.Contains(err.Error(), "bad policy") {
		t.Fatalf("InitialState() error = %v, want bad policy", err)
	}
}

func TestValidateState(t *testing.T) {
	def, err := validBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	state, err := InitialState(def, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid", func(t *testing.T) {
		if err := ValidateState(def, state); err != nil {
			t.Fatalf("ValidateState() error = %v", err)
		}
	})

	t.Run("type mismatch", func(t *testing.T) {
		bad := state
		bad.Type = "other"
		if err := ValidateState(
			def,
			bad,
		); err == nil ||
			!strings.Contains(err.Error(), "does not match") {
			t.Fatalf("ValidateState() error = %v, want type mismatch", err)
		}
	})

	t.Run("invalid current node", func(t *testing.T) {
		bad := state
		bad.CurrentNodeID = "missing"
		if err := ValidateState(
			def,
			bad,
		); err == nil ||
			!strings.Contains(err.Error(), "current node") {
			t.Fatalf("ValidateState() error = %v, want current node", err)
		}
	})

	t.Run("missing attempts", func(t *testing.T) {
		bad := state
		bad.Attempts = nil
		if err := ValidateState(
			def,
			bad,
		); err == nil ||
			!strings.Contains(err.Error(), "attempts is required") {
			t.Fatalf("ValidateState() error = %v, want attempts", err)
		}
	})

	t.Run("missing nodes", func(t *testing.T) {
		bad := state
		bad.Nodes = nil
		if err := ValidateState(
			def,
			bad,
		); err == nil ||
			!strings.Contains(err.Error(), "nodes is required") {
			t.Fatalf("ValidateState() error = %v, want nodes", err)
		}
	})
}

type errBadPolicy struct{}

func (errBadPolicy) Error() string { return "bad policy" }
