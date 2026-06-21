package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func maybeHandleLauncherCommand(ctx context.Context, args []string, out io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "launcher" {
		return false, nil
	}
	if len(args) == 1 || args[1] == "help" || args[1] == "--help" || args[1] == "-h" {
		printLauncherUsage(out)
		return true, nil
	}

	switch args[1] {
	case "configure":
		configPath, runtimeRoot, err := parseLauncherConfigureArgs(args[2:])
		if err != nil {
			printLauncherUsage(out)
			return true, err
		}
		if configPath == "" {
			configPath, err = defaultLauncherConfigPath()
			if err != nil {
				return true, err
			}
		}
		if err := configureLauncherState(configPath, runtimeRoot); err != nil {
			return true, err
		}
		fmt.Fprintf(out, "configured launcher config: %s\n", configPath)
		fmt.Fprintf(out, "runtime source root: %s\n", mustCleanRuntimeSourceRoot(runtimeRoot))
		return true, nil
	case "doctor":
		configPath, err := parseLauncherDoctorArgs(args[2:])
		if err != nil {
			printLauncherUsage(out)
			return true, err
		}
		if configPath != "" {
			ctx = context.WithValue(ctx, launcherDoctorConfigPathKey{}, configPath)
		}
		return true, doctorLauncher(ctx, out)
	default:
		printLauncherUsage(out)
		return true, fmt.Errorf("unknown launcher command %q", args[1])
	}
}

type launcherDoctorConfigPathKey struct{}

func parseLauncherConfigureArgs(args []string) (configPath string, runtimeRoot string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return "", "", errors.New("--config requires a path")
			}
			configPath = args[i]
		case "--runtime-source-root":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return "", "", errors.New("--runtime-source-root requires an absolute path")
			}
			runtimeRoot = args[i]
		default:
			return "", "", fmt.Errorf("unknown configure flag %q", args[i])
		}
	}
	if strings.TrimSpace(runtimeRoot) == "" {
		return "", "", errors.New("--runtime-source-root is required")
	}
	return configPath, runtimeRoot, nil
}

func parseLauncherDoctorArgs(args []string) (configPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return "", errors.New("--config requires a path")
			}
			configPath = args[i]
		default:
			return "", fmt.Errorf("unknown doctor flag %q", args[i])
		}
	}
	return configPath, nil
}

func configureLauncherState(path, runtimeSourceRoot string) error {
	validated, err := validateRuntimeSourceRoot(runtimeSourceRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		path, err = defaultLauncherConfigPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create launcher config dir %q: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(LauncherConfig{RuntimeSourceRoot: validated}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode launcher config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write launcher config %q: %w", path, err)
	}
	return nil
}

func doctorLauncher(ctx context.Context, out io.Writer) error {
	configPath, err := launcherDoctorConfigPath(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "config: %s\n", configPath)

	cfg, err := loadLauncherConfig(configPath)
	if err != nil {
		return fmt.Errorf("load launcher config %q: %w; run vamos launcher configure --runtime-source-root /path/to/vamos", configPath, err)
	}
	root, err := validateRuntimeSourceRoot(cfg.RuntimeSourceRoot)
	if err != nil {
		return fmt.Errorf("validate runtime source root from %q: %w", configPath, err)
	}
	fmt.Fprintf(out, "runtime source root: %s\n", root)

	source := RuntimeSource{Root: root, SourceKey: sourceRootKey(root), SourceFrom: configPath}
	fp, err := computeRuntimeFingerprint(ctx, source)
	if err != nil {
		return fmt.Errorf("compute runtime fingerprint: %w", err)
	}
	fmt.Fprintf(out, "fingerprint: %s\n", fp.Value)

	cacheDir, err := defaultLauncherCacheDir()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "cache: %s\n", cacheDir)

	target := managedRuntimePath(cacheDir, source, fp)
	if err := ensureManagedRuntime(ctx, source, target); err != nil {
		return fmt.Errorf("ensure managed runtime: %w", err)
	}
	fmt.Fprintf(out, "managed runtime: %s\n", target.BinaryPath)
	fmt.Fprintln(out, "status: ok")
	return nil
}

func launcherDoctorConfigPath(ctx context.Context) (string, error) {
	if path, ok := ctx.Value(launcherDoctorConfigPathKey{}).(string); ok && strings.TrimSpace(path) != "" {
		return path, nil
	}
	return defaultLauncherConfigPath()
}

func printLauncherUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  vamos launcher configure [--config path] --runtime-source-root /absolute/path/to/vamos")
	fmt.Fprintln(out, "  vamos launcher doctor [--config path]")
}

func mustCleanRuntimeSourceRoot(root string) string {
	validated, err := validateRuntimeSourceRoot(root)
	if err != nil {
		return strings.TrimSpace(root)
	}
	return validated
}
