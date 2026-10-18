package main

import "github.com/egoist/mygo/ui"

type Theme struct {
	IsDark    bool
	ToolbarBg ui.Color // Distinct top bar color
	SidebarBg ui.Color // Soft linen/parchment in light, muted charcoal in dark
	ReadingBg ui.Color // Soft paper background for long-form reading
	EditorBg  ui.Color // Elevated card surface for rich text
	Border    ui.Color
	Text      ui.Color
	TextMuted ui.Color
	Accent    ui.Color
	Highlight ui.Color
}

func LightTheme() Theme {
	return Theme{
		IsDark:    false,
		ToolbarBg: ui.RGB(236, 233, 225), // Distinct warm-grey toolbar
		SidebarBg: ui.RGB(244, 241, 234), // Soft linen/cream sidebar (not harsh white)
		ReadingBg: ui.RGB(252, 250, 246), // Classic editorial paper reading pane
		EditorBg:  ui.RGB(248, 246, 241), // Rich text card
		Border:    ui.RGB(222, 218, 208),
		Text:      ui.RGB(38, 42, 46), // Readable charcoal ink
		TextMuted: ui.RGB(115, 120, 128),
		Accent:    ui.RGB(175, 42, 42), // Editorial crimson
		Highlight: ui.RGB(230, 226, 216),
	}
}

func DarkTheme() Theme {
	return Theme{
		IsDark:    true,
		ToolbarBg: ui.RGB(24, 26, 30), // Dark slate toolbar
		SidebarBg: ui.RGB(28, 31, 36), // Muted slate sidebar
		ReadingBg: ui.RGB(18, 20, 23), // Eye-friendly deep charcoal reading pane
		EditorBg:  ui.RGB(26, 29, 34), // Card surface
		Border:    ui.RGB(46, 50, 58),
		Text:      ui.RGB(226, 228, 232), // Soft pearl white text
		TextMuted: ui.RGB(138, 144, 156),
		Accent:    ui.RGB(232, 110, 75), // Terracotta accent
		Highlight: ui.RGB(40, 44, 52),
	}
}
