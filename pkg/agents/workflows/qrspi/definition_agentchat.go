package qrspi

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

const AgentChatWorkflowType wruntime.WorkflowID = "qrspi"

const (
	NodeQuestion wruntime.NodeID = "question"
	NodeResearch wruntime.NodeID = "research"
	NodeDesign   wruntime.NodeID = "design"

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
	NodeVerify                    wruntime.NodeID = "verify"
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
		Agent(NodeQuestion, Skill(".pi/skills/q-question/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeResearch, Skill(".pi/skills/q-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeDesign, Skill(".pi/skills/q-design/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeOutline, Skill(".pi/skills/q-outline/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewOutline, Skill(".pi/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForPlan, wruntime.OutcomeNeedsReviewResearch).
		RequiresPrimaryArtifact().
		HumanReview(NodeHumanReviewOutline, "outline approved by human").
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		AutoApprovable(true).
		Agent(NodeResearchForReviewOutline, Skill(".pi/skills/q-research-for-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeAddressReviewResearchOutline, Skill(".pi/skills/q-address-review-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodePlan, Skill(".pi/skills/q-plan/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewPlan, Skill(".pi/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForWorkspace, wruntime.OutcomeReadyForImplement, wruntime.OutcomeReadyForImplementation, wruntime.OutcomeNeedsReviewResearch).
		RequiresPrimaryArtifact().
		Agent(NodeResearchForReviewPlan, Skill(".pi/skills/q-research-for-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeAddressReviewResearchPlan, Skill(".pi/skills/q-address-review-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeWorkspace, Skill(".pi/skills/q-workspace/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete, wruntime.OutcomeReadyForImplement, wruntime.OutcomeReadyForImplementation).
		RequiresPrimaryArtifact().
		Agent(NodeImplement, Skill(".pi/skills/q-implement/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeReviewImplementation, Skill(".pi/skills/q-review/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeReadyForHumanReview, wruntime.OutcomeNeedsFollowup).
		RequiresPrimaryArtifact().
		Agent(NodeVerify, Skill(".pi/skills/q-verify/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusHandoff, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		HumanReview(NodeHumanReviewImplementation, "implementation approved by human").
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		AutoApprovable(false).
		Done(NodeDone).
		From(NodeQuestion).On(wruntime.OutcomeComplete).GoTo(NodeResearch).
		From(NodeResearch).On(wruntime.OutcomeComplete).GoTo(NodeDesign).
		From(NodeDesign).On(wruntime.OutcomeComplete).GoTo(NodeOutline).
		From(NodeOutline).When(ConfigPlanReviewsEnabled).GoTo(NodeReviewOutline).
		From(NodeOutline).When(ConfigPlanReviewsDisabled).GoTo(NodePlan).
		From(NodeReviewOutline).
		On(wruntime.OutcomeNeedsReviewResearch).GoTo(NodeResearchForReviewOutline).
		From(NodeReviewOutline).
		On(wruntime.OutcomeReadyForPlan).GoTo(NodePlan).
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
		From(NodeReviewPlan).On(wruntime.OutcomeReadyForImplement).GoTo(NodeImplement).
		From(NodeReviewPlan).
		On(wruntime.OutcomeReadyForImplementation).GoTo(NodeImplement).
		From(NodeResearchForReviewPlan).
		On(wruntime.OutcomeComplete).GoTo(NodeAddressReviewResearchPlan).
		From(NodeAddressReviewResearchPlan).
		On(wruntime.OutcomeComplete).GoTo(NodeReviewPlan).
		From(NodeWorkspace).On(wruntime.OutcomeComplete).GoTo(NodeImplement).
		From(NodeWorkspace).On(wruntime.OutcomeReadyForImplement).GoTo(NodeImplement).
		From(NodeWorkspace).
		On(wruntime.OutcomeReadyForImplementation).GoTo(NodeImplement).
		From(NodeImplement).On(wruntime.OutcomeComplete).GoTo(NodeReviewImplementation).
		From(NodeReviewImplementation).
		On(wruntime.OutcomeNeedsFollowup).GoTo(NodeQuestion).
		From(NodeReviewImplementation).
		On(wruntime.OutcomeReadyForHumanReview).GoTo(NodeVerify).
		From(NodeVerify).On(wruntime.OutcomeComplete).GoTo(NodeHumanReviewImplementation).
		From(NodeHumanReviewImplementation).On(wruntime.OutcomeComplete).GoTo(NodeDone).
		ResultParser(QRSPIResultParser{}).
		ResultConverter(QRSPIResultConverter{}).
		Build()
}
