package main

import "github.com/egoist/mygo/ui"

type Theme struct {
	IsDark    bool
	Bg        ui.Color
	PanelBg   ui.Color
	CardBg    ui.Color
	Border    ui.Color
	Text      ui.Color
	TextMuted ui.Color
	Accent    ui.Color
}

func LightTheme() Theme {
	return Theme{
		IsDark:    false,
		Bg:        ui.RGB(246, 245, 240),
		PanelBg:   ui.RGB(255, 255, 255),
		CardBg:    ui.RGB(240, 239, 234),
		Border:    ui.RGB(220, 218, 210),
		Text:      ui.RGB(35, 38, 41),
		TextMuted: ui.RGB(115, 115, 120),
		Accent:    ui.RGB(190, 40, 40),
	}
}

func DarkTheme() Theme {
	return Theme{
		IsDark:    true,
		Bg:        ui.RGB(20, 21, 23),
		PanelBg:   ui.RGB(28, 30, 34),
		CardBg:    ui.RGB(36, 38, 43),
		Border:    ui.RGB(48, 52, 58),
		Text:      ui.RGB(230, 230, 230),
		TextMuted: ui.RGB(145, 150, 160),
		Accent:    ui.RGB(240, 120, 90),
	}
}
