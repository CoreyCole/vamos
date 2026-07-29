package hermescmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
)

type manualResumeEntry struct {
	PlanDir string `json:"plan_dir"`
	Session string `json:"session"`
}

type manualResumeIndex struct {
	Entries map[string]manualResumeEntry `json:"entries"`
}

func defaultManualResumeIndexPath() (string, error) {
	if path := os.Getenv("VAMOS_HERMES_RESUME_INDEX"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(
		home,
		".local",
		"state",
		"vamos",
		"hermes",
		"manual-resume.json",
	), nil
}

func manualResumeID(ctx PlanContext, result PiResult) string {
	sum := sha256.Sum256([]byte(ctx.PlanRel + "\x00" + result.Session))
	return hex.EncodeToString(sum[:])[:12]
}

func recordManualResume(ctx PlanContext, result PiResult) (string, error) {
	path, err := defaultManualResumeIndexPath()
	if err != nil {
		return "", err
	}
	index, err := readManualResumeIndex(path)
	if err != nil {
		return "", err
	}
	id := manualResumeID(ctx, result)
	index.Entries[id] = manualResumeEntry{PlanDir: ctx.PlanDir, Session: result.Session}
	if err := writeManualResumeIndex(path, index); err != nil {
		return "", err
	}
	return id, nil
}

func readManualResumeIndex(path string) (manualResumeIndex, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return manualResumeIndex{Entries: make(map[string]manualResumeEntry)}, nil
	}
	if err != nil {
		return manualResumeIndex{}, err
	}
	var index manualResumeIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return manualResumeIndex{}, fmt.Errorf("read manual resume index: %w", err)
	}
	if index.Entries == nil {
		index.Entries = make(map[string]manualResumeEntry)
	}
	return index, nil
}

func writeManualResumeIndex(path string, index manualResumeIndex) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".manual-resume-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use: "doctor", Short: "Report host-local Hermes state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := defaultManualResumeIndexPath()
			if err != nil {
				return err
			}
			index, err := readManualResumeIndex(path)
			if err != nil {
				return err
			}
			var size int64
			if info, err := os.Stat(path); err == nil {
				size = info.Size()
			} else if !os.IsNotExist(err) {
				return err
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"manual resume index: %s\nentries: %d\nsize: %d bytes\n",
				path,
				len(index.Entries),
				size,
			)
			if len(index.Entries) >= 1000 || size >= 1<<20 {
				fmt.Fprintln(cmd.OutOrStdout(), "warning: manual resume index is large")
			}
			return nil
		},
	}
}

func newContinueCommand(run commandRunner) *cobra.Command {
	var threadID, configPath string
	cmd := &cobra.Command{
		Use: "continue <short-id>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := defaultManualResumeIndexPath()
			if err != nil {
				return err
			}
			index, err := readManualResumeIndex(path)
			if err != nil {
				return err
			}
			entry, ok := index.Entries[args[0]]
			if !ok {
				return fmt.Errorf("manual continuation %q not found", args[0])
			}
			plan, err := LoadPlanContext(entry.PlanDir)
			if err != nil {
				return err
			}
			if err := ValidateSafeComponent(entry.Session); err != nil {
				return fmt.Errorf("validate previous Pi session: %w", err)
			}
			result, err := ReadPiResult(plan.PlanDir, entry.Session)
			if err != nil {
				return err
			}
			session, err := newSessionID()
			if err != nil {
				return err
			}
			if err := ValidateSafeComponent(session); err != nil {
				return fmt.Errorf("validate generated Pi session: %w", err)
			}
			threadID, managed, err := resolveManagedHermesThread(threadID)
			if err != nil {
				return err
			}
			prompt := RenderPiPrompt(
				plan,
				"Continue the "+string(result.Next)+" stage using the previous result.",
				entry.Session,
				managed,
			)
			contextArgs, err := piContextArgs(plan, &result)
			if err != nil {
				return err
			}
			dir := filepath.Join(plan.PlanDir, ".vamos", "sessions", "pi")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			promptPath, err := writePromptFile(dir, session, prompt)
			if err != nil {
				return err
			}
			if managed {
				if err := registerManagedPiRun(
					cmd.Context(), configPath, plan.PlanDir, threadID, session,
				); err != nil {
					return err
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "pi session:", session)
			piArgs := append(
				[]string{"--session-id", session, "--session-dir", dir},
				contextArgs...)
			piArgs = append(piArgs, "@"+promptPath)
			return run(
				cmd.Context(),
				"pi",
				piArgs,
				managedPiEnvironment(os.Environ(), plan.PlanDir, session, threadID),
				cmd.OutOrStdout(),
				cmd.ErrOrStderr(),
			)
		},
	}
	cmd.Flags().StringVar(&threadID, "thread-id", "", "owning Hermes thread ID")
	cmd.Flags().StringVar(&configPath, "config", "", "host-local Hermes config path")
	return cmd
}

func manualResumeEntries(index manualResumeIndex) []string {
	ids := make([]string, 0, len(index.Entries))
	for id := range index.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
