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
	"golang.design/x/clipboard"
)

type App struct {
	Projects       []string
	ActiveProject  string
	NewProjectName string
	Articles       []*Article
	ActiveIdx      int

	// Fonts & Numerical Font Sizes (*string for ui.Select)
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
	_ = clipboard.Init()
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
		Theme:            DarkTheme(),
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
	if a.ActiveProject == "" {
		return
	}
	files, err := PickFiles("Select HTML Article(s)", true, "HTML files (*.html *.htm) | *.html *.htm")
	if err != nil || len(files) == 0 {
		return
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
	if a.ActiveProject == "" {
		return
	}
	files, err := PickFiles("Select Markdown Rollup", false, "Markdown files (*.md) | *.md")
	if err != nil || len(files) == 0 {
		return
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		return
	}

	parsed, err := ParseMarkdownRollup(string(data))
	if err == nil && len(parsed) > 0 {
		a.Articles = append(a.Articles, parsed...)
		_ = SaveProjectState(a.ActiveProject, a.Articles)
	}
}

func (a *App) CopyTitlesForGoogleTasks() {
	var titles []string
	for _, art := range a.Articles {
		if art.Selected {
			titles = append(titles, art.Title)
		}
	}
	if len(titles) > 0 {
		clipboard.Write(clipboard.FmtText, []byte(strings.Join(titles, "\n")))
	}
}

func (a *App) CopyContentForChatGPT() {
	var sb strings.Builder
	for _, art := range a.Articles {
		if art.Selected {
			sb.WriteString(fmt.Sprintf("# %s\n\n%s\n\n---\n\n", art.Title, art.FullText))
		}
	}
	if sb.Len() > 0 {
		clipboard.Write(clipboard.FmtText, []byte(sb.String()))
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

	// Clean up temporary project state file
	_ = RemoveProjectState(a.ActiveProject)
	a.Articles = []*Article{}
	a.ActiveIdx = 0

	// Remove project from list
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

func (a *App) View(c *ui.Context) {
	ui.Column(c).Fill().Background(a.Theme.Bg).Children(func() {
		// 1. TOP TOOLBAR
		a.renderHeader(c)

		// 2. MAIN THREE-COLUMN VIEW
		ui.Row(c).Fill().Children(func() {
			// COLUMN 1: Leftmost Sidebar (Projects, Imports, Actions, Article list)
			a.renderLeftSidebar(c)

			// COLUMN 2: Full Text Reading Pane
			a.renderFullTextPane(c)

			// COLUMN 3: Summary Pane
			a.renderSummaryPane(c)
		})

		// 3. EXPORT MODAL OVERLAY (managed by ui.Modal with &a.ShowExportModal)
		a.renderExportModal(c)
	})
}

func (a *App) renderHeader(c *ui.Context) {
	ui.Row(c).Padding(10).Gap(10).Background(a.Theme.PanelBg).Children(func() {
		ui.Text(c, "Mebot").FontSize(18).Bold().TextColor(a.Theme.Accent)

		ui.Spacer(c)

		// Font selector dropdown (binds to *string)
		ui.Text(c, "Font:").FontSize(12).TextColor(a.Theme.TextMuted)
		if len(a.AvailableFonts) > 0 {
			ui.Select(c, &a.SelectedFont, a.AvailableFonts)
		}

		// Numerical Font Size dropdown (binds to *string)
		ui.Text(c, "Size:").FontSize(12).TextColor(a.Theme.TextMuted)
		ui.Select(c, &a.SelectedFontSize, a.FontSizeOptions)

		// Dark / Light Mode Switch
		modeLabel := "🌙 Dark"
		if a.Theme.IsDark {
			modeLabel = "☀️ Light"
		}
		if ui.Button(c, modeLabel).Clicked() {
			if a.Theme.IsDark {
				a.Theme = LightTheme()
			} else {
				a.Theme = DarkTheme()
			}
		}
	})
}

func (a *App) renderLeftSidebar(c *ui.Context) {
	ui.Column(c).Width(320).Padding(10).Gap(8).Background(a.Theme.PanelBg).Children(func() {
		// Project Creator & Selector
		ui.Text(c, "PROJECTS").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
		ui.Row(c).Gap(4).Children(func() {
			ui.TextInput(c, &a.NewProjectName).Placeholder("New project...").Grow(1)
			if ui.Button(c, "+").Clicked() {
				a.CreateProject(a.NewProjectName)
			}
		})

		if len(a.Projects) > 0 {
			ui.Row(c).Gap(4).Children(func() {
				for _, p := range a.Projects {
					btn := ui.Button(c, p)
					if p == a.ActiveProject {
						btn.Bold()
					}
					if btn.Clicked() {
						a.SwitchProject(p)
					}
				}
			})
		}

		ui.Divider(c)

		// Imports
		ui.Row(c).Gap(6).Children(func() {
			if ui.Button(c, "📥 Import HTML").Clicked() {
				a.ImportHTMLFiles()
			}
			if ui.Button(c, "📑 Import MD").Clicked() {
				a.ImportMarkdownFile()
			}
		})

		// Clipboard Actions
		ui.Row(c).Gap(6).Children(func() {
			if ui.Button(c, "Tasks 📋").Clicked() {
				a.CopyTitlesForGoogleTasks()
			}
			if ui.Button(c, "Prompt 🤖").Clicked() {
				a.CopyContentForChatGPT()
			}
		})

		// Multi-select controls
		ui.Row(c).Children(func() {
			ui.Text(c, fmt.Sprintf("Articles (%d)", len(a.Articles))).Bold()
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

		// Article Scrollable List
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				for idx, art := range a.Articles {
					ui.Row(c).Gap(6).Children(func() {
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

		// Export Trigger (Opens Modal)
		if ui.PrimaryButton(c, "Export Rollup...").Clicked() {
			a.ShowExportModal = true
		}
	})
}

func (a *App) renderFullTextPane(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Grow(1).Padding(14).Gap(10).Children(func() {
		ui.Text(c, "FULL TEXT").FontSize(11).Bold().TextColor(a.Theme.TextMuted)

		if current == nil {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "No article selected").TextColor(a.Theme.TextMuted)
			})
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

func (a *App) renderSummaryPane(c *ui.Context) {
	current := a.CurrentArticle()
	ui.Column(c).Grow(1).Padding(14).Gap(10).Background(a.Theme.CardBg).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "SUMMARY").FontSize(11).Bold().TextColor(a.Theme.TextMuted)
			ui.Spacer(c)
			toggleLabel := "View Rendered"
			if a.SummaryIsMd {
				toggleLabel = "Edit Raw Code"
			}
			if ui.Button(c, toggleLabel).Clicked() {
				a.SummaryIsMd = !a.SummaryIsMd
			}
		})

		if current == nil {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "No article selected").TextColor(a.Theme.TextMuted)
			})
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
		ui.Column(c).Width(400).Padding(16).Gap(12).Background(a.Theme.PanelBg).Children(func() {
			ui.Text(c, "Export Weekly Rollup").FontSize(16).Bold().TextColor(a.Theme.Text)

			ui.Text(c, "Select Export Date:").FontSize(12).TextColor(a.Theme.TextMuted)
			ui.Calendar(c, &a.SelectedDate)

			exportName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
			ui.Text(c, "File: "+exportName).FontSize(13).Bold().TextColor(a.Theme.Accent)

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
