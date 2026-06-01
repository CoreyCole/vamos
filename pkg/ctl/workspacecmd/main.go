package workspacecmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

func Main(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if args[0] == "create" {
		fs := flag.NewFlagSet("vamos ctl workspace create", flag.ContinueOnError)
		plan := fs.String("plan", "", "QRSPI plan.md path")
		managerURL := fs.String("manager-url", os.Getenv("VAMOS_WORKSPACE_MANAGER_URL"), "workspace manager URL")
		restartToken := fs.String("restart-token", os.Getenv("VAMOS_WORKSPACE_RESTART_TOKEN"), "workspace restart/API token")
		projectID := fs.String("project", "", "project id for plan workspace binding")
		slug := fs.String("slug", "", "workspace slug")
		path := fs.String("path", "", "requested workspace path")
		source := fs.String("source-checkout", "", "source checkout path")
		baseline := fs.String("baseline-checkout", "", "baseline checkout path")
		trunk := fs.String("trunk", "main", "trunk branch")
		parent := fs.String("parent-stack-ref", "", "parent stack ref for continuation/review follow-up")
		reviewFollowup := fs.Bool("review-followup", false, "base on reviewed implementation stack")
		force := fs.Bool("force", false, "force provision when server allows it")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return RunCreate(ctx, CreateOptions{PlanPath: *plan, ProjectID: *projectID, ManagerURL: *managerURL, RestartToken: *restartToken, WorkspaceSlug: *slug, RequestedPath: *path, SourceCheckout: *source, BaselineCheckout: *baseline, TrunkBranch: *trunk, ParentStackRef: *parent, ReviewFollowup: *reviewFollowup, Force: *force}, os.Stdout)
	}
	if args[0] == "register-current" {
		fs := flag.NewFlagSet(
			"vamos ctl workspace register-current",
			flag.ContinueOnError,
		)
		managerURL := fs.String("manager-url", "", "workspace manager URL")
		restartToken := fs.String("restart-token", "", "workspace restart/API token")
		planDir := fs.String(
			"plan-dir",
			"",
			"QRSPI plan directory to bind to this checkout",
		)
		projectID := fs.String("project", "", "project id for plan workspace binding")
		createdBy := fs.String(
			"created-by",
			"vamos ctl workspace register-current",
			"binding provenance",
		)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return RunRegisterCurrent(ctx, cwd, RegisterOptions{
			ManagerURL:   *managerURL,
			RestartToken: *restartToken,
			PlanDir:      *planDir,
			ProjectID:    *projectID,
			CreatedBy:    *createdBy,
		}, os.Stdout)
	}
	cfg, err := LoadConfig(cwd)
	if err != nil {
		return err
	}
	switch args[0] {
	case "status":
		return RunStatus(ctx, cfg, os.Stdout)
	case "logs":
		fs := flag.NewFlagSet("vamos ctl workspace logs", flag.ContinueOnError)
		tail := fs.Int("tail", 100, "lines to print")
		var target string
		logArgs := args[1:]
		if len(logArgs) > 0 && !strings.HasPrefix(logArgs[0], "-") {
			target = logArgs[0]
			logArgs = logArgs[1:]
		}
		if err := fs.Parse(logArgs); err != nil {
			return err
		}
		if target == "" && fs.NArg() == 1 {
			target = fs.Arg(0)
		}
		if target == "" || fs.NArg() > 1 {
			return fmt.Errorf(
				"usage: vamos ctl workspace logs <web|temporal|ts-worker> [--tail N]",
			)
		}
		return RunLogs(ctx, cfg, WorkspaceLogTarget(target), *tail, os.Stdout)
	case "doctor":
		fs := flag.NewFlagSet("vamos ctl workspace doctor", flag.ContinueOnError)
		tail := fs.Int("tail", 120, "lines to print from each log")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return RunDoctor(ctx, cfg, *tail, os.Stdout)
	case "restart":
		fs := flag.NewFlagSet("vamos ctl workspace restart", flag.ContinueOnError)
		force := fs.Bool("force", false, "force restart after stale process/stop failure")
		components := multiStringFlag{}
		fs.Var(
			&components,
			"component",
			"component to restart: web or ts_worker; repeatable",
		)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		normalized, err := componentsFromWorkspaceCLIFlags(components)
		if err != nil {
			return err
		}
		return RunRestart(ctx, cfg, normalized, *force, os.Stdout)
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf(
		"usage: vamos ctl workspace <create|status|logs|doctor|restart|register-current> [flags]",
	)
}

type multiStringFlag []string

func (f *multiStringFlag) String() string { return strings.Join(*f, ",") }

func (f *multiStringFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}
