package hermescmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type StartPiInput struct {
	Plan            string
	PreviousSession string
	Task            string
	ThreadID        string
	ConfigPath      string
}

type (
	commandRunner        func(context.Context, string, []string, []string, io.Writer, io.Writer) error
	piCompletionNotifier func(context.Context, hostConfig, string, string) error
)

var errHermesManagerNotFound = errors.New("Hermes manager session not found")

const (
	managerWakeManagerThreadID = "VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID"
	managerWakePiSessionID     = "VAMOS_MANAGER_WAKE_PI_SESSION_ID"
	managerWakeGatewayURL      = "VAMOS_MANAGER_WAKE_GATEWAY_URL"
	managerWakeIngressToken    = "VAMOS_MANAGER_WAKE_INGRESS_TOKEN"
)

func newPiCommand(run commandRunner) *cobra.Command {
	cmd := &cobra.Command{Use: "pi", Short: "Launch and record isolated Pi workers"}
	cmd.AddCommand(
		newStartCommand(run),
		newContinueCommand(run),
		newDoneCommand(notifyPiCompletion),
		newResultCommand(),
	)
	return cmd
}

func newStartCommand(run commandRunner) *cobra.Command {
	var input StartPiInput
	cmd := &cobra.Command{
		Use:  "start --plan <absolute-plan-dir> [--previous-session <id>] <task>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input.Task = args[0]
			ctx, err := LoadPlanContext(input.Plan)
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
			var prior *PiResult
			if input.PreviousSession != "" {
				if err := ValidateSafeComponent(input.PreviousSession); err != nil {
					return fmt.Errorf("validate previous Pi session: %w", err)
				}
				result, err := ReadPiResult(ctx.PlanDir, input.PreviousSession)
				if err != nil {
					return fmt.Errorf("read previous result: %w", err)
				}
				prior = &result
			}
			threadID, managed, err := resolveManagedHermesThread(input.ThreadID)
			if err != nil {
				return err
			}
			prompt := RenderPiPrompt(ctx, input.Task, input.PreviousSession, managed)
			contextArgs, err := piContextArgs(ctx, prior)
			if err != nil {
				return err
			}
			dir := ctx.PlanDir + "/.vamos/sessions/pi"
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			promptPath, err := writePromptFile(dir, session, prompt)
			if err != nil {
				return err
			}
			var config hostConfig
			if managed {
				config, err = readManagedHostConfig(input.ConfigPath)
				if err != nil {
					return err
				}
				if err := registerManagedPiRun(
					cmd.Context(), config, ctx.PlanDir, threadID, session,
				); err != nil {
					return err
				}
			}
			env := managedPiEnvironment(
				os.Environ(),
				ctx.PlanDir,
				session,
				threadID,
				config,
			)
			fmt.Fprintln(cmd.OutOrStdout(), "pi session:", session)
			piArgs := append(
				[]string{"--session-id", session, "--session-dir", dir},
				contextArgs...)
			piArgs = append(piArgs, "@"+promptPath)
			return run(
				cmd.Context(),
				"pi",
				piArgs,
				env,
				cmd.OutOrStdout(),
				cmd.ErrOrStderr(),
			)
		},
	}
	cmd.Flags().StringVar(&input.Plan, "plan", "", "absolute plan directory")
	cmd.Flags().
		StringVar(&input.PreviousSession, "previous-session", "", "prior Pi session ID")
	cmd.Flags().StringVar(&input.ThreadID, "thread-id", "", "owning Hermes thread ID")
	cmd.Flags().
		StringVar(&input.ConfigPath, "config", "", "host-local Hermes config path")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func resolveManagedHermesThread(
	requested string,
) (threadID string, managed bool, err error) {
	threadID = strings.TrimSpace(requested)
	if threadID == "" {
		threadID = strings.TrimSpace(os.Getenv("VAMOS_HERMES_THREAD_ID"))
	}
	if threadID == "" {
		return "", false, nil
	}
	if err := ValidateSafeComponent(threadID); err != nil {
		return "", false, fmt.Errorf("validate Hermes thread ID: %w", err)
	}
	return threadID, true, nil
}

func managedPiEnvironment(
	base []string,
	planDir, session, threadID string,
	config hostConfig,
) []string {
	authoritative := map[string]string{
		"VAMOS_PLAN_DIR":      planDir,
		"VAMOS_THOUGHTS_ROOT": thoughtsRoot(planDir),
		"PI_SESSION_ID":       session,
	}
	keys := []string{
		"VAMOS_PLAN_DIR",
		"VAMOS_THOUGHTS_ROOT",
		"PI_SESSION_ID",
		"VAMOS_HERMES_THREAD_ID",
		managerWakeManagerThreadID,
		managerWakePiSessionID,
		managerWakeGatewayURL,
		managerWakeIngressToken,
	}
	if threadID != "" {
		authoritative[managerWakeManagerThreadID] = threadID
		authoritative[managerWakePiSessionID] = session
		authoritative[managerWakeGatewayURL] = strings.TrimSpace(config.GatewayURL)
		authoritative[managerWakeIngressToken] = config.IngressToken
	}
	env := make([]string, 0, len(base)+len(authoritative))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found && containsString(keys, key) {
			continue
		}
		env = append(env, entry)
	}
	for _, key := range keys {
		if value, ok := authoritative[key]; ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readManagedHostConfig(configPath string) (hostConfig, error) {
	if configPath == "" {
		var err error
		configPath, err = defaultConfigPath()
		if err != nil {
			return hostConfig{}, err
		}
	}
	config, err := readHostConfig(configPath)
	if err != nil {
		return hostConfig{}, fmt.Errorf("read Hermes host configuration: %w", err)
	}
	if strings.TrimSpace(config.GatewayURL) == "" ||
		strings.TrimSpace(config.VamosURL) == "" ||
		strings.TrimSpace(config.IngressToken) == "" ||
		strings.TrimSpace(config.CallbackToken) == "" {
		return hostConfig{}, errors.New(
			"Hermes gateway URL, Hermes ingress credential, Vamos callback URL, and Vamos callback credential are required; rerun hermes setup",
		)
	}
	config.GatewayURL = strings.TrimSpace(config.GatewayURL)
	return config, nil
}

func registerManagedPiRun(
	ctx context.Context,
	config hostConfig,
	planDir, threadID, session string,
) error {
	if threadID == "" {
		return nil
	}
	if err := ValidateSafeComponent(threadID); err != nil {
		return fmt.Errorf("validate Hermes thread ID: %w", err)
	}
	if err := ValidateSafeComponent(session); err != nil {
		return fmt.Errorf("validate Pi session ID: %w", err)
	}
	gatewayRequest, err := http.NewRequestWithContext(
		ctx, http.MethodGet, config.GatewayURL, nil,
	)
	if err != nil {
		return fmt.Errorf("build Hermes gateway readiness request: %w", err)
	}
	gatewayResponse, err := http.DefaultClient.Do(gatewayRequest)
	if err != nil {
		return fmt.Errorf("verify Hermes gateway readiness: %w", err)
	}
	gatewayResponse.Body.Close()
	if gatewayResponse.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("verify Hermes gateway readiness: %s", gatewayResponse.Status)
	}
	// Registering the owned session proves authenticated Vamos callback ingress
	// is reachable before the child process starts.
	payload, err := json.Marshal(struct {
		PlanDir     string `json:"plan_dir"`
		ID          string `json:"id"`
		Type        string `json:"type"`
		PiSessionID string `json:"pi_session_id"`
	}{
		PlanDir:     planDir,
		ID:          "pi-run-" + session,
		Type:        "pi_run",
		PiSessionID: session,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(config.VamosURL, "/")+
			"/agent-chat/api/hermes/threads/"+url.PathEscape(threadID)+"/events",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+config.CallbackToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("register managed Pi run: %s", response.Status)
	}

	return nil
}

func piContextArgs(ctx PlanContext, prior *PiResult) ([]string, error) {
	root, err := piResourceRoot()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 5)
	for _, name := range []string{"AGENTS.md", "design.md", "outline.md", "plan.md"} {
		path := filepath.Join(ctx.PlanDir, name)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load Pi context %q: %w", path, err)
		}
	}
	paths = append(
		paths,
		filepath.Join(root, ".pi", "skills", "qrspi-planning", "SKILL.md"),
	)
	if prior != nil {
		if prior.Outcome == OutcomeHandoff {
			paths = append(paths,
				filepath.Join(root, ".pi", "skills", "q-resume", "SKILL.md"),
				filepath.Join(root, ".pi", "skills", "q-handoff", "SKILL.md"),
			)
		}
		paths = append(
			paths,
			filepath.Join(root, ".pi", "skills", "q-"+string(prior.Next), "SKILL.md"),
		)
	}
	if prior != nil && prior.Artifact != "" {
		artifact := prior.Artifact
		if !filepath.IsAbs(artifact) {
			for root := ctx.PlanDir; root != filepath.Dir(root); root = filepath.Dir(root) {
				if filepath.Base(root) == "thoughts" {
					artifact = filepath.Join(
						root,
						strings.TrimPrefix(artifact, "thoughts/"),
					)
					break
				}
			}
		}
		paths = append(paths, artifact)
	}
	args := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("load Pi context %q: %w", path, err)
		}
		args = append(args, "@"+path)
	}
	return args, nil
}

func piResourceRoot() (string, error) {
	if root := os.Getenv("VAMOS_PACKAGE_ROOT"); root != "" {
		return root, nil
	}
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := root; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(
			filepath.Join(dir, ".pi", "skills", "qrspi-planning", "SKILL.md"),
		); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("find Vamos Pi skills from %q", root)
}

func writePromptFile(dir, session, prompt string) (string, error) {
	path := dir + "/" + session + "_prompt.md"
	return path, os.WriteFile(path, []byte(prompt), 0o600)
}

func RenderPiPrompt(plan PlanContext, task, previous string, managed bool) string {
	completion := "Finish by creating durable artifacts, then call `vamos hermes pi done` with PI_SESSION_ID."
	if managed {
		completion = "Settlement is system-recorded. Finish by creating durable artifacts and explain the result normally. Do not invoke completion commands. A fenced YAML or YML block is optional opaque evidence only; it has no required schema, outcome, or successor."
	}
	return fmt.Sprintf(
		"You are an isolated Pi worker.\nPlan: %s\nGoal: %s\nTask: %s\n%s\n",
		plan.PlanRel,
		plan.PlanGoal,
		task,
		completion,
	)
}

func newDoneCommand(notify piCompletionNotifier) *cobra.Command {
	var plan, session, outcomeValue, nextValue, artifact, summary, configPath string
	cmd := &cobra.Command{
		Use: "done [--plan <dir>] [--session <id>] --outcome <outcome> --next <action> --summary <numbered-summary>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if plan == "" {
				plan = os.Getenv("VAMOS_PLAN_DIR")
			}
			if session == "" {
				session = os.Getenv("PI_SESSION_ID")
			}
			ctx, err := LoadPlanContext(plan)
			if err != nil {
				return err
			}
			outcome, err := ParseOutcome(outcomeValue)
			if err != nil {
				return err
			}
			next, err := ParseNextAction(nextValue)
			if err != nil {
				return err
			}
			result := PiResult{
				Session:  session,
				Outcome:  outcome,
				Next:     next,
				Artifact: artifact,
				Summary:  summary,
			}
			path, err := WritePiResult(ctx, result)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			if configPath == "" {
				configPath, err = defaultConfigPath()
				if err != nil {
					return err
				}
			}
			config, err := readHostConfig(configPath)
			if errors.Is(err, os.ErrNotExist) {
				return printManualContinuation(cmd, ctx, result)
			}
			if err != nil {
				return fmt.Errorf("read Hermes host configuration: %w", err)
			}
			if err := notify(cmd.Context(), config, ctx.PlanDir, session); err != nil {
				if errors.Is(err, errHermesManagerNotFound) {
					return printManualContinuation(cmd, ctx, result)
				}
				return fmt.Errorf("notify Vamos of Pi completion: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "", "absolute plan directory")
	cmd.Flags().StringVar(&session, "session", "", "Pi session ID")
	cmd.Flags().StringVar(&outcomeValue, "outcome", "", "completion outcome")
	cmd.Flags().StringVar(&nextValue, "next", "", "non-binding next action")
	cmd.Flags().StringVar(&artifact, "artifact", "", "optional artifact path")
	cmd.Flags().StringVar(&configPath, "config", "", "host-local Hermes config path")
	cmd.Flags().StringVar(&summary, "summary", "", "concise numbered summary")
	for _, name := range []string{"outcome", "next", "summary"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func printManualContinuation(cmd *cobra.Command, ctx PlanContext, result PiResult) error {
	id, err := recordManualResume(ctx, result)
	if err != nil {
		return fmt.Errorf("record manual continuation: %w", err)
	}
	fmt.Fprintln(
		cmd.OutOrStdout(),
		"Pi result recorded locally; no Hermes manager owns this session.",
	)
	fmt.Fprintln(
		cmd.OutOrStdout(),
		"Human action required to start the recommended next session.",
	)
	fmt.Fprintln(
		cmd.OutOrStdout(),
		"Do not run this command from the active Pi worker.",
	)
	fmt.Fprintln(cmd.OutOrStdout(), "After the worker exits, run from a terminal:")
	fmt.Fprintln(cmd.OutOrStdout(), "vamos hermes pi continue", id)
	return nil
}

func newResultCommand() *cobra.Command {
	var plan, session, format string
	cmd := &cobra.Command{
		Use: "result --plan <dir> --session <id>",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := LoadPlanContext(plan)
			if err != nil {
				return err
			}
			result, err := ReadPiResult(ctx.PlanDir, session)
			if err != nil {
				return err
			}
			result.RecommendedCommand = RecommendedCommand(ctx, result)
			if format == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Summary)
			if result.RecommendedCommand != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "recommended:", result.RecommendedCommand)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "", "absolute plan directory")
	cmd.Flags().StringVar(&session, "session", "", "Pi session ID")
	cmd.Flags().StringVar(&format, "format", "text", "text or json")
	_ = cmd.MarkFlagRequired("plan")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

func notifyPiCompletion(
	ctx context.Context,
	config hostConfig,
	planDir, session string,
) error {
	if strings.TrimSpace(config.VamosURL) == "" ||
		strings.TrimSpace(config.CallbackToken) == "" {
		return fmt.Errorf(
			"Vamos callback URL and credential are required; rerun hermes setup",
		)
	}
	payload := strings.NewReader(fmt.Sprintf(`{"plan_dir":%q}`, planDir))
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(
			config.VamosURL,
			"/",
		)+"/agent-chat/api/hermes/pi/"+session+"/complete",
		payload,
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+config.CallbackToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errHermesManagerNotFound
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Vamos callback: %s", resp.Status)
	}
	return nil
}

func defaultRunner(
	ctx context.Context,
	name string,
	args, env []string,
	out, errOut io.Writer,
) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = env
	command.Stdin = os.Stdin
	command.Stdout = out
	command.Stderr = errOut
	return command.Run()
}

func newSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
