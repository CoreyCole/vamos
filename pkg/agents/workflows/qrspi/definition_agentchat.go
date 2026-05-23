package qrspi

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

const AgentChatWorkflowType wruntime.WorkflowID = "qrspi"

const (
	NodeQuestion wruntime.NodeID = "question"
	NodeResearch wruntime.NodeID = "research"
	NodeDesign   wruntime.NodeID = "design"

	NodeReviewDesign                wruntime.NodeID = "review-design"
	NodeResearchForReviewDesign     wruntime.NodeID = "research-for-review-design"
	NodeAddressReviewResearchDesign wruntime.NodeID = "address-review-research-design"

	NodeOutline                      wruntime.NodeID = "outline"
	NodeReviewOutline                wruntime.NodeID = "review-outline"
	NodeHumanReviewOutline           wruntime.NodeID = "human-review-outline"
	NodeResearchForReviewOutline     wruntime.NodeID = "research-for-review-outline"
	NodeAddressReviewResearchOutline wruntime.NodeID = "address-review-research-outline"

	NodePlan                      wruntime.NodeID = "plan"
	NodeReviewPlan                wruntime.NodeID = "review-plan"
	NodeResearchForReviewPlan     wruntime.NodeID = "research-for-review-plan"
	NodeAddressReviewResearchPlan wruntime.NodeID = "address-review-research-plan"

	NodeWorkspace                 wruntime.NodeID = "workspace"
	NodeImplement                 wruntime.NodeID = "implement"
	NodeReviewImplementation      wruntime.NodeID = "review-implementation"
	NodeHumanReviewImplementation wruntime.NodeID = "human-review-implementation"
	NodeDone                      wruntime.NodeID = "done"
)

func Skill(path string) wruntime.PromptSpec {
	return wruntime.PromptSpec{SkillPath: path}
}

func Definition() (wruntime.Definition, error) {
	return wruntime.New[Config](AgentChatWorkflowType).
		Config(DefaultConfig(), ValidateConfig).
		Version("v1").Name("QRSPI").Start(NodeQuestion).
		Agent(NodeQuestion, Skill("~/.agents/skills/q-question/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeResearch, Skill("~/.agents/skills/q-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeDesign, Skill("~/.agents/skills/q-design/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewDesign, Skill("~/.agents/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForOutline, wruntime.OutcomeNeedsReviewResearch).
		RequiresPrimaryArtifact().
		Agent(NodeResearchForReviewDesign, Skill("~/.agents/skills/q-research-for-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeAddressReviewResearchDesign, Skill("~/.agents/skills/q-address-review-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeOutline, Skill("~/.agents/skills/q-outline/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewOutline, Skill("~/.agents/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForHumanReview, wruntime.OutcomeNeedsReviewResearch).
		RequiresPrimaryArtifact().
		HumanReview(NodeHumanReviewOutline, "outline approved by human").
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		AutoApprovable(true).
		Agent(NodeResearchForReviewOutline, Skill("~/.agents/skills/q-research-for-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeAddressReviewResearchOutline, Skill("~/.agents/skills/q-address-review-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodePlan, Skill("~/.agents/skills/q-plan/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewPlan, Skill("~/.agents/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForWorkspace, wruntime.OutcomeNeedsReviewResearch).
		RequiresPrimaryArtifact().
		Agent(NodeResearchForReviewPlan, Skill("~/.agents/skills/q-research-for-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeAddressReviewResearchPlan, Skill("~/.agents/skills/q-address-review-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeWorkspace, Skill("~/.agents/skills/q-workspace/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeImplement, Skill("~/.agents/skills/q-implement/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewImplementation, Skill("~/.agents/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForHumanReview, wruntime.OutcomeNeedsFollowup).
		RequiresPrimaryArtifact().
		HumanReview(NodeHumanReviewImplementation, "implementation approved by human").
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		AutoApprovable(false).
		Done(NodeDone).
		From(NodeQuestion).On(wruntime.OutcomeComplete).GoTo(NodeResearch).
		From(NodeResearch).On(wruntime.OutcomeComplete).GoTo(NodeDesign).
		From(NodeDesign).When(ConfigPlanReviewsEnabled).GoTo(NodeReviewDesign).
		From(NodeDesign).When(ConfigPlanReviewsDisabled).GoTo(NodeOutline).
		From(NodeReviewDesign).
		On(wruntime.OutcomeNeedsReviewResearch).GoTo(NodeResearchForReviewDesign).
		From(NodeReviewDesign).On(wruntime.OutcomeReadyForOutline).GoTo(NodeOutline).
		From(NodeResearchForReviewDesign).
		On(wruntime.OutcomeComplete).GoTo(NodeAddressReviewResearchDesign).
		From(NodeAddressReviewResearchDesign).
		On(wruntime.OutcomeComplete).GoTo(NodeReviewDesign).
		From(NodeOutline).When(ConfigPlanReviewsEnabled).GoTo(NodeReviewOutline).
		From(NodeOutline).When(ConfigPlanReviewsDisabled).GoTo(NodePlan).
		From(NodeReviewOutline).
		On(wruntime.OutcomeNeedsReviewResearch).GoTo(NodeResearchForReviewOutline).
		From(NodeReviewOutline).
		On(wruntime.OutcomeReadyForHumanReview).GoTo(NodeHumanReviewOutline).
		From(NodeHumanReviewOutline).On(wruntime.OutcomeComplete).GoTo(NodePlan).
		From(NodeResearchForReviewOutline).
		On(wruntime.OutcomeComplete).GoTo(NodeAddressReviewResearchOutline).
		From(NodeAddressReviewResearchOutline).
		On(wruntime.OutcomeComplete).GoTo(NodeReviewOutline).
		From(NodePlan).When(ConfigPlanReviewsEnabled).GoTo(NodeReviewPlan).
		From(NodePlan).When(ConfigPlanReviewsDisabled).GoTo(NodeWorkspace).
		From(NodeReviewPlan).
		On(wruntime.OutcomeNeedsReviewResearch).GoTo(NodeResearchForReviewPlan).
		From(NodeReviewPlan).On(wruntime.OutcomeReadyForWorkspace).GoTo(NodeWorkspace).
		From(NodeResearchForReviewPlan).
		On(wruntime.OutcomeComplete).GoTo(NodeAddressReviewResearchPlan).
		From(NodeAddressReviewResearchPlan).
		On(wruntime.OutcomeComplete).GoTo(NodeReviewPlan).
		From(NodeWorkspace).On(wruntime.OutcomeComplete).GoTo(NodeImplement).
		From(NodeImplement).On(wruntime.OutcomeComplete).GoTo(NodeReviewImplementation).
		From(NodeReviewImplementation).
		On(wruntime.OutcomeNeedsFollowup).GoTo(NodeQuestion).
		From(NodeReviewImplementation).
		On(wruntime.OutcomeReadyForHumanReview).GoTo(NodeHumanReviewImplementation).
		From(NodeHumanReviewImplementation).On(wruntime.OutcomeComplete).GoTo(NodeDone).
		ResultParser(QRSPIXMLParser{}).
		ResultConverter(QRSPIResultConverter{}).
		Build()
}
