package agentchat

import (
	"fmt"
	"path/filepath"
	"strings"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	servercfg "github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/services/planworkspace"
)

const workingDirInjectPath = "working-directory.md"

func (s *Service) resolvePlanProcessCwd(
	roomDisk string,
	fm planworkspace.PlanWorkspaceFrontmatter,
) (string, error) {
	cwd := strings.TrimSpace(fm.ImplDir)
	if cwd == "" {
		resolved, err := s.hostWorkingCheckout(fm.Project)
		if err != nil {
			return "", err
		}
		cwd = resolved
	}
	cwd = filepath.Clean(cwd)
	if cwd == "" {
		return "", fmt.Errorf("plan process cwd is empty")
	}
	if samePath(cwd, roomDisk) {
		return "", fmt.Errorf("plan process cwd must not be the plan directory")
	}
	if s.isBaselineCheckout(cwd, fm.Project) {
		return "", fmt.Errorf("plan process cwd must not be the baseline checkout")
	}
	return cwd, nil
}

func (s *Service) hostWorkingCheckout(projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		resolution, err := servercfg.ResolveProjectCheckout(s.projects, projectID)
		if err == nil {
			if root := strings.TrimSpace(resolution.RootPath); root != "" {
				return filepath.Clean(root), nil
			}
		}
	}
	dir, err := servercfg.DefaultWorkingDir(s.projects)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) != "" {
		return filepath.Clean(dir), nil
	}
	return strings.TrimSpace(s.defaultCwd), nil
}

func (s *Service) isBaselineCheckout(cwd, projectID string) bool {
	cwd = filepath.Clean(cwd)
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = strings.TrimSpace(s.projects.DefaultRepo)
	}
	if projectID == "" {
		return false
	}
	_, _, checkout, err := servercfg.BaselineCheckout(s.projects, projectID)
	if err != nil {
		return false
	}
	return samePath(cwd, checkout.RootPath)
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func workingDirInjectFile(
	processCwd, planDirRel, implDir string,
) conversation.InjectFile {
	return conversation.InjectFile{
		Path:    workingDirInjectPath,
		Content: formatWorkingDirContext(processCwd, planDirRel, implDir),
	}
}

func formatWorkingDirContext(processCwd, planDirRel, implDir string) string {
	return formatMessageFrontmatter([]messageFrontmatterField{
		{Key: "process_cwd", Value: processCwd},
		{Key: "plan_dir", Value: planDirRel},
		{Key: "impl_dir", Value: implDir},
	}, "")
}

func workingDirInjectContent(files []conversation.InjectFile) string {
	for _, file := range files {
		if file.Path == workingDirInjectPath {
			return file.Content
		}
	}
	return ""
}

func recordPlanImplDir(roomDisk, implDir string) error {
	implDir = strings.TrimSpace(implDir)
	if implDir == "" {
		return fmt.Errorf("impl_dir is required")
	}
	path := filepath.Join(roomDisk, "AGENTS.md")
	return planworkspace.MergePlanWorkspaceFrontmatter(
		path,
		planworkspace.PlanWorkspaceFrontmatter{
			ImplDir: implDir,
		},
	)
}
