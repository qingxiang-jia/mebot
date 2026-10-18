package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"golang.design/x/clipboard"
)

type App struct {
	Projects      []string
	ActiveProject string
	Articles      []*Article
	ActiveIdx     int

	// Settings & Controls
	Theme        Theme
	FontSize     float32
	FontFamily   string
	SummaryIsMd  bool // Toggle between raw editor & rendered markdown
	SelectedDate time.Time

	// MyGo markdown renderer options
	mdOpts markdown.Options
}

func NewApp() *App {
	_ = clipboard.Init()
	return &App{
		Projects:      []string{"Economist", "WSJ", "ForeignAffairs"},
		ActiveProject: "Economist",
		Articles:      []*Article{},
		ActiveIdx:     0,
		Theme:         DarkTheme(),
		FontSize:      16,
		FontFamily:    "Serif",
		SummaryIsMd:   false,
		SelectedDate:  time.Now(),
		mdOpts:        markdown.Options{},
	}
}

func (a *App) CurrentArticle() *Article {
	if a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		return a.Articles[a.ActiveIdx]
	}
	return nil
}

func (a *App) SwitchProject(p string) {
	_ = SaveProjectState(a.ActiveProject, a.Articles)
	a.ActiveProject = p
	loaded, err := LoadProjectState(p)
	if err == nil {
		a.Articles = loaded
	} else {
		a.Articles = []*Article{}
	}
	a.ActiveIdx = 0
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

func (a *App) ExportRollup() {
	if len(a.Articles) == 0 {
		return
	}
	fileName := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
	home, _ := os.UserHomeDir()
	outPath := filepath.Join(home, "Documents", fileName)

	md := SerializeToMarkdown(a.Articles)
	_ = os.WriteFile(outPath, []byte(md), 0644)

	// Clean up temp state file upon export
	_ = RemoveProjectState(a.ActiveProject)
	a.Articles = []*Article{}
	a.ActiveIdx = 0
}

// Layout view
func (a *App) View(c *ui.Context) {
	ui.Column(c).Fill().Background(a.Theme.Bg).Children(func() {
		// 1. TOP HEADER / TOOLBAR
		a.renderHeader(c)

		// 2. MAIN SPLIT BODY
		ui.Row(c).Fill().Children(func() {
			// Left Sidebar (Projects, Articles, Actions, Calendar)
			a.renderSidebar(c)

			// Right Side-by-Side Area (Full Text | Summary)
			a.renderReadingArea(c)
		})
	})
}

func (a *App) renderHeader(c *ui.Context) {
	ui.Row(c).Padding(10).Gap(8).Background(a.Theme.PanelBg).Children(func() {
		ui.Text(c, "Mebot").FontSize(18).Bold().TextColor(a.Theme.Accent)

		// Project Selector Buttons
		for _, p := range a.Projects {
			btn := ui.Button(c, p)
			if p == a.ActiveProject {
				btn.Bold()
			}
			if btn.Clicked() {
				a.SwitchProject(p)
			}
		}

		ui.Spacer(c)

		// Typography Controls
		if ui.Button(c, "A-").Clicked() && a.FontSize > 12 {
			a.FontSize -= 1
		}
		if ui.Button(c, "A+").Clicked() && a.FontSize < 28 {
			a.FontSize += 1
		}
		if ui.Button(c, a.FontFamily).Clicked() {
			if a.FontFamily == "Serif" {
				a.FontFamily = "Sans"
			} else {
				a.FontFamily = "Serif"
			}
		}

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

func (a *App) renderSidebar(c *ui.Context) {
	ui.Column(c).Width(320).Padding(12).Gap(8).Background(a.Theme.PanelBg).Children(func() {
		// Action buttons
		ui.Row(c).Gap(6).Children(func() {
			if ui.Button(c, "Tasks 📋").Clicked() {
				a.CopyTitlesForGoogleTasks()
			}
			if ui.Button(c, "Prompt 🤖").Clicked() {
				a.CopyContentForChatGPT()
			}
		})

		// Article List Header
		ui.Row(c).Children(func() {
			ui.Text(c, fmt.Sprintf("Articles (%d)", len(a.Articles))).Bold()
			ui.Spacer(c)
			if ui.Button(c, "Select All").Clicked() {
				for _, art := range a.Articles {
					art.Selected = true
				}
			}
		})

		// Article Scrollable List using ui.Scroll
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

		// Export Calendar Section
		ui.Text(c, "Export Date:").FontSize(12).TextColor(a.Theme.TextMuted)
		ui.Calendar(c, &a.SelectedDate)

		if ui.PrimaryButton(c, "Export & Clear Project").Clicked() {
			a.ExportRollup()
		}
	})
}

func (a *App) renderReadingArea(c *ui.Context) {
	current := a.CurrentArticle()
	if current == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "No article selected").TextColor(a.Theme.TextMuted)
		})
		return
	}

	ui.Row(c).Grow(1).Padding(12).Gap(12).Children(func() {
		// LEFT COLUMN: Full Text
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, current.Title).FontSize(a.FontSize + 4).Bold().TextColor(a.Theme.Text)
			ui.Spacer(c).Height(8)

			ui.Scroll(c).Grow(1).Children(func() {
				ui.Text(c, current.FullText).
					FontSize(a.FontSize).
					TextColor(a.Theme.Text)
			})
		})

		// RIGHT COLUMN: Summary
		ui.Column(c).Grow(1).Children(func() {
			ui.Row(c).Children(func() {
				ui.Text(c, "Summary").FontSize(16).Bold().TextColor(a.Theme.Text)
				ui.Spacer(c)
				toggleLabel := "View Rendered"
				if a.SummaryIsMd {
					toggleLabel = "Edit Raw Code"
				}
				if ui.Button(c, toggleLabel).Clicked() {
					a.SummaryIsMd = !a.SummaryIsMd
				}
			})
			ui.Spacer(c).Height(8)

			if a.SummaryIsMd {
				// Rendered Markdown View
				ui.Scroll(c).Grow(1).Children(func() {
					markdown.View(c, current.Summary, &a.mdOpts)
				})
			} else {
				// Editable Raw Markdown Multi-line Input
				ui.TextArea(c, &current.Summary).
					Grow(1).
					FontSize(a.FontSize)
			}
		})
	})
}

func main() {
	app := NewApp()
	app.SwitchProject("Economist")

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Mebot",
			Width:   1400,
			Height:  900,
			Content: ui.View(app.View),
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
