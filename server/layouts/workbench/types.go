package workbench

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/a-h/templ"
)

type WorkbenchPage string

const (
	WorkbenchPageAgentChat WorkbenchPage = "agent-chat"
	WorkbenchPageThoughts  WorkbenchPage = "thoughts"
)

type WorkbenchView string

type ViewportClass string

type WorkbenchLayoutMode string

const (
	WorkbenchViewFocus WorkbenchView = "focus"
	WorkbenchViewSplit WorkbenchView = "split"

	ViewportMobile      ViewportClass = "mobile"
	ViewportDesktopHalf ViewportClass = "desktop-half"
	ViewportDesktopFull ViewportClass = "desktop-full"

	WorkbenchWorkspaceDocChat WorkbenchLayoutMode = "workspace-doc-chat"
)

type WorkbenchSlot string

const (
	WorkbenchSlotNavigation WorkbenchSlot = "navigation"
	WorkbenchSlotPrimary    WorkbenchSlot = "primary"
	WorkbenchSlotContext    WorkbenchSlot = "context"
)

type RegionKind string

const (
	RegionChat              RegionKind = "chat"
	RegionDocument          RegionKind = "document"
	RegionComments          RegionKind = "comments"
	RegionSections          RegionKind = "sections"
	RegionThoughtsTree      RegionKind = "thoughts-tree"
	RegionPlanSidebar       RegionKind = "plan-sidebar"
	RegionWorkspaceTopology RegionKind = "workspace-topology"
	RegionDoc               RegionKind = "doc"
	RegionArtifact                     = RegionDoc
	RegionWorkflow          RegionKind = "workflow"
	RegionRuns              RegionKind = "runs"
	RegionEmpty             RegionKind = "empty"
)

type WorkbenchRegion struct {
	ID        string
	SignalKey string
	Slot      WorkbenchSlot
	Kind      RegionKind
	Ratio     float64
	MinRem    float64
	Visible   bool
	TargetID  string
	Title     string
	Component templ.Component
}

type WorkbenchConfig struct {
	Version       int           `json:"version"`
	Page          WorkbenchPage `json:"page"`
	View          WorkbenchView `json:"view"`
	ViewportClass ViewportClass `json:"viewportClass,omitempty"`
	Regions       []RegionSpec  `json:"regions"`
	Mobile        MobileSpec    `json:"mobile"`
}

type RegionSpec struct {
	ID      string        `json:"id"`
	Slot    WorkbenchSlot `json:"slot"`
	Kind    RegionKind    `json:"kind"`
	Ratio   float64       `json:"ratio"`
	Visible bool          `json:"visible"`
}

type MobileSpec struct {
	ActiveRegionID string `json:"activeRegionID"`
}

type RegionNormalState struct {
	SignalKey string
	Available bool
	Visible   bool
}

type WorkbenchState struct {
	UserEmail     string
	Page          WorkbenchPage
	View          WorkbenchView
	ViewportClass ViewportClass
	ActivePath    string
	ContextMode   string
	RouteHref     string
	Config        WorkbenchConfig
	Regions       []WorkbenchRegion

	FocusDefault  bool
	NormalRegions []RegionNormalState
}

type BuildWorkbenchStateInput struct {
	UserEmail     string
	Page          WorkbenchPage
	View          WorkbenchView
	ViewportClass ViewportClass
	ActivePath    string
	ContextMode   string
	RouteHref     string
	SavedConfig   *WorkbenchConfig
	Regions       []WorkbenchRegion

	FocusDefault  bool
	NormalRegions []RegionNormalState
}

func ValidateWorkbenchConfig(config WorkbenchConfig) error {
	if config.Version != 1 {
		return fmt.Errorf("unsupported workbench config version %d", config.Version)
	}
	if !validPage(config.Page) {
		return fmt.Errorf("invalid page %q", config.Page)
	}
	if !validView(config.View) {
		return fmt.Errorf("invalid view %q", config.View)
	}
	if config.ViewportClass != "" {
		if _, ok := ParseViewportClass(string(config.ViewportClass)); !ok {
			return fmt.Errorf("invalid viewport class %q", config.ViewportClass)
		}
	}

	seen := map[string]bool{}
	for _, region := range config.Regions {
		if region.ID == "" {
			return errors.New("region id is required")
		}
		if seen[region.ID] {
			return fmt.Errorf("duplicate region id %q", region.ID)
		}
		seen[region.ID] = true
		if !validSlot(region.Slot) {
			return fmt.Errorf("invalid slot %q", region.Slot)
		}
		if !validKind(region.Kind) {
			return fmt.Errorf("invalid kind %q", region.Kind)
		}
		if math.IsNaN(region.Ratio) || math.IsInf(region.Ratio, 0) || region.Ratio < 0 ||
			region.Ratio > 1 {
			return fmt.Errorf("invalid ratio for %q", region.ID)
		}
	}

	if config.Mobile.ActiveRegionID != "" && !seen[config.Mobile.ActiveRegionID] {
		return fmt.Errorf(
			"mobile active region %q not present",
			config.Mobile.ActiveRegionID,
		)
	}
	return nil
}

func ParseViewportClass(value string) (ViewportClass, bool) {
	switch ViewportClass(strings.TrimSpace(value)) {
	case ViewportMobile:
		return ViewportMobile, true
	case ViewportDesktopHalf:
		return ViewportDesktopHalf, true
	case ViewportDesktopFull:
		return ViewportDesktopFull, true
	default:
		return "", false
	}
}

func (v ViewportClass) IsDesktop() bool {
	return v == ViewportDesktopHalf || v == ViewportDesktopFull
}

func normalizeViewportClass(value ViewportClass) ViewportClass {
	if parsed, ok := ParseViewportClass(string(value)); ok {
		return parsed
	}
	return ViewportDesktopFull
}

func validPage(page WorkbenchPage) bool {
	switch page {
	case WorkbenchPageAgentChat, WorkbenchPageThoughts:
		return true
	default:
		return false
	}
}

func validView(view WorkbenchView) bool {
	switch view {
	case WorkbenchViewFocus, WorkbenchViewSplit:
		return true
	default:
		return false
	}
}

func validSlot(slot WorkbenchSlot) bool {
	switch slot {
	case WorkbenchSlotNavigation, WorkbenchSlotPrimary, WorkbenchSlotContext:
		return true
	default:
		return false
	}
}

func validKind(kind RegionKind) bool {
	switch kind {
	case RegionChat,
		RegionDocument,
		RegionComments,
		RegionSections,
		RegionThoughtsTree,
		RegionPlanSidebar,
		RegionWorkspaceTopology,
		RegionDoc,
		RegionWorkflow,
		RegionRuns,
		RegionEmpty:
		return true
	default:
		return false
	}
}

func SignalKey(region WorkbenchRegion) string {
	if strings.TrimSpace(region.SignalKey) != "" {
		return SignalKeyForID(region.SignalKey)
	}
	return SignalKeyForID(region.ID)
}

func SignalKeyForID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "region"
	}
	var b strings.Builder
	capitalizeNext := false
	for i, r := range trimmed {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			capitalizeNext = b.Len() > 0
			continue
		}
		if b.Len() == 0 {
			if unicode.IsDigit(r) {
				b.WriteString("region")
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		if capitalizeNext {
			b.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "region"
	}
	return b.String()
}
