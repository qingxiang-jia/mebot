package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	Projects         []string
	ActiveProjectIdx int
	ActiveProject    string
	NewProjectName   string
	Articles         []*Article
	ActiveIdx        int
	LastActiveIdx    int

	// List State with selection pointer bound to ActiveIdx
	ArticleListState ui.ListState
	ChosenArticles   ui.Selection[int]

	// Split view pane dimensions (DIPs)
	SidebarWidth float32
	ReaderWidth  float32

	// Code Editor State for Summary (ui/editor)
	CodeState          *editor.State
	LastBufferText     string
	ShowMarkdownRender bool
	mdOpts             markdown.Options

	// Auto-save & Debounce State
	LastSavedAt   string
	IsDirty       bool
	debounceTimer *time.Timer
	saveMu        sync.Mutex

	// Fonts & Settings
	Settings         *AppSettings
	AvailableFonts   []string
	SelectedFont     string
	FontSizeOptions  []string
	SelectedFontSize string

	Theme        MacOSTheme
	IsDarkMode   bool
	SelectedDate time.Time

	ShowExportModal          bool
	ShowDeleteProjectConfirm bool
	ShowDeleteArticleConfirm bool
	ShowNewProjectModal      bool
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
		} else {
			initialProject = ""
		}
	}

	allFonts := GetLinuxSystemFonts()
	fonts := FilterCleanReadingFonts(allFonts)

	// Pick default font: settings -> first sans font -> "Sans"
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
		ActiveProjectIdx:         0,
		ActiveProject:            "",
		Articles:                 []*Article{},
		ActiveIdx:                0,
		LastActiveIdx:            -1,
		SidebarWidth:             320,
		ReaderWidth:              680,
		ShowMarkdownRender:       false,
		Settings:                 settings,
		AvailableFonts:           fonts,
		SelectedFont:             defaultFont,
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         fontSize,
		Theme:                    theme,
		IsDarkMode:               settings.IsDarkMode,
		SelectedDate:             time.Now(),
		ShowExportModal:          false,
		ShowDeleteProjectConfirm: false,
		ShowDeleteArticleConfirm: false,
		ShowNewProjectModal:      false,
		mdOpts:                   markdown.Options{},
	}

	// Bind selection pointer to ActiveIdx so list clicks update the active article
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

// ---------------- STATE & PERSISTENCE ----------------

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

	// 1. Flush summary of the PREVIOUS article ONLY if it was valid and different
	if a.LastActiveIdx >= 0 && a.LastActiveIdx < len(a.Articles) && a.LastActiveIdx != newIdx {
		if a.CodeState != nil && !a.ShowMarkdownRender {
			a.Articles[a.LastActiveIdx].Summary = a.CodeState.Buffer.String()
		}
	}

	a.ActiveIdx = newIdx
	a.LastActiveIdx = newIdx

	// 2. Fetch target article and safely load its summary without wiping it
	cur := a.Articles[newIdx]
	a.initCodeBuffer(cur.Summary)

	// 3. Smart view: Default to Markdown if summary exists, else Code Editor
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
	a.NewProjectName = ""
	a.ShowNewProjectModal = false
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
				sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n---\n\n", art.Title, art.FullText))
				hasAny = true
			}
		}
	}
	if !hasAny && a.CurrentArticle() != nil {
		art := a.CurrentArticle()
		sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n---\n\n", art.Title, art.FullText))
	}
	if sb.Len() > 0 {
		clipboard.Write(clipboard.FmtText, []byte(sb.String()))
	}
}

func (a *App) ImportHTMLFiles() {
	files, err := PickFiles("Select HTML Article(s)", true, "*.html", "*.htm")
	if err != nil || len(files) == 0 {
		return
	}

	if a.ActiveProject == "" {
		base := filepath.Base(files[0])
		projName := strings.TrimSuffix(base, filepath.Ext(base))
		a.CreateProject(projName)
	}

	for _, f := range files {
		art, err := ExtractArticleFromHTML(f)
		if err == nil {
			a.Articles = append(a.Articles, art)
		}
	}
	a.SelectArticle(a.ActiveIdx)
	a.SaveNow()
}

func (a *App) ImportMarkdownFile() {
	files, err := PickFiles("Select Markdown Rollup", false, "*.md", "*.markdown")
	if err != nil || len(files) == 0 {
		return
	}

	filePath := files[0]
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	parsed, err := ParseMarkdownRollup(string(data))
	if err != nil || len(parsed) == 0 {
		return
	}

	if a.ActiveProject == "" {
		base := filepath.Base(filePath)
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

func (a *App) PerformExport() {
	if len(a.Articles) == 0 || a.ActiveProject == "" {
		a.ShowExportModal = false
		return
	}

	cur := a.CurrentArticle()
	if cur != nil && a.CodeState != nil && !a.ShowMarkdownRender {
		cur.Summary = a.CodeState.Buffer.String()
	}

	fileName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
	home, _ := os.UserHomeDir()
	outPath := filepath.Join(home, "Documents", fileName)

	md := SerializeToMarkdown(a.Articles)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	_ = RemoveProjectState(a.ActiveProject)
	delete(a.Settings.ProjectStates, a.ActiveProject)
	_ = a.Settings.Save()

	a.Articles = []*Article{}
	a.ActiveIdx = 0
	a.LastActiveIdx = -1
	a.ChosenArticles.Clear()
	a.initCodeBuffer("")

	var updatedProjects []string
	for _, p := range a.Projects {
		if p != a.ActiveProject {
			updatedProjects = append(updatedProjects, p)
		}
	}
	a.Projects = updatedProjects
	if len(a.Projects) > 0 {
		a.SwitchProject(a.Projects[0])
	} else {
		a.ActiveProject = ""
		a.LastSavedAt = ""
	}

	a.ShowExportModal = false
}

// ---------------- UI VIEW ----------------

func (a *App) View(c *ui.Context) {
	// Sync active article when user clicks or uses keyboard arrows
	if a.ActiveIdx != a.LastActiveIdx && a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		a.SelectArticle(a.ActiveIdx)
		c.Invalidate()
	}

	ui.Column(c).Fill().Background(a.Theme.WindowBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		// 1. macOS Style Toolbar with Font Selectors
		a.renderToolbar(c)

		// 2. Resizable Split Panes (Sidebar | Full Text | Summary)
		ui.Split(c, &a.SidebarWidth, func() {
			a.renderArticleListColumn(c)
		}, func() {
			ui.Split(c, &a.ReaderWidth, func() {
				a.renderFullTextColumn(c)
			}, func() {
				a.renderSummaryColumn(c)
			}).Fill()
		}).Grow(1)

		// 3. Modals
		a.renderExportModal(c)
		a.renderDeleteProjectModal(c)
		a.renderDeleteArticleModal(c)
		a.renderNewProjectModal(c)
	})
}

func (a *App) renderToolbar(c *ui.Context) {
	ui.Toolbar(c, func() {
		// Project Switcher
		if len(a.Projects) > 1 {
			prevIdx := a.ActiveProjectIdx
			ui.Segmented(c, &a.ActiveProjectIdx, a.Projects...).Label("Projects")
			if a.ActiveProjectIdx != prevIdx && a.ActiveProjectIdx < len(a.Projects) {
				a.SwitchProject(a.Projects[a.ActiveProjectIdx])
			}
		} else if len(a.Projects) == 1 {
			ui.Button(c, a.Projects[0])
		}

		if ui.Button(c, "➕").Tooltip("New Project").Clicked() {
			a.ShowNewProjectModal = true
		}

		if len(a.Projects) > 0 {
			if ui.Button(c, "🗑️").Tooltip("Delete Current Project").Clicked() {
				a.ShowDeleteProjectConfirm = true
			}
		}

		ui.Divider(c)

		// File Actions with Tooltips
		if ui.Button(c, "📥").Tooltip("Import HTML Article(s)").Clicked() {
			a.ImportHTMLFiles()
		}
		if ui.Button(c, "📑").Tooltip("Import Markdown Rollup").Clicked() {
			a.ImportMarkdownFile()
		}
		if ui.PrimaryButton(c, "📤").Tooltip("Export to Documents").Clicked() {
			a.ShowExportModal = true
		}

		ui.Spacer(c)

		// Auto-save Status
		if a.IsDirty {
			ui.Text(c, "Saving...").FontSize(11).TextColor(a.Theme.Accent)
		} else if a.LastSavedAt != "" {
			ui.Text(c, "Saved "+a.LastSavedAt).FontSize(11).TextColor(a.Theme.TextMuted)
		}

		ui.Divider(c)

		// Font selection and size selection
		ui.Text(c, "Font:").FontSize(11).TextColor(a.Theme.TextMuted)
		if len(a.AvailableFonts) > 0 {
			prevFont := a.SelectedFont
			ui.Select(c, &a.SelectedFont, a.AvailableFonts)
			if a.SelectedFont != prevFont {
				a.Settings.FontFamily = a.SelectedFont
				_ = a.Settings.Save()
			}
		}

		ui.Text(c, "Size:").FontSize(11).TextColor(a.Theme.TextMuted)
		prevSize := a.SelectedFontSize
		ui.Select(c, &a.SelectedFontSize, a.FontSizeOptions)
		if a.SelectedFontSize != prevSize {
			a.Settings.FontSize = a.SelectedFontSize
			_ = a.Settings.Save()
		}

		ui.Divider(c)

		// Theme Toggle
		themeIcon := "🌙"
		themeTip := "Switch to Dark Mode"
		if a.IsDarkMode {
			themeIcon = "☀️"
			themeTip = "Switch to Light Mode"
		}
		if ui.Button(c, themeIcon).Tooltip(themeTip).Clicked() {
			a.ToggleTheme()
		}
	}).Background(a.Theme.ToolbarBg).Label("Mebot Toolbar")
}

// Column 1: macOS Style Sidebar with Action Buttons at the Top
func (a *App) renderArticleListColumn(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Background(a.Theme.SidebarBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		// macOS Icon Action Bar at top of Sidebar
		ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
			if ui.Button(c, "🗑️").Tooltip("Delete Selected Article(s)").Clicked() {
				if a.ChosenArticles.Len() > 0 || a.CurrentArticle() != nil {
					a.ShowDeleteArticleConfirm = true
				}
			}
			if ui.Button(c, "📋").Tooltip("Copy Selected Titles").Clicked() {
				a.CopySelectedTitles()
			}
			if ui.Button(c, "📄").Tooltip("Copy Selected Content").Clicked() {
				a.CopySelectedContents()
			}

			ui.Spacer(c)

			if ui.Button(c, "☑️").Tooltip("Select All").Clicked() {
				for i := 0; i < len(a.Articles); i++ {
					a.ChosenArticles.Add(i)
				}
				a.SaveProjectSelection()
			}
			if ui.Button(c, "⬜").Tooltip("Clear Selection").Clicked() {
				a.ChosenArticles.Clear()
				a.SaveProjectSelection()
			}
		})

		// Native ui.List styled with macOS selection pill highlights
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

// Column 2: Full Text Area (macOS Crisp Reading Canvas)
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

// Column 3: Summary Area with Code Editor (SoftWrap) & Markdown Switch
func (a *App) renderSummaryColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Fill().Padding(16).Gap(10).Background(a.Theme.CardBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
			ui.Spacer(c)

			ui.Switch(c, &a.ShowMarkdownRender).Tooltip("Toggle Markdown Preview")
			ui.Text(c, "Preview").FontSize(11).TextColor(a.Theme.TextMuted)
		})

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		if a.ShowMarkdownRender {
			ui.Scroll(c).Grow(1).Children(func() {
				markdown.View(c, current.Summary, &a.mdOpts)
			})
		} else {
			if a.CodeState != nil {
				editor.View(c, a.CodeState).Grow(1).FillWidth()

				latestText := a.CodeState.Buffer.String()
				if latestText != a.LastBufferText {
					a.LastBufferText = latestText
					current.Summary = latestText
					a.TriggerDebouncedSave()
				}
			}
		}
	})
}

func (a *App) renderNewProjectModal(c *ui.Context) {
	ui.Modal(c, &a.ShowNewProjectModal, func() {
		ui.Column(c).Width(320).Padding(16).Gap(12).Background(a.Theme.SidebarBg).Radius(8).Justify(ui.Start).Children(func() {
			ui.Text(c, "New Project").FontSize(15).Bold().TextColor(a.Theme.Text)
			ui.TextInput(c, &a.NewProjectName).Placeholder("Project name...")

			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Create").Clicked() {
					a.CreateProject(a.NewProjectName)
				}
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowNewProjectModal = false
				}
			})
		})
	})
}

func (a *App) renderExportModal(c *ui.Context) {
	ui.Modal(c, &a.ShowExportModal, func() {
		ui.Column(c).Width(280).Padding(14).Gap(10).Background(a.Theme.SidebarBg).Radius(8).Justify(ui.Start).Children(func() {
			ui.Text(c, "Export Weekly Rollup").FontSize(15).Bold().TextColor(a.Theme.Text)

			ui.Text(c, "Select Export Date:").FontSize(11).TextColor(a.Theme.TextMuted)
			ui.Calendar(c, &a.SelectedDate)

			exportName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
			ui.Text(c, exportName).FontSize(12).Bold().TextColor(a.Theme.Accent)

			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Confirm & Export").Clicked() {
					a.PerformExport()
				}
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowExportModal = false
				}
			})
		})
	})
}

func (a *App) renderDeleteProjectModal(c *ui.Context) {
	ui.Modal(c, &a.ShowDeleteProjectConfirm, func() {
		ui.Column(c).Width(340).Padding(16).Gap(12).Background(a.Theme.SidebarBg).Radius(8).Justify(ui.Start).Children(func() {
			ui.Text(c, "Delete Project").FontSize(15).Bold().TextColor(a.Theme.Accent)
			ui.Text(c, fmt.Sprintf("Are you sure you want to delete '%s' and all its articles?", a.ActiveProject)).
				FontSize(13).TextColor(a.Theme.Text)

			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Yes, Delete").Clicked() {
					a.DeleteActiveProject()
				}
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowDeleteProjectConfirm = false
				}
			})
		})
	})
}

func (a *App) renderDeleteArticleModal(c *ui.Context) {
	ui.Modal(c, &a.ShowDeleteArticleConfirm, func() {
		count := a.ChosenArticles.Len()
		msg := "Are you sure you want to delete the selected article?"
		if count > 1 {
			msg = fmt.Sprintf("Are you sure you want to delete all %d selected articles?", count)
		} else if a.CurrentArticle() != nil {
			msg = fmt.Sprintf("Are you sure you want to delete '%s'?", a.CurrentArticle().Title)
		}

		ui.Column(c).Width(340).Padding(16).Gap(12).Background(a.Theme.SidebarBg).Radius(8).Justify(ui.Start).Children(func() {
			ui.Text(c, "Delete Article").FontSize(15).Bold().TextColor(a.Theme.Accent)
			ui.Text(c, msg).FontSize(13).TextColor(a.Theme.Text)

			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Yes, Delete").Clicked() {
					a.ConfirmDeleteChosenArticles()
				}
				if ui.Button(c, "Cancel").Clicked() {
					a.ShowDeleteArticleConfirm = false
				}
			})
		})
	})
}

func main() {
	os.Setenv("MYGO_GPU", "0")

	app := NewApp()

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Mebot",
			Width:   1760,
			Height:  1000,
			Content: ui.View(app.View),
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
