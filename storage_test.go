package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMarkdownRollupRoundTripsDuplicateTitles(t *testing.T) {
	articles := []*Article{
		{Title: "Same", FullText: "First body.", Summary: "First summary."},
		{Title: "Other", FullText: "Other body."},
		{Title: "Same", FullText: "Second body.", Summary: "Second summary."},
	}

	parsed, err := ParseMarkdownRollup(SerializeToMarkdown(articles))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(articles) {
		t.Fatalf("parsed %d articles, want %d: %+v", len(parsed), len(articles), parsed)
	}

	want := []struct {
		title, fullText, summary string
	}{
		{"Same", "First body.", "First summary."},
		{"Other", "Other body.", ""},
		{"Same", "Second body.", "Second summary."},
	}
	for i, w := range want {
		got := parsed[i]
		if got.Title != w.title || got.FullText != w.fullText || got.Summary != w.summary {
			t.Errorf("article %d = %+v, want title %q, body %q, summary %q",
				i, got, w.title, w.fullText, w.summary)
		}
	}
}

func TestParseMarkdownRollupIgnoresSectionNamesInBodyText(t *testing.T) {
	rollup := "# Full Text\n\n" +
		"## One\n\n" +
		"The article mentions the # Summary section without starting one.\n\n" +
		"# Summary\n\n" +
		"## One\n\n" +
		"A short summary.\n"

	parsed, err := ParseMarkdownRollup(rollup)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("parsed %d articles, want 1: %+v", len(parsed), parsed)
	}
	if got := parsed[0].FullText; got != "The article mentions the # Summary section without starting one." {
		t.Errorf("FullText = %q", got)
	}
	if got := parsed[0].Summary; got != "A short summary." {
		t.Errorf("Summary = %q", got)
	}
}

func TestParseMarkdownRollupTrimsHeaderIndentation(t *testing.T) {
	rollup := "  # Full Text\n\n## One\n\nBody text.\n\n  # Summary\n\n## One\n\nSummary text.\n"

	parsed, err := ParseMarkdownRollup(rollup)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("parsed %d articles, want 1: %+v", len(parsed), parsed)
	}
	if got := parsed[0].FullText; got != "Body text." {
		t.Errorf("FullText = %q", got)
	}
	if got := parsed[0].Summary; got != "Summary text." {
		t.Errorf("Summary = %q", got)
	}
}

func TestSaveProjectStateRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	articles := []*Article{
		{Title: "One", FullText: "First body.", Summary: "First summary."},
		{Title: "Two", FullText: "Second body."},
	}
	if err := SaveProjectState("Reading", articles); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProjectState("Reading")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(articles) {
		t.Fatalf("loaded %d articles, want %d", len(loaded), len(articles))
	}
	if loaded[0].Title != "One" || loaded[0].FullText != "First body." ||
		loaded[0].Summary != "First summary." {
		t.Errorf("first article = %+v", loaded[0])
	}
	if loaded[1].Title != "Two" || loaded[1].FullText != "Second body." ||
		loaded[1].Summary != "" {
		t.Errorf("second article = %+v", loaded[1])
	}
}

func TestWriteFileAtomicKeepsTheOldFileOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "project.md")
	if err := writeFileAtomic(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755)

	if err := writeFileAtomic(path, []byte("new"), 0644); err == nil {
		t.Fatal("writeFileAtomic succeeded in a read-only directory")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Errorf("the failed write changed the file to %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "project.md" {
			t.Errorf("a failed write left %q behind", entry.Name())
		}
	}
}

func TestStateDirMigratesTheOldShareFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	shareDir := filepath.Join(home, ".local", "share", "mebot")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(shareDir, "Old Project.md")
	if err := os.WriteFile(oldFile, []byte("# Full Text\n\n"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := StateDir()
	if _, err := os.Stat(filepath.Join(dir, "Old Project.md")); err != nil {
		t.Errorf("the old project was not migrated: %v", err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("the old project was not removed from the share folder: %v", err)
	}

	// Migration runs once, so a file added later is left alone rather than
	// moved on every settings save.
	later := filepath.Join(shareDir, "Later.md")
	if err := os.WriteFile(later, []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}
	StateDir()
	if _, err := os.Stat(later); err != nil {
		t.Errorf("a later share file was migrated on a repeat call: %v", err)
	}
}

func TestValidateProjectName(t *testing.T) {
	for _, name := range []string{"", "  ", ".", "..", "a/b", `a\b`, "a\x00b", "a\nb"} {
		if err := ValidateProjectName(name); err == nil {
			t.Errorf("ValidateProjectName(%q) accepted the name", name)
		}
	}
	for _, name := range []string{"Weekly", "2026-11-14 Notes", "A.B", "Économie"} {
		if err := ValidateProjectName(name); err != nil {
			t.Errorf("ValidateProjectName(%q) = %v", name, err)
		}
	}
}
