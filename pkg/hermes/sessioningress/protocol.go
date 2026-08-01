package sessioningress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const (
	ProtocolVersion        = 1
	MaxFrameBytes          = 262_144
	ExactSessionCapability = "exact-session-next-turn-v1"
)

var (
	piSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	messageIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	capabilityPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

type ProtocolError struct {
	Code string
	Err  error
}

func (e *ProtocolError) Error() string { return e.Err.Error() }
func (e *ProtocolError) Unwrap() error { return e.Err }

func protocolError(message string) error {
	return &ProtocolError{Code: "malformed", Err: errors.New(message)}
}

func versionError(message string, request bool) error {
	code := "malformed"
	if request {
		code = "unsupported_version"
	}
	return &ProtocolError{Code: code, Err: errors.New(message)}
}

type CapabilityRequest struct {
	Op      string `json:"op"`
	Version int    `json:"version"`
}

type EnqueueRequest struct {
	HermesSessionID string `json:"hermes_session_id"`
	Message         string `json:"message"`
	MessageID       string `json:"message_id"`
	Op              string `json:"op"`
	PiSessionID     string `json:"pi_session_id"`
	Version         int    `json:"version"`
}

type CapabilityResponse struct {
	Capabilities     []string `json:"capabilities"`
	Code             string   `json:"code"`
	MaxFrameBytes    int      `json:"max_frame_bytes"`
	ProtocolVersions []int    `json:"protocol_versions"`
	Version          int      `json:"version"`
}

type AcceptedResponse struct {
	Code    string `json:"code"`
	Version int    `json:"version"`
}

type RejectionResponse struct {
	Code         string `json:"code"`
	Detail       string `json:"detail,omitempty"`
	RetryAfterMS *int   `json:"retry_after_ms,omitempty"`
	Version      int    `json:"version"`
}

type (
	Request  interface{ isRequest() }
	Response interface{ isResponse() }
)

func (CapabilityRequest) isRequest()   {}
func (EnqueueRequest) isRequest()      {}
func (CapabilityResponse) isResponse() {}
func (AcceptedResponse) isResponse()   {}
func (RejectionResponse) isResponse()  {}

var acceptedCodes = map[string]struct{}{
	"accepted_idle": {}, "accepted_queued": {},
}

var retryableCodes = map[string]struct{}{
	"queue_full": {}, "temporarily_unavailable": {},
}

var terminalCodes = map[string]struct{}{
	"ambiguous_session": {}, "malformed": {}, "origin_unavailable": {},
	"session_expired": {}, "session_suspended": {}, "stale_session": {},
	"surface_unsupported": {}, "target_closing": {}, "unauthorized": {},
	"unknown_session": {}, "unsupported_version": {},
}

var httpStatusByCode = map[string]int{
	"capabilities": 200, "accepted_idle": 202, "accepted_queued": 202,
	"ambiguous_session": 409, "malformed": 400, "origin_unavailable": 422,
	"queue_full": 429, "session_expired": 410, "session_suspended": 409,
	"stale_session": 410, "surface_unsupported": 422, "target_closing": 409,
	"temporarily_unavailable": 503, "unauthorized": 401, "unknown_session": 404,
	"unsupported_version": 426,
}

func ParseRequest(payload []byte) (Request, error) {
	fields, err := decodeObject(payload)
	if err != nil {
		return nil, err
	}
	op, err := requiredString(fields, "op")
	if err != nil {
		return nil, err
	}
	switch op {
	case "capabilities":
		if err := requireFields(fields, []string{"op", "version"}, nil); err != nil {
			return nil, err
		}
		if err := requireVersion(fields["version"], true); err != nil {
			return nil, err
		}
		return CapabilityRequest{Op: op, Version: ProtocolVersion}, nil
	case "enqueue":
		required := []string{
			"hermes_session_id",
			"message",
			"message_id",
			"op",
			"pi_session_id",
			"version",
		}
		if err := requireFields(fields, required, nil); err != nil {
			return nil, err
		}
		if err := requireVersion(fields["version"], true); err != nil {
			return nil, err
		}
		hermesID, err := requiredString(fields, "hermes_session_id")
		if err != nil {
			return nil, err
		}
		if _, err := ValidateSessionID(hermesID); err != nil {
			return nil, protocolError("hermes_session_id violates its grammar")
		}
		message, err := requiredString(fields, "message")
		if err != nil || !validUTF8Size(message, 1, 131_072) ||
			bytes.IndexByte([]byte(message), 0) >= 0 {
			return nil, protocolError("message violates its grammar")
		}
		messageID, err := requiredString(fields, "message_id")
		if err != nil || !messageIDPattern.MatchString(messageID) {
			return nil, protocolError("message_id violates its grammar")
		}
		piID, err := requiredString(fields, "pi_session_id")
		if err != nil || !piSessionIDPattern.MatchString(piID) {
			return nil, protocolError("pi_session_id violates its grammar")
		}
		return EnqueueRequest{
			HermesSessionID: hermesID,
			Message:         message,
			MessageID:       messageID,
			Op:              op,
			PiSessionID:     piID,
			Version:         ProtocolVersion,
		}, nil
	default:
		return nil, protocolError("unknown request operation")
	}
}

func ParseResponse(payload []byte) (Response, error) {
	fields, err := decodeObject(payload)
	if err != nil {
		return nil, err
	}
	code, err := requiredString(fields, "code")
	if err != nil {
		return nil, err
	}
	if code == "capabilities" {
		required := []string{
			"capabilities",
			"code",
			"max_frame_bytes",
			"protocol_versions",
			"version",
		}
		if err := requireFields(fields, required, nil); err != nil {
			return nil, err
		}
		if err := requireVersion(fields["version"], false); err != nil {
			return nil, err
		}
		max, err := strictInt(fields["max_frame_bytes"])
		if err != nil || max != MaxFrameBytes {
			return nil, protocolError("max_frame_bytes does not match protocol v1")
		}
		versions, err := strictIntSlice(fields["protocol_versions"])
		if err != nil || len(versions) != 1 || versions[0] != ProtocolVersion {
			return nil, protocolError("protocol_versions must be [1]")
		}
		capabilities, err := strictStringSlice(fields["capabilities"])
		if err != nil || len(capabilities) == 0 {
			return nil, protocolError("capabilities must be a nonempty array")
		}
		for i, capability := range capabilities {
			if !capabilityPattern.MatchString(capability) {
				return nil, protocolError("invalid capability name")
			}
			if i > 0 && capabilities[i-1] >= capability {
				return nil, protocolError(
					"capabilities must be sorted and duplicate-free",
				)
			}
		}
		return CapabilityResponse{
			Capabilities:     capabilities,
			Code:             code,
			MaxFrameBytes:    max,
			ProtocolVersions: versions,
			Version:          ProtocolVersion,
		}, nil
	}
	if _, ok := acceptedCodes[code]; ok {
		if err := requireFields(fields, []string{"code", "version"}, nil); err != nil {
			return nil, err
		}
		if err := requireVersion(fields["version"], false); err != nil {
			return nil, err
		}
		return AcceptedResponse{Code: code, Version: ProtocolVersion}, nil
	}
	_, terminal := terminalCodes[code]
	_, retryable := retryableCodes[code]
	if !terminal && !retryable {
		return nil, protocolError("unknown response code")
	}
	optional := []string{"detail"}
	if retryable {
		optional = append(optional, "retry_after_ms")
	}
	if err := requireFields(fields, []string{"code", "version"}, optional); err != nil {
		return nil, err
	}
	if err := requireVersion(fields["version"], false); err != nil {
		return nil, err
	}
	response := RejectionResponse{Code: code, Version: ProtocolVersion}
	if raw, ok := fields["detail"]; ok {
		detail, err := decodeString(raw)
		if err != nil || !validUTF8Size(detail, 1, 256) || hasControl(detail) {
			return nil, protocolError("detail violates its grammar")
		}
		response.Detail = detail
	}
	if raw, ok := fields["retry_after_ms"]; ok {
		retryAfter, err := strictInt(raw)
		if err != nil || retryAfter < 1 || retryAfter > 60_000 {
			return nil, protocolError("retry_after_ms is outside its allowed range")
		}
		response.RetryAfterMS = &retryAfter
	}
	return response, nil
}

func EncodeCanonical(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, &ProtocolError{
			Code: "malformed",
			Err:  fmt.Errorf("encode canonical JSON: %w", err),
		}
	}
	return payload, nil
}

func ClassifyResult(code string) (string, error) {
	if code == "capabilities" {
		return "capability", nil
	}
	if _, ok := acceptedCodes[code]; ok {
		return "accepted", nil
	}
	if _, ok := retryableCodes[code]; ok {
		return "retryable", nil
	}
	if _, ok := terminalCodes[code]; ok {
		return "terminal", nil
	}
	return "", protocolError("unknown response code")
}

func HTTPStatusForCode(code string) (int, error) {
	status, ok := httpStatusByCode[code]
	if !ok {
		return 0, protocolError("unknown response code")
	}
	return status, nil
}

func ValidateHTTPStatus(code string, status int) error {
	expected, err := HTTPStatusForCode(code)
	if err != nil {
		return err
	}
	if status != expected {
		return protocolError("HTTP status does not match response code")
	}
	return nil
}

func decodeObject(payload []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(payload) {
		return nil, protocolError("payload is not valid UTF-8")
	}
	if err := validateJSONUnicodeEscapes(payload); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil {
		return nil, protocolError("payload is not valid JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, protocolError("top-level JSON value must be an object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, protocolError("payload is not valid JSON")
		}
		key, ok := token.(string)
		if !ok {
			return nil, protocolError("JSON object key must be a string")
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, protocolError("duplicate JSON field: " + key)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, protocolError("payload is not valid JSON")
		}
		fields[key] = raw
	}
	if _, err := decoder.Token(); err != nil {
		return nil, protocolError("payload is not valid JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, protocolError("trailing JSON data")
	}
	return fields, nil
}

func requireFields(fields map[string]json.RawMessage, required, optional []string) error {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
		if _, ok := fields[field]; !ok {
			return protocolError("missing field: " + field)
		}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return protocolError("unknown field: " + field)
		}
	}
	return nil
}

func requiredString(fields map[string]json.RawMessage, field string) (string, error) {
	raw, ok := fields[field]
	if !ok {
		return "", protocolError("missing field: " + field)
	}
	value, err := decodeString(raw)
	if err != nil {
		return "", protocolError(field + " must be a string")
	}
	return value, nil
}

func decodeString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	if !utf8.ValidString(value) {
		return "", errors.New("invalid Unicode string")
	}
	return value, nil
}

func requireVersion(raw json.RawMessage, request bool) error {
	version, err := strictInt(raw)
	if err != nil {
		return versionError("version must be the integer 1", request)
	}
	if version != ProtocolVersion {
		return versionError("unsupported protocol version", request)
	}
	return nil
}

func strictInt(raw json.RawMessage) (int, error) {
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func strictIntSlice(raw json.RawMessage) ([]int, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]int, len(values))
	for i, raw := range values {
		value, err := strictInt(raw)
		if err != nil {
			return nil, err
		}
		result[i] = value
	}
	return result, nil
}

func strictStringSlice(raw json.RawMessage) ([]string, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	result := make([]string, len(values))
	for i, raw := range values {
		value, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		result[i] = value
	}
	return result, nil
}

func validUTF8Size(value string, min, max int) bool {
	return utf8.ValidString(value) && len([]byte(value)) >= min &&
		len([]byte(value)) <= max
}

func hasControl(value string) bool {
	for _, r := range value {
		if r <= 0x1f || r >= 0x7f && r <= 0x9f {
			return true
		}
	}
	return false
}

func validateJSONUnicodeEscapes(payload []byte) error {
	inString := false
	for i := 0; i < len(payload); i++ {
		switch payload[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || i+1 >= len(payload) {
				continue
			}
			if payload[i+1] != 'u' {
				i++
				continue
			}
			value, ok := parseHexQuad(payload, i+2)
			if !ok {
				continue
			}
			if value >= 0xdc00 && value <= 0xdfff {
				return protocolError("payload contains an unpaired Unicode surrogate")
			}
			if value >= 0xd800 && value <= 0xdbff {
				if i+12 > len(payload) || payload[i+6] != '\\' || payload[i+7] != 'u' {
					return protocolError("payload contains an unpaired Unicode surrogate")
				}
				low, valid := parseHexQuad(payload, i+8)
				if !valid || low < 0xdc00 || low > 0xdfff {
					return protocolError("payload contains an unpaired Unicode surrogate")
				}
				i += 11
				continue
			}
			i += 5
		}
	}
	return nil
}

func parseHexQuad(payload []byte, start int) (uint64, bool) {
	if start+4 > len(payload) {
		return 0, false
	}
	value, err := strconv.ParseUint(string(payload[start:start+4]), 16, 16)
	return value, err == nil
}
