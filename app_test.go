package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCreateProjectRejectsAPathSeparator(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	a := &App{Settings: &AppSettings{ProjectStates: map[string]ProjectSelectionState{}}}
	a.CreateProject("bad/name")

	if len(a.Projects) != 0 {
		t.Errorf("the invalid name was added: %q", a.Projects)
	}
	if a.ActiveProject != "" {
		t.Errorf("the invalid name became active: %q", a.ActiveProject)
	}
	if a.ProjectNameError == "" {
		t.Error("the invalid name was rejected without a message")
	}
}

func TestSwitchToEmptyProjectClearsTheSummary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := SaveProjectState("Empty", nil); err != nil {
		t.Fatal(err)
	}

	a := &App{
		Settings:  &AppSettings{ProjectStates: map[string]ProjectSelectionState{}},
		Projects:  []string{"Empty"},
		Articles:  []*Article{{Title: "Old", Summary: "Old summary."}},
		ActiveIdx: 0,
	}
	a.initSummaryDoc("Old summary.")

	a.SwitchProject("Empty")

	if len(a.Articles) != 0 {
		t.Fatalf("the empty project kept %d articles", len(a.Articles))
	}
	if a.SummaryDoc == nil {
		t.Fatal("the summary document is nil")
	}
	if got := a.currentSummaryMarkdown(); got != "" {
		t.Errorf("the previous project's summary is still loaded: %q", got)
	}
	if a.ActiveIdx != 0 {
		t.Errorf("ActiveIdx = %d, want 0", a.ActiveIdx)
	}
}

func TestDeleteKeepsTheActiveArticleWhenItSurvives(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	a := &App{
		Settings:      &AppSettings{ProjectStates: map[string]ProjectSelectionState{}},
		ActiveProject: "Reading",
		Projects:      []string{"Reading"},
		Articles:      []*Article{{Title: "A"}, {Title: "B"}, {Title: "C"}},
		ActiveIdx:     2,
		LastActiveIdx: 2,
	}
	a.initSummaryDoc("")
	a.ChosenArticles.Add(0)
	a.ChosenArticles.Add(1)

	a.ConfirmDeleteChosenArticles()

	if len(a.Articles) != 1 {
		t.Fatalf("kept %d articles, want 1", len(a.Articles))
	}
	if got := a.CurrentArticle(); got == nil || got.Title != "C" {
		t.Errorf("the active article after deleting the others is %+v, want C", got)
	}
}

func TestChosenArticleIndexesAreInListOrder(t *testing.T) {
	a := &App{Articles: []*Article{{Title: "A"}, {Title: "B"}, {Title: "C"}}}
	a.ChosenArticles.Add(2)
	a.ChosenArticles.Add(0)

	if got, want := a.chosenArticleIndexes(), []int{0, 2}; !slices.Equal(got, want) {
		t.Errorf("chosenArticleIndexes() = %v, want %v", got, want)
	}
}

func TestSaveNowReportsAFailedSave(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}

	base := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	if err := os.Chmod(base, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(base, 0755)

	a := &App{
		Settings:      &AppSettings{ProjectStates: map[string]ProjectSelectionState{}},
		ActiveProject: "Reading",
		Articles:      []*Article{{Title: "A"}},
		ActiveIdx:     0,
	}
	a.initSummaryDoc("")

	a.SaveNow()

	if a.LastSaveError == "" {
		t.Error("a failed save was reported as successful")
	}
	if !a.IsDirty {
		t.Error("a failed save cleared the dirty state")
	}
	if a.LastSavedAt != "" {
		t.Errorf("a failed save set LastSavedAt = %q", a.LastSavedAt)
	}
}
