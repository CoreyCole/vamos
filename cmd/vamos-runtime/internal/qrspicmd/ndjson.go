package qrspicmd

import (
	"encoding/json"
	"io"
)

type Event struct {
	Type     string          `json:"type"`
	Ref      map[string]any  `json:"ref,omitempty"`
	Decision *ParsedDecision `json:"decision,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func WriteNDJSON(out io.Writer, event Event) error {
	enc := json.NewEncoder(out)
	return enc.Encode(event)
}
