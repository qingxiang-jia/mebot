package main

import "github.com/egoist/mygo/ui"

type Theme struct {
	IsDark    bool
	Bg        ui.Color
	PanelBg   ui.Color
	Border    ui.Color
	Text      ui.Color
	TextMuted ui.Color
	Accent    ui.Color
}

func LightTheme() Theme {
	return Theme{
		IsDark:    false,
		Bg:        ui.RGB(246, 245, 240), // Paper-like warm background
		PanelBg:   ui.RGB(255, 255, 255),
		Border:    ui.RGB(225, 223, 216),
		Text:      ui.RGB(35, 38, 41), // Soft charcoal ink
		TextMuted: ui.RGB(115, 115, 120),
		Accent:    ui.RGB(190, 40, 40), // Editorial crimson
	}
}

func DarkTheme() Theme {
	return Theme{
		IsDark:    true,
		Bg:        ui.RGB(24, 25, 27),
		PanelBg:   ui.RGB(32, 34, 38),
		Border:    ui.RGB(50, 54, 60),
		Text:      ui.RGB(230, 230, 230),
		TextMuted: ui.RGB(145, 150, 160),
		Accent:    ui.RGB(240, 120, 90),
	}
}
