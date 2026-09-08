package comments

import (
	"context"

	"github.com/CoreyCole/vamos/pkg/db"
)

type DocCommentCount struct {
	Total    int
	Resolved int
}

func (s *Service) CountsByDocPath(
	ctx context.Context,
	docPaths []string,
) (map[string]DocCommentCount, error) {
	out := make(map[string]DocCommentCount, len(docPaths))
	for _, docPath := range docPaths {
		if docPath == "" {
			continue
		}
		rows, err := s.queries.ListDocumentComments(
			ctx,
			db.ListDocumentCommentsParams{
				DocPath:         docPath,
				IncludeResolved: 1,
			},
		)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		count := DocCommentCount{Total: len(rows)}
		for _, row := range rows {
			if row.Resolved {
				count.Resolved++
			}
		}
		out[docPath] = count
	}
	return out, nil
}
