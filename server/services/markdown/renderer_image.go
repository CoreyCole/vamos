package markdown

import (
	"bytes"
	"context"
	"path/filepath"
)

type ImageRenderer struct{}

type ImagePreviewArgs struct {
	Filename string
	Src      string
	Alt      string
}

func (r ImageRenderer) Match(req DocumentRequest) bool {
	return isRasterImageExt(req.Extension)
}

func (r ImageRenderer) Render(
	_ context.Context,
	req DocumentRequest,
) (RenderedDocument, error) {
	docPath := "thoughts/" + req.CleanPath
	src := thoughtsRawURL(req.CleanPath)
	filename := filepath.Base(req.CleanPath)
	var buf bytes.Buffer
	if err := ImagePreview(ImagePreviewArgs{
		Filename: filename,
		Src:      src,
		Alt:      filename,
	}).Render(context.Background(), &buf); err != nil {
		return RenderedDocument{}, err
	}
	return RenderedDocument{
		Path:        docPath,
		Title:       DocumentTitle(docPath, nil),
		Kind:        DocumentKindImage,
		HTMLContent: buf.String(),
		Component: ImagePreview(
			ImagePreviewArgs{Filename: filename, Src: src, Alt: filename},
		),
		CommentMode: CommentModeDocumentOnly,
	}, nil
}
