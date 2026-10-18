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
	"github.com/egoist/mygo/ui/richtext"
	"golang.design/x/clipboard"
)

type App struct {
	Projects         []string
	ActiveProjectIdx int
	ActiveProject    string
	NewProjectName   string
	Articles         []*Article
	ActiveIdx        int
	LastClickedIdx   int

	// Rich Text Editor Document
	SummaryDoc *richtext.Document

	// Auto-save & Debounce State
	LastSavedAt   string
	IsDirty       bool
	debounceTimer *time.Timer
	saveMu        sync.Mutex

	AvailableFonts   []string
	SelectedFont     string
	FontSizeOptions  []string
	SelectedFontSize string

	Theme                    Theme
	SelectedDate             time.Time
	ShowExportModal          bool
	ShowDeleteProjectConfirm bool

	// Article Deletion Confirmation
	ShowDeleteArticleConfirm bool
	ArticleIdxToDelete       int
}

func NewApp() *App {
	_ = clipboard.Init()
	mygo.Theme.SetSource(mygo.ThemeLight)

	existingProjects := DiscoverProjects()
	initialProject := ""
	if len(existingProjects) > 0 {
		initialProject = existingProjects[0]
	}

	fonts := GetLinuxSystemFonts()
	defaultFont := "Serif"
	if len(fonts) > 0 {
		defaultFont = fonts[0]
	}

	fontSizeOptions := []string{"12", "13", "14", "15", "16", "17", "18", "20", "22", "24", "28", "32"}

	app := &App{
		Projects:                 existingProjects,
		ActiveProjectIdx:         0,
		ActiveProject:            "",
		Articles:                 []*Article{},
		ActiveIdx:                0,
		LastClickedIdx:           0,
		SummaryDoc:               richtext.NewDocument(richtext.NewText("")),
		AvailableFonts:           fonts,
		SelectedFont:             defaultFont,
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         "16",
		Theme:                    LightTheme(),
		SelectedDate:             time.Now(),
		ShowExportModal:          false,
		ShowDeleteProjectConfirm: false,
		ShowDeleteArticleConfirm: false,
		ArticleIdxToDelete:       -1,
	}

	if initialProject != "" {
		app.SwitchProject(initialProject)
	}

	return app
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
	return "Serif"
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

func (a *App) syncSummaryDoc() {
	cur := a.CurrentArticle()
	if cur != nil {
		a.SummaryDoc = richtext.NewDocument(richtext.NewText(cur.Summary))
	} else {
		a.SummaryDoc = richtext.NewDocument(richtext.NewText(""))
	}
}

func (a *App) SelectArticle(idx int) {
	if a.ActiveIdx == idx {
		return
	}
	cur := a.CurrentArticle()
	if cur != nil && a.SummaryDoc != nil {
		cur.Summary = a.SummaryDoc.Text().Markdown()
	}

	a.ActiveIdx = idx
	a.syncSummaryDoc()
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
	if cur != nil && a.SummaryDoc != nil {
		cur.Summary = a.SummaryDoc.Text().Markdown()
	}

	if a.ActiveProject != "" {
		_ = SaveProjectState(a.ActiveProject, a.Articles)
		a.LastSavedAt = time.Now().Format("15:04:05")
		a.IsDirty = false
	}
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
	a.ActiveIdx = 0
	a.LastClickedIdx = 0
	a.syncSummaryDoc()

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
			return
		}
	}
	a.Projects = append(a.Projects, name)
	a.SwitchProject(name)
	a.NewProjectName = ""
}

func (a *App) DeleteActiveProject() {
	if a.ActiveProject == "" {
		return
	}

	target := a.ActiveProject
	_ = RemoveProjectState(target)

	var updated []string
	for _, p := range a.Projects {
		if p != target {
			updated = append(updated, p)
		}
	}
	a.Projects = updated
	a.Articles = []*Article{}
	a.ActiveIdx = 0
	a.syncSummaryDoc()

	if len(a.Projects) > 0 {
		a.SwitchProject(a.Projects[0])
	} else {
		a.ActiveProject = ""
		a.LastSavedAt = ""
		a.IsDirty = false
	}
	a.ShowDeleteProjectConfirm = false
}

func (a *App) RequestDeleteArticle(idx int) {
	a.ArticleIdxToDelete = idx
	a.ShowDeleteArticleConfirm = true
}

func (a *App) ConfirmDeleteArticle() {
	idx := a.ArticleIdxToDelete
	if idx < 0 || idx >= len(a.Articles) {
		a.ShowDeleteArticleConfirm = false
		return
	}
	a.Articles = append(a.Articles[:idx], a.Articles[idx+1:]...)
	if a.ActiveIdx >= len(a.Articles) {
		a.ActiveIdx = len(a.Articles) - 1
	}
	if a.ActiveIdx < 0 {
		a.ActiveIdx = 0
	}
	a.syncSummaryDoc()
	a.SaveNow()
	a.ShowDeleteArticleConfirm = false
}

func (a *App) CopySelectedTitles() {
	var titles []string
	for _, art := range a.Articles {
		if art.Selected {
			titles = append(titles, art.Title)
		}
	}
	if len(titles) == 0 && a.CurrentArticle() != nil {
		titles = append(titles, a.CurrentArticle().Title)
	}
	if len(titles) > 0 {
		clipboard.Write(clipboard.FmtText, []byte(strings.Join(titles, "\n")))
	}
}

func (a *App) CopySelectedContents() {
	var sb strings.Builder
	hasAny := false
	for _, art := range a.Articles {
		if art.Selected {
			sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n---\n\n", art.Title, art.FullText))
			hasAny = true
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
	a.syncSummaryDoc()
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
	a.syncSummaryDoc()
	a.SaveNow()
}

func (a *App) ToggleTheme() {
	if a.Theme.IsDark {
		a.Theme = LightTheme()
		mygo.Theme.SetSource(mygo.ThemeLight)
	} else {
		a.Theme = DarkTheme()
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
}

func (a *App) PerformExport() {
	if len(a.Articles) == 0 || a.ActiveProject == "" {
		a.ShowExportModal = false
		return
	}

	cur := a.CurrentArticle()
	if cur != nil && a.SummaryDoc != nil {
		cur.Summary = a.SummaryDoc.Text().Markdown()
	}

	fileName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
	home, _ := os.UserHomeDir()
	outPath := filepath.Join(home, "Documents", fileName)

	md := SerializeToMarkdown(a.Articles)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	_ = RemoveProjectState(a.ActiveProject)
	a.Articles = []*Article{}
	a.ActiveIdx = 0
	a.syncSummaryDoc()

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
	ui.Column(c).Fill().Background(a.Theme.ReadingBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		// 1. TOOLBAR with distinct background
		a.renderToolbar(c)

		// 2. MAIN WORKSPACE
		ui.Row(c).Grow(1).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
			a.renderArticleListColumn(c)
			a.renderFullTextColumn(c)
			a.renderSummaryColumn(c)
		})

		// 3. DIALOG MODALS
		a.renderExportModal(c)
		a.renderDeleteProjectModal(c)
		a.renderDeleteArticleModal(c)
	})
}

func (a *App) renderToolbar(c *ui.Context) {
	ui.Toolbar(c, func() {
		ui.TextInput(c, &a.NewProjectName).Placeholder("New project...").Width(120)
		if ui.Button(c, "+").Clicked() {
			a.CreateProject(a.NewProjectName)
		}

		if len(a.Projects) > 0 {
			prevIdx := a.ActiveProjectIdx
			ui.Segmented(c, &a.ActiveProjectIdx, a.Projects...).Label("Projects")
			if a.ActiveProjectIdx != prevIdx && a.ActiveProjectIdx < len(a.Projects) {
				a.SwitchProject(a.Projects[a.ActiveProjectIdx])
			}

			if ui.Button(c, "🗑").Clicked() {
				a.ShowDeleteProjectConfirm = true
			}
		}

		if ui.Button(c, "📥 HTML").Clicked() {
			a.ImportHTMLFiles()
		}
		if ui.Button(c, "📑 MD").Clicked() {
			a.ImportMarkdownFile()
		}
		if ui.PrimaryButton(c, "Export").Clicked() {
			a.ShowExportModal = true
		}

		ui.Spacer(c)

		if a.IsDirty {
			ui.Text(c, "● Saving in 3s...").FontSize(11).TextColor(a.Theme.Accent)
		} else if a.LastSavedAt != "" {
			ui.Text(c, fmt.Sprintf("✓ Saved at %s", a.LastSavedAt)).FontSize(11).TextColor(a.Theme.TextMuted)
		}

		ui.Text(c, "Font:").FontSize(12).TextColor(a.Theme.TextMuted)
		if len(a.AvailableFonts) > 0 {
			ui.Select(c, &a.SelectedFont, a.AvailableFonts)
		}

		ui.Text(c, "Size:").FontSize(12).TextColor(a.Theme.TextMuted)
		ui.Select(c, &a.SelectedFontSize, a.FontSizeOptions)

		modeLabel := "🌙 Dark"
		if a.Theme.IsDark {
			modeLabel = "☀️ Light"
		}
		if ui.Button(c, modeLabel).Clicked() {
			a.ToggleTheme()
		}
	}).Background(a.Theme.ToolbarBg).Label("Mebot Controls")
}

func (a *App) renderArticleListColumn(c *ui.Context) {
	ui.Column(c).Width(330).Padding(10).Gap(6).Background(a.Theme.SidebarBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, fmt.Sprintf("Articles (%d)", len(a.Articles))).Bold().TextColor(a.Theme.Text)
			ui.Spacer(c)

			if ui.Button(c, "All").Clicked() {
				for _, art := range a.Articles {
					art.Selected = true
				}
			}
			if ui.Button(c, "None").Clicked() {
				for _, art := range a.Articles {
					art.Selected = false
				}
			}

			if ui.Button(c, "📋 Titles").Clicked() {
				a.CopySelectedTitles()
			}
			if ui.Button(c, "📄 Content").Clicked() {
				a.CopySelectedContents()
			}
		})

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(4).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
				if len(a.Articles) == 0 {
					ui.Text(c, "No articles yet.").FontSize(12).TextColor(a.Theme.TextMuted)
				}
				for idx, art := range a.Articles {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Checkbox(c, &art.Selected, "")

						itemBtn := ui.Button(c, art.Title).Grow(1)
						if idx == a.ActiveIdx {
							itemBtn.Bold()
						}

						if itemBtn.Clicked() {
							isShift := (c.Modifiers() & ui.Shift) != 0
							if isShift && a.LastClickedIdx >= 0 && a.LastClickedIdx < len(a.Articles) {
								start := a.LastClickedIdx
								end := idx
								if start > end {
									start, end = end, start
								}
								for i := start; i <= end; i++ {
									a.Articles[i].Selected = true
								}
							} else {
								a.LastClickedIdx = idx
							}
							a.SelectArticle(idx)
						}

						if ui.Button(c, "✕").Clicked() {
							a.RequestDeleteArticle(idx)
						}
					})
				}
			})
		})
	})
}

func (a *App) renderFullTextColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Grow(1).Padding(16).Gap(10).Background(a.Theme.ReadingBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
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
	ui.Column(c).Grow(1).Padding(14).Gap(8).Background(a.Theme.EditorBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		if a.SummaryDoc == nil {
			a.SummaryDoc = richtext.NewDocument(richtext.NewText(current.Summary))
		}

		a.renderRichTextToolbar(c)

		editorOpts := richtext.EditorOptions{
			Font: ui.Font{
				Family: a.CurrentFontFamily(),
				Size:   a.CurrentFontSize(),
			},
		}

		if richtext.Editor(c, a.SummaryDoc, editorOpts).Fill().Changed() {
			current.Summary = a.SummaryDoc.Text().Markdown()
			a.TriggerDebouncedSave()
		}
	})
}

func (a *App) renderRichTextToolbar(c *ui.Context) {
	f := a.SummaryDoc.Formatting()

	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "↶").KeepFocus().Clicked() {
			a.SummaryDoc.Undo()
		}
		if ui.Button(c, "↷").KeepFocus().Clicked() {
			a.SummaryDoc.Redo()
		}

		ui.Divider(c)

		btnBold := ui.Button(c, "B").Bold().KeepFocus()
		if f.Bold == richtext.FormatOn {
			btnBold.Background(a.Theme.Accent)
		}
		if btnBold.Clicked() {
			a.SummaryDoc.ToggleBold()
		}

		btnItalic := ui.Button(c, "I").Italic().KeepFocus()
		if f.Italic == richtext.FormatOn {
			btnItalic.Background(a.Theme.Accent)
		}
		if btnItalic.Clicked() {
			a.SummaryDoc.ToggleItalic()
		}

		btnUnderline := ui.Button(c, "U").KeepFocus()
		if f.Underline == richtext.FormatOn {
			btnUnderline.Background(a.Theme.Accent)
		}
		if btnUnderline.Clicked() {
			a.SummaryDoc.ToggleUnderline()
		}

		btnStrike := ui.Button(c, "S̶").KeepFocus()
		if f.Strikethrough == richtext.FormatOn {
			btnStrike.Background(a.Theme.Accent)
		}
		if btnStrike.Clicked() {
			a.SummaryDoc.ToggleStrikethrough()
		}

		btnCode := ui.Button(c, "</>").KeepFocus()
		if f.Code == richtext.FormatOn {
			btnCode.Background(a.Theme.Accent)
		}
		if btnCode.Clicked() {
			a.SummaryDoc.ToggleCode()
		}

		ui.Divider(c)

		if ui.Button(c, "H1").KeepFocus().Clicked() {
			a.SummaryDoc.SetHeading(1)
		}
		if ui.Button(c, "H2").KeepFocus().Clicked() {
			a.SummaryDoc.SetHeading(2)
		}

		if ui.Button(c, "• List").KeepFocus().Clicked() {
			a.SummaryDoc.ToggleList(richtext.Bullets)
		}
		if ui.Button(c, "1. List").KeepFocus().Clicked() {
			a.SummaryDoc.ToggleList(richtext.Numbers)
		}
		if ui.Button(c, "☑ Task").KeepFocus().Clicked() {
			a.SummaryDoc.ToggleList(richtext.Tasks)
		}

		if ui.Button(c, "“ Quote").KeepFocus().Clicked() {
			a.SummaryDoc.ToggleQuote()
		}
		if ui.Button(c, "Code Block").KeepFocus().Clicked() {
			a.SummaryDoc.ToggleCodeBlock()
		}
	})
}

func (a *App) renderExportModal(c *ui.Context) {
	ui.Modal(c, &a.ShowExportModal, func() {
		ui.Column(c).Width(280).Padding(14).Gap(10).Background(a.Theme.SidebarBg).Justify(ui.Start).Children(func() {
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
		ui.Column(c).Width(340).Padding(16).Gap(12).Background(a.Theme.SidebarBg).Justify(ui.Start).Children(func() {
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
		title := "this article"
		if a.ArticleIdxToDelete >= 0 && a.ArticleIdxToDelete < len(a.Articles) {
			title = fmt.Sprintf("'%s'", a.Articles[a.ArticleIdxToDelete].Title)
		}

		ui.Column(c).Width(340).Padding(16).Gap(12).Background(a.Theme.SidebarBg).Justify(ui.Start).Children(func() {
			ui.Text(c, "Delete Article").FontSize(15).Bold().TextColor(a.Theme.Accent)
			ui.Text(c, fmt.Sprintf("Are you sure you want to delete %s?", title)).
				FontSize(13).TextColor(a.Theme.Text)

			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Yes, Delete").Clicked() {
					a.ConfirmDeleteArticle()
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
			Width:   1500,
			Height:  900,
			Content: ui.View(app.View),
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
