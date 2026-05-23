package agentchat

import (
	"errors"
	"path/filepath"
	"strings"
)

// CanonicalThoughtsPath normalizes a document identity to a clean thoughts/... path.
func CanonicalThoughtsPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", errors.New("document path is required")
	}
	if filepath.IsAbs(p) {
		return "", errors.New("document path must be relative")
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", errors.New("document path escapes thoughts root")
	}
	if !strings.HasPrefix(p, "thoughts/") {
		return "", errors.New("document path must start with thoughts/")
	}
	return p, nil
}

// DocPathFromRoot joins an doc root and workspace-relative path into a
// canonical thoughts/... document identity.
func DocPathFromRoot(docRoot, relPath string) (string, error) {
	root, err := canonicalRootDocPath(docRoot)
	if err != nil {
		return "", err
	}
	rel := strings.TrimSpace(relPath)
	if rel == "" {
		return "", errors.New("doc path is required")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("doc path must be relative")
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", errors.New("doc path escapes workspace root")
	}
	return CanonicalThoughtsPath(strings.TrimSuffix(root, "/") + "/" + rel)
}

// RelPathFromDocPath returns the display/local path under docRoot.
func RelPathFromDocPath(docRoot, documentPath string) (string, error) {
	root, err := canonicalRootDocPath(docRoot)
	if err != nil {
		return "", err
	}
	doc, err := CanonicalThoughtsPath(documentPath)
	if err != nil {
		return "", err
	}
	if doc == root {
		return "", errors.New("document path is workspace root")
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if !strings.HasPrefix(doc, prefix) {
		return "", errors.New("document path is outside workspace root")
	}
	rel := strings.TrimPrefix(doc, prefix)
	if rel == "" {
		return "", errors.New("document path is workspace root")
	}
	return rel, nil
}

func canonicalRootDocPath(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("doc root is required")
	}
	if !filepath.IsAbs(root) {
		return CanonicalThoughtsPath(root)
	}
	clean := filepath.ToSlash(filepath.Clean(root))
	if strings.HasSuffix(clean, "/thoughts") {
		return "thoughts", nil
	}
	marker := "/thoughts/"
	idx := strings.LastIndex(clean, marker)
	if idx == -1 {
		return "", errors.New("absolute doc root must be under thoughts")
	}
	return CanonicalThoughtsPath("thoughts/" + clean[idx+len(marker):])
}

func IsDocumentUnderRoot(docRoot, documentPath string) bool {
	_, err := RelPathFromDocPath(docRoot, documentPath)
	return err == nil
}

func DocPathOrEmpty(docRoot, relPath string) string {
	doc, err := DocPathFromRoot(docRoot, relPath)
	if err != nil {
		return ""
	}
	return doc
}
