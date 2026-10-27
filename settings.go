package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultGeminiModel  = "gemini-3.5-flash-lite"
	DefaultGeminiPrompt = "Can you provide a detailed summary for each of the following articles? Do not change the title. I prefer simple English terms and no bullet points so that the flow is smooth."

	// legacyGeminiModel was the default before Gemini 3.8 Flash started
	// returning 503 on the free tier. Existing settings are moved to the
	// current default on load.
	legacyGeminiModel = "gemini-3.8-flash"
)

type ProjectSelectionState struct {
	ActiveIdx int   `json:"active_idx"`
	Selected  []int `json:"selected"`
}

type AppSettings struct {
	FontFamily    string                           `json:"font_family"`
	FontSize      string                           `json:"font_size"`
	IsDarkMode    bool                             `json:"is_dark_mode"`
	LastProject   string                           `json:"last_project"`
	ProjectStates map[string]ProjectSelectionState `json:"project_states"`

	GeminiAPIKey string `json:"gemini_api_key"`
	GeminiModel  string `json:"gemini_model"`
	GeminiPrompt string `json:"gemini_prompt"`
}

func SettingsFilePath() string {
	return filepath.Join(StateDir(), "settings.json")
}

func LoadSettings() *AppSettings {
	s := &AppSettings{
		FontFamily:    "Sans",
		FontSize:      "16",
		IsDarkMode:    false,
		LastProject:   "",
		ProjectStates: make(map[string]ProjectSelectionState),
		GeminiModel:   DefaultGeminiModel,
		GeminiPrompt:  DefaultGeminiPrompt,
	}

	data, err := os.ReadFile(SettingsFilePath())
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.ProjectStates == nil {
		s.ProjectStates = make(map[string]ProjectSelectionState)
	}
	if strings.TrimSpace(s.GeminiModel) == "" || s.GeminiModel == legacyGeminiModel {
		s.GeminiModel = DefaultGeminiModel
	}
	if strings.TrimSpace(s.GeminiPrompt) == "" {
		s.GeminiPrompt = DefaultGeminiPrompt
	}
	return s
}

func (s *AppSettings) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsFilePath(), data, 0644)
}
