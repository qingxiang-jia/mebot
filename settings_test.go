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
