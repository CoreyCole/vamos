package hermescmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestCaptureOpaqueSettlementFences(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []OpaqueSettlementFence
	}{
		{name: "prose and unfenced YAML", raw: "outcome: handoff\nnext: implement\n"},
		{
			name: "empty",
			raw:  "```yaml\n```\n",
			want: []OpaqueSettlementFence{{Language: "yaml", Raw: "```yaml\n```\n"}},
		},
		{
			name: "multiple preserve order",
			raw:  "```YAML\na: 1\n```\ntext\n```yMl\nb: 2\n```",
			want: []OpaqueSettlementFence{
				{Language: "YAML", Raw: "```YAML\na: 1\n```\n"},
				{Language: "yMl", Raw: "```yMl\nb: 2\n```"},
			},
		},
		{
			name: "crlf copied",
			raw:  "```yml \t\r\na: café 🌰\r\n``` \t\r\n",
			want: []OpaqueSettlementFence{
				{Language: "yml", Raw: "```yml \t\r\na: café 🌰\r\n``` \t\r\n"},
			},
		},
		{
			name: "exact delimiter run",
			raw:  "````yaml\na\n```\n````\n",
			want: []OpaqueSettlementFence{
				{Language: "yaml", Raw: "````yaml\na\n```\n````\n"},
			},
		},
		{
			name: "longer and shorter are nonclosers",
			raw:  "```yaml\na\n````\nb\n``\nc\n```\n",
			want: []OpaqueSettlementFence{
				{Language: "yaml", Raw: "```yaml\na\n````\nb\n``\nc\n```\n"},
			},
		},
		{
			name: "opener attributes rejected",
			raw:  "```yaml title=x\na\n```\n```yml {x}\nb\n```\n",
		},
		{
			name: "non yaml and inline excluded",
			raw:  "```json\na\n```\ninline ```yaml nope\n",
		},
		{name: "unclosed", raw: "```yaml\na: 1\n"},
		{
			name: "malformed contradictory unknown opaque",
			raw:  "```yaml\na: [\noutcome: complete\noutcome: handoff\nunknown: ☃\n```\n",
			want: []OpaqueSettlementFence{
				{
					Language: "yaml",
					Raw:      "```yaml\na: [\noutcome: complete\noutcome: handoff\nunknown: ☃\n```\n",
				},
			},
		},
		{
			name: "trailing no newline",
			raw:  "```yaml\t\na\n```\t ",
			want: []OpaqueSettlementFence{
				{Language: "yaml", Raw: "```yaml\t\na\n```\t "},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CaptureOpaqueSettlementFences(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CaptureOpaqueSettlementFences() = %#v, want %#v", got, tt.want)
			}
			for _, fence := range got {
				if !strings.Contains(tt.raw, fence.Raw) {
					t.Fatalf("captured bytes not copied from raw response: %q", fence.Raw)
				}
			}
		})
	}
}

func TestCaptureOpaqueSettlementEvidenceCopiesRawResponse(t *testing.T) {
	raw := "prose\r\n```yaml\nvalue: café 🌰\n```"
	got := CaptureOpaqueSettlementEvidence(raw)
	if got.RawResponse != raw {
		t.Fatalf("RawResponse = %q, want exact %q", got.RawResponse, raw)
	}
	if len(got.FencedYAMLBlocks) != 1 ||
		got.FencedYAMLBlocks[0].Raw != "```yaml\nvalue: café 🌰\n```" {
		t.Fatalf("FencedYAMLBlocks = %#v", got.FencedYAMLBlocks)
	}
}
