package projectmetadatacmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type options struct {
	root           string
	fromRepository string
	toProject      string
	write          bool
}

func Main(args []string) error {
	if len(args) == 0 || args[0] != "migrate-frontmatter" {
		return errors.New("usage: project-metadata migrate-frontmatter --root <thoughts> --from-repository <name> --to-project <project> [--write]")
	}
	fs := flag.NewFlagSet("migrate-frontmatter", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts options
	fs.StringVar(&opts.root, "root", "", "root directory to scan")
	fs.StringVar(&opts.fromRepository, "from-repository", "", "legacy repository frontmatter value")
	fs.StringVar(&opts.toProject, "to-project", "", "canonical project frontmatter value")
	fs.BoolVar(&opts.write, "write", false, "rewrite matching files instead of dry-run")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(opts.root) == "" || strings.TrimSpace(opts.fromRepository) == "" || strings.TrimSpace(opts.toProject) == "" {
		return errors.New("--root, --from-repository, and --to-project are required")
	}
	return migrateRoot(opts)
}

func migrateRoot(opts options) error {
	root := filepath.Clean(opts.root)
	changed := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		migrated, err := migrateMarkdown(path, opts)
		if err != nil {
			return err
		}
		if migrated {
			changed++
			_, _ = fmt.Fprintln(os.Stdout, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	mode := "would update"
	if opts.write {
		mode = "updated"
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s %d file(s)\n", mode, changed)
	return nil
}

func migrateMarkdown(path string, opts options) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	frontmatter, body, ok := splitYAMLFrontmatter(data)
	if !ok {
		return false, nil
	}
	var fields map[string]any
	if err := yaml.Unmarshal(frontmatter, &fields); err != nil {
		return false, fmt.Errorf("parse %s frontmatter: %w", path, err)
	}
	if !isQRSPIYAML(fields) && !isUnderQRSPIPlan(path) {
		return false, nil
	}
	if yamlString(fields, "repository") != strings.TrimSpace(opts.fromRepository) {
		return false, nil
	}
	fields["project"] = strings.TrimSpace(opts.toProject)
	delete(fields, "repository")
	rewritten, err := marshalFrontmatterPreservingBody(fields, body)
	if err != nil {
		return false, err
	}
	if opts.write {
		if err := os.WriteFile(path, rewritten, 0o644); err != nil {
			return false, err
		}
	}
	return true, nil
}

func splitYAMLFrontmatter(data []byte) ([]byte, []byte, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("---\r\n")):
		const prefix = "---\r\n"
		const marker = "\r\n---\r\n"
		end := bytes.Index(data[len(prefix):], []byte(marker))
		if end < 0 {
			return nil, nil, false
		}
		start := len(prefix)
		end += start
		return data[start:end], data[end+len(marker):], true
	case bytes.HasPrefix(data, []byte("---\n")):
		const prefix = "---\n"
		const marker = "\n---\n"
		end := bytes.Index(data[len(prefix):], []byte(marker))
		if end < 0 {
			return nil, nil, false
		}
		start := len(prefix)
		end += start
		return data[start:end], data[end+len(marker):], true
	default:
		return nil, nil, false
	}
}

func isQRSPIYAML(fields map[string]any) bool {
	return yamlString(fields, "plan_dir") != "" || yamlString(fields, "stage") != ""
}

func yamlString(fields map[string]any, key string) string {
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func isUnderQRSPIPlan(path string) bool {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			return strings.Contains(filepath.ToSlash(dir), "/plans/") || strings.Contains(filepath.ToSlash(dir), "plans/")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

func marshalFrontmatterPreservingBody(fields map[string]any, body []byte) ([]byte, error) {
	frontmatter, err := yaml.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	out := make([]byte, 0, len(frontmatter)+len(body)+8)
	out = append(out, []byte("---\n")...)
	out = append(out, frontmatter...)
	out = append(out, []byte("---\n")...)
	out = append(out, body...)
	return out, nil
}
