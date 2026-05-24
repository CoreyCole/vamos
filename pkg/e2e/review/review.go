package review

import "context"

type VisualReviewInput struct {
	RunManifestPath string
	BaselineRef     string
	BaselineCommit  string
	WorkspaceCommit string
	PlanDir         string
	SkillName       string
}

type VisualReviewResult struct {
	Verdict         string
	ArtifactPath    string
	Classifications []VisualDifference
}

type VisualDifference struct {
	Story          string
	Scenario       string
	Viewport       string
	Classification string
	Rationale      string
}

func RunVisualReview(
	ctx context.Context,
	input VisualReviewInput,
) (VisualReviewResult, error) {
	return VisualReviewResult{
		Verdict: "needs-human-review",
		Classifications: []VisualDifference{
			{
				Classification: "needs human decision",
				Rationale:      "Pi invocation adapter not configured in deterministic CLI yet; artifact records inputs for project skill review.",
			},
		},
	}, nil
}
