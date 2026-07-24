package qrspicmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ResultHandoffOptions struct {
	PlanDir  string
	ResultID string
	Print    bool
	Inject   bool
}

type AttachManagedSessionOptions struct {
	PlanDir      string
	ProjectRoot  string
	ManagerRunID string
	ResultID     string
	SessionProof string
}

// ResolveResultID finds a durable record only in the selected plan. It never
// consults manager state, so manual handoff cannot accidentally bind a run.
func ResolveResultID(planDir, id string) (ResultRecordRef, error) {
	id = strings.TrimSpace(id)
	if id == "" || filepath.Base(id) != id {
		return ResultRecordRef{}, errors.New("result ID is required")
	}
	entries, err := os.ReadDir(PlanResultDir(planDir))
	if err != nil {
		if os.IsNotExist(err) {
			return ResultRecordRef{}, fmt.Errorf("result record %q was not found", id)
		}
		return ResultRecordRef{}, err
	}
	var matches []ResultRecordRef
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(PlanResultDir(planDir), entry.Name())
		record, err := ReadResultRecord(path)
		if err != nil {
			continue
		}
		if record.ID == id {
			matches = append(matches, ResultRecordRef{ID: id, Path: path})
		}
	}
	switch len(matches) {
	case 0:
		return ResultRecordRef{}, fmt.Errorf("result record %q was not found", id)
	case 1:
		return matches[0], nil
	default:
		return ResultRecordRef{}, fmt.Errorf("result record %q is ambiguous", id)
	}
}

// RenderResultContext is safe to inject into a manual Pi session: it contains
// durable work context, never manager state, policy, or child-authored edges.
func RenderResultContext(record ResultRecord) (string, error) {
	if err := validateRecordShape(record); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("QRSPI durable result context\n")
	fmt.Fprintf(&b, "Result ID: %s\n", record.ID)
	fmt.Fprintf(&b, "Result record: %s\n", record.Node)
	fmt.Fprintf(&b, "Lifecycle state: %s\n", record.State)
	if record.Outcome != "" {
		fmt.Fprintf(&b, "Outcome: %s\n", record.Outcome)
	}
	fmt.Fprintf(&b, "Session transcript: %s\n", record.Session.Path)
	if record.Summary.TextContent() != "" {
		fmt.Fprintf(&b, "Summary: %s\n", record.Summary.TextContent())
	}
	if len(record.Artifacts) > 0 {
		b.WriteString("Artifacts:\n")
		for _, artifact := range record.Artifacts {
			fmt.Fprintf(&b, "- %s: %s\n", artifact.Role, artifact.Path)
		}
	}
	b.WriteString(
		"Read the named plan artifacts and continue manually. This does not attach a q-manager.\n",
	)
	return b.String(), nil
}

func RunResultHandoff(opts ResultHandoffOptions, out io.Writer) error {
	if strings.TrimSpace(opts.PlanDir) == "" {
		return errors.New("plan-dir is required")
	}
	if opts.Print && opts.Inject {
		return errors.New("use only one of --print or --inject")
	}
	ref, err := ResolveResultID(opts.PlanDir, opts.ResultID)
	if err != nil {
		return err
	}
	record, err := ReadResultRecord(ref.Path)
	if err != nil {
		return err
	}
	resultContext, err := RenderResultContext(record)
	if err != nil {
		return err
	}
	// `--inject` deliberately emits the exact prompt payload to stdout. A Pi
	// extension/current session owns delivery; this CLI remains manager-free.
	_, err = fmt.Fprintf(
		ensureWriter(out),
		"Result record path: %s\n%s",
		ref.Path,
		resultContext,
	)
	return err
}

func RunAttachManagedSession(
	ctx context.Context,
	opts AttachManagedSessionOptions,
	d deps,
	out io.Writer,
) error {
	if strings.TrimSpace(opts.PlanDir) == "" ||
		strings.TrimSpace(opts.ManagerRunID) == "" ||
		strings.TrimSpace(opts.ResultID) == "" ||
		strings.TrimSpace(opts.SessionProof) == "" {
		return errors.New("plan-dir, manager-run, result, and session are required")
	}
	projectRoot := strings.TrimSpace(opts.ProjectRoot)
	if projectRoot == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	planDir, err := CanonicalPlanDir(projectRoot, opts.PlanDir)
	if err != nil {
		return err
	}
	repoID, err := RepoID(projectRoot)
	if err != nil {
		return err
	}
	root, err := stateRoot(d)
	if err != nil {
		return err
	}
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	store := stateStore(d, root, clock)
	key := LockKey{RepoID: repoID, CanonicalPlanDir: planDir}
	stateFile := StatePath(root, key, opts.ManagerRunID)
	lock, err := store.AcquireOperationLock(ctx, stateFile)
	if err != nil {
		return err
	}
	defer lock.Release()

	state, err := store.Load(stateFile)
	if err != nil {
		return err
	}
	if state.ManagerRunID != opts.ManagerRunID || state.CanonicalPlanDir != planDir ||
		state.ActiveChild == nil {
		return errors.New("manager run is not eligible for attachment")
	}
	ref, err := ResolveResultID(planDir, opts.ResultID)
	if err != nil {
		return err
	}
	record, err := ReadResultRecord(ref.Path)
	if err != nil {
		return err
	}
	if record.ManagerRunID != state.ManagerRunID ||
		record.SourceChildID != state.ActiveChild.ID ||
		record.SourceChildGeneration != state.ActiveChild.Generation ||
		record.Node != state.ActiveChild.Stage ||
		record.Session.ID != opts.SessionProof ||
		state.ActiveChild.SessionID != opts.SessionProof {
		return errors.New("result is not eligible for this manager session")
	}
	sessionPath, err := ResolveThoughtsReference(planDir, record.Session.Path)
	if err != nil {
		return errors.New("result session path is not eligible for this manager session")
	}
	if state.ActiveChild.SessionPath != "" {
		stateSessionRef, refErr := ThoughtsRelativePath(
			planDir,
			state.ActiveChild.SessionPath,
		)
		if refErr != nil || stateSessionRef != record.Session.Path {
			return errors.New(
				"result session path is not eligible for this manager session",
			)
		}
	}
	claim, err := store.AcquireClaim(ctx, ClaimRequest{
		Key:                     key,
		Operation:               ClaimManagerAttach,
		HolderRunID:             state.ManagerRunID,
		ExpectedRecordID:        record.ID,
		ExpectedChildID:         state.ActiveChild.ID,
		ExpectedChildGeneration: state.ActiveChild.Generation,
		ExpectedTransitionEpoch: state.TransitionEpoch,
	})
	if err != nil {
		return err
	}
	defer store.ReleaseClaim(context.Background(), claim) // best-effort cleanup

	latest, err := store.Load(stateFile)
	if err != nil {
		return err
	}
	if latest.ActiveChild == nil || latest.ManagerRunID != state.ManagerRunID ||
		latest.TransitionEpoch != state.TransitionEpoch || latest.ActiveChild.ID != state.ActiveChild.ID ||
		latest.ActiveChild.Generation != state.ActiveChild.Generation || latest.ActiveChild.SessionID != opts.SessionProof {
		return errors.New("manager changed before attachment")
	}
	latest.ActiveChild.ResultID = ref.ID
	latest.ActiveChild.ResultPath = ref.Path
	latest.ActiveChild.SessionPath = sessionPath
	if err := ValidateResultRecordAt(latest, ref, record); err != nil {
		return err
	}
	if err := store.Save(stateFile, latest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		ensureWriter(out),
		"attached result: %s\nmanager run: %s\n",
		ref.ID,
		state.ManagerRunID,
	)
	return err
}
