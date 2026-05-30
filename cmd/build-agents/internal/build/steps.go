package build

import (
	"os"
	"path/filepath"
)

func BuildSteps(opts Options) []Step {
	return []Step{
		ProtoStep(),
		SQLCStep(),
		TemplStep(),
		GoStep(opts.RepoRoot, opts.BinaryName),
		TailwindStep(),
		TSWorkerStep(),
		DatastarAssetsStep(DatastarAssetsOptions{
			RuntimeAsset: "static/js/datastar-pro-v1.js",
			HostAsset:    defaultDatastarHostAsset(),
		}),
	}
}

func defaultDatastarHostAsset() string {
	if asset := os.Getenv("VAMOS_DATASTAR_PRO_ASSET"); asset != "" {
		return asset
	}
	return "../datastar-pro/datastar-pro-v1.js"
}

func ProtoStep() Step {
	return Step{
		Name:        StepProto,
		ForceTarget: ForceProto,
		Inputs: HashSpec{Roots: []string{
			"buf.yaml",
			"buf.gen.yaml",
			"pkg/proto/source",
		}},
		Outputs: HashSpec{
			Roots:    []string{"pkg/proto"},
			Includes: []string{"pkg/proto/**/*.pb.go", "pkg/proto/**/*connect.go"},
			Optional: true,
		},
		Command: CommandSpec{Args: []string{"buf", "generate"}},
	}
}

func SQLCStep() Step {
	return Step{
		Name:        StepSQLC,
		ForceTarget: ForceSQLC,
		Inputs: HashSpec{Roots: []string{
			"sqlc.yaml",
			"pkg/db/queries",
			"pkg/db/migrations/schema.sql",
		}},
		Outputs: HashSpec{
			Roots:    []string{"pkg/db"},
			Includes: []string{"pkg/db/**/*.go"},
			Optional: true,
		},
		Required:  []RequiredPath{{Path: "pkg/db"}},
		Command:   CommandSpec{Args: []string{"sqlc", "generate"}},
		DependsOn: []StepName{StepProto},
	}
}

func TemplStep() Step {
	return Step{
		Name:        StepTempl,
		ForceTarget: ForceTempl,
		Inputs: HashSpec{
			Roots:    []string{"."},
			Includes: []string{"**/*.templ"},
			Excludes: []string{".build-agents/**", "**/node_modules/**", "dist/**"},
		},
		Outputs: HashSpec{
			Roots:    []string{"."},
			Includes: []string{"**/*_templ.go"},
			Excludes: []string{".build-agents/**", "**/node_modules/**", "dist/**"},
			Optional: true,
		},
		Command: CommandSpec{Args: []string{"templ", "generate"}},
	}
}

func GoStep(repoRoot, binaryName string) Step {
	return Step{
		Name:        StepGo,
		ForceTarget: ForceGo,
		Inputs: HashSpec{
			Roots:    []string{"go.mod", "go.sum", "cmd/server", "server", "pkg"},
			Includes: []string{"go.mod", "go.sum", "**/*.go"},
			Excludes: []string{
				".build-agents/**",
				"node_modules/**",
				"dist/**",
			},
		},
		Outputs:        HashSpec{Roots: []string{binaryName}, Optional: true},
		RestartOutputs: HashSpec{Roots: []string{binaryName}, Optional: true},
		Required:       []RequiredPath{{Path: binaryName}},
		Command: CommandSpec{
			Env: map[string]string{
				"GOCACHE": filepath.Join(repoRoot, ".build-agents", "go-build-cache"),
			},
			Args: []string{"go", "build", "-o", binaryName, "./cmd/server"},
		},
		DependsOn: []StepName{StepSQLC, StepTempl},
	}
}

func TailwindStep() Step {
	return Step{
		Name:        StepTailwind,
		ForceTarget: ForceTailwind,
		Inputs: HashSpec{
			Roots: []string{"static/css", "server", "pkg/datastarui"},
			Includes: []string{
				"static/css/**/*.css",
				"server/**",
				"pkg/datastarui/**/*.css",
				"pkg/datastarui/**/*.go",
				"pkg/datastarui/**/*.templ",
				"pkg/datastarui/datastarui.lock.json",
			},
			Excludes: []string{"static/css/out*.css"},
		},
		Outputs: HashSpec{
			Roots:    []string{"static/css"},
			Includes: []string{"static/css/out.css", "static/css/out.*.css"},
			Optional: true,
		},
		RestartOutputs: HashSpec{
			Roots:    []string{"static/css"},
			Includes: []string{"static/css/out.*.css"},
			Optional: true,
		},
		Command:   CommandSpec{Func: BuildTailwindHashedAsset},
		DependsOn: []StepName{StepTempl},
	}
}

func TSWorkerStep() Step {
	return Step{
		Name:        StepTSWorker,
		ForceTarget: ForceTSWorker,
		Inputs: HashSpec{
			Roots: []string{
				"tsconfig.json",
				"package.json",
				"pnpm-lock.yaml",
				"package-lock.json",
				"yarn.lock",
				"pkg/agents/temporal/workers/ts",
			},
			Includes: []string{
				"tsconfig.json",
				"package.json",
				"pnpm-lock.yaml",
				"package-lock.json",
				"yarn.lock",
				"pkg/agents/temporal/workers/ts/**/*.ts",
			},
			Optional: true,
		},
		Outputs: HashSpec{
			Roots: []string{"dist"},
			Excludes: []string{
				"dist/**/*.test.*",
				"dist/**/*_test.js",
				"dist/**/*_test.d.ts",
			},
			Optional: true,
		},
		RestartInputs: HashSpec{
			Roots: []string{
				"package.json",
				"pnpm-lock.yaml",
				"package-lock.json",
				"yarn.lock",
				"pkg/agents/temporal/workers/ts",
			},
			Includes: []string{
				"package.json",
				"pnpm-lock.yaml",
				"package-lock.json",
				"yarn.lock",
				"pkg/agents/temporal/workers/ts/**/*.ts",
			},
			Excludes: []string{
				"pkg/agents/temporal/workers/ts/**/*.test.ts",
			},
			Optional: true,
		},
		RestartOutputs: HashSpec{
			Roots: []string{"dist/pkg/agents/temporal/workers/ts"},
			Excludes: []string{
				"dist/**/*.test.*",
				"dist/**/*_test.js",
				"dist/**/*_test.d.ts",
			},
			Optional: true,
		},
		Prerequisites: []RequiredPath{{Path: "node_modules"}},
		Command:       CommandSpec{Args: []string{"./node_modules/.bin/tsc"}},
	}
}

func DatastarAssetsStep(opts DatastarAssetsOptions) Step {
	return Step{
		Name:        StepDatastarAssets,
		ForceTarget: ForceDatastarAssets,
		Inputs: HashSpec{
			Roots:    []string{"static/js", opts.HostAsset},
			Optional: true,
		},
		Outputs: HashSpec{
			Roots:    []string{opts.RuntimeAsset},
			Optional: true,
		},
		Command: CommandSpec{Func: SyncDatastarAssets(opts)},
	}
}
