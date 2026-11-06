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

	SidebarWidth float32
	ReaderWidth  float32

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

	Theme        MacOSTheme
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

	theme := MacOSLightTheme()
	if settings.IsDarkMode {
		theme = MacOSDarkTheme()
	}

	fontSizeOptions := []string{"12", "13", "14", "15", "16", "17", "18", "20", "22", "24", "28", "32"}

	app := &App{
		Projects:                 existingProjects,
		Articles:                 []*Article{},
		ActiveIdx:                0,
		LastActiveIdx:            -1,
		SidebarWidth:             320,
		ReaderWidth:              680,
		Settings:                 settings,
		AvailableFonts:           fonts,
		SelectedFont:             defaultFont,
		FontPickerText:           defaultFont,
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         fontSize,
		Theme:                    theme,
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
		// it, whose menu mygo draws in the window: the view's own is the
		// system's, GTK's on Linux.
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
	if m.Item(del).Chosen() {
		a.ShowDeleteArticleConfirm = true
	}
}

// Native Dialogs: run in goroutines, update UI via win.Update()
func (a *App) ImportHTMLFiles() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "Import HTML Article(s)",
			Filters:  []mygo.FileFilter{{Name: "HTML Articles", Extensions: []string{"html", "htm"}}},
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
			Filters:  []mygo.FileFilter{{Name: "Markdown Files", Extensions: []string{"md", "markdown"}}},
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
		a.Theme = MacOSDarkTheme()
		mygo.Theme.SetSource(mygo.ThemeDark)
	} else {
		a.Theme = MacOSLightTheme()
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

// renderLocalLLMButton is the toolbar power switch. Its icon is green while
// the local model is running and red while it is off.
func (a *App) renderLocalLLMButton(c *ui.Context) {
	color := ui.Hex("#ff3b30")
	tooltip := "Turn On Local LLM"
	busy := a.LocalLLMPhase == "checking" || a.LocalLLMPhase == "starting" || a.LocalLLMPhase == "stopping"

	switch a.LocalLLMPhase {
	case "ready":
		color = ui.Hex("#34c759")
		tooltip = "Turn Off Local LLM"
	case "checking":
		color = ui.Hex("#ff9f0a")
		tooltip = "Checking Local LLM status"
	case "starting":
		color = ui.Hex("#ff9f0a")
		tooltip = "Local LLM is starting"
	case "stopping":
		color = ui.Hex("#ff9f0a")
		tooltip = "Local LLM is stopping"
	case "error":
		tooltip = "Local LLM error; click to try again"
	}

	button := ui.Button(c, "").
		Tooltip(tooltip).
		Disabled(busy).
		Gap(6).
		Children(func() {
			ui.Icon(c, IconPower).Size(14, 14).TextColor(color)
			ui.Text(c, "Local LLM").SingleLine()
		})

	if !button.Clicked() {
		return
	}
	switch a.LocalLLMPhase {
	case "ready":
		a.StopLocalLLM()
	case "checking", "starting", "stopping":
		// The running action will settle the state.
	default:
		a.StartLocalLLM()
	}
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

// ---------------- UI VIEW ----------------

func (a *App) View(c *ui.Context) {
	// Sync accent across all native widgets
	t := *c.Theme()
	t.Accent = a.Theme.ActiveRowBg
	t.AccentText = a.Theme.ActiveRowText
	c.SetTheme(&t)

	if a.ActiveIdx != a.LastActiveIdx && a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		a.SelectArticle(a.ActiveIdx)
		c.Invalidate()
	}

	ui.Column(c).Fill().Background(a.Theme.WindowBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		// 1. Toolbar
		a.renderToolbar(c)

		// 1px separator line matching split view border
		ui.Row(c).Height(1).FillWidth().Background(a.Theme.Border)

		// 2. Resizable Split Panes
		ui.Split(c, &a.SidebarWidth, func() {
			a.renderArticleListColumn(c)
		}, func() {
			ui.Split(c, &a.ReaderWidth, func() {
				a.renderFullTextColumn(c)
			}, func() {
				a.renderSummaryColumn(c)
			}).Fill()
		}).Grow(1)

		// 3. Modals & Native Dialogs
		a.renderModals(c)
	})
}

func (a *App) renderToolbar(c *ui.Context) {
	ui.Toolbar(c, func() {
		// Group 1: Projects
		if len(a.Projects) > 1 {
			prevIdx := a.ActiveProjectIdx
			ui.Segmented(c.Key("projectSegment"), &a.ActiveProjectIdx, a.Projects...).Label("Projects")
			if a.ActiveProjectIdx != prevIdx && a.ActiveProjectIdx < len(a.Projects) {
				a.SwitchProject(a.Projects[a.ActiveProjectIdx])
			}
		} else if len(a.Projects) == 1 {
			ui.Text(c, a.Projects[0]).FontSize(13).TextColor(a.Theme.Text)
		}

		if ui.Button(c, "").
			Tooltip("New Project").
			Gap(6).
			Children(func() {
				ui.Image(c, IconAdd).Size(14, 14)
				ui.Text(c, "New").SingleLine()
			}).Clicked() {
			a.ShowNewProjectModal = true
		}

		if len(a.Projects) > 0 {
			if ui.Button(c, "").
				Tooltip("Rename Current Project").
				Gap(6).
				Children(func() {
					ui.Image(c, IconEdit).Size(14, 14)
					ui.Text(c, "Rename").SingleLine()
				}).Clicked() {
				a.RenameProjectName = a.ActiveProject
				a.ShowRenameProjectModal = true
			}

			if ui.Button(c, "").
				Tooltip("Delete Current Project").
				Gap(6).
				Children(func() {
					ui.Image(c, IconTrash).Size(14, 14)
					ui.Text(c, "Delete").SingleLine()
				}).Clicked() {
				a.ShowDeleteProjectConfirm = true
			}
		}

		ui.Divider(c)

		// Group 2: Article Actions
		if ui.Button(c, "").
			Tooltip("Import HTML Article(s)").
			Gap(6).
			Children(func() {
				ui.Image(c, IconImport).Size(14, 14)
				ui.Text(c, "Import HTML").SingleLine()
			}).Clicked() {
			a.ImportHTMLFiles()
		}

		if ui.Button(c, "").
			Tooltip("Import Markdown Rollup").
			Gap(6).
			Children(func() {
				ui.Image(c, IconDocument).Size(14, 14)
				ui.Text(c, "Import MD").SingleLine()
			}).Clicked() {
			a.ImportMarkdownFile()
		}

		if ui.Button(c, "").
			Tooltip("Match Pasted Summaries by Title").
			Gap(6).
			Children(func() {
				ui.Image(c, IconPaste).Size(14, 14)
				ui.Text(c, "Match").SingleLine()
			}).Clicked() {
			a.OpenBatchSummaryModal()
		}

		a.renderLocalLLMButton(c)

		if ui.Button(c, "").
			Tooltip("Export Weekly Rollup").
			Gap(6).
			Children(func() {
				ui.Image(c, IconExport).Size(14, 14)
				ui.Text(c, "Export").SingleLine()
			}).Clicked() {
			a.ShowExportModal = true
		}

		ui.Spacer(c)

		if a.IsDirty {
			ui.Text(c, "Saving...").FontSize(11).TextColor(a.Theme.Accent)
		} else if a.LastSavedAt != "" {
			ui.Text(c, "Saved "+a.LastSavedAt).FontSize(11).TextColor(a.Theme.TextMuted)
		}

		ui.Divider(c)

		// Group 3: Typography & Theme
		ui.Text(c, "Font:").FontSize(11).TextColor(a.Theme.TextMuted)
		if len(a.AvailableFonts) > 0 {
			a.fontCombobox(c)
		}

		ui.Text(c, "Size:").FontSize(11).TextColor(a.Theme.TextMuted)
		prevSize := a.SelectedFontSize
		ui.Select(c.Key("fontSizeSelector"), &a.SelectedFontSize, a.FontSizeOptions).Width(60)
		if a.SelectedFontSize != prevSize {
			a.Settings.FontSize = a.SelectedFontSize
			_ = a.Settings.Save()
		}

		ui.Divider(c)

		themeIcon := IconSun
		themeText := "Light"
		themeTip := "Switch to Light Mode"
		if !a.IsDarkMode {
			themeIcon = IconMoon
			themeText = "Dark"
			themeTip = "Switch to Dark Mode"
		}
		if ui.Button(c, "").
			Tooltip(themeTip).
			Gap(6).
			Children(func() {
				ui.Image(c, themeIcon).Size(14, 14)
				ui.Text(c, themeText).SingleLine()
			}).Clicked() {
			a.ToggleTheme()
		}

		ui.Divider(c)

		if ui.Button(c, "").
			Tooltip("Settings").
			Gap(6).
			Children(func() {
				ui.Image(c, IconSettings).Size(14, 14)
				ui.Text(c, "Settings").SingleLine()
			}).Clicked() {
			a.OpenSettingsModal()
		}
	}).Label("Mebot Toolbar")
}

func (a *App) renderArticleListColumn(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Background(a.Theme.SidebarBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
			if ui.Button(c, "").Tooltip("Delete Selected Article(s)").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconTrash).Size(14, 14)
			}).Clicked() {
				if a.ChosenArticles.Len() > 0 || a.CurrentArticle() != nil {
					a.ShowDeleteArticleConfirm = true
				}
			}
			if ui.Button(c, "").Tooltip("Copy Selected Titles").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconCopy).Size(14, 14)
			}).Clicked() {
				a.CopySelectedTitles()
			}
			if ui.Button(c, "").Tooltip("Copy Selected Content").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconPaste).Size(14, 14)
			}).Clicked() {
				a.CopySelectedContents()
			}
			if ui.Button(c, "").Tooltip("Copy Selected Summaries").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconDocument).Size(14, 14)
			}).Clicked() {
				a.CopySelectedSummaries()
			}

			ui.Spacer(c)

			if ui.Button(c, "").Tooltip("Select All").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconSelectAll).Size(14, 14)
			}).Clicked() {
				a.SelectAllArticles()
			}
			if ui.Button(c, "").Tooltip("Clear Selection").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Image(c, IconSelectNone).Size(14, 14)
			}).Clicked() {
				a.ClearArticleSelection()
			}
		})

		ui.List(c, &a.ArticleListState, len(a.Articles), func(i int) {
			art := a.Articles[i]
			isCurrent := (i == a.ActiveIdx)

			row := ui.Row(c).Padding(6, 10).Radius(5)
			if isCurrent {
				row.Background(a.Theme.ActiveRowBg)
			}
			row.PopoverMenu(a.buildArticleMenu)

			row.Children(func() {
				t := ui.Text(c, art.Title).
					Font(a.CurrentFontFamily()).
					FontSize(13).
					Grow(1)

				if isCurrent {
					t.Bold().TextColor(a.Theme.ActiveRowText)
				} else {
					t.TextColor(a.Theme.Text)
				}
			})
		}).Grow(1).Children(func() {
			if len(a.Articles) == 0 {
				ui.Text(c, "No articles yet.").FontSize(12).TextColor(a.Theme.TextMuted).Padding(16)
			}
		})
	})
}

func (a *App) renderFullTextColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Fill().Padding(20).Gap(12).Background(a.Theme.CardBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, "FULL TEXT").FontSize(11).Bold().TextColor(a.Theme.TextMuted)

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		ui.Text(c, current.Title).
			Font(a.CurrentFontFamily()).
			FontSize(a.CurrentFontSize() + 4).
			Bold().
			TextColor(a.Theme.Text)

		ui.Scroll(c).Grow(1).Children(func() {
			box := ui.Column(c).Selectable().FillWidth()
			box.Children(func() {
				ui.Text(c, current.FullText).
					Font(a.CurrentFontFamily()).
					FontSize(a.CurrentFontSize()).
					TextColor(a.Theme.Text)
			})
			a.observeSelection(box, "fulltext")
		})

		a.renderSelectionExplanation(c, "fulltext")
	})
}

func (a *App) renderSummaryColumn(c *ui.Context) {
	current := a.CurrentArticle()
	running := a.AISummaryRunning && current != nil && a.AISummaryArticle == a.ActiveIdx
	ui.Column(c).Fill().Padding(16).Gap(10).Background(a.Theme.CardBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		wasPreview := a.ShowMarkdownRender
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
			ui.Spacer(c)

			ui.Switch(c.Key("summaryPreviewSwitch"), &a.ShowMarkdownRender).Tooltip("Toggle Markdown Preview")
			ui.Text(c, "Preview").FontSize(11).TextColor(a.Theme.TextMuted)

			if running {
				ui.Spinner(c)
			}

			aiTip := "Generate a summary for this article with Local AI"
			if a.LocalLLMPhase != "ready" {
				aiTip = "Turn on Local LLM to generate summaries"
			} else if current == nil {
				aiTip = "Select an article to summarize"
			} else if running {
				aiTip = "Summarizing this article..."
			}
			if ui.Button(c, "").
				Tooltip(aiTip).
				Disabled(a.LocalLLMPhase != "ready" || current == nil || running).
				Gap(6).
				Children(func() {
					ui.Image(c, IconSparkle).Size(14, 14)
					ui.Text(c, "AI Summary").SingleLine()
				}).Clicked() {
				a.StartAISummary()
			}

			if running {
				if ui.Button(c, "Cancel").Border(1, a.Theme.Border).Radius(6).Clicked() {
					a.cancelArticleAISummary()
				}
			}
		})

		if a.AISummaryError != "" {
			ui.Text(c, a.AISummaryError).FontSize(11).TextColor(ui.Hex("#ff3b30"))
		}

		// Turning the preview on shows what was just typed, which the
		// rich-text editor holds as runs until it is written back.
		if a.ShowMarkdownRender && !wasPreview && current != nil && a.SummaryDoc != nil {
			current.Summary = a.currentSummaryMarkdown()
		}

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		if !a.ShowMarkdownRender && wasPreview && a.SelectionSource == "summary" {
			a.clearSelectionExplanation()
		}

		if a.mdOpts.Theme == nil {
			a.mdOpts.Theme = &markdown.Theme{}
		}
		a.mdOpts.Theme.Font = a.CurrentFontFamily()
		a.mdOpts.Theme.FontSize = a.CurrentFontSize()

		if running {
			ui.Scroll(c).Grow(1).Children(func() {
				if strings.TrimSpace(a.AISummaryOutput) == "" {
					ui.Text(c, "Waiting for the local LLM...").
						FontSize(a.CurrentFontSize()).
						TextColor(a.Theme.TextMuted)
					return
				}
				markdown.View(c, a.AISummaryOutput, &a.mdOpts)
			})
		} else if a.ShowMarkdownRender {
			ui.Scroll(c).Grow(1).Children(func() {
				// The markdown view gives its selection to this box, whose
				// menu MyGo draws in the window, in the theme's look. The
				// view's own menu is the system's, GTK's on Linux.
				box := ui.Column(c).Selectable().PopoverMenu(func(m *ui.Menu) {
					m.EditItems()
					m.Separator()
					if m.Item("Copy Summary as Markdown").Chosen() {
						writeClipboardText(current.Summary)
					}
				})
				box.Children(func() {
					markdown.View(c, current.Summary, &a.mdOpts)
				})
				a.observeSelection(box, "summary")
			})
		} else {
			if a.SummaryDoc != nil {
				view := richtext.EditorBase(c.Key(fmt.Sprintf("summaryMarkdownEditor_%d", a.ActiveIdx)), a.SummaryDoc, richtext.EditorOptions{
					Label:       "Summary",
					Placeholder: "Write the summary in Markdown...",
					Font:        ui.Font{Family: a.CurrentFontFamily(), Size: a.CurrentFontSize()},
				}).Grow(1).FillWidth()

				if view.Changed() {
					latestText := a.currentSummaryMarkdown()
					if latestText != a.LastSummaryText {
						a.LastSummaryText = latestText
						current.Summary = latestText
						a.TriggerDebouncedSave()
					}
				}
			}
		}

		a.renderSelectionExplanation(c, "summary")
	})
}

// Native Modals using ui.AlertDialog and ui.Modal
func (a *App) renderModals(c *ui.Context) {
	// 1. Delete Project Alert Dialog
	if a.ShowDeleteProjectConfirm {
		msg := fmt.Sprintf("Are you sure you want to delete '%s' and all its articles?", a.ActiveProject)
		switch ui.AlertDialog(c, &a.ShowDeleteProjectConfirm, "Delete Project", msg, "Cancel", "Delete") {
		case 1:
			a.DeleteActiveProject()
		}
	}

	// 2. Delete Article Alert Dialog
	if a.ShowDeleteArticleConfirm {
		count := a.ChosenArticles.Len()
		msg := "Are you sure you want to delete the selected article?"
		if count > 1 {
			msg = fmt.Sprintf("Are you sure you want to delete all %d selected articles?", count)
		} else if cur := a.CurrentArticle(); cur != nil {
			msg = fmt.Sprintf("Are you sure you want to delete '%s'?", cur.Title)
		}

		switch ui.AlertDialog(c, &a.ShowDeleteArticleConfirm, "Delete Article", msg, "Cancel", "Delete") {
		case 1:
			a.ConfirmDeleteChosenArticles()
		}
	}

	// 3. New Project Modal
	ui.Modal(c, &a.ShowNewProjectModal, func() {
		ui.Text(c, "New Project").FontSize(16).Bold().TextColor(a.Theme.Text)
		if ui.TextInput(c.Key("newProjInput"), &a.NewProjectName).AutoFocus().Placeholder("Project name...").Submitted() {
			a.CreateProject(a.NewProjectName)
		}
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				a.ShowNewProjectModal = false
			}
			if ui.PrimaryButton(c, "Create").Clicked() {
				a.CreateProject(a.NewProjectName)
			}
		})
	})

	// 4. Rename Project Modal
	ui.Modal(c, &a.ShowRenameProjectModal, func() {
		ui.Text(c, "Rename Project").FontSize(16).Bold().TextColor(a.Theme.Text)
		if ui.TextInput(c.Key("renameProjInput"), &a.RenameProjectName).AutoFocus().Placeholder("New project name...").Submitted() {
			a.RenameActiveProject(a.RenameProjectName)
		}
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				a.ShowRenameProjectModal = false
			}
			if ui.PrimaryButton(c, "Rename").Clicked() {
				a.RenameActiveProject(a.RenameProjectName)
			}
		})
	})

	// 5. Export Modal
	ui.Modal(c, &a.ShowExportModal, func() {
		ui.Text(c, "Export Weekly Rollup").FontSize(16).Bold().TextColor(a.Theme.Text)

		ui.Text(c, "Select Export Date:").FontSize(11).TextColor(a.Theme.TextMuted)
		ui.Calendar(c, &a.SelectedDate)

		exportName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)

		ui.Text(c, "Destination Folder:").FontSize(11).TextColor(a.Theme.TextMuted)
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.TextInput(c.Key("exportDirInput"), &a.ExportDirectory).Grow(1)
			if ui.Button(c, "Browse...").Border(1, a.Theme.Border).Radius(6).Clicked() {
				a.BrowseExportDirectory()
			}
		})

		destPath := filepath.Join(a.ExportDirectory, exportName)
		ui.Text(c, "File: "+exportName).FontSize(11).Bold().TextColor(a.Theme.Accent)

		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				a.ShowExportModal = false
			}
			if ui.PrimaryButton(c, "Export").Clicked() {
				a.PerformExportTo(destPath)
			}
		})
	})

	// 6. Batch AI Summary Matcher Modal
	ui.Modal(c, &a.ShowBatchSummaryModal, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Match Markdown Summaries").FontSize(16).Bold().TextColor(a.Theme.Text)
			ui.Spacer(c)
			if a.BatchMatchingStep == 1 {
				if ui.Button(c, "Back to Edit").Clicked() {
					a.BatchMatchingStep = 0
				}
			}
		})

		emptyCount := 0
		for _, art := range a.Articles {
			if IsSummaryEmpty(art.Summary) {
				emptyCount++
			}
		}

		if a.BatchMatchingStep == 0 {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Paste your markdown summaries below:").FontSize(12).TextColor(a.Theme.TextMuted)
				ui.Spacer(c)
				ui.Text(c, fmt.Sprintf("Project has %d articles (%d without summary)", len(a.Articles), emptyCount)).
					FontSize(11).TextColor(a.Theme.Accent)
			})

			ui.TextArea(c.Key("batchAreaInput"), &a.BatchPastedMarkdown).
				Placeholder("## Article Title\nSummary content here...\n\n## Another Article Title\nSummary content here...").
				Height(280).
				Grow(1)

			ui.Checkbox(c.Key("batchIncludeExistingCheck"), &a.BatchIncludeExisting, "Include articles that already have summaries")

			if a.BatchMatchError != "" {
				ui.Text(c, a.BatchMatchError).FontSize(12).Bold().TextColor(ui.Hex("#ff3b30"))
			}

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowBatchSummaryModal = false
				}
				if ui.PrimaryButton(c, "Analyze & Match").Clicked() {
					a.AnalyzeBatchMarkdown()
				}
			})
		} else {
			ui.Text(c, "Verify or adjust the matched articles. Any option set to '[Do Not Match]' will be skipped.").
				FontSize(12).TextColor(a.Theme.TextMuted)

			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
					for i := range a.ParsedSummaryMatches {
						item := &a.ParsedSummaryMatches[i]

						ui.Column(c).
							Padding(10).
							Radius(6).
							Background(a.Theme.SidebarBg).
							Border(1, a.Theme.Border).
							Gap(6).
							Children(func() {
								ui.Text(c, "Summary Title: "+item.RawTitle).
									FontSize(13).Bold().TextColor(a.Theme.Text)

								excerpt := item.Body
								if len([]rune(excerpt)) > 120 {
									excerpt = string([]rune(excerpt)[:120]) + "..."
								}
								ui.Text(c, excerpt).FontSize(11).TextColor(a.Theme.TextMuted)

								ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
									ui.Text(c, "Assign to:").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
									ui.Select(c.Key(fmt.Sprintf("assignSel_%d", i)), &item.SelectedOption, a.UnmatchedOptions).Width(340)
								})
							})
					}
				})
			})

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowBatchSummaryModal = false
				}
				if ui.PrimaryButton(c, "Apply All Matches").Clicked() {
					a.ApplyBatchMatches()
				}
			})
		}
	})

	// 7. Settings Modal
	ui.Modal(c, &a.ShowSettingsModal, func() {
		ui.Column(c).Width(520).Gap(12).Children(func() {
			ui.Text(c, "Settings").FontSize(16).Bold().TextColor(a.Theme.Text)

			ui.Text(c, "Local AI").FontSize(11).TextColor(a.Theme.TextMuted)
			ui.Text(c, fmt.Sprintf("%s • %s", DefaultLocalLLMModel, LocalLLMBaseURL)).
				FontSize(12).TextColor(a.Theme.Text)

			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Summary prompt").FontSize(11).TextColor(a.Theme.TextMuted)
				ui.Spacer(c)
				if ui.Button(c, "Reset to default").Border(1, a.Theme.Border).Radius(6).Clicked() {
					a.SettingsDraftPrompt = DefaultSummaryPrompt
				}
			})
			ui.TextArea(c.Key("summaryPromptInput"), &a.SettingsDraftPrompt).
				Placeholder(DefaultSummaryPrompt).
				Height(140).
				Grow(1)

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowSettingsModal = false
				}
				if ui.PrimaryButton(c, "Save Settings").Clicked() {
					a.SaveSettingsModal()
					c.Toast("Settings saved")
				}
			})
		})
	})

}

func main() {
	os.Setenv("MYGO_GPU", "0")
	debug.SetGCPercent(40)

	app := NewApp()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:   "Mebot",
			Width:   1760,
			Height:  1000,
			Content: ui.View(app.View),
		})
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
