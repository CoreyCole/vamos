package build

import (
	"fmt"
	"io"
	"strings"
)

type Reporter interface {
	LockAcquired()
	Step(result StepResult)
	Restart(plan RestartPlan, noRestart bool)
	Done(summary RunSummary)
}

type TextReporter struct{ w io.Writer }

func NewTextReporter(w io.Writer) *TextReporter {
	if w == nil {
		w = io.Discard
	}
	return &TextReporter{w: w}
}

func (r *TextReporter) LockAcquired() {
	fmt.Fprintln(r.w, "build-agents: lock acquired")
}

func (r *TextReporter) Step(result StepResult) {
	switch result.Decision {
	case DecisionSkipped:
		fmt.Fprintf(r.w, "%s: skipped (inputs unchanged)\n", result.Name)
	case DecisionSkippedUnavailable:
		fmt.Fprintf(
			r.w,
			"%s: skipped (optional prerequisites unavailable)\n",
			result.Name,
		)
	case DecisionRunForced,
		DecisionRunInputChanged,
		DecisionRunMissingOutput,
		DecisionRunDependencyChanged:
		r.runLine(result)
	default:
		r.runLine(result)
	}
}

func (r *TextReporter) runLine(result StepResult) {
	parts := []string{decisionReason(result.Decision)}
	if result.OutputChanged {
		parts = append(parts, "outputs changed")
	} else {
		parts = append(parts, "outputs unchanged")
	}
	fmt.Fprintf(r.w, "%s: ran (%s)\n", result.Name, strings.Join(parts, ", "))
}

func (r *TextReporter) Restart(plan RestartPlan, noRestart bool) {
	r.restartLine(ServiceWeb, plan.Web, noRestart)
	r.restartLine(ServiceTSWorker, plan.TSWorker, noRestart)
}

func (r *TextReporter) Done(RunSummary) {
	fmt.Fprintln(r.w, "build-agents: complete")
}

func (r *TextReporter) restartLine(
	service ServiceName,
	reason RestartReason,
	noRestart bool,
) {
	if !reason.Needed {
		fmt.Fprintf(r.w, "restart: %s skipped\n", service)
		return
	}
	label := restartReasonLabel(reason)
	if noRestart {
		fmt.Fprintf(r.w, "restart: %s pending (%s, --no-restart)\n", service, label)
		return
	}
	fmt.Fprintf(r.w, "restart: %s restarted (%s)\n", service, label)
}

func decisionReason(decision StepDecision) string {
	switch decision {
	case DecisionRunForced:
		return "forced"
	case DecisionRunInputChanged:
		return "inputs changed"
	case DecisionRunMissingOutput:
		return "missing outputs"
	case DecisionRunDependencyChanged:
		return "dependency changed"
	case DecisionSkippedUnavailable, DecisionSkipped:
		return string(decision)
	default:
		return string(decision)
	}
}

func restartReasonLabel(reason RestartReason) string {
	parts := []string{}
	if reason.OutputChanged {
		parts = append(parts, "outputs changed")
	}
	if reason.InputChanged {
		parts = append(parts, "runtime inputs changed")
	}
	if reason.Pending {
		parts = append(parts, "pending restart")
	}
	if len(parts) == 0 {
		return "needed"
	}
	return strings.Join(parts, ", ")
}
