package hermescmd

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

func validHandoffFrame() HandoffFrame {
	return HandoffFrame{
		Version:     1,
		LaunchNonce: strings.Repeat("a", 32),
		PiSessionID: "pi-123",
		MessageID:   "pi-settlement-v1-test",
	}
}

func TestHandoffRoundTripAndBoundedSequence(t *testing.T) {
	t.Parallel()

	frame := validHandoffFrame()
	encoded, err := EncodeHandoffFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if got := int(binary.BigEndian.Uint32(encoded[:4])); got != len(encoded)-4 {
		t.Fatalf("prefix=%d payload=%d", got, len(encoded)-4)
	}
	decoded, err := ReadHandoffFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if decoded != frame {
		t.Fatalf("decoded=%+v want=%+v", decoded, frame)
	}
	frames, err := ReadHandoffFrames(bytes.NewReader(append(encoded, encoded...)), 2)
	if err != nil || len(frames) != 2 {
		t.Fatalf("frames=%v err=%v", frames, err)
	}
	if _, err := ReadHandoffFrames(
		bytes.NewReader(append(encoded, encoded...)),
		1,
	); err == nil {
		t.Fatal("excess frame was accepted")
	}
}

func TestHandoffRejectsMalformedPartialOversizedAndUnknownData(t *testing.T) {
	t.Parallel()

	valid, err := EncodeHandoffFrame(validHandoffFrame())
	if err != nil {
		t.Fatal(err)
	}
	oversized := make([]byte, 4)
	binary.BigEndian.PutUint32(oversized, MaxHandoffFrameBytes+1)
	duplicate := framedJSON(
		`{"version":1,"version":1,"launch_nonce":"` + strings.Repeat(
			"a",
			32,
		) + `","pi_session_id":"p","message_id":"m"}`,
	)
	unknown := framedJSON(
		`{"version":1,"launch_nonce":"` + strings.Repeat(
			"a",
			32,
		) + `","pi_session_id":"p","message_id":"m","endpoint":"hidden"}`,
	)
	for name, payload := range map[string][]byte{
		"empty":        {0, 0, 0, 0},
		"oversized":    oversized,
		"prefix":       valid[:3],
		"payload":      valid[:len(valid)-1],
		"invalid utf8": framedPayload([]byte{0xff}),
		"duplicate":    duplicate,
		"unknown":      unknown,
		"trailing":     framedPayload([]byte(`{} {}`)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := ReadHandoffFrame(bytes.NewReader(payload)); err == nil {
				t.Fatal("malformed handoff frame was accepted")
			}
		})
	}
}

func TestHandoffValidatesEveryIdentityAxis(t *testing.T) {
	t.Parallel()

	base := validHandoffFrame()
	cases := []HandoffFrame{
		{
			Version:     2,
			LaunchNonce: base.LaunchNonce,
			PiSessionID: base.PiSessionID,
			MessageID:   base.MessageID,
		},
		{
			Version:     1,
			LaunchNonce: "short",
			PiSessionID: base.PiSessionID,
			MessageID:   base.MessageID,
		},
		{
			Version:     1,
			LaunchNonce: base.LaunchNonce,
			PiSessionID: "bad/path",
			MessageID:   base.MessageID,
		},
		{
			Version:     1,
			LaunchNonce: base.LaunchNonce,
			PiSessionID: base.PiSessionID,
			MessageID:   "bad:message",
		},
	}
	for _, frame := range cases {
		if _, err := EncodeHandoffFrame(frame); err == nil {
			t.Fatalf("invalid frame accepted: %+v", frame)
		}
	}
}

func TestHandoffContainsNoTransportAuthorityOrResponseText(t *testing.T) {
	t.Parallel()

	encoded, err := EncodeHandoffFrame(validHandoffFrame())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"credential", "token", "http://", "https://", "socket", "origin",
		"manager_thread", "raw_response", "outcome", "next", "complete",
	} {
		if bytes.Contains(bytes.ToLower(encoded), []byte(forbidden)) {
			t.Errorf("handoff contains forbidden authority/content marker %q", forbidden)
		}
	}
	if _, err := io.Copy(io.Discard, bytes.NewReader(encoded)); err != nil {
		t.Fatal(err)
	}
}

func framedJSON(payload string) []byte { return framedPayload([]byte(payload)) }

func framedPayload(payload []byte) []byte {
	frame := make([]byte, 4+len(payload))
	//nolint:gosec // Test payloads are bounded far below uint32.
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	return frame
}
