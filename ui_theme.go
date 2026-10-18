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
		Bg:        ui.RGB(247, 246, 242), // Soft warm paper
		PanelBg:   ui.RGB(255, 255, 255), // Pure white surfaces
		CardBg:    ui.RGB(242, 241, 236),
		Border:    ui.RGB(226, 224, 216),
		Text:      ui.RGB(33, 37, 41), // Soft deep charcoal
		TextMuted: ui.RGB(110, 115, 125),
		Accent:    ui.RGB(180, 40, 40), // Crimson editorial accent
	}
}

func DarkTheme() Theme {
	return Theme{
		IsDark:    true,
		Bg:        ui.RGB(18, 19, 21), // Deep slate background
		PanelBg:   ui.RGB(26, 28, 32), // Elevated panels
		CardBg:    ui.RGB(34, 37, 43), // Input & preview surfaces
		Border:    ui.RGB(50, 54, 64),
		Text:      ui.RGB(226, 230, 236), // Off-white readable text
		TextMuted: ui.RGB(140, 146, 160),
		Accent:    ui.RGB(235, 115, 85), // Warm peach/orange accent
	}
}
