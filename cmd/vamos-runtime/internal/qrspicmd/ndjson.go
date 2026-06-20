package qrspicmd

import (
	"encoding/json"
	"io"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type Event struct {
	Type     string          `json:"type"`
	Ref      map[string]any  `json:"ref,omitempty"`
	Decision *ParsedDecision `json:"decision,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// ParsedDecision is populated by graph/result helpers in a later slice.
type ParsedDecision struct {
	Result   wruntime.WorkflowResult     `json:"result"`
	Decision wruntime.TransitionDecision `json:"decision"`
	RawYAML  string                      `json:"rawYaml,omitempty"`
}

func WriteNDJSON(out io.Writer, event Event) error {
	enc := json.NewEncoder(out)
	return enc.Encode(event)
}
