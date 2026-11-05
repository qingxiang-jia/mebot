package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultSummaryPrompt = "Can you provide a detailed summary for each of the following articles? Do not change the title. I prefer simple English terms and no bullet points so that the flow is smooth."
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

	// ExportDirectory is where the weekly rollup was last exported.
	ExportDirectory string `json:"export_directory"`

	// SummaryPrompt is the instruction sent to the local model. The JSON tag
	// is kept from the Gemini version so prompts saved by older builds are
	// preserved when upgrading.
	SummaryPrompt string `json:"gemini_prompt"`
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
		SummaryPrompt: DefaultSummaryPrompt,
	}

	data, err := os.ReadFile(SettingsFilePath())
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.ProjectStates == nil {
		s.ProjectStates = make(map[string]ProjectSelectionState)
	}
	if strings.TrimSpace(s.SummaryPrompt) == "" {
		s.SummaryPrompt = DefaultSummaryPrompt
	}
	if strings.TrimSpace(s.ExportDirectory) == "" {
		s.ExportDirectory = DefaultExportDir()
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
