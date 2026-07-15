package qrspi

import wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"

const ProjectPlanningWorkflowType wruntime.WorkflowID = "qrspi-project-planning"

const (
	NodeMilestoneQuestion      wruntime.NodeID = "milestone-question"
	NodeMilestoneResearch      wruntime.NodeID = "milestone-research"
	NodeMilestoneDesign        wruntime.NodeID = "milestone-design"
	NodeMilestoneCreateTickets wruntime.NodeID = "milestone-create-tickets"
)

func ProjectPlanningDefinition() (wruntime.Definition, error) {
	return wruntime.New[Config](ProjectPlanningWorkflowType).
		Config(DefaultConfig(), ValidateConfig).
		Version("v1").Name("QRSPI Project Planning").Start(NodeMilestoneQuestion).
		Agent(NodeMilestoneQuestion, Skill(".pi/skills/q-milestone-question/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeMilestoneResearch, Skill(".pi/skills/q-milestone-research/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeMilestoneDesign, Skill(".pi/skills/q-milestone-design/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Agent(NodeMilestoneCreateTickets, Skill(".pi/skills/q-milestone-create-tickets/SKILL.md")).
		Statuses(wruntime.StatusComplete, wruntime.StatusNeedsHuman, wruntime.StatusBlocked, wruntime.StatusError).
		Outcomes(wruntime.OutcomeComplete).
		RequiresPrimaryArtifact().
		Done(NodeDone).
		From(NodeMilestoneQuestion).
		On(wruntime.OutcomeComplete).GoTo(NodeMilestoneResearch).
		From(NodeMilestoneResearch).
		On(wruntime.OutcomeComplete).GoTo(NodeMilestoneDesign).
		From(NodeMilestoneDesign).
		On(wruntime.OutcomeComplete).GoTo(NodeMilestoneCreateTickets).
		From(NodeMilestoneCreateTickets).On(wruntime.OutcomeComplete).GoTo(NodeDone).
		ResultParser(QRSPIResultParser{}).
		ResultConverter(QRSPIResultConverter{}).
		Build()
}
