package agentchat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	opaqueSettlementDeliveryVersion = 1
	opaqueSettlementScanLimit       = 256
)

// OpaqueSettlementDeliveryRequest keeps the immutable settlement identity and
// the Pi-authored bytes separate. SettlementBytesBase64 is never reconstructed
// from the decoded envelope.
type OpaqueSettlementDeliveryRequest struct {
	Version int `json:"version"`
	// DeliveryID is server-derived from the immutable settlement path and is the
	// receiver's idempotency key; Pi-authored settlement bytes cannot select it.
	DeliveryID            string `json:"delivery_id"`
	Plan                  string `json:"plan"`
	ManagerThread         string `json:"manager_thread"`
	Session               string `json:"session"`
	FinalEntryID          string `json:"final_entry_id"`
	SettlementBytesBase64 string `json:"settlement_bytes_base64"`
}

type opaqueSettlementFence struct {
	Language string `json:"language"`
	Raw      string `json:"raw"`
}

type opaqueSettlementEnvelope struct {
	Version          int                      `json:"version"`
	Kind             string                   `json:"kind"`
	Session          string                   `json:"session"`
	Plan             string                   `json:"plan"`
	ManagerThread    string                   `json:"manager_thread"`
	AssistantEntryID string                   `json:"assistant_entry_id"`
	SettledAt        time.Time                `json:"settled_at"`
	RawResponse      string                   `json:"raw_response"`
	FencedYAMLBlocks *[]opaqueSettlementFence `json:"fenced_yaml_blocks,omitempty"`
}

// OpaqueSettlementReceiver is deliberately a one-way delivery boundary. It
// cannot start or steer a child and it cannot select a successor action.
type OpaqueSettlementReceiver interface {
	DeliverOpaqueSettlement(context.Context, OpaqueSettlementDeliveryRequest) error
}

type opaqueSettlementPlanSource interface {
	Scan(context.Context) ([]DiscoveredPlanWorkspace, error)
}

type OpaqueSettlementDeliveryActivities struct {
	ThoughtsRoot string
	// PlanSource is the bounded server-owned plan projection. Discovery must
	// never infer plans by traversing arbitrary thoughts directories.
	PlanSource opaqueSettlementPlanSource
	Admissions OpaqueSettlementAdmissionStore
	Receiver   OpaqueSettlementReceiver
}

type opaqueSettlementWorker interface {
	RegisterWorkflow(any)
	RegisterActivity(any)
}

// RegisterOpaqueSettlementDelivery installs only the scheduled delivery
// workflow and its IO activity; it does not expose a child-callable command.
func RegisterOpaqueSettlementDelivery(
	worker opaqueSettlementWorker,
	activities *OpaqueSettlementDeliveryActivities,
) {
	worker.RegisterWorkflow(OpaqueSettlementDiscoveryWorkflow)
	worker.RegisterActivity(activities)
}

type OpaqueSettlementDeliveryInput struct{ ThoughtsRoot string }

// OpaqueSettlementDiscoveryWorkflow is scheduled; all filesystem and gateway
// IO stays in its activity so a worker restart only replays immutable requests.
func OpaqueSettlementDiscoveryWorkflow(
	ctx workflow.Context,
	input OpaqueSettlementDeliveryInput,
) error {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 5},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)
	return workflow.ExecuteActivity(ctx, "DeliverOpaqueSettlements", input).Get(ctx, nil)
}

func (a *OpaqueSettlementDeliveryActivities) DeliverOpaqueSettlements(
	ctx context.Context,
	input OpaqueSettlementDeliveryInput,
) error {
	root := strings.TrimSpace(input.ThoughtsRoot)
	if root == "" {
		root = a.ThoughtsRoot
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve opaque settlement thoughts root: %w", err)
	}
	if a.Receiver == nil {
		return errors.New("opaque settlement receiver is nil")
	}
	plans, err := a.planDirectories(ctx, root)
	if err != nil {
		return err
	}
	for _, planDir := range plans {
		if err := a.deliverPlan(ctx, root, planDir); err != nil {
			return err
		}
	}
	return nil
}

func (a *OpaqueSettlementDeliveryActivities) planDirectories(
	ctx context.Context,
	root string,
) ([]string, error) {
	source := a.PlanSource
	if source == nil {
		return nil, errors.New("opaque settlement plan projection is required")
	}
	plans, err := source.Scan(ctx)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(plans))
	seen := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		dir, err := containedOpaqueDirectory(root, plan.PlanDir)
		if err != nil {
			return nil, fmt.Errorf("validate projected plan directory: %w", err)
		}
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) > opaqueSettlementScanLimit {
		return nil, fmt.Errorf(
			"opaque settlement plan discovery exceeds limit %d",
			opaqueSettlementScanLimit,
		)
	}
	sort.Strings(dirs)
	return dirs, nil
}

func containedOpaqueDirectory(root, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes configured thoughts root")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("plan path is not a directory")
	}
	return resolved, nil
}

func (a *OpaqueSettlementDeliveryActivities) deliverPlan(
	ctx context.Context,
	root, planDir string,
) error {
	piRoot := filepath.Join(planDir, ".vamos", "sessions", "pi")
	sessions, err := os.ReadDir(piRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if !session.IsDir() || !safeOpaqueComponent(session.Name()) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(piRoot, session.Name(), "settlements"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() ||
				strings.TrimSuffix(entry.Name(), ".json") == entry.Name() {
				continue
			}
			path := filepath.Join(piRoot, session.Name(), "settlements", entry.Name())
			if err := validateOpaqueSettlementPath(planDir, path); err != nil {
				return err
			}
			if err := a.deliverOne(ctx, root, planDir, path); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateOpaqueSettlementPath permits only the exact v1 settlement location
// under a contained plan and rejects a symlink that resolves outside that plan.
func validateOpaqueSettlementPath(planDir, path string) error {
	plan, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(plan, resolved)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		rel == ".." {
		return errors.New("opaque settlement path escapes plan directory")
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 6 || parts[0] != ".vamos" || parts[1] != "sessions" ||
		parts[2] != "pi" ||
		parts[4] != "settlements" ||
		!safeOpaqueComponent(parts[3]) ||
		!strings.HasSuffix(parts[5], ".json") ||
		!safeOpaqueComponent(strings.TrimSuffix(parts[5], ".json")) {
		return errors.New("opaque settlement path is not an exact settlement json path")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("opaque settlement path is not a regular file")
	}
	return nil
}

func (a *OpaqueSettlementDeliveryActivities) deliverOne(
	ctx context.Context,
	root, planDir, path string,
) error {
	if err := validateOpaqueSettlementPath(planDir, path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	envelope, err := decodeOpaqueSettlement(data)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Join(planDir, ".vamos", "sessions", "pi"), path)
	if err != nil {
		return err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 || parts[1] != "settlements" ||
		strings.TrimSuffix(parts[2], ".json") != envelope.AssistantEntryID ||
		parts[0] != envelope.Session {
		return errors.New("opaque settlement path identity mismatch")
	}
	if envelope.Plan != thoughtsRelativeTo(root, planDir) {
		return errors.New("opaque settlement plan identity mismatch")
	}
	if err := VerifyHermesPiRunBinding(
		planDir,
		envelope.ManagerThread,
		envelope.Session,
	); err != nil {
		return err
	}
	request := OpaqueSettlementDeliveryRequest{
		Version: opaqueSettlementDeliveryVersion,
		DeliveryID: opaqueSettlementDeliveryID(
			envelope.Plan,
			envelope.Session,
			envelope.AssistantEntryID,
		),
		Plan:                  envelope.Plan,
		ManagerThread:         envelope.ManagerThread,
		Session:               envelope.Session,
		FinalEntryID:          envelope.AssistantEntryID,
		SettlementBytesBase64: base64.StdEncoding.EncodeToString(data),
	}
	if a.Admissions == nil {
		return errors.New("opaque settlement admission store is required")
	}
	if err := a.Admissions.Admit(ctx, request, data); err != nil {
		return err
	}
	projection, err := opaqueSettlementProjectionPath(
		planDir,
		envelope.Session,
		envelope.AssistantEntryID,
	)
	if err != nil {
		return err
	}
	unlock, err := lockOpaqueSettlementProjection(projection)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(projection); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := appendOpaqueSettlementAttempt(planDir, request, "started"); err != nil {
		return err
	}
	if err := a.Receiver.DeliverOpaqueSettlement(ctx, request); err != nil {
		_ = appendOpaqueSettlementAttempt(planDir, request, "failed")
		return err
	}
	if err := writeOpaqueSettlementProjection(projection, request); err != nil {
		return err
	}
	return appendOpaqueSettlementAttempt(planDir, request, "delivered")
}

// VerifyHermesPiRunBinding accepts exactly one pi_run for this session in the
// settlement's hinted transcript. It intentionally does not search other
// threads, preventing a forged settlement from routing itself.
func VerifyHermesPiRunBinding(planDir, hintedThread, session string) error {
	if !safeOpaqueComponent(hintedThread) || !safeOpaqueComponent(session) {
		return errors.New("safe Hermes thread and Pi session IDs are required")
	}
	events, err := readHermesTranscript(planDir, hintedThread)
	if err != nil {
		return err
	}
	matches := 0
	for _, event := range events {
		if event.Type == "pi_run" && event.PiSessionID == session {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf(
			"expected exactly one pi_run binding in hinted transcript, found %d",
			matches,
		)
	}
	return nil
}

func decodeOpaqueSettlement(data []byte) (opaqueSettlementEnvelope, error) {
	var e opaqueSettlementEnvelope
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return e, err
	}
	if raw, ok := fields["raw_response"]; !ok || len(raw) == 0 || raw[0] != '"' {
		return e, errors.New("opaque settlement raw_response is required")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		return e, err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return e, errors.New("opaque settlement has trailing data")
	}
	if e.Version != 1 || e.Kind != "pi_assistant_settlement" || e.SettledAt.IsZero() ||
		!safeOpaqueComponent(e.Session) || !safeOpaqueComponent(e.ManagerThread) ||
		!safeOpaqueComponent(e.AssistantEntryID) || strings.TrimSpace(e.Plan) == "" {
		return e, errors.New("invalid opaque settlement")
	}
	if raw, ok := fields["fenced_yaml_blocks"]; ok {
		if bytes.Equal(raw, []byte("null")) {
			return e, errors.New("invalid opaque settlement fences")
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return e, errors.New("invalid opaque settlement fences")
		}
		for _, block := range blocks {
			raw, ok := block["raw"]
			if !ok || len(raw) == 0 || raw[0] != '"' {
				return e, errors.New("invalid opaque settlement fence")
			}
		}
	}
	if e.FencedYAMLBlocks != nil {
		for _, block := range *e.FencedYAMLBlocks {
			if block.Language == "" {
				return e, errors.New("invalid opaque settlement fence")
			}
		}
	}
	return e, nil
}

func safeOpaqueComponent(v string) bool {
	if v == "" || v == "." || v == ".." {
		return false
	}
	for _, r := range v {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func thoughtsRelativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func opaqueSettlementProjectionPath(plan, session, entry string) (string, error) {
	if !safeOpaqueComponent(session) || !safeOpaqueComponent(entry) {
		return "", errors.New("unsafe opaque settlement projection identity")
	}
	return filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		session,
		"delivery-projections",
		entry+".json",
	), nil
}

func opaqueSettlementDeliveryID(plan, session, entry string) string {
	return "opaque-settlement:" + plan + ":" + session + ":" + entry
}

func lockOpaqueSettlementProjection(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func appendOpaqueSettlementAttempt(
	plan string,
	request OpaqueSettlementDeliveryRequest,
	state string,
) error {
	path := filepath.Join(
		plan,
		".vamos",
		"sessions",
		"pi",
		request.Session,
		"delivery-attempts",
		request.FinalEntryID+".jsonl",
	)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		At         time.Time `json:"at"`
		State      string    `json:"state"`
		DeliveryID string    `json:"delivery_id"`
	}{time.Now().UTC(), state, request.DeliveryID})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func writeOpaqueSettlementProjection(
	path string,
	request OpaqueSettlementDeliveryRequest,
) error {
	return writeOpaqueSettlementJSON(path, request, ".delivery-projection-*")
}

func writeOpaqueSettlementJSON(path string, value any, temporaryPattern string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// The projection is created only while its advisory lock is held. Rename
	// prevents readers from observing a partially written server-owned fact.
	tmp, err := os.CreateTemp(filepath.Dir(path), temporaryPattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
