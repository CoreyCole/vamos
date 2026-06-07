package qrspi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"gopkg.in/yaml.v3"
)

// Deprecated compatibility aliases are kept until parser tests and downstream helpers
// are migrated from XML names to YAML result names in later slices.
type ResultXML = Result
type WorkspaceMetadataXML = WorkspaceMetadata
type PolicyXML = PolicyYAML
type SummaryXML = Summary
type ArtifactXML = Artifact
type NextXML = Next
type QRSPIXMLParser = QRSPIResultParser

type Result struct {
	Project           string            `yaml:"project" json:"project,omitempty"`
	RelatedProjects   []string          `yaml:"related_projects" json:"relatedProjects,omitempty"`
	Stage             string            `yaml:"stage" json:"stage"`
	Status            string            `yaml:"status" json:"status"`
	Outcome           string            `yaml:"outcome" json:"outcome,omitempty"`
	Workspace         string            `yaml:"workspace" json:"workspace,omitempty"`
	WorkspaceMetadata WorkspaceMetadata `yaml:"workspace_metadata" json:"workspaceMetadata,omitempty"`
	Policy            PolicyYAML        `yaml:"policy" json:"policy"`
	Summary           Summary           `yaml:"summary" json:"summary"`
	Artifact          string            `yaml:"artifact" json:"artifact"`
	Artifacts         []Artifact        `yaml:"artifacts" json:"artifacts,omitempty"`
	Next              Next              `yaml:"next" json:"next"`
}

type resultEnvelope struct {
	Result Result `yaml:"qrspi_result"`
}

type WorkspaceMetadata struct {
	PlanWorkspace           string `yaml:"plan_workspace" json:"planWorkspace,omitempty"`
	ImplementationWorkspace string `yaml:"implementation_workspace" json:"implementationWorkspace,omitempty"`
	TrunkBranch             string `yaml:"trunk_branch" json:"trunkBranch,omitempty"`
	StackBottomBranch       string `yaml:"stack_bottom_branch" json:"stackBottomBranch,omitempty"`
	ParentBranch            string `yaml:"parent_branch" json:"parentBranch,omitempty"`
	CurrentBranch           string `yaml:"current_branch" json:"currentBranch,omitempty"`
}

type PolicyYAML struct {
	AdvanceMode             AdvanceMode `yaml:"advance_mode" json:"advanceMode,omitempty"`
	AutoMode                bool        `yaml:"auto_mode" json:"autoMode"`
	EnablePlanReviews       bool        `yaml:"enable_plan_reviews" json:"enablePlanReviews"`
	InvalidResultRetryLimit int         `yaml:"invalid_result_retry_limit" json:"invalidResultRetryLimit"`
}

type Summary struct {
	PlanGoal       string `yaml:"plan_goal" json:"plan_goal,omitempty"`
	StageCompleted string `yaml:"stage_completed" json:"stage_completed,omitempty"`
	KeyDecisions   string `yaml:"key_decisions" json:"key_decisions,omitempty"`
}

type Artifact struct {
	Role string `yaml:"role" json:"role,omitempty"`
	Path string `yaml:"path" json:"path"`
}

type Next struct {
	Steps []NextStep `yaml:"steps" json:"steps,omitempty"`
}

type NextAction string

const (
	NextActionReadSkill            NextAction = "read_skill"
	NextActionReadArtifact         NextAction = "read_artifact"
	NextActionStartStage           NextAction = "start_stage"
	NextActionRequestHumanApproval NextAction = "request_human_approval"
)

type NextStep struct {
	Action NextAction `yaml:"action" json:"action"`
	Param  string     `yaml:"param" json:"param"`
}

func (m WorkspaceMetadata) Trimmed() WorkspaceMetadata {
	return WorkspaceMetadata{
		PlanWorkspace:           strings.TrimSpace(m.PlanWorkspace),
		ImplementationWorkspace: strings.TrimSpace(m.ImplementationWorkspace),
		TrunkBranch:             strings.TrimSpace(m.TrunkBranch),
		StackBottomBranch:       strings.TrimSpace(m.StackBottomBranch),
		ParentBranch:            strings.TrimSpace(m.ParentBranch),
		CurrentBranch:           strings.TrimSpace(m.CurrentBranch),
	}
}

func (n Next) Trimmed() Next {
	steps := make([]NextStep, 0, len(n.Steps))
	for _, step := range n.Steps {
		step.Action = NextAction(strings.TrimSpace(string(step.Action)))
		step.Param = strings.TrimSpace(step.Param)
		if step.Action == "" && step.Param == "" {
			continue
		}
		steps = append(steps, step)
	}
	return Next{Steps: steps}
}

func (n Next) DisplayText() string {
	n = n.Trimmed()
	lines := make([]string, 0, len(n.Steps))
	for _, step := range n.Steps {
		lines = append(lines, step.DisplayText())
	}
	return strings.Join(lines, "\n")
}

func (s NextStep) DisplayText() string {
	s.Param = strings.TrimSpace(s.Param)
	s.Action = NextAction(strings.TrimSpace(string(s.Action)))
	switch s.Action {
	case NextActionReadSkill:
		return "Read skill: " + s.Param
	case NextActionReadArtifact:
		return "Read artifact: " + s.Param
	case NextActionStartStage:
		return "Start stage: " + s.Param
	case NextActionRequestHumanApproval:
		return "Request human approval: " + s.Param
	default:
		return strings.TrimSpace(string(s.Action) + ": " + s.Param)
	}
}

func (a NextAction) Valid() bool {
	switch a {
	case NextActionReadSkill, NextActionReadArtifact, NextActionStartStage, NextActionRequestHumanApproval:
		return true
	default:
		return false
	}
}

func (s Summary) TextContent() string {
	return normalizeResultText(strings.Join([]string{s.PlanGoal, s.StageCompleted, s.KeyDecisions}, " "))
}

func WorkflowResultProject(result wruntime.WorkflowResult) string {
	if len(result.Raw) == 0 {
		return ""
	}
	var parsed Result
	if err := json.Unmarshal(result.Raw, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Project)
}

func WorkflowResultWorkspaceMetadata(result wruntime.WorkflowResult) WorkspaceMetadata {
	if len(result.Raw) == 0 {
		return WorkspaceMetadata{}
	}
	var parsed Result
	if err := json.Unmarshal(result.Raw, &parsed); err != nil {
		return WorkspaceMetadata{}
	}
	return parsed.WorkspaceMetadata.Trimmed()
}

func WorkflowResultImplementationWorkspace(result wruntime.WorkflowResult) string {
	return WorkflowResultWorkspaceMetadata(result).ImplementationWorkspace
}

func QRSPIResultProject(result Result) string {
	return strings.TrimSpace(result.Project)
}

func QRSPIResultWorkspaceMetadata(result Result) WorkspaceMetadata {
	return result.WorkspaceMetadata.Trimmed()
}

type QRSPIResultParser struct{}

func (QRSPIResultParser) Parse(output string, ctx wruntime.ParseContext) (any, error) {
	yamlText, err := extractQRSPIResultYAML(output)
	if err != nil {
		return nil, err
	}
	parsed, err := decodeQRSPIResultYAML(yamlText)
	if err != nil {
		return nil, err
	}
	parsed = trimQRSPIResult(parsed)
	if err := validateQRSPIResult(parsed, ctx); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (QRSPIResultParser) CorrectionPrompt(err error, attempt int) string {
	return fmt.Sprintf(
		"Your previous response did not contain a valid QRSPI workflow result (%v). Re-emit only one corrected fenced YAML block with top-level qrspi_result for attempt %d. Review stages must use canonical stage IDs review-outline, review-plan, or review-implementation.",
		err,
		attempt,
	)
}

var fencedYAMLPattern = regexp.MustCompile("(?s)```(?:yaml|yml)\\s*\\n(.*?)\\n?```")

func ExtractQRSPIResultYAML(output string) (string, error) {
	return extractQRSPIResultYAML(output)
}

func extractQRSPIResultYAML(output string) (string, error) {
	for _, match := range fencedYAMLPattern.FindAllStringSubmatch(output, -1) {
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			continue
		}
		if hasQRSPIResultRoot(candidate) {
			return candidate, nil
		}
	}
	whole := strings.TrimSpace(output)
	if hasOnlyQRSPIResultRoot(whole) {
		return whole, nil
	}
	return "", fmt.Errorf("missing fenced YAML qrspi_result")
}

func decodeQRSPIResultYAML(yamlText string) (Result, error) {
	var envelope resultEnvelope
	decoder := yaml.NewDecoder(strings.NewReader(yamlText))
	decoder.KnownFields(true)
	if err := decoder.Decode(&envelope); err != nil {
		return Result{}, fmt.Errorf("parse qrspi result YAML: %w", err)
	}
	if strings.TrimSpace(envelope.Result.Stage) == "" && strings.TrimSpace(envelope.Result.Status) == "" {
		return Result{}, fmt.Errorf("missing top-level qrspi_result")
	}
	return envelope.Result, nil
}

func hasQRSPIResultRoot(text string) bool {
	var root map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&root); err != nil {
		return false
	}
	_, ok := root["qrspi_result"]
	return ok
}

func hasOnlyQRSPIResultRoot(text string) bool {
	var root map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&root); err != nil {
		return false
	}
	_, ok := root["qrspi_result"]
	return ok && len(root) == 1
}

func validateQRSPIResult(parsed Result, ctx wruntime.ParseContext) error {
	if parsed.Stage == "" {
		return fmt.Errorf("qrspi result stage is required")
	}
	if parsed.Stage == "review" {
		return fmt.Errorf(
			"ambiguous qrspi review stage %q; emit review-outline, review-plan, or review-implementation",
			parsed.Stage,
		)
	}
	if ctx.ExpectedNodeID != "" && parsed.Stage != string(ctx.ExpectedNodeID) {
		return fmt.Errorf(
			"qrspi result stage %q does not match expected workflow node %q",
			parsed.Stage,
			ctx.ExpectedNodeID,
		)
	}
	if parsed.Status == "" {
		return fmt.Errorf("qrspi result status is required")
	}
	if parsed.Status == string(wruntime.StatusComplete) && parsed.Outcome == "" {
		return fmt.Errorf("qrspi result outcome is required when status is complete")
	}
	if parsed.Summary.TextContent() == "" {
		return fmt.Errorf("qrspi result summary is required")
	}
	for i, step := range parsed.Next.Steps {
		if !step.Action.Valid() {
			return fmt.Errorf("qrspi result next.steps[%d].action %q is invalid", i, step.Action)
		}
		if strings.TrimSpace(step.Param) == "" {
			return fmt.Errorf("qrspi result next.steps[%d].param is required", i)
		}
	}
	return nil
}

func trimQRSPIResult(parsed Result) Result {
	parsed.Project = strings.TrimSpace(parsed.Project)
	for i := range parsed.RelatedProjects {
		parsed.RelatedProjects[i] = strings.TrimSpace(parsed.RelatedProjects[i])
	}
	parsed.Stage = strings.TrimSpace(parsed.Stage)
	parsed.Status = strings.TrimSpace(parsed.Status)
	parsed.Outcome = strings.TrimSpace(parsed.Outcome)
	parsed.Workspace = strings.TrimSpace(parsed.Workspace)
	parsed.WorkspaceMetadata = parsed.WorkspaceMetadata.Trimmed()
	parsed.Policy.AdvanceMode = AdvanceMode(strings.TrimSpace(string(parsed.Policy.AdvanceMode)))
	parsed.Summary.PlanGoal = strings.TrimSpace(parsed.Summary.PlanGoal)
	parsed.Summary.StageCompleted = strings.TrimSpace(parsed.Summary.StageCompleted)
	parsed.Summary.KeyDecisions = strings.TrimSpace(parsed.Summary.KeyDecisions)
	parsed.Artifact = strings.TrimSpace(parsed.Artifact)
	for i := range parsed.Artifacts {
		parsed.Artifacts[i].Role = strings.TrimSpace(parsed.Artifacts[i].Role)
		parsed.Artifacts[i].Path = strings.TrimSpace(parsed.Artifacts[i].Path)
	}
	parsed.Next = parsed.Next.Trimmed()
	return parsed
}

type QRSPIResultConverter struct{}

func (QRSPIResultConverter) ToWorkflowResult(
	result any,
	ctx wruntime.ParseContext,
) (wruntime.WorkflowResult, error) {
	parsed, ok := result.(Result)
	if !ok {
		return wruntime.WorkflowResult{}, fmt.Errorf(
			"expected qrspi Result, got %T",
			result,
		)
	}
	raw, _ := json.Marshal(parsed)
	policy, _ := json.Marshal(
		Policy{
			AdvanceMode:             parsed.Policy.AdvanceMode,
			AutoMode:                parsed.Policy.AutoMode,
			EnablePlanReviews:       parsed.Policy.EnablePlanReviews,
			InvalidResultRetryLimit: parsed.Policy.InvalidResultRetryLimit,
		},
	)
	artifacts := make([]wruntime.ArtifactRef, 0, 1+len(parsed.Artifacts))
	if strings.TrimSpace(parsed.Artifact) != "" {
		artifacts = append(
			artifacts,
			wruntime.ArtifactRef{
				Role: "primary",
				Path: strings.TrimSpace(parsed.Artifact),
			},
		)
	}
	for _, artifact := range parsed.Artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path == "" {
			continue
		}
		role := strings.TrimSpace(artifact.Role)
		if role == "" {
			role = "related"
		}
		artifacts = append(artifacts, wruntime.ArtifactRef{Role: role, Path: path})
	}
	return wruntime.WorkflowResult{
		WorkflowType:    ctx.WorkflowType,
		SourceNodeID:    wruntime.NodeID(parsed.Stage),
		Status:          wruntime.ResultStatus(parsed.Status),
		Outcome:         wruntime.ResultOutcome(parsed.Outcome),
		Summary:         parsed.Summary.TextContent(),
		PrimaryArtifact: strings.TrimSpace(parsed.Artifact),
		Artifacts:       artifacts,
		Workspace:       strings.TrimSpace(parsed.Workspace),
		DisplayNext:     parsed.Next.DisplayText(),
		Policy:          policy,
		Evidence: wruntime.EvidenceRef{
			RunID:       ctx.RunID,
			ThreadID:    ctx.ThreadID,
			SessionID:   ctx.SessionID,
			HeadEntryID: ctx.HeadEntryID,
			SessionPath: ctx.SessionPath,
		},
		Raw: raw,
	}, nil
}

func ValidateOutcomeArtifacts(result wruntime.WorkflowResult) error {
	roles := artifactRoles(result)
	switch result.Outcome {
	case wruntime.OutcomeNeedsReviewResearch:
		if len(roles["followup-questions"]) == 0 && len(roles["questions"]) == 0 {
			return fmt.Errorf(
				"outcome %q requires followup questions artifact",
				result.Outcome,
			)
		}
	case wruntime.OutcomeNeedsFollowup:
		if len(roles["followup-plan"]) == 0 && len(roles["followup-questions"]) == 0 {
			return fmt.Errorf(
				"outcome %q requires followup plan or questions artifact",
				result.Outcome,
			)
		}
	}
	return nil
}

func artifactRoles(result wruntime.WorkflowResult) map[string][]string {
	roles := map[string][]string{}
	if path := strings.TrimSpace(result.PrimaryArtifact); path != "" {
		roles["primary"] = append(roles["primary"], path)
	}
	for _, artifact := range result.Artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path == "" {
			continue
		}
		role := strings.TrimSpace(artifact.Role)
		if role == "" {
			role = "related"
		}
		roles[role] = append(roles[role], path)
	}
	return roles
}

func normalizeResultText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
