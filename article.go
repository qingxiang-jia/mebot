package main

type Article struct {
	Title    string `json:"title"`
	FullText string `json:"full_text"`
	Summary  string `json:"summary"`
	Selected bool   `json:"selected,omitempty"`
}
