package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/qingxiang-jia/go-look/apple"
	"github.com/qingxiang-jia/go-look/apple/kit"
)

// This file is mebot's Apple HIG surface. The palette and metrics come from
// go-look/apple; the layout and controls come from go-look/apple/kit. The
// app's behavior stays in main.go: this file only decides where the existing
// projects, articles, menus, summaries and dialogs live.

var (
	iconFolder  = kit.MustPhosphor("folder")
	iconInbox   = kit.MustPhosphor("inbox")
	iconShare   = kit.MustPhosphor("share")
	iconMore    = kit.MustPhosphor("more")
	iconPlus    = kit.MustPhosphor("plus")
	iconCompose = kit.MustPhosphor("compose")

	iconChevron = kit.Icon(`<path d="M6.5 9.5 12 15l5.5-5.5"/>`)
	iconPower   = kit.Icon(`<path d="M12 3v9"/><path d="M6.4 6.6a8 8 0 1 0 11.2 0"/>`)
	iconSparkle = kit.Icon(`<path d="M12 2.8l2.2 7L21.2 12l-7 2.2L12 21.2l-2.2-7L2.8 12l7-2.2z"/>`)
	iconClose   = kit.Icon(`<path d="M6 6l12 12M18 6 6 18"/>`)
)

func appTheme(dark bool) *ui.Theme {
	if dark {
		return apple.Dark()
	}
	return apple.Light()
}

func appleInk(t *ui.Theme, name string) ui.Color {
	if c, ok := apple.Extra(name, t); ok {
		return c
	}
	return t.Background
}

func menuFace(t *ui.Theme) ui.Color { return appleInk(t, "Menu") }

// menuPanel is the opaque menu material macOS uses, rather than the window
// chrome the theme's background would put behind a popup.
func menuPanel(t *ui.Theme) func(ui.Element) {
	return func(panel ui.Element) {
		panel.Background(menuFace(t)).Radius(apple.RadiusPopover).
			NoShadow().Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.22))
	}
}

func hairline(c *ui.Context, t *ui.Theme) {
	ui.Row(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
}

func hairlineV(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Width(1).Shrink(0).FillHeight().Background(t.Border)
}

func boolWeight(b bool) int {
	if b {
		return 600
	}
	return 400
}

// View is the whole window: the client-side chrome, the project source list,
// the toolbar over the article list and reader, and the two reading panes.
func (a *App) View(c *ui.Context) {
	t := appTheme(a.IsDarkMode)
	c.SetTheme(t)
	c.SetMenuPresentation(ui.PopoverMenus)
	c.Root().Background(ui.Transparent)

	if a.ActiveIdx != a.LastActiveIdx && a.ActiveIdx >= 0 && a.ActiveIdx < len(a.Articles) {
		a.SelectArticle(a.ActiveIdx)
		c.Invalidate()
	}

	kit.WindowFrame(c, t, a.WindowMaximized, func() {
		ui.Row(c).Fill().Children(func() {
			a.renderProjectsSidebar(c)
			hairlineV(c, t)
			right := ui.Column(c).Fill()
			right.Children(func() {
				a.renderToolbar(c)
				hairline(c, t)
				ui.Split(c, &a.SidebarWidth, func() {
					a.renderArticleListColumn(c)
				}, func() {
					ui.SplitPercent(c, &a.ReaderPercent, func() {
						a.renderFullTextColumn(c)
					}, func() {
						a.renderSummaryColumn(c)
					}).Fill()
				}).Fill()
			})
		})
	})

	a.renderModals(c)
}

// ---------------------------------------------------------------------
// Project source list.

func (a *App) renderProjectsSidebar(c *ui.Context) {
	t := c.Theme()
	side := ui.Column(c).Width(226).Shrink(0).FillHeight().
		Background(appleInk(t, "Sidebar"))
	side.Children(func() {
		head := ui.Row(c).Height(apple.ToolbarCompact).Shrink(0).
			Padding(0, 0, 0, apple.LightMargin).Gap(apple.SpaceL).
			AlignItems(ui.Center).DragWindow()
		head.Children(func() {
			kit.TrafficLights(c, t, a.win, false)
		})
		if head.WindowDoubleClicked() && a.win != nil {
			a.win.ToggleMaximize()
		}

		body := ui.Column(c).FillWidth().Grow(1).
			Padding(apple.SpaceL, apple.SpaceL, apple.Space, apple.SpaceL)
		body.Children(func() {
			ui.Text(c, "Projects").FontSize(apple.FontSubheadline).FontWeight(600).
				TextColor(t.TextMuted).Padding(0, apple.Space, apple.Space, apple.Space)
			if len(a.Projects) == 0 {
				ui.Text(c, "No Projects").FontSize(apple.FontBody).TextColor(t.TextMuted).
					Padding(0, apple.Space)
				return
			}
			ui.Scroll(c).FillWidth().Grow(1).Children(func() {
				rows := ui.Column(c).FillWidth().Gap(1)
				rows.Children(func() {
					for i, name := range a.Projects {
						active := name == a.ActiveProject
						row := ui.ButtonBase(c.Key(fmt.Sprintf("project-%d", i))).Label(name).
							AlignItems(ui.Center).Gap(apple.Space).FillWidth().Height(28).Shrink(0).
							Padding(0, apple.Space).Radius(t.Radius)
						switch {
						case active:
							row.Background(appleInk(t, "Unemphasized"))
						case row.Hovered():
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							ui.Icon(c, iconFolder).FontSize(15).TextColor(t.TextMuted)
							ui.Text(c, name).FontSize(apple.FontBody).
								FontWeight(boolWeight(active)).SingleLine().TextColor(t.Text)
						})
						if row.Clicked() {
							a.SwitchProject(name)
						}
					}
				})
			})
		})

		foot := ui.Row(c).FillWidth().Height(38).Shrink(0).
			Padding(0, apple.SpaceL, 0, apple.Space).Gap(apple.Space).AlignItems(ui.Center)
		foot.Children(func() {
			if kit.IconButton(c, "new-project", "New Project", iconPlus) {
				a.NewProjectName = ""
				a.ShowNewProjectModal = true
			}
			ui.Row(c).Grow(1)
			a.projectMenuButton(c)
		})
	})
}

func (a *App) projectMenuButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("project-menu")).Label("Project Actions").Tooltip("Project Actions").
		Width(apple.ControlHeight).Height(apple.ControlHeight).Padding(0).Shrink(0).
		Justify(ui.Center).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, iconMore).FontSize(kit.IconSize).TextColor(t.TextMuted)
	})
	b.PopoverMenuButton(a.buildProjectMenu, ui.PopoverMenuButtonOptions{
		At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Panel: menuPanel(t),
	})
}

func (a *App) buildProjectMenu(m *ui.Menu) {
	if m.Item("New Project…").Chosen() {
		a.NewProjectName = ""
		a.ShowNewProjectModal = true
	}
	m.Separator()
	if m.Item("Rename Project…").Disabled(a.ActiveProject == "").Chosen() {
		a.RenameProjectName = a.ActiveProject
		a.ShowRenameProjectModal = true
	}
	if m.Item("Delete Project…").Disabled(a.ActiveProject == "").Danger().Chosen() {
		a.ShowDeleteProjectConfirm = true
	}
}

// ---------------------------------------------------------------------
// Unified toolbar.

func (a *App) renderToolbar(c *ui.Context) {
	t := c.Theme()
	bar := ui.Row(c).Height(apple.ToolbarCompact).Shrink(0).
		Padding(0, apple.SpaceL).Gap(apple.Space).AlignItems(ui.Center).
		DragWindow().Background(appleInk(t, "Chrome"))
	bar.Children(func() {
		ui.Icon(c, iconFolder).FontSize(15).TextColor(t.TextMuted)
		title := a.ActiveProject
		if title == "" {
			title = "Mebot"
		}
		ui.Text(c, title).FontSize(apple.FontBody).FontWeight(600).
			SingleLine().MaxWidth(260).TextColor(t.Text)
		ui.Row(c).Grow(1)

		if a.IsDirty {
			ui.Text(c, "Saving…").FontSize(apple.FontSubheadline).TextColor(t.Accent)
		} else if a.LastSavedAt != "" {
			ui.Text(c, "Saved "+a.LastSavedAt).FontSize(apple.FontSubheadline).
				TextColor(t.TextMuted)
		}

		a.renderLocalLLMButton(c)
		a.importMenuButton(c)
		if toolbarButton(c, "export", "Export", iconShare) {
			a.ShowExportModal = true
		}
		a.moreMenuButton(c)
	})
	if bar.WindowDoubleClicked() && a.win != nil {
		a.win.ToggleMaximize()
	}
}

func toolbarButton(c *ui.Context, key, label string, icon *ui.SVG) bool {
	b := ui.Button(c.Key(key), "").Height(apple.ControlHeight).
		Padding(0, 10).Shrink(0).Gap(6)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(14)
		}
		ui.Text(c, label).FontSize(apple.FontBody).SingleLine()
	})
	return b.Clicked()
}

func (a *App) renderLocalLLMButton(c *ui.Context) {
	t := c.Theme()
	color := t.Danger
	tip := "Turn On Local LLM"
	label := "Local LLM"
	busy := a.LocalLLMPhase == "checking" || a.LocalLLMPhase == "starting" || a.LocalLLMPhase == "stopping"

	switch a.LocalLLMPhase {
	case "ready":
		color = t.Success
		tip = "Turn Off Local LLM"
	case "checking":
		color = t.Warning
		tip = "Checking Local LLM Status"
		label = "Checking…"
	case "starting":
		color = t.Warning
		tip = "Local LLM Is Starting"
		label = "Starting…"
	case "stopping":
		color = t.Warning
		tip = "Local LLM Is Stopping"
		label = "Stopping…"
	case "error":
		color = t.Danger
		tip = "Local LLM Error; Click to Try Again"
		label = "LLM Error"
	}

	button := ui.Button(c.Key("llm"), "").Tooltip(tip).Disabled(busy).
		Height(apple.ControlHeight).Padding(0, 10).Shrink(0).Gap(6)
	button.Children(func() {
		ui.Icon(c, iconPower).FontSize(14).TextColor(color)
		ui.Text(c, label).FontSize(apple.FontBody).SingleLine()
	})
	if !button.Clicked() {
		return
	}
	switch a.LocalLLMPhase {
	case "ready":
		a.StopLocalLLM()
	case "checking", "starting", "stopping":
		// The running action settles the state.
	default:
		a.StartLocalLLM()
	}
}

func (a *App) importMenuButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("import")).Label("Import").Height(apple.ControlHeight).
		Padding(0, 10).Shrink(0).Gap(6).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, iconInbox).FontSize(14).TextColor(t.Text)
		ui.Text(c, "Import").FontSize(apple.FontBody).SingleLine()
		ui.Icon(c, iconChevron).FontSize(9).TextColor(t.TextMuted)
	})
	b.PopoverMenuButton(a.buildImportMenu, ui.PopoverMenuButtonOptions{
		At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Panel: menuPanel(t),
	})
}

func (a *App) buildImportMenu(m *ui.Menu) {
	if m.Item("Import HTML…").Chosen() {
		a.ImportHTMLFiles()
	}
	if m.Item("Import Markdown Rollup…").Chosen() {
		a.ImportMarkdownFile()
	}
	m.Separator()
	if m.Item("Match Pasted Summaries…").Chosen() {
		a.OpenBatchSummaryModal()
	}
}

func (a *App) moreMenuButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("more")).Label("More").Tooltip("More").
		Width(apple.ControlHeight).Height(apple.ControlHeight).Padding(0).Shrink(0).
		Justify(ui.Center).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, iconMore).FontSize(kit.IconSize).TextColor(t.Text)
	})
	b.PopoverMenuButton(a.buildAppMenu, ui.PopoverMenuButtonOptions{
		At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Panel: menuPanel(t),
	})
}

func (a *App) buildAppMenu(m *ui.Menu) {
	if m.Item("Dark Mode").Checked(a.IsDarkMode).Chosen() {
		a.ToggleTheme()
	}
	m.Separator()
	if m.Item("Settings…").Chosen() {
		a.OpenSettingsModal()
	}
}

// ---------------------------------------------------------------------
// Article list.

func (a *App) renderArticleListColumn(c *ui.Context) {
	t := c.Theme()
	list := ui.Column(c).Fill().Background(appleInk(t, "Content")).Clip()
	list.Children(func() {
		head := ui.Row(c).FillWidth().Height(34).Shrink(0).
			Padding(0, apple.SpaceL).Gap(apple.Space).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "Articles").FontSize(apple.FontSubheadline).FontWeight(600).
				TextColor(t.TextMuted)
			ui.Row(c).Grow(1)
			a.articleMenuButton(c)
		})
		hairline(c, t)

		if len(a.Articles) == 0 {
			empty := ui.Column(c).Fill().Grow(1).Padding(apple.Space2X).
				Justify(ui.Center).AlignItems(ui.Center).Gap(apple.Space)
			empty.Children(func() {
				ui.Icon(c, iconInbox).FontSize(28).TextColor(t.TextMuted)
				ui.Text(c, "No Articles").FontSize(apple.FontHeadline).Bold()
				ui.Text(c, "Import an article to begin.").FontSize(apple.FontSubheadline).
					TextColor(t.TextMuted)
			})
			return
		}

		ui.List(c, &a.ArticleListState, len(a.Articles)).Rows(func(r ui.ListRow) {
			i := r.Index
			if i < 0 || i >= len(a.Articles) {
				return
			}
			active := i == a.ActiveIdx
			chosen := r.Selected()
			row := ui.Row(r.Context).FillWidth().MinHeight(44).
				Padding(apple.Space, apple.SpaceL).AlignItems(ui.Center)
			row.PopoverMenu(a.buildArticleMenu)
			if active && !chosen {
				// The list paints the multi-selection, but an article
				// chosen at startup has not been through a click yet.
				row.Background(t.Accent).Radius(t.Radius)
			}
			row.Children(func() {
				ink := t.Text
				if active || chosen {
					ink = t.AccentText
				}
				ui.Text(r.Context, a.Articles[i].Title).FontSize(apple.FontBody).
					MaxLines(2).LineHeight(1.25).TextColor(ink)
			})
		}).Grow(1)
	})
}

func (a *App) articleMenuButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("article-menu")).Label("Article Actions").Tooltip("Article Actions").
		Width(apple.ControlHeight).Height(apple.ControlHeight).Padding(0).Shrink(0).
		Justify(ui.Center).AlignItems(ui.Center)
	b.Children(func() {
		ui.Icon(c, iconMore).FontSize(kit.IconSize).TextColor(t.TextMuted)
	})
	b.PopoverMenuButton(a.buildArticleMenu, ui.PopoverMenuButtonOptions{
		At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd, Panel: menuPanel(t),
	})
}

// ---------------------------------------------------------------------
// Full text.

func (a *App) renderFullTextColumn(c *ui.Context) {
	t := c.Theme()
	current := a.CurrentArticle()

	col := ui.Column(c).Fill().Background(appleInk(t, "Content")).Clip()
	col.Children(func() {
		head := ui.Row(c).FillWidth().Height(44).Shrink(0).
			Padding(0, apple.SpaceXL).Gap(apple.Space).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "Full Text").FontSize(apple.FontHeadline).Bold().TextColor(t.Text)
			ui.Row(c).Grow(1)
			a.typographyButton(c)
		})
		hairline(c, t)

		if current == nil {
			empty := ui.Column(c).Fill().Grow(1).Justify(ui.Center).
				AlignItems(ui.Center).Gap(apple.Space)
			empty.Children(func() {
				ui.Icon(c, iconCompose).FontSize(30).TextColor(t.TextMuted)
				ui.Text(c, "No Article Selected").FontSize(apple.FontTitle2).FontWeight(600)
				ui.Text(c, "Choose an article from the list.").FontSize(apple.FontBody).
					TextColor(t.TextMuted)
			})
			return
		}

		ui.Scroll(c).Fill().Grow(1).Children(func() {
			content := ui.Column(c).FillWidth().
				Padding(apple.Space2X, 30).Gap(apple.SpaceL)
			content.Children(func() {
				ui.Text(c, current.Title).
					Font(a.CurrentFontFamily()).
					FontSize(a.CurrentFontSize() + 4).
					Bold().Balance().LineHeight(1.22).
					TextColor(t.Text)
				ui.Row(c).Height(apple.SpaceXS)
				// The title is a heading, not part of the passage a
				// selection explains; only the body is selectable.
				box := ui.Column(c).Selectable().FillWidth()
				box.Children(func() {
					ui.Text(c, current.FullText).
						Font(a.CurrentFontFamily()).
						FontSize(a.CurrentFontSize()).
						LineHeight(1.55).
						TextColor(t.Text)
				})
				a.observeSelection(box, "fulltext")
			})
		})

		a.renderSelectionExplanation(c, "fulltext")
	})
}

func (a *App) typographyButton(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("typography")).Label("Typography").Tooltip("Font and Size").
		Height(24).Padding(0, 7).Shrink(0).Radius(t.Radius).
		Justify(ui.Center).AlignItems(ui.Center)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Text(c, "Aa").FontSize(14).FontWeight(600).TextColor(t.Text)
	})
	if b.Clicked() {
		a.TypographyOpen = !a.TypographyOpen
	}
	ui.PopoverBase(c, b, &a.TypographyOpen, func(panel ui.Element) {
		panel.Width(260).Padding(apple.SpaceL).Gap(apple.SpaceL).
			Background(menuFace(t)).Radius(apple.RadiusPopover).
			NoShadow().Shadow(0, 10, 28, 0, ui.RGBA(0, 0, 0, 0.22))
		panel.Children(func() {
			ui.Text(c, "Reading Type").FontSize(apple.FontHeadline).Bold()
			row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(apple.SpaceL)
			row.Children(func() {
				ui.Text(c, "Font").FontSize(apple.FontBody).Grow(1)
				a.fontCombobox(c)
			})
			row = ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(apple.SpaceL)
			row.Children(func() {
				ui.Text(c, "Size").FontSize(apple.FontBody).Grow(1)
				prevSize := a.SelectedFontSize
				ui.Select(c.Key("fontSizeSelector"), &a.SelectedFontSize, a.FontSizeOptions).Width(80)
				if a.SelectedFontSize != prevSize {
					a.Settings.FontSize = a.SelectedFontSize
					_ = a.Settings.Save()
				}
			})
		})
	})
}

// ---------------------------------------------------------------------
// Summary.

func (a *App) renderSummaryColumn(c *ui.Context) {
	t := c.Theme()
	current := a.CurrentArticle()
	running := a.AISummaryRunning && current != nil && a.AISummaryArticle == a.ActiveIdx

	col := ui.Column(c).Fill().Background(appleInk(t, "Content")).Clip()
	col.Children(func() {
		wasPreview := a.ShowMarkdownRender
		head := ui.Row(c).FillWidth().Height(44).Shrink(0).
			Padding(0, apple.SpaceXL).Gap(apple.Space).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, "Summary").FontSize(apple.FontHeadline).Bold().TextColor(t.Text)
			if running {
				ui.Spinner(c)
			}
			ui.Row(c).Grow(1)

			ui.Text(c, "Preview").FontSize(apple.FontSubheadline).TextColor(t.TextMuted)
			ui.Switch(c.Key("summaryPreviewSwitch"), &a.ShowMarkdownRender).
				Label("Preview")

			if running {
				if kit.Button(c, "cancel-ai", "Cancel") {
					a.cancelArticleAISummary()
				}
			} else {
				aiTip := "Generate a summary for this article with Local AI"
				if a.LocalLLMPhase != "ready" {
					aiTip = "Turn on Local LLM to generate summaries"
				} else if current == nil {
					aiTip = "Select an article to summarize"
				}
				ai := ui.Button(c.Key("ai-summary"), "").Tooltip(aiTip).
					Disabled(a.LocalLLMPhase != "ready" || current == nil).
					Height(apple.ControlHeight).Padding(0, 10).Shrink(0).Gap(6).
					Background(appleInk(t, "BlueTint")).Radius(t.Radius)
				ai.Children(func() {
					ui.Icon(c, iconSparkle).FontSize(13).TextColor(t.Accent)
					ui.Text(c, "AI Summary").FontSize(apple.FontBody).SingleLine().
						TextColor(t.Accent)
				})
				if ai.Clicked() {
					a.StartAISummary()
				}
			}
		})
		hairline(c, t)

		if a.AISummaryError != "" {
			ui.Text(c, a.AISummaryError).FontSize(apple.FontSubheadline).TextColor(t.Danger).
				Padding(apple.Space, apple.SpaceXL)
		}

		if a.ShowMarkdownRender && !wasPreview && current != nil && a.SummaryDoc != nil {
			current.Summary = a.currentSummaryMarkdown()
		}

		if current == nil {
			empty := ui.Column(c).Fill().Grow(1).Justify(ui.Center).
				AlignItems(ui.Center).Gap(apple.Space)
			empty.Children(func() {
				ui.Icon(c, iconSparkle).FontSize(30).TextColor(t.TextMuted)
				ui.Text(c, "No Article Selected").FontSize(apple.FontTitle2).FontWeight(600)
				ui.Text(c, "Choose an article to read its summary.").FontSize(apple.FontBody).
					TextColor(t.TextMuted)
			})
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
		a.mdOpts.Theme.LinkWeight = 500

		if running {
			ui.Scroll(c).Fill().Grow(1).Children(func() {
				body := ui.Column(c).FillWidth().Padding(apple.Space2X, 26)
				body.Children(func() {
					if strings.TrimSpace(a.AISummaryOutput) == "" {
						ui.Text(c, "Waiting for the local LLM...").
							FontSize(a.CurrentFontSize()).
							TextColor(t.TextMuted)
						return
					}
					markdown.View(c, a.AISummaryOutput, &a.mdOpts)
				})
			})
		} else if a.ShowMarkdownRender {
			ui.Scroll(c).Fill().Grow(1).Children(func() {
				box := ui.Column(c).Selectable().PopoverMenu(func(m *ui.Menu) {
					m.EditItems()
					m.Separator()
					if m.Item("Copy Summary as Markdown").Chosen() {
						writeClipboardText(current.Summary)
					}
				})
				box.Children(func() {
					body := ui.Column(c).FillWidth().Padding(apple.Space2X, 26)
					body.Children(func() {
						markdown.View(c, current.Summary, &a.mdOpts)
					})
				})
				a.observeSelection(box, "summary")
			})
		} else {
			if a.SummaryDoc != nil {
				editor := ui.Column(c).Fill().Grow(1).Padding(apple.Space2X, 26)
				editor.Children(func() {
					view := richtext.EditorBase(c.Key(fmt.Sprintf("summaryMarkdownEditor_%d", a.ActiveIdx)),
						a.SummaryDoc, richtext.EditorOptions{
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
				})
			}
		}

		a.renderSelectionExplanation(c, "summary")
	})
}

// ---------------------------------------------------------------------
// Dialogs. These are the app's existing modals, redrawn as macOS panels.

func (a *App) renderModals(c *ui.Context) {
	if a.ShowDeleteProjectConfirm {
		a.confirmSheet(c, &a.ShowDeleteProjectConfirm, "Delete Project",
			fmt.Sprintf("Are you sure you want to delete '%s' and all its articles?", a.ActiveProject),
			"Delete", a.DeleteActiveProject)
	}

	if a.ShowDeleteArticleConfirm {
		count := a.ChosenArticles.Len()
		msg := "Are you sure you want to delete the selected article?"
		if count > 1 {
			msg = fmt.Sprintf("Are you sure you want to delete all %d selected articles?", count)
		} else if cur := a.CurrentArticle(); cur != nil {
			msg = fmt.Sprintf("Are you sure you want to delete '%s'?", cur.Title)
		}
		a.confirmSheet(c, &a.ShowDeleteArticleConfirm, "Delete Article", msg,
			"Delete", a.ConfirmDeleteChosenArticles)
	}

	a.newProjectSheet(c)
	a.renameProjectSheet(c)
	a.exportSheet(c)
	a.batchSummarySheet(c)
	a.settingsSheet(c)
}

func (a *App) sheet(c *ui.Context, open *bool, width float32, build func(t *ui.Theme)) {
	t := c.Theme()
	ui.Themed(c, apple.Alert(t)).Children(func() {
		ui.Modal(c, open, func() {
			ui.Column(c).Width(width).Gap(apple.SpaceL).Children(func() {
				build(t)
			})
		})
	})
}

func (a *App) sheetTitle(c *ui.Context, t *ui.Theme, title, subtitle string) {
	ui.Text(c, title).FontSize(apple.FontTitle2).FontWeight(600).TextColor(t.Text)
	if subtitle != "" {
		ui.Text(c, subtitle).FontSize(apple.FontBody).LineHeight(1.4).
			TextColor(t.TextMuted).Width(0).MinWidthPercent(100)
	}
}

func (a *App) sheetCard(c *ui.Context, t *ui.Theme, build func()) {
	ui.Column(c).FillWidth().Padding(0, apple.SpaceL).
		Radius(apple.RadiusCard).Background(appleInk(t, "Content")).
		Border(1, t.Border).Children(build)
}

func (a *App) sheetRow(c *ui.Context, t *ui.Theme, label, note string, control func()) {
	row := ui.Row(c).FillWidth().MinHeight(42).Padding(apple.Space, 0).
		Gap(apple.SpaceL).AlignItems(ui.Center)
	row.Children(func() {
		col := ui.Column(c).Gap(1).Grow(1)
		col.Children(func() {
			ui.Text(c, label).FontSize(apple.FontBody).TextColor(t.Text)
			if note != "" {
				ui.Text(c, note).FontSize(apple.FontCaption).TextColor(t.TextMuted)
			}
		})
		control()
	})
}

func (a *App) sheetFooter(c *ui.Context, t *ui.Theme, cancelLabel string, cancel func(), primaryLabel string, primary func(), danger bool) {
	row := ui.Row(c).FillWidth().Justify(ui.End).Gap(apple.Space)
	row.Children(func() {
		if kit.Button(c, "sheet-cancel", cancelLabel) {
			cancel()
		}
		if danger {
			b := ui.Button(c.Key("sheet-primary"), primaryLabel).
				Height(apple.ControlHeight).Padding(0, 14).Shrink(0).AutoFocus()
			b.TextColor(t.Danger)
			if b.Clicked() {
				primary()
			}
		} else if kit.PrimaryButton(c, "sheet-primary", primaryLabel) {
			primary()
		}
	})
}

func (a *App) confirmSheet(c *ui.Context, open *bool, title, message, action string, run func()) {
	a.sheet(c, open, 360, func(t *ui.Theme) {
		a.sheetTitle(c, t, title, message)
		a.sheetFooter(c, t, "Cancel", func() { *open = false }, action, run, true)
	})
}

func (a *App) newProjectSheet(c *ui.Context) {
	if !a.ShowNewProjectModal {
		return
	}
	a.sheet(c, &a.ShowNewProjectModal, 380, func(t *ui.Theme) {
		a.sheetTitle(c, t, "New Project", "")
		a.sheetCard(c, t, func() {
			ui.Column(c).Padding(apple.SpaceL).Gap(apple.Space).Children(func() {
				ui.Text(c, "Project Name").FontSize(apple.FontSubheadline).TextColor(t.TextMuted)
				if ui.TextInput(c.Key("newProjInput"), &a.NewProjectName).
					AutoFocus().Placeholder("Project name...").Submitted() {
					a.CreateProject(a.NewProjectName)
				}
			})
		})
		a.sheetFooter(c, t, "Cancel", func() { a.ShowNewProjectModal = false },
			"Create", func() { a.CreateProject(a.NewProjectName) }, false)
	})
}

func (a *App) renameProjectSheet(c *ui.Context) {
	if !a.ShowRenameProjectModal {
		return
	}
	a.sheet(c, &a.ShowRenameProjectModal, 380, func(t *ui.Theme) {
		a.sheetTitle(c, t, "Rename Project", "")
		a.sheetCard(c, t, func() {
			ui.Column(c).Padding(apple.SpaceL).Gap(apple.Space).Children(func() {
				ui.Text(c, "Project Name").FontSize(apple.FontSubheadline).TextColor(t.TextMuted)
				if ui.TextInput(c.Key("renameProjInput"), &a.RenameProjectName).
					AutoFocus().Placeholder("New project name...").Submitted() {
					a.RenameActiveProject(a.RenameProjectName)
				}
			})
		})
		a.sheetFooter(c, t, "Cancel", func() { a.ShowRenameProjectModal = false },
			"Rename", func() { a.RenameActiveProject(a.RenameProjectName) }, false)
	})
}

func (a *App) exportSheet(c *ui.Context) {
	if !a.ShowExportModal {
		return
	}
	a.sheet(c, &a.ShowExportModal, 500, func(t *ui.Theme) {
		a.sheetTitle(c, t, "Export Weekly Rollup",
			"Choose a date and a destination. Every article is written to the file, with its summary where one exists.")
		a.sheetCard(c, t, func() {
			ui.Column(c).Padding(apple.SpaceL).Gap(apple.SpaceL).Children(func() {
				ui.Text(c, "Select Export Date").FontSize(apple.FontSubheadline).
					FontWeight(600).TextColor(t.TextMuted)
				ui.Calendar(c, &a.SelectedDate).Browse()
			})
		})
		a.sheetCard(c, t, func() {
			a.sheetRow(c, t, "Destination Folder", "", func() {
				ui.TextInput(c.Key("exportDirInput"), &a.ExportDirectory).Width(280)
				if kit.Button(c, "browse-export", "Browse…") {
					a.BrowseExportDirectory()
				}
			})
			hairline(c, t)
			name := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
			a.sheetRow(c, t, "File", "", func() {
				ui.Text(c, name).FontSize(apple.FontBody).TextColor(t.Accent)
			})
		})
		a.sheetFooter(c, t, "Cancel", func() { a.ShowExportModal = false }, "Export", func() {
			name := fmt.Sprintf("%s %s.md", a.SelectedDate.Format("2006-01-02"), a.ActiveProject)
			a.PerformExportTo(filepath.Join(a.ExportDirectory, name))
		}, false)
	})
}

func (a *App) batchSummarySheet(c *ui.Context) {
	if !a.ShowBatchSummaryModal {
		return
	}
	if a.BatchMatchingStep == 0 {
		a.sheet(c, &a.ShowBatchSummaryModal, 620, func(t *ui.Theme) {
			a.sheetTitle(c, t, "Match Markdown Summaries",
				"Paste your markdown summaries below. The app matches them to articles by title.")
			a.sheetCard(c, t, func() {
				emptyCount := 0
				for _, art := range a.Articles {
					if IsSummaryEmpty(art.Summary) {
						emptyCount++
					}
				}
				ui.Column(c).Padding(apple.SpaceL).Gap(apple.SpaceL).Children(func() {
					ui.Text(c, fmt.Sprintf("Project has %d articles (%d without summary)", len(a.Articles), emptyCount)).
						FontSize(apple.FontSubheadline).TextColor(t.TextMuted)
					ui.TextArea(c.Key("batchAreaInput"), &a.BatchPastedMarkdown).
						Placeholder("## Article Title\nSummary content here...\n\n## Another Article Title\nSummary content here...").
						Height(240).Grow(1)
					ui.Checkbox(c.Key("batchIncludeExistingCheck"), &a.BatchIncludeExisting,
						"Include articles that already have summaries")
				})
			})
			if a.BatchMatchError != "" {
				ui.Text(c, a.BatchMatchError).FontSize(apple.FontSubheadline).Bold().
					TextColor(t.Danger)
			}
			a.sheetFooter(c, t, "Cancel", func() { a.ShowBatchSummaryModal = false },
				"Analyze & Match", a.AnalyzeBatchMarkdown, false)
		})
		return
	}

	a.sheet(c, &a.ShowBatchSummaryModal, 620, func(t *ui.Theme) {
		a.sheetTitle(c, t, "Match Markdown Summaries",
			"Verify or adjust the matched articles. Any option set to '[Do Not Match]' will be skipped.")
		ui.Scroll(c).Height(330).Children(func() {
			ui.Column(c).FillWidth().Gap(apple.SpaceL).Children(func() {
				for i := range a.ParsedSummaryMatches {
					item := &a.ParsedSummaryMatches[i]
					a.sheetCard(c, t, func() {
						ui.Column(c).Padding(apple.SpaceL).Gap(apple.Space).Children(func() {
							ui.Text(c, "Summary Title: "+item.RawTitle).
								FontSize(apple.FontBody).FontWeight(600).TextColor(t.Text)
							excerpt := item.Body
							if len([]rune(excerpt)) > 120 {
								excerpt = string([]rune(excerpt)[:120]) + "..."
							}
							ui.Text(c, excerpt).FontSize(apple.FontSubheadline).
								TextColor(t.TextMuted).MaxLines(2)
							row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(apple.SpaceL)
							row.Children(func() {
								ui.Text(c, "Assign to:").FontSize(apple.FontSubheadline).
									TextColor(t.TextMuted)
								ui.Select(c.Key(fmt.Sprintf("assignSel_%d", i)),
									&item.SelectedOption, a.UnmatchedOptions).Grow(1)
							})
						})
					})
				}
			})
		})
		footer := ui.Row(c).FillWidth().Gap(apple.Space).AlignItems(ui.Center)
		footer.Children(func() {
			if kit.Button(c, "back-to-edit", "Back to Edit") {
				a.BatchMatchingStep = 0
			}
			ui.Row(c).Grow(1)
			if kit.Button(c, "cancel-match", "Cancel") {
				a.ShowBatchSummaryModal = false
			}
			if kit.PrimaryButton(c, "apply-matches", "Apply All Matches") {
				a.ApplyBatchMatches()
			}
		})
	})
}

func (a *App) settingsSheet(c *ui.Context) {
	if !a.ShowSettingsModal {
		return
	}
	a.sheet(c, &a.ShowSettingsModal, 540, func(t *ui.Theme) {
		a.sheetTitle(c, t, "Settings", "")
		a.sheetCard(c, t, func() {
			a.sheetRow(c, t, "Local AI",
				fmt.Sprintf("%s · %s", DefaultLocalLLMModel, LocalLLMBaseURL), func() {})
			hairline(c, t)
			ui.Column(c).Padding(apple.SpaceL).Gap(apple.Space).Children(func() {
				row := ui.Row(c).FillWidth().AlignItems(ui.Center)
				row.Children(func() {
					ui.Text(c, "Summary prompt").FontSize(apple.FontBody).TextColor(t.Text)
					ui.Row(c).Grow(1)
					if kit.Button(c, "reset-prompt", "Reset to Default") {
						a.SettingsDraftPrompt = DefaultSummaryPrompt
					}
				})
				ui.TextArea(c.Key("summaryPromptInput"), &a.SettingsDraftPrompt).
					Placeholder(DefaultSummaryPrompt).Height(140).FontSize(apple.FontBody)
			})
		})
		a.sheetFooter(c, t, "Cancel", func() { a.ShowSettingsModal = false }, "Save Settings", func() {
			a.SaveSettingsModal()
			c.Toast("Settings saved")
		}, false)
	})
}

// ---------------------------------------------------------------------
// Window lifecycle helpers.

func (a *App) prepareWindow(win *mygo.Window) {
	win.OnMaximize(func() {
		a.WindowMaximized = true
		win.Invalidate()
	})
	win.OnUnmaximize(func() {
		a.WindowMaximized = false
		win.Invalidate()
	})
}
