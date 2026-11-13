package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

type App struct {
	win              *mygo.Window
	Projects         []string
	ActiveProjectIdx int
	ActiveProject    string
	NewProjectName   string
	Articles         []*Article
	ActiveIdx        int
	LastActiveIdx    int

	ArticleListState ui.ListState
	ChosenArticles   ui.Selection[int]

	SidebarWidth  float32
	ReaderPercent float32

	WindowMaximized bool
	TypographyOpen  bool

	SummaryDoc         *richtext.Document
	LastSummaryText    string
	ShowMarkdownRender bool
	mdOpts             markdown.Options

	LastSavedAt   string
	IsDirty       bool
	debounceTimer *time.Timer
	saveMu        sync.Mutex

	Settings         *AppSettings
	AvailableFonts   []string
	SelectedFont     string
	FontPickerText   string
	FontPickerOpen   bool
	FontSizeOptions  []string
	SelectedFontSize string

	IsDarkMode   bool
	SelectedDate time.Time

	// Dialog & Modal State
	ShowExportModal          bool
	ExportDirectory          string
	ShowDeleteProjectConfirm bool
	ShowDeleteArticleConfirm bool
	ShowNewProjectModal      bool
	ShowRenameProjectModal   bool
	RenameProjectName        string

	// Summary Matcher Modal State
	ShowBatchSummaryModal bool
	BatchPastedMarkdown   string
	BatchMatchError       string
	BatchIncludeExisting  bool
	ParsedSummaryMatches  []ParsedSummaryBlock
	UnmatchedOptions      []string
	BatchMatchingStep     int

	// Per-article Local AI Summary State
	AISummaryArticle int // index being summarized, -1 when idle
	AISummaryRunning bool
	AISummaryOutput  string
	AISummaryError   string
	LocalLLMPhase    string // off, checking, starting, ready, stopping, error
	LocalLLMError    string
	localLLMRunID    int
	aiCancel         context.CancelFunc
	aiRunID          int

	// Selection Explanation State. The last selection's explanation lives
	// in memory only and is shown in the pane where the text was selected.
	SelectionText          string
	SelectionSentence      string
	SelectionSource        string // "summary" or "fulltext"
	SelectionExplanation   string
	SelectionError         string
	SelectionRunning       bool
	SelectionClosed        bool
	selectionKey           string
	selectionExplainedKey  string
	selectionExplainTimer  *time.Timer
	selectionExplainCancel context.CancelFunc
	selectionExplainRunID  int

	// Settings Modal State
	ShowSettingsModal   bool
	SettingsDraftPrompt string
}

func NewApp() *App {
	settings := LoadSettings()
	if settings.IsDarkMode {
		mygo.Theme.SetSource(mygo.ThemeDark)
	} else {
		mygo.Theme.SetSource(mygo.ThemeLight)
	}

	existingProjects := DiscoverProjects()
	initialProject := settings.LastProject
	if initialProject == "" || !contains(existingProjects, initialProject) {
		if len(existingProjects) > 0 {
			initialProject = existingProjects[0]
		}
	}

	allFonts := GetLinuxSystemFonts()
	fonts := FilterCleanReadingFonts(allFonts)

	defaultFont := settings.FontFamily
	if defaultFont == "" || !contains(fonts, defaultFont) {
		defaultFont = "Sans"
		for _, f := range fonts {
			if strings.Contains(strings.ToLower(f), "sans") {
				defaultFont = f
				break
			}
		}
	}

	fontSize := settings.FontSize
	if fontSize == "" {
		fontSize = "16"
	}

	fontSizeOptions := []string{"12", "13", "14", "15", "16", "17", "18", "20", "22", "24", "28", "32"}

	app := &App{
		Projects:                 existingProjects,
		Articles:                 []*Article{},
		ActiveIdx:                0,
		LastActiveIdx:            -1,
		SidebarWidth:             292,
		ReaderPercent:            54,
		Settings:                 settings,
		AvailableFonts:           fonts,
		SelectedFont:             defaultFont,
		FontPickerText:           defaultFont,
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         fontSize,
		IsDarkMode:               settings.IsDarkMode,
		SelectedDate:             time.Now(),
		ExportDirectory:          settings.ExportDirectory,
		ShowExportModal:          false,
		ShowDeleteProjectConfirm: false,
		ShowDeleteArticleConfirm: false,
		ShowNewProjectModal:      false,
		ShowRenameProjectModal:   false,
		ShowBatchSummaryModal:    false,
		AISummaryArticle:         -1,
		LocalLLMPhase:            "checking",
		ShowSettingsModal:        false,
		// The summary's preview shares its selection with the box around
		// it, whose menu adds Copy Summary as Markdown; the window draws
		// every menu in its own look (Context.SetMenuPresentation).
		mdOpts: markdown.Options{ShareSelection: true},
	}

	app.ArticleListState.Selected = &app.ActiveIdx
	app.ArticleListState.Selection = &app.ChosenArticles
	app.ArticleListState.Label = func(i int) string {
		if i >= 0 && i < len(app.Articles) {
			return app.Articles[i].Title
		}
		return ""
	}

	app.initSummaryDoc("")

	if initialProject != "" {
		app.SwitchProject(initialProject)
	}

	return app
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func FilterCleanReadingFonts(rawFonts []string) []string {
	var clean []string
	for _, f := range rawFonts {
		lower := strings.ToLower(f)
		if strings.Contains(lower, "emoji") || strings.Contains(lower, "icons") ||
			strings.Contains(lower, "symbols") || strings.Contains(lower, "music") ||
			strings.Contains(lower, "braille") || strings.Contains(lower, "math") {
			continue
		}
		// Noto Sans and Noto Serif have a family for each script and
		// weight, but their Latin glyphs are the base family's, so they
		// only fill the picker with near duplicates.
		if isNotoVariant(lower) {
			continue
		}
		if strings.Contains(lower, "sans") || strings.Contains(lower, "serif") ||
			strings.Contains(lower, "mono") || strings.Contains(lower, "roboto") ||
			strings.Contains(lower, "inter") || strings.Contains(lower, "ubuntu") ||
			strings.Contains(lower, "cantarell") || strings.Contains(lower, "dejavu") ||
			strings.Contains(lower, "liberation") || strings.Contains(lower, "arial") {
			clean = append(clean, f)
		}
	}
	if len(clean) > 0 {
		return clean
	}
	return rawFonts
}

// isNotoVariant reports whether lower names a per-script or per-weight cut
// of Noto Sans or Noto Serif. The base families, and their distinct Mono
// and Display designs, are not variants.
func isNotoVariant(lower string) bool {
	switch lower {
	case "noto sans", "noto sans mono", "noto sans display",
		"noto serif", "noto serif display":
		return false
	}
	return strings.HasPrefix(lower, "noto sans ") || strings.HasPrefix(lower, "noto serif ")
}

func (a *App) CurrentFontSize() float32 {
	sz, err := strconv.Atoi(a.SelectedFontSize)
	if err != nil || sz <= 0 {
		return 16
	}
	return float32(sz)
}

func (a *App) CurrentFontFamily() string {
	if a.SelectedFont != "" {
		return a.SelectedFont
	}
	return "Sans"
}

func (a *App) CurrentArticle() *Article {
	if a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		return a.Articles[a.ActiveIdx]
	}
	return nil
}

func (a *App) updateActiveProjectIdx() {
	for i, p := range a.Projects {
		if p == a.ActiveProject {
			a.ActiveProjectIdx = i
			return
		}
	}
	a.ActiveProjectIdx = 0
}

func (a *App) initSummaryDoc(source string) {
	a.SummaryDoc = richtext.NewDocument(richtext.ParseMarkdown(source))
	// Keep the baseline in the document's canonical Markdown, so the first
	// edit is compared against what the rich-text editor round-trips.
	a.LastSummaryText = a.SummaryDoc.Text().Markdown()
}

func (a *App) currentSummaryMarkdown() string {
	if a.SummaryDoc == nil {
		return ""
	}
	return a.SummaryDoc.Text().Markdown()
}

func (a *App) SelectArticle(newIdx int) {
	if newIdx < 0 || newIdx >= len(a.Articles) {
		return
	}

	if a.AISummaryRunning && a.AISummaryArticle != newIdx {
		a.cancelArticleAISummary()
	}
	a.AISummaryError = ""
	a.clearSelectionExplanation()

	if a.LastActiveIdx >= 0 && a.LastActiveIdx < len(a.Articles) && a.LastActiveIdx != newIdx {
		if a.SummaryDoc != nil && !a.ShowMarkdownRender {
			a.Articles[a.LastActiveIdx].Summary = a.currentSummaryMarkdown()
		}
	}

	a.ActiveIdx = newIdx
	a.LastActiveIdx = newIdx

	cur := a.Articles[newIdx]
	a.initSummaryDoc(cur.Summary)

	if strings.TrimSpace(cur.Summary) != "" {
		a.ShowMarkdownRender = true
	} else {
		a.ShowMarkdownRender = false
	}

	a.SaveProjectSelection()
}

func (a *App) SaveProjectSelection() {
	if a.ActiveProject == "" {
		return
	}
	var selected []int
	for idx := range a.ChosenArticles.All() {
		selected = append(selected, idx)
	}
	if a.Settings.ProjectStates == nil {
		a.Settings.ProjectStates = make(map[string]ProjectSelectionState)
	}
	a.Settings.ProjectStates[a.ActiveProject] = ProjectSelectionState{
		ActiveIdx: a.ActiveIdx,
		Selected:  selected,
	}
	a.Settings.LastProject = a.ActiveProject
	_ = a.Settings.Save()
}

func (a *App) RestoreProjectSelection(project string) {
	if st, ok := a.Settings.ProjectStates[project]; ok {
		a.ChosenArticles.Clear()
		for _, idx := range st.Selected {
			if idx >= 0 && idx < len(a.Articles) {
				a.ChosenArticles.Add(idx)
			}
		}
		if st.ActiveIdx >= 0 && st.ActiveIdx < len(a.Articles) {
			a.SelectArticle(st.ActiveIdx)
			return
		}
	}
	a.ChosenArticles.Clear()
	if len(a.Articles) > 0 {
		a.SelectArticle(0)
	}
}

func (a *App) TriggerDebouncedSave() {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()

	a.IsDirty = true
	if a.debounceTimer != nil {
		a.debounceTimer.Stop()
	}

	a.debounceTimer = time.AfterFunc(3*time.Second, func() {
		a.SaveNow()
	})
}

func (a *App) SaveNow() {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()

	if a.debounceTimer != nil {
		a.debounceTimer.Stop()
	}

	cur := a.CurrentArticle()
	if cur != nil && a.SummaryDoc != nil && !a.ShowMarkdownRender {
		cur.Summary = a.currentSummaryMarkdown()
	}

	if a.ActiveProject != "" {
		_ = SaveProjectState(a.ActiveProject, a.Articles)
		a.LastSavedAt = time.Now().Format("15:04:05")
		a.IsDirty = false
	}
	a.SaveProjectSelection()
}

func (a *App) SwitchProject(newProject string) {
	if newProject == "" {
		return
	}
	a.cancelArticleAISummary()
	a.clearSelectionExplanation()

	if a.ActiveProject != "" && a.ActiveProject != newProject && a.IsDirty {
		_ = SaveProjectState(a.ActiveProject, a.Articles)
		a.IsDirty = false
	}

	a.ActiveProject = newProject
	a.updateActiveProjectIdx()

	loaded, err := LoadProjectState(newProject)
	if err == nil {
		a.Articles = loaded
	} else {
		a.Articles = []*Article{}
	}

	a.LastActiveIdx = -1
	a.RestoreProjectSelection(newProject)

	a.LastSavedAt = time.Now().Format("15:04:05")
	a.IsDirty = false
}

func (a *App) CreateProject(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for _, p := range a.Projects {
		if p == name {
			a.SwitchProject(name)
			a.NewProjectName = ""
			a.ShowNewProjectModal = false
			return
		}
	}
	a.Projects = append(a.Projects, name)
	a.SwitchProject(name)

	// Immediately save the project file and update settings so DiscoverProjects() finds it on next launch
	_ = SaveProjectState(name, a.Articles)
	a.SaveProjectSelection()

	a.NewProjectName = ""
	a.ShowNewProjectModal = false
}

func (a *App) RenameActiveProject(newName string) {
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == a.ActiveProject {
		a.ShowRenameProjectModal = false
		return
	}

	oldName := a.ActiveProject
	for _, p := range a.Projects {
		if p == newName {
			return
		}
	}

	oldPath := ProjectFilePath(oldName)
	newPath := ProjectFilePath(newName)
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.Rename(oldPath, newPath)
	} else {
		_ = SaveProjectState(newName, a.Articles)
	}

	for i, p := range a.Projects {
		if p == oldName {
			a.Projects[i] = newName
			break
		}
	}

	if st, ok := a.Settings.ProjectStates[oldName]; ok {
		a.Settings.ProjectStates[newName] = st
		delete(a.Settings.ProjectStates, oldName)
	}
	a.ActiveProject = newName
	a.updateActiveProjectIdx()
	a.Settings.LastProject = newName
	_ = a.Settings.Save()

	a.ShowRenameProjectModal = false
}

func (a *App) DeleteActiveProject() {
	if a.ActiveProject == "" {
		return
	}
	a.cancelArticleAISummary()
	a.clearSelectionExplanation()

	target := a.ActiveProject
	_ = RemoveProjectState(target)
	delete(a.Settings.ProjectStates, target)

	var updated []string
	for _, p := range a.Projects {
		if p != target {
			updated = append(updated, p)
		}
	}
	a.Projects = updated
	a.Articles = []*Article{}
	a.ActiveIdx = 0
	a.LastActiveIdx = -1
	a.ChosenArticles.Clear()
	a.initSummaryDoc("")

	if len(a.Projects) > 0 {
		a.SwitchProject(a.Projects[0])
	} else {
		a.ActiveProject = ""
		a.LastSavedAt = ""
		a.IsDirty = false
		a.Settings.LastProject = ""
		_ = a.Settings.Save()
	}
	a.ShowDeleteProjectConfirm = false
}

func (a *App) ConfirmDeleteChosenArticles() {
	a.cancelArticleAISummary()
	a.clearSelectionExplanation()
	if a.ChosenArticles.Len() == 0 && a.CurrentArticle() != nil {
		idx := a.ActiveIdx
		a.Articles = append(a.Articles[:idx], a.Articles[idx+1:]...)
	} else {
		toDelete := make(map[int]bool)
		for idx := range a.ChosenArticles.All() {
			toDelete[idx] = true
		}
		var remaining []*Article
		for i, art := range a.Articles {
			if !toDelete[i] {
				remaining = append(remaining, art)
			}
		}
		a.Articles = remaining
	}

	a.ChosenArticles.Clear()
	if a.ActiveIdx >= len(a.Articles) {
		a.ActiveIdx = len(a.Articles) - 1
	}
	if a.ActiveIdx < 0 {
		a.ActiveIdx = 0
	}
	a.LastActiveIdx = -1
	a.SelectArticle(a.ActiveIdx)
	a.SaveNow()
	a.ShowDeleteArticleConfirm = false
}

// writeClipboardText puts plain text on the clipboard. golang.design/x/clipboard
// advertised "image/png" next to the text target even for text-only writes, so
// web apps pasted the text as a broken image. MyGo's clipboard only offers the
// text representations it was given.
func writeClipboardText(text string) {
	mygo.Clipboard.WriteText(text)
}

func (a *App) CopySelectedTitles() {
	var titles []string
	if a.ChosenArticles.Len() > 0 {
		for idx := range a.ChosenArticles.All() {
			if idx >= 0 && idx < len(a.Articles) {
				titles = append(titles, a.Articles[idx].Title)
			}
		}
	} else if a.CurrentArticle() != nil {
		titles = append(titles, a.CurrentArticle().Title)
	}
	if len(titles) > 0 {
		writeClipboardText(strings.Join(titles, "\n"))
	}
}

func (a *App) CopySelectedContents() {
	var sb strings.Builder
	hasAny := false
	if a.ChosenArticles.Len() > 0 {
		for idx := range a.ChosenArticles.All() {
			if idx >= 0 && idx < len(a.Articles) {
				art := a.Articles[idx]
				sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", art.Title, art.FullText))
				hasAny = true
			}
		}
	}
	if !hasAny && a.CurrentArticle() != nil {
		art := a.CurrentArticle()
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", art.Title, art.FullText))
	}
	if sb.Len() > 0 {
		writeClipboardText(sb.String())
	}
}

func (a *App) CopySelectedSummaries() {
	var sb strings.Builder
	hasAny := false
	if a.ChosenArticles.Len() > 0 {
		for idx := range a.ChosenArticles.All() {
			if idx >= 0 && idx < len(a.Articles) {
				art := a.Articles[idx]
				if IsSummaryEmpty(art.Summary) {
					continue
				}
				sb.WriteString(fmt.Sprintf("# %s\n\n%s\n\n", art.Title, strings.TrimSpace(art.Summary)))
				hasAny = true
			}
		}
	}
	if !hasAny && a.CurrentArticle() != nil {
		art := a.CurrentArticle()
		if !IsSummaryEmpty(art.Summary) {
			sb.WriteString(fmt.Sprintf("# %s\n\n%s\n\n", art.Title, strings.TrimSpace(art.Summary)))
		}
	}
	if sb.Len() > 0 {
		writeClipboardText(sb.String())
	}
}

// CanCopySummaries reports whether any article CopySelectedSummaries would
// copy has a summary.
func (a *App) CanCopySummaries() bool {
	if a.ChosenArticles.Len() == 0 {
		cur := a.CurrentArticle()
		return cur != nil && !IsSummaryEmpty(cur.Summary)
	}
	for idx := range a.ChosenArticles.All() {
		if idx >= 0 && idx < len(a.Articles) && !IsSummaryEmpty(a.Articles[idx].Summary) {
			return true
		}
	}
	return false
}

// SelectAllArticles chooses every article of the project.
func (a *App) SelectAllArticles() {
	for i := range a.Articles {
		a.ChosenArticles.Add(i)
	}
	a.SaveProjectSelection()
}

// ClearArticleSelection chooses no article.
func (a *App) ClearArticleSelection() {
	a.ChosenArticles.Clear()
	a.SaveProjectSelection()
}

// buildArticleMenu fills the menu of an article in the list, which does what
// the buttons above the list do, on the articles chosen. Right-clicking a
// row not chosen chooses it first, as the list does for Finder.
func (a *App) buildArticleMenu(m *ui.Menu) {
	count := a.ChosenArticles.Len()
	if count == 0 {
		count = 1
	}

	if m.Item("Copy Titles").Chosen() {
		a.CopySelectedTitles()
	}
	if m.Item("Copy Full Text").Chosen() {
		a.CopySelectedContents()
	}
	if m.Item("Copy Summaries").Disabled(!a.CanCopySummaries()).Chosen() {
		a.CopySelectedSummaries()
	}

	m.Separator()

	if m.Item("Select All").Disabled(len(a.Articles) == 0).Chosen() {
		a.SelectAllArticles()
	}
	if m.Item("Clear Selection").Disabled(a.ChosenArticles.Len() == 0).Chosen() {
		a.ClearArticleSelection()
	}

	m.Separator()

	del := "Delete Article…"
	if count > 1 {
		del = fmt.Sprintf("Delete %d Articles…", count)
	}
	if m.Item(del).Danger().Chosen() {
		a.ShowDeleteArticleConfirm = true
	}
}

// Native Dialogs: run in goroutines, update UI via win.Update()
func (a *App) ImportHTMLFiles() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "Import HTML Article(s)",
			Filters:  []mygo.FileFilter{{Name: "HTML Articles", Extensions: []string{"html", "htm"}, MIMETypes: []string{"text/html"}}},
			Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}

		var parsed []*Article
		for _, f := range paths {
			art, err := ExtractArticleFromHTML(f)
			if err == nil {
				parsed = append(parsed, art)
			}
		}

		if len(parsed) > 0 {
			a.win.Update(func() {
				if a.ActiveProject == "" {
					base := filepath.Base(paths[0])
					a.CreateProject(strings.TrimSuffix(base, filepath.Ext(base)))
				}
				a.Articles = append(a.Articles, parsed...)
				a.SelectArticle(a.ActiveIdx)
				a.SaveNow()
			})
			debug.FreeOSMemory()
		}
	}()
}

func (a *App) ImportMarkdownFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "Import Markdown Rollup",
			Filters:  []mygo.FileFilter{{Name: "Markdown Files", Extensions: []string{"md", "markdown"}, MIMETypes: []string{"text/markdown", "text/x-markdown"}}},
			Multiple: false,
		})
		if err != nil || len(paths) == 0 {
			return
		}

		data, err := os.ReadFile(paths[0])
		if err != nil {
			return
		}

		parsed, err := ParseMarkdownRollup(string(data))
		if err != nil || len(parsed) == 0 {
			return
		}

		a.win.Update(func() {
			if a.ActiveProject == "" {
				base := filepath.Base(paths[0])
				projName := strings.TrimSuffix(base, filepath.Ext(base))
				parts := strings.SplitN(projName, " ", 2)
				if len(parts) == 2 && len(parts[0]) == 10 && strings.Count(parts[0], "-") == 2 {
					projName = parts[1]
				}
				a.CreateProject(projName)
			}
			a.Articles = append(a.Articles, parsed...)
			a.SelectArticle(a.ActiveIdx)
			a.SaveNow()
		})
		debug.FreeOSMemory()
	}()
}

func (a *App) BrowseExportDirectory() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "Select Export Directory",
			Directory: true,
		})
		if err == nil && len(paths) > 0 {
			a.win.Update(func() {
				a.ExportDirectory = paths[0]
				a.Settings.ExportDirectory = paths[0]
				_ = a.Settings.Save()
			})
		}
	}()
}

func (a *App) PerformExportTo(outPath string) {
	if a.ActiveProject == "" || strings.TrimSpace(outPath) == "" {
		a.ShowExportModal = false
		return
	}

	cur := a.CurrentArticle()
	if cur != nil && a.SummaryDoc != nil && !a.ShowMarkdownRender {
		cur.Summary = a.currentSummaryMarkdown()
	}

	md := SerializeToMarkdown(a.Articles)
	_ = os.MkdirAll(filepath.Dir(outPath), 0755)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	a.SaveNow()
	a.Settings.ExportDirectory = a.ExportDirectory
	_ = a.Settings.Save()
	a.ShowExportModal = false
}

func (a *App) ToggleTheme() {
	a.IsDarkMode = !a.IsDarkMode
	a.Settings.IsDarkMode = a.IsDarkMode
	_ = a.Settings.Save()

	if a.IsDarkMode {
		mygo.Theme.SetSource(mygo.ThemeDark)
	} else {
		mygo.Theme.SetSource(mygo.ThemeLight)
	}
}

// Batch AI Summary Matcher
func (a *App) OpenBatchSummaryModal() {
	a.BatchPastedMarkdown = ""
	a.BatchMatchError = ""
	a.ParsedSummaryMatches = nil
	a.UnmatchedOptions = nil
	a.BatchMatchingStep = 0

	emptyCount := 0
	for _, art := range a.Articles {
		if IsSummaryEmpty(art.Summary) {
			emptyCount++
		}
	}
	a.BatchIncludeExisting = (emptyCount == 0)
	a.ShowBatchSummaryModal = true
}

func (a *App) AnalyzeBatchMarkdown() {
	a.BatchMatchError = ""

	if strings.TrimSpace(a.BatchPastedMarkdown) == "" {
		a.BatchMatchError = "Please paste some markdown summary text first."
		return
	}

	if len(a.Articles) == 0 {
		a.BatchMatchError = "The active project has no articles to match against."
		return
	}

	emptyCount := 0
	for _, art := range a.Articles {
		if IsSummaryEmpty(art.Summary) {
			emptyCount++
		}
	}

	if emptyCount == 0 {
		a.BatchIncludeExisting = true
	}

	blocks := ParsePastedSummaries(a.BatchPastedMarkdown)
	if len(blocks) == 0 {
		a.BatchMatchError = "Could not parse any summaries from the text. Make sure sections have titles."
		return
	}

	matched, options := MatchSummariesToArticles(blocks, a.Articles, a.BatchIncludeExisting)
	a.ParsedSummaryMatches = matched
	a.UnmatchedOptions = options
	a.BatchMatchingStep = 1
}

func (a *App) ApplyBatchMatches() {
	a.applySummaryMatches(a.ParsedSummaryMatches)
	a.ShowBatchSummaryModal = false
}

func (a *App) applySummaryMatches(matches []ParsedSummaryBlock) []string {
	applied := ApplySummaryMatches(matches, a.Articles)
	a.clearSelectionExplanation()

	// Keep the editor buffer in sync so SaveNow cannot overwrite a freshly
	// generated summary on the currently selected article.
	cur := a.CurrentArticle()
	if cur != nil && strings.TrimSpace(cur.Summary) != "" {
		a.initSummaryDoc(cur.Summary)
		a.ShowMarkdownRender = true
	}

	a.SaveNow()
	return applied
}

// ---------------- Local AI Summary ----------------

func (a *App) updateUI(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// RefreshLocalLLMStatus checks the local server without blocking the UI. It
// leaves the current phase alone while the service is starting or stopping,
// so a poll cannot overwrite an explicit action.
func (a *App) RefreshLocalLLMStatus() {
	a.updateUI(func() {
		if a.LocalLLMPhase == "starting" || a.LocalLLMPhase == "stopping" {
			return
		}
		a.localLLMRunID++
		runID := a.localLLMRunID

		go func() {
			healthy := LocalLLMHealthy()
			a.updateUI(func() {
				if a.localLLMRunID != runID {
					return
				}
				if a.LocalLLMPhase == "starting" || a.LocalLLMPhase == "stopping" {
					return
				}
				if healthy {
					a.LocalLLMPhase = "ready"
					a.LocalLLMError = ""
				} else {
					if a.LocalLLMPhase == "ready" {
						a.clearSelectionExplanation()
					}
					if a.LocalLLMPhase != "error" {
						a.LocalLLMPhase = "off"
					}
				}
			})
		}()
	})
}

func (a *App) StartLocalLLM() {
	if a.LocalLLMPhase == "starting" || a.LocalLLMPhase == "ready" {
		return
	}
	a.localLLMRunID++
	runID := a.localLLMRunID
	a.LocalLLMPhase = "starting"
	a.LocalLLMError = ""

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), localLLMStartTimeout)
		defer cancel()
		err := StartLocalLLMService(ctx)

		a.updateUI(func() {
			if a.localLLMRunID != runID {
				return
			}
			if err != nil {
				a.LocalLLMPhase = "error"
				a.LocalLLMError = err.Error()
				return
			}
			a.LocalLLMPhase = "ready"
			a.LocalLLMError = ""
		})
	}()
}

func (a *App) StopLocalLLM() {
	if a.LocalLLMPhase == "stopping" || a.LocalLLMPhase == "off" {
		return
	}
	a.cancelArticleAISummary()
	a.clearSelectionExplanation()
	a.localLLMRunID++
	runID := a.localLLMRunID
	a.LocalLLMPhase = "stopping"
	a.LocalLLMError = ""

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := StopLocalLLMService(ctx)

		a.updateUI(func() {
			if a.localLLMRunID != runID {
				return
			}
			if err != nil {
				a.LocalLLMPhase = "error"
				a.LocalLLMError = err.Error()
				return
			}
			a.LocalLLMPhase = "off"
			a.LocalLLMError = ""
		})
	}()
}

func (a *App) StartAISummary() {
	if a.aiCancel != nil {
		return
	}
	target := a.CurrentArticle()
	if target == nil {
		return
	}
	a.clearSelectionExplanation()
	if a.LocalLLMPhase != "ready" {
		a.AISummaryError = "Turn on the local LLM first."
		return
	}

	targetIndex := a.ActiveIdx
	targetTitle := target.Title
	a.AISummaryArticle = targetIndex
	a.AISummaryRunning = true
	a.AISummaryOutput = ""
	a.AISummaryError = ""

	ctx, cancel := context.WithCancel(context.Background())
	a.aiCancel = cancel
	a.aiRunID++
	runID := a.aiRunID

	prompt := BuildSummaryPrompt(a.Settings.SummaryPrompt, []Article{*target})
	go func() {
		if !LocalLLMHealthy() {
			a.updateUI(func() {
				if a.aiRunID != runID {
					return
				}
				a.aiCancel = nil
				a.AISummaryArticle = -1
				a.AISummaryRunning = false
				a.AISummaryOutput = ""
				a.AISummaryError = "The local LLM is not running. Turn it on first."
				a.LocalLLMPhase = "off"
			})
			return
		}

		var answer strings.Builder
		var lastFlush time.Time

		onText := func(chunk string) {
			answer.WriteString(chunk)
			if time.Since(lastFlush) < 100*time.Millisecond {
				return
			}
			lastFlush = time.Now()
			live := liveSummaryText(answer.String())
			a.updateUI(func() {
				if a.aiRunID != runID {
					return
				}
				a.AISummaryOutput = live
			})
		}

		result, err := StreamLocalSummary(ctx, prompt, onText)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return
			}
			a.updateUI(func() {
				if a.aiRunID != runID {
					return
				}
				a.aiCancel = nil
				a.AISummaryArticle = -1
				a.AISummaryRunning = false
				a.AISummaryOutput = ""
				a.AISummaryError = fmt.Sprintf("Could not summarize %q: %s", targetTitle, err)
			})
			return
		}

		summary := summaryFromAnswer(result.Answer)
		if strings.TrimSpace(summary) == "" {
			a.updateUI(func() {
				if a.aiRunID != runID {
					return
				}
				a.aiCancel = nil
				a.AISummaryArticle = -1
				a.AISummaryRunning = false
				a.AISummaryOutput = ""
				a.AISummaryError = fmt.Sprintf("The local LLM returned no summary for %q.", targetTitle)
			})
			return
		}

		a.updateUI(func() {
			if a.aiRunID != runID {
				return
			}
			a.aiCancel = nil
			a.AISummaryArticle = -1
			a.AISummaryRunning = false
			a.AISummaryOutput = ""
			a.AISummaryError = ""

			// Only write the summary if this article is still part of the
			// active project. Switching projects replaces the slice, so a
			// stale index must not touch the new project's articles.
			idx := -1
			for i, article := range a.Articles {
				if article == target {
					idx = i
					break
				}
			}
			if idx < 0 {
				return
			}

			target.Summary = summary
			if a.ActiveIdx == idx {
				a.initSummaryDoc(summary)
				a.ShowMarkdownRender = true
			}
			a.SaveNow()
		})
	}()
}

func (a *App) cancelAIRequest() {
	if a.aiCancel != nil {
		a.aiCancel()
		a.aiCancel = nil
	}
	a.aiRunID++
}

// cancelArticleAISummary stops an in-flight per-article summary. Any partial
// text is discarded and the article keeps its previous summary.
func (a *App) cancelArticleAISummary() {
	wasRunning := a.aiCancel != nil || a.AISummaryRunning
	a.cancelAIRequest()
	if !wasRunning {
		return
	}
	a.AISummaryArticle = -1
	a.AISummaryRunning = false
	a.AISummaryOutput = ""
}

// ---------------- Settings ----------------

func (a *App) OpenSettingsModal() {
	a.SettingsDraftPrompt = a.Settings.SummaryPrompt
	a.ShowSettingsModal = true
}

func (a *App) SaveSettingsModal() {
	a.Settings.SummaryPrompt = strings.TrimSpace(a.SettingsDraftPrompt)
	if a.Settings.SummaryPrompt == "" {
		a.Settings.SummaryPrompt = DefaultSummaryPrompt
	}
	_ = a.Settings.Save()
	a.ShowSettingsModal = false
}

func main() {
	os.Setenv("MYGO_GPU", "0")
	debug.SetGCPercent(40)

	app := NewApp()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:       "Mebot",
			Width:       1600,
			Height:      1000,
			MinWidth:    1280,
			MinHeight:   760,
			Frameless:   true,
			Transparent: true,
			Content:     ui.View(app.View),
		})
		app.prepareWindow(win)
		// Show the window only once its first frame is drawn, instead of
		// opening blank and filling in.
		win.OnReadyToShow(win.Show)
		app.win = win
		app.RefreshLocalLLMStatus()

		// Keep the toolbar's power indicator in step with the service. The
		// server can shut itself down after its idle timeout.
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				app.RefreshLocalLLMStatus()
			}
		}()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
