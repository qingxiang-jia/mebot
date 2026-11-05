package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Icons from the elementary icon theme (elementary/icons, GPL-3.0),
// shown in their own colors.

//go:embed icons/*.svg
var iconFiles embed.FS

func mustIcon(name string) *ui.SVG {
	data, err := iconFiles.ReadFile("icons/" + name)
	if err != nil {
		panic("icon " + name + ": " + err.Error())
	}
	return ui.MustParseSVG(data)
}

var (
	// actions: list-add
	IconAdd = mustIcon("add.svg")

	// actions: document-edit
	IconEdit = mustIcon("edit.svg")

	// places: user-trash
	IconTrash = mustIcon("trash.svg")

	// mimes: text-html
	IconImport = mustIcon("import.svg")

	// mimes: x-office-document
	IconDocument = mustIcon("document.svg")

	// actions: document-export
	IconExport = mustIcon("export.svg")

	// status: weather-clear, tinted with the theme's yellow
	IconSun = mustIcon("sun.svg")

	// status: weather-clear-night, tinted with the theme's blue and yellow
	IconMoon = mustIcon("moon.svg")

	// actions: edit-copy
	IconCopy = mustIcon("copy.svg")

	// actions: edit-paste
	IconPaste = mustIcon("paste.svg")

	// status: process-completed
	IconSelectAll = mustIcon("select-all.svg")

	// status: process-stop
	IconSelectNone = mustIcon("select-none.svg")

	// status: starred (AI summary)
	IconSparkle = mustIcon("sparkle.svg")

	// actions: system-shutdown (local LLM power)
	IconPower = mustIcon("power.svg")

	// categories: preferences-system (Settings)
	IconSettings = mustIcon("settings.svg")
)
