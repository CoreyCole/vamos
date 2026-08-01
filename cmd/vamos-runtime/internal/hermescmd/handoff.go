package hermescmd

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"unicode/utf8"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const (
	HandoffProtocolVersion = 1
	MaxHandoffFrameBytes   = 4_096
	MaxHandoffFrames       = 128
	handoffFramePrefixSize = 4
	handoffFieldCount      = 4
)

var launchNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

//nolint:tagliatelle // The child handoff wire schema uses protocol-defined snake_case fields.
type HandoffFrame struct {
	Version     int    `json:"version"`
	LaunchNonce string `json:"launch_nonce"`
	PiSessionID string `json:"pi_session_id"`
	MessageID   string `json:"message_id"`
}

func EncodeHandoffFrame(frame HandoffFrame) ([]byte, error) {
	if err := validateHandoffFrame(frame); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return nil, errors.New("encode handoff frame")
	}
	if len(payload) > MaxHandoffFrameBytes {
		return nil, errors.New("handoff frame exceeds size limit")
	}
	encoded := make([]byte, handoffFramePrefixSize+len(payload))
	//nolint:gosec // The payload bound above is smaller than uint32.
	binary.BigEndian.PutUint32(encoded[:handoffFramePrefixSize], uint32(len(payload)))
	copy(encoded[handoffFramePrefixSize:], payload)

	return encoded, nil
}

func WriteHandoffFrame(writer io.Writer, frame HandoffFrame) error {
	encoded, err := EncodeHandoffFrame(frame)
	if err != nil {
		return err
	}
	written, err := io.Copy(writer, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("write handoff frame: %w", err)
	}
	if written != int64(len(encoded)) {
		return errors.New("short handoff frame write")
	}

	return nil
}

func ReadHandoffFrame(reader io.Reader) (HandoffFrame, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return HandoffFrame{}, fmt.Errorf("read handoff frame prefix: %w", err)
	}
	size := binary.BigEndian.Uint32(prefix[:])
	if size == 0 || size > MaxHandoffFrameBytes {
		return HandoffFrame{}, errors.New("invalid handoff frame size")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return HandoffFrame{}, fmt.Errorf("read handoff frame payload: %w", err)
	}
	if !utf8.Valid(payload) {
		return HandoffFrame{}, errors.New("handoff frame is not valid UTF-8")
	}
	frame, err := decodeHandoffPayload(payload)
	if err != nil {
		return HandoffFrame{}, err
	}

	return frame, nil
}

func ReadHandoffFrames(reader io.Reader, maximum int) ([]HandoffFrame, error) {
	if maximum < 1 || maximum > MaxHandoffFrames {
		return nil, errors.New("invalid handoff frame count limit")
	}
	buffered := bufio.NewReader(reader)
	frames := make([]HandoffFrame, 0, maximum)
	for {
		if _, err := buffered.Peek(1); errors.Is(err, io.EOF) {
			return frames, nil
		} else if err != nil {
			return nil, errors.New("read handoff stream")
		}
		if len(frames) == maximum {
			return nil, errors.New("handoff frame count exceeds limit")
		}
		frame, err := ReadHandoffFrame(buffered)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
}

func decodeHandoffPayload(payload []byte) (HandoffFrame, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var frame HandoffFrame
	if err := decoder.Decode(&frame); err != nil {
		return HandoffFrame{}, errors.New("malformed handoff frame")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return HandoffFrame{}, errors.New("malformed handoff frame")
	}
	if err := rejectDuplicateHandoffFields(payload); err != nil {
		return HandoffFrame{}, err
	}
	if err := validateHandoffFrame(frame); err != nil {
		return HandoffFrame{}, err
	}

	return frame, nil
}

func rejectDuplicateHandoffFields(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("malformed handoff frame")
	}
	seen := make(map[string]struct{}, handoffFieldCount)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errors.New("malformed handoff frame")
		}
		name, ok := token.(string)
		if !ok {
			return errors.New("malformed handoff frame")
		}
		if _, duplicate := seen[name]; duplicate {
			return errors.New("duplicate handoff frame field")
		}
		seen[name] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return errors.New("malformed handoff frame")
		}
	}

	return nil
}

func validateHandoffFrame(frame HandoffFrame) error {
	if frame.Version != HandoffProtocolVersion {
		return errors.New("unsupported handoff protocol version")
	}
	if !launchNoncePattern.MatchString(frame.LaunchNonce) {
		return errors.New("invalid handoff launch nonce")
	}
	if err := sessioningress.ValidatePiSessionID(frame.PiSessionID); err != nil {
		return errors.New("invalid handoff Pi session ID")
	}
	if err := sessioningress.ValidateMessageID(frame.MessageID); err != nil {
		return errors.New("invalid handoff message ID")
	}

	return nil
}
