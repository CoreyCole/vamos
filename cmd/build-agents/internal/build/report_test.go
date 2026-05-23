package build

import (
	"bytes"
	"testing"
	"time"
)

func TestTextReporter(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reporter := NewTextReporter(&out)
	reporter.LockAcquired()
	reporter.Step(StepResult{Name: StepSQLC, Decision: DecisionSkipped})
	reporter.Step(StepResult{
		Name:          StepTempl,
		Decision:      DecisionRunInputChanged,
		OutputChanged: true,
	})
	reporter.Step(StepResult{
		Name:          StepGo,
		Decision:      DecisionRunDependencyChanged,
		OutputChanged: true,
	})
	reporter.Step(StepResult{
		Name:          StepTailwind,
		Decision:      DecisionRunDependencyChanged,
		OutputChanged: false,
	})
	reporter.Step(StepResult{Name: StepTSWorker, Decision: DecisionSkipped})
	reporter.Restart(RestartPlan{
		Web:      RestartReason{Needed: true, OutputChanged: true},
		TSWorker: RestartReason{},
	}, false)
	reporter.Done(RunSummary{Success: true, FinishedAt: time.Now()})

	want := "" +
		"build-agents: lock acquired\n" +
		"sqlc: skipped (inputs unchanged)\n" +
		"templ: ran (inputs changed, outputs changed)\n" +
		"go: ran (dependency changed, outputs changed)\n" +
		"tailwind: ran (dependency changed, outputs unchanged)\n" +
		"ts-worker: skipped (inputs unchanged)\n" +
		"restart: vamos restarted (outputs changed)\n" +
		"restart: vamos-ts-worker skipped\n" +
		"build-agents: complete\n"
	if got := out.String(); got != want {
		t.Fatalf("report output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextReporterNoRestartPending(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reporter := NewTextReporter(&out)
	reporter.Restart(RestartPlan{
		Web: RestartReason{Needed: true, OutputChanged: true, Pending: true},
		TSWorker: RestartReason{
			Needed:       true,
			InputChanged: true,
		},
	}, true)

	want := "" +
		"restart: vamos pending (outputs changed, pending restart, --no-restart)\n" +
		"restart: vamos-ts-worker pending (runtime inputs changed, --no-restart)\n"
	if got := out.String(); got != want {
		t.Fatalf("report output mismatch:\n got: %q\nwant: %q", got, want)
	}
}
