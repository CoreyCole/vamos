package steps

import (
	"fmt"
	"sort"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

type (
	StepVerb  string
	Args      map[string]string
	GoLiteral string
)

type Call struct {
	Function string
	Args     []GoLiteral
}

type Definition struct {
	Verb    StepVerb
	Pattern string
	Compile func(story.Step) (Call, error)
	Execute func(t testing.TB, ctx any, args Args)
}

type Catalog interface {
	Resolve(step story.Step) (Definition, Args, error)
	ResolveStep(step story.Step) error
	Verbs() []StepVerb
}

type defaultCatalog struct{ defs map[story.StepVerb]Definition }

func DefaultCatalog() Catalog {
	defs := map[story.StepVerb]Definition{}
	add := func(verb story.StepVerb, fn string, argKeys ...string) {
		defs[verb] = Definition{
			Verb:    StepVerb(verb),
			Pattern: string(verb),
			Compile: compileCall(fn, argKeys...),
		}
	}
	add("authenticated_as", "AuthenticatedAs", "email")
	add("load_fixture", "LoadFixture", "name")
	add("visit", "Visit", "path")
	add("open_plan_workspace", "OpenPlanWorkspace", "plan_dir")
	add("open_workspace_chat", "OpenWorkspaceChat", "target")
	add("open_freeform_chat_fixture", "OpenFreeformChatFixture", "name")
	add("open_thoughts_root_chat", "OpenThoughtsRootChat", "target")
	add("send_freeform_chat_prompt", "SendFreeformChatPrompt", "marker")
	add("wait_for_latest_freeform_chat_run", "WaitForLatestFreeformChatRun", "target")
	add(
		"wait_for_latest_freeform_chat_run_completion",
		"WaitForLatestFreeformChatRunCompletion",
		"target",
	)
	add("seed_latest_workspace_chats", "SeedLatestWorkspaceChats", "marker_a", "marker_b")
	add("seed_project_plan_workspaces", "SeedProjectPlanWorkspaces", "project_a", "project_b")
	add("open_seeded_workspace_chat", "OpenSeededWorkspaceChat", "label")
	add("open_workspace_document_without_chat_params", "OpenWorkspaceDocumentWithoutChatParams", "label")
	add("open_thoughts_root_chat_context", "OpenThoughtsRootChatContext", "target")
	add("follow_first_sidebar_document_link", "FollowFirstSidebarDocumentLink", "target")
	add("follow_first_breadcrumb_link", "FollowFirstBreadcrumbLink", "target")
	add("switch_tab", "SwitchTab", "key")
	add("toggle_region", "ToggleRegion", "key")
	add("enable_show_historical_workspaces", "EnableShowHistoricalWorkspaces", "target")
	add("cleanup_workspace", "CleanupWorkspace", "name")
	add("reload_chat", "ReloadChat", "target")
	add("reopen_current_chat", "ReopenCurrentChat", "target")
	add("remember_file_hash", "RememberFileHash", "path")
	add("send_pi_docs_review_prompt", "SendPiDocsReviewPrompt", "marker", "artifact")
	add("wait_for_chat_marker", "WaitForChatMarker", "marker")
	add("seed_latest_freeform_chat_qrspi_project_result", "SeedLatestFreeformChatQRSPIProjectResult", "project")
	add("wait_for_feature_ready", "WaitForFeatureReady", "feature")
	add("expect_region_visible", "ExpectRegionVisible", "key")
	add("expect_region_reachable", "ExpectRegionReachable", "key")
	add("expect_tab_selected", "ExpectTabSelected", "key")
	add("expect_inactive_tab_panels_hidden", "ExpectInactiveTabPanelsHidden", "target")
	add("expect_workspace_visible", "ExpectWorkspaceVisible", "name")
	add("expect_workspace_absent", "ExpectWorkspaceAbsent", "name")
	add("expect_workspace_before", "ExpectWorkspaceBefore", "first", "second")
	add("expect_workspace_cleanup_succeeds", "ExpectWorkspaceCleanupSucceeds", "target")
	add("expect_text_absent", "ExpectTextAbsent", "text")
	add("expect_browser_url_contains", "ExpectBrowserURLContains", "text")
	add("expect_console_clean", "ExpectConsoleClean", "scope")
	add("expect_transcript_contains", "ExpectTranscriptContains", "text")
	add("expect_thread_metadata_project", "ExpectThreadMetadataProject", "project")
	add("expect_plan_sidebar_contains_project_plan", "ExpectPlanSidebarContainsProjectPlan", "project")
	add("expect_plan_sidebar_absent_project_plan", "ExpectPlanSidebarAbsentProjectPlan", "project")
	add("expect_file_hash_changed", "ExpectFileHashChanged", "path")
	add("expect_pi_review_file_sections", "ExpectPiReviewFileSections", "path")
	add("expect_only_file_changed", "ExpectOnlyFileChanged", "path")
	return defaultCatalog{defs: defs}
}

func (c defaultCatalog) Resolve(step story.Step) (Definition, Args, error) {
	d, ok := c.defs[step.Verb]
	if !ok {
		return Definition{}, nil, fmt.Errorf("unsupported step verb %q", step.Verb)
	}
	return d, Args(step.Args), nil
}

func (c defaultCatalog) ResolveStep(step story.Step) error {
	_, _, err := c.Resolve(step)
	return err
}

func (c defaultCatalog) Verbs() []StepVerb {
	out := make([]StepVerb, 0, len(c.defs))
	for _, d := range c.defs {
		out = append(out, d.Verb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func compileCall(fn string, argKeys ...string) func(story.Step) (Call, error) {
	return func(step story.Step) (Call, error) {
		args := make([]GoLiteral, 0, len(argKeys))
		for _, key := range argKeys {
			val := step.Args[key]
			if val == "" {
				return Call{}, fmt.Errorf("missing arg %s", key)
			}
			args = append(args, GoLiteral(fmt.Sprintf("%q", val)))
		}
		return Call{Function: fn, Args: args}, nil
	}
}
