package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsKeepsSavedSummaryPrompt(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	stateDir := filepath.Join(stateHome, "mebot")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Older builds stored the prompt under "gemini_prompt". The key is
	// deliberately kept so the user's custom prompt survives the upgrade.
	legacy := `{"gemini_prompt": "Summarize these in one sentence."}`
	if err := os.WriteFile(filepath.Join(stateDir, "settings.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	settings := LoadSettings()
	if settings.SummaryPrompt != "Summarize these in one sentence." {
		t.Errorf("SummaryPrompt = %q, want the saved prompt", settings.SummaryPrompt)
	}
}

func TestLoadSettingsBackwardsCompatible(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	stateDir := filepath.Join(stateHome, "mebot")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacy := `{
		"font_family": "Serif",
		"font_size": "18",
		"is_dark_mode": true,
		"last_project": "Old Project",
		"project_states": {}
	}`
	if err := os.WriteFile(filepath.Join(stateDir, "settings.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	settings := LoadSettings()

	if settings.FontFamily != "Serif" {
		t.Errorf("FontFamily = %q", settings.FontFamily)
	}
	if settings.FontSize != "18" {
		t.Errorf("FontSize = %q", settings.FontSize)
	}
	if !settings.IsDarkMode {
		t.Error("IsDarkMode should stay true")
	}
	if settings.LastProject != "Old Project" {
		t.Errorf("LastProject = %q", settings.LastProject)
	}
	if settings.SummaryPrompt != DefaultSummaryPrompt {
		t.Errorf("SummaryPrompt = %q, want the default prompt", settings.SummaryPrompt)
	}
}

func TestSettingsKeepExportDirectory(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	settings := LoadSettings()
	wantDefault := filepath.Join(home, "Documents")
	if settings.ExportDirectory != wantDefault {
		t.Fatalf("ExportDirectory = %q, want the default %q", settings.ExportDirectory, wantDefault)
	}

	chosen := filepath.Join(home, "Exports")
	settings.ExportDirectory = chosen
	if err := settings.Save(); err != nil {
		t.Fatal(err)
	}

	if got := LoadSettings().ExportDirectory; got != chosen {
		t.Errorf("ExportDirectory = %q, want the saved %q", got, chosen)
	}
}
