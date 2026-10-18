package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	}

	data, err := os.ReadFile(SettingsFilePath())
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.ProjectStates == nil {
		s.ProjectStates = make(map[string]ProjectSelectionState)
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
