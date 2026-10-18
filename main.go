package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	AvailableFonts   []string
	SelectedFont     string
	FontSizeOptions  []string
	SelectedFontSize string

	SummaryIsMd     bool
	Theme           Theme
	SelectedDate    time.Time
	ShowExportModal bool

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
		Projects:         existingProjects,
		ActiveProject:    initialProject,
		Articles:         []*Article{},
		ActiveIdx:        0,
		AvailableFonts:   fonts,
		SelectedFont:     defaultFont,
		FontSizeOptions:  fontSizeOptions,
		SelectedFontSize: "16",
		SummaryIsMd:      false,
		Theme:            LightTheme(),
		SelectedDate:     time.Now(),
		ShowExportModal:  false,
		mdOpts:           markdown.Options{},
	}

	if initialProject != "" {
		app.SwitchProject(initialProject)
	}

	return app
}

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

func (a *App) SwitchProject(p string) {
	if a.ActiveProject != "" {
		_ = SaveProjectState(a.ActiveProject, a.Articles)
	}
	a.ActiveProject = p
	loaded, err := LoadProjectState(p)
	if err == nil {
		a.Articles = loaded
	} else {
		a.Articles = []*Article{}
	}
	a.ActiveIdx = 0
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
	_ = SaveProjectState(name, []*Article{})
}

func (a *App) ImportHTMLFiles() {
	files, err := PickFiles("Select HTML Article(s)", true, "*.html", "*.htm")
	if err != nil || len(files) == 0 {
		return
	}

	// Auto-create a project if none exists
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
	_ = SaveProjectState(a.ActiveProject, a.Articles)
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

	// Auto-create project if none exists (e.g. "2026-10-18 Economist.md" -> "Economist")
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
	_ = SaveProjectState(a.ActiveProject, a.Articles)
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
	}

	a.ShowExportModal = false
}

// ---------------- UI LAYOUT ----------------

func (a *App) View(c *ui.Context) {
	// Root container: sticks to top, stretches width
	ui.Column(c).Fill().Background(a.Theme.Bg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
		// Top fixed control toolbar
		a.renderTopControlBar(c)

		// 3-Column main area: sticks to top, stretches height
		ui.Row(c).Grow(1).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
			a.renderArticleListColumn(c)
			a.renderFullTextColumn(c)
			a.renderSummaryColumn(c)
		})

		a.renderExportModal(c)
	})
}

func (a *App) renderTopControlBar(c *ui.Context) {
	ui.Row(c).Padding(8, 12).Gap(8).Background(a.Theme.PanelBg).AlignItems(ui.Center).Children(func() {
		// Project Creator Input
		ui.TextInput(c, &a.NewProjectName).Placeholder("New project...").Width(140)
		if ui.Button(c, "+").Clicked() {
			a.CreateProject(a.NewProjectName)
		}

		if len(a.Projects) > 0 {
			ui.Select(c, &a.ActiveProject, a.Projects)
		}

		ui.Divider(c)

		// Imports & Export
		if ui.Button(c, "📥 Import HTML").Clicked() {
			a.ImportHTMLFiles()
		}
		if ui.Button(c, "📑 Import MD").Clicked() {
			a.ImportMarkdownFile()
		}
		if ui.PrimaryButton(c, "Export").Clicked() {
			a.ShowExportModal = true
		}

		ui.Spacer(c)

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
	})
}

// Column 1: Leftmost Article List
func (a *App) renderArticleListColumn(c *ui.Context) {
	ui.Column(c).Width(300).Padding(10).Gap(6).Background(a.Theme.PanelBg).Justify(ui.Start).AlignItems(ui.Stretch).Children(func() {
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
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Checkbox(c, &art.Selected, "")
						itemBtn := ui.Button(c, art.Title)
						if idx == a.ActiveIdx {
							itemBtn.Bold()
						}
						if itemBtn.Clicked() {
							a.ActiveIdx = idx
						}
					})
				}
			})
		})
	})
}

// Column 2: Full Text Area (Aligned to top)
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

// Column 3: Summary Area (Aligned to top)
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
			ui.TextArea(c, &current.Summary).
				Grow(1).
				FontSize(a.CurrentFontSize())
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
