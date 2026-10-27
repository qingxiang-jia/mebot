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
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"golang.design/x/clipboard"
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

	CodeState          *editor.State
	LastBufferText     string
	ShowMarkdownRender bool
	mdOpts             markdown.Options

	LastSavedAt   string
	IsDirty       bool
	debounceTimer *time.Timer
	saveMu        sync.Mutex

	Settings         *AppSettings
	AvailableFonts   []string
	SelectedFont     string
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

	// Gemini AI Summary State
	ShowAISummaryModal bool
	AISummaryStep      int // 0 = confirm, 1 = running/error, 2 = review
	AISummaryTargets   []Article
	AISummaryThinking  string
	AISummaryOutput    string
	AISummaryStatus    string
	AISummaryError     string
	AISummaryWarning   string
	AISummaryMatches   []ParsedSummaryBlock
	AISummaryOptions   []string
	aiCancel           context.CancelFunc
	aiRunID            int

	// Settings Modal State
	ShowSettingsModal   bool
	SettingsDraftAPIKey string
	SettingsDraftModel  string
	SettingsDraftPrompt string
}

func NewApp() *App {
	_ = clipboard.Init()

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
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         fontSize,
		Theme:                    theme,
		IsDarkMode:               settings.IsDarkMode,
		SelectedDate:             time.Now(),
		ExportDirectory:          DefaultExportDir(),
		ShowExportModal:          false,
		ShowDeleteProjectConfirm: false,
		ShowDeleteArticleConfirm: false,
		ShowNewProjectModal:      false,
		ShowRenameProjectModal:   false,
		ShowBatchSummaryModal:    false,
		ShowAISummaryModal:       false,
		AISummaryStep:            0,
		ShowSettingsModal:        false,
		mdOpts:                   markdown.Options{},
	}

	app.ArticleListState.Selected = &app.ActiveIdx
	app.ArticleListState.Selection = &app.ChosenArticles
	app.ArticleListState.Label = func(i int) string {
		if i >= 0 && i < len(app.Articles) {
			return app.Articles[i].Title
		}
		return ""
	}

	app.initCodeBuffer("")

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

func (a *App) initCodeBuffer(source string) {
	buf := editor.NewBuffer(source)
	buf.SetLanguage(editor.LanguageFor("summary.md"))
	a.CodeState = &editor.State{
		Buffer:   buf,
		SoftWrap: true,
	}
	a.LastBufferText = source
}

func (a *App) SelectArticle(newIdx int) {
	if newIdx < 0 || newIdx >= len(a.Articles) {
		return
	}

	if a.LastActiveIdx >= 0 && a.LastActiveIdx < len(a.Articles) && a.LastActiveIdx != newIdx {
		if a.CodeState != nil && !a.ShowMarkdownRender {
			a.Articles[a.LastActiveIdx].Summary = a.CodeState.Buffer.String()
		}
	}

	a.ActiveIdx = newIdx
	a.LastActiveIdx = newIdx

	cur := a.Articles[newIdx]
	a.initCodeBuffer(cur.Summary)

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
	if cur != nil && a.CodeState != nil && !a.ShowMarkdownRender {
		cur.Summary = a.CodeState.Buffer.String()
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
	a.initCodeBuffer("")

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
		clipboard.Write(clipboard.FmtText, []byte(strings.Join(titles, "\n")))
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
		clipboard.Write(clipboard.FmtText, []byte(sb.String()))
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
		clipboard.Write(clipboard.FmtText, []byte(sb.String()))
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
	if cur != nil && a.CodeState != nil && !a.ShowMarkdownRender {
		cur.Summary = a.CodeState.Buffer.String()
	}

	md := SerializeToMarkdown(a.Articles)
	_ = os.MkdirAll(filepath.Dir(outPath), 0755)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	a.SaveNow()
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

	// Keep the editor buffer in sync so SaveNow cannot overwrite a freshly
	// generated summary on the currently selected article.
	cur := a.CurrentArticle()
	if cur != nil && strings.TrimSpace(cur.Summary) != "" {
		a.initCodeBuffer(cur.Summary)
		a.ShowMarkdownRender = true
	}

	a.SaveNow()
	return applied
}

// ---------------- Gemini AI Summary ----------------

func (a *App) OpenAISummaryModal() {
	a.cancelAIRequest()
	a.AISummaryStep = 0
	a.AISummaryThinking = ""
	a.AISummaryOutput = ""
	a.AISummaryStatus = ""
	a.AISummaryError = ""
	a.AISummaryWarning = ""
	a.AISummaryMatches = nil
	a.AISummaryOptions = nil
	a.AISummaryTargets = nil

	for _, art := range a.Articles {
		if IsSummaryEmpty(art.Summary) {
			a.AISummaryTargets = append(a.AISummaryTargets, *art)
		}
	}
	a.ShowAISummaryModal = true
}

func (a *App) geminiAPIKey() string {
	key := strings.TrimSpace(a.Settings.GeminiAPIKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	return key
}

func (a *App) StartAISummary() {
	if a.aiCancel != nil {
		return
	}
	if len(a.AISummaryTargets) == 0 {
		a.AISummaryError = "There are no articles without a summary."
		return
	}

	apiKey := a.geminiAPIKey()
	if apiKey == "" {
		a.AISummaryError = "Add your Gemini API key in Settings first."
		return
	}

	model := strings.TrimSpace(a.Settings.GeminiModel)
	if model == "" {
		model = DefaultGeminiModel
	}
	prompt := BuildSummaryPrompt(a.Settings.GeminiPrompt, a.AISummaryTargets)

	a.AISummaryStep = 1
	a.AISummaryError = ""
	a.AISummaryWarning = ""
	a.AISummaryThinking = ""
	a.AISummaryOutput = ""
	a.AISummaryStatus = "Starting Gemini..."
	a.AISummaryMatches = nil
	a.AISummaryOptions = nil

	ctx, cancel := context.WithCancel(context.Background())
	a.aiCancel = cancel
	a.aiRunID++
	runID := a.aiRunID

	go func() {
		var thoughts, answer strings.Builder
		var lastFlush time.Time
		status := "Starting Gemini..."

		flush := func() {
			t := thoughts.String()
			ans := answer.String()
			st := status
			a.win.Update(func() {
				if a.aiRunID != runID {
					return
				}
				a.AISummaryThinking = t
				a.AISummaryOutput = ans
				a.AISummaryStatus = st
			})
		}

		onThought := func(chunk string) {
			thoughts.WriteString(chunk)
			status = "Thinking..."
			if time.Since(lastFlush) >= 100*time.Millisecond {
				lastFlush = time.Now()
				flush()
			}
		}
		onText := func(chunk string) {
			answer.WriteString(chunk)
			status = "Writing summaries..."
			if time.Since(lastFlush) >= 100*time.Millisecond {
				lastFlush = time.Now()
				flush()
			}
		}

		result, err := StreamGeminiSummaries(ctx, apiKey, model, prompt, onThought, onText)

		a.win.Update(func() {
			if a.aiRunID != runID {
				return
			}
			a.aiCancel = nil
			a.AISummaryStatus = ""

			if err != nil {
				if ctx.Err() == context.Canceled {
					a.AISummaryStep = 0
					a.AISummaryThinking = ""
					a.AISummaryOutput = ""
					return
				}
				a.AISummaryThinking = result.Thoughts
				a.AISummaryOutput = result.Answer
				a.AISummaryError = err.Error()
				return
			}

			a.AISummaryThinking = result.Thoughts
			a.AISummaryOutput = result.Answer
			switch result.FinishReason {
			case "MAX_TOKENS":
				a.AISummaryWarning = "Gemini's response was truncated. Some summaries may be incomplete."
			case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT":
				a.AISummaryWarning = fmt.Sprintf("Gemini stopped early (%s). Some summaries may be missing.", result.FinishReason)
			}

			blocks := ParsePastedSummaries(stripCodeFences(result.Answer))
			if len(blocks) == 0 {
				a.AISummaryError = "Gemini returned text, but no summaries could be parsed. Try again."
				return
			}

			matches, options := MatchSummariesToArticles(blocks, a.Articles, false)
			a.AISummaryMatches = matches
			a.AISummaryOptions = options
			a.AISummaryStep = 2
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

func (a *App) CancelAISummary() {
	a.cancelAIRequest()
	a.AISummaryStep = 0
	a.AISummaryStatus = ""
	a.AISummaryThinking = ""
	a.AISummaryOutput = ""
	a.AISummaryError = ""
	a.AISummaryWarning = ""
}

func (a *App) CloseAISummaryModal() {
	a.cancelAIRequest()
	a.ShowAISummaryModal = false
}

// ---------------- Settings ----------------

func (a *App) OpenSettingsModal() {
	a.SettingsDraftAPIKey = a.Settings.GeminiAPIKey
	a.SettingsDraftModel = a.Settings.GeminiModel
	a.SettingsDraftPrompt = a.Settings.GeminiPrompt
	a.ShowSettingsModal = true
}

func (a *App) SaveSettingsModal() {
	a.Settings.GeminiAPIKey = strings.TrimSpace(a.SettingsDraftAPIKey)
	a.Settings.GeminiModel = strings.TrimSpace(a.SettingsDraftModel)
	if a.Settings.GeminiModel == "" {
		a.Settings.GeminiModel = DefaultGeminiModel
	}
	a.Settings.GeminiPrompt = strings.TrimSpace(a.SettingsDraftPrompt)
	if a.Settings.GeminiPrompt == "" {
		a.Settings.GeminiPrompt = DefaultGeminiPrompt
	}
	_ = a.Settings.Save()
	a.ShowSettingsModal = false
}

// ---------------- UI VIEW ----------------

func (a *App) View(c *ui.Context) {
	// Closing the AI modal (backdrop or Escape) also cancels an in-flight request.
	if !a.ShowAISummaryModal && a.aiCancel != nil {
		a.cancelAIRequest()
	}

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
				ui.Icon(c, IconAdd).Size(14, 14)
				ui.Text(c, "New").SingleLine()
			}).Clicked() {
			a.ShowNewProjectModal = true
		}

		if len(a.Projects) > 0 {
			if ui.Button(c, "").
				Tooltip("Rename Current Project").
				Gap(6).
				Children(func() {
					ui.Icon(c, IconEdit).Size(14, 14)
					ui.Text(c, "Rename").SingleLine()
				}).Clicked() {
				a.RenameProjectName = a.ActiveProject
				a.ShowRenameProjectModal = true
			}

			if ui.Button(c, "").
				Tooltip("Delete Current Project").
				Gap(6).
				Children(func() {
					ui.Icon(c, IconTrash).Size(14, 14)
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
				ui.Icon(c, IconImport).Size(14, 14)
				ui.Text(c, "Import HTML").SingleLine()
			}).Clicked() {
			a.ImportHTMLFiles()
		}

		if ui.Button(c, "").
			Tooltip("Import Markdown Rollup").
			Gap(6).
			Children(func() {
				ui.Icon(c, IconDocument).Size(14, 14)
				ui.Text(c, "Import MD").SingleLine()
			}).Clicked() {
			a.ImportMarkdownFile()
		}

		if ui.Button(c, "").
			Tooltip("Match Pasted Summaries by Title").
			Gap(6).
			Children(func() {
				ui.Icon(c, IconPaste).Size(14, 14)
				ui.Text(c, "Match").SingleLine()
			}).Clicked() {
			a.OpenBatchSummaryModal()
		}

		if ui.Button(c, "").
			Tooltip("Generate Missing Summaries with Gemini").
			Gap(6).
			Children(func() {
				ui.Icon(c, IconSparkle).Size(14, 14)
				ui.Text(c, "AI Summary").SingleLine()
			}).Clicked() {
			a.OpenAISummaryModal()
		}

		if ui.Button(c, "").
			Tooltip("Export Weekly Rollup").
			Gap(6).
			Children(func() {
				ui.Icon(c, IconExport).Size(14, 14)
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
			prevFont := a.SelectedFont
			ui.Combobox(c.Key("fontFamilySelector"), &a.SelectedFont, a.AvailableFonts).Width(160)
			if a.SelectedFont != prevFont {
				a.Settings.FontFamily = a.SelectedFont
				_ = a.Settings.Save()
			}
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
				ui.Icon(c, themeIcon).Size(14, 14)
				ui.Text(c, themeText).SingleLine()
			}).Clicked() {
			a.ToggleTheme()
		}

		ui.Divider(c)

		if ui.Button(c, "").
			Tooltip("Settings").
			Gap(6).
			Children(func() {
				ui.Icon(c, IconSettings).Size(14, 14)
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
				ui.Icon(c, IconTrash).Size(14, 14)
			}).Clicked() {
				if a.ChosenArticles.Len() > 0 || a.CurrentArticle() != nil {
					a.ShowDeleteArticleConfirm = true
				}
			}
			if ui.Button(c, "").Tooltip("Copy Selected Titles").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Icon(c, IconCopy).Size(14, 14)
			}).Clicked() {
				a.CopySelectedTitles()
			}
			if ui.Button(c, "").Tooltip("Copy Selected Content").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Icon(c, IconPaste).Size(14, 14)
			}).Clicked() {
				a.CopySelectedContents()
			}
			if ui.Button(c, "").Tooltip("Copy Selected Summaries").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Icon(c, IconDocument).Size(14, 14)
			}).Clicked() {
				a.CopySelectedSummaries()
			}

			ui.Spacer(c)

			if ui.Button(c, "").Tooltip("Select All").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Icon(c, IconSelectAll).Size(14, 14)
			}).Clicked() {
				for i := 0; i < len(a.Articles); i++ {
					a.ChosenArticles.Add(i)
				}
				a.SaveProjectSelection()
			}
			if ui.Button(c, "").Tooltip("Clear Selection").Border(1, a.Theme.Border).Radius(6).Children(func() {
				ui.Icon(c, IconSelectNone).Size(14, 14)
			}).Clicked() {
				a.ChosenArticles.Clear()
				a.SaveProjectSelection()
			}
		})

		ui.List(c, &a.ArticleListState, len(a.Articles), func(i int) {
			art := a.Articles[i]
			isCurrent := (i == a.ActiveIdx)

			row := ui.Row(c).Padding(6, 10).Radius(5)
			if isCurrent {
				row.Background(a.Theme.ActiveRowBg)
			}

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
			ui.Text(c, current.FullText).
				Font(a.CurrentFontFamily()).
				FontSize(a.CurrentFontSize()).
				TextColor(a.Theme.Text)
		})
	})
}

func (a *App) renderSummaryColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Fill().Padding(16).Gap(10).Background(a.Theme.CardBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
			ui.Spacer(c)

			ui.Switch(c.Key("summaryPreviewSwitch"), &a.ShowMarkdownRender).Tooltip("Toggle Markdown Preview")
			ui.Text(c, "Preview").FontSize(11).TextColor(a.Theme.TextMuted)
		})

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		if a.mdOpts.Theme == nil {
			a.mdOpts.Theme = &markdown.Theme{}
		}
		a.mdOpts.Theme.Font = a.CurrentFontFamily()
		a.mdOpts.Theme.FontSize = a.CurrentFontSize()

		if a.ShowMarkdownRender {
			ui.Scroll(c).Grow(1).Children(func() {
				markdown.View(c, current.Summary, &a.mdOpts)
			})
		} else {
			if a.CodeState != nil {
				a.CodeState.Font = ui.Font{Size: a.CurrentFontSize()}
				a.CodeState.Theme = editor.DefaultTheme(c.Theme())

				editor.View(c, a.CodeState).Grow(1).FillWidth()

				if a.CodeState.Buffer.Modified() {
					latestText := a.CodeState.Buffer.String()
					if latestText != a.LastBufferText {
						a.LastBufferText = latestText
						current.Summary = latestText
						a.TriggerDebouncedSave()
					}
				}
			}
		}
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
		ui.Text(c, "Settings").FontSize(16).Bold().TextColor(a.Theme.Text)

		ui.Text(c, "Gemini").FontSize(11).Bold().TextColor(a.Theme.TextMuted)

		ui.Text(c, "API Key").FontSize(11).TextColor(a.Theme.TextMuted)
		ui.TextInput(c.Key("geminiAPIKeyInput"), &a.SettingsDraftAPIKey).
			Password().
			Placeholder("AIza...").
			Grow(1)
		ui.Text(c, "Stored locally in settings.json. You can also set GEMINI_API_KEY.").
			FontSize(11).TextColor(a.Theme.TextMuted)

		ui.Text(c, "Model").FontSize(11).TextColor(a.Theme.TextMuted)
		ui.TextInput(c.Key("geminiModelInput"), &a.SettingsDraftModel).
			Placeholder(DefaultGeminiModel).
			Grow(1)

		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Summary prompt").FontSize(11).TextColor(a.Theme.TextMuted)
			ui.Spacer(c)
			if ui.Button(c, "Reset to default").Border(1, a.Theme.Border).Radius(6).Clicked() {
				a.SettingsDraftPrompt = DefaultGeminiPrompt
			}
		})
		ui.TextArea(c.Key("geminiPromptInput"), &a.SettingsDraftPrompt).
			Placeholder(DefaultGeminiPrompt).
			Height(140).
			Grow(1)

		ui.Text(c, "The free tier has strict limits. One request is sent per Generate click, with no automatic retries. A 503 usually means Gemini is at capacity; retry later or try another model.").
			FontSize(11).TextColor(a.Theme.TextMuted)

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

	// 8. Gemini AI Summary Modal
	ui.Modal(c, &a.ShowAISummaryModal, func() {
		switch a.AISummaryStep {
		case 0:
			ui.Row(c).FillWidth().MinWidth(480).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Generate Missing Summaries").FontSize(16).Bold().TextColor(a.Theme.Text)
				ui.Spacer(c)
				ui.Box(c).Width(16)
				if ui.Button(c, "Settings").Clicked() {
					a.CloseAISummaryModal()
					a.OpenSettingsModal()
				}
			})

			if len(a.Articles) == 0 {
				ui.Text(c, "This project has no articles.").FontSize(12).TextColor(a.Theme.TextMuted)
				ui.Row(c).Justify(ui.End).Children(func() {
					if ui.Button(c, "Close").Clicked() {
						a.CloseAISummaryModal()
					}
				})
				return
			}

			if len(a.AISummaryTargets) == 0 {
				ui.Text(c, "All articles in this project already have a summary.").
					FontSize(12).TextColor(a.Theme.TextMuted)
				ui.Row(c).Justify(ui.End).Children(func() {
					if ui.Button(c, "Close").Clicked() {
						a.CloseAISummaryModal()
					}
				})
				return
			}

			ui.Text(c, fmt.Sprintf("%d article(s) have no summary yet. Gemini will summarize all of them in one request.",
				len(a.AISummaryTargets))).FontSize(12).TextColor(a.Theme.TextMuted)
			model := strings.TrimSpace(a.Settings.GeminiModel)
			if model == "" {
				model = DefaultGeminiModel
			}
			ui.Text(c, "Model: "+model+" • One API request for the whole batch").
				FontSize(11).TextColor(a.Theme.TextMuted)

			ui.Scroll(c).Height(240).Children(func() {
				ui.Column(c).Gap(4).AlignItems(ui.Stretch).Children(func() {
					for _, art := range a.AISummaryTargets {
						ui.Text(c, "• "+art.Title).FontSize(12).TextColor(a.Theme.Text).SingleLine()
					}
				})
			})

			if a.geminiAPIKey() == "" {
				ui.Text(c, "Gemini API key is not set. Add it in Settings first.").
					FontSize(12).Bold().TextColor(ui.Hex("#ff3b30"))
			}
			if a.AISummaryError != "" {
				ui.Text(c, a.AISummaryError).FontSize(12).Bold().TextColor(ui.Hex("#ff3b30"))
			}

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.CloseAISummaryModal()
				}
				if a.geminiAPIKey() != "" {
					if ui.PrimaryButton(c, "Generate Summaries").Clicked() {
						a.StartAISummary()
					}
				}
			})

		case 1:
			ui.Row(c).FillWidth().MinWidth(480).AlignItems(ui.Center).Gap(8).Children(func() {
				if a.AISummaryError == "" && a.AISummaryStatus != "" {
					ui.Spinner(c)
				}
				ui.Text(c, "Gemini").FontSize(16).Bold().TextColor(a.Theme.Text)
				ui.Spacer(c)
				if a.AISummaryStatus != "" {
					ui.Text(c, a.AISummaryStatus).FontSize(11).TextColor(a.Theme.TextMuted)
				}
			})

			if a.AISummaryThinking != "" {
				ui.Text(c, "THINKING").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
				ui.Scroll(c).Height(120).Children(func() {
					ui.Text(c, tailRunes(a.AISummaryThinking, 2500)).
						FontSize(11).TextColor(a.Theme.TextMuted)
				})
			}

			if a.AISummaryOutput != "" {
				ui.Text(c, "SUMMARIES").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
				ui.Scroll(c).Height(240).Children(func() {
					ui.Text(c, tailRunes(a.AISummaryOutput, 4000)).
						FontSize(12).TextColor(a.Theme.Text)
				})
			}

			if a.AISummaryError != "" {
				ui.Text(c, a.AISummaryError).FontSize(12).Bold().TextColor(ui.Hex("#ff3b30"))
				ui.Text(c, "Each retry sends another Gemini request.").
					FontSize(11).TextColor(a.Theme.TextMuted)
			}

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if a.AISummaryError == "" {
					if ui.Button(c, "Cancel").Clicked() {
						a.CancelAISummary()
					}
				} else {
					if ui.Button(c, "Back").Clicked() {
						a.AISummaryStep = 0
						a.AISummaryError = ""
					}
					if ui.Button(c, "Settings").Clicked() {
						a.CloseAISummaryModal()
						a.OpenSettingsModal()
					}
					if ui.PrimaryButton(c, "Retry").Clicked() {
						a.StartAISummary()
					}
				}
			})

		case 2:
			ui.Row(c).FillWidth().MinWidth(480).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Review AI Summaries").FontSize(16).Bold().TextColor(a.Theme.Text)
				ui.Spacer(c)
				if ui.Button(c, "Back").Clicked() {
					a.AISummaryStep = 0
					a.AISummaryError = ""
				}
			})

			matchedSet := make(map[string]bool)
			var matchedTitles []string
			for _, match := range a.AISummaryMatches {
				if match.SelectedOption == OptionDoNotMatch || matchedSet[match.SelectedOption] {
					continue
				}
				matchedSet[match.SelectedOption] = true
				matchedTitles = append(matchedTitles, match.SelectedOption)
			}
			matchedCount := len(matchedTitles)

			ui.Text(c, fmt.Sprintf("Gemini returned %d summaries. %d article(s) will receive a summary.",
				len(a.AISummaryMatches), matchedCount)).
				FontSize(12).TextColor(a.Theme.TextMuted)

			if a.AISummaryWarning != "" {
				ui.Text(c, a.AISummaryWarning).FontSize(11).TextColor(ui.Hex("#b26a00"))
			}
			if matchedCount < len(a.AISummaryTargets) {
				ui.Text(c, fmt.Sprintf("%d of the %d articles without a summary still have no matched summary.",
					len(a.AISummaryTargets)-matchedCount, len(a.AISummaryTargets))).
					FontSize(11).TextColor(ui.Hex("#b26a00"))
			}
			if matchedCount > 0 {
				ui.Text(c, "Articles receiving summaries:").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
				ui.Scroll(c).Height(80).Children(func() {
					ui.Column(c).Gap(2).AlignItems(ui.Stretch).Children(func() {
						for _, title := range matchedTitles {
							ui.Text(c, "• "+title).
								FontSize(11).TextColor(a.Theme.Accent).SingleLine()
						}
					})
				})
			}

			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
					for i := range a.AISummaryMatches {
						item := &a.AISummaryMatches[i]

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
									ui.Select(c.Key(fmt.Sprintf("aiAssignSel_%d", i)), &item.SelectedOption, a.AISummaryOptions).Width(340)
								})
							})
					}
				})
			})

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.CloseAISummaryModal()
				}
				if matchedCount > 0 {
					if ui.PrimaryButton(c, "Save Summaries").Clicked() {
						applied := a.applySummaryMatches(a.AISummaryMatches)
						a.ShowAISummaryModal = false
						a.AISummaryStep = 0
						switch len(applied) {
						case 0:
							c.Toast("No summaries were saved")
						case 1:
							c.Toast("Saved summary for 1 article")
						default:
							c.Toast(fmt.Sprintf("Saved summaries for %d articles", len(applied)))
						}
					}
				}
			})
		}
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
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
