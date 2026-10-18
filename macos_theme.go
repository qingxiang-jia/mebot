package main

import "github.com/egoist/mygo/ui"

type MacOSTheme struct {
	IsDark        bool
	WindowBg      ui.Color // macOS window chrome
	ToolbarBg     ui.Color // macOS unified toolbar
	SidebarBg     ui.Color // macOS source list grouped sidebar
	CardBg        ui.Color // macOS reading sheet & editor canvas
	Border        ui.Color // 1px separator border
	Text          ui.Color // Primary label
	TextMuted     ui.Color // Secondary label
	Accent        ui.Color // macOS System Blue (#007AFF)
	ActiveRowBg   ui.Color // Active selection fill
	ActiveRowText ui.Color // Active selection text
	HoverBg       ui.Color // Subtle row hover
}

func MacOSLightTheme() MacOSTheme {
	return MacOSTheme{
		IsDark:        false,
		WindowBg:      ui.RGB(242, 242, 247), // macOS systemGroupedBackground
		ToolbarBg:     ui.RGB(246, 246, 246), // macOS unified toolbar background
		SidebarBg:     ui.RGB(238, 238, 242), // macOS Finder/Notes source list
		CardBg:        ui.RGB(255, 255, 255), // macOS pure white content sheet
		Border:        ui.RGB(218, 218, 222), // macOS subtle divider
		Text:          ui.RGB(28, 28, 30),    // macOS primary label
		TextMuted:     ui.RGB(142, 142, 147), // macOS secondary label
		Accent:        ui.RGB(0, 122, 255),   // macOS System Blue
		ActiveRowBg:   ui.RGB(0, 122, 255),   // Blue active row fill
		ActiveRowText: ui.RGB(255, 255, 255), // White text on active row
		HoverBg:       ui.RGB(228, 228, 234),
	}
}

func MacOSDarkTheme() MacOSTheme {
	return MacOSTheme{
		IsDark:        true,
		WindowBg:      ui.RGB(28, 28, 30),    // macOS dark systemGroupedBackground
		ToolbarBg:     ui.RGB(38, 38, 42),    // macOS dark toolbar
		SidebarBg:     ui.RGB(32, 32, 36),    // macOS dark source list sidebar
		CardBg:        ui.RGB(24, 24, 26),    // macOS dark canvas
		Border:        ui.RGB(54, 54, 58),    // macOS dark divider
		Text:          ui.RGB(242, 242, 247), // macOS dark primary label
		TextMuted:     ui.RGB(152, 152, 157), // macOS dark secondary label
		Accent:        ui.RGB(10, 132, 255),  // macOS dark vivid blue
		ActiveRowBg:   ui.RGB(10, 132, 255),  // Blue active row fill
		ActiveRowText: ui.RGB(255, 255, 255), // White text on active row
		HoverBg:       ui.RGB(44, 44, 48),
	}
}
