package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsMigratesLegacyGeminiModel(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	stateDir := filepath.Join(stateHome, "mebot")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacy := `{"gemini_model": "gemini-3.8-flash"}`
	if err := os.WriteFile(filepath.Join(stateDir, "settings.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	settings := LoadSettings()
	if settings.GeminiModel != DefaultGeminiModel {
		t.Errorf("GeminiModel = %q, want %q", settings.GeminiModel, DefaultGeminiModel)
	}
}

// TestLoadSettingsKeepsLegacyModelChosenAfterMigration checks that the
// migration off the old default runs once: a user who picks that model on
// purpose keeps it across restarts.
func TestLoadSettingsKeepsLegacyModelChosenAfterMigration(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)

	stateDir := filepath.Join(stateHome, "mebot")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacy := `{"gemini_model": "gemini-3.8-flash"}`
	if err := os.WriteFile(filepath.Join(stateDir, "settings.json"), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	// The first load migrates the old default.
	first := LoadSettings()
	if first.GeminiModel != DefaultGeminiModel {
		t.Fatalf("GeminiModel = %q, want %q", first.GeminiModel, DefaultGeminiModel)
	}

	// The user then chooses the old model on purpose from Settings.
	first.GeminiModel = legacyGeminiModel
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}

	second := LoadSettings()
	if second.GeminiModel != legacyGeminiModel {
		t.Errorf("GeminiModel = %q, want the chosen %q", second.GeminiModel, legacyGeminiModel)
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
	if settings.GeminiModel != DefaultGeminiModel {
		t.Errorf("GeminiModel = %q, want %q", settings.GeminiModel, DefaultGeminiModel)
	}
	if settings.GeminiPrompt != DefaultGeminiPrompt {
		t.Errorf("GeminiPrompt = %q, want the default prompt", settings.GeminiPrompt)
	}
	if settings.GeminiAPIKey != "" {
		t.Errorf("GeminiAPIKey = %q, want empty", settings.GeminiAPIKey)
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
