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
	"github.com/egoist/mygo/ui/markdown"
)

type App struct {
	Projects       []string
	ActiveProject  string
	NewProjectName string
	Articles       []*Article
	ActiveIdx      int

	// Auto-save & Debounce State
	LastSavedAt   string
	IsDirty       bool
	debounceTimer *time.Timer
	saveMu        sync.Mutex

	AvailableFonts   []string
	SelectedFont     string
	FontSizeOptions  []string
	SelectedFontSize string

	SummaryIsMd              bool
	Theme                    Theme
	SelectedDate             time.Time
	ShowExportModal          bool
	ShowDeleteProjectConfirm bool

	mdOpts markdown.Options
}

func NewApp() *App {
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
		ActiveProject:            "", // Left empty so initial boot won't overwrite disk
		Articles:                 []*Article{},
		ActiveIdx:                0,
		AvailableFonts:           fonts,
		SelectedFont:             defaultFont,
		FontSizeOptions:          fontSizeOptions,
		SelectedFontSize:         "16",
		SummaryIsMd:              false,
		Theme:                    LightTheme(),
		SelectedDate:             time.Now(),
		ShowExportModal:          false,
		ShowDeleteProjectConfirm: false,
		mdOpts:                   markdown.Options{},
	}

	// Safely load initial project without wiping its disk state
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

func (a *App) CurrentArticle() *Article {
	if a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		return a.Articles[a.ActiveIdx]
	}
	return nil
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

	// Only save prior project if it genuinely had unsaved changes
	if a.ActiveProject != "" && a.ActiveProject != newProject && a.IsDirty {
		_ = SaveProjectState(a.ActiveProject, a.Articles)
		a.IsDirty = false
	}

	a.ActiveProject = newProject
	loaded, err := LoadProjectState(newProject)
	if err == nil {
		a.Articles = loaded
	} else {
		a.Articles = []*Article{}
	}
	a.ActiveIdx = 0
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

	if len(a.Projects) > 0 {
		a.SwitchProject(a.Projects[0])
	} else {
		a.ActiveProject = ""
		a.LastSavedAt = ""
		a.IsDirty = false
	}
	a.ShowDeleteProjectConfirm = false
}

func (a *App) DeleteArticle(idx int) {
	if idx < 0 || idx >= len(a.Articles) {
		return
	}
	a.Articles = append(a.Articles[:idx], a.Articles[idx+1:]...)
	if a.ActiveIdx >= len(a.Articles) {
		a.ActiveIdx = len(a.Articles) - 1
	}
	if a.ActiveIdx < 0 {
		a.ActiveIdx = 0
	}
	a.SaveNow()
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
	fileName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
	home, _ := os.UserHomeDir()
	outPath := filepath.Join(home, "Documents", fileName)

	md := SerializeToMarkdown(a.Articles)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	_ = RemoveProjectState(a.ActiveProject)
	a.Articles = []*Article{}
	a.ActiveIdx = 0

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
	ui.Column(c).Fill().Background(a.Theme.Bg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		a.renderTopControlBar(c)

		ui.Row(c).Grow(1).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
			a.renderArticleListColumn(c)
			a.renderFullTextColumn(c)
			a.renderSummaryColumn(c)
		})

		a.renderExportModal(c)
		a.renderDeleteProjectModal(c)
	})
}

func (a *App) renderTopControlBar(c *ui.Context) {
	ui.Row(c).Padding(8, 12).Gap(8).Background(a.Theme.PanelBg).AlignItems(ui.Center).Children(func() {
		// Project Creator
		ui.TextInput(c, &a.NewProjectName).Placeholder("New project...").Width(130)
		if ui.Button(c, "+").Clicked() {
			a.CreateProject(a.NewProjectName)
		}

		// Project Selector with change detection
		if len(a.Projects) > 0 {
			prevProject := a.ActiveProject
			ui.Select(c, &a.ActiveProject, a.Projects)
			if a.ActiveProject != prevProject {
				selected := a.ActiveProject
				a.ActiveProject = prevProject // Revert temporarily so SwitchProject can cleanly save it
				a.SwitchProject(selected)
			}

			if ui.Button(c, "🗑 Project").Clicked() {
				a.ShowDeleteProjectConfirm = true
			}
		}

		ui.Divider(c)

		// Imports & Export
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

		// Auto-save Indicator
		if a.IsDirty {
			ui.Text(c, "● Saving in 3s...").FontSize(11).TextColor(a.Theme.Accent)
		} else if a.LastSavedAt != "" {
			ui.Text(c, fmt.Sprintf("✓ Saved at %s", a.LastSavedAt)).FontSize(11).TextColor(a.Theme.TextMuted)
		}

		ui.Divider(c)

		// Font Controls
		ui.Text(c, "Font:").FontSize(12).TextColor(a.Theme.TextMuted)
		if len(a.AvailableFonts) > 0 {
			ui.Select(c, &a.SelectedFont, a.AvailableFonts)
		}

		ui.Text(c, "Size:").FontSize(12).TextColor(a.Theme.TextMuted)
		ui.Select(c, &a.SelectedFontSize, a.FontSizeOptions)

		// Theme Switch
		modeLabel := "🌙 Dark"
		if a.Theme.IsDark {
			modeLabel = "☀️ Light"
		}
		if ui.Button(c, modeLabel).Clicked() {
			a.ToggleTheme()
		}
	})
}

func (a *App) renderArticleListColumn(c *ui.Context) {
	ui.Column(c).Width(320).Padding(10).Gap(6).Background(a.Theme.PanelBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
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
							a.ActiveIdx = idx
						}

						if ui.Button(c, "✕").Clicked() {
							a.DeleteArticle(idx)
						}
					})
				}
			})
		})
	})
}

func (a *App) renderFullTextColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Grow(1).Padding(14).Gap(10).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, "FULL TEXT").FontSize(11).Bold().TextColor(a.Theme.TextMuted)

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		ui.Text(c, current.Title).FontSize(a.CurrentFontSize() + 4).Bold().TextColor(a.Theme.Text)

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Text(c, current.FullText).
				FontSize(a.CurrentFontSize()).
				TextColor(a.Theme.Text)
		})
	})
}

func (a *App) renderSummaryColumn(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Grow(1).Padding(14).Gap(10).Background(a.Theme.CardBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
			ui.Spacer(c)
			toggleLabel := "View Rendered"
			if a.SummaryIsMd {
				toggleLabel = "Edit Raw"
			}
			if ui.Button(c, toggleLabel).Clicked() {
				a.SummaryIsMd = !a.SummaryIsMd
			}
		})

		if current == nil {
			ui.Text(c, "No article selected.").TextColor(a.Theme.TextMuted)
			return
		}

		if a.SummaryIsMd {
			ui.Scroll(c).Grow(1).Children(func() {
				markdown.View(c, current.Summary, &a.mdOpts)
			})
		} else {
			prevSummary := current.Summary
			ui.TextArea(c, &current.Summary).
				Grow(1).
				FontSize(a.CurrentFontSize())

			if current.Summary != prevSummary {
				a.TriggerDebouncedSave()
			}
		}
	})
}

func (a *App) renderExportModal(c *ui.Context) {
	ui.Modal(c, &a.ShowExportModal, func() {
		ui.Column(c).Width(420).Padding(18).Gap(12).Background(a.Theme.PanelBg).Justify(ui.Start).Children(func() {
			ui.Text(c, "Export Weekly Rollup").FontSize(16).Bold().TextColor(a.Theme.Text)

			ui.Text(c, "Select Export Date:").FontSize(12).TextColor(a.Theme.TextMuted)
			ui.Calendar(c, &a.SelectedDate)

			exportName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
			ui.Text(c, "Output: Documents/"+exportName).FontSize(13).Bold().TextColor(a.Theme.Accent)

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
		ui.Column(c).Width(360).Padding(18).Gap(12).Background(a.Theme.PanelBg).Justify(ui.Start).Children(func() {
			ui.Text(c, "Delete Project").FontSize(16).Bold().TextColor(a.Theme.Accent)
			ui.Text(c, fmt.Sprintf("Are you sure you want to delete project '%s' and all its saved state?", a.ActiveProject)).
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

func main() {
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
