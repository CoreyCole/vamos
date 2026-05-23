package qrspi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type ResultXML struct {
	XMLName   xml.Name      `xml:"qrspi-result"       json:"-"`
	Stage     string        `xml:"stage"              json:"stage"`
	Status    string        `xml:"status"             json:"status"`
	Outcome   string        `xml:"outcome"            json:"outcome,omitempty"`
	Workspace string        `xml:"workspace"          json:"workspace,omitempty"`
	Policy    PolicyXML     `xml:"policy"             json:"policy"`
	Summary   SummaryXML    `xml:"summary"            json:"summary"`
	Artifact  string        `xml:"artifact"           json:"artifact"`
	Artifacts []ArtifactXML `xml:"artifacts>artifact" json:"artifacts,omitempty"`
	Next      string        `xml:"next"               json:"next"`
}

type PolicyXML struct {
	AutoMode                bool `xml:"autoMode"                json:"autoMode"`
	EnablePlanReviews       bool `xml:"enablePlanReviews"       json:"enablePlanReviews"`
	InvalidResultRetryLimit int  `xml:"invalidResultRetryLimit" json:"invalidResultRetryLimit"`
}

type SummaryXML struct {
	PlanGoal       string `xml:"plan-goal"       json:"plan_goal,omitempty"`
	StageCompleted string `xml:"stage-completed" json:"stage_completed,omitempty"`
	KeyDecisions   string `xml:"key-decisions"   json:"key_decisions,omitempty"`
	Text           string `xml:",chardata"       json:"text,omitempty"`
	InnerXML       string `xml:",innerxml"       json:"inner_xml,omitempty"`
}

type ArtifactXML struct {
	Role string `xml:"role,attr" json:"role,omitempty"`
	Path string `xml:",chardata" json:"path"`
}

type QRSPIXMLParser struct{}

func (QRSPIXMLParser) Parse(output string, ctx wruntime.ParseContext) (any, error) {
	xmlText, err := extractQRSPIXML(output)
	if err != nil {
		return nil, err
	}
	var parsed ResultXML
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	decoder.Strict = true
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse qrspi result XML: %w", err)
	}

	parsed.Stage = strings.TrimSpace(parsed.Stage)
	parsed.Status = strings.TrimSpace(parsed.Status)
	parsed.Outcome = strings.TrimSpace(parsed.Outcome)
	parsed.Workspace = strings.TrimSpace(parsed.Workspace)
	parsed.Artifact = strings.TrimSpace(parsed.Artifact)
	parsed.Next = strings.TrimSpace(parsed.Next)
	if parsed.Stage == "" {
		return nil, fmt.Errorf("qrspi result stage is required")
	}
	if parsed.Stage == "review" {
		return nil, fmt.Errorf(
			"ambiguous qrspi review stage %q; emit review-design, review-outline, review-plan, or review-implementation",
			parsed.Stage,
		)
	}
	if ctx.ExpectedNodeID != "" && parsed.Stage != string(ctx.ExpectedNodeID) {
		return nil, fmt.Errorf(
			"qrspi result stage %q does not match expected workflow node %q",
			parsed.Stage,
			ctx.ExpectedNodeID,
		)
	}
	if parsed.Status == "" {
		return nil, fmt.Errorf("qrspi result status is required")
	}
	if parsed.Status == string(wruntime.StatusComplete) && parsed.Outcome == "" {
		return nil, fmt.Errorf("qrspi result outcome is required when status is complete")
	}
	if parsed.Summary.TextContent() == "" {
		return nil, fmt.Errorf("qrspi result summary is required")
	}
	return parsed, nil
}

func (QRSPIXMLParser) CorrectionPrompt(err error, attempt int) string {
	return fmt.Sprintf(
		"Your previous response did not contain a valid QRSPI workflow result (%v). Re-emit only the corrected <qrspi-result> XML for attempt %d. Review stages must use canonical stage IDs review-design, review-outline, review-plan, or review-implementation.",
		err,
		attempt,
	)
}

var qrspiXMLPattern = regexp.MustCompile(`(?s)<qrspi-result>.*?</qrspi-result>`)

func extractQRSPIXML(output string) (string, error) {
	match := qrspiXMLPattern.FindString(output)
	if strings.TrimSpace(match) == "" {
		return "", fmt.Errorf("missing <qrspi-result> XML")
	}
	return strings.TrimSpace(match), nil
}

type QRSPIResultConverter struct{}

func (QRSPIResultConverter) ToWorkflowResult(
	result any,
	ctx wruntime.ParseContext,
) (wruntime.WorkflowResult, error) {
	parsed, ok := result.(ResultXML)
	if !ok {
		return wruntime.WorkflowResult{}, fmt.Errorf(
			"expected qrspi ResultXML, got %T",
			result,
		)
	}
	raw, _ := json.Marshal(parsed)
	policy, _ := json.Marshal(
		Policy{
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
		DisplayNext:     strings.TrimSpace(parsed.Next),
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

func (s SummaryXML) TextContent() string {
	parts := []string{s.PlanGoal, s.StageCompleted, s.KeyDecisions, s.Text}
	text := normalizeXMLText(strings.Join(parts, " "))
	if text != "" {
		return text
	}
	return normalizeXMLText(stripXMLTags(s.InnerXML))
}

func normalizeXMLText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func stripXMLTags(value string) string {
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, " ")
}
