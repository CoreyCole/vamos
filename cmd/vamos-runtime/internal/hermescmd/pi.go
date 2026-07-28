package hermescmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type StartPiInput struct {
	Plan            string
	PreviousSession string
	Task            string
}

type (
	commandRunner        func(context.Context, string, []string, []string, io.Writer, io.Writer) error
	piCompletionNotifier func(context.Context, hostConfig, string, string) error
)

func newPiCommand(run commandRunner) *cobra.Command {
	cmd := &cobra.Command{Use: "pi", Short: "Launch and record isolated Pi workers"}
	cmd.AddCommand(
		newStartCommand(run),
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
			prompt := RenderPiPrompt(ctx, input.Task, input.PreviousSession)
			if input.PreviousSession != "" {
				prior, err := ReadPiResult(ctx.PlanDir, input.PreviousSession)
				if err != nil {
					return fmt.Errorf("read previous result: %w", err)
				}
				prompt += "\nPrevious completion:\n" + prior.Summary + "\nArtifact: " + prior.Artifact + "\n"
			}
			dir := ctx.PlanDir + "/.vamos/sessions/pi"
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			promptPath, err := writePromptFile(dir, session, prompt)
			if err != nil {
				return err
			}
			env := append(
				os.Environ(),
				"VAMOS_PI_SESSION_ID="+session,
				"VAMOS_PLAN_DIR="+ctx.PlanDir,
			)
			fmt.Fprintln(cmd.OutOrStdout(), "pi session:", session)
			return run(
				cmd.Context(),
				"pi",
				[]string{
					"--session-id",
					session,
					"--session-dir",
					dir,
					"@" + promptPath,
				},
				env,
				cmd.OutOrStdout(),
				cmd.ErrOrStderr(),
			)
		},
	}
	cmd.Flags().StringVar(&input.Plan, "plan", "", "absolute plan directory")
	cmd.Flags().
		StringVar(&input.PreviousSession, "previous-session", "", "prior Pi session ID")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func writePromptFile(dir, session, prompt string) (string, error) {
	path := dir + "/" + session + "_prompt.md"
	return path, os.WriteFile(path, []byte(prompt), 0o600)
}

func RenderPiPrompt(plan PlanContext, task, previous string) string {
	return fmt.Sprintf(
		"You are an isolated Pi worker.\nPlan: %s\nGoal: %s\nTask: %s\nFinish by creating durable artifacts, then call `vamos hermes pi done` with VAMOS_PI_SESSION_ID.\n",
		plan.PlanRel,
		plan.PlanGoal,
		task,
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
				session = os.Getenv("VAMOS_PI_SESSION_ID")
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
			path, err := WritePiResult(
				ctx,
				PiResult{
					Session:  session,
					Outcome:  outcome,
					Next:     next,
					Artifact: artifact,
					Summary:  summary,
				},
			)
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
			if err != nil {
				return fmt.Errorf("read Hermes host configuration: %w", err)
			}
			if err := notify(cmd.Context(), config, ctx.PlanDir, session); err != nil {
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
