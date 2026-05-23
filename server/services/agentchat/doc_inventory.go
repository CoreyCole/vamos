package agentchat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ArtifactInventoryChange struct {
	RelPath string
	DocPath string
	Kind    string
	Action  string
}

func (s *Service) SyncWorkspaceDocInventory(
	ctx context.Context,
	workspace db.Workspace,
) ([]ArtifactInventoryChange, error) {
	root := strings.TrimSpace(workspace.RootDocPath)
	if root == "" {
		return nil, nil
	}

	files, err := listRenderableDocs(root)
	if err != nil {
		return nil, err
	}
	existingRows, err := s.queries.ListWorkspaceDocs(ctx, workspace.ID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]db.WorkspaceDoc, len(existingRows))
	for _, row := range existingRows {
		rel, err := RelPathFromDocPath(root, row.DocPath)
		if err != nil {
			continue
		}
		existing[rel] = row
	}

	seen := make(map[string]struct{}, len(files))
	changes := make([]ArtifactInventoryChange, 0)
	for _, rel := range files {
		cleanRel, err := ValidateWorkspaceRelPath(root, rel)
		if err != nil {
			continue
		}
		seen[cleanRel] = struct{}{}

		abs := filepath.Join(root, filepath.FromSlash(cleanRel))
		info, err := os.Stat(abs)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		hash, err := hashRegularFile(abs)
		if err != nil {
			return nil, err
		}
		action := classifyArtifactChange(
			existing[cleanRel],
			true,
			info.Size(),
			info.ModTime().Unix(),
			hash,
		)
		if action == "" {
			continue
		}
		documentPath, err := DocPathFromRoot(root, cleanRel)
		if err != nil {
			return nil, err
		}
		if err := s.queries.UpsertWorkspaceDoc(ctx, db.UpsertWorkspaceDocParams{
			WorkspaceID: workspace.ID,
			DocPath:     documentPath,
			Kind:        "file",
			SizeBytes:   info.Size(),
			MtimeUnix:   info.ModTime().Unix(),
			ContentHash: nullString(hash),
		}); err != nil {
			return nil, err
		}
		changes = append(
			changes,
			ArtifactInventoryChange{
				RelPath: cleanRel,
				DocPath: documentPath,
				Kind:    "file",
				Action:  action,
			},
		)
	}

	for _, row := range existingRows {
		rel, err := RelPathFromDocPath(root, row.DocPath)
		if err != nil {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		if err := s.queries.MarkWorkspaceDocDeleted(
			ctx,
			db.MarkWorkspaceDocDeletedParams{
				WorkspaceID: workspace.ID,
				DocPath:     row.DocPath,
			},
		); err != nil {
			return nil, err
		}
		changes = append(
			changes,
			ArtifactInventoryChange{
				RelPath: rel,
				DocPath: row.DocPath,
				Kind:    row.Kind,
				Action:  "deleted",
			},
		)
	}

	for _, change := range changes {
		row := existing[change.RelPath]
		key := artifactEventKey(change, row)
		if change.Action != "deleted" {
			documentPath, err := DocPathFromRoot(root, change.RelPath)
			if err != nil {
				return nil, err
			}
			if current, err := s.queries.GetWorkspaceDoc(
				ctx,
				db.GetWorkspaceDocParams{
					WorkspaceID: workspace.ID,
					DocPath:     documentPath,
				},
			); err == nil {
				key = artifactEventKey(change, current)
			}
		}
		documentPath, err := DocPathFromRoot(root, change.RelPath)
		if err != nil {
			return nil, err
		}
		if _, err := s.AppendWorkspaceEvent(ctx, s.queries, AppendWorkspaceEventInput{
			WorkspaceID: workspace.ID,
			EventType:   "artifact_" + change.Action,
			ActorType:   "system",
			DocPath:     documentPath,
			EventKey:    key,
		}); err != nil {
			return nil, err
		}
	}

	return changes, nil
}

func classifyArtifactChange(
	row db.WorkspaceDoc,
	exists bool,
	size int64,
	mtime int64,
	hash string,
) string {
	if !exists || row.WorkspaceID == "" {
		return "created"
	}
	if row.Kind != "file" || row.SizeBytes != size || row.MtimeUnix != mtime ||
		row.ContentHash.String != hash ||
		!row.ContentHash.Valid {
		return "updated"
	}
	return ""
}

func hashRegularFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func artifactEventKey(change ArtifactInventoryChange, row db.WorkspaceDoc) string {
	version := fmt.Sprintf(
		"%s:%d:%d",
		strings.TrimSpace(row.ContentHash.String),
		row.SizeBytes,
		row.MtimeUnix,
	)
	if version == ":0:0" {
		version = "missing"
	}
	return fmt.Sprintf("artifact:%s:%s:%s", change.Action, change.RelPath, version)
}

func workspaceArtifactRelPaths(root string, rows []db.WorkspaceDoc) []string {
	files := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.DocPath) == "" || row.DeletedAt.Valid {
			continue
		}
		rel, err := RelPathFromDocPath(root, row.DocPath)
		if err != nil {
			continue
		}
		files = append(files, rel)
	}
	sort.SliceStable(files, func(i, j int) bool {
		return lessDocPath(files[i], files[j])
	})
	return files
}
