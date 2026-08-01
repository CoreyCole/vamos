package sessioningress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

const (
	fixtureRevision = 1
	fixtureSHA256   = "d88f77ea8226cc6d55be144569a75d10baa54e47ab69c6131ed3dbac19a0256b"
	hermesH0Commit  = "e77efc4b6d543f8948026405ee9f7863e7a11900"
)

type protocolFixture struct {
	FixtureRevision int `json:"fixture_revision"`
	Protocol        struct {
		Capabilities  []string `json:"capabilities"`
		Version       int      `json:"version"`
		MaxFrameBytes int      `json:"max_frame_bytes"`
	} `json:"protocol"`
	Canonical map[string]struct {
		FrameHex     string         `json:"frame_hex"`
		Object       map[string]any `json:"object"`
		Payload      string         `json:"payload"`
		PayloadBytes int            `json:"payload_bytes"`
	} `json:"canonical"`
	CodeClasses      map[string][]string `json:"code_classes"`
	HTTPStatusByCode map[string]int      `json:"http_status_by_code"`
}

func loadFixture(t *testing.T) protocolFixture {
	t.Helper()
	raw, err := os.ReadFile("../testdata/session_ingress_protocol_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if digest != fixtureSHA256 {
		t.Fatalf("fixture SHA-256 = %s, want %s", digest, fixtureSHA256)
	}
	var fixture protocolFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.FixtureRevision != fixtureRevision {
		t.Fatalf(
			"fixture revision = %d, want %d",
			fixture.FixtureRevision,
			fixtureRevision,
		)
	}
	return fixture
}

func TestFixturePairMetadata(t *testing.T) {
	if len(hermesH0Commit) != 40 {
		t.Fatalf("invalid H0 commit pin %q", hermesH0Commit)
	}
	loadFixture(t)
}

func TestFrozenConstantsStatusesAndCanonicalVectors(t *testing.T) {
	fixture := loadFixture(t)
	if ProtocolVersion != 1 || MaxFrameBytes != 262_144 ||
		ExactSessionCapability != "exact-session-next-turn-v1" {
		t.Fatal("protocol constants differ from frozen protocol v1")
	}
	if !reflect.DeepEqual(
		fixture.Protocol.Capabilities,
		[]string{ExactSessionCapability},
	) {
		t.Fatalf("capabilities = %#v", fixture.Protocol.Capabilities)
	}
	unauthorized := make([]string, 0, 1)
	for code, want := range fixture.HTTPStatusByCode {
		got, err := HTTPStatusForCode(code)
		if err != nil || got != want {
			t.Fatalf("HTTP status for %s = %d, %v; want %d", code, got, err, want)
		}
		if got == 401 {
			unauthorized = append(unauthorized, code)
		}
		if err := ValidateHTTPStatus(code, want); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(unauthorized, []string{"unauthorized"}) {
		t.Fatalf("401 codes = %#v", unauthorized)
	}

	for name, vector := range fixture.Canonical {
		payload := []byte(vector.Payload)
		if len(payload) != vector.PayloadBytes {
			t.Fatalf(
				"%s payload bytes = %d, want %d",
				name,
				len(payload),
				vector.PayloadBytes,
			)
		}
		frame, err := EncodeFrame(payload)
		if err != nil {
			t.Fatalf("encode %s: %v", name, err)
		}
		if hex.EncodeToString(frame) != vector.FrameHex {
			t.Fatalf("%s frame mismatch", name)
		}
		decoded, err := DecodeFrame(frame)
		if err != nil || string(decoded) != vector.Payload {
			t.Fatalf("decode %s = %q, %v", name, decoded, err)
		}
		var parsed any
		if strings.HasSuffix(name, "request") {
			parsed, err = ParseRequest(payload)
		} else {
			parsed, err = ParseResponse(payload)
		}
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		emitted, err := EncodeCanonical(parsed)
		if err != nil || string(emitted) != vector.Payload {
			t.Fatalf("emit %s = %q, %v", name, emitted, err)
		}
	}

	capability := fixture.Canonical["capability_success"]
	if capability.PayloadBytes != 130 ||
		!strings.HasPrefix(capability.FrameHex, "00000082") {
		t.Fatalf(
			"capability golden length/prefix = %d/%s",
			capability.PayloadBytes,
			capability.FrameHex[:8],
		)
	}
}

func TestRequestSchemasAreClosedAndStrict(t *testing.T) {
	for _, payload := range []string{
		`{"op":"capabilities"}`,
		`{"version":1}`,
		`{"op":"capabilities","unknown":1,"version":1}`,
	} {
		if _, err := ParseRequest([]byte(payload)); err == nil {
			t.Fatalf("accepted invalid capability request %s", payload)
		}
	}
	valid := map[string]any{
		"hermes_session_id": "20260731_153837_5d85bf",
		"message":           "done",
		"message_id":        "pi-settlement-v1-test",
		"op":                "enqueue",
		"pi_session_id":     "pi-123",
		"version":           1,
	}
	for field := range valid {
		candidate := cloneMap(valid)
		delete(candidate, field)
		assertRequestError(t, candidate)
	}
	assertRequestError(t, mergeMap(valid, "unknown", 1))
	for _, payload := range []string{
		`{"op":"capabilities","op":"capabilities","version":1}`,
		`{"op":"capabilities","version":1,"version":1}`,
		`null`, `[]`, `"object"`, `1`, `true`,
	} {
		if _, err := ParseRequest([]byte(payload)); err == nil {
			t.Fatalf("accepted invalid request %s", payload)
		}
	}
	for _, version := range []any{true, "1", nil} {
		assertRequestError(t, map[string]any{"op": "capabilities", "version": version})
	}
	if _, err := ParseRequest([]byte(`{"op":"capabilities","version":1.0}`)); err == nil {
		t.Fatal("accepted floating-point version")
	}
	_, err := ParseRequest([]byte(`{"op":"capabilities","version":2}`))
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Code != "unsupported_version" {
		t.Fatalf("unsupported version error = %#v", err)
	}
}

func TestRequestGrammarBoundsAndForbiddenValues(t *testing.T) {
	base := map[string]any{
		"hermes_session_id": "h",
		"message":           "done",
		"message_id":        "m",
		"op":                "enqueue",
		"pi_session_id":     "p",
		"version":           1,
	}
	cases := []struct {
		field string
		value any
		valid bool
	}{
		{"hermes_session_id", "", false},
		{"hermes_session_id", "a", true},
		{"hermes_session_id", strings.Repeat("é", 512), true},
		{"hermes_session_id", strings.Repeat("é", 513), false},
		{"pi_session_id", "", false},
		{"pi_session_id", strings.Repeat("a", 128), true},
		{"pi_session_id", strings.Repeat("a", 129), false},
		{"message_id", "", false},
		{"message_id", strings.Repeat("a", 128), true},
		{"message_id", strings.Repeat("a", 129), false},
		{"message", "", false},
		{"message", strings.Repeat("é", 65_536), true},
		{"message", strings.Repeat("é", 65_537), false},
		{"hermes_session_id", "a\u0085b", false},
		{"message", "a\x00b", false},
		{"pi_session_id", "-bad", false},
		{"pi_session_id", "bad space", false},
		{"message_id", "bad:colon", false},
		{"message", false, false},
		{"message_id", nil, false},
		{"pi_session_id", map[string]any{}, false},
	}
	for _, test := range cases {
		candidate := cloneMap(base)
		candidate[test.field] = test.value
		payload, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseRequest(payload)
		if (err == nil) != test.valid {
			t.Errorf("%s=%#v valid=%v, error=%v", test.field, test.value, test.valid, err)
		}
	}
	invalidUTF8 := append([]byte(`{"op":"capabilities","version":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	for _, payload := range [][]byte{
		invalidUTF8,
		[]byte(`{"op":"capabilities","version":NaN}`),
		[]byte(`{"hermes_session_id":"\ud800","message":"done","message_id":"m","op":"enqueue","pi_session_id":"p","version":1}`),
	} {
		if _, err := ParseRequest(payload); err == nil {
			t.Fatalf("accepted malformed payload %q", payload)
		}
	}
}

func TestResponseSchemasGrammarsAndClassification(t *testing.T) {
	fixture := loadFixture(t)
	optionalFields := map[string]map[string]bool{
		"terminal_rejection":  {"detail": true},
		"retryable_rejection": {"detail": true, "retry_after_ms": true},
	}
	for name, vector := range fixture.Canonical {
		if strings.HasSuffix(name, "request") {
			continue
		}
		for field := range vector.Object {
			candidate := cloneMap(vector.Object)
			delete(candidate, field)
			payload, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseResponse(payload)
			if optionalFields[name][field] != (err == nil) {
				t.Errorf("%s without %s: %v", name, field, err)
			}
		}
		assertResponseError(t, mergeMap(vector.Object, "unknown", 1))
	}
	for _, payload := range []string{
		`{"code":"accepted_idle","detail":"no","version":1}`,
		`{"code":"stale_session","retry_after_ms":1,"version":1}`,
		`{"code":"future_code","version":1}`,
		`{"capabilities":["a","a"],"code":"capabilities","max_frame_bytes":262144,"protocol_versions":[1],"version":1}`,
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":262144,"protocol_versions":[1,1],"version":1}`,
	} {
		if _, err := ParseResponse([]byte(payload)); err == nil {
			t.Fatalf("accepted invalid response %s", payload)
		}
	}
	for _, capabilities := range []any{[]any{}, []any{""}, []any{strings.Repeat("a", 65)}, []any{"A"}, []any{"é"}, []any{"b", "a"}, []any{"a", "a"}, "a"} {
		assertResponseError(
			t,
			map[string]any{
				"capabilities":      capabilities,
				"code":              "capabilities",
				"max_frame_bytes":   MaxFrameBytes,
				"protocol_versions": []int{1},
				"version":           1,
			},
		)
	}
	for _, detail := range []string{"", strings.Repeat("é", 129), "a\x00b", "a\u0085b"} {
		assertResponseError(
			t,
			map[string]any{"code": "stale_session", "detail": detail, "version": 1},
		)
	}
	for _, retry := range []any{0, 60_001, true} {
		assertResponseError(
			t,
			map[string]any{"code": "queue_full", "retry_after_ms": retry, "version": 1},
		)
	}
	if _, err := ParseResponse(
		[]byte(`{"code":"queue_full","retry_after_ms":1.0,"version":1}`),
	); err == nil {
		t.Fatal("accepted floating-point retry hint")
	}
	for _, mutation := range []string{
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":true,"protocol_versions":[1],"version":1}`,
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":262144.0,"protocol_versions":[1],"version":1}`,
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":262143,"protocol_versions":[1],"version":1}`,
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":262144,"protocol_versions":[true],"version":1}`,
		`{"capabilities":["a"],"code":"capabilities","max_frame_bytes":262144,"protocol_versions":[1.0],"version":1}`,
	} {
		if _, err := ParseResponse([]byte(mutation)); err == nil {
			t.Fatalf("accepted invalid capability response %s", mutation)
		}
	}
	for class, codes := range fixture.CodeClasses {
		for _, code := range codes {
			got, err := ClassifyResult(code)
			if err != nil || got != class {
				t.Fatalf("classify %s = %q, %v; want %q", code, got, err, class)
			}
		}
	}
	if _, err := ClassifyResult("future_code"); err == nil {
		t.Fatal("accepted unknown result code")
	}
	if err := ValidateHTTPStatus("unauthorized", 403); err == nil {
		t.Fatal("accepted mismatched HTTP status")
	}
}

func assertRequestError(t *testing.T, value map[string]any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRequest(payload); err == nil {
		t.Fatalf("accepted invalid request %s", payload)
	}
}

func assertResponseError(t *testing.T, value map[string]any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseResponse(payload); err == nil {
		t.Fatalf("accepted invalid response %s", payload)
	}
}

func cloneMap(value map[string]any) map[string]any {
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func mergeMap(value map[string]any, key string, item any) map[string]any {
	clone := cloneMap(value)
	clone[key] = item
	return clone
}
