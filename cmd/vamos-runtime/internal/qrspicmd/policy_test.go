package qrspicmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
)

func TestRunSetPolicyFast(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	state := ManagerState{Workflow: testWorkflowState(t, qrspi.NodePlan, nil)}
	saveManagerState(t, stateFile, state)

	var out bytes.Buffer
	err := RunSetPolicy(
		t.Context(),
		SetPolicyOptions{StateFile: stateFile, Preset: "fast"},
		deps{},
		&out,
	)
	if err != nil {
		t.Fatalf("RunSetPolicy error = %v", err)
	}
	if !strings.Contains(out.String(), "policy: autopilot, plan reviews off") {
		t.Fatalf("output = %q", out.String())
	}

	loaded, err := (FileStateStore{}).Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var policy qrspi.Policy
	if err := json.Unmarshal(loaded.Workflow.Policy, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.EffectiveAdvanceMode() != qrspi.AdvanceModeAutopilot ||
		policy.EnablePlanReviews ||
		!policy.AutoMode {
		t.Fatalf("policy = %#v", policy)
	}
}

func TestInitialPolicyRejectsFileAndPreset(t *testing.T) {
	_, err := initialPolicy("policy.json", "fast")
	if err == nil ||
		!strings.Contains(err.Error(), "either --policy-file or --policy-preset") {
		t.Fatalf("initialPolicy error = %v", err)
	}
}
