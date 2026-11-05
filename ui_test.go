package main

import (
	"slices"
	"strings"
	"testing"
	"time"

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

// TestAISummaryModalKeepsItsWidthWhileStreaming checks that the running
// dialog stays at the width it opens with and wraps the streamed text,
// instead of growing as the text gets longer.
func TestAISummaryModalKeepsItsWidthWhileStreaming(t *testing.T) {
	output := strings.Repeat("A summary line with plenty of words that have to wrap at the dialog's width. ", 15)
	a := &App{
		Articles:           []*Article{{Title: "First"}},
		ShowAISummaryModal: true,
		AISummaryStep:      1,
		AISummaryTargets:   []AISummaryTarget{{Index: 0, Article: Article{Title: "First"}}},
		AISummaryCurrent:   0,
		AISummaryStatus:    "Summarizing 1 of 1...",
		AISummaryOutput:    output,
		AISummaryStartedAt: time.Now(),
		LocalLLMPhase:      "ready",
		Settings:           &AppSettings{},
		Theme:              MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderModals(c) }, 900, 700)

	if !tt.HasText("First") || !tt.HasText("PROGRESS") || !tt.HasText("Model: "+DefaultLocalLLMModel) {
		t.Fatalf("the running dialog does not show the request details: %q", tt.Texts())
	}
	rect, ok := tt.Find(output)
	if !ok {
		t.Fatalf("the streamed text is not drawn: %q", tt.Texts())
	}
	if rect.W > 500 {
		t.Errorf("the streamed text is %v wide, want at most 500: the dialog grew with it", rect.W)
	}
}

// TestAISummaryModalOffersToStartLocalService checks that the confirm step
// does not let the user generate summaries until the local service is on.
func TestAISummaryModalOffersToStartLocalService(t *testing.T) {
	a := &App{
		Articles:           []*Article{{Title: "First"}},
		ShowAISummaryModal: true,
		AISummaryStep:      0,
		AISummaryTargets:   []AISummaryTarget{{Index: 0, Article: Article{Title: "First"}}},
		LocalLLMPhase:      "off",
		Settings:           &AppSettings{SummaryPrompt: DefaultSummaryPrompt},
		Theme:              MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderModals(c) }, 900, 700)

	if !tt.HasText("Turn On Service") {
		t.Fatalf("the confirm step has no button to start the service: %q", tt.Texts())
	}
	if tt.HasText("Generate Summaries") {
		t.Errorf("the generate button is shown while the service is off: %q", tt.Texts())
	}
}

// TestAISummaryModalStopsServiceWhenNoSummariesRemain checks that the
// service control is still available after every article has a summary, so
// the model can be turned off instead of being left running.
func TestAISummaryModalStopsServiceWhenNoSummariesRemain(t *testing.T) {
	a := &App{
		Articles:           []*Article{{Title: "First", Summary: "Already summarized"}},
		ShowAISummaryModal: true,
		AISummaryStep:      0,
		LocalLLMPhase:      "ready",
		Settings:           &AppSettings{SummaryPrompt: DefaultSummaryPrompt},
		Theme:              MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderModals(c) }, 900, 700)

	if !tt.HasText("All articles in this project already have a summary.") {
		t.Fatalf("the no-targets message is not shown: %q", tt.Texts())
	}
	if !tt.HasText("Turn Off Service") {
		t.Fatalf("the service cannot be stopped when no summaries remain: %q", tt.Texts())
	}
	if tt.HasText("Turn On Service") {
		t.Errorf("the off button is shown while the service is ready: %q", tt.Texts())
	}
}

// TestAISummaryModalShowsProgressAndStreamedSummary checks that the running
// step reports how many articles are done and shows the live summary.
func TestAISummaryModalShowsProgressAndStreamedSummary(t *testing.T) {
	a := &App{
		Articles:           []*Article{{Title: "First"}, {Title: "Second"}},
		ShowAISummaryModal: true,
		AISummaryStep:      1,
		AISummaryTargets: []AISummaryTarget{
			{Index: 0, Article: Article{Title: "First"}},
			{Index: 1, Article: Article{Title: "Second"}},
		},
		AISummaryResults:   []AISummaryResult{{Index: 0, Title: "First", Summary: "First summary."}},
		AISummaryCurrent:   1,
		AISummaryStatus:    "Summarizing 2 of 2...",
		AISummaryOutput:    "Second summary is streaming",
		AISummaryStartedAt: time.Now(),
		LocalLLMPhase:      "ready",
		Settings:           &AppSettings{SummaryPrompt: DefaultSummaryPrompt},
		Theme:              MacOSLightTheme(),
	}
	tt := ui.NewTester(func(c *ui.Context) { a.renderModals(c) }, 900, 700)

	for _, want := range []string{"1 of 2 articles summarized", "First summary.", "Second summary is streaming"} {
		if !tt.HasText(want) {
			t.Errorf("the running dialog does not show %q: %q", want, tt.Texts())
		}
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
