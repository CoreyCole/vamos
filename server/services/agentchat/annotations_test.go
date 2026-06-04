package agentchat

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestCreateChatAnnotationHandlerAnchorsMessageNode(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)

	form := url.Values{}
	form.Set("node_id", "session-1:7")
	form.Set("event_seq", "7")
	form.Set("body_markdown", "Please expand this result.")
	req := httptest.NewRequest(
		http.MethodPost,
		"/chat-sessions/"+session.ID+"/annotations",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.CreateChatAnnotation(c); err != nil {
		t.Fatalf("CreateChatAnnotation() error = %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	annotations, err := service.queries.ListChatAnnotationsBySession(
		context.Background(),
		session.ID,
	)
	if err != nil {
		t.Fatalf("ListChatAnnotationsBySession() error = %v", err)
	}
	if len(annotations) != 1 || annotations[0].NodeID != "session-1:7" ||
		annotations[0].EventSeq != 7 {
		t.Fatalf("annotations = %+v, want anchored annotation", annotations)
	}
}

func TestComposerPostsSelectedAnnotationIDs(t *testing.T) {
	var body bytes.Buffer
	if err := AgentChatComposer(AgentChatComposerArgs{
		Action:      "@post('/agent-chat/chat-sessions/session-1/commands', {contentType: 'form'})",
		Placeholder: "Reply",
		SelectedAnnotations: []SelectedChatAnnotation{{
			ID:       "annotation-1",
			Label:    "session-1:7",
			NodeID:   "session-1:7",
			EventSeq: 7,
		}},
	}).Render(context.Background(), &body); err != nil {
		t.Fatalf("Render composer: %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`name="annotation_ids[]"`,
		`value="annotation-1"`,
		"Annotation: session-1:7",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("composer HTML missing %q:\n%s", want, html)
		}
	}
}

func TestPostChatSessionCommandAppendsAnnotationContext(t *testing.T) {
	service := newTestAgentChatService(t)
	service.temporal = &fakeTemporalStarter{}
	handler := NewHandler(service, nil)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	annotation, err := service.chatSessions.CreateAnnotation(
		context.Background(),
		chatsession.CreateAnnotationInput{
			WorkspaceID:  workspace.ID,
			SessionID:    session.ID,
			NodeID:       "session-1:7",
			EventSeq:     7,
			AuthorEmail:  "owner@example.com",
			BodyMarkdown: "Please expand this result.",
		},
	)
	if err != nil {
		t.Fatalf("CreateAnnotation() error = %v", err)
	}

	form := url.Values{}
	form.Set("type", "message.send")
	form.Set("idempotency_key", "idem-annotation")
	form.Set("prompt", "continue")
	form.Add("annotation_ids[]", annotation.ID)
	req := httptest.NewRequest(
		http.MethodPost,
		"/chat-sessions/"+session.ID+"/commands",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("session_id")
	c.SetParamValues(session.ID)
	c.Set("user_email", "coworker@example.com")

	if err := handler.PostChatSessionCommand(c); err != nil {
		t.Fatalf("PostChatSessionCommand() error = %v", err)
	}
	command, err := service.queries.GetChatCommandByIdempotencyKey(
		context.Background(),
		db.GetChatCommandByIdempotencyKeyParams{
			SessionID:      session.ID,
			IdempotencyKey: "idem-annotation",
		},
	)
	if err != nil {
		t.Fatalf("GetChatCommandByIdempotencyKey() error = %v", err)
	}
	if !strings.Contains(command.PayloadJson, "annotation_context") ||
		!strings.Contains(command.PayloadJson, "Please expand this result.") {
		t.Fatalf("payload = %s, want annotation context", command.PayloadJson)
	}
}

func TestResolveAnnotationRemovesUnresolvedBadge(t *testing.T) {
	ctx := context.Background()
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "owner@example.com")
	session := mustCreateHandlerChatSession(t, service, workspace)
	annotation, err := service.chatSessions.CreateAnnotation(
		ctx,
		chatsession.CreateAnnotationInput{
			WorkspaceID:  workspace.ID,
			SessionID:    session.ID,
			NodeID:       "session-1:7",
			EventSeq:     7,
			AuthorEmail:  "owner@example.com",
			BodyMarkdown: "Please expand this result.",
		},
	)
	if err != nil {
		t.Fatalf("CreateAnnotation() error = %v", err)
	}
	projection := chatsession.ApplyAnnotationCounts(
		chatsession.ChatProjection{
			Tree: chatsession.ChatTreeProjection{
				Nodes: []chatsession.WorkspaceTreeNode{{ID: "session-1:7"}},
			},
		},
		[]db.ChatAnnotation{dbChatAnnotation(t, service, session.ID)},
	)
	if projection.Tree.Nodes[0].UnresolvedCount != 1 {
		t.Fatalf(
			"unresolved before resolve = %d, want 1",
			projection.Tree.Nodes[0].UnresolvedCount,
		)
	}
	if err := service.chatSessions.ResolveAnnotation(
		ctx,
		annotation.ID,
		"coworker@example.com",
	); err != nil {
		t.Fatalf("ResolveAnnotation() error = %v", err)
	}
	projection = chatsession.ApplyAnnotationCounts(
		projection,
		[]db.ChatAnnotation{dbChatAnnotation(t, service, session.ID)},
	)
	if projection.Tree.Nodes[0].UnresolvedCount != 0 {
		t.Fatalf(
			"unresolved after resolve = %d, want 0",
			projection.Tree.Nodes[0].UnresolvedCount,
		)
	}
}

func dbChatAnnotation(
	t *testing.T,
	service *Service,
	sessionID string,
) db.ChatAnnotation {
	t.Helper()
	annotations, err := service.queries.ListChatAnnotationsBySession(
		context.Background(),
		sessionID,
	)
	if err != nil {
		t.Fatalf("ListChatAnnotationsBySession() error = %v", err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations len = %d, want 1", len(annotations))
	}
	return annotations[0]
}
