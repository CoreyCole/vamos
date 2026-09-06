package workbench

import "strings"

// Workbench V2 cross-document View Transition chrome name map — single source of truth.
// Hand CSS in static/css/index.css stays authoritative for cascade; this table drives
// Story expected inventories, CSS presence asserts, and the docs name-map section.
// Defer CSS codegen.

// VTChromeMedia scopes when a selector carries its Name.
type VTChromeMedia string

const (
	// VTChromeMediaAll — name applies at all viewports.
	VTChromeMediaAll VTChromeMedia = "all"
	// VTChromeMediaMaxMD — named only under max-width 767px; must be none on md+.
	VTChromeMediaMaxMD VTChromeMedia = "max-md"
	// VTChromeMediaDesktop — desktop chrome (Tailwind max-md:hidden); still named in CSS globally.
	VTChromeMediaDesktop VTChromeMedia = "desktop"
)

// VTChromeName is one chrome / parent entry in the Workbench V2 VT name map.
type VTChromeName struct {
	Selector    string
	Name        string
	Media       VTChromeMedia
	Class       string
	Freeze      bool
	MustBeNone  bool
	ProbeKey    string
	UnnameUnder string
	DocsMedia   string
}

// WorkbenchV2ChromeNames is the canonical chrome name map for Workbench V2 sibling GETs.
func WorkbenchV2ChromeNames() []VTChromeName {
	const chrome = "workbench-chrome"
	return []VTChromeName{
		{
			Selector: "#app-header", Name: "app-header", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "header", DocsMedia: "all",
		},
		{
			Selector: "#workbench-mobile-tabs", Name: "workbench-mobile-tabs", Media: VTChromeMediaMaxMD,
			Class: chrome, Freeze: true, ProbeKey: "tabs",
			DocsMedia: "named `@media (max-width: 767px)` only; `view-transition-name: none` on `md+`",
		},
		{
			Selector: "#workbench-v2-threads", Name: "workbench-v2-threads", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "threads", DocsMedia: "all",
		},
		{
			Selector: "#workbench-v2-threads-reopen", Name: "workbench-v2-threads-reopen", Media: VTChromeMediaDesktop,
			Class: chrome, Freeze: true, ProbeKey: "threadsReopen",
			DocsMedia: "desktop (`max-md:hidden`)",
		},
		{
			Selector: "#workbench-v2-chat", Name: "workbench-v2-chat", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "chat",
			UnnameUnder: `html[data-wb2-vt-nav="thread-switch"] #workbench-v2-chat`,
			DocsMedia:   "named by default (sibling artifact freeze); `view-transition-name: none` while `html[data-wb2-vt-nav=thread-switch]` (thread→thread only)",
		},
		{
			Selector: "#workbench-v2-comments", Name: "workbench-v2-comments", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "comments", DocsMedia: "all",
		},
		{
			Selector: "#thread-artifact-path-header", Name: "thread-artifact-path-header", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "path", DocsMedia: "all",
		},
		{
			Selector: "#thread-artifact-browser", Name: "thread-artifact-browser", Media: VTChromeMediaAll,
			Class: chrome, Freeze: true, ProbeKey: "browser", DocsMedia: "all",
		},
		{
			Selector: "#thread-artifact-document", Name: "thread-artifact-document", Media: VTChromeMediaAll,
			Freeze: false, ProbeKey: "document", DocsMedia: "all",
		},
		{
			Selector: "#workbench-root", Name: "none", Media: VTChromeMediaAll,
			MustBeNone: true, ProbeKey: "root", DocsMedia: "all",
		},
		{
			Selector: "#workbench-regions", Name: "none", Media: VTChromeMediaAll,
			MustBeNone: true, ProbeKey: "regions", DocsMedia: "all",
		},
		{
			Selector: "#workbench-v2-artifact", Name: "none", Media: VTChromeMediaAll,
			MustBeNone: true, ProbeKey: "artifact", DocsMedia: "all",
		},
		{
			Selector: "#thread-artifact-pane", Name: "none", Media: VTChromeMediaAll,
			MustBeNone: true, DocsMedia: "all",
		},
	}
}

// ExpectedComputedName returns the view-transition-name Story probes should see.
// desktop=true means md+ (min-width 768). threadSwitch reflects html[data-wb2-vt-nav=thread-switch].
func (e VTChromeName) ExpectedComputedName(desktop, threadSwitch bool) string {
	if e.MustBeNone || e.Name == "none" {
		return "none"
	}
	if e.UnnameUnder != "" && threadSwitch {
		return "none"
	}
	if e.Media == VTChromeMediaMaxMD && desktop {
		return "none"
	}
	return e.Name
}

// DesktopSiblingDocNameInventory is the Playwright probe map for desktop sibling artifact GETs.
func DesktopSiblingDocNameInventory() map[string]string {
	out := make(map[string]string)
	for _, e := range WorkbenchV2ChromeNames() {
		if e.ProbeKey == "" {
			continue
		}
		out[e.ProbeKey] = e.ExpectedComputedName(true, false)
	}
	return out
}

// MobileSiblingDocNameInventory is the Playwright probe map for mobile under-tabs sibling GETs.
func MobileSiblingDocNameInventory() map[string]string {
	out := make(map[string]string)
	for _, e := range WorkbenchV2ChromeNames() {
		switch e.ProbeKey {
		case "tabs", "path", "browser", "chat", "document", "artifact":
			out[e.ProbeKey] = e.ExpectedComputedName(false, false)
		}
	}
	return out
}

// CSSPresenceSnippets returns substrings that must appear in static/css/index.css.
func CSSPresenceSnippets() []string {
	var out []string
	for _, e := range WorkbenchV2ChromeNames() {
		if e.MustBeNone {
			out = append(out, e.Selector)
			continue
		}
		out = append(out, e.Selector+" {")
		if e.Media == VTChromeMediaMaxMD {
			out = append(out,
				"@media (max-width: 767px) {",
				"view-transition-name: "+e.Name+";",
				"@media (min-width: 768px) {",
				"view-transition-name: none;",
			)
		} else {
			out = append(out, "view-transition-name: "+e.Name+";")
		}
		if e.Class != "" {
			out = append(out, "view-transition-class: "+e.Class+";")
		}
		if e.Freeze && e.Name != "none" {
			out = append(out,
				"::view-transition-old("+e.Name+")",
				"::view-transition-new("+e.Name+")",
				"::view-transition-group("+e.Name+")",
			)
		}
		if e.UnnameUnder != "" {
			out = append(out, e.UnnameUnder+" {", "view-transition-name: none;")
		}
	}
	return out
}

// DocsNameMapMarkdownTable returns the docs name-map markdown table (including header).
func DocsNameMapMarkdownTable() string {
	var b strings.Builder
	b.WriteString("| selector | name | class | media |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	var noneParts []string
	for _, e := range WorkbenchV2ChromeNames() {
		if e.MustBeNone {
			noneParts = append(noneParts, "`"+e.Selector+"`")
			continue
		}
		nameCell := "`" + e.Name + "`"
		switch {
		case e.UnnameUnder != "":
			nameCell = "`" + e.Name + "` / `none` on thread→thread"
		case e.Media == VTChromeMediaMaxMD:
			nameCell = "`" + e.Name + "` / `none`"
		}
		classCell := "—"
		if e.Class != "" {
			classCell = "`" + e.Class + "`"
		}
		b.WriteString("| `")
		b.WriteString(e.Selector)
		b.WriteString("` | ")
		b.WriteString(nameCell)
		b.WriteString(" | ")
		b.WriteString(classCell)
		b.WriteString(" | ")
		b.WriteString(e.DocsMedia)
		b.WriteString(" |\n")
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(noneParts, " / "))
	b.WriteString(" | `none` | — | all |\n")
	return b.String()
}
