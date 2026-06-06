package agentchat

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	datastar "github.com/starfederation/datastar-go/datastar"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	workspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/db"
	servercfg "github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
	"github.com/CoreyCole/vamos/server/services/commentui"
	"github.com/CoreyCole/vamos/server/services/docs"
	"github.com/CoreyCole/vamos/server/services/layoutprefs"
	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/services/theme"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

const resetAndFocusComposerScript = `
const composer = document.getElementById('agent-chat-composer-form');
const input = document.getElementById('agent-chat-composer-input');
composer?.reset();
if (input) {
  input.value = '';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.focus({ preventScroll: true });
}
`

type Handler struct {
	service               *Service
	themeService          *theme.Service
	layoutPrefs           *layoutprefs.Service
	internalToken         string
	internalAllowLoopback bool
	machineCredentials    serverauth.MachineCredentialStore
	projectsConfig        servercfg.ProjectsConfig
	publicBaseURL         string
}

type HandlerOptions struct {
	InternalToken         string
	InternalAllowLoopback bool
	MachineCredentials    serverauth.MachineCredentialStore
	ProjectsConfig        servercfg.ProjectsConfig
	PublicBaseURL         string
}

type planSidebarSubscription struct {
	key   string
	ch    chan WorkspaceStreamSignal
	since int64
}

func NewHandler(
	service *Service,
	themeService *theme.Service,
	opts ...HandlerOptions,
) *Handler {
	options := HandlerOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	return &Handler{
		service:               service,
		themeService:          themeService,
		internalToken:         strings.TrimSpace(options.InternalToken),
		internalAllowLoopback: options.InternalAllowLoopback,
		machineCredentials:    options.MachineCredentials,
		projectsConfig:        options.ProjectsConfig,
		publicBaseURL:         strings.TrimRight(strings.TrimSpace(options.PublicBaseURL), "/"),
	}
}

func (h *Handler) WithLayoutPreferenceService(service *layoutprefs.Service) *Handler {
	h.layoutPrefs = service
	return h
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.HandleAgentChatIndex)
	g.GET("/thoughts/*", h.HandleThoughtsDoc)
	g.GET("/:workspace_id", h.HandleWorkspacePage)
	g.GET("/:workspace_id/thread/:thread_id", h.HandleWorkspacePage)
	h.RegisterRuntimeRoutes(g)
}

func (h *Handler) RegisterNotFoundPageRoutes(g *echo.Group) {
	g.GET("", h.notFoundAgentChatPage)
	g.GET("/", h.notFoundAgentChatPage)
	g.GET("/thoughts/*", h.notFoundAgentChatPage)
	g.GET("/:workspace_id", h.notFoundAgentChatPage)
	g.GET("/:workspace_id/thread/:thread_id", h.notFoundAgentChatPage)
}

func (h *Handler) notFoundAgentChatPage(c echo.Context) error {
	return echo.NewHTTPError(
		http.StatusNotFound,
		"Agent Chat page routes have moved to /thoughts",
	)
}

// RegisterRuntimeRoutes keeps temporary chat/session endpoints available while the
// visible Agent Chat page surface moves under the Thoughts workbench.
// TODO(slice-5-runtime-rehome): move these endpoints under /thoughts/chat/*.
func (h *Handler) RegisterRuntimeRoutes(g *echo.Group) {
	g.POST("/api/runs", h.PostCLIChatRun)
	g.POST("/api/steer", h.PostCLIChatSteer)
	g.GET("/api/chat-sessions/:session_id", h.GetCLIChatSession)
	g.GET("/api/chat-sessions/:session_id/events", h.StreamCLIChatSessionEvents)
	g.GET("/thread/:thread_id/stream", h.StreamThread)
	g.GET("/thread/:thread_id/slash-commands", h.ListThreadSlashCommands)
	g.POST("/thread/:thread_id/resume", h.ResumeThreadByPath)
	g.POST("/thread/:thread_id/fork", h.ForkThreadByPath)
	g.GET("/sessions/stream", h.StreamSessions)
	g.POST("/pi-sessions/open", h.OpenPiSession)
	g.GET("/chat-sessions/:session_id", h.GetChatSessionSnapshot)
	g.GET("/chat-sessions/:session_id/events", h.StreamChatSessionEvents)
	g.POST("/chat-sessions/:session_id/commands", h.PostChatSessionCommand)
	g.POST("/chat-sessions/:session_id/annotations", h.CreateChatAnnotation)
	g.POST(
		"/chat-sessions/:session_id/annotations/:annotation_id/resolve",
		h.ResolveChatAnnotation,
	)
	g.GET("/plan-workspace", h.OpenPlanWorkspace)
	g.GET("/document/open", h.OpenDocumentChat)
	g.POST("/document/open", h.OpenDocumentChat)
	g.POST("/thread/:thread_id/workflow/advance", h.AdvanceThreadWorkflow)
	g.POST("/thread/:thread_id/workflow/policy", h.UpdateThreadWorkflowPolicy)
	g.POST("/thread/:thread_id/workflow/next", h.CreateNextQRSPIThread)
	g.POST("/thread/:thread_id/new", h.CreateThreadFromTarget)
	g.POST("/:workspace_id/send", h.SendWorkspacePrompt)
	g.POST("/:workspace_id/workflow/advance", h.AdvanceWorkspaceWorkflow)
	g.POST("/:workspace_id/workflow/policy", h.UpdateWorkspaceWorkflowPolicy)
	g.GET("/:workspace_id/slash-commands", h.ListWorkspaceSlashCommands)
	g.POST("/:workspace_id/plan-workspace-action", h.PlanWorkspaceAction)
	g.POST("/:workspace_id/docs/select", h.SelectWorkspaceDoc)
	g.POST("/:workspace_id/docs/comments/show", h.ShowWorkspaceDocCommentForm)
	g.POST(
		"/:workspace_id/docs/comments/cancel",
		h.CancelWorkspaceDocCommentForm,
	)
	g.POST("/:workspace_id/docs/comments/expand", h.ExpandWorkspaceDocComments)
	g.POST("/:workspace_id/docs/comments", h.CreateWorkspaceDocComment)
	g.POST(
		"/:workspace_id/docs/comments/:comment_id/replies",
		h.ReplyWorkspaceDocComment,
	)
	g.POST(
		"/:workspace_id/docs/comments/:comment_id/resolve",
		h.ResolveWorkspaceDocComment,
	)
	g.POST(
		"/:workspace_id/docs/comments/:comment_id/reopen",
		h.ReopenWorkspaceDocComment,
	)
	g.GET("/:workspace_id/comments", h.ListWorkspaceDocCommentsAPI)
	g.GET("/:workspace_id/comments/:comment_id", h.GetWorkspaceDocCommentAPI)
	g.POST(
		"/:workspace_id/comments/:comment_id/replies",
		h.AgentReplyWorkspaceDocCommentAPI,
	)
	g.POST(
		"/:workspace_id/comments/:comment_id/resolve",
		h.AgentResolveWorkspaceDocCommentAPI,
	)
	g.POST(
		"/:workspace_id/comments/:comment_id/reopen",
		h.AgentReopenWorkspaceDocCommentAPI,
	)
	g.POST("/send", h.SendPrompt)
	g.POST("/cwd", h.ChangeCwd)
	g.POST("/docs/select", h.SelectDoc)
}

func (h *Handler) HandleThoughtsDoc(c echo.Context) error {
	docPath, err := docs.ParseThoughtsDocPath("agent-chat/thoughts", c.Param("*"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.Redirect(http.StatusFound, docs.ThoughtsDocHref(docPath))
}

func (h *Handler) redirectToThoughtsDoc(c echo.Context, docPath string) error {
	return c.Redirect(http.StatusSeeOther, h.thoughtsDocRedirectURL(docPath, nil))
}

func (h *Handler) thoughtsDocRedirectURL(docPath string, values url.Values) string {
	return thoughtsDocRedirectURLForRoot(h.service.thoughtsRoot, docPath, values)
}

func thoughtsDocRedirectURL(docPath string, values url.Values) string {
	return thoughtsDocRedirectURLForRoot("", docPath, values)
}

func thoughtsDocRedirectURLForRoot(
	thoughtsRoot, docPath string,
	values url.Values,
) string {
	docPath = strings.TrimSpace(filepath.ToSlash(docPath))
	if root := strings.TrimSpace(filepath.ToSlash(thoughtsRoot)); root != "" {
		root = strings.TrimSuffix(root, "/") + "/"
		docPath = strings.TrimPrefix(docPath, root)
	}
	if idx := strings.Index(docPath, "/thoughts/"); idx >= 0 {
		docPath = docPath[idx+len("/thoughts/"):]
	}
	docPath = strings.TrimPrefix(strings.Trim(docPath, "/"), "thoughts/")
	if docPath == "" {
		out := "/thoughts/"
		if len(values) > 0 {
			out += "?" + values.Encode()
		}
		return out
	}
	parts := strings.Split(path.Clean(docPath), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	out := "/thoughts/" + strings.Join(parts, "/")
	if len(values) > 0 {
		out += "?" + values.Encode()
	}
	return out
}

func e2eQRSPIStartPromptOverride(c echo.Context) string {
	if !e2eBoolEnv("VAMOS_E2E_QRSPI_PROMPT_OVERRIDE") {
		return ""
	}
	if prompt := strings.TrimSpace(c.QueryParam("e2e_qrspi_start_prompt")); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(c.FormValue("e2e_qrspi_start_prompt"))
}

func e2eBoolEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (h *Handler) OpenPlanWorkspace(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.Redirect(http.StatusFound, "/login")
	}

	planDir, ok := h.service.canonicalPlanDirFromSource(c.QueryParam("plan_dir"))
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid plan workspace")
	}
	if WorkspaceWorkflowType(
		strings.TrimSpace(c.QueryParam("workflow_type")),
	) != WorkspaceWorkflowQRSPI {
		workspaceRecord, err := h.service.GetOrCreateWorkspaceForRootDocPath(
			c.Request().Context(),
			markdown.ChatWorkspaceOpenInput{
				UserEmail:    userEmail,
				RootDocPath:  planDir,
				Title:        planWorkspaceLabel(planDir),
				WorkflowType: string(WorkspaceWorkflowFreeform),
				Source:       string(WorkspaceSourceWeb),
			},
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return h.redirectToThoughtsDoc(c, workspaceRecord.RootDocPath)
	}

	preset, err := parseWorkflowPolicyPreset(c.QueryParam("policy_preset"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	policyJSON, err := marshalWorkflowPolicy(PolicyForPreset(preset))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	runID, err := h.service.StartWorkflow(c.Request().Context(), StartWorkflowInput{
		UserEmail:      userEmail,
		Title:          planWorkspaceLabel(planDir),
		RootDocPath:    planDir,
		Cwd:            h.service.defaultCwd,
		WorkflowType:   WorkspaceWorkflowQRSPI,
		Policy:         policyJSON,
		PromptOverride: e2eQRSPIStartPromptOverride(c),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if _, err := h.service.WorkspaceIDForRun(c.Request().Context(), runID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return h.redirectToThoughtsDoc(c, planDir)
}

func (h *Handler) OpenDocumentChat(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.Redirect(http.StatusFound, "/login")
	}
	documentPath := strings.TrimSpace(c.QueryParam("doc_path"))
	if documentPath == "" {
		documentPath = strings.TrimSpace(c.FormValue("doc_path"))
	}
	result, err := h.service.OpenDocumentWorkspace(
		c.Request().Context(),
		userEmail,
		documentPath,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	targetDocPath := strings.TrimSpace(result.Workspace.RootDocPath)
	rel := strings.Trim(strings.TrimSpace(result.RelPath), "/")
	if rel != "" && rel != "." {
		targetDocPath = strings.Trim(strings.TrimSpace(targetDocPath), "/") + "/" + rel
	}
	values := url.Values{}
	if parseFormBool(c.FormValue("attach")) || parseFormBool(c.QueryParam("attach")) {
		values.Set("chat", "open")
	}
	return c.Redirect(
		http.StatusSeeOther,
		h.thoughtsDocRedirectURL(targetDocPath, values),
	)
}

func (h *Handler) HandleAgentChatIndex(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.Redirect(http.StatusFound, "/login")
	}

	return h.HandleChatPage(c)
}

func (h *Handler) HandleChatPage(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return c.Redirect(http.StatusFound, "/login")
	}

	args, err := h.service.BuildPageArgs(
		c.Request().Context(),
		userEmail,
		c.QueryParam("thread"),
		c.QueryParam("run"),
		c.QueryParam("doc"),
		c.QueryParam("cwd"),
	)
	if err != nil {
		if isRecoverableThreadWorkspaceMismatch(err) {
			h.logRecoverableThreadWorkspaceMismatch(c, err)
			return c.Redirect(http.StatusSeeOther, "/")
		}
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	currentTheme := "dark"
	currentSyntaxTheme := ""
	if h.themeService != nil {
		currentTheme = h.themeService.GetCurrentThemeMode(c)
		currentSyntaxTheme = h.themeService.GetCurrentTheme(c)
	}

	args.UserEmail = userEmail
	args.CurrentTheme = currentTheme
	args.CurrentSyntaxTheme = currentSyntaxTheme
	workbenchState, err := h.buildFreeformWorkbenchState(c, *args)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	args.Workbench = workbenchState
	return ChatPage(*args).Render(c.Request().Context(), c.Response().Writer)
}

func isRecoverableThreadWorkspaceMismatch(err error) bool {
	return errors.Is(err, ErrThreadWorkspaceMismatch)
}

func (h *Handler) logRecoverableThreadWorkspaceMismatch(c echo.Context, err error) {
	if c == nil || c.Request() == nil {
		log.Printf("workspace_error source=agentchat severity=warn message=%q", err.Error())
		return
	}
	log.Printf(
		"workspace_error source=agentchat severity=warn path=%q query=%q message=%q",
		c.Request().URL.Path,
		c.Request().URL.RawQuery,
		err.Error(),
	)
}

func (h *Handler) buildFreeformWorkbenchState(
	c echo.Context,
	args ChatPageArgs,
) (workbench.WorkbenchState, error) {
	var saved *workbench.WorkbenchConfig
	viewportClass := workbench.ResolveViewportClass(c.Request().Header, c.Request().UserAgent())
	if h.layoutPrefs != nil {
		cfg := h.layoutPrefs.GetOrDefault(
			c.Request().Context(),
			args.UserEmail,
			workbench.WorkbenchPageAgentChat,
			workbench.WorkbenchViewFocus,
			"",
			viewportClass,
		)
		saved = &cfg
	}
	return buildFreeformWorkbenchState(args, saved, c.Request().URL.RequestURI(), viewportClass)
}

func (h *Handler) HandleWorkspacePage(c echo.Context) error {
	if userEmail, ok := c.Get("user_email").(string); !ok || userEmail == "" {
		return c.Redirect(http.StatusFound, "/login")
	}
	return echo.NewHTTPError(
		http.StatusGone,
		"workspace chat routes were removed; use /agent-chat?thread=<id>",
	)
}

func (h *Handler) buildWorkspaceWorkbenchState(
	c echo.Context,
	args WorkspacePageArgs,
) (workbench.WorkbenchState, error) {
	var saved *workbench.WorkbenchConfig
	viewportClass := workbench.ResolveViewportClass(c.Request().Header, c.Request().UserAgent())
	if h.layoutPrefs != nil {
		cfg := h.layoutPrefs.GetOrDefault(
			c.Request().Context(),
			args.UserEmail,
			workbench.WorkbenchPageAgentChat,
			workbench.WorkbenchViewSplit,
			"docs",
			viewportClass,
		)
		saved = &cfg
	}
	return buildWorkspaceWorkbenchState(args, saved, c.Request().URL.RequestURI(), viewportClass)
}

func (h *Handler) StreamSessions(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()
	planSidebarSubs := h.subscribePlanSidebarSignals(userEmail)
	defer h.unsubscribePlanSidebarSignals(planSidebarSubs)

	if err := h.patchPlanSidebar(c, sse, defaultPlanSidebarTargetID); err != nil {
		return err
	}
	h.service.RequestPiSessionIndex(PiSessionIndexRequest{
		UserEmail: userEmail,
		Reason:    "sessions-stream",
	})

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-planSidebarChannel(planSidebarSubs, 0):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 0, signal) {
				if err := h.patchPlanSidebar(
					c,
					sse,
					defaultPlanSidebarTargetID,
				); err != nil {
					return err
				}
			}
		case signal := <-planSidebarChannel(planSidebarSubs, 1):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 1, signal) {
				if err := h.patchPlanSidebar(
					c,
					sse,
					defaultPlanSidebarTargetID,
				); err != nil {
					return err
				}
			}
		}
	}
}

func (h *Handler) subscribePlanSidebarSignals(
	userEmail string,
) []planSidebarSubscription {
	if h.service == nil || h.service.notifier == nil {
		return nil
	}
	keys := []string{
		h.service.planSidebarNotifyKey(userEmail),
		h.service.projectPlanSidebarNotifyKey(),
	}
	subs := make([]planSidebarSubscription, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || seen[key] {
			continue
		}
		seen[key] = true
		ch := h.service.notifier.Subscribe(key)
		subs = append(subs, planSidebarSubscription{
			key:   key,
			ch:    ch,
			since: h.service.CurrentCursor(key),
		})
	}
	return subs
}

func (h *Handler) unsubscribePlanSidebarSignals(subs []planSidebarSubscription) {
	if h.service == nil || h.service.notifier == nil {
		return
	}
	for _, sub := range subs {
		h.service.notifier.Unsubscribe(sub.key, sub.ch)
	}
}

func planSidebarChannel(
	subs []planSidebarSubscription,
	idx int,
) <-chan WorkspaceStreamSignal {
	if idx < 0 || idx >= len(subs) {
		return nil
	}
	return subs[idx].ch
}

func (h *Handler) shouldPatchPlanSidebar(
	subs []planSidebarSubscription,
	idx int,
	signal WorkspaceStreamSignal,
) bool {
	if idx < 0 || idx >= len(subs) || signal.Cursor <= subs[idx].since {
		return false
	}
	subs[idx].since = signal.Cursor
	return true
}

func (h *Handler) patchPlanSidebar(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	targetID string,
) error {
	userEmail, _ := c.Get("user_email").(string)
	state, err := h.service.BuildPlanSidebarState(
		c.Request().Context(),
		PlanSidebarInput{
			UserEmail:      userEmail,
			ProjectID:      strings.TrimSpace(c.QueryParam("project")),
			ActiveThreadID: firstNonEmpty(c.QueryParam("thread"), c.QueryParam("current_thread")),
		},
	)
	if err != nil {
		return err
	}
	state.TargetID = targetID
	return sse.PatchElementTempl(PlanSidebar(state))
}

func (h *Handler) StreamThread(c echo.Context) error {
	threadID := strings.TrimSpace(c.Param("thread_id"))
	if threadID == "" {
		threadID = strings.TrimSpace(c.QueryParam("thread"))
	}
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	userEmail, _ := c.Get("user_email").(string)

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()

	signalCh := h.service.notifier.Subscribe(threadID)
	defer h.service.notifier.Unsubscribe(threadID, signalCh)
	planSidebarSubs := h.subscribePlanSidebarSignals(userEmail)
	defer h.unsubscribePlanSidebarSignals(planSidebarSubs)
	if strings.TrimSpace(userEmail) != "" {
		h.service.RequestPiSessionIndex(PiSessionIndexRequest{
			UserEmail: userEmail,
			Reason:    "thread-stream",
		})
	}

	since := parseSince(c.QueryParam("since"))
	currentCursor := h.service.CurrentCursor(threadID)
	if since < currentCursor {
		if err := h.patchThreadPage(c, sse, PatchThreadPage); err != nil {
			return err
		}
		since = h.service.CurrentCursor(threadID)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signalCh:
			if signal.Cursor <= since {
				continue
			}
			since = signal.Cursor
			if err := h.patchThreadPage(c, sse, signal.Scope); err != nil {
				return err
			}
		case signal := <-planSidebarChannel(planSidebarSubs, 0):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 0, signal) {
				if err := h.patchPlanSidebar(
					c,
					sse,
					defaultPlanSidebarTargetID,
				); err != nil {
					return err
				}
			}
		case signal := <-planSidebarChannel(planSidebarSubs, 1):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 1, signal) {
				if err := h.patchPlanSidebar(
					c,
					sse,
					defaultPlanSidebarTargetID,
				); err != nil {
					return err
				}
			}
		}
	}
}

func (h *Handler) StreamWorkspace(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		workspaceID,
	); err != nil {
		threadID := strings.TrimSpace(c.QueryParam("thread"))
		if _, _, trustedErr := h.service.trustedImportedWorkspaceThread(
			c.Request().Context(),
			workspaceID,
			threadID,
		); trustedErr != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()
	signalCh := h.service.notifier.Subscribe(workspaceID)
	defer h.service.notifier.Unsubscribe(workspaceID, signalCh)
	planSidebarSubs := h.subscribePlanSidebarSignals(userEmail)
	defer h.unsubscribePlanSidebarSignals(planSidebarSubs)
	h.service.RequestPiSessionIndex(PiSessionIndexRequest{
		UserEmail: userEmail,
		Reason:    "workspace-stream",
	})

	since := parseSince(c.QueryParam("since"))
	currentCursor := h.service.CurrentCursor(workspaceID)
	if workspace.NeedsCatchup(since, currentCursor) {
		if err := h.patchWorkspaceResource(c, sse); err != nil {
			return err
		}
		since = h.service.CurrentCursor(workspaceID)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signalCh:
			if signal.Cursor <= since {
				continue
			}
			if signal.Cursor > since+1 || signal.Scope == PatchWorkspaceResource {
				if err := h.patchWorkspaceResource(c, sse); err != nil {
					return err
				}
			} else if err := h.patchWorkspace(c, sse, signal.Scope); err != nil {
				return err
			}
			since = signal.Cursor
		case signal := <-planSidebarChannel(planSidebarSubs, 0):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 0, signal) {
				if err := h.patchWorkspacePlanSidebar(c, sse); err != nil {
					return err
				}
			}
		case signal := <-planSidebarChannel(planSidebarSubs, 1):
			if h.shouldPatchPlanSidebar(planSidebarSubs, 1, signal) {
				if err := h.patchWorkspacePlanSidebar(c, sse); err != nil {
					return err
				}
			}
		}
	}
}

func (h *Handler) StreamEmbeddedFreeform(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	threadID := strings.TrimSpace(c.QueryParam("thread"))
	if threadID == "" {
		return nil
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()
	signalCh := h.service.notifier.Subscribe(threadID)
	defer h.service.notifier.Unsubscribe(threadID, signalCh)
	h.service.RequestPiSessionIndex(PiSessionIndexRequest{
		UserEmail: userEmail,
		Reason:    "embedded-freeform-stream",
	})

	since := parseSince(c.QueryParam("since"))
	currentCursor := h.service.CurrentCursor(threadID)
	if since < currentCursor {
		if err := h.patchEmbeddedFreeformChatPanel(c, sse, userEmail); err != nil {
			return err
		}
		since = h.service.CurrentCursor(threadID)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signalCh:
			if signal.Cursor <= since {
				continue
			}
			if signal.Scope == PatchLiveTranscript {
				if err := h.patchEmbeddedFreeformLiveTranscript(c, sse, userEmail); err != nil {
					return err
				}
			} else if err := h.patchEmbeddedFreeformChatPanel(c, sse, userEmail); err != nil {
				return err
			}
			since = signal.Cursor
		}
	}
}

func (h *Handler) StreamEmbeddedThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	if _, err := h.service.queries.GetAgentThreadForUser(
		c.Request().Context(),
		db.GetAgentThreadForUserParams{ID: threadID, UserEmail: userEmail},
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	workspaceRecord, hasPrimary, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		threadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()
	signalCh := h.service.notifier.Subscribe(threadID)
	defer h.service.notifier.Unsubscribe(threadID, signalCh)
	h.service.RequestPiSessionIndex(PiSessionIndexRequest{
		UserEmail: userEmail,
		Reason:    "embedded-thread-stream",
	})

	patchPanel := func() error {
		if hasPrimary {
			input := h.embeddedPatchInput(c, userEmail)
			input.WorkspaceID = workspaceRecord.ID
			input.ThreadID = threadID
			return h.patchEmbeddedChatPanel(c, sse, input)
		}
		return h.patchEmbeddedFreeformChatPanel(c, sse, userEmail)
	}
	patchTranscript := func() error {
		if hasPrimary {
			input := h.embeddedPatchInput(c, userEmail)
			input.WorkspaceID = workspaceRecord.ID
			input.ThreadID = threadID
			return h.patchEmbeddedChatLiveTranscript(c, sse, input)
		}
		return h.patchEmbeddedFreeformLiveTranscript(c, sse, userEmail)
	}

	since := parseSince(c.QueryParam("since"))
	currentCursor := h.service.CurrentCursor(threadID)
	if since < currentCursor {
		if err := patchPanel(); err != nil {
			return err
		}
		since = h.service.CurrentCursor(threadID)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signalCh:
			if signal.Cursor <= since {
				continue
			}
			if signal.Scope == PatchLiveTranscript {
				if err := patchTranscript(); err != nil {
					return err
				}
			} else if err := patchPanel(); err != nil {
				return err
			}
			since = signal.Cursor
		}
	}
}

func (h *Handler) StreamEmbeddedWorkspace(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		workspaceID,
	); err != nil {
		threadID := strings.TrimSpace(c.QueryParam("thread"))
		if _, _, trustedErr := h.service.trustedImportedWorkspaceThread(
			c.Request().Context(),
			workspaceID,
			threadID,
		); trustedErr != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	ctx := c.Request().Context()
	signalCh := h.service.notifier.Subscribe(workspaceID)
	defer h.service.notifier.Unsubscribe(workspaceID, signalCh)
	h.service.RequestPiSessionIndex(PiSessionIndexRequest{
		UserEmail: userEmail,
		Reason:    "embedded-workspace-stream",
	})

	since := parseSince(c.QueryParam("since"))
	currentCursor := h.service.CurrentCursor(workspaceID)
	if workspace.NeedsCatchup(since, currentCursor) {
		if err := h.patchEmbeddedChatPanel(
			c,
			sse,
			h.embeddedPatchInput(c, userEmail),
		); err != nil {
			return err
		}
		since = h.service.CurrentCursor(workspaceID)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal := <-signalCh:
			if signal.Cursor <= since {
				continue
			}
			if signal.Cursor > since+1 || signal.Scope == PatchWorkspaceResource {
				if err := h.patchEmbeddedChatPanel(
					c,
					sse,
					h.embeddedPatchInput(c, userEmail),
				); err != nil {
					return err
				}
			} else if signal.Scope == PatchLiveTranscript {
				if err := h.patchEmbeddedChatLiveTranscript(
					c,
					sse,
					h.embeddedPatchInput(c, userEmail),
				); err != nil {
					return err
				}
			} else if err := h.patchEmbeddedChatPanel(
				c,
				sse,
				h.embeddedPatchInput(c, userEmail),
			); err != nil {
				return err
			}
			since = signal.Cursor
		}
	}
}

func (h *Handler) parseAttachedPaths(c echo.Context) ([]AttachedPath, error) {
	form, err := c.FormParams()
	if err != nil {
		return nil, err
	}
	raw := form["attached_paths[]"]
	if len(raw) == 0 {
		raw = form["attached_paths"]
	}
	paths := make([]AttachedPath, 0, len(raw))
	for _, p := range raw {
		attached, err := h.service.ValidateAttachedThoughtsPath(p)
		if err != nil {
			return nil, err
		}
		paths = append(paths, attached)
	}
	return paths, nil
}

func (h *Handler) SendEmbeddedFreeformPrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	thread, run, err := h.service.StartThread(
		c.Request().Context(),
		userEmail,
		c.FormValue("cwd"),
		prompt,
		attachments,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	runID := ""
	if run != nil {
		runID = run.ID
	}
	if run != nil && run.WorkspaceID.Valid {
		if err := h.service.PersistEmbeddedChatSelection(
			c.Request().Context(),
			userEmail,
			EmbeddedChatSelection{
				WorkspaceID: run.WorkspaceID.String,
				ThreadID:    thread.ID,
				RunID:       runID,
				Scope:       EmbeddedChatSelectionScopeFreeform,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	args, err := h.service.BuildEmbeddedFreeformPanelArgs(
		c.Request().Context(),
		userEmail,
		thread.ID,
		runID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		EmbeddedFreeformRightRailPanel(args),
		datastar.WithSelectorID("doc-right-chat-panel"),
	)
}

func (h *Handler) ResumeEmbeddedFreeformThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.FormValue("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	return h.resumeEmbeddedFreeformThreadByID(c, userEmail, threadID)
}

func (h *Handler) resumeEmbeddedFreeformThreadByID(c echo.Context, userEmail, threadID string) error {
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	_, run, err := h.service.ResumeThread(
		c.Request().Context(),
		userEmail,
		threadID,
		prompt,
		attachments,
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrThreadRunInProgress) {
			status = http.StatusConflict
		} else if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	runID := ""
	if run != nil {
		runID = run.ID
	}
	if run != nil && run.WorkspaceID.Valid {
		if err := h.service.PersistEmbeddedChatSelection(
			c.Request().Context(),
			userEmail,
			EmbeddedChatSelection{
				WorkspaceID: run.WorkspaceID.String,
				ThreadID:    threadID,
				RunID:       runID,
				Scope:       EmbeddedChatSelectionScopeFreeform,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	args, err := h.service.BuildEmbeddedFreeformPanelArgs(
		c.Request().Context(),
		userEmail,
		threadID,
		runID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		EmbeddedFreeformRightRailPanel(args),
		datastar.WithSelectorID("doc-right-chat-panel"),
	)
}

func (h *Handler) SendPrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}

	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	_, _, err = h.service.StartThread(
		c.Request().Context(),
		userEmail,
		c.FormValue("cwd"),
		prompt,
		attachments,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) ResumeThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	threadID := strings.TrimSpace(c.FormValue("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	return h.resumeFreeformThreadByID(c, userEmail, threadID)
}

func (h *Handler) ResumeThreadByPath(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	if threadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		threadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if ok {
		return h.resumeWorkspaceThreadByID(c, userEmail, workspace.ID, threadID)
	}
	return h.resumeFreeformThreadByID(c, userEmail, threadID)
}

func (h *Handler) resumeFreeformThreadByID(c echo.Context, userEmail, threadID string) error {
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}

	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	_, _, err = h.service.ResumeThread(
		c.Request().Context(),
		userEmail,
		threadID,
		prompt,
		attachments,
	)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrThreadRunInProgress):
			status = http.StatusConflict
		case errors.Is(err, sql.ErrNoRows):
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) ForkThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	sourceThreadID := strings.TrimSpace(c.FormValue("source_thread_id"))
	if sourceThreadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "source_thread_id is required")
	}
	return h.forkFreeformThreadByID(c, userEmail, sourceThreadID)
}

func (h *Handler) ForkThreadByPath(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	sourceThreadID := strings.TrimSpace(c.Param("thread_id"))
	if sourceThreadID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "thread_id is required")
	}
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		sourceThreadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if ok {
		return h.forkWorkspaceThreadByID(c, userEmail, workspace.ID, sourceThreadID)
	}
	return h.forkFreeformThreadByID(c, userEmail, sourceThreadID)
}

func (h *Handler) forkFreeformThreadByID(c echo.Context, userEmail, sourceThreadID string) error {
	sourceEntryID := strings.TrimSpace(c.FormValue("source_entry_id"))
	if sourceEntryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "source_entry_id is required")
	}

	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}

	_, _, err := h.service.ForkThread(
		c.Request().Context(),
		userEmail,
		sourceThreadID,
		sourceEntryID,
		prompt,
	)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, sql.ErrNoRows):
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) OpenPiSession(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	userEmail = strings.TrimSpace(userEmail)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	sessionPath := strings.TrimSpace(c.FormValue("session_path"))
	result, err := h.service.ImportPiSession(
		c.Request().Context(),
		SessionImportInput{
			SessionPath:          sessionPath,
			Source:               AgentSessionSourceAdopted,
			ExplicitWorkspaceID:  c.FormValue("workspace_id"),
			ExplicitWorkspaceDir: c.FormValue("workspace_dir"),
			UserEmail:            userEmail,
		},
	)
	if err != nil {
		h.logPiSessionOpenFailure(c, userEmail, sessionPath, result, err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(result.ThreadID) == "" {
		err := fmt.Errorf(
			"pi session import did not produce a thread (status %s)",
			strings.TrimSpace(result.Status),
		)
		h.logPiSessionOpenFailure(c, userEmail, sessionPath, result, err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	redirectURL, err := h.service.RedirectURLForImportedThread(
		c.Request().Context(),
		result.WorkspaceID,
		result.ThreadID,
	)
	if err != nil {
		h.logPiSessionOpenFailure(c, userEmail, sessionPath, result, err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect(redirectURL)
}

func (h *Handler) logPiSessionOpenFailure(
	c echo.Context,
	userEmail string,
	sessionPath string,
	result SessionImportResult,
	err error,
) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	c.Logger().Warnf(
		"pi session open failed user=%q session_file=%q status=%q workspace_id=%q thread_id=%q error=%q",
		userEmail,
		filepath.Base(strings.TrimSpace(sessionPath)),
		result.Status,
		result.WorkspaceID,
		result.ThreadID,
		message,
	)
}

func (h *Handler) ChangeCwd(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	_, err := h.service.UpdateThreadCwd(
		c.Request().Context(),
		userEmail,
		c.FormValue("thread_id"),
		c.FormValue("cwd"),
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) SelectDoc(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	threadID := strings.TrimSpace(c.FormValue("thread_id"))
	runID := strings.TrimSpace(c.FormValue("run_id"))
	relPath := strings.TrimSpace(c.FormValue("doc"))
	if relPath == "" {
		relPath = strings.TrimSpace(c.FormValue("artifact"))
	}

	if threadID == "" {
		pane, err := h.service.BuildFreeformDocsPane(
			c.Request().Context(),
			userEmail,
			relPath,
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		sse := datastar.NewSSE(c.Response().Writer, c.Request())
		return sse.PatchElementTempl(
			DocPane(pane, ""),
			datastar.WithSelectorID("agent-chat-doc-pane"),
			datastar.WithModeInner(),
		)
	}

	if _, err := h.service.queries.GetAgentThreadForUser(
		c.Request().Context(),
		db.GetAgentThreadForUserParams{
			ID:        threadID,
			UserEmail: userEmail,
		},
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}

	args, err := h.service.BuildPageArgs(
		c.Request().Context(),
		userEmail,
		threadID,
		runID,
		relPath,
		"",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		DocPane(args.DocPane, threadID),
		datastar.WithSelectorID("agent-chat-doc-pane"),
		datastar.WithModeInner(),
	)
}

func (h *Handler) SendWorkspacePrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	thread, run, _, err := h.service.StartWorkspaceThread(
		c.Request().Context(),
		workspaceRow.ID,
		userEmail,
		prompt,
		attachments,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect(
		workspaceThreadURLForRequest(c, workspaceRow.ID, thread.ID, run.ID),
	)
}

func (h *Handler) SendEmbeddedWorkspacePrompt(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	thread, run, _, err := h.service.StartWorkspaceThread(
		c.Request().Context(),
		workspaceRow.ID,
		userEmail,
		prompt,
		attachments,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	docPath := markdown.CanonicalThoughtsDocPathLoose(c.FormValue("doc_path"))
	if docPath == "" {
		docPath = markdown.CanonicalThoughtsDocPathLoose(c.FormValue("doc"))
	}
	selection := EmbeddedChatSelection{
		WorkspaceID: workspaceRow.ID,
		ThreadID:    thread.ID,
		RunID:       run.ID,
		Scope:       embeddedChatSelectionScopeForWorkspace(workspaceRow),
	}
	if err := h.service.PersistEmbeddedChatSelection(
		c.Request().Context(),
		userEmail,
		selection,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	input := EmbeddedChatPatchInput{
		UserEmail:   userEmail,
		DocPath:     docPath,
		WorkspaceID: workspaceRow.ID,
		ThreadID:    thread.ID,
		RunID:       run.ID,
	}
	if err := h.patchEmbeddedChatPanel(c, sse, input); err != nil {
		return err
	}
	if err := h.replaceEmbeddedChatURL(sse, EmbeddedChatURLState{
		DocPath:     docPath,
		WorkspaceID: workspaceRow.ID,
		ThreadID:    thread.ID,
		RunID:       run.ID,
	}); err != nil {
		return err
	}
	return h.resetAndFocusEmbeddedComposer(sse)
}

func embeddedChatSelectionScopeForWorkspace(workspace db.Workspace) EmbeddedChatSelectionScope {
	if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) == WorkspaceWorkflowFreeform {
		return EmbeddedChatSelectionScopeFreeform
	}
	return EmbeddedChatSelectionScopeWorkspace
}

func embeddedChatSelectionScopeForWorkspaceRow(
	workspaceID string,
	service *Service,
	ctx context.Context,
	userEmail string,
) EmbeddedChatSelectionScope {
	workspace, err := service.GetWorkspaceForUserOrTrustedImport(
		ctx,
		userEmail,
		workspaceID,
	)
	if err != nil {
		return EmbeddedChatSelectionScopeWorkspace
	}
	return embeddedChatSelectionScopeForWorkspace(workspace)
}

func (h *Handler) patchEmbeddedChatPanel(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	input EmbeddedChatPatchInput,
) error {
	args, err := h.service.BuildEmbeddedChatPanelArgs(c.Request().Context(), input)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(
		EmbeddedChatRightRailPanel(args),
		datastar.WithSelectorID("doc-right-chat-panel"),
	)
}

func (h *Handler) replaceEmbeddedChatURL(
	sse *datastar.ServerSentEventGenerator,
	state EmbeddedChatURLState,
) error {
	return sse.PatchElements(
		`<div id="thoughts-url-sync" data-replace-url="`+
			stdhtml.EscapeString(strconv.Quote(BuildThoughtsChatDocURL(state)))+
			`"></div>`,
		datastar.WithSelectorID("thoughts-url-sync"),
	)
}

func (h *Handler) resetAndFocusEmbeddedComposer(
	sse *datastar.ServerSentEventGenerator,
) error {
	if err := sse.MarshalAndPatchSignals(map[string]any{
		"agentChatLastWriteOK": true,
	}); err != nil {
		return err
	}
	return sse.ExecuteScript(resetAndFocusComposerScript)
}

func (h *Handler) AttachCurrentDocToEmbeddedThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		threadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "thread has no primary workspace")
	}
	return h.attachCurrentDocToEmbeddedChat(c, userEmail, workspace.ID, threadID)
}

func (h *Handler) AttachCurrentDocToEmbeddedChat(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		workspaceID,
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return h.attachCurrentDocToEmbeddedChat(c, userEmail, workspaceID, strings.TrimSpace(c.FormValue("thread_id")))
}

func (h *Handler) attachCurrentDocToEmbeddedChat(c echo.Context, userEmail, workspaceID, threadID string) error {
	docPath := markdown.CanonicalThoughtsDocPathLoose(c.FormValue("doc_path"))
	if docPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "doc_path is required")
	}
	existingAttachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	currentDocAttached := attachedPathContains(existingAttachments, docPath)
	args, err := h.service.BuildEmbeddedChatPanelArgs(
		c.Request().Context(),
		EmbeddedChatPatchInput{
			UserEmail:   userEmail,
			DocPath:     docPath,
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			RunID:       strings.TrimSpace(c.FormValue("run_id")),
		},
	)
	if !currentDocAttached {
		attached, attachErr := h.service.ValidateAttachedThoughtsPath(
			"thoughts/" + docPath,
		)
		if attachErr != nil {
			return echo.NewHTTPError(http.StatusBadRequest, attachErr.Error())
		}
		existingAttachments = append(existingAttachments, attached)
	} else {
		kept := existingAttachments[:0]
		for _, attached := range existingAttachments {
			if markdown.CanonicalThoughtsDocPathLoose(attached.Path) != docPath {
				kept = append(kept, attached)
			}
		}
		existingAttachments = kept
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	args.PendingAttachments = existingAttachments
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.PatchElementTempl(
		EmbeddedWorkspaceComposer(args),
		datastar.WithSelectorID("agent-chat-workspace-composer"),
	)
}

func (h *Handler) AdvanceThreadWorkflow(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(c.Request().Context(), userEmail, threadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "primary workspace not found")
	}
	if _, err := h.service.AdvanceWorkflowHumanGate(
		c.Request().Context(),
		workspace.ID,
		userEmail,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return h.patchThread(c, sse, threadID, PatchThreadPage)
}

func (h *Handler) AdvanceWorkspaceWorkflow(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if _, err := h.service.AdvanceWorkflowHumanGate(
		c.Request().Context(),
		workspaceID,
		userEmail,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return h.patchWorkspaceResource(c, sse)
}

func (h *Handler) UpdateThreadWorkflowPolicy(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(c.Request().Context(), userEmail, threadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "primary workspace not found")
	}
	retryLimit, err := strconv.Atoi(
		strings.TrimSpace(c.FormValue("invalidResultRetryLimit")),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid retry limit")
	}
	if _, err := h.service.UpdateWorkspaceWorkflowPolicy(
		c.Request().Context(),
		UpdateWorkspaceWorkflowPolicyInput{
			WorkspaceID:             workspace.ID,
			UserEmail:               userEmail,
			AdvanceMode:             qrspi.AdvanceMode(strings.TrimSpace(c.FormValue("advanceMode"))),
			AutoMode:                c.FormValue("autoMode") == "on",
			EnablePlanReviews:       c.FormValue("enablePlanReviews") == "on",
			InvalidResultRetryLimit: retryLimit,
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return h.patchThread(c, sse, threadID, PatchThreadPage)
}

func (h *Handler) CreateNextQRSPIThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	thread, err := h.service.CreateNextQRSPIThread(c.Request().Context(), userEmail, c.Param("thread_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect(h.threadRedirectURL(c.Request().Context(), userEmail, thread.ID))
}

func (h *Handler) CreateThreadFromTarget(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	kind := NewThreadTargetKind(strings.TrimSpace(firstNonEmpty(c.FormValue("target_kind"), c.QueryParam("target_kind"))))
	if kind == "" {
		kind = NewThreadTargetPrimary
	}
	if kind != NewThreadTargetPrimary && kind != NewThreadTargetRelated && kind != NewThreadTargetFreeform {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid target kind")
	}
	thread, err := h.service.CreateThreadFromWorkspace(c.Request().Context(), CreateThreadFromWorkspaceInput{
		UserEmail:         userEmail,
		SourceThreadID:    c.Param("thread_id"),
		TargetWorkspaceID: strings.TrimSpace(firstNonEmpty(c.FormValue("workspace_id"), c.QueryParam("workspace_id"))),
		TargetKind:        kind,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return sse.Redirect(h.threadRedirectURL(c.Request().Context(), userEmail, thread.ID))
}

func (h *Handler) threadRedirectURL(ctx context.Context, userEmail, threadID string) string {
	workspaceContext, err := h.service.GetThreadWorkspaceContext(ctx, userEmail, threadID)
	if err == nil {
		return threadThoughtsURL(workspaceContext)
	}
	return BuildThoughtsChatDocURL(EmbeddedChatURLState{Context: ThoughtsChatContext, ThreadID: threadID})
}

func (h *Handler) UpdateWorkspaceWorkflowPolicy(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	retryLimit, err := strconv.Atoi(
		strings.TrimSpace(c.FormValue("invalidResultRetryLimit")),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid retry limit")
	}
	if _, err := h.service.UpdateWorkspaceWorkflowPolicy(
		c.Request().Context(),
		UpdateWorkspaceWorkflowPolicyInput{
			WorkspaceID:             c.Param("workspace_id"),
			UserEmail:               userEmail,
			AdvanceMode:             qrspi.AdvanceMode(strings.TrimSpace(c.FormValue("advanceMode"))),
			AutoMode:                c.FormValue("autoMode") == "on",
			EnablePlanReviews:       c.FormValue("enablePlanReviews") == "on",
			InvalidResultRetryLimit: retryLimit,
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return h.patchWorkspace(c, sse, PatchWorkflowPanel)
}

func (h *Handler) ListThreadSlashCommands(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		threadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !ok {
		workspaceContext, ctxErr := h.service.GetThreadWorkspaceContext(c.Request().Context(), userEmail, threadID)
		if ctxErr != nil {
			return echo.NewHTTPError(http.StatusNotFound, ctxErr.Error())
		}
		return h.listSlashCommandsForCwd(c, firstNonEmpty(c.QueryParam("cwd"), workspaceContext.Thread.Cwd))
	}
	cwd := strings.TrimSpace(c.QueryParam("cwd"))
	if cwd == "" {
		cwd = h.service.WorkspaceSlashCommandCwd(workspace)
	}
	return h.listSlashCommandsForCwd(c, cwd)
}

func (h *Handler) ListWorkspaceSlashCommands(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspace, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusBadRequest
		}
		return echo.NewHTTPError(status, err.Error())
	}
	cwd := strings.TrimSpace(c.QueryParam("cwd"))
	if cwd == "" {
		cwd = h.service.WorkspaceSlashCommandCwd(workspace)
	}
	return h.listSlashCommandsForCwd(c, cwd)
}

func (h *Handler) listSlashCommandsForCwd(c echo.Context, cwd string) error {
	commands, err := h.service.ListSlashCommands(
		c.Request().Context(),
		ListSlashCommandsInput{Cwd: cwd, Prefix: c.QueryParam("prefix")},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, commands)
}

func (h *Handler) PlanWorkspaceAction(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	if _, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		workspaceID,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	if err := h.service.HandlePlanWorkspaceAction(
		c.Request().Context(),
		PlanWorkspaceActionInput{
			WorkspaceID:   workspaceID,
			PlanRoot:      c.FormValue("plan_root"),
			Slug:          c.FormValue("slug"),
			Action:        WorkspaceAction(c.FormValue("action")),
			ConfirmDelete: c.FormValue("confirm_delete"),
			ActorEmail:    userEmail,
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if h.service.notifier != nil {
		h.service.notifier.NotifyWorkspaceResource(workspaceID)
	}
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	return h.patchWorkspaceResource(c, sse)
}

func (h *Handler) ResumeWorkspaceThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := c.Param("workspace_id")
	threadID := strings.TrimSpace(c.Param("thread_id"))
	return h.resumeWorkspaceThreadByID(c, userEmail, workspaceID, threadID)
}

func (h *Handler) resumeWorkspaceThreadByID(c echo.Context, userEmail, workspaceID, threadID string) error {
	if _, err := h.service.queries.GetAgentThreadForWorkspaceUser(
		c.Request().Context(),
		db.GetAgentThreadForWorkspaceUserParams{
			ThreadID:    threadID,
			WorkspaceID: workspaceID,
			UserEmail:   userEmail,
		},
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if _, _, _, err := h.service.ResumeWorkspaceThread(
		c.Request().Context(),
		workspaceID,
		userEmail,
		threadID,
		prompt,
		attachments,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrThreadRunInProgress) {
			status = http.StatusConflict
		} else if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	return h.writeNoRedirectSuccess(c)
}

func (h *Handler) ResumeEmbeddedThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	workspace, ok, err := h.service.ResolvePrimaryWorkspaceForThread(
		c.Request().Context(),
		userEmail,
		threadID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !ok {
		return h.resumeEmbeddedFreeformThreadByID(c, userEmail, threadID)
	}
	return h.resumeEmbeddedWorkspaceThread(c, userEmail, workspace.ID, threadID)
}

func (h *Handler) ResumeEmbeddedWorkspaceThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := strings.TrimSpace(c.Param("workspace_id"))
	threadID := strings.TrimSpace(c.Param("thread_id"))
	return h.resumeEmbeddedWorkspaceThread(c, userEmail, workspaceID, threadID)
}

func (h *Handler) resumeEmbeddedWorkspaceThread(c echo.Context, userEmail, workspaceID, threadID string) error {
	if _, err := h.service.queries.GetAgentThreadForWorkspaceUser(
		c.Request().Context(),
		db.GetAgentThreadForWorkspaceUserParams{
			ThreadID:    threadID,
			WorkspaceID: workspaceID,
			UserEmail:   userEmail,
		},
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	attachments, err := h.parseAttachedPaths(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	_, run, _, err := h.service.ResumeWorkspaceThread(
		c.Request().Context(),
		workspaceID,
		userEmail,
		threadID,
		prompt,
		attachments,
	)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrThreadRunInProgress) {
			status = http.StatusConflict
		} else if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	runID := ""
	if run != nil {
		runID = run.ID
	}
	docPath := markdown.CanonicalThoughtsDocPathLoose(c.FormValue("doc_path"))
	if docPath == "" {
		docPath = markdown.CanonicalThoughtsDocPathLoose(c.FormValue("doc"))
	}
	selection := EmbeddedChatSelection{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		RunID:       runID,
		Scope:       embeddedChatSelectionScopeForWorkspaceRow(workspaceID, h.service, c.Request().Context(), userEmail),
	}
	if err := h.service.PersistEmbeddedChatSelection(
		c.Request().Context(),
		userEmail,
		selection,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	input := EmbeddedChatPatchInput{
		UserEmail:   userEmail,
		DocPath:     docPath,
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		RunID:       runID,
	}
	if err := h.patchEmbeddedChatPanel(c, sse, input); err != nil {
		return err
	}
	if err := h.replaceEmbeddedChatURL(sse, EmbeddedChatURLState{
		DocPath:     docPath,
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		RunID:       runID,
	}); err != nil {
		return err
	}
	return h.resetAndFocusEmbeddedComposer(sse)
}

func (h *Handler) ForkWorkspaceThread(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceID := c.Param("workspace_id")
	sourceThreadID := strings.TrimSpace(c.Param("thread_id"))
	return h.forkWorkspaceThreadByID(c, userEmail, workspaceID, sourceThreadID)
}

func (h *Handler) forkWorkspaceThreadByID(c echo.Context, userEmail, workspaceID, sourceThreadID string) error {
	if _, err := h.service.queries.GetAgentThreadForWorkspaceUser(
		c.Request().Context(),
		db.GetAgentThreadForWorkspaceUserParams{
			ThreadID:    sourceThreadID,
			WorkspaceID: workspaceID,
			UserEmail:   userEmail,
		},
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	sourceEntryID := strings.TrimSpace(c.FormValue("source_entry_id"))
	if sourceEntryID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "source_entry_id is required")
	}
	prompt := strings.TrimSpace(c.FormValue("prompt"))
	if prompt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "prompt is required")
	}
	if _, _, _, err := h.service.ForkWorkspaceThread(
		c.Request().Context(),
		workspaceID,
		userEmail,
		sourceThreadID,
		sourceEntryID,
		prompt,
	); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return echo.NewHTTPError(status, err.Error())
	}
	return h.writeNoRedirectSuccess(c)
}

func (h *Handler) SelectWorkspaceDoc(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	relPath, err := workspaceDocFormPath(
		workspaceRow.RootDocPath,
		firstNonEmpty(c.FormValue("doc"), c.FormValue("artifact")),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.service.queries.UpdateWorkspaceSelectedDoc(
		c.Request().Context(),
		db.UpdateWorkspaceSelectedDocParams{
			ID:              workspaceRow.ID,
			SelectedDocPath: nullString(relPath),
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if relPath != "" {
		event, err := h.service.AppendWorkspaceEvent(
			c.Request().Context(),
			h.service.queries,
			AppendWorkspaceEventInput{
				WorkspaceID: workspaceRow.ID,
				EventType:   "doc_selected",
				ActorEmail:  userEmail,
				ActorType:   "user",
				DocPath:     relPath,
				EventKey:    "doc_selected:" + relPath,
			},
		)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		h.service.NotifyWorkspaceForEvent(event)
	}
	return h.writeNoRedirectSuccess(c)
}

func (h *Handler) ShowWorkspaceDocCommentForm(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	refresh := docCommentTargetRefreshFromForm(workspaceRow.ID, c)
	args, err := h.service.LoadWorkspaceDocCommentTarget(
		c.Request().Context(),
		userEmail,
		refresh,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	target := docCommentTargetView(args)
	form := docCommentFormView(WorkspaceDocCommentFormArgs{
		WorkspaceID:  workspaceRow.ID,
		DocPath:      refresh.DocRelPath,
		SectionHint:  refresh.SectionHint,
		HeadingHint:  refresh.HeadingHint,
		SelectedText: c.FormValue("selected_text"),
	}, target)
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := sse.PatchElementTempl(
		commentui.CommentTargetWithForm(target, form),
		datastar.WithSelectorID(target.ID),
	); err != nil {
		return err
	}
	return sse.MarshalAndPatchSignals(map[string]any{
		"comment_" + target.SignalKey + "_expanded": true,
	})
}

func (h *Handler) CancelWorkspaceDocCommentForm(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	refresh := docCommentTargetRefreshFromForm(workspaceRow.ID, c)
	return h.patchWorkspaceDocCommentTarget(c, userEmail, refresh, nil)
}

func (h *Handler) ExpandWorkspaceDocComments(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || strings.TrimSpace(userEmail) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	refresh := docCommentTargetRefreshFromForm(workspaceRow.ID, c)
	return h.patchWorkspaceDocCommentTarget(c, userEmail, refresh, nil)
}

func (h *Handler) patchWorkspaceDocCommentTarget(
	c echo.Context,
	userEmail string,
	refresh WorkspaceDocCommentTargetRefresh,
	withForm *WorkspaceDocCommentFormArgs,
) error {
	args, err := h.service.LoadWorkspaceDocCommentTarget(
		c.Request().Context(),
		userEmail,
		refresh,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	target := docCommentTargetView(args)
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if withForm != nil {
		form := docCommentFormView(*withForm, target)
		if err := sse.PatchElementTempl(
			commentui.CommentTargetWithForm(target, form),
			datastar.WithSelectorID(target.ID),
		); err != nil {
			return err
		}
	} else if err := sse.PatchElementTempl(
		commentui.CommentTarget(target),
		datastar.WithSelectorID(target.ID),
	); err != nil {
		return err
	}
	return nil
}

func (h *Handler) CreateWorkspaceDocComment(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	comment, err := h.service.CreateWorkspaceDocComment(
		c.Request().Context(),
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspaceRow.ID,
			DocPath: firstNonEmpty(
				c.FormValue("doc_rel_path"),
				c.FormValue("artifact_rel_path"),
			),
			UserEmail:   userEmail,
			CommentText: c.FormValue("comment_text"),
			Anchor: WorkspaceDocCommentAnchor{
				SelectedText: c.FormValue("selected_text"),
				SectionHint:  c.FormValue("section_hint"),
				HeadingHint:  c.FormValue("heading_hint"),
				StartLine:    parseFormInt(c.FormValue("start_line")),
				StartColumn:  parseFormInt(c.FormValue("start_column")),
				EndLine:      parseFormInt(c.FormValue("end_line")),
				EndColumn:    parseFormInt(c.FormValue("end_column")),
			},
		},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return h.patchWorkspaceDocCommentTarget(
		c,
		userEmail,
		docCommentTargetRefreshFromComment(comment),
		nil,
	)
}

func (h *Handler) refreshForWorkspaceDocCommentAction(
	c echo.Context,
	userEmail string,
	workspace db.Workspace,
	commentID string,
) (WorkspaceDocCommentTargetRefresh, error) {
	refresh := docCommentTargetRefreshFromForm(workspace.ID, c)
	if strings.TrimSpace(refresh.DocRelPath) != "" {
		return refresh, nil
	}
	view, err := h.service.GetAgentFacingWorkspaceDocComment(
		c.Request().Context(),
		userEmail,
		workspace.ID,
		commentID,
	)
	if err != nil {
		return WorkspaceDocCommentTargetRefresh{}, err
	}
	return docCommentTargetRefreshFromComment(view.Comment), nil
}

func (h *Handler) ReplyWorkspaceDocComment(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	commentID := c.Param("comment_id")
	if _, err := h.service.ReplyToWorkspaceDocComment(
		c.Request().Context(),
		ReplyWorkspaceDocCommentInput{
			WorkspaceID: workspaceRow.ID,
			CommentID:   commentID,
			UserEmail:   userEmail,
			ActorType:   "user",
			ReplyText:   c.FormValue("reply_text"),
			RequestID:   c.FormValue("request_id"),
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	refresh, err := h.refreshForWorkspaceDocCommentAction(
		c,
		userEmail,
		workspaceRow,
		commentID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return h.patchWorkspaceDocCommentTarget(c, userEmail, refresh, nil)
}

func (h *Handler) ResolveWorkspaceDocComment(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	commentID := c.Param("comment_id")
	requestID := c.FormValue("request_id")
	if reply := strings.TrimSpace(c.FormValue("reply_text")); reply != "" {
		replyRequestID := ""
		if strings.TrimSpace(requestID) != "" {
			replyRequestID = requestID + ":reply"
		}
		if _, err := h.service.ReplyToWorkspaceDocComment(
			c.Request().Context(),
			ReplyWorkspaceDocCommentInput{
				WorkspaceID: workspaceRow.ID,
				CommentID:   commentID,
				UserEmail:   userEmail,
				ActorType:   "user",
				ReplyText:   reply,
				RequestID:   replyRequestID,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if err := h.service.ResolveWorkspaceDocCommentWithRequest(
		c.Request().Context(),
		userEmail,
		workspaceRow.ID,
		commentID,
		userEmail,
		"user",
		requestID,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	refresh, err := h.refreshForWorkspaceDocCommentAction(
		c,
		userEmail,
		workspaceRow,
		commentID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return h.patchWorkspaceDocCommentTarget(c, userEmail, refresh, nil)
}

func (h *Handler) ReopenWorkspaceDocComment(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	commentID := c.Param("comment_id")
	requestID := c.FormValue("request_id")
	if reply := strings.TrimSpace(c.FormValue("reply_text")); reply != "" {
		replyRequestID := ""
		if strings.TrimSpace(requestID) != "" {
			replyRequestID = requestID + ":reply"
		}
		if _, err := h.service.ReplyToWorkspaceDocComment(
			c.Request().Context(),
			ReplyWorkspaceDocCommentInput{
				WorkspaceID: workspaceRow.ID,
				CommentID:   commentID,
				UserEmail:   userEmail,
				ActorType:   "user",
				ReplyText:   reply,
				RequestID:   replyRequestID,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if err := h.service.ReopenWorkspaceDocCommentWithRequest(
		c.Request().Context(),
		userEmail,
		workspaceRow.ID,
		commentID,
		userEmail,
		"user",
		requestID,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	refresh, err := h.refreshForWorkspaceDocCommentAction(
		c,
		userEmail,
		workspaceRow,
		commentID,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return h.patchWorkspaceDocCommentTarget(c, userEmail, refresh, nil)
}

func (h *Handler) ListWorkspaceDocCommentsAPI(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	views, err := h.service.ListAgentFacingWorkspaceDocComments(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
		c.QueryParam("doc"),
		parseFormBool(c.QueryParam("resolved")),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"comments": views})
}

func (h *Handler) GetWorkspaceDocCommentAPI(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	view, err := h.service.GetAgentFacingWorkspaceDocComment(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
		c.Param("comment_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, view)
}

func (h *Handler) AgentReplyWorkspaceDocCommentAPI(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	reply, err := h.service.ReplyToWorkspaceDocComment(
		c.Request().Context(),
		ReplyWorkspaceDocCommentInput{
			WorkspaceID: workspaceRow.ID,
			CommentID:   c.Param("comment_id"),
			UserEmail:   userEmail,
			ActorType:   "agent",
			ReplyText:   c.FormValue("message"),
			RequestID:   c.FormValue("request_id"),
		},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, reply)
}

func (h *Handler) AgentResolveWorkspaceDocCommentAPI(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if err := h.service.AgentReplyAndMaybeResolveWorkspaceDocComment(
		c.Request().Context(),
		AgentCommentActionInput{
			WorkspaceID: workspaceRow.ID,
			CommentID:   c.Param("comment_id"),
			UserEmail:   userEmail,
			ReplyText:   c.FormValue("message"),
			Resolve:     true,
			DocUpdated: parseFormBool(
				firstNonEmpty(
					c.FormValue("doc_updated"),
					c.FormValue("artifact_updated"),
				),
			),
			ArtifactUpdated:  parseFormBool(c.FormValue("artifact_updated")),
			NoChangeDecision: parseFormBool(c.FormValue("no_change_decision")),
			RequestID:        c.FormValue("request_id"),
		},
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) AgentReopenWorkspaceDocCommentAPI(c echo.Context) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	workspaceRow, err := h.service.GetWorkspaceForUserOrTrustedImport(
		c.Request().Context(),
		userEmail,
		c.Param("workspace_id"),
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	requestID := c.FormValue("request_id")
	if reply := strings.TrimSpace(c.FormValue("message")); reply != "" {
		replyRequestID := ""
		if strings.TrimSpace(requestID) != "" {
			replyRequestID = requestID + ":reply"
		}
		if _, err := h.service.ReplyToWorkspaceDocComment(
			c.Request().Context(),
			ReplyWorkspaceDocCommentInput{
				WorkspaceID: workspaceRow.ID,
				CommentID:   c.Param("comment_id"),
				UserEmail:   userEmail,
				ActorType:   "agent",
				ReplyText:   reply,
				RequestID:   replyRequestID,
			},
		); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if err := h.service.ReopenWorkspaceDocCommentWithRequest(
		c.Request().Context(),
		userEmail,
		workspaceRow.ID,
		c.Param("comment_id"),
		userEmail,
		"agent",
		requestID,
	); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) HandleInternalPiSessionImport(c echo.Context) error {
	if !h.trustedInternalRequest(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid internal token")
	}
	var payload struct {
		SessionPath  string `json:"session_path"`
		WorkspaceID  string `json:"workspace_id"`
		WorkspaceDir string `json:"workspace_dir"`
		UserEmail    string `json:"user_email"`
		Source       string `json:"source"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid import payload")
	}
	result, err := h.service.ImportPiSession(c.Request().Context(), SessionImportInput{
		SessionPath:          payload.SessionPath,
		Source:               AgentSessionSource(strings.TrimSpace(payload.Source)),
		ExplicitWorkspaceID:  payload.WorkspaceID,
		ExplicitWorkspaceDir: payload.WorkspaceDir,
		UserEmail:            payload.UserEmail,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) HandleInternalWorkspaceProbe(c echo.Context) error {
	if !h.trustedInternalRequest(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid internal token")
	}
	var req workspaces.AgentChatProbeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid probe payload")
	}
	result, err := h.service.RunWorkspaceProbe(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, result)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) HandleInternalRunSnapshot(c echo.Context) error {
	if !h.trustedInternalRequest(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid internal token")
	}
	runID := strings.TrimSpace(c.QueryParam("run_id"))
	if runID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "run_id is required")
	}
	run, err := h.service.queries.GetAgentRun(c.Request().Context(), runID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	snapshot, err := h.service.BuildSnapshot(
		c.Request().Context(),
		run.ThreadID,
		run.RestoreHeadEntryID.String,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) HandleInternalRunEvent(c echo.Context) error {
	if !h.trustedInternalRequest(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid internal token")
	}
	var env conversation.EventEnvelope
	if err := c.Bind(&env); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event envelope")
	}
	run, err := h.validateRunEnvelope(c, env)
	if err != nil {
		return err
	}

	switch {
	case isLiveConversationEvent(env.EventType):
		if run.ID != "" && run.Status != agentRunStatusRunning {
			return c.NoContent(http.StatusAccepted)
		}
		if err := h.service.ApplyLiveAgentEvent(c.Request().Context(), env); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid live stream payload")
		}
		if strings.TrimSpace(env.WorkspaceID) != "" {
			h.service.notifyLiveTranscriptDirty(env.WorkspaceID, env.ThreadID)
		} else {
			h.service.notifyThreadScope(
				c.Request().Context(),
				env.ThreadID,
				PatchLiveTranscript,
			)
		}
		return c.NoContent(http.StatusAccepted)
	case env.EventType == conversation.EventCheckpoint:
		var cp conversation.Checkpoint
		if err := json.Unmarshal([]byte(env.PayloadJSON), &cp); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid checkpoint payload")
		}
		if cp.EventKey == "" {
			cp.EventKey = env.EventKey
		}
		if cp.WorkspaceID == "" {
			cp.WorkspaceID = env.WorkspaceID
		}
		if cp.SessionID == "" {
			cp.SessionID = env.SessionID
		}
		if err := h.service.ApplyCheckpoint(c.Request().Context(), cp); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	case env.EventType == conversation.EventRunComplete:
		var result conversation.RunResult
		if err := json.Unmarshal([]byte(env.PayloadJSON), &result); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid run result payload")
		}
		if result.EventKey == "" {
			result.EventKey = env.EventKey
		}
		if result.WorkspaceID == "" {
			result.WorkspaceID = env.WorkspaceID
		}
		if result.SessionID == "" {
			result.SessionID = env.SessionID
		}
		if err := h.service.FinalizeRun(c.Request().Context(), result); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	case env.EventType == conversation.EventRunFailed:
		var failure conversation.RunFailure
		if err := json.Unmarshal([]byte(env.PayloadJSON), &failure); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid run failure payload")
		}
		if failure.EventKey == "" {
			failure.EventKey = env.EventKey
		}
		if failure.WorkspaceID == "" {
			failure.WorkspaceID = env.WorkspaceID
		}
		if failure.SessionID == "" {
			failure.SessionID = env.SessionID
		}
		if err := h.service.FailRun(
			c.Request().Context(),
			failure,
		); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.NoContent(http.StatusAccepted)
}

const agentRunStatusRunning = "running"

func isLiveConversationEvent(eventType string) bool {
	return workspace.IsLiveConversationEvent(eventType)
}

func (h *Handler) validateRunEnvelope(
	c echo.Context,
	env conversation.EventEnvelope,
) (db.AgentRun, error) {
	runID := strings.TrimSpace(env.RunID)
	if runID == "" {
		return db.AgentRun{}, nil
	}
	run, err := h.service.queries.GetAgentRun(c.Request().Context(), runID)
	if err != nil {
		return db.AgentRun{}, echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if strings.TrimSpace(env.ThreadID) != "" && env.ThreadID != run.ThreadID {
		return db.AgentRun{}, echo.NewHTTPError(
			http.StatusBadRequest,
			"event thread does not match run",
		)
	}
	if strings.TrimSpace(env.WorkspaceID) != "" &&
		(!run.WorkspaceID.Valid || env.WorkspaceID != run.WorkspaceID.String) {
		return db.AgentRun{}, echo.NewHTTPError(
			http.StatusBadRequest,
			"event workspace does not match run",
		)
	}
	if strings.TrimSpace(env.SessionID) != "" &&
		(!run.SessionID.Valid || env.SessionID != run.SessionID.String) {
		return db.AgentRun{}, echo.NewHTTPError(
			http.StatusBadRequest,
			"event session does not match run",
		)
	}
	return run, nil
}

func (h *Handler) trustedInternalRequest(c echo.Context) bool {
	expected := strings.TrimSpace(h.internalToken)
	if expected != "" {
		actual := strings.TrimSpace(c.Request().Header.Get("X-Vamos-Internal-Token"))
		return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
	}
	if !h.internalAllowLoopback {
		return false
	}
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil {
		host = c.Request().RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseFormInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func parseFormBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseSince(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	since, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || since < 0 {
		return 0
	}
	return since
}

func workspaceDocFormPath(rootPath, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", nil
	}
	if filepath.IsAbs(rawPath) {
		root, err := resolveWorkspacePath(rootPath)
		if err != nil {
			return "", err
		}
		resolved, err := resolveWorkspacePath(rawPath)
		if err != nil {
			return "", err
		}
		if !pathWithinRoot(resolved, root) && !sameFilesystemPath(resolved, root) {
			return "", errors.New("doc path escapes workspace root")
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil {
			return "", err
		}
		if rel == "." {
			return "", nil
		}
		return filepath.ToSlash(rel), nil
	}
	return ValidateWorkspaceRelPath(rootPath, rawPath)
}

func threadThoughtsChatURL(threadID, runID string) string {
	values := url.Values{}
	values.Set("context", ThoughtsChatContext)
	values.Set("thread", strings.TrimSpace(threadID))
	if strings.TrimSpace(runID) != "" {
		values.Set("run", strings.TrimSpace(runID))
	}
	return "/thoughts/?" + values.Encode()
}

func workspaceThreadURL(workspaceID, threadID, runID string) string {
	return threadThoughtsChatURL(threadID, runID)
}

func workspaceThreadURLForRequest(
	c echo.Context,
	workspaceID, threadID, runID string,
) string {
	return threadThoughtsChatURL(threadID, runID)
}

func (h *Handler) writeNoRedirectSuccess(c echo.Context) error {
	sse := datastar.NewSSE(c.Response().Writer, c.Request())
	if err := sse.MarshalAndPatchSignals(
		map[string]any{"agentChatLastWriteOK": true},
	); err != nil {
		return err
	}
	return sse.ExecuteScript(resetAndFocusComposerScript)
}

func (h *Handler) patchWorkspacePage(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	scope StreamPatchScope,
) error {
	return h.patchWorkspace(c, sse, WorkspacePatchScope(scope))
}

func (h *Handler) patchWorkspaceResource(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return nil
	}
	args, err := h.service.BuildWorkspacePageArgs(
		c.Request().Context(),
		h.workspacePatchInput(c, userEmail),
	)
	if err != nil {
		if isRecoverableThreadWorkspaceMismatch(err) {
			h.logRecoverableThreadWorkspaceMismatch(c, err)
			return sse.ExecuteScript("window.location.href = '/';")
		}
		return err
	}
	workbenchState, err := h.buildWorkspaceWorkbenchState(c, *args)
	if err != nil {
		return err
	}
	args.Workbench = workbenchState
	return sse.PatchElementTempl(WorkspaceResource(*args))
}

func (h *Handler) patchWorkspacePlanSidebar(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return nil
	}
	args, err := h.service.BuildWorkspacePageArgs(
		c.Request().Context(),
		h.workspacePatchInput(c, userEmail),
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(
		workbench.SharedSidebar(BuildAgentChatWorkspaceSidebarArgs(*args)),
		datastar.WithSelectorID("agent-chat-shared-sidebar"),
	)
}

func (h *Handler) patchWorkspaceLiveTranscript(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return nil
	}
	state, threadID, err := h.service.BuildWorkspaceLiveTranscriptState(
		c.Request().Context(),
		h.workspacePatchInput(c, userEmail),
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(
		LiveTranscriptRegion(
			threadID,
			state,
			workspaceForkAction(c.Param("workspace_id"), threadID),
		),
	)
}

func (h *Handler) patchEmbeddedFreeformChatPanel(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	userEmail string,
) error {
	args, err := h.service.BuildEmbeddedFreeformPanelArgs(
		c.Request().Context(),
		userEmail,
		firstNonEmpty(c.QueryParam("thread"), c.Param("thread_id")),
		strings.TrimSpace(c.QueryParam("run")),
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(
		EmbeddedFreeformRightRailPanel(args),
		datastar.WithSelectorID("doc-right-chat-panel"),
	)
}

func (h *Handler) patchEmbeddedFreeformLiveTranscript(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	userEmail string,
) error {
	args, err := h.service.BuildEmbeddedFreeformPanelArgs(
		c.Request().Context(),
		userEmail,
		firstNonEmpty(c.QueryParam("thread"), c.Param("thread_id")),
		strings.TrimSpace(c.QueryParam("run")),
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(LiveTranscriptRegion(args.ThreadID, args.Transcript, freeformForkAction(args.ThreadID)))
}

func (h *Handler) patchEmbeddedChatLiveTranscript(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	input EmbeddedChatPatchInput,
) error {
	state, threadID, err := h.service.BuildWorkspaceLiveTranscriptState(
		c.Request().Context(),
		BuildWorkspacePageInput{
			UserEmail:   input.UserEmail,
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			RunID:       input.RunID,
			DocRelPath:  input.DocPath,
			DocPath:     input.DocPath,
		},
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(LiveTranscriptRegion(threadID, state, ""))
}

func (h *Handler) patchWorkspaceWorkflowPanel(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return nil
	}
	args, err := h.service.BuildWorkspacePageArgs(
		c.Request().Context(),
		h.workspacePatchInput(c, userEmail),
	)
	if err != nil {
		return err
	}
	return sse.PatchElementTempl(WorkspaceWorkflowPanel(args.Projection.Workflow))
}

func (h *Handler) patchWorkspace(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	scope WorkspacePatchScope,
) error {
	switch scope {
	case PatchLiveTranscript:
		return h.patchWorkspaceLiveTranscript(c, sse)
	case PatchWorkflowPanel:
		return h.patchWorkspaceWorkflowPanel(c, sse)
	default:
		return h.patchWorkspaceResource(c, sse)
	}
}

func (h *Handler) patchWorkspaceCatchup(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
) error {
	return h.patchWorkspaceResource(c, sse)
}

func (h *Handler) workspacePatchInput(
	c echo.Context,
	userEmail string,
) BuildWorkspacePageInput {
	threadID := strings.TrimSpace(c.QueryParam("thread"))
	if threadID == "" {
		threadID = strings.TrimSpace(c.FormValue("thread_id"))
	}
	documentPath := strings.TrimSpace(c.QueryParam("doc"))
	if documentPath == "" {
		documentPath = strings.TrimSpace(c.FormValue("doc"))
	}
	return BuildWorkspacePageInput{
		UserEmail:   userEmail,
		WorkspaceID: c.Param("workspace_id"),
		ThreadID:    threadID,
		RunID:       c.QueryParam("run"),
		DocRelPath:  documentPath,
		DocPath:     documentPath,
	}
}

func (h *Handler) embeddedPatchInput(
	c echo.Context,
	userEmail string,
) EmbeddedChatPatchInput {
	docPath := firstNonEmpty(
		c.QueryParam("doc"),
		c.FormValue("doc"),
		c.FormValue("doc_path"),
	)
	return EmbeddedChatPatchInput{
		UserEmail:   userEmail,
		DocPath:     markdown.CanonicalThoughtsDocPathLoose(docPath),
		WorkspaceID: strings.TrimSpace(c.Param("workspace_id")),
		ThreadID: firstNonEmpty(
			c.QueryParam("thread"),
			c.Param("thread_id"),
			c.FormValue("thread_id"),
		),
		RunID: firstNonEmpty(
			c.QueryParam("run"),
			c.FormValue("run_id"),
		),
	}
}

func (h *Handler) patchThreadPage(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	scope StreamPatchScope,
) error {
	threadID := firstNonEmpty(c.QueryParam("thread"), c.Param("thread_id"), c.FormValue("thread_id"))
	return h.patchThread(c, sse, threadID, scope)
}

func (h *Handler) patchThread(
	c echo.Context,
	sse *datastar.ServerSentEventGenerator,
	threadID string,
	scope StreamPatchScope,
) error {
	userEmail, ok := c.Get("user_email").(string)
	if !ok || userEmail == "" {
		return nil
	}

	threadID = strings.TrimSpace(threadID)
	if scope == PatchLiveTranscript {
		state, err := h.service.BuildLiveTranscriptState(
			c.Request().Context(),
			userEmail,
			threadID,
		)
		if err != nil {
			return err
		}
		return sse.PatchElementTempl(
			LiveTranscriptRegion(threadID, state, freeformForkAction(threadID)),
		)
	}

	args, err := h.service.BuildPageArgs(
		c.Request().Context(),
		userEmail,
		threadID,
		c.QueryParam("run"),
		c.QueryParam("doc"),
		c.QueryParam("cwd"),
	)
	if err != nil {
		return err
	}

	patchSidebar := func() error {
		return sse.PatchElementTempl(PlanSidebar(args.PlanSidebar))
	}
	patchRunHeader := func() error {
		return sse.PatchElementTempl(RunHeader(ensureFreeformWorkbenchState(*args)))
	}
	patchMessages := func() error {
		threadID := getThreadID(args.CurrentThread)
		return sse.PatchElementTempl(
			MessagesPane(
				threadID,
				args.Transcript,
				args.CurrentThread != nil,
				freeformForkAction(threadID),
			),
		)
	}
	patchLive := func() error {
		threadID := getThreadID(args.CurrentThread)
		return sse.PatchElementTempl(
			LiveTranscriptRegion(
				threadID,
				args.Transcript,
				freeformForkAction(threadID),
			),
		)
	}
	patchDocs := func() error {
		return sse.PatchElementTempl(
			DocPane(args.DocPane, getThreadID(args.CurrentThread)),
		)
	}

	switch scope {
	case PatchSidebar:
		return patchSidebar()
	case PatchRunHeader:
		if err := patchRunHeader(); err != nil {
			return err
		}
		return patchLive()
	case PatchDocPane:
		return patchDocs()
	case PatchStableTranscript:
		return patchMessages()
	case PatchThreadPage:
		if err := patchSidebar(); err != nil {
			return err
		}
		if err := patchRunHeader(); err != nil {
			return err
		}
		if err := patchMessages(); err != nil {
			return err
		}
		return patchDocs()
	default:
		return patchMessages()
	}
}

func (h *Handler) SelectArtifact(c echo.Context) error { return h.SelectDoc(c) }
