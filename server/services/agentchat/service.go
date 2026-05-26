package agentchat

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
	conversationworkflow "github.com/CoreyCole/vamos/pkg/agents/workflows/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	agentworkspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type ThemeProvider interface {
	GetCurrentTheme(c echo.Context) string
	GetCurrentThemeMode(c echo.Context) string
}

type TemporalStarter interface {
	StartWorkflow(
		ctx context.Context,
		workflowID string,
		workflowFunc, input any,
	) (string, error)
}

type workflowCompletionService interface {
	OnRunComplete(ctx context.Context, result conversation.RunResult) error
	AdvanceHumanGate(ctx context.Context, workspaceID, userEmail string) (string, error)
}

type StartWorkflowInput struct {
	UserEmail    string
	Title        string
	RootDocPath  string
	Cwd          string
	WorkflowType WorkspaceWorkflowType
	Policy       json.RawMessage
}

var ErrThreadRunInProgress = errors.New("thread already has an active run")

const transcriptRoleUser = "user"

type Service struct {
	db                          *sql.DB
	queries                     *db.Queries
	notifier                    *Notifier
	temporal                    TemporalStarter
	themeService                ThemeProvider
	renderer                    *markdown.Renderer
	projectRoot                 string
	projectName                 string
	defaultCwd                  string
	thoughtsRoot                string
	piSessionsDir               string
	piIndexMu                   sync.Mutex
	piIndexRunning              map[string]bool
	piIndexQueued               map[string]PiSessionIndexRequest
	callbackBaseURL             string
	detailCollapseLineLimit     int
	liveMu                      sync.RWMutex
	liveThreads                 map[string]*liveThreadState
	liveFlush                   *agentworkspace.LiveFlushLoop
	appendWorkspaceEventForTest func(context.Context, *db.Queries, AppendWorkspaceEventInput) (db.WorkspaceEvent, error)
	workflowService             workflowCompletionService
	devWorkspaceManager         devWorkspaceManager
	implWorkspaceDiscovery      workspaces.ImplWorkspaceDiscoveryConfig
	workspaceManagerURL         string
	workspaceRestartToken       string
	piCommandDiscovery          PiCommandDiscovery
	chatSessions                *chatsession.Service
}

type liveThreadState struct {
	RunID   string
	Reducer *conversation.LiveTurnReducer
}

type ServiceOptions struct {
	ProjectRoot             string
	ProjectName             string
	DefaultCwd              string
	ThoughtsRoot            string
	DetailCollapseLineLimit int
	CallbackBaseURL         string
	ImplWorkspaceDiscovery  workspaces.ImplWorkspaceDiscoveryConfig
	WorkspaceManagerURL     string
	WorkspaceRestartToken   string
}

func NewService(
	database *sql.DB,
	queries *db.Queries,
	notifier *Notifier,
	temporalMgr *temporalmgr.Manager,
	themeService ThemeProvider,
	projectRoot, defaultCwd string,
	detailCollapseLineLimit int,
	callbackBaseURL string,
) (*Service, error) {
	return NewServiceWithOptions(
		database,
		queries,
		notifier,
		temporalMgr,
		themeService,
		ServiceOptions{
			ProjectRoot:             projectRoot,
			DefaultCwd:              defaultCwd,
			ThoughtsRoot:            legacyThoughtsRoot(projectRoot, defaultCwd),
			DetailCollapseLineLimit: detailCollapseLineLimit,
			CallbackBaseURL:         callbackBaseURL,
		},
	)
}

func NewServiceWithOptions(
	database *sql.DB,
	queries *db.Queries,
	notifier *Notifier,
	temporalMgr *temporalmgr.Manager,
	themeService ThemeProvider,
	opts ServiceOptions,
) (*Service, error) {
	renderer, err := markdown.NewRenderer("")
	if err != nil {
		return nil, err
	}
	if opts.DetailCollapseLineLimit <= 0 {
		opts.DetailCollapseLineLimit = 10
	}

	cleanProjectRoot := cleanAbs(opts.ProjectRoot)
	projectName := strings.TrimSpace(opts.ProjectName)
	if projectName == "" {
		projectName = filepath.Base(filepath.Clean(cleanProjectRoot))
	}
	if projectName == "." || projectName == string(filepath.Separator) ||
		projectName == "" {
		projectName = "project"
	}

	thoughtsRoot := cleanAbs(opts.ThoughtsRoot)
	if thoughtsRoot == "" {
		return nil, errors.New("agentchat thoughts root is required")
	}
	defaultCwd := strings.TrimSpace(opts.DefaultCwd)
	if defaultCwd == "" {
		defaultCwd = cleanProjectRoot
	}

	base := strings.TrimRight(strings.TrimSpace(opts.CallbackBaseURL), "/")
	if base == "" {
		base = "http://localhost:4200"
	}

	svc := &Service{
		db:                      database,
		queries:                 queries,
		notifier:                notifier,
		temporal:                temporalMgr,
		themeService:            themeService,
		renderer:                renderer,
		projectRoot:             cleanProjectRoot,
		projectName:             projectName,
		defaultCwd:              defaultCwd,
		thoughtsRoot:            thoughtsRoot,
		piSessionsDir:           defaultPiSessionsDir(),
		piIndexRunning:          make(map[string]bool),
		piIndexQueued:           make(map[string]PiSessionIndexRequest),
		callbackBaseURL:         base,
		detailCollapseLineLimit: opts.DetailCollapseLineLimit,
		liveThreads:             make(map[string]*liveThreadState),
		implWorkspaceDiscovery:  opts.ImplWorkspaceDiscovery,
		workspaceManagerURL:     strings.TrimSpace(opts.WorkspaceManagerURL),
		workspaceRestartToken:   strings.TrimSpace(opts.WorkspaceRestartToken),
		chatSessions:            chatsession.NewService(database, queries),
	}
	return initializeWorkflowRuntime(svc, notifier, queries)
}

func cleanAbs(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	if abs, err := filepath.Abs(clean); err == nil {
		return abs
	}
	return clean
}

func legacyThoughtsRoot(projectRoot, defaultCwd string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = strings.TrimSpace(defaultCwd)
	}
	if root == "" {
		return ""
	}
	return filepath.Join(root, "thoughts")
}

func initializeWorkflowRuntime(
	svc *Service,
	notifier *Notifier,
	queries *db.Queries,
) (*Service, error) {
	registry := wruntime.NewRegistry()
	def, err := qrspi.Definition()
	if err != nil {
		return nil, err
	}
	if err := registry.Register(def); err != nil {
		return nil, err
	}
	svc.workflowService = &agentchatworkflows.Service{
		Definitions: registry,
		Store:       agentchatworkflows.NewDBStore(queries),
		Runner:      svc,
	}
	svc.liveFlush = agentworkspace.NewLiveFlushLoop(
		agentworkspace.LiveFlushPolicy{},
		func(workspaceID string) {
			if notifier != nil {
				notifier.NotifyLiveTranscript(workspaceID)
			}
		},
	)
	return svc, nil
}

func (s *Service) SetImplWorkspaceDiscoveryConfig(
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) {
	s.implWorkspaceDiscovery = cfg
}

func (s *Service) SetWorkspaceRuntimeConfig(managerURL, restartToken string) {
	s.workspaceManagerURL = strings.TrimSpace(managerURL)
	s.workspaceRestartToken = strings.TrimSpace(restartToken)
}

func (s *Service) PlanWorkspaceDiscoveryInput() PlanWorkspaceDiscoveryInput {
	projectRoot := strings.TrimSpace(s.projectRoot)
	if projectRoot == "" {
		projectRoot = strings.TrimSpace(s.defaultCwd)
	}
	if projectRoot == "" {
		projectRoot = strings.TrimSpace(s.thoughtsRoot)
	}
	implCfg := s.implWorkspaceDiscovery
	if strings.TrimSpace(implCfg.MainCheckoutPath) == "" {
		implCfg.MainCheckoutPath = projectRoot
	}
	if strings.TrimSpace(implCfg.ParentDir) == "" &&
		strings.TrimSpace(projectRoot) != "" {
		implCfg.ParentDir = filepath.Dir(projectRoot)
	}
	return PlanWorkspaceDiscoveryInput{
		ProjectName:        s.projectName,
		ProjectInstanceKey: projectInstanceKey(s.projectName, projectRoot),
		ProjectRoot:        s.projectRoot,
		ThoughtsRoot:       s.thoughtsRoot,
		ImplWorkspaces:     implCfg,
	}
}

func (s *Service) PlanWorkspaceDiscoverySyncer() *PlanWorkspaceSyncer {
	return &PlanWorkspaceSyncer{
		Queries:  s.queries,
		Scanner:  PlanWorkspaceScanner{ThoughtsRoot: s.thoughtsRoot},
		Notifier: s,
	}
}

func (s *Service) WorkspaceSyncInput() SyncWorkspacesInput {
	planInput := s.PlanWorkspaceDiscoveryInput()
	return SyncWorkspacesInput{
		ProjectName:        planInput.ProjectName,
		ProjectInstanceKey: planInput.ProjectInstanceKey,
		ProjectRoot:        planInput.ProjectRoot,
		ThoughtsRoot:       planInput.ThoughtsRoot,
		ImplWorkspaces:     planInput.ImplWorkspaces,
		ManagerURL:         s.workspaceManagerURL,
		RestartToken:       s.workspaceRestartToken,
		TrunkBranch:        "main",
	}
}

func (s *Service) WorkspaceSyncer() *WorkspaceSyncer {
	return &WorkspaceSyncer{
		PlanSyncer: s.PlanWorkspaceDiscoverySyncer(),
		ImplSyncer: &workspaces.ImplWorkspaceSyncer{Queries: s.queries},
	}
}

func (s *Service) callbackURL(path string) string {
	base := strings.TrimRight(strings.TrimSpace(s.callbackBaseURL), "/")
	if base == "" {
		base = "http://localhost:4200"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *Service) CreateWorkspace(
	ctx context.Context,
	input WorkspaceCreateInput,
) (db.Workspace, error) {
	id, err := NewWorkspaceID()
	if err != nil {
		return db.Workspace{}, err
	}
	root, err := ValidateWorkspaceRootDocPath(
		input.RootDocPath,
		s.thoughtsRoot,
		input.UserEmail,
	)
	if err != nil {
		return db.Workspace{}, err
	}
	workflowType := input.WorkflowType
	if workflowType == "" {
		workflowType = WorkspaceWorkflowFreeform
	}
	source := input.Source
	if source == "" {
		source = WorkspaceSourceWeb
	}
	return s.queries.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID:                id,
		UserEmail:         input.UserEmail,
		Title:             validateWorkspaceTitle(input.Title),
		RootDocPath:       root,
		Cwd:               nullString(input.Cwd),
		WorkflowType:      string(workflowType),
		WorkflowStateJson: nullString(string(input.WorkflowState)),
		Source:            string(source),
		SelectedThreadID:  sql.NullString{},
		SelectedDocPath:   sql.NullString{},
	})
}

func (s *Service) GetOrCreateWorkspaceForRootDocPath(
	ctx context.Context,
	input markdown.ChatWorkspaceOpenInput,
) (db.Workspace, error) {
	root, err := ValidateWorkspaceRootDocPath(
		input.RootDocPath,
		s.thoughtsRoot,
		input.UserEmail,
	)
	if err != nil {
		return db.Workspace{}, err
	}
	workspace, err := s.queries.FindWorkspaceByRootDocPath(ctx, root)
	if err == nil {
		return workspace, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Workspace{}, err
	}

	workflowType := WorkspaceWorkflowType(strings.TrimSpace(input.WorkflowType))
	if workflowType == "" {
		workflowType = WorkspaceWorkflowFreeform
	}
	source := WorkspaceSource(strings.TrimSpace(input.Source))
	if source == "" {
		source = WorkspaceSourceWeb
	}
	return s.CreateWorkspace(ctx, WorkspaceCreateInput{
		UserEmail:    input.UserEmail,
		Title:        validateWorkspaceTitle(input.Title),
		RootDocPath:  root,
		WorkflowType: workflowType,
		Source:       source,
	})
}

func (s *Service) GetWorkspaceForUser(
	ctx context.Context,
	userEmail, workspaceID string,
) (db.Workspace, error) {
	return s.queries.GetWorkspaceForUser(ctx, db.GetWorkspaceForUserParams{
		ID:        strings.TrimSpace(workspaceID),
		UserEmail: userEmail,
	})
}

func (s *Service) GetWorkspaceForUserOrTrustedImport(
	ctx context.Context,
	userEmail, workspaceID string,
) (db.Workspace, error) {
	_ = strings.TrimSpace(userEmail)
	workspace, err := s.queries.GetWorkspace(ctx, strings.TrimSpace(workspaceID))
	if err == nil {
		return workspace, nil
	}

	trustedWorkspace, trustedErr := s.trustedImportedWorkspace(ctx, workspaceID)
	if trustedErr == nil {
		return trustedWorkspace, nil
	}
	return db.Workspace{}, err
}

func (s *Service) StartWorkflow(
	ctx context.Context,
	input StartWorkflowInput,
) (string, error) {
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter.Definitions == nil {
		return "", errors.New("workflow service is not configured")
	}
	workflowType := input.WorkflowType
	if workflowType == "" {
		workflowType = WorkspaceWorkflowQRSPI
	}
	def, ok := adapter.Definitions.Get(wruntime.WorkflowID(string(workflowType)))
	if !ok {
		return "", fmt.Errorf("workflow definition %q is not registered", workflowType)
	}
	state, err := wruntime.InitialState(def, input.Policy)
	if err != nil {
		return "", err
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	node := def.Nodes[def.Start]
	prompt, err := agentchatworkflows.RenderNodePrompt(ctx, def, node, state)
	if err != nil {
		return "", err
	}
	workspaceRecord, err := s.CreateWorkspace(ctx, WorkspaceCreateInput{
		UserEmail:     input.UserEmail,
		Title:         input.Title,
		RootDocPath:   input.RootDocPath,
		Cwd:           input.Cwd,
		WorkflowType:  workflowType,
		WorkflowState: stateJSON,
		Source:        WorkspaceSourceWeb,
	})
	if err != nil {
		return "", err
	}
	return s.StartNodeRun(ctx, agentchatworkflows.StartNodeRunInput{
		WorkspaceID: workspaceRecord.ID,
		NodeID:      def.Start,
		Prompt:      prompt,
		Attempt:     1,
	})
}

func (s *Service) WorkspaceIDForRun(ctx context.Context, runID string) (string, error) {
	run, err := s.queries.GetAgentRun(ctx, strings.TrimSpace(runID))
	if err != nil {
		return "", err
	}
	if !run.WorkspaceID.Valid || strings.TrimSpace(run.WorkspaceID.String) == "" {
		return "", errors.New("run has no workspace")
	}
	return run.WorkspaceID.String, nil
}

func (s *Service) StartNodeRun(
	ctx context.Context,
	input agentchatworkflows.StartNodeRunInput,
) (string, error) {
	if s.temporal == nil {
		return "", errors.New("temporal not configured")
	}
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		return "", errors.New("workspace id is required")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	workspaceRecord, err := s.queries.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	nodeID := input.NodeID
	if strings.TrimSpace(string(nodeID)) == "" {
		return "", errors.New("workflow node id is required")
	}
	attempt := input.Attempt
	if attempt <= 0 {
		attempt = 1
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	thread, err := s.workflowThread(
		ctx,
		q,
		workspaceRecord,
		strings.TrimSpace(input.ThreadID),
		prompt,
		strings.TrimSpace(input.Cwd),
	)
	if err != nil {
		return "", err
	}
	session, err := s.createWebAgentSession(ctx, q, workspaceRecord, thread)
	if err != nil {
		return "", err
	}
	run, err := s.createWorkflowRunRecord(
		ctx,
		q,
		workspaceRecord,
		session,
		thread,
		prompt,
		nodeID,
		attempt,
	)
	if err != nil {
		return "", err
	}
	chatSession, err := ensureWorkspaceChatSessionTx(
		ctx,
		q,
		workspaceRecord,
		workspaceRecord.UserEmail,
		run.WorkflowID,
		run.WorkflowNodeID.String,
		int(run.WorkflowAttempt),
	)
	if err != nil {
		return "", err
	}
	if err := appendPromptAndRunStartedSessionEventsTx(
		ctx,
		q,
		chatSession,
		workspaceRecord.UserEmail,
		thread,
		run,
		prompt,
	); err != nil {
		return "", err
	}
	if err := s.updateWorkflowRunState(
		ctx,
		q,
		workspaceRecord,
		nodeID,
		attempt,
	); err != nil {
		return "", err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
		ID:               workspaceRecord.ID,
		SelectedThreadID: nullString(thread.ID),
	}); err != nil {
		return "", err
	}
	event, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspaceRecord.ID,
		EventType:   "workflow_node_started",
		ActorType:   "system",
		ThreadID:    thread.ID,
		SessionID:   session.ID,
		RunID:       run.ID,
		EventKey:    "workflow_node_started:" + run.ID,
	})
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}

	s.NotifyWorkspaceForEvent(event)
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return "", err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return "", err
	}
	if startedRun != nil {
		return startedRun.ID, nil
	}
	return run.ID, nil
}

func (s *Service) workflowThread(
	ctx context.Context,
	q *db.Queries,
	workspaceRecord db.Workspace,
	threadID string,
	prompt string,
	effectiveCwd string,
) (db.AgentThread, error) {
	if threadID == "" && workspaceRecord.SelectedThreadID.Valid {
		threadID = strings.TrimSpace(workspaceRecord.SelectedThreadID.String)
	}
	if threadID != "" {
		thread, err := q.GetAgentThreadForWorkspaceUser(ctx, db.GetAgentThreadForWorkspaceUserParams{
			ThreadID:    threadID,
			WorkspaceID: workspaceRecord.ID,
			UserEmail:   workspaceRecord.UserEmail,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return db.AgentThread{}, errors.New("thread is not attached to workflow workspace")
			}
			return db.AgentThread{}, err
		}
		thread.WorkspaceID = nullString(workspaceRecord.ID)
		if effectiveCwd == "" || strings.TrimSpace(thread.Cwd) == effectiveCwd {
			return thread, nil
		}
	}
	cwd := effectiveCwd
	if cwd == "" {
		cwd = s.workspaceThreadCwd(workspaceRecord)
	}
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         workspaceRecord.UserEmail,
		Title:             truncateTitle(prompt),
		Cwd:               cwd,
		LineageID:         uuid.NewString(),
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		return db.AgentThread{}, err
	}
	if err := q.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{
		ID:          thread.ID,
		WorkspaceID: nullString(workspaceRecord.ID),
	}); err != nil {
		return db.AgentThread{}, err
	}
	thread.WorkspaceID = nullString(workspaceRecord.ID)
	return thread, nil
}

func (s *Service) updateWorkflowRunState(
	ctx context.Context,
	q *db.Queries,
	workspaceRecord db.Workspace,
	nodeID wruntime.NodeID,
	attempt int,
) error {
	var state wruntime.State
	if err := json.Unmarshal(
		[]byte(workspaceRecord.WorkflowStateJson.String),
		&state,
	); err != nil {
		return err
	}
	if state.Attempts == nil {
		state.Attempts = map[wruntime.NodeID]int{}
	}
	if state.Nodes == nil {
		state.Nodes = map[wruntime.NodeID]wruntime.NodeState{}
	}
	state.CurrentNodeID = nodeID
	state.Status = wruntime.WorkspaceStatusRunning
	state.PendingNextNodeID = ""
	state.Attempts[nodeID] = attempt
	nodeState := state.Nodes[nodeID]
	nodeState.Status = wruntime.NodeStatusRunning
	nodeState.Attempts = attempt
	state.Nodes[nodeID] = nodeState
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return q.UpdateWorkspaceWorkflowState(ctx, db.UpdateWorkspaceWorkflowStateParams{
		ID:                workspaceRecord.ID,
		WorkflowType:      state.Type,
		WorkflowStateJson: nullString(string(encoded)),
	})
}

func (s *Service) AdvanceWorkflowHumanGate(
	ctx context.Context,
	workspaceID string,
	userEmail string,
) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", errors.New("workspace id is required")
	}
	if _, err := s.GetWorkspaceForUser(ctx, userEmail, workspaceID); err != nil {
		return "", err
	}
	if s.workflowService == nil {
		return "", errors.New("workflow service is not configured")
	}
	runID, err := s.workflowService.AdvanceHumanGate(ctx, workspaceID, userEmail)
	if err != nil {
		return "", err
	}
	if s.notifier != nil {
		s.notifier.NotifyWorkspaceResource(workspaceID)
	}
	return runID, nil
}

func (s *Service) ReconcileUnattachedAgentChatThreads(
	ctx context.Context,
	userEmail string,
) error {
	threads, err := s.queries.ListAgentThreads(
		ctx,
		db.ListAgentThreadsParams{UserEmail: userEmail, Limit: 500},
	)
	if err != nil {
		return err
	}
	for _, thread := range threads {
		if _, err := s.queries.GetPrimaryWorkspaceForThread(ctx, db.GetPrimaryWorkspaceForThreadParams{ThreadID: thread.ID, UserEmail: userEmail}); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// Historical unattached threads can point at arbitrary cwd values from
		// before workspace doc-root constraints existed. Reconcile valid
		// plan-scoped threads opportunistically without breaking freeform chat.
		_, _, _ = s.EnsureThreadWorkspace(ctx, userEmail, thread.ID)
	}
	return nil
}

func (s *Service) EnsureThreadWorkspace(
	ctx context.Context,
	userEmail, threadID string,
) (db.Workspace, db.AgentThread, error) {
	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        strings.TrimSpace(threadID),
		UserEmail: userEmail,
	})
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if workspace, err := s.queries.GetPrimaryWorkspaceForThread(ctx, db.GetPrimaryWorkspaceForThreadParams{ThreadID: thread.ID, UserEmail: userEmail}); err == nil {
		thread.WorkspaceID = sql.NullString{String: workspace.ID, Valid: true}
		return workspace, thread, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.Workspace{}, db.AgentThread{}, err
	}

	root, err := ValidateWorkspaceRootDocPath(thread.Cwd, s.thoughtsRoot, userEmail)
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	workspace, err := q.FindWorkspaceByRootDocPathForUser(
		ctx,
		db.FindWorkspaceByRootDocPathForUserParams{
			UserEmail:   userEmail,
			RootDocPath: root,
		},
	)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return db.Workspace{}, db.AgentThread{}, err
		}
		workspaceID, err := NewWorkspaceID()
		if err != nil {
			return db.Workspace{}, db.AgentThread{}, err
		}
		workspace, err = q.CreateWorkspace(ctx, db.CreateWorkspaceParams{
			ID:                workspaceID,
			UserEmail:         userEmail,
			Title:             validateWorkspaceTitle(thread.Title),
			RootDocPath:       root,
			Cwd:               nullString(thread.Cwd),
			WorkflowType:      string(WorkspaceWorkflowFreeform),
			WorkflowStateJson: sql.NullString{},
			Source:            string(WorkspaceSourceImported),
			SelectedThreadID:  sql.NullString{},
			SelectedDocPath:   sql.NullString{},
		})
		if err != nil {
			return db.Workspace{}, db.AgentThread{}, err
		}
	}

	if err := q.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          thread.ID,
			WorkspaceID: nullString(workspace.ID),
		},
	); err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if err := q.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: nullString(thread.ID),
		},
	); err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if err := s.attachThreadRunsAndSessionsToWorkspace(
		ctx,
		tx,
		thread.ID,
		workspace.ID,
	); err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if _, err := s.AppendWorkspaceEvent(
		ctx,
		q,
		AppendWorkspaceEventInput{
			WorkspaceID: workspace.ID,
			EventType:   "thread_attached",
			ActorEmail:  userEmail,
			ActorType:   "user",
			ThreadID:    thread.ID,
			EventKey:    "thread_attached:" + thread.ID,
		},
	); err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	if err := tx.Commit(); err != nil {
		return db.Workspace{}, db.AgentThread{}, err
	}
	thread.WorkspaceID = sql.NullString{String: workspace.ID, Valid: true}
	workspace.SelectedThreadID = sql.NullString{String: thread.ID, Valid: true}
	return workspace, thread, nil
}

func (s *Service) attachThreadRunsAndSessionsToWorkspace(
	ctx context.Context,
	tx *sql.Tx,
	threadID string,
	workspaceID string,
) error {
	workspaceID = strings.TrimSpace(workspaceID)
	threadID = strings.TrimSpace(threadID)
	if workspaceID == "" || threadID == "" {
		return nil
	}
	q := s.queries.WithTx(tx)
	if err := q.BackfillAgentSessionsWorkspaceForThread(ctx, db.BackfillAgentSessionsWorkspaceForThreadParams{WorkspaceID: nullString(workspaceID), ThreadID: nullString(threadID)}); err != nil {
		return err
	}
	return q.BackfillAgentRunsWorkspaceForThread(ctx, db.BackfillAgentRunsWorkspaceForThreadParams{WorkspaceID: nullString(workspaceID), ThreadID: threadID})
}

func ValidateWorkspaceRootDocPath(
	rootPath, thoughtsRoot, userEmail string,
) (string, error) {
	root, err := resolveWorkspacePath(rootPath)
	if err != nil {
		return "", err
	}
	base, err := resolveWorkspacePath(thoughtsRoot)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(userEmail) != "" {
		userBase := filepath.Join(base, userEmail)
		if resolvedUserBase, err := resolveWorkspacePath(
			userBase,
		); err == nil &&
			pathWithinRoot(root, resolvedUserBase) {
			return root, nil
		}
	}
	if !pathWithinRoot(root, base) {
		return "", fmt.Errorf(
			"workspace doc root %q is outside thoughts root %q",
			root,
			base,
		)
	}
	return root, nil
}

func ValidateWorkspaceRelPath(rootPath, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", errors.New("doc path is required")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.New("doc path must be relative")
	}
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, "../") {
		return "", errors.New("doc path escapes workspace root")
	}
	root, err := resolveWorkspacePath(rootPath)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, filepath.FromSlash(cleanRel))
	resolved, err := resolveWorkspacePath(candidate)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(resolved, root) {
		return "", errors.New("doc path escapes workspace root")
	}
	return cleanRel, nil
}

func resolveWorkspacePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func (s *Service) ValidateAttachedThoughtsPath(path string) (AttachedPath, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return AttachedPath{}, errors.New("attachment path is required")
	}
	if filepath.IsAbs(clean) || strings.Contains(clean, "\x00") {
		return AttachedPath{}, errors.New("invalid attachment path")
	}
	clean = filepath.ToSlash(clean)
	rel, ok := strings.CutPrefix(clean, "thoughts/")
	if !ok || strings.TrimSpace(rel) == "" {
		return AttachedPath{}, errors.New("attachment must be under thoughts")
	}
	thoughtsRoot, err := filepath.Abs(s.thoughtsRoot)
	if err != nil {
		return AttachedPath{}, err
	}
	thoughtsRoot, err = filepath.EvalSymlinks(thoughtsRoot)
	if err != nil {
		return AttachedPath{}, err
	}
	resolved, err := filepath.EvalSymlinks(
		filepath.Join(thoughtsRoot, filepath.FromSlash(rel)),
	)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AttachedPath{}, errors.New("attachment does not exist")
		}
		return AttachedPath{}, err
	}
	if resolved != thoughtsRoot &&
		!strings.HasPrefix(resolved, thoughtsRoot+string(os.PathSeparator)) {
		return AttachedPath{}, errors.New("attachment escapes thoughts root")
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return AttachedPath{}, errors.New("attachment escapes thoughts root")
	}
	return AttachedPath{Path: "thoughts/" + rel, Basename: filepath.Base(rel)}, nil
}

func flattenAttachedPaths(paths [][]AttachedPath) []AttachedPath {
	if len(paths) == 0 {
		return nil
	}
	return paths[0]
}

func (s *Service) appendRunAttachments(
	ctx context.Context,
	q db.Querier,
	runID, threadID string,
	paths []AttachedPath,
) error {
	for i, p := range paths {
		if _, err := q.CreateAgentRunAttachment(ctx, db.CreateAgentRunAttachmentParams{
			ID:       uuid.NewString(),
			RunID:    runID,
			ThreadID: threadID,
			Path:     p.Path,
			Basename: p.Basename,
			Position: int64(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) attachedPathsForRun(
	ctx context.Context,
	runID string,
) ([]AttachedPath, error) {
	rows, err := s.queries.ListAgentRunAttachmentsForRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	paths := make([]AttachedPath, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, AttachedPath{Path: row.Path, Basename: row.Basename})
	}
	return paths, nil
}

func (s *Service) attachmentsByRunForThread(
	ctx context.Context,
	threadID string,
) (map[string][]AttachedPath, error) {
	rows, err := s.queries.ListAgentRunAttachmentsForThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	attachments := make(map[string][]AttachedPath)
	for _, row := range rows {
		attachments[row.RunID] = append(
			attachments[row.RunID],
			AttachedPath{Path: row.Path, Basename: row.Basename},
		)
	}
	return attachments, nil
}

func attachTranscriptAttachments(
	messages []TranscriptMessage,
	attachments []AttachedPath,
) {
	if len(attachments) == 0 {
		return
	}
	for i := range messages {
		if messages[i].Variant == "bubble" && messages[i].Role == transcriptRoleUser {
			messages[i].Attachments = attachments
			return
		}
	}
}

func BuildAttachedPathContext(paths []AttachedPath) string {
	if len(paths) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(
		"The user attached these documents for this prompt only. Read the full paths before answering if relevant:\n",
	)
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p.Path)
	}
	return b.String()
}

func defaultPiSessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
}

func (s *Service) StartWorkspaceThread(
	ctx context.Context,
	workspaceID, userEmail, prompt string,
	attachments ...[]AttachedPath,
) (*db.AgentThread, *db.AgentRun, *db.AgentSession, error) {
	if s.temporal == nil {
		return nil, nil, nil, fmt.Errorf("temporal not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, nil, fmt.Errorf("prompt is required")
	}

	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         userEmail,
		Title:             truncateTitle(prompt),
		Cwd:               s.workspaceThreadCwd(workspace),
		LineageID:         uuid.NewString(),
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := q.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{
		ID:          thread.ID,
		WorkspaceID: nullString(workspace.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	thread.WorkspaceID = nullString(workspace.ID)

	session, err := s.createWebAgentSession(ctx, q, workspace, thread)
	if err != nil {
		return nil, nil, nil, err
	}
	run, err := s.createRunForSession(
		ctx,
		q,
		workspace,
		session,
		thread,
		conversation.RunTriggerSend,
		prompt,
		sql.NullString{},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chatSession, err := ensureWorkspaceChatSessionTx(
		ctx,
		q,
		workspace,
		userEmail,
		run.WorkflowID,
		run.WorkflowNodeID.String,
		int(run.WorkflowAttempt),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := appendPromptAndRunStartedSessionEventsTx(
		ctx,
		q,
		chatSession,
		userEmail,
		thread,
		run,
		prompt,
	); err != nil {
		return nil, nil, nil, err
	}
	if err := s.appendRunAttachments(
		ctx,
		q,
		run.ID,
		thread.ID,
		flattenAttachedPaths(attachments),
	); err != nil {
		return nil, nil, nil, err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
		ID:               workspace.ID,
		SelectedThreadID: nullString(thread.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	event, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   "thread_created",
		ActorEmail:  userEmail,
		ActorType:   "user",
		ThreadID:    thread.ID,
		SessionID:   session.ID,
		RunID:       run.ID,
		EventKey:    "thread_created:" + thread.ID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}

	s.NotifyWorkspaceForEvent(event)
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, &session, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, &session, err
	}
	return &thread, startedRun, &session, nil
}

func (s *Service) ResumeWorkspaceThread(
	ctx context.Context,
	workspaceID, userEmail, threadID, prompt string,
	attachments ...[]AttachedPath,
) (*db.AgentThread, *db.AgentRun, *db.AgentSession, error) {
	if s.temporal == nil {
		return nil, nil, nil, fmt.Errorf("temporal not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, nil, fmt.Errorf("prompt is required")
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, nil, nil, fmt.Errorf("thread_id is required")
	}

	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	thread, err := s.sharedWorkspaceThread(ctx, workspace.ID, threadID)
	if err != nil {
		return nil, nil, nil, err
	}
	session, err := s.createWebAgentSession(ctx, q, workspace, thread)
	if err != nil {
		return nil, nil, nil, err
	}
	run, err := s.createRunForSession(
		ctx,
		q,
		workspace,
		session,
		thread,
		conversation.RunTriggerResume,
		prompt,
		thread.HeadEntryID,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chatSession, err := ensureWorkspaceChatSessionTx(
		ctx,
		q,
		workspace,
		userEmail,
		run.WorkflowID,
		run.WorkflowNodeID.String,
		int(run.WorkflowAttempt),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := appendPromptAndRunStartedSessionEventsTx(
		ctx,
		q,
		chatSession,
		userEmail,
		thread,
		run,
		prompt,
	); err != nil {
		return nil, nil, nil, err
	}
	if err := s.appendRunAttachments(
		ctx,
		q,
		run.ID,
		thread.ID,
		flattenAttachedPaths(attachments),
	); err != nil {
		return nil, nil, nil, err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
		ID:               workspace.ID,
		SelectedThreadID: nullString(thread.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, &session, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, &session, err
	}
	return &thread, startedRun, &session, nil
}

func (s *Service) ForkWorkspaceThread(
	ctx context.Context,
	workspaceID, userEmail, sourceThreadID, sourceEntryID, prompt string,
) (*db.AgentThread, *db.AgentRun, *db.AgentSession, error) {
	if s.temporal == nil {
		return nil, nil, nil, fmt.Errorf("temporal not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, nil, fmt.Errorf("prompt is required")
	}
	sourceThreadID = strings.TrimSpace(sourceThreadID)
	if sourceThreadID == "" {
		return nil, nil, nil, fmt.Errorf("source_thread_id is required")
	}
	sourceEntryID = strings.TrimSpace(sourceEntryID)
	if sourceEntryID == "" {
		return nil, nil, nil, fmt.Errorf("source_entry_id is required")
	}

	workspace, err := s.GetWorkspaceForUserOrTrustedImport(ctx, userEmail, workspaceID)
	if err != nil {
		return nil, nil, nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	sourceThread, err := s.sharedWorkspaceThread(ctx, workspace.ID, sourceThreadID)
	if err != nil {
		return nil, nil, nil, err
	}
	sourceEntry, err := q.GetAgentEntry(ctx, db.GetAgentEntryParams{
		LineageID: sourceThread.LineageID,
		EntryID:   sourceEntryID,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	thread, restoreHead, err := s.createForkThreadRecord(
		ctx,
		q,
		sourceThread,
		sourceEntry,
		prompt,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	session, err := s.createWebAgentSession(ctx, q, workspace, thread)
	if err != nil {
		return nil, nil, nil, err
	}
	run, err := s.createRunForSession(
		ctx,
		q,
		workspace,
		session,
		thread,
		conversation.RunTriggerFork,
		prompt,
		restoreHead,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	parentChatSession, err := ensureWorkspaceChatSessionTx(
		ctx,
		q,
		workspace,
		userEmail,
		run.WorkflowID,
		run.WorkflowNodeID.String,
		int(run.WorkflowAttempt),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	forkedAtSeq, err := chatSessionEventSeqForNode(
		ctx,
		q,
		parentChatSession.ID,
		sourceEntryID,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chatSession, err := chatsession.ForkSessionWithQueries(
		ctx,
		q,
		chatsession.ForkSessionInput{
			WorkspaceID:     workspace.ID,
			ParentID:        parentChatSession.ID,
			ForkedAtSeq:     forkedAtSeq,
			ForkedNodeID:    sourceEntryID,
			ActorEmail:      userEmail,
			Prompt:          prompt,
			WorkflowID:      run.WorkflowID,
			WorkflowNodeID:  run.WorkflowNodeID.String,
			WorkflowAttempt: int(run.WorkflowAttempt),
		},
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := appendPromptAndRunStartedSessionEventsTx(
		ctx,
		q,
		chatSession,
		userEmail,
		thread,
		run,
		prompt,
	); err != nil {
		return nil, nil, nil, err
	}
	if err := q.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{
		ID:               workspace.ID,
		SelectedThreadID: nullString(thread.ID),
	}); err != nil {
		return nil, nil, nil, err
	}
	event, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   "thread_forked",
		ActorEmail:  userEmail,
		ActorType:   "user",
		ThreadID:    thread.ID,
		SessionID:   session.ID,
		RunID:       run.ID,
		EventKey:    "thread_forked:" + thread.ID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, nil, err
	}
	s.NotifyWorkspaceForEvent(event)
	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, &session, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, &session, err
	}
	return &thread, startedRun, &session, nil
}

func (s *Service) createWebAgentSession(
	ctx context.Context,
	q *db.Queries,
	workspace db.Workspace,
	thread db.AgentThread,
) (db.AgentSession, error) {
	return q.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  uuid.NewString(),
		WorkspaceID:         nullString(workspace.ID),
		ThreadID:            nullString(thread.ID),
		UserEmail:           normalizeSessionOwnerEmail(workspace.UserEmail),
		Source:              string(AgentSessionSourceWeb),
		SessionPath:         sql.NullString{},
		SessionID:           sql.NullString{},
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString(thread.Cwd),
		Status:              "pending",
		InferredWorkspaceID: sql.NullString{},
		InferredPlanDir:     sql.NullString{},
		ImportedHeadEntryID: sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	})
}

func (s *Service) workspaceThreadCwd(record db.Workspace) string {
	if cwd := strings.TrimSpace(s.defaultCwd); cwd != "" {
		return cwd
	}
	if root := strings.TrimSpace(s.projectRoot); root != "" {
		return root
	}
	if record.Cwd.Valid {
		if cwd := strings.TrimSpace(record.Cwd.String); cwd != "" {
			return cwd
		}
	}
	return record.RootDocPath
}

func (s *Service) createFreeformWorkspaceRoot(
	userEmail, threadID, title string,
) (string, error) {
	root := strings.TrimSpace(s.thoughtsRoot)
	if root == "" {
		root = filepath.Join(s.resolveCwd(""), "thoughts")
	}
	userSegment := sanitizeWorkspacePathSegment(userEmail)
	if userSegment == "" {
		userSegment = "anonymous"
	}
	dir := filepath.Join(
		root,
		userSegment,
		"freeform",
		sanitizeWorkspacePathSegment(threadID),
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	agentsPath := filepath.Join(dir, "AGENTS.md")
	content := fmt.Sprintf(
		"# Freeform Agent Chat Workspace\n\nThis workspace was created for a freeform Agent Chat thread.\n\n- Thread: `%s`\n- Title: %s\n\nUse this directory for docs produced while answering the chat.\n",
		threadID,
		strings.TrimSpace(title),
	)
	file, err := os.OpenFile(agentsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return dir, nil
		}
		return "", err
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return "", err
	}
	return dir, nil
}

func sanitizeWorkspacePathSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(
		"/", "-",
		string(filepath.Separator), "-",
		"\\", "-",
		":", "-",
	).Replace(value)
	value = strings.Trim(value, ". ")
	return value
}

func (s *Service) shouldCreateFreeformWorkspace(cwd string) bool {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return true
	}
	defaultCwd := strings.TrimSpace(s.defaultCwd)
	if defaultCwd == "" {
		return false
	}
	return sameFilesystemPath(cwd, defaultCwd)
}

func (s *Service) StartThread(
	ctx context.Context,
	userEmail, cwd, prompt string,
	attachments ...[]AttachedPath,
) (*db.AgentThread, *db.AgentRun, error) {
	if s.temporal == nil {
		return nil, nil, fmt.Errorf("temporal not configured")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}

	threadID := uuid.NewString()
	lineageID := uuid.NewString()
	title := truncateTitle(prompt)
	var resolvedCwd string
	if s.shouldCreateFreeformWorkspace(cwd) {
		var err error
		resolvedCwd, err = s.createFreeformWorkspaceRoot(userEmail, threadID, title)
		if err != nil {
			return nil, nil, err
		}
	} else {
		var err error
		resolvedCwd, err = s.resolveRequestedCwd(cwd)
		if err != nil {
			return nil, nil, err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	q := s.queries.WithTx(tx)
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                threadID,
		UserEmail:         userEmail,
		Title:             title,
		Cwd:               resolvedCwd,
		LineageID:         lineageID,
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    sql.NullString{},
		ForkedFromEntryID: sql.NullString{},
	})
	if err != nil {
		return nil, nil, err
	}

	run, err := s.createRun(
		ctx,
		q,
		thread,
		conversation.RunTriggerSend,
		prompt,
		sql.NullString{},
	)
	if err != nil {
		return nil, nil, err
	}
	if err := s.appendRunAttachments(
		ctx,
		q,
		run.ID,
		thread.ID,
		flattenAttachedPaths(attachments),
	); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	if _, attachedThread, err := s.EnsureThreadWorkspace(
		ctx,
		userEmail,
		thread.ID,
	); err == nil {
		thread = attachedThread
		run.WorkspaceID = attachedThread.WorkspaceID
	}

	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, err
	}

	return &thread, startedRun, nil
}

func (s *Service) ResumeThread(
	ctx context.Context,
	userEmail, threadID, prompt string,
	attachments ...[]AttachedPath,
) (*db.AgentThread, *db.AgentRun, error) {
	if s.temporal == nil {
		return nil, nil, fmt.Errorf("temporal not configured")
	}

	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, nil, fmt.Errorf("thread_id is required")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	q := s.queries.WithTx(tx)
	thread, err := q.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        threadID,
		UserEmail: userEmail,
	})
	if err != nil {
		return nil, nil, err
	}

	run, err := s.createRun(
		ctx,
		q,
		thread,
		conversation.RunTriggerResume,
		prompt,
		thread.HeadEntryID,
	)
	if err != nil {
		return nil, nil, err
	}
	if err := s.appendRunAttachments(
		ctx,
		q,
		run.ID,
		thread.ID,
		flattenAttachedPaths(attachments),
	); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	if err != nil {
		return &thread, nil, err
	}

	return &thread, startedRun, nil
}

func (s *Service) UpdateThreadCwd(
	ctx context.Context,
	userEmail, threadID, cwd string,
) (*db.AgentThread, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("thread_id is required")
	}

	resolvedCwd, err := s.resolveRequestedCwd(cwd)
	if err != nil {
		return nil, err
	}

	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        threadID,
		UserEmail: userEmail,
	})
	if err != nil {
		return nil, err
	}

	if sameFilesystemPath(thread.Cwd, resolvedCwd) {
		return &thread, nil
	}

	if err := s.queries.UpdateAgentThreadCwd(ctx, db.UpdateAgentThreadCwdParams{ID: thread.ID, Cwd: resolvedCwd}); err != nil {
		return nil, err
	}

	thread.Cwd = resolvedCwd
	s.notifyThreadScope(ctx, thread.ID, PatchThreadPage)
	return &thread, nil
}

func (s *Service) ForkThread(
	ctx context.Context,
	userEmail, sourceThreadID, sourceEntryID, prompt string,
) (*db.AgentThread, *db.AgentRun, error) {
	if s.temporal == nil {
		return nil, nil, fmt.Errorf("temporal not configured")
	}

	sourceThreadID = strings.TrimSpace(sourceThreadID)
	if sourceThreadID == "" {
		return nil, nil, fmt.Errorf("source_thread_id is required")
	}

	sourceEntryID = strings.TrimSpace(sourceEntryID)
	if sourceEntryID == "" {
		return nil, nil, fmt.Errorf("source_entry_id is required")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	q := s.queries.WithTx(tx)
	sourceThread, err := q.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        sourceThreadID,
		UserEmail: userEmail,
	})
	if err != nil {
		return nil, nil, err
	}

	sourceEntry, err := q.GetAgentEntry(ctx, db.GetAgentEntryParams{
		LineageID: sourceThread.LineageID,
		EntryID:   sourceEntryID,
	})
	if err != nil {
		return nil, nil, err
	}

	thread, run, err := s.createForkThread(ctx, q, sourceThread, sourceEntry, prompt)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	if err := s.seedPendingUserPrompt(thread, run); err != nil {
		return &thread, nil, err
	}
	startedRun, err := s.startRun(ctx, thread, run)
	s.notifyThreadScope(ctx, sourceThread.ID, PatchSidebar)
	if err != nil {
		return &thread, nil, err
	}

	return &thread, startedRun, nil
}

func (s *Service) createRun(
	ctx context.Context,
	q *db.Queries,
	thread db.AgentThread,
	trigger conversation.RunTrigger,
	prompt string,
	restoreHeadEntryID sql.NullString,
) (db.AgentRun, error) {
	return s.createRunRecord(
		ctx,
		q,
		thread.WorkspaceID,
		sql.NullString{},
		thread,
		trigger,
		prompt,
		restoreHeadEntryID,
		thread.Cwd,
	)
}

func (s *Service) createRunForSession(
	ctx context.Context,
	q *db.Queries,
	workspace db.Workspace,
	session db.AgentSession,
	thread db.AgentThread,
	trigger conversation.RunTrigger,
	prompt string,
	restoreHeadEntryID sql.NullString,
) (db.AgentRun, error) {
	return s.createRunRecord(
		ctx,
		q,
		nullString(workspace.ID),
		nullString(session.ID),
		thread,
		trigger,
		prompt,
		restoreHeadEntryID,
		workspace.RootDocPath,
	)
}

func (s *Service) createRunRecord(
	ctx context.Context,
	q *db.Queries,
	workspaceID sql.NullString,
	sessionID sql.NullString,
	thread db.AgentThread,
	trigger conversation.RunTrigger,
	prompt string,
	restoreHeadEntryID sql.NullString,
	docRoot string,
) (db.AgentRun, error) {
	runID := uuid.NewString()
	workflowID := fmt.Sprintf("agent-chat-run-%s", runID)

	run, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		ID:                   runID,
		WorkspaceID:          workspaceID,
		ThreadID:             thread.ID,
		SessionID:            sessionID,
		Trigger:              string(trigger),
		Status:               "running",
		PromptText:           prompt,
		RestoreHeadEntryID:   restoreHeadEntryID,
		ResultHeadEntryID:    sql.NullString{},
		WorkflowID:           workflowID,
		TemporalRunID:        sql.NullString{},
		WorkflowNodeID:       sql.NullString{},
		WorkflowAttempt:      0,
		WorkflowResultStatus: sql.NullString{},
		WorkflowResultJson:   sql.NullString{},
		RootDocPath:          docRoot,
		ErrorMessage:         sql.NullString{},
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			return db.AgentRun{}, ErrThreadRunInProgress
		}
		return db.AgentRun{}, err
	}

	return run, nil
}

func (s *Service) createWorkflowRunRecord(
	ctx context.Context,
	q *db.Queries,
	workspaceRecord db.Workspace,
	session db.AgentSession,
	thread db.AgentThread,
	prompt string,
	nodeID wruntime.NodeID,
	attempt int,
) (db.AgentRun, error) {
	runID := uuid.NewString()
	return q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		ID:                   runID,
		WorkspaceID:          nullString(workspaceRecord.ID),
		ThreadID:             thread.ID,
		SessionID:            nullString(session.ID),
		Trigger:              string(conversation.RunTriggerSend),
		Status:               "running",
		PromptText:           prompt,
		RestoreHeadEntryID:   thread.HeadEntryID,
		ResultHeadEntryID:    sql.NullString{},
		WorkflowID:           "agent-chat-workflow-" + runID,
		TemporalRunID:        sql.NullString{},
		WorkflowNodeID:       nullString(string(nodeID)),
		WorkflowAttempt:      int64(attempt),
		WorkflowResultStatus: sql.NullString{},
		WorkflowResultJson:   sql.NullString{},
		RootDocPath:          workspaceRecord.RootDocPath,
		ErrorMessage:         sql.NullString{},
	})
}

func (s *Service) createForkThreadRecord(
	ctx context.Context,
	q *db.Queries,
	sourceThread db.AgentThread,
	sourceEntry db.AgentEntry,
	prompt string,
) (db.AgentThread, sql.NullString, error) {
	var payload struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Message  struct {
			Role string `json:"role"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(sourceEntry.PayloadJson), &payload); err != nil {
		return db.AgentThread{}, sql.NullString{}, err
	}

	entryID := strings.TrimSpace(payload.ID)
	if entryID == "" {
		entryID = sourceEntry.EntryID
	}

	parentEntryID := strings.TrimSpace(payload.ParentID)
	if parentEntryID == "" && sourceEntry.ParentEntryID.Valid {
		parentEntryID = sourceEntry.ParentEntryID.String
	}

	restoreHeadID := resolveForkRestoreHead(
		sourceEntry.EntryType,
		payload.Message.Role,
		entryID,
		parentEntryID,
	)
	restoreHead := sql.NullString{String: restoreHeadID, Valid: restoreHeadID != ""}

	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         sourceThread.UserEmail,
		Title:             truncateTitle(prompt),
		Cwd:               sourceThread.Cwd,
		LineageID:         sourceThread.LineageID,
		HeadEntryID:       restoreHead,
		ParentThreadID:    sql.NullString{String: sourceThread.ID, Valid: true},
		ForkedFromEntryID: sql.NullString{String: sourceEntry.EntryID, Valid: true},
	})
	if err != nil {
		return db.AgentThread{}, sql.NullString{}, err
	}

	return thread, restoreHead, nil
}

func (s *Service) createForkThread(
	ctx context.Context,
	q *db.Queries,
	sourceThread db.AgentThread,
	sourceEntry db.AgentEntry,
	prompt string,
) (db.AgentThread, db.AgentRun, error) {
	thread, restoreHead, err := s.createForkThreadRecord(
		ctx,
		q,
		sourceThread,
		sourceEntry,
		prompt,
	)
	if err != nil {
		return db.AgentThread{}, db.AgentRun{}, err
	}

	run, err := s.createRun(
		ctx,
		q,
		thread,
		conversation.RunTriggerFork,
		prompt,
		restoreHead,
	)
	if err != nil {
		return db.AgentThread{}, db.AgentRun{}, err
	}

	return thread, run, nil
}

func resolveForkRestoreHead(
	entryType, messageRole, entryID, parentEntryID string,
) string {
	switch entryType {
	case "message":
		if strings.TrimSpace(messageRole) == "user" {
			return parentEntryID
		}
		return entryID
	case "custom_message":
		return parentEntryID
	default:
		return entryID
	}
}

func (s *Service) seedPendingUserPrompt(
	thread db.AgentThread,
	run db.AgentRun,
) error {
	workspaceID := strings.TrimSpace(run.WorkspaceID.String)
	if !run.WorkspaceID.Valid || workspaceID == "" {
		return nil
	}
	prompt := strings.TrimSpace(run.PromptText)
	if prompt == "" {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "user",
			"content": prompt,
		},
	})
	if err != nil {
		return err
	}

	sessionID := ""
	if run.SessionID.Valid {
		sessionID = run.SessionID.String
	}
	if err := s.ApplyLiveEvent(conversation.EventEnvelope{
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_start",
		PayloadJSON: string(payload),
		EventKey:    run.ID + ":pending_user_prompt",
	}); err != nil {
		return err
	}
	s.notifyLiveTranscriptDirty(workspaceID, thread.ID)
	return nil
}

type preparedRunInput struct {
	Input         conversation.RunInput
	ChatSessionID string
}

func (s *Service) buildRunInput(
	ctx context.Context,
	thread db.AgentThread,
	run db.AgentRun,
) (preparedRunInput, error) {
	restoreHead := ""
	if run.RestoreHeadEntryID.Valid {
		restoreHead = run.RestoreHeadEntryID.String
	}
	workspaceID := ""
	if run.WorkspaceID.Valid {
		workspaceID = run.WorkspaceID.String
	}
	sessionID := ""
	if run.SessionID.Valid {
		sessionID = run.SessionID.String
	}
	inputContext := s.runDocumentContext(ctx, run)
	if attachments, err := s.attachedPathsForRun(ctx, run.ID); err == nil {
		attachmentContext := BuildAttachedPathContext(attachments)
		if attachmentContext != "" {
			if inputContext != "" {
				inputContext += "\n\n"
			}
			inputContext += attachmentContext
		}
	}
	chatSessionID := s.chatSessionIDForRun(ctx, run)
	return preparedRunInput{
		Input: conversation.RunInput{
			WorkspaceID:            workspaceID,
			SessionID:              sessionID,
			ChatSessionID:          chatSessionID,
			RunID:                  run.ID,
			ThreadID:               thread.ID,
			Trigger:                conversation.RunTrigger(run.Trigger),
			Prompt:                 run.PromptText,
			Context:                inputContext,
			Cwd:                    thread.Cwd,
			RootDocPath:            run.RootDocPath,
			ThinkingLevel:          "high",
			CallbackEndpoint:       s.callbackURL("/internal/agent-chat/events"),
			SnapshotLoaderEndpoint: s.callbackURL("/internal/agent-chat/snapshots"),
			SnapshotRef: conversation.SnapshotRef{
				LineageID:   thread.LineageID,
				HeadEntryID: restoreHead,
			},
		},
		ChatSessionID: chatSessionID,
	}, nil
}

func (s *Service) startRun(
	ctx context.Context,
	thread db.AgentThread,
	run db.AgentRun,
) (*db.AgentRun, error) {
	if s.temporal == nil {
		return nil, fmt.Errorf("temporal not configured")
	}
	prepared, err := s.buildRunInput(ctx, thread, run)
	if err != nil {
		return nil, err
	}
	input := prepared.Input

	temporalRunID, err := s.temporal.StartWorkflow(
		ctx,
		run.WorkflowID,
		conversationworkflow.RunTurnWorkflow,
		input,
	)
	if err != nil {
		s.failRunRecord(ctx, run.ID, err.Error())
		s.notifyThreadScope(ctx, thread.ID, PatchRunHeader)
		return nil, err
	}

	if err := s.queries.UpdateAgentRunStarted(ctx, db.UpdateAgentRunStartedParams{
		ID:            run.ID,
		TemporalRunID: sql.NullString{String: temporalRunID, Valid: true},
	}); err != nil {
		s.notifyThreadScope(ctx, thread.ID, PatchRunHeader)
		return &run, nil
	}
	if err := s.recordTemporalWorkerSurface(ctx, run, prepared.ChatSessionID); err != nil {
		s.notifyThreadScope(ctx, thread.ID, PatchRunHeader)
		return &run, nil
	}

	latest, err := s.queries.GetAgentRun(ctx, run.ID)
	if err != nil {
		s.notifyThreadScope(ctx, thread.ID, PatchRunHeader)
		return &run, nil
	}

	if input.WorkspaceID != "" {
		event, _ := s.AppendWorkspaceEvent(ctx, s.queries, AppendWorkspaceEventInput{
			WorkspaceID: input.WorkspaceID,
			EventType:   "run_started",
			ActorType:   "system",
			ThreadID:    thread.ID,
			SessionID:   input.SessionID,
			RunID:       run.ID,
			EventKey:    "run_started:" + run.ID,
		})
		s.NotifyWorkspaceForEvent(event)
	}
	// Keep notifying the thread scope for compatibility with legacy thread-scoped
	// SSE streams and the live transcript fast lane.
	s.notifyThreadScope(ctx, thread.ID, PatchRunHeader)
	return &latest, nil
}

func (s *Service) failRunRecord(ctx context.Context, runID, errorMessage string) {
	_ = s.queries.FailAgentRun(ctx, db.FailAgentRunParams{
		ID:           runID,
		ErrorMessage: sql.NullString{String: errorMessage, Valid: errorMessage != ""},
	})
}

func (s *Service) CurrentCursor(workspaceID string) int64 {
	if s.notifier == nil {
		return 0
	}
	return s.notifier.CurrentCursor(workspaceID)
}

func (s *Service) StartLiveFlushLoop(ctx context.Context) {
	if s == nil || s.liveFlush == nil {
		return
	}
	go s.liveFlush.Run(ctx)
}

func (s *Service) notifyLiveTranscriptDirty(workspaceID, threadID string) {
	if s == nil {
		return
	}
	workspaceID = strings.TrimSpace(workspaceID)
	threadID = strings.TrimSpace(threadID)
	if s.liveFlush == nil {
		if s.notifier != nil && workspaceID != "" {
			s.notifier.NotifyLiveTranscript(workspaceID)
		}
		return
	}
	s.liveFlush.MarkDirty(workspaceID, threadID)
}

func (s *Service) notifyThreadScope(
	ctx context.Context,
	threadID string,
	scope WorkspacePatchScope,
) {
	s.notifyThreadScopes(ctx, threadID, scope)
}

func (s *Service) notifyThreadScopes(
	ctx context.Context,
	threadID string,
	scopes ...WorkspacePatchScope,
) {
	if s.notifier == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}

	// Keep notifying the thread scope for compatibility with legacy thread-scoped
	// SSE streams and the live transcript fast lane.
	s.notifier.NotifyScopes(threadID, scopes...)

	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil || !thread.WorkspaceID.Valid ||
		strings.TrimSpace(thread.WorkspaceID.String) == "" {
		return
	}
	workspaceID := strings.TrimSpace(thread.WorkspaceID.String)
	if workspaceID == threadID {
		return
	}
	s.notifier.NotifyScopes(workspaceID, scopes...)
}

func (s *Service) defaultTranscriptRenderPolicy() TranscriptRenderPolicy {
	return TranscriptRenderPolicy{}
}

func (s *Service) ApplyLiveEvent(env conversation.EventEnvelope) error {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()

	state := s.liveThreads[env.ThreadID]
	if state == nil || state.RunID != env.RunID {
		state = &liveThreadState{
			RunID:   env.RunID,
			Reducer: conversation.NewLiveTurnReducer(),
		}
		state.Reducer.Reset(env.RunID)
		s.liveThreads[env.ThreadID] = state
	}

	if state.Reducer == nil {
		state.Reducer = conversation.NewLiveTurnReducer()
		state.Reducer.Reset(env.RunID)
	}

	_, err := state.Reducer.Apply(env)
	return err
}

func (s *Service) resetLiveThread(threadID string) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	delete(s.liveThreads, threadID)
}

func (s *Service) snapshotLiveThread(threadID string) conversation.LiveTurnState {
	s.liveMu.RLock()
	defer s.liveMu.RUnlock()

	state := s.liveThreads[threadID]
	if state == nil || state.Reducer == nil {
		return conversation.LiveTurnState{}
	}

	return state.Reducer.Snapshot()
}

func (s *Service) BuildSnapshot(
	ctx context.Context,
	threadID, headEntryID string,
) (conversation.Snapshot, error) {
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return conversation.Snapshot{}, err
	}

	if strings.TrimSpace(headEntryID) == "" && thread.HeadEntryID.Valid {
		headEntryID = thread.HeadEntryID.String
	}

	snapshot := conversation.Snapshot{
		Header: conversation.SnapshotHeader{
			SessionID:       thread.ID,
			ParentSessionID: thread.ParentThreadID.String,
			Cwd:             thread.Cwd,
		},
		LineageID:   thread.LineageID,
		HeadEntryID: headEntryID,
		Entries:     []conversation.SnapshotEntry{},
	}

	if strings.TrimSpace(headEntryID) == "" {
		return snapshot, nil
	}

	rows, err := s.queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: headEntryID,
	})
	if err != nil {
		return conversation.Snapshot{}, err
	}

	snapshot.Entries = make([]conversation.SnapshotEntry, 0, len(rows))
	for _, row := range rows {
		snapshot.Entries = append(snapshot.Entries, conversation.SnapshotEntry{
			LineageID:     row.LineageID,
			EntryID:       row.EntryID,
			ParentEntryID: row.ParentEntryID.String,
			EntryType:     row.EntryType,
			Timestamp:     row.SessionTimestamp,
			OriginOrder:   row.OriginOrder,
			PayloadJSON:   row.PayloadJson,
		})
	}

	return snapshot, nil
}

func (s *Service) ApplyCheckpoint(ctx context.Context, cp conversation.Checkpoint) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := s.queries.WithTx(tx)
	run, err := q.GetAgentRun(ctx, cp.RunID)
	if err != nil {
		return err
	}
	thread, err := q.GetAgentThread(ctx, run.ThreadID)
	if err != nil {
		return err
	}

	for _, entry := range cp.NewEntries {
		lineageID := thread.LineageID
		if strings.TrimSpace(entry.LineageID) != "" {
			lineageID = entry.LineageID
		}

		err := q.CreateAgentEntry(ctx, db.CreateAgentEntryParams{
			LineageID: lineageID,
			EntryID:   entry.EntryID,
			ParentEntryID: sql.NullString{
				String: entry.ParentEntryID,
				Valid:  entry.ParentEntryID != "",
			},
			EntryType:        entry.EntryType,
			OriginOrder:      entry.OriginOrder,
			PayloadJson:      entry.PayloadJSON,
			OriginThreadID:   thread.ID,
			OriginRunID:      sql.NullString{String: run.ID, Valid: true},
			OriginSessionID:  run.SessionID,
			SessionTimestamp: entry.Timestamp,
		})
		if err != nil {
			if isUniqueConstraintError(err) {
				continue
			}
			return err
		}
	}

	if err := q.UpdateAgentRunCheckpoint(ctx, db.UpdateAgentRunCheckpointParams{
		ID: run.ID,
		ResultHeadEntryID: sql.NullString{
			String: cp.HeadEntryID,
			Valid:  cp.HeadEntryID != "",
		},
	}); err != nil {
		return err
	}

	if err := q.UpdateAgentThreadHead(ctx, db.UpdateAgentThreadHeadParams{
		ID:          thread.ID,
		HeadEntryID: sql.NullString{String: cp.HeadEntryID, Valid: cp.HeadEntryID != ""},
	}); err != nil {
		return err
	}

	workspaceID := ""
	if run.WorkspaceID.Valid {
		workspaceID = run.WorkspaceID.String
	}
	if workspaceID != "" {
		eventKey := strings.TrimSpace(cp.EventKey)
		if eventKey == "" {
			eventKey = fmt.Sprintf("%s:checkpoint:%d", run.ID, cp.TurnIndex)
		}
		_, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
			WorkspaceID: workspaceID,
			EventType:   "run_checkpointed",
			ActorType:   "system",
			ThreadID:    thread.ID,
			SessionID:   run.SessionID.String,
			RunID:       run.ID,
			EventKey:    eventKey,
		})
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(cp.ChatSessionID) == "" {
		cp.ChatSessionID = s.chatSessionIDForRun(ctx, run)
	}
	semanticEvents, err := SemanticEventForCheckpoint(cp)
	if err != nil {
		return err
	}
	for _, semanticEvent := range semanticEvents {
		if _, err := appendChatSessionEventTx(ctx, q, semanticEvent); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.resetLiveThread(thread.ID)
	if workspaceID != "" {
		workspaceRecord, err := s.queries.GetWorkspace(ctx, workspaceID)
		if err == nil {
			if changes, err := s.SyncWorkspaceDocInventory(
				ctx,
				workspaceRecord,
			); err == nil && len(changes) > 0 {
				s.NotifyWorkspaceForEvent(db.WorkspaceEvent{
					WorkspaceID: workspaceID,
					EventType:   "doc_updated",
				})
			}
		}
		s.NotifyWorkspaceForEvent(db.WorkspaceEvent{
			WorkspaceID: workspaceID,
			EventType:   "run_checkpointed",
		})
	}
	s.notifyThreadScopes(
		ctx,
		thread.ID,
		PatchStableTranscript,
		PatchDocPane,
	)
	return nil
}

func (s *Service) FinalizeRun(ctx context.Context, result conversation.RunResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	run, event, err := s.finalizeRunInTx(ctx, q, result)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.resetLiveThread(run.ThreadID)
	if event != nil {
		s.NotifyWorkspaceForEvent(*event)
	}
	s.notifyThreadScope(ctx, run.ThreadID, PatchRunHeader)
	if s.workflowService != nil {
		return s.workflowService.OnRunComplete(ctx, result)
	}
	return nil
}

func (s *Service) finalizeRunInTx(
	ctx context.Context,
	q *db.Queries,
	result conversation.RunResult,
) (db.AgentRun, *db.WorkspaceEvent, error) {
	if err := q.CompleteAgentRun(ctx, db.CompleteAgentRunParams{
		ID: result.RunID,
		ResultHeadEntryID: sql.NullString{
			String: result.HeadEntryID,
			Valid:  result.HeadEntryID != "",
		},
	}); err != nil {
		return db.AgentRun{}, nil, err
	}

	run, err := q.GetAgentRun(ctx, result.RunID)
	if err != nil {
		return db.AgentRun{}, nil, err
	}
	if !run.WorkspaceID.Valid {
		return run, nil, nil
	}

	eventKey := strings.TrimSpace(result.EventKey)
	if eventKey == "" {
		eventKey = run.ID + ":run_complete"
	}
	event, err := s.appendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: run.WorkspaceID.String,
		EventType:   "run_completed",
		ActorType:   "system",
		ThreadID:    run.ThreadID,
		SessionID:   runSessionID(run),
		RunID:       run.ID,
		EventKey:    eventKey,
	})
	if err != nil {
		return db.AgentRun{}, nil, err
	}
	if strings.TrimSpace(result.ChatSessionID) == "" {
		result.ChatSessionID = s.chatSessionIDForRun(ctx, run)
	}
	if strings.TrimSpace(result.ChatSessionID) != "" {
		if _, err := appendChatSessionEventTx(
			ctx,
			q,
			SemanticEventForRunResult(result),
		); err != nil {
			return db.AgentRun{}, nil, err
		}
	}
	return run, &event, nil
}

func (s *Service) FailRun(ctx context.Context, failure conversation.RunFailure) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	run, event, err := s.failRunInTx(ctx, q, failure)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.resetLiveThread(run.ThreadID)
	if event != nil {
		s.NotifyWorkspaceForEvent(*event)
	}
	s.notifyThreadScope(ctx, run.ThreadID, PatchRunHeader)
	return nil
}

func (s *Service) failRunInTx(
	ctx context.Context,
	q *db.Queries,
	failure conversation.RunFailure,
) (db.AgentRun, *db.WorkspaceEvent, error) {
	if err := q.FailAgentRun(
		ctx,
		db.FailAgentRunParams{
			ID: failure.RunID,
			ErrorMessage: sql.NullString{
				String: failure.ErrorMessage,
				Valid:  failure.ErrorMessage != "",
			},
		},
	); err != nil {
		return db.AgentRun{}, nil, err
	}

	run, err := q.GetAgentRun(ctx, failure.RunID)
	if err != nil {
		return db.AgentRun{}, nil, err
	}
	if !run.WorkspaceID.Valid {
		return run, nil, nil
	}

	eventKey := strings.TrimSpace(failure.EventKey)
	if eventKey == "" {
		eventKey = run.ID + ":run_failed"
	}
	event, err := s.appendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: run.WorkspaceID.String,
		EventType:   "run_failed",
		ActorType:   "system",
		ThreadID:    run.ThreadID,
		SessionID:   runSessionID(run),
		RunID:       run.ID,
		EventKey:    eventKey,
	})
	if err != nil {
		return db.AgentRun{}, nil, err
	}
	if strings.TrimSpace(failure.ChatSessionID) == "" {
		failure.ChatSessionID = s.chatSessionIDForRun(ctx, run)
	}
	if strings.TrimSpace(failure.ChatSessionID) != "" {
		if _, err := appendChatSessionEventTx(
			ctx,
			q,
			SemanticEventForRunFailure(failure),
		); err != nil {
			return db.AgentRun{}, nil, err
		}
	}
	return run, &event, nil
}

func (s *Service) FailConversationRunAfterActivityError(
	ctx context.Context,
	input conversation.ActivityFailureInput,
) error {
	failure := conversation.RunFailure{
		WorkspaceID:   input.WorkspaceID,
		SessionID:     input.SessionID,
		ChatSessionID: input.ChatSessionID,
		RunID:         input.RunID,
		ThreadID:      input.ThreadID,
		RootDocPath:   input.RootDocPath,
		ErrorMessage:  input.ErrorMessage,
		EventKey:      input.EventKey,
	}
	return s.FailRunIfRunning(ctx, failure)
}

func (s *Service) FailRunIfRunning(
	ctx context.Context,
	failure conversation.RunFailure,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	run, event, err := s.failRunIfRunningInTx(ctx, q, failure)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.resetLiveThread(run.ThreadID)
	if event != nil {
		s.NotifyWorkspaceForEvent(*event)
	}
	s.notifyThreadScope(ctx, run.ThreadID, PatchRunHeader)
	return nil
}

func (s *Service) failRunIfRunningInTx(
	ctx context.Context,
	q *db.Queries,
	failure conversation.RunFailure,
) (db.AgentRun, *db.WorkspaceEvent, error) {
	errorMessage := sql.NullString{
		String: failure.ErrorMessage,
		Valid:  failure.ErrorMessage != "",
	}
	run, err := q.FailAgentRunIfRunning(ctx, db.FailAgentRunIfRunningParams{
		ID:           failure.RunID,
		ErrorMessage: errorMessage,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			existing, getErr := q.GetAgentRun(ctx, failure.RunID)
			if getErr != nil {
				return db.AgentRun{}, nil, getErr
			}
			return existing, nil, nil
		}
		return db.AgentRun{}, nil, err
	}
	if !run.WorkspaceID.Valid {
		return run, nil, nil
	}

	eventKey := strings.TrimSpace(failure.EventKey)
	if eventKey == "" {
		eventKey = run.ID + ":run_failed"
	}
	event, err := s.appendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: run.WorkspaceID.String,
		EventType:   "run_failed",
		ActorType:   "system",
		ThreadID:    run.ThreadID,
		SessionID:   runSessionID(run),
		RunID:       run.ID,
		EventKey:    eventKey,
	})
	if err != nil {
		return db.AgentRun{}, nil, err
	}
	if strings.TrimSpace(failure.ChatSessionID) == "" {
		failure.ChatSessionID = s.chatSessionIDForRun(ctx, run)
	}
	if strings.TrimSpace(failure.ChatSessionID) != "" {
		if _, err := appendChatSessionEventTx(
			ctx,
			q,
			SemanticEventForRunFailure(failure),
		); err != nil {
			return db.AgentRun{}, nil, err
		}
	}
	return run, &event, nil
}

const agentChatSidebarThreadLimit = 50

func runSessionID(run db.AgentRun) string {
	if !run.SessionID.Valid {
		return ""
	}
	return run.SessionID.String
}

func (s *Service) appendWorkspaceEvent(
	ctx context.Context,
	q *db.Queries,
	input AppendWorkspaceEventInput,
) (db.WorkspaceEvent, error) {
	if s.appendWorkspaceEventForTest != nil {
		return s.appendWorkspaceEventForTest(ctx, q, input)
	}
	return s.AppendWorkspaceEvent(ctx, q, input)
}

func (s *Service) BuildLiveTranscriptState(
	ctx context.Context,
	userEmail, threadID string,
) (TranscriptPaneState, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return TranscriptPaneState{
			Stable: []TranscriptMessage{},
			Live:   LiveTranscriptView{Items: []TranscriptMessage{}},
			Policy: s.defaultTranscriptRenderPolicy(),
		}, nil
	}

	thread, err := s.queries.GetAgentThreadForUser(
		ctx,
		db.GetAgentThreadForUserParams{ID: threadID, UserEmail: userEmail},
	)
	if err != nil {
		return TranscriptPaneState{}, err
	}

	live, cursor := s.buildLiveTranscript(thread.ID)
	return TranscriptPaneState{
		Cursor: cursor,
		Stable: []TranscriptMessage{},
		Live:   live,
		Policy: s.defaultTranscriptRenderPolicy(),
	}, nil
}

func (s *Service) BuildPageArgs(
	ctx context.Context,
	userEmail, threadID, runID, selectedDoc, selectedCwd string,
) (*ChatPageArgs, error) {
	threads, err := s.queries.ListAgentThreads(
		ctx,
		db.ListAgentThreadsParams{
			UserEmail: userEmail,
			Limit:     agentChatSidebarThreadLimit,
		},
	)
	if err != nil {
		return nil, err
	}

	sessions := s.buildAgentChatSessionSidebarRefs(threads)

	cwd := s.resolveCwd(selectedCwd)
	activePlanDir := ""
	if activePlan, ok := s.canonicalPlanDirFromSource(cwd); ok {
		activePlanDir = activePlan
	}

	planSidebar, err := s.BuildPlanSidebarState(ctx, PlanSidebarInput{
		UserEmail:       userEmail,
		ActiveThreadID:  threadID,
		ActivePlanDir:   activePlanDir,
		IncludeArchived: false,
	})
	if err != nil {
		return nil, err
	}
	planSidebar.TargetID = "agent-chat-thread-sidebar"

	args := &ChatPageArgs{
		Sessions:     sessions,
		ThreadGroups: s.buildThreadSidebarGroups(sessions, strings.TrimSpace(threadID)),
		PlanSidebar:  planSidebar,
		Cwd:          cwd,
		Transcript: TranscriptPaneState{
			Stable: []TranscriptMessage{},
			Live:   LiveTranscriptView{Items: []TranscriptMessage{}},
			Policy: s.defaultTranscriptRenderPolicy(),
		},
		DocPane: DocPaneState{},
	}

	if strings.TrimSpace(threadID) == "" {
		args.DocPane, err = s.BuildFreeformDocsPane(ctx, userEmail, selectedDoc)
		if err != nil {
			return nil, err
		}
		return args, nil
	}

	thread, err := s.queries.GetAgentThreadForUser(
		ctx,
		db.GetAgentThreadForUserParams{ID: threadID, UserEmail: userEmail},
	)
	if err != nil {
		return nil, err
	}
	args.CurrentThread = &thread
	if activePlan, ok := s.canonicalPlanDirFromSource(thread.Cwd); ok {
		args.PlanSidebar, err = s.BuildPlanSidebarState(ctx, PlanSidebarInput{
			UserEmail:      userEmail,
			ActiveThreadID: thread.ID,
			ActivePlanDir:  activePlan,
		})
		if err != nil {
			return nil, err
		}
		args.PlanSidebar.TargetID = "agent-chat-thread-sidebar"
	}
	args.Transcript.Policy = s.defaultTranscriptRenderPolicy()
	args.Transcript.Stable, err = s.buildStableTranscript(ctx, thread)
	if err != nil {
		return nil, err
	}
	args.Transcript.Live, args.Cursor = s.buildLiveTranscript(thread.ID)
	args.Transcript.Cursor = args.Cursor

	if strings.TrimSpace(runID) != "" {
		run, err := s.queries.GetAgentRun(ctx, runID)
		if err == nil && run.ThreadID == thread.ID {
			args.ActiveRun = &run
		}
	}
	if args.ActiveRun == nil {
		if run, err := s.queries.GetLatestAgentRunByThread(ctx, thread.ID); err == nil {
			args.ActiveRun = &run
		}
	}

	activeRunID := ""
	if args.ActiveRun != nil {
		activeRunID = args.ActiveRun.ID
	}
	args.DocPane, err = s.BuildDocPane(
		ctx,
		thread.ID,
		activeRunID,
		selectedDoc,
	)
	if err != nil {
		return nil, err
	}

	return args, nil
}

func (s *Service) BuildFreeformDocsPane(
	ctx context.Context,
	userEmail, selectedPath string,
) (DocPaneState, error) {
	_ = ctx
	root := s.freeformDocsRoot(userEmail)
	if root == "" {
		return DocPaneState{}, nil
	}
	return s.buildDocPaneForRoot(root, "", selectedPath, false)
}

func (s *Service) freeformDocsRoot(userEmail string) string {
	if root := userPlansDirectory(s.thoughtsRoot, userEmail); root != "" {
		return root
	}
	if defaultCwd := strings.TrimSpace(s.defaultCwd); defaultCwd != "" {
		return defaultCwd
	}
	return strings.TrimSpace(s.projectRoot)
}

func userPlansDirectory(thoughtsRoot, userEmail string) string {
	thoughtsRoot = strings.TrimSpace(thoughtsRoot)
	if thoughtsRoot == "" {
		return ""
	}
	for _, owner := range userDirectoryCandidates(thoughtsRoot, userEmail) {
		candidate := filepath.Join(thoughtsRoot, owner, "plans")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func userDirectoryCandidates(thoughtsRoot, userEmail string) []string {
	trimmed := strings.TrimSpace(userEmail)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	local := ""
	if value, _, ok := strings.Cut(trimmed, "@"); ok {
		local = strings.TrimSpace(value)
		if local != "" {
			candidates = append(candidates, local)
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
	}

	for _, owner := range matchingThoughtsPlanOwners(thoughtsRoot, trimmed, local) {
		if !seen[owner] {
			seen[owner] = true
			out = append(out, owner)
		}
	}
	return out
}

func matchingThoughtsPlanOwners(thoughtsRoot, email, local string) []string {
	entries, err := os.ReadDir(thoughtsRoot)
	if err != nil {
		return nil
	}
	needles := []string{normalizeOwnerCandidate(email), normalizeOwnerCandidate(local)}
	matches := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		owner := entry.Name()
		if info, err := os.Stat(
			filepath.Join(thoughtsRoot, owner, "plans"),
		); err != nil ||
			!info.IsDir() {
			continue
		}
		ownerKey := normalizeOwnerCandidate(owner)
		if ownerKey == "" {
			continue
		}
		for _, needle := range needles {
			if needle == "" {
				continue
			}
			if ownerKey == needle || strings.HasSuffix(needle, ownerKey) ||
				strings.HasSuffix(ownerKey, needle) {
				matches = append(matches, owner)
				break
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func normalizeOwnerCandidate(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Service) BuildDocPane(
	ctx context.Context,
	threadID, runID, selectedPath string,
) (DocPaneState, error) {
	threadID = strings.TrimSpace(threadID)
	runID = strings.TrimSpace(runID)

	if threadID == "" {
		return DocPaneState{}, nil
	}

	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocPaneState{}, nil
		}
		return DocPaneState{}, err
	}

	activeRunID := ""
	if runID != "" {
		run, err := s.queries.GetAgentRun(ctx, runID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return DocPaneState{}, err
			}
		} else if run.ThreadID == thread.ID {
			activeRunID = run.ID
		}
	}
	if activeRunID == "" {
		if run, err := s.queries.GetLatestAgentRunByThread(ctx, thread.ID); err == nil {
			activeRunID = run.ID
		}
	}

	baseRoot := strings.TrimSpace(thread.Cwd)
	if baseRoot == "" {
		return DocPaneState{ActiveRunID: activeRunID}, nil
	}

	return s.buildDocPaneForRoot(baseRoot, activeRunID, selectedPath, true)
}

func (s *Service) buildDocPaneForRoot(
	baseRoot, activeRunID, selectedPath string,
	selectDefault bool,
) (DocPaneState, error) {
	baseFiles, err := listRenderableDocs(baseRoot)
	if err != nil {
		return DocPaneState{}, err
	}

	selectedAbs := resolveSelectedDocAbsolutePath(baseRoot, baseFiles, selectedPath)
	if selectedAbs == "" && strings.TrimSpace(selectedPath) != "" {
		focusedRoot := focusedRootDocPath(baseRoot, selectedPath)
		if focusedRoot != "" && !sameFilesystemPath(focusedRoot, baseRoot) {
			focusedFiles, err := listRenderableDocs(focusedRoot)
			if err != nil {
				return DocPaneState{}, err
			}
			selectedAbs = resolveSelectedDocAbsolutePath(
				focusedRoot,
				focusedFiles,
				selectedPath,
			)
		}
	}
	if selectedAbs == "" && selectDefault {
		selectedAbs = defaultDocAbsolutePath(baseRoot, baseFiles)
	}

	root := focusedRootDocPath(baseRoot, selectedAbs)
	files := baseFiles
	if !sameFilesystemPath(root, baseRoot) {
		files, err = listRenderableDocs(root)
		if err != nil {
			return DocPaneState{}, err
		}
	}

	selectedRel := relativeDocPath(root, selectedAbs)
	if selectedRel == "" && selectDefault {
		selectedRel = defaultDocPath(files)
	}

	state := DocPaneState{
		ActiveRunID: activeRunID,
		RootDocPath: root,
		RootLabel:   docRootLabel(root),
		WorkingDir: s.buildWorkingDirectoryState(
			baseRoot,
			docResetPath(baseRoot, root),
		),
	}
	state.Tree = buildDocTree(files, selectedRel, root)
	if selectedRel == "" {
		return state, nil
	}
	selected, err := s.renderDoc(root, selectedRel)
	if err != nil {
		return DocPaneState{}, err
	}
	state.Selected = selected
	return state, nil
}

type piSessionHeader struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

type piSessionInfoEntry struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type piSessionMessagePayload struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type piSessionMessageEntry struct {
	Type    string                  `json:"type"`
	Message piSessionMessagePayload `json:"message"`
}

type piSessionCustomEntry struct {
	Type       string          `json:"type"`
	CustomType string          `json:"customType"`
	Data       json.RawMessage `json:"data"`
}

type piPlanClassification struct {
	PlanDir string `json:"planDir"`
	Source  string `json:"source"`
}

type threadSidebarGroupMeta struct {
	Key       string
	KindLabel string
	Label     string
	Timestamp string
	RootPath  string
}

func (s *Service) buildAgentChatSessionSidebarRefs(
	threads []db.AgentThread,
) []SessionThreadRef {
	refs := make([]SessionThreadRef, 0, len(threads))
	for _, thread := range threads {
		refs = append(refs, SessionThreadRef{
			Key:       "agentchat:" + thread.ID,
			Source:    SessionThreadSourceAgentChat,
			ThreadID:  thread.ID,
			PlanDir:   s.normalizeSessionPlanDir(thread.Cwd),
			Title:     thread.Title,
			Cwd:       thread.Cwd,
			UpdatedAt: thread.UpdatedAt,
		})
	}
	return refs
}

func (s *Service) buildSessionSidebarRefs(
	threads []db.AgentThread,
) ([]SessionThreadRef, error) {
	refs := s.buildAgentChatSessionSidebarRefs(threads)

	piRefs, err := s.listPlanScopedPiSessions()
	if err != nil {
		return nil, err
	}
	refs = append(refs, piRefs...)

	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].UpdatedAt.Equal(refs[j].UpdatedAt) {
			return refs[i].Key > refs[j].Key
		}
		return refs[i].UpdatedAt.After(refs[j].UpdatedAt)
	})

	return refs, nil
}

func (s *Service) BuildThreadSidebarArgs(
	ctx context.Context,
	userEmail string,
	threadID string,
) (ThreadSidebarArgs, error) {
	state, err := s.BuildPlanSidebarState(ctx, PlanSidebarInput{
		UserEmail:      userEmail,
		ActiveThreadID: strings.TrimSpace(threadID),
	})
	if err != nil {
		return ThreadSidebarArgs{}, err
	}
	return threadSidebarArgsFromPlanSidebarState(state), nil
}

func threadSidebarArgsFromPlanSidebarState(state PlanSidebarState) ThreadSidebarArgs {
	groups := make([]ThreadSidebarGroup, 0, len(state.Nodes))
	appendPlanSidebarGroups(&groups, state.Nodes)
	return ThreadSidebarArgs{
		Groups:            groups,
		HasSelectedThread: state.HasSelection,
	}
}

func appendPlanSidebarGroups(groups *[]ThreadSidebarGroup, nodes []PlanSidebarNode) {
	for _, node := range nodes {
		group := ThreadSidebarGroup{
			Key:           node.Key,
			KindLabel:     "Plan",
			Label:         node.Label,
			Timestamp:     formatWorkspaceEventTime(planSidebarTimestamp(node)),
			ThreadCount:   node.AggregateCount,
			IsActive:      node.Active,
			WorkspaceHref: node.Href,
		}
		if node.LatestThreadID != "" {
			group.Threads = append(group.Threads, ThreadSidebarThread{
				ID:          node.LatestThreadID,
				Href:        node.Href,
				Title:       node.Label,
				SourceLabel: node.LatestSourceLabel,
				IsActive:    node.Active,
			})
		}
		*groups = append(*groups, group)
		appendPlanSidebarGroups(groups, node.Children)
	}
}

func (s *Service) listPlanScopedPiSessions() ([]SessionThreadRef, error) {
	root := strings.TrimSpace(s.piSessionsDir)
	if root == "" {
		return []SessionThreadRef{}, nil
	}

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SessionThreadRef{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []SessionThreadRef{}, nil
	}

	refs := make([]SessionThreadRef, 0)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		ref, ok := s.readPiSessionRef(path, info)
		if ok {
			refs = append(refs, ref)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}

func (s *Service) readPiSessionRef(
	path string,
	info fs.FileInfo,
) (SessionThreadRef, bool) {
	file, err := os.Open(path)
	if err != nil {
		return SessionThreadRef{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	if !scanner.Scan() {
		return SessionThreadRef{}, false
	}

	var header piSessionHeader
	if err := json.Unmarshal(
		scanner.Bytes(),
		&header,
	); err != nil ||
		header.Type != "session" {
		return SessionThreadRef{}, false
	}

	title := ""
	planDir := ""
	for scanner.Scan() {
		line := scanner.Bytes()
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}

		switch probe.Type {
		case "custom":
			var entry piSessionCustomEntry
			if err := json.Unmarshal(
				line,
				&entry,
			); err != nil ||
				entry.CustomType != "plan-classification" {
				continue
			}
			var classification piPlanClassification
			if err := json.Unmarshal(entry.Data, &classification); err != nil {
				continue
			}
			if normalized := s.normalizeSessionPlanDir(
				classification.PlanDir,
			); normalized != "" {
				planDir = normalized
			}
		case "session_info":
			var entry piSessionInfoEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if name := strings.TrimSpace(entry.Name); name != "" {
				title = name
			}
		case "message":
			if title != "" {
				continue
			}
			var entry piSessionMessageEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if strings.TrimSpace(entry.Message.Role) != "user" {
				continue
			}
			if promptTitle := truncateTitle(
				extractContentText(entry.Message.Content),
			); promptTitle != "New Chat" {
				title = promptTitle
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return SessionThreadRef{}, false
	}

	if planDir == "" {
		planDir = s.normalizeSessionPlanDir(header.Cwd)
	}
	if planDir == "" {
		return SessionThreadRef{}, false
	}
	if s.projectRoot != "" && !pathWithinRoot(planDir, s.projectRoot) {
		return SessionThreadRef{}, false
	}
	if title == "" {
		title = s.defaultPiSessionTitle(path, header.Cwd)
	}

	return SessionThreadRef{
		Key:         "pi:" + path,
		Source:      SessionThreadSourcePi,
		SessionPath: path,
		PlanDir:     planDir,
		Title:       title,
		Cwd:         header.Cwd,
		UpdatedAt:   info.ModTime(),
	}, true
}

func (s *Service) defaultPiSessionTitle(path, cwd string) string {
	label := filepath.Base(filepath.Clean(strings.TrimSpace(cwd)))
	if label != "" && label != "." && label != string(filepath.Separator) {
		return truncateTitle(label)
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.TrimSpace(base) != "" {
		return base
	}
	return "Pi session"
}

func (s *Service) normalizeSessionPlanDir(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	if root := planDirectoryRoot(clean); root != "" {
		clean = root
	}
	if !filepath.IsAbs(clean) && s.projectRoot != "" {
		clean = filepath.Join(s.projectRoot, clean)
	}
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	return planDirectoryRoot(clean)
}

func (s *Service) buildThreadSidebarGroups(
	refs []SessionThreadRef,
	currentThreadID string,
) []ThreadSidebarGroup {
	groups := make([]ThreadSidebarGroup, 0)
	groupIndexes := make(map[string]int)
	currentThreadID = strings.TrimSpace(currentThreadID)

	for _, ref := range refs {
		meta := s.sessionSidebarGroupMeta(ref)
		idx, ok := groupIndexes[meta.Key]
		if !ok {
			groups = append(groups, ThreadSidebarGroup{
				Key:       meta.Key,
				KindLabel: meta.KindLabel,
				Label:     meta.Label,
				Timestamp: meta.Timestamp,
				Threads:   []ThreadSidebarThread{},
			})
			idx = len(groups) - 1
			groupIndexes[meta.Key] = idx
		}

		isActive := ref.Source == SessionThreadSourceAgentChat &&
			strings.TrimSpace(ref.ThreadID) != "" &&
			ref.ThreadID == currentThreadID
		if isActive {
			groups[idx].IsActive = true
		}

		thread := ThreadSidebarThread{
			ID:          ref.ThreadID,
			Href:        s.sessionSidebarHref(ref),
			Title:       ref.Title,
			CwdLabel:    s.threadSidebarCwdLabel(ref.Cwd, meta.RootPath),
			SourceLabel: s.sessionSidebarSourceLabel(ref),
			IsActive:    isActive,
		}
		if action := s.sessionSidebarOpenPiSessionAction(ref); action != "" {
			thread.OpenPiSessionAction = action
			thread.SessionPath = strings.TrimSpace(ref.SessionPath)
			thread.WorkspaceDir = strings.TrimSpace(ref.PlanDir)
		}
		groups[idx].Threads = append(groups[idx].Threads, thread)
		groups[idx].ThreadCount = len(groups[idx].Threads)
	}

	return groups
}

func (s *Service) sessionSidebarGroupMeta(ref SessionThreadRef) threadSidebarGroupMeta {
	if planRoot := strings.TrimSpace(ref.PlanDir); planRoot != "" {
		label, timestamp := formatSidebarGroupDisplay(filepath.Base(planRoot))
		return threadSidebarGroupMeta{
			Key:       "plan:" + planRoot,
			KindLabel: "Plan",
			Label:     label,
			Timestamp: timestamp,
			RootPath:  planRoot,
		}
	}
	return s.threadSidebarGroupMeta(ref.Cwd)
}

func (s *Service) sessionSidebarHref(ref SessionThreadRef) string {
	if ref.Source != SessionThreadSourceAgentChat ||
		strings.TrimSpace(ref.ThreadID) == "" {
		return ""
	}
	href := s.thoughtsHrefForPlanDir(ref.PlanDir)
	if href == "" {
		return ""
	}
	values := url.Values{"thread": []string{strings.TrimSpace(ref.ThreadID)}}
	separator := "?"
	if strings.Contains(href, "?") {
		separator = "&"
	}
	return href + separator + values.Encode()
}

func (s *Service) sessionSidebarOpenPiSessionAction(ref SessionThreadRef) string {
	if ref.Source != SessionThreadSourcePi ||
		strings.TrimSpace(ref.SessionPath) == "" ||
		strings.TrimSpace(ref.PlanDir) == "" {
		return ""
	}
	return piSessionOpenAction()
}

func (s *Service) sessionSidebarSourceLabel(ref SessionThreadRef) string {
	if ref.Source == SessionThreadSourcePi {
		return "Pi"
	}
	return ""
}

func (s *Service) threadSidebarGroupMeta(cwd string) threadSidebarGroupMeta {
	clean := strings.TrimSpace(cwd)
	if clean != "" {
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
	}

	if planRoot := planDirectoryRoot(clean); planRoot != "" {
		label, timestamp := formatSidebarGroupDisplay(filepath.Base(planRoot))
		return threadSidebarGroupMeta{
			Key:       "plan:" + planRoot,
			KindLabel: "Plan",
			Label:     label,
			Timestamp: timestamp,
			RootPath:  planRoot,
		}
	}

	if clean != "" && s.projectRoot != "" && pathWithinRoot(clean, s.projectRoot) {
		label := strings.TrimSpace(s.projectName)
		if label == "" {
			label = filepath.Base(filepath.Clean(s.projectRoot))
		}
		if label == "." || label == string(filepath.Separator) || label == "" {
			label = "Workspace"
		}
		return threadSidebarGroupMeta{
			Key:       "workspace:" + s.projectRoot,
			KindLabel: "Workspace",
			Label:     label,
			RootPath:  s.projectRoot,
		}
	}

	if clean == "" {
		return threadSidebarGroupMeta{
			Key:       "workspace:",
			KindLabel: "Workspace",
			Label:     "Workspace",
		}
	}

	label := filepath.Base(filepath.Clean(clean))
	if label == "." || label == string(filepath.Separator) || label == "" {
		label = clean
	}
	return threadSidebarGroupMeta{
		Key:       "directory:" + clean,
		KindLabel: "Directory",
		Label:     label,
		RootPath:  clean,
	}
}

func (s *Service) threadSidebarCwdLabel(cwd, groupRoot string) string {
	clean := strings.TrimSpace(cwd)
	if clean == "" {
		return ""
	}
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}

	trimRelative := func(root string) string {
		if root == "" {
			return ""
		}
		if !pathWithinRoot(clean, root) {
			return ""
		}
		rel, err := filepath.Rel(root, clean)
		if err != nil || rel == "." {
			return ""
		}
		return filepath.ToSlash(rel)
	}

	if groupRoot != "" && sameFilesystemPath(clean, groupRoot) {
		return ""
	}
	if rel := trimRelative(groupRoot); rel != "" {
		return rel
	}
	if s.projectRoot != "" && sameFilesystemPath(clean, s.projectRoot) {
		return ""
	}
	if rel := trimRelative(s.projectRoot); rel != "" {
		return rel
	}

	label := filepath.Base(filepath.Clean(clean))
	if label == "." || label == string(filepath.Separator) || label == "" {
		return clean
	}
	return label
}

func planDirectoryRoot(path string) string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." {
		return ""
	}

	parts := strings.Split(clean, "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "thoughts" || parts[i+2] != "plans" {
			continue
		}
		return filepath.FromSlash(strings.Join(parts[:i+4], "/"))
	}
	return ""
}

func listRenderableDocs(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return []string{}, nil
	}

	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" ||
				strings.HasPrefix(name, ".pi") {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".md", ".txt", ".json", ".yaml", ".yml":
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}

	sort.SliceStable(files, func(i, j int) bool {
		return lessDocPath(files[i], files[j])
	})
	return files, nil
}

type docTreeBuilder struct {
	name     string
	path     string
	isDir    bool
	children map[string]*docTreeBuilder
}

func buildDocTree(files []string, selectedPath, rootPath string) []DocTreeNode {
	root := &docTreeBuilder{isDir: true, children: map[string]*docTreeBuilder{}}
	for _, file := range files {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(file)))
		if clean == "" || clean == "." {
			continue
		}

		parts := strings.Split(clean, "/")
		current := root
		currentPath := ""
		for i, part := range parts {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath += "/" + part
			}

			child, ok := current.children[part]
			if !ok {
				child = &docTreeBuilder{
					name:     part,
					path:     currentPath,
					isDir:    i < len(parts)-1,
					children: map[string]*docTreeBuilder{},
				}
				current.children[part] = child
			}
			if i < len(parts)-1 {
				child.isDir = true
			}
			current = child
		}
	}

	nodes, _ := buildDocTreeNodes(
		root.children,
		selectedPath,
		strings.TrimSpace(rootPath),
	)
	return nodes
}

func buildDocTreeNodes(
	children map[string]*docTreeBuilder,
	selectedPath, rootPath string,
) ([]DocTreeNode, bool) {
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		left := children[names[i]]
		right := children[names[j]]
		if left.isDir != right.isDir {
			return left.isDir
		}
		return lessDocName(left.name, right.name)
	})

	nodes := make([]DocTreeNode, 0, len(names))
	hasSelectedDescendant := false
	for _, name := range names {
		child := children[name]
		absolutePath := ""
		if rootPath != "" {
			absolutePath = filepath.Join(rootPath, filepath.FromSlash(child.path))
		}
		node := DocTreeNode{
			Path:         child.path,
			AbsolutePath: absolutePath,
			Name:         docDisplayName(child.name, child.isDir),
			IsDir:        child.isDir,
			Selected:     !child.isDir && sameDocPath(child.path, selectedPath),
		}
		if child.isDir {
			node.Children, node.IsExpanded = buildDocTreeNodes(
				child.children,
				selectedPath,
				rootPath,
			)
			hasSelectedDescendant = hasSelectedDescendant || node.IsExpanded
		} else {
			hasSelectedDescendant = hasSelectedDescendant || node.Selected
		}
		nodes = append(nodes, node)
	}
	return nodes, hasSelectedDescendant
}

func docDisplayName(name string, isDir bool) string {
	if isDir {
		return name
	}
	if strings.EqualFold(filepath.Ext(name), ".md") {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

func lessDocPath(left, right string) bool {
	leftKey, leftHasTimestamp := firstTimestampSegment(left)
	rightKey, rightHasTimestamp := firstTimestampSegment(right)
	switch {
	case leftHasTimestamp && rightHasTimestamp && leftKey != rightKey:
		return leftKey > rightKey
	case leftHasTimestamp != rightHasTimestamp:
		return leftHasTimestamp
	default:
		return strings.ToLower(left) < strings.ToLower(right)
	}
}

func lessDocName(left, right string) bool {
	leftKey, leftHasTimestamp := timestampPrefix(left)
	rightKey, rightHasTimestamp := timestampPrefix(right)
	switch {
	case leftHasTimestamp && rightHasTimestamp && leftKey != rightKey:
		return leftKey > rightKey
	case leftHasTimestamp != rightHasTimestamp:
		return leftHasTimestamp
	default:
		return strings.ToLower(left) < strings.ToLower(right)
	}
}

func defaultDocPath(files []string) string {
	hasPlanRoot := containsDocPath(files, "design.md") ||
		containsDocPath(files, "outline.md")
	matchers := []func(string) bool{
		func(path string) bool {
			return sameDocPath(path, "design.md")
		},
		func(path string) bool {
			return sameDocPath(path, "outline.md")
		},
		func(path string) bool {
			return hasPlanRoot && sameDocPath(path, "plan.md")
		},
		func(path string) bool {
			return pathWithinPrimaryDocSection(path, "research")
		},
		func(path string) bool {
			return pathWithinPrimaryDocSection(path, "questions")
		},
		func(path string) bool {
			return strings.EqualFold(filepath.Base(strings.TrimSpace(path)), "design.md")
		},
	}

	for _, matcher := range matchers {
		for _, file := range files {
			if matcher(file) {
				return file
			}
		}
	}
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

func resolveSelectedDocAbsolutePath(
	root string,
	files []string,
	selectedPath string,
) string {
	root = strings.TrimSpace(root)
	selectedPath = strings.TrimSpace(selectedPath)
	if root == "" || selectedPath == "" {
		return ""
	}

	if filepath.IsAbs(selectedPath) {
		if !sameFilesystemPath(selectedPath, root) &&
			!pathWithinRoot(selectedPath, root) {
			return ""
		}
		rel, err := filepath.Rel(root, selectedPath)
		if err != nil {
			return ""
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || !containsDocPath(files, rel) {
			return ""
		}
		return filepath.Join(root, filepath.FromSlash(rel))
	}

	if !containsDocPath(files, selectedPath) {
		return ""
	}
	return filepath.Join(
		root,
		filepath.FromSlash(filepath.ToSlash(filepath.Clean(selectedPath))),
	)
}

func defaultDocAbsolutePath(root string, files []string) string {
	rel := defaultDocPath(files)
	if rel == "" {
		return ""
	}
	return filepath.Join(strings.TrimSpace(root), filepath.FromSlash(rel))
}

func relativeDocPath(root, absolutePath string) string {
	root = strings.TrimSpace(root)
	absolutePath = strings.TrimSpace(absolutePath)
	if root == "" || absolutePath == "" {
		return ""
	}
	if !sameFilesystemPath(absolutePath, root) && !pathWithinRoot(absolutePath, root) {
		return ""
	}
	rel, err := filepath.Rel(root, absolutePath)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ""
	}
	return rel
}

func focusedRootDocPath(root, selectedPath string) string {
	root = strings.TrimSpace(root)
	selectedPath = strings.TrimSpace(selectedPath)
	if root == "" {
		return ""
	}
	if childRoot := implementationReviewPlanRoot(root); childRoot != "" {
		return childRoot
	}
	if childRoot := implementationReviewPlanRoot(selectedPath); childRoot != "" &&
		(sameFilesystemPath(childRoot, root) || pathWithinRoot(childRoot, root)) {
		return childRoot
	}
	if planRoot := planDirectoryRoot(root); planRoot != "" {
		if !sameFilesystemPath(root, planRoot) {
			return root
		}
		return planRoot
	}
	if planRoot := planDirectoryRoot(selectedPath); planRoot != "" {
		if sameFilesystemPath(planRoot, root) || pathWithinRoot(planRoot, root) {
			return planRoot
		}
	}
	return root
}

func docResetPath(cwd, root string) string {
	cwd = strings.TrimSpace(cwd)
	if planRoot := planDirectoryRoot(cwd); planRoot != "" {
		return planRoot
	}
	return strings.TrimSpace(root)
}

func pathWithinPrimaryDocSection(path, section string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." {
		return false
	}

	parts := strings.Split(clean, "/")
	if len(parts) < 2 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parts[0]), strings.TrimSpace(section))
}

func firstTimestampSegment(path string) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." {
		return "", false
	}
	for _, segment := range strings.Split(clean, "/") {
		if key, ok := timestampPrefix(segment); ok {
			return key, true
		}
	}
	return "", false
}

func timestampPrefix(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 10 {
		return "", false
	}
	for _, index := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if trimmed[index] < '0' || trimmed[index] > '9' {
			return "", false
		}
	}
	if trimmed[4] != '-' || trimmed[7] != '-' {
		return "", false
	}

	end := 10
	for end < len(trimmed) {
		char := trimmed[end]
		if (char >= '0' && char <= '9') || char == '-' || char == '_' {
			end++
			continue
		}
		break
	}
	return trimmed[:end], true
}

func docRootLabel(root string) string {
	clean := strings.TrimSpace(root)
	if clean == "" {
		return "Docs"
	}
	label := filepath.Base(filepath.Clean(clean))
	if label == "." || label == string(filepath.Separator) || label == "" {
		return "Docs"
	}
	return label
}

func (s *Service) buildWorkingDirectoryState(
	cwd, resetPath string,
) WorkingDirectoryState {
	abs := strings.TrimSpace(cwd)
	state := WorkingDirectoryState{
		ProjectName:  s.projectName,
		ProjectPath:  s.projectRoot,
		AbsolutePath: abs,
		ResetPath:    strings.TrimSpace(resetPath),
	}
	if abs == "" {
		return state
	}

	base := filepath.Base(filepath.Clean(abs))
	state.CurrentTitle, state.CurrentTimestamp = formatWorkingDirectoryDisplay(base)
	if state.CurrentTitle == "" {
		state.CurrentTitle = base
	}

	if s.projectRoot == "" || !pathWithinRoot(abs, s.projectRoot) {
		state.RelativePath = filepath.Base(abs)
		return state
	}

	rel, err := filepath.Rel(s.projectRoot, abs)
	if err != nil || rel == "." {
		state.CurrentTitle = s.projectName
		return state
	}
	state.RelativePath = filepath.ToSlash(rel)

	parts := strings.Split(state.RelativePath, "/")
	if len(parts) == 0 {
		return state
	}

	state.Crumbs = make([]WorkingDirCrumb, 0, len(parts))
	state.Crumbs = append(
		state.Crumbs,
		WorkingDirCrumb{Label: s.projectName, Path: s.projectRoot},
	)
	current := s.projectRoot
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		state.Crumbs = append(state.Crumbs, WorkingDirCrumb{Label: part, Path: current})
	}
	return state
}

func formatSidebarGroupDisplay(name string) (string, string) {
	label, timestamp := formatWorkingDirectoryDisplay(strings.TrimSpace(name))
	if strings.TrimSpace(label) == "" {
		return strings.TrimSpace(name), timestamp
	}
	return label, timestamp
}

func formatWorkingDirectoryDisplay(name string) (string, string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ""
	}

	prefix, ok := timestampPrefix(trimmed)
	if !ok {
		return trimmed, ""
	}

	timestamp := strings.TrimRight(prefix, "-_")
	remainder := strings.TrimLeft(strings.TrimPrefix(trimmed, prefix), "-_")
	if remainder == "" {
		return trimmed, formatWorkingDirectoryTimestamp(timestamp)
	}
	return humanizeWorkingDirectoryTitle(
			remainder,
		), formatWorkingDirectoryTimestamp(
			timestamp,
		)
}

func humanizeWorkingDirectoryTitle(value string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ")
	parts := strings.Fields(replacer.Replace(strings.TrimSpace(value)))
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func formatWorkingDirectoryTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < len("2006-01-02_15-04-05") {
		return trimmed
	}
	if len(trimmed) >= 19 && trimmed[10] == '_' {
		return trimmed[:10] + " " + trimmed[11:19]
	}
	return strings.Replace(trimmed, "_", " ", 1)
}

func (s *Service) renderDoc(root, relPath string) (DocRenderView, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return DocRenderView{}, nil
	}

	clean := filepath.Clean(strings.TrimSpace(relPath))
	if clean == "" || clean == "." {
		return DocRenderView{RootPath: root, Exists: false}, nil
	}
	if filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return DocRenderView{}, errors.New("doc path traversal is not allowed")
	}

	absolutePath := filepath.Join(root, clean)
	relToRoot, err := filepath.Rel(root, absolutePath)
	if err != nil {
		return DocRenderView{}, err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return DocRenderView{}, errors.New("doc path traversal is not allowed")
	}

	content, err := os.ReadFile(absolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DocRenderView{
				RootPath:     root,
				RelativePath: filepath.ToSlash(clean),
				DisplayName:  filepath.Base(clean),
				Exists:       false,
			}, nil
		}
		return DocRenderView{}, fmt.Errorf("read doc: %w", err)
	}

	return DocRenderView{
		RootPath:     root,
		RelativePath: filepath.ToSlash(clean),
		DisplayName:  filepath.Base(clean),
		HTML:         s.renderer.MarkdownBytesToHTML(content),
		Sections:     s.renderer.RenderToSections(content),
		Exists:       true,
	}, nil
}

func containsDocPath(files []string, selectedPath string) bool {
	for _, file := range files {
		if sameDocPath(file, selectedPath) {
			return true
		}
	}
	return false
}

func sameDocPath(left, right string) bool {
	return filepath.Clean(
		strings.TrimSpace(left),
	) == filepath.Clean(
		strings.TrimSpace(right),
	)
}

type transcriptMetadata struct {
	ModelLabel    string
	ThinkingLabel string
}

func (s *Service) buildTranscriptMetadata(
	ctx context.Context,
	thread db.AgentThread,
) (transcriptMetadata, error) {
	metadata := transcriptMetadata{}
	if !thread.HeadEntryID.Valid {
		return metadata, nil
	}

	rows, err := s.queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: thread.HeadEntryID.String,
	})
	if err != nil {
		return metadata, err
	}

	for _, row := range rows {
		var envelope struct {
			Type          string `json:"type"`
			Provider      string `json:"provider"`
			ModelID       string `json:"modelId"` //nolint:tagliatelle // Pi session JSON uses modelId.
			ThinkingLevel string `json:"thinkingLevel"`
		}
		if err := json.Unmarshal([]byte(row.PayloadJson), &envelope); err != nil {
			return metadata, err
		}
		switch envelope.Type {
		case "model_change":
			model := strings.TrimSpace(envelope.ModelID)
			provider := strings.TrimSpace(envelope.Provider)
			if model != "" && provider != "" {
				metadata.ModelLabel = provider + " / " + model
			} else if model != "" {
				metadata.ModelLabel = model
			}
		case "thinking_level_change":
			metadata.ThinkingLabel = strings.TrimSpace(envelope.ThinkingLevel)
		}
	}

	return metadata, nil
}

func (s *Service) buildStableTranscript(
	ctx context.Context,
	thread db.AgentThread,
) ([]TranscriptMessage, error) {
	messages := []TranscriptMessage{}
	toolCallPresentations := map[string]TranscriptMessage{}

	if !thread.HeadEntryID.Valid {
		return messages, nil
	}

	rows, err := s.queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: thread.HeadEntryID.String,
	})
	if err != nil {
		return nil, err
	}

	attachmentsByRun, err := s.attachmentsByRunForThread(ctx, thread.ID)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		items, err := s.decodePersistedTranscriptItems(
			row.PayloadJson,
			toolCallPresentations,
		)
		if err != nil {
			return nil, err
		}
		if row.OriginRunID.Valid {
			attachTranscriptAttachments(items, attachmentsByRun[row.OriginRunID.String])
		}
		messages = append(messages, items...)
	}

	return combinePairedToolMessages(messages), nil
}

func (s *Service) buildLiveTranscript(threadID string) (LiveTranscriptView, int64) {
	snapshot := s.snapshotLiveThread(threadID)
	liveAttachments := []AttachedPath{}
	if strings.TrimSpace(snapshot.RunID) != "" {
		if paths, err := s.attachedPathsForRun(
			context.Background(),
			snapshot.RunID,
		); err == nil {
			liveAttachments = paths
		}
	}
	policy := s.defaultTranscriptRenderPolicy()
	toolStatus := make(map[string]conversation.ToolExecutionStatus)
	for _, item := range snapshot.Items {
		if item.Kind == conversation.LiveTurnToolExecution {
			toolStatus[strings.TrimSpace(item.ToolCallID)] = item.Status
		}
	}

	toolCallPresentations := map[string]TranscriptMessage{}
	items := []TranscriptMessage{}
	for _, item := range snapshot.Items {
		decoded, err := s.liveTurnTranscriptItems(
			threadID,
			item,
			policy,
			toolStatus,
			toolCallPresentations,
		)
		if err != nil {
			return LiveTranscriptView{}, s.CurrentCursor(threadID)
		}
		attachTranscriptAttachments(decoded, liveAttachments)
		items = append(items, decoded...)
	}

	cursor := s.CurrentCursor(threadID)
	return LiveTranscriptView{Items: combinePairedToolMessages(items)}, cursor
}

func combinePairedToolMessages(items []TranscriptMessage) []TranscriptMessage {
	if len(items) == 0 {
		return items
	}
	combined := make([]TranscriptMessage, 0, len(items))
	pendingIndexByCallID := map[string]int{}
	for _, item := range items {
		toolCallID := strings.TrimSpace(item.ToolCallID)
		if item.ToolResult && toolCallID != "" && isPairableToolName(item.Title) {
			if idx, ok := pendingIndexByCallID[toolCallID]; ok {
				combined = append(combined[:idx], combined[idx+1:]...)
				delete(pendingIndexByCallID, toolCallID)
				for callID, pendingIdx := range pendingIndexByCallID {
					if pendingIdx > idx {
						pendingIndexByCallID[callID] = pendingIdx - 1
					}
				}
			}
		}
		if !item.ToolResult && toolCallID != "" && isPairableToolName(item.Title) {
			pendingIndexByCallID[toolCallID] = len(combined)
		}
		combined = append(combined, item)
	}
	return combined
}

func (s *Service) decodePersistedTranscriptItems(
	payload string,
	toolCallPresentations map[string]TranscriptMessage,
) ([]TranscriptMessage, error) {
	var envelope struct {
		Type          string `json:"type"`
		ID            string `json:"id"`
		Provider      string `json:"provider"`
		ModelID       string `json:"modelId"`
		ThinkingLevel string `json:"thinkingLevel"`
		Message       struct {
			Role       string `json:"role"`
			Content    any    `json:"content"`
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
			Details    any    `json:"details"`
			IsError    bool   `json:"isError"`
		} `json:"message"`
		Content any  `json:"content"`
		Display bool `json:"display"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}

	domID := "entry-" + envelope.ID
	switch envelope.Type {
	case "model_change", "thinking_level_change":
		return nil, nil
	case "message":
		return s.messageTranscriptItems(
			domID,
			envelope.ID,
			envelope.Message.Role,
			envelope.Message.Content,
			true,
			envelope.Message.ToolName,
			envelope.Message.ToolCallID,
			envelope.Message.Details,
			envelope.Message.IsError,
			toolCallPresentations,
		), nil
	case "custom_message":
		if !envelope.Display {
			return nil, nil
		}
		text := extractContentText(envelope.Content)
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []TranscriptMessage{
			s.newBubbleTranscriptMessage(domID, envelope.ID, "assistant", text, true),
		}, nil
	default:
		return nil, nil
	}
}

func (s *Service) liveTurnTranscriptItems(
	threadID string,
	item conversation.LiveTurnItem,
	policy TranscriptRenderPolicy,
	toolState map[string]conversation.ToolExecutionStatus,
	toolCallPresentations map[string]TranscriptMessage,
) ([]TranscriptMessage, error) {
	domID := "live-" + strings.TrimSpace(item.Key)
	if domID == "live-" {
		domID = "live-" + threadID
	}

	switch item.Kind {
	case conversation.LiveTurnUserMessage,
		conversation.LiveTurnAssistantMessage,
		conversation.LiveTurnToolResult:
		var message struct {
			Role       string `json:"role"`
			Content    any    `json:"content"`
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
			Details    any    `json:"details"`
			IsError    bool   `json:"isError"`
		}
		if len(item.MessageJSON) > 0 {
			if err := json.Unmarshal(item.MessageJSON, &message); err != nil {
				return nil, err
			}
		}
		return s.messageTranscriptItemsWithPolicy(
			domID,
			domID,
			message.Role,
			message.Content,
			false,
			message.ToolName,
			message.ToolCallID,
			message.Details,
			message.IsError,
			policy,
			toolState,
			toolCallPresentations,
		), nil
	case conversation.LiveTurnToolExecution:
		return s.toolExecutionTranscriptItems(
			domID,
			domID,
			item,
			toolCallPresentations,
		), nil
	default:
		return nil, nil
	}
}

func (s *Service) toolExecutionTranscriptItems(
	domID, entryID string,
	item conversation.LiveTurnItem,
	toolCallPresentations map[string]TranscriptMessage,
) []TranscriptMessage {
	args := decodeRawJSONValue(item.MessageJSON)
	title, headerCode, headerSummary, hideBodyWhenCollapsed := s.compactToolCallPresentation(
		item.ToolName,
		args,
	)
	if presentation, ok := toolCallPresentations[strings.TrimSpace(item.ToolCallID)]; ok {
		if headerCode == "" {
			headerCode = presentation.HeaderCode
		}
		if headerSummary == "" {
			headerSummary = presentation.HeaderSummary
		}
	}

	statusLabel := liveToolExecutionStatusLabel(item.Status)
	if statusLabel != "" {
		if headerSummary != "" {
			headerSummary += " · " + statusLabel
		} else {
			headerSummary = statusLabel
		}
	}

	body := liveToolExecutionBody(
		item.ToolName,
		item.Status,
		args,
		decodeRawJSONValue(item.ResultJSON),
	)
	msg := s.newDetailTranscriptMessage(
		domID,
		entryID,
		title,
		body,
		item.Status == conversation.ToolExecutionError,
		true,
	)
	msg.ToolCallID = strings.TrimSpace(item.ToolCallID)
	msg.ToolResult = isPairableToolName(title)
	msg.HeaderCode = headerCode
	msg.HeaderSummary = headerSummary
	msg.DetailHeader = detailToolContext(
		item.ToolName,
		extractToolContext(item.ToolName, args),
	)
	msg.HideBodyWhenCollapsed = hideBodyWhenCollapsed &&
		strings.HasPrefix(strings.TrimSpace(body), "```")
	if msg.HideBodyWhenCollapsed {
		msg.Collapsible = true
	}
	return []TranscriptMessage{msg}
}

func liveToolExecutionStatusLabel(status conversation.ToolExecutionStatus) string {
	switch status {
	case conversation.ToolExecutionPending:
		return "pending"
	case conversation.ToolExecutionRunning:
		return "running"
	case conversation.ToolExecutionDone:
		return "done"
	case conversation.ToolExecutionError:
		return "error"
	default:
		return ""
	}
}

func liveToolExecutionBody(
	toolName string,
	status conversation.ToolExecutionStatus,
	args, result any,
) string {
	switch status {
	case conversation.ToolExecutionPending:
		if body := formatJSONBlock(args); body != "" {
			return body
		}
		return "_Pending execution._"
	case conversation.ToolExecutionRunning:
		if body := liveToolResultBody(toolName, result); body != "" {
			return body
		}
		if body := formatJSONBlock(args); body != "" {
			return body
		}
		return "_Tool execution in progress._"
	case conversation.ToolExecutionError:
		if body := liveToolResultBody(toolName, result); body != "" {
			return body
		}
		return "_Tool execution failed._"
	case conversation.ToolExecutionDone:
		return "_Tool execution complete._"
	default:
		if body := formatJSONBlock(args); body != "" {
			return body
		}
		return "_Pending execution._"
	}
}

func liveToolResultBody(toolName string, result any) string {
	rawBody := strings.TrimSpace(extractContentText(result))
	if isReadToolResult(toolName, rawBody) {
		return formatReadToolResultBlock(rawBody)
	}
	if rawBody != "" {
		if isCodeBlockToolResult(toolName) {
			return formatToolResultBlock(rawBody)
		}
		return rawBody
	}
	return formatJSONBlock(result)
}

func decodeRawJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func (s *Service) messageTranscriptItems(
	domID, entryID, role string,
	content any,
	showFork bool,
	toolName, toolCallID string,
	details any,
	isError bool,
	toolCallPresentations map[string]TranscriptMessage,
) []TranscriptMessage {
	return s.messageTranscriptItemsWithPolicy(
		domID,
		entryID,
		role,
		content,
		showFork,
		toolName,
		toolCallID,
		details,
		isError,
		s.defaultTranscriptRenderPolicy(),
		nil,
		toolCallPresentations,
	)
}

func (s *Service) messageTranscriptItemsWithPolicy(
	domID, entryID, role string,
	content any,
	showFork bool,
	toolName, toolCallID string,
	details any,
	isError bool,
	policy TranscriptRenderPolicy,
	toolState map[string]conversation.ToolExecutionStatus,
	toolCallPresentations map[string]TranscriptMessage,
) []TranscriptMessage {
	role = strings.TrimSpace(role)
	switch role {
	case "user":
		text := strings.TrimSpace(extractContentText(content))
		if text == "" {
			return nil
		}
		return []TranscriptMessage{
			s.newBubbleTranscriptMessage(domID, entryID, "user", text, showFork),
		}
	case "assistant":
		return s.assistantTranscriptItemsWithPolicy(
			domID,
			entryID,
			content,
			showFork,
			policy,
			toolState,
			toolCallPresentations,
		)
	case "toolResult":
		title := compactToolResultTitle(toolName)
		rawBody := strings.TrimSpace(extractContentText(content))
		body := rawBody
		if isCodeBlockToolResult(toolName) && rawBody != "" {
			title = compactToolCallTitle(toolName)
			body = formatToolResultBlock(rawBody)
		} else if body == "" {
			body = formatJSONBlock(details)
		}
		if body == "" {
			body = fmt.Sprintf("`%s`", strings.TrimSpace(toolCallID))
		}
		msg := s.newDetailTranscriptMessage(domID, entryID, title, body, isError, true)
		msg.ToolCallID = strings.TrimSpace(toolCallID)
		msg.ToolResult = isPairableToolName(title)
		if isCodeBlockToolResult(toolName) && rawBody != "" {
			msg.HideBodyWhenCollapsed = true
			msg.Collapsible = true
		}
		if presentation, ok := toolCallPresentations[strings.TrimSpace(toolCallID)]; ok {
			if msg.HeaderCode == "" {
				msg.HeaderCode = presentation.HeaderCode
			}
			if msg.HeaderSummary == "" {
				msg.HeaderSummary = presentation.HeaderSummary
			}
		}
		msg.DetailHeader = detailToolContext(toolName, msg.HeaderCode)
		return []TranscriptMessage{msg}
	default:
		return nil
	}
}

func (s *Service) assistantTranscriptItems(
	domID, entryID string,
	content any,
	showFork bool,
	toolCallPresentations map[string]TranscriptMessage,
) []TranscriptMessage {
	return s.assistantTranscriptItemsWithPolicy(
		domID,
		entryID,
		content,
		showFork,
		s.defaultTranscriptRenderPolicy(),
		nil,
		toolCallPresentations,
	)
}

func (s *Service) assistantTranscriptItemsWithPolicy(
	domID, entryID string,
	content any,
	showFork bool,
	policy TranscriptRenderPolicy,
	toolState map[string]conversation.ToolExecutionStatus,
	toolCallPresentations map[string]TranscriptMessage,
) []TranscriptMessage {
	blocks := contentBlocks(content)
	if len(blocks) == 0 {
		text := strings.TrimSpace(extractContentText(content))
		if text == "" {
			return nil
		}
		return []TranscriptMessage{
			s.newBubbleTranscriptMessage(domID, entryID, "assistant", text, showFork),
		}
	}

	items := []TranscriptMessage{}
	bubbleIndexes := []int{}
	textParts := []string{}
	textIndex := 0
	detailIndex := 0

	flushText := func() {
		text := strings.TrimSpace(strings.Join(textParts, ""))
		if text == "" {
			textParts = nil
			return
		}
		suffix := ""
		if textIndex > 0 {
			suffix = fmt.Sprintf("-text-%d", textIndex)
		}
		items = append(
			items,
			s.newBubbleTranscriptMessage(domID+suffix, entryID, "assistant", text, false),
		)
		bubbleIndexes = append(bubbleIndexes, len(items)-1)
		textParts = nil
		textIndex++
	}

	for _, block := range blocks {
		switch strings.TrimSpace(valueAsString(block["type"])) {
		case "text":
			if text := valueAsString(block["text"]); text != "" {
				textParts = append(textParts, text)
			}
		case "thinking":
			flushText()
			if policy.HideThinking {
				continue
			}
			body := strings.TrimSpace(valueAsString(block["thinking"]))
			if body == "" {
				body = extractSummaryText(block["summary"])
			}
			if body == "" {
				body = "Reasoning block recorded; model did not expose readable text."
			}
			msg := s.newDetailTranscriptMessage(
				fmt.Sprintf("%s-detail-%d", domID, detailIndex),
				entryID,
				"thinking",
				formatMarkdownBlockquote(body),
				false,
				true,
			)
			msg.HeaderSummary = thinkingHeaderSummary(body)
			msg.HideBodyWhenCollapsed = true
			msg.Collapsible = true
			items = append(items, msg)
			detailIndex++
		case "toolCall":
			flushText()
			toolCallID := strings.TrimSpace(valueAsString(block["id"]))
			title, headerCode, headerSummary, hideBodyWhenCollapsed := s.compactToolCallPresentation(
				valueAsString(block["name"]),
				block["arguments"],
			)
			body := formatJSONBlock(block["arguments"])
			if body == "" {
				if partial := strings.TrimSpace(
					valueAsString(block["partialJson"]),
				); partial != "" {
					body = "```json\n" + partial + "\n```"
				}
			}
			if body == "" {
				body = "_No arguments captured._"
			}
			msg := s.newDetailTranscriptMessage(
				fmt.Sprintf("%s-detail-%d", domID, detailIndex),
				entryID,
				title,
				body,
				false,
				true,
			)
			msg.ToolCallID = toolCallID
			msg.HeaderCode = headerCode
			msg.HeaderSummary = headerSummary
			msg.HideBodyWhenCollapsed = hideBodyWhenCollapsed &&
				strings.HasPrefix(strings.TrimSpace(body), "```json")
			if msg.HideBodyWhenCollapsed {
				msg.Collapsible = true
			}
			if toolCallPresentations != nil && toolCallID != "" {
				toolCallPresentations[toolCallID] = msg
			}
			if policy.HideToolCalls {
				continue
			}
			if policy.HidePendingToolCalls && toolState[toolCallID] == "" {
				continue
			}
			items = append(items, msg)
			detailIndex++
		}
	}

	flushText()
	if showFork && len(bubbleIndexes) > 0 {
		items[bubbleIndexes[len(bubbleIndexes)-1]].ShowForkForm = true
	}
	return items
}

func formatMarkdownBlockquote(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	quoted := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			quoted = append(quoted, ">")
			continue
		}
		quoted = append(quoted, "> "+line)
	}
	return strings.Join(quoted, "\n")
}

func thinkingHeaderSummary(body string) string {
	preview := strings.Join(strings.Fields(body), " ")
	charCount := len([]rune(preview))
	if charCount == 0 {
		return "[0]"
	}

	const previewRuneLimit = 240
	previewRunes := []rune(preview)
	if len(previewRunes) > previewRuneLimit {
		preview = string(previewRunes[:previewRuneLimit]) + "…"
	}
	return fmt.Sprintf("[%d] %s", charCount, preview)
}

func (s *Service) compactToolCallPresentation(
	toolName string,
	arguments any,
) (title, headerCode, headerSummary string, hideBodyWhenCollapsed bool) {
	title = compactToolCallTitle(toolName)
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch name {
	case "bash":
		return title, extractArgumentString(arguments, "command"), "", true
	case "read":
		return title, s.compactProjectPath(
			extractArgumentString(arguments, "path"),
		), "", true
	case "subagent":
		agent := extractArgumentString(arguments, "agent")
		task := compactSingleLine(extractArgumentString(arguments, "task"), 96)
		parts := make([]string, 0, 2)
		if agent != "" {
			parts = append(parts, "agent: "+agent)
		}
		if task != "" {
			parts = append(parts, "task: "+task)
		}
		return title, "", strings.Join(parts, " · "), true
	default:
		return title, "", "", false
	}
}

func (s *Service) compactProjectPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}

	projectName := strings.TrimSpace(s.projectName)
	if projectName == "" {
		projectName = filepath.Base(filepath.Clean(s.projectRoot))
	}
	if projectName == "." || projectName == string(filepath.Separator) {
		projectName = ""
	}

	if filepath.IsAbs(trimmed) {
		if s.projectRoot != "" &&
			(sameFilesystemPath(trimmed, s.projectRoot) || pathWithinRoot(trimmed, s.projectRoot)) {
			rel, err := filepath.Rel(s.projectRoot, trimmed)
			if err == nil {
				if rel == "." {
					return projectName
				}
				if projectName != "" {
					return filepath.ToSlash(filepath.Join(projectName, rel))
				}
				return filepath.ToSlash(rel)
			}
		}
		return filepath.ToSlash(trimmed)
	}

	clean := filepath.Clean(trimmed)
	if clean == "." {
		return projectName
	}
	if strings.HasPrefix(clean, "..") {
		return filepath.ToSlash(clean)
	}
	if projectName != "" {
		return filepath.ToSlash(filepath.Join(projectName, clean))
	}
	return filepath.ToSlash(clean)
}

func extractArgumentString(arguments any, key string) string {
	typed, ok := arguments.(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(valueAsString(typed[key]))
}

func compactSingleLine(value string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if trimmed == "" || limit <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "…"
}

func compactToolCallTitle(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name != "" {
		return name
	}
	return "tool use"
}

func compactToolResultTitle(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name != "" {
		return name + " output"
	}
	return "tool output"
}

func detailToolContext(toolName, toolContext string) string {
	trimmed := strings.TrimSpace(toolContext)
	if trimmed == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(toolName), "bash") {
		return "$ " + trimmed
	}
	return trimmed
}

func extractToolContext(toolName string, args any) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash":
		return extractArgumentString(args, "command")
	case "read":
		return extractArgumentString(args, "path")
	default:
		return ""
	}
}

func isPairableToolName(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read", "bash":
		return true
	default:
		return false
	}
}

func (s *Service) newBubbleTranscriptMessage(
	domID, entryID, role, text string,
	showFork bool,
) TranscriptMessage {
	htmlContent := ""
	if role == "assistant" {
		htmlContent = s.renderer.MarkdownBytesToHTML([]byte(text))
	}
	return TranscriptMessage{
		DOMID:        domID,
		EntryID:      entryID,
		Variant:      "bubble",
		Role:         role,
		Content:      text,
		HTMLContent:  htmlContent,
		ShowForkForm: showFork,
	}
}

func (s *Service) newDetailTranscriptMessage(
	domID, entryID, title, body string,
	isError, collapsible bool,
) TranscriptMessage {
	htmlContent := ""
	if strings.TrimSpace(body) != "" {
		htmlContent = s.renderer.MarkdownBytesToHTML([]byte(body))
	}
	return TranscriptMessage{
		DOMID:       domID,
		EntryID:     entryID,
		Variant:     "detail",
		Title:       title,
		Content:     body,
		HTMLContent: htmlContent,
		IsError:     isError,
		Collapsible: collapsible && s.shouldCollapseDetail(body),
	}
}

func contentBlocks(content any) []map[string]any {
	items, ok := content.([]any)
	if !ok {
		return nil
	}
	blocks := make([]map[string]any, 0, len(items))
	for _, item := range items {
		block, ok := item.(map[string]any)
		if ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func extractSummaryText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(extractContentText(item))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func valueAsString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func formatJSONBlock(value any) string {
	if value == nil {
		return ""
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(encoded))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" || trimmed == "[]" {
		return ""
	}
	return "```json\n" + trimmed + "\n```"
}

func isReadToolResult(toolName, body string) bool {
	return strings.EqualFold(strings.TrimSpace(toolName), "read") &&
		strings.TrimSpace(body) != ""
}

func isCodeBlockToolResult(toolName string) bool {
	return isPairableToolName(toolName)
}

func formatReadToolResultBlock(body string) string {
	return formatToolResultBlock(body)
}

func formatToolResultBlock(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	return "````\n" + trimmed + "\n````"
}

func (s *Service) shouldCollapseDetail(body string) bool {
	return detailLineCount(body) > s.detailCollapseLineLimit
}

func detailLineCount(body string) int {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return 0
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") &&
		strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[1 : len(lines)-1]
	}
	if len(lines) == 0 {
		return 0
	}
	return len(lines)
}

func (s *Service) resolveRequestedCwd(cwd string) (string, error) {
	resolved := strings.TrimSpace(cwd)
	if resolved == "" {
		return "", fmt.Errorf("cwd is required")
	}
	if abs, err := filepath.Abs(resolved); err == nil {
		resolved = abs
	}
	if s.projectRoot != "" && !pathWithinRoot(resolved, s.projectRoot) {
		return "", fmt.Errorf("cwd must stay within %s", s.projectName)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd must be a directory")
	}
	return resolved, nil
}

func pathWithinRoot(path, root string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	if cleanPath == "" || cleanRoot == "" {
		return false
	}
	if cleanPath == cleanRoot {
		return true
	}
	prefix := cleanRoot + string(filepath.Separator)
	return strings.HasPrefix(cleanPath, prefix)
}

func sameFilesystemPath(left, right string) bool {
	cleanLeft := filepath.Clean(strings.TrimSpace(left))
	cleanRight := filepath.Clean(strings.TrimSpace(right))
	if cleanLeft == cleanRight {
		return true
	}
	absLeft, errLeft := filepath.Abs(cleanLeft)
	absRight, errRight := filepath.Abs(cleanRight)
	return errLeft == nil && errRight == nil && absLeft == absRight
}

func (s *Service) resolveCwd(cwd string) string {
	resolved := strings.TrimSpace(cwd)
	if resolved == "" {
		resolved = s.defaultCwd
	}
	if resolved != "" {
		if validated, err := s.resolveRequestedCwd(resolved); err == nil {
			return validated
		}
	}
	if resolved == "" {
		if wd, err := os.Getwd(); err == nil {
			resolved = wd
		}
	}
	if resolved == "" {
		return "."
	}
	if abs, err := filepath.Abs(resolved); err == nil {
		return abs
	}
	return resolved
}

func truncateTitle(prompt string) string {
	firstLine := strings.TrimSpace(strings.Split(prompt, "\n")[0])
	if firstLine == "" {
		return "New Chat"
	}

	runes := []rune(firstLine)
	if len(runes) > 72 {
		return string(runes[:72]) + "…"
	}
	return firstLine
}

func extractContentText(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			text := strings.TrimSpace(extractContentText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := value["text"].(string); ok {
			return text
		}
		if contentText, ok := value["content"].(string); ok {
			return contentText
		}
		if nested, ok := value["content"].([]any); ok {
			return extractContentText(nested)
		}
		return ""
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}

func listRenderableArtifacts(
	root string,
) ([]string, error) {
	return listRenderableDocs(root)
}

func defaultArtifactPath(
	files []string,
) string {
	return defaultDocPath(files)
}

func (s *Service) renderArtifact(root, relPath string) (ArtifactRenderView, error) {
	return s.renderDoc(root, relPath)
}

func (s *Service) BuildArtifactPane(
	ctx context.Context,
	threadID, runID, selectedPath string,
) (ArtifactPaneState, error) {
	return s.BuildDocPane(ctx, threadID, runID, selectedPath)
}
