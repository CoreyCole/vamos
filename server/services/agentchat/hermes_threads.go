package agentchat

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/safecomponent"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

type HermesThread struct {
	ID              string
	CreatorEmail    string
	OwnerEmail      string
	PromptAuthority HermesPromptAuthority
	Title           string
	WorkspaceID     string
	PlanDir         string
	UpdatedAt       time.Time
}

type ThreadQuery struct {
	PlanDir string
	Search  string
}

type HermesThreadGroup struct {
	PlanDir string
	Threads []HermesThread
}

type CreateHermesThreadInput struct {
	PlanDir      HermesPlanIdentity
	CreatorEmail string
	Title        string
}

var newHermesThreadID = func() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func (s *Service) CanPromptThread(userEmail string, thread HermesThread) bool {
	return thread.PromptAuthority.PrincipalType == "authenticated_email" &&
		normalizeHermesAuthorityEmail(userEmail) == thread.PromptAuthority.PrincipalValue
}

func (s *Service) CreateHermesThread(
	ctx context.Context, input CreateHermesThreadInput,
) (HermesThread, error) {
	planDir, err := s.hermesPlanDir(string(input.PlanDir))
	if err != nil {
		return HermesThread{}, err
	}
	creator := strings.TrimSpace(input.CreatorEmail)
	authority := normalizeHermesAuthorityEmail(creator)
	if creator == "" || authority == "" {
		return HermesThread{}, errors.New("authenticated creator is required")
	}
	for attempt := 0; attempt < 16; attempt++ {
		threadID := newHermesThreadID()
		path, err := HermesTranscriptPath(planDir, threadID)
		if err != nil {
			return HermesThread{}, err
		}
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return HermesThread{}, err
		}
		created := time.Now().UTC()
		event := HermesTranscriptEvent{
			ID:           "metadata_" + threadID,
			At:           created,
			Type:         "thread_metadata",
			ThreadID:     threadID,
			PlanDir:      input.PlanDir,
			CreatorEmail: creator,
			PromptAuthority: &HermesPromptAuthority{
				PrincipalType: "authenticated_email", PrincipalValue: authority,
			},
			Title: strings.TrimSpace(input.Title),
		}
		if err := appendHermesTranscript(ctx, planDir, event); err != nil {
			return HermesThread{}, err
		}
		return HermesThread{
			ID: threadID, CreatorEmail: creator, OwnerEmail: authority,
			PromptAuthority: *event.PromptAuthority,
			Title:           event.Title, PlanDir: string(input.PlanDir), UpdatedAt: created,
		}, nil
	}
	return HermesThread{}, errors.New("could not allocate Hermes thread ID")
}

func (s *Service) ListHermesThreads(
	ctx context.Context, query ThreadQuery,
) ([]HermesThread, error) {
	if strings.TrimSpace(query.PlanDir) == "" {
		return []HermesThread{}, nil
	}
	identity := HermesPlanIdentity(query.PlanDir)
	planDir, err := s.hermesPlanDir(query.PlanDir)
	if err != nil {
		return nil, err
	}
	root, err := resolveExistingDirectory(s.thoughtsRoot)
	if err != nil {
		return nil, err
	}
	artifacts, err := ScanHermesThreads(root, planDir)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query.Search))
	threads := make([]HermesThread, 0, len(artifacts))
	for _, item := range artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item.PlanIdentity != identity {
			return nil, errors.New("Hermes artifact plan identity mismatch")
		}
		title := item.Title
		if title == "" {
			title = item.ID
		}
		thread := HermesThread{
			ID: item.ID, CreatorEmail: item.CreatorEmail,
			OwnerEmail:      item.PromptAuthority.PrincipalValue,
			PromptAuthority: item.PromptAuthority, Title: title,
			PlanDir: string(item.PlanIdentity), UpdatedAt: item.UpdatedAt,
		}
		if needle == "" || strings.Contains(strings.ToLower(title), needle) {
			threads = append(threads, thread)
		}
	}
	sort.SliceStable(threads, func(i, j int) bool {
		return threads[i].UpdatedAt.After(threads[j].UpdatedAt)
	})
	return threads, nil
}

func GroupHermesThreads(threads []HermesThread) []HermesThreadGroup {
	byPlan := map[string][]HermesThread{}
	for _, thread := range threads {
		byPlan[thread.PlanDir] = append(byPlan[thread.PlanDir], thread)
	}
	groups := make([]HermesThreadGroup, 0, len(byPlan))
	for plan, items := range byPlan {
		groups = append(groups, HermesThreadGroup{PlanDir: plan, Threads: items})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].PlanDir < groups[j].PlanDir })
	return groups
}

type HermesThreadsPanelArgs struct {
	UserEmail             string
	CurrentFile           string
	PlanDir               string
	Threads               []HermesThread
	ThreadHrefs           map[string]string
	SelectedID            string
	SelectedTitle         string
	Messages              []ChatMessageArgs
	DeliveryStatus        string
	CanPrompt             bool
	NoPlan                bool
	ReturnURL             string
	CommandID             string
	ConversationReference string
	StreamURL             string
}

func (s *Service) RenderHermesThreadsPanel(
	ctx context.Context,
	request markdown.HermesThreadsRenderRequest,
) (templ.Component, markdown.HermesThreadsURLReplacement, error) {
	args := HermesThreadsPanelArgs{
		UserEmail: request.UserEmail, CurrentFile: request.DocPath,
		NoPlan: request.NoPlan, ThreadHrefs: map[string]string{},
		ReturnURL: request.CurrentURL,
	}
	if args.ReturnURL == "" {
		args.ReturnURL = markdown.ThoughtsHrefWithWorkbenchState(
			request.DocPath, request.IsDirectory, request.LinkState,
		)
	}
	if request.NoPlan {
		return HermesThreadsPanel(args), hermesSelectionReplacement(request), nil
	}
	identity, _, err := ResolveHermesPlanIdentity(
		s.thoughtsRoot,
		request.SelectedPlanPath,
		request.PlanHint,
	)
	if err != nil {
		return nil, markdown.HermesThreadsURLReplacement{}, err
	}
	args.PlanDir = string(identity)
	threads, err := s.ListHermesThreads(ctx, ThreadQuery{PlanDir: string(identity)})
	if err != nil {
		return nil, markdown.HermesThreadsURLReplacement{}, err
	}
	args.Threads = threads
	for _, thread := range threads {
		state := request.LinkState
		state.SelectedPlanPath = string(identity)
		state = state.WithHermesThread(thread.ID)
		args.ThreadHrefs[thread.ID] = markdown.ThoughtsHrefWithWorkbenchState(
			request.DocPath,
			request.IsDirectory,
			state,
		)
	}
	selectedID := strings.TrimSpace(request.LinkState.HermesThreadID)
	if selectedID == "" {
		return HermesThreadsPanel(args), markdown.HermesThreadsURLReplacement{}, nil
	}
	if err := safecomponent.ValidateBounded(selectedID); err != nil {
		return HermesThreadsPanel(args), hermesSelectionReplacement(request), nil
	}
	for _, thread := range threads {
		if thread.ID != selectedID {
			continue
		}
		view, err := s.renderHermesTranscriptView(ctx, string(identity), thread.ID)
		if err != nil {
			return nil, markdown.HermesThreadsURLReplacement{}, err
		}
		args.SelectedID = thread.ID
		args.SelectedTitle = thread.Title
		args.Messages = view.Messages
		args.DeliveryStatus = view.Status
		args.StreamURL = "/agent-chat/hermes/threads/" + url.PathEscape(thread.ID) +
			"/transcript?plan_dir=" + url.QueryEscape(string(identity))
		args.CanPrompt = s.CanPromptThread(request.UserEmail, thread)
		if args.CanPrompt {
			args.CommandID = strings.ReplaceAll(uuid.NewString(), "-", "")
			args.ConversationReference, err = HermesConversationReference(identity, thread.ID)
			if err != nil {
				return nil, markdown.HermesThreadsURLReplacement{}, err
			}
		}
		return HermesThreadsPanel(args), markdown.HermesThreadsURLReplacement{}, nil
	}
	return HermesThreadsPanel(args), hermesSelectionReplacement(request), nil
}

func hermesSelectionReplacement(
	request markdown.HermesThreadsRenderRequest,
) markdown.HermesThreadsURLReplacement {
	if strings.TrimSpace(request.LinkState.HermesThreadID) == "" {
		return markdown.HermesThreadsURLReplacement{}
	}
	state := request.LinkState.WithoutHermesThread()
	current := strings.TrimSpace(request.CurrentURL)
	if current == "" {
		current = markdown.ThoughtsHrefWithWorkbenchState(
			request.DocPath,
			request.IsDirectory,
			state,
		)
	} else if parsed, err := url.Parse(current); err == nil {
		query := parsed.Query()
		query.Del("hermes_thread")
		parsed.RawQuery = query.Encode()
		current = parsed.String()
	}
	return markdown.HermesThreadsURLReplacement{URL: current}
}
