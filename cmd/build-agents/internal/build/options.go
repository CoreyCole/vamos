package build

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type ForceTarget string

const (
	ForceAll            ForceTarget = "all"
	ForceProto          ForceTarget = "proto"
	ForceSQLC           ForceTarget = "sqlc"
	ForceTempl          ForceTarget = "templ"
	ForceGo             ForceTarget = "go"
	ForceTailwind       ForceTarget = "tailwind"
	ForceTSWorker       ForceTarget = "ts-worker"
	ForceDatastarAssets ForceTarget = "datastar-assets"
)

type ForceSet map[ForceTarget]bool

type Options struct {
	RepoRoot   string
	StateDir   string
	BinaryName string

	Clean     bool
	NoRestart bool
	Force     ForceSet

	Stdout   io.Writer
	Stderr   io.Writer
	Reporter Reporter
}

func NewRootCommand(defaults Options) *cobra.Command {
	opts := defaults
	if opts.StateDir == "" {
		opts.StateDir = ".build-agents"
	}
	if opts.BinaryName == "" {
		opts.BinaryName = "agents-server"
	}
	if opts.Force == nil {
		opts.Force = ForceSet{}
	}

	var forceRaw string
	cmd := &cobra.Command{
		Use:          "build-agents",
		SilenceUsage: true,
		Short:        "Smart Vamos build orchestrator",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			parsedForce, err := ParseForce(forceRaw)
			if err != nil {
				return err
			}
			opts.Force = parsedForce
			opts.StateDir = filepath.Clean(opts.StateDir)
			return Run(cmd.Context(), opts)
		},
	}
	cmd.Flags().
		BoolVar(&opts.Clean, "clean", opts.Clean, "clean builder-owned state/cache before building")
	cmd.Flags().
		BoolVar(&opts.NoRestart, "no-restart", opts.NoRestart, "record pending restarts instead of restarting services")
	cmd.Flags().
		StringVar(&forceRaw, "force", "", "force steps: all, proto, sqlc, templ, go, tailwind, ts-worker, datastar-assets, or comma-separated values")
	cmd.Flags().Lookup("force").NoOptDefVal = string(ForceAll)
	return cmd
}

func ParseForce(value string) (ForceSet, error) {
	set := ForceSet{}
	value = strings.TrimSpace(value)
	if value == "" {
		return set, nil
	}
	allowed := map[ForceTarget]bool{
		ForceAll:            true,
		ForceProto:          true,
		ForceSQLC:           true,
		ForceTempl:          true,
		ForceGo:             true,
		ForceTailwind:       true,
		ForceTSWorker:       true,
		ForceDatastarAssets: true,
	}
	for _, raw := range strings.Split(value, ",") {
		target := ForceTarget(strings.TrimSpace(raw))
		if target == "" {
			return nil, fmt.Errorf("empty --force target in %q", value)
		}
		if !allowed[target] {
			return nil, fmt.Errorf("unknown --force target %q", target)
		}
		set[target] = true
	}
	return set, nil
}

func (f ForceSet) Has(target ForceTarget) bool {
	if f == nil {
		return false
	}
	return f[ForceAll] || f[target]
}
