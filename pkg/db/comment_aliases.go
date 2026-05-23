package db

// Backwards-compatible type aliases while comment UI call sites move from the
// retired workspace_doc_comments table to document_comments.
type (
	WorkspaceDocComment      = DocumentComment
	WorkspaceDocCommentReply = DocumentCommentReply
)
