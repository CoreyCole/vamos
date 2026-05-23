package agentchat

import (
	"os"
	"path/filepath"
	"strings"
)

type PlanNodeKind string

const (
	PlanNodeParent                       PlanNodeKind = "parent"
	PlanNodeImplementationReviewFollowup PlanNodeKind = "implementation_review_followup"
)

type PlanNode struct {
	RootRelPath   string
	AbsPath       string
	Kind          PlanNodeKind
	Title         string
	ParentRelPath string
	Children      []PlanNode
	Workspace     *WorkspaceLink
	Stack         *StackSummary
}

type WorkspaceLink struct {
	Slug         string
	CheckoutName string
	CheckoutPath string
	URL          string
	Status       string
	Phase        string
	Error        string
	Actions      []WorkspaceAction
}

type StackSummary struct {
	Branch       string
	TopBranch    string
	BottomParent string
	BaseBranch   string
	AheadCount   int
	BehindCount  int
	Merged       bool
	Available    bool
	Detail       string
}

type WorkspaceAction string

const (
	WorkspaceActionStart   WorkspaceAction = "start"
	WorkspaceActionRetry   WorkspaceAction = "retry"
	WorkspaceActionStop    WorkspaceAction = "stop"
	WorkspaceActionMerge   WorkspaceAction = "merge"
	WorkspaceActionDelete  WorkspaceAction = "delete"
	WorkspaceActionRefresh WorkspaceAction = "refresh"
)

func DiscoverPlanNodes(root string) (PlanNode, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return PlanNode{}, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = filepath.Clean(root)
	}
	parentRoot := planDirectoryRoot(absRoot)
	if parentRoot == "" {
		parentRoot = absRoot
	}

	parent := PlanNode{
		RootRelPath: planNodeRelPath(parentRoot, parentRoot),
		AbsPath:     parentRoot,
		Kind:        PlanNodeParent,
		Title:       planNodeTitle(parentRoot),
		Children:    []PlanNode{},
	}

	reviewsDir := filepath.Join(parentRoot, "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return parent, nil
		}
		return PlanNode{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childPath := filepath.Join(reviewsDir, entry.Name())
		if !IsImplementationReviewPlanDir(childPath) {
			continue
		}
		parent.Children = append(parent.Children, PlanNode{
			RootRelPath:   planNodeRelPath(parentRoot, childPath),
			AbsPath:       childPath,
			Kind:          PlanNodeImplementationReviewFollowup,
			Title:         planNodeTitle(childPath),
			ParentRelPath: parent.RootRelPath,
			Children:      []PlanNode{},
		})
	}
	return parent, nil
}

func IsImplementationReviewPlanDir(path string) bool {
	base := filepath.Base(filepath.Clean(strings.TrimSpace(path)))
	return strings.HasSuffix(base, "_implementation-review") && hasPlanMarkers(path)
}

func hasPlanMarkers(path string) bool {
	for _, name := range []string{"AGENTS.md", "design.md", "outline.md", "plan.md"} {
		if _, err := os.Stat(filepath.Join(path, name)); err != nil {
			return false
		}
	}
	return true
}

func implementationReviewPlanRoot(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return ""
	}
	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	sepPrefix := strings.HasPrefix(rest, string(filepath.Separator))
	parts := strings.Split(
		strings.Trim(rest, string(filepath.Separator)),
		string(filepath.Separator),
	)
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "reviews" ||
			!strings.HasSuffix(parts[i+1], "_implementation-review") {
			continue
		}
		candidateParts := parts[:i+2]
		candidate := filepath.Join(candidateParts...)
		if sepPrefix {
			candidate = string(filepath.Separator) + candidate
		}
		candidate = volume + candidate
		if hasPlanMarkers(candidate) {
			return candidate
		}
	}
	return ""
}

func planNodeRelPath(parentRoot, path string) string {
	rel, err := filepath.Rel(parentRoot, path)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func planNodeTitle(path string) string {
	label, timestamp := formatWorkingDirectoryDisplay(filepath.Base(filepath.Clean(path)))
	if timestamp == "" {
		return label
	}
	return strings.TrimSpace(label + " · " + timestamp)
}

func planNodeKindLabel(kind PlanNodeKind) string {
	switch kind {
	case PlanNodeParent:
		return "Parent plan"
	case PlanNodeImplementationReviewFollowup:
		return "Implementation review follow-up"
	default:
		return "Plan"
	}
}
