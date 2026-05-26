package agentchat

import (
	"os"
	"path/filepath"
	"strings"
)

type AttachablePlanRoot struct {
	AbsPath       string
	RelPath       string
	ParentRelPath string
	IsNested      bool
	Reason        string
}

func (s *Service) ResolveAttachablePlanRoot(rawPath string) (AttachablePlanRoot, bool) {
	return s.ResolveAttachablePlanRootFrom(rawPath, "")
}

func (s *Service) ResolveAttachablePlanRootFrom(rawPath, cwd string) (AttachablePlanRoot, bool) {
	abs, ok := s.resolveAttachableCandidatePath(rawPath, cwd)
	if !ok {
		return AttachablePlanRoot{}, false
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	root, err := resolveWorkspacePath(s.thoughtsRoot)
	if err != nil {
		return AttachablePlanRoot{}, false
	}
	for dir := filepath.Clean(abs); pathWithinRoot(dir, root); dir = filepath.Dir(dir) {
		if s.isAttachablePlanWorkspaceRoot(dir) {
			rel, err := planWorkspaceRel(root, dir)
			if err != nil {
				return AttachablePlanRoot{}, false
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			isNested := len(parts) > 3
			parentRel := ""
			if isNested {
				parentRel = strings.Join(parts[:3], "/")
			}
			return AttachablePlanRoot{
				AbsPath:       dir,
				RelPath:       rel,
				ParentRelPath: parentRel,
				IsNested:      isNested,
				Reason:        attachablePlanRootReason(dir, isNested),
			}, true
		}
		if filepath.Clean(dir) == root {
			break
		}
	}
	return AttachablePlanRoot{}, false
}

func (s *Service) resolveAttachableCandidatePath(rawPath, cwd string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}
	candidate := rawPath
	if !filepath.IsAbs(candidate) {
		if strings.HasPrefix(filepath.ToSlash(candidate), "thoughts/") {
			candidate = filepath.Join(s.thoughtsRoot, filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(candidate), "thoughts/")))
		} else if strings.TrimSpace(cwd) != "" {
			candidate = filepath.Join(cwd, candidate)
		} else {
			candidate = filepath.Join(s.thoughtsRoot, candidate)
		}
	}
	resolved, err := resolveWorkspacePath(candidate)
	if err != nil {
		return "", false
	}
	root, err := resolveWorkspacePath(s.thoughtsRoot)
	if err != nil || !pathWithinRoot(resolved, root) {
		return "", false
	}
	return resolved, true
}

func (s *Service) isAttachablePlanWorkspaceRoot(dir string) bool {
	rel, err := planWorkspaceRel(s.thoughtsRoot, dir)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "plans" || parts[2] == "" {
		return false
	}
	if len(parts) == 3 {
		return true
	}
	if IsImplementationReviewPlanDir(dir) {
		return true
	}
	if info, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil || info.IsDir() {
		return false
	}
	return hasAttachableQRSPIPlanMarker(dir)
}

func hasAttachableQRSPIPlanMarker(dir string) bool {
	for _, name := range []string{"design.md", "outline.md", "plan.md"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	for _, name := range []string{"questions", "research", "adrs", "reviews"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func attachablePlanRootReason(dir string, nested bool) string {
	if !nested {
		return "top_level_plan"
	}
	if IsImplementationReviewPlanDir(dir) {
		return "implementation_review_plan"
	}
	return "nested_qrspi_plan"
}
