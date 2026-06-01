package markdown

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type QRSPIMetadata struct {
	Present         bool
	Stage           string
	PlanSlug        string
	PlanTitle       string
	PlanDate        string
	PlanTime        string
	PlanDir         string
	Ticket          string
	Project         string
	RelatedProjects []string
	Repository      string
	UpdatedAt       time.Time
	UpdatedBy       string
	Nav             []QRSPINavItem
	RelatedDocs     []QRSPIDocGroup
}

type QRSPINavItem struct {
	Label    string
	Href     string
	Current  bool
	Disabled bool
}

type QRSPIDocGroup struct {
	Label string
	Docs  []QRSPIDocLink
}

type QRSPIDocLink struct {
	Label   string
	Href    string
	Current bool
	Primary bool
}

func (s *Service) buildQRSPIMetadata(pageArgs *PageArgs) QRSPIMetadata {
	if pageArgs == nil || pageArgs.ViewerArgs.Frontmatter == nil {
		return QRSPIMetadata{}
	}
	fm := pageArgs.ViewerArgs.Frontmatter
	planDir := canonicalPlanDir(fm.PlanDir, pageArgs.FilePath)
	if planDir == "" {
		return QRSPIMetadata{}
	}

	slug := planSlug(planDir)
	date, clock, title := splitPlanSlug(filepath.Base(strings.Trim(planDir, "/")), slug)
	meta := QRSPIMetadata{
		Present:         true,
		Stage:           strings.TrimSpace(fm.Stage),
		PlanSlug:        slug,
		PlanTitle:       title,
		PlanDate:        date,
		PlanTime:        clock,
		PlanDir:         planDir,
		Ticket:          strings.Trim(strings.TrimSpace(fm.Ticket), `"`),
		Project:         strings.TrimSpace(fm.Project),
		RelatedProjects: fm.RelatedProjects,
		Repository:      canonicalRepositoryLabel(fm.Repository),
		UpdatedAt:       fm.LastUpdated,
		UpdatedBy:       strings.TrimSpace(fm.LastUpdatedBy),
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = fm.Date
	}
	if meta.UpdatedBy == "" {
		meta.UpdatedBy = strings.TrimSpace(fm.Researcher)
	}

	planFS := filepath.Join(s.basePath, strings.TrimPrefix(planDir, "thoughts/"))
	current := NormalizeWorkspaceDocPath(pageArgs.FilePath)
	if !strings.HasPrefix(current, "thoughts/") {
		current = "thoughts/" + current
	}
	docs := discoverQRSPIDocs(planDir, planFS, current, fm)
	meta.Nav = buildQRSPINav(docs, meta.Stage)
	meta.RelatedDocs = buildQRSPIRelatedGroups(docs)
	return meta
}

type qrspiDocs struct {
	Questions     []QRSPIDocLink
	Research      []QRSPIDocLink
	Design        *QRSPIDocLink
	ProductDesign *QRSPIDocLink
	Outline       *QRSPIDocLink
	Plan          *QRSPIDocLink
	ADRs          []QRSPIDocLink
	Reviews       []QRSPIDocLink
	Brainstorms   []QRSPIDocLink
}

func discoverQRSPIDocs(planDir, planFS, current string, fm *Frontmatter) qrspiDocs {
	var docs qrspiDocs
	docs.Questions = listMarkdownLinks(
		filepath.Join(planFS, "questions"),
		planDir+"/questions",
		current,
	)
	docs.Research = listMarkdownLinks(
		filepath.Join(planFS, "research"),
		planDir+"/research",
		current,
	)
	docs.ADRs = listMarkdownLinks(filepath.Join(planFS, "adrs"), planDir+"/adrs", current)
	docs.Reviews = listReviewLinks(
		filepath.Join(planFS, "reviews"),
		planDir+"/reviews",
		current,
	)
	docs.Brainstorms = frontmatterLinks(fm.BrainstormDocs, current)
	markPrimary(docs.ADRs, fm.RelatedADRs)

	docs.Design = existingDoc(planFS, planDir, "design.md", current)
	docs.ProductDesign = existingDoc(planFS, planDir, "design-product.md", current)
	docs.Outline = existingDoc(planFS, planDir, "outline.md", current)
	docs.Plan = existingDoc(planFS, planDir, "plan.md", current)
	return docs
}

func canonicalPlanDir(frontmatterPlanDir, docPath string) string {
	planDir := strings.Trim(strings.TrimSpace(frontmatterPlanDir), `"`)
	if planDir != "" {
		return strings.Trim(planDir, "/")
	}
	path := NormalizeWorkspaceDocPath(docPath)
	if !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + path
	}
	idx := strings.Index(path, "/plans/")
	if idx < 0 {
		return ""
	}
	rest := path[idx+len("/plans/"):]
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return ""
	}
	return path[:idx+len("/plans/")+len(parts[0])]
}

func planSlug(planDir string) string {
	base := filepath.Base(strings.Trim(planDir, "/"))
	if len(base) > len("2006-01-02_15-04-05_") && base[4] == '-' && base[10] == '_' {
		return base[len("2006-01-02_15-04-05_"):]
	}
	return base
}

func splitPlanSlug(base, slug string) (string, string, string) {
	if len(base) > len("2006-01-02_15-04-05_") && base[4] == '-' && base[10] == '_' {
		return base[:10], strings.ReplaceAll(base[11:19], "-", ":"), prettyDocLabel(slug)
	}
	return "", "", prettyDocLabel(slug)
}

func directoryTitle(args *DirectoryArgs) string {
	if args == nil || strings.TrimSpace(args.Path) == "" {
		return "Documentation Root"
	}
	return GetDisplayPath(args.Path)
}

func canonicalRepositoryLabel(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	base := filepath.Base(repo)
	if strings.HasPrefix(base, "monorepo-") || strings.HasPrefix(base, "monorepo_") {
		return "monorepo"
	}
	if strings.HasPrefix(base, "cn-agents-") || strings.HasPrefix(base, "cn-agents_") {
		return "cn-agents"
	}
	return base
}

func existingDoc(planFS, planDir, name, current string) *QRSPIDocLink {
	if _, err := os.Stat(filepath.Join(planFS, name)); err != nil {
		return nil
	}
	link := QRSPIDocLink{
		Label: labelForDoc(name),
		Href:  thoughtsHref(planDir + "/" + name),
	}
	link.Current = normalizeQRSPIDocPath(
		planDir+"/"+name,
	) == normalizeQRSPIDocPath(
		current,
	)
	return &link
}

func listMarkdownLinks(dirFS, dirPath, current string) []QRSPIDocLink {
	entries, err := os.ReadDir(dirFS)
	if err != nil {
		return nil
	}
	links := make([]QRSPIDocLink, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := dirPath + "/" + entry.Name()
		links = append(
			links,
			QRSPIDocLink{
				Label:   labelForDoc(entry.Name()),
				Href:    thoughtsHref(path),
				Current: normalizeQRSPIDocPath(path) == normalizeQRSPIDocPath(current),
			},
		)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Href < links[j].Href })
	return links
}

func listReviewLinks(dirFS, dirPath, current string) []QRSPIDocLink {
	entries, err := os.ReadDir(dirFS)
	if err != nil {
		return nil
	}
	links := make([]QRSPIDocLink, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := dirPath + "/" + entry.Name() + "/review.md"
		if _, err := os.Stat(
			filepath.Join(dirFS, entry.Name(), "review.md"),
		); err != nil {
			continue
		}
		links = append(
			links,
			QRSPIDocLink{
				Label:   labelForDoc(entry.Name()),
				Href:    thoughtsHref(path),
				Current: normalizeQRSPIDocPath(path) == normalizeQRSPIDocPath(current),
			},
		)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Href < links[j].Href })
	return links
}

func frontmatterLinks(paths []string, current string) []QRSPIDocLink {
	links := make([]QRSPIDocLink, 0, len(paths))
	for _, path := range paths {
		path = strings.Trim(strings.TrimSpace(path), `"`)
		if path == "" {
			continue
		}
		links = append(
			links,
			QRSPIDocLink{
				Label:   labelForDoc(filepath.Base(path)),
				Href:    thoughtsHref(path),
				Current: normalizeQRSPIDocPath(path) == normalizeQRSPIDocPath(current),
				Primary: true,
			},
		)
	}
	return links
}

func markPrimary(links []QRSPIDocLink, primaryPaths []string) {
	primary := map[string]bool{}
	for _, path := range primaryPaths {
		primary[normalizeQRSPIDocPath(path)] = true
	}
	for i := range links {
		links[i].Primary = primary[normalizeQRSPIDocPath(strings.TrimPrefix(links[i].Href, "/thoughts/"))]
	}
}

func buildQRSPINav(docs qrspiDocs, currentStage string) []QRSPINavItem {
	return []QRSPINavItem{
		navItem("Questions", mostRecent(docs.Questions), currentStage == "question"),
		navItem("Research", mostRecent(docs.Research), currentStage == "research"),
		navItem("Design", docs.Design, currentStage == "design"),
		navItem("Product Design", docs.ProductDesign, currentStage == "design-product"),
		navItem("Outline", docs.Outline, currentStage == "outline"),
		navItem("Plan", docs.Plan, currentStage == "plan"),
		navItem(
			"Reviews",
			mostRecent(docs.Reviews),
			strings.HasPrefix(currentStage, "review"),
		),
	}
}

func navItem(label string, link *QRSPIDocLink, current bool) QRSPINavItem {
	item := QRSPINavItem{Label: label, Current: current}
	if link == nil {
		item.Disabled = true
		return item
	}
	item.Href = link.Href
	item.Current = item.Current || link.Current
	return item
}

func mostRecent(links []QRSPIDocLink) *QRSPIDocLink {
	if len(links) == 0 {
		return nil
	}
	return &links[len(links)-1]
}

func buildQRSPIRelatedGroups(docs qrspiDocs) []QRSPIDocGroup {
	groups := []QRSPIDocGroup{}
	add := func(label string, links []QRSPIDocLink) {
		if len(links) > 0 {
			groups = append(groups, QRSPIDocGroup{Label: label, Docs: links})
		}
	}
	canonical := []QRSPIDocLink{}
	for _, link := range []*QRSPIDocLink{docs.Design, docs.ProductDesign, docs.Outline, docs.Plan} {
		if link != nil {
			canonical = append(canonical, *link)
		}
	}
	add("Primary docs", canonical)
	add("Questions", docs.Questions)
	add("Research", docs.Research)
	add("ADRs", docs.ADRs)
	add("Reviews", docs.Reviews)
	add("Brainstorms", docs.Brainstorms)
	return groups
}

func normalizeQRSPIDocPath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), `"`)
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + path
	}
	return path
}

func labelForDoc(name string) string {
	name = strings.TrimSuffix(name, ".md")
	if len(name) > 20 && name[4] == '-' && strings.Contains(name, "_") {
		parts := strings.SplitN(name, "_", 2)
		name = parts[len(parts)-1]
	}
	return prettyDocLabel(name)
}

func prettyDocLabel(name string) string {
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "Document"
	}
	return strings.Title(name)
}
