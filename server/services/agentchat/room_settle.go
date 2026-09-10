package agentchat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const roomSettleTimestampLayout = "2006-01-02_15-04-05"

type RoomHandoffSpec struct {
	Timestamp string
	Body      string
	Files     []string
}

type RoomRotateResult struct {
	Timestamp  string
	HandoffRel string
	HistoryRel string
	CurrentRel string
	HandoffAbs string
	HistoryAbs string
	CurrentAbs string
}

func FormatRoomSettleTimestamp(at time.Time) string {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return at.UTC().Format(roomSettleTimestampLayout)
}

func ValidateHandoffFiles(thoughtsRoot string, files []string) error {
	for _, raw := range files {
		rel, err := CanonicalThoughtsPath(raw)
		if err != nil {
			return fmt.Errorf("files: %w", err)
		}
		abs, err := AbsFromThoughtsRel(thoughtsRoot, rel)
		if err != nil {
			return fmt.Errorf("files: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("files: missing %s", rel)
			}
			return fmt.Errorf("files: %s: %w", rel, err)
		}
	}
	return nil
}

func WriteRoomHandoffMarkdown(
	thoughtsRoot string,
	id RoomIdentity,
	spec RoomHandoffSpec,
) (rel, abs string, err error) {
	ts := strings.TrimSpace(spec.Timestamp)
	if ts == "" {
		ts = FormatRoomSettleTimestamp(time.Time{})
	}
	handoffsRel, err := id.HandoffsRel()
	if err != nil {
		return "", "", err
	}
	rel = handoffsRel + "/" + ts + ".md"
	abs, err = AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", "", err
	}
	raw, err := encodeHandoffMarkdown(spec.Files, spec.Body)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(abs, []byte(raw), 0o644); err != nil {
		return "", "", err
	}
	return rel, abs, nil
}

func RotateRoomCurrentJSONL(
	thoughtsRoot string,
	id RoomIdentity,
	spec RoomHandoffSpec,
) (RoomRotateResult, error) {
	var out RoomRotateResult
	ts := strings.TrimSpace(spec.Timestamp)
	if ts == "" {
		ts = FormatRoomSettleTimestamp(time.Time{})
	}
	spec.Timestamp = ts

	if err := ValidateHandoffFiles(thoughtsRoot, spec.Files); err != nil {
		return out, err
	}

	currentRel, err := id.CurrentJSONLRel()
	if err != nil {
		return out, err
	}
	historyRel, err := id.HistoryRel()
	if err != nil {
		return out, err
	}
	historyRel = historyRel + "/" + ts + ".jsonl"

	currentAbs, err := EnsureRoomCurrentJSONL(thoughtsRoot, id)
	if err != nil {
		return out, err
	}
	historyAbs, err := AbsFromThoughtsRel(thoughtsRoot, historyRel)
	if err != nil {
		return out, err
	}
	if err := os.MkdirAll(filepath.Dir(historyAbs), 0o755); err != nil {
		return out, err
	}

	handoffRel, handoffAbs, err := WriteRoomHandoffMarkdown(thoughtsRoot, id, spec)
	if err != nil {
		return out, err
	}

	if err := os.Rename(currentAbs, historyAbs); err != nil {
		return out, err
	}
	if err := os.WriteFile(currentAbs, nil, 0o644); err != nil {
		return out, err
	}

	out = RoomRotateResult{
		Timestamp:  ts,
		HandoffRel: handoffRel,
		HistoryRel: historyRel,
		CurrentRel: currentRel,
		HandoffAbs: handoffAbs,
		HistoryAbs: historyAbs,
		CurrentAbs: currentAbs,
	}
	return out, nil
}

func encodeHandoffMarkdown(files []string, body string) (string, error) {
	var listed []string
	for _, raw := range files {
		rel, err := CanonicalThoughtsPath(raw)
		if err != nil {
			return "", err
		}
		listed = append(listed, rel)
	}
	front, err := yaml.Marshal(map[string]any{"files": listed})
	if err != nil {
		return "", err
	}
	return "---\n" + string(front) + "---\n" + body, nil
}
