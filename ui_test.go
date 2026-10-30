package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// articleListApp builds the list of the sidebar, with the menu its rows
// open, and the bare state those need, without files or settings.
func articleListApp() (*App, func(c *ui.Context)) {
	a := &App{
		Articles: []*Article{
			{Title: "First", Summary: "One"},
			{Title: "Second"},
		},
		ActiveIdx: -1,
		Theme:     MacOSLightTheme(),
	}
	a.ArticleListState.Selected = &a.ActiveIdx
	a.ArticleListState.Selection = &a.ChosenArticles
	a.ArticleListState.Label = func(i int) string { return a.Articles[i].Title }

	return a, func(c *ui.Context) {
		ui.Column(c).Fill().Padding(8).Children(func() {
			ui.List(c, &a.ArticleListState, len(a.Articles), func(i int) {
				row := ui.Row(c).Padding(6, 10).Radius(5)
				row.PopoverMenu(a.buildArticleMenu)
				row.Children(func() { ui.Text(c, a.Articles[i].Title) })
			}).Grow(1)
		})
	}
}

// TestArticleMenuActsOnTheRowRightClicked checks that a right-click opens
// the menu of the article under the pointer, choosing it as Finder does.
func TestArticleMenuActsOnTheRowRightClicked(t *testing.T) {
	a, view := articleListApp()
	tt := ui.NewTester(view, 400, 300)

	if err := tt.RightClick("Second"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Copy Titles") {
		t.Fatalf("no menu after a right-click: texts %q", tt.Texts())
	}
	for _, item := range []string{"Copy Titles", "Copy Full Text", "Copy Summaries", "Select All", "Clear Selection", "Delete Article…"} {
		if !tt.HasText(item) {
			t.Errorf("the menu has no item %q", item)
		}
	}
	if a.ActiveIdx != 1 || a.ChosenArticles.Len() != 1 || !a.ChosenArticles.Has(1) {
		t.Fatalf("after the right-click: active %d, chosen %d, chosen 1 %v", a.ActiveIdx, a.ChosenArticles.Len(), a.ChosenArticles.Has(1))
	}

	if err := tt.Click("Select All"); err != nil {
		t.Fatal(err)
	}
	if a.ChosenArticles.Len() != 2 {
		t.Fatalf("Select All chose %d articles, want 2", a.ChosenArticles.Len())
	}

	// A right-click on a chosen row keeps the choice, and its menu counts it.
	if err := tt.RightClick("First"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Delete 2 Articles…") {
		t.Errorf("the menu does not count the 2 chosen articles: %q", tt.Texts())
	}
	if a.ChosenArticles.Len() != 2 {
		t.Fatalf("a right-click on a chosen row chose %d articles, want 2", a.ChosenArticles.Len())
	}

	if err := tt.Click("Clear Selection"); err != nil {
		t.Fatal(err)
	}
	if a.ChosenArticles.Len() != 0 {
		t.Fatalf("Clear Selection left %d chosen", a.ChosenArticles.Len())
	}

	if err := tt.RightClick("First"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Delete Article…"); err != nil {
		t.Fatal(err)
	}
	if !a.ShowDeleteArticleConfirm {
		t.Error("Delete did not ask to confirm")
	}
}

// TestSummaryPreviewMenuIsDrawn checks that right-clicking the summary's
// preview opens a menu MyGo draws in the window, not the system's.
func TestSummaryPreviewMenuIsDrawn(t *testing.T) {
	a := &App{
		Articles:           []*Article{{Title: "One", Summary: "hello world"}},
		ActiveIdx:          0,
		ShowMarkdownRender: true,
		Theme:              MacOSLightTheme(),
		mdOpts:             markdown.Options{ShareSelection: true},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderSummaryColumn(c) }, 500, 400)

	r, ok := tt.Find("hello world")
	if !ok {
		t.Fatalf("the preview shows no text: %q", tt.Texts())
	}
	tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	if !tt.HasText("Copy Summary as Markdown") {
		t.Fatalf("the preview's menu is not the one drawn in the window: texts %q", tt.Texts())
	}
	if menu := tt.Menu(); menu != nil {
		t.Errorf("the system shows a menu too: %q", menu)
	}
}

// TestToolbarOverflows checks that the toolbar mygo builds puts the
// controls it has no room for in its menu, labeled by the controls' text.
func TestToolbarOverflows(t *testing.T) {
	a := &App{
		Projects:         []string{"One", "Two"},
		ActiveProjectIdx: 0,
		AvailableFonts:   []string{"Sans"},
		SelectedFont:     "Sans",
		FontPickerText:   "Sans",
		FontSizeOptions:  []string{"16"},
		SelectedFontSize: "16",
		Theme:            MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderToolbar(c) }, 320, 60)

	if !tt.HasText("More") {
		t.Fatalf("a narrow toolbar shows no overflow button: %q", tt.Texts())
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	if !slices.Contains(menu, "Settings") {
		t.Fatalf("the overflow menu does not hold Settings: %q", menu)
	}
}
