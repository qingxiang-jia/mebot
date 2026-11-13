package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/qingxiang-jia/go-look/apple"
)

// fontChooser is the font panel inside the typography popover: a search
// field over the family list, as the macOS Fonts panel has, rather than a
// second popup opened from the first. Each family is drawn in its own face,
// and the family in use is filled with the accent.
func (a *App) fontChooser(c *ui.Context) {
	t := c.Theme()

	search := ui.TextInput(c.Key("fontSearch"), &a.FontSearch).
		Placeholder("Search Fonts").Height(apple.ControlHeight).
		Padding(0, apple.Space).FontSize(apple.FontBody).
		Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
		AutoFocus()
	search.Label("Search Fonts")

	shown := a.AvailableFonts
	if strings.TrimSpace(a.FontSearch) != "" {
		shown = fontMatches(a.AvailableFonts, a.FontSearch)
	}

	ui.Scroll(c).FillWidth().Height(150).Children(func() {
		if len(shown) == 0 {
			ui.Text(c, "No matching fonts").
				FontSize(apple.FontBody).TextColor(t.TextMuted).
				Padding(apple.Space, apple.Space)
			return
		}
		rows := ui.Column(c).FillWidth().Gap(1)
		rows.Children(func() {
			for _, font := range shown {
				row := ui.ButtonBase(c.Key("font-"+font)).Label(font).
					FillWidth().Height(26).Shrink(0).Radius(t.Radius).
					Padding(0, apple.Space).AlignItems(ui.Center)
				switch {
				case font == a.SelectedFont:
					row.Background(t.Accent)
				case row.Hovered():
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ink := t.Text
					if font == a.SelectedFont {
						ink = t.AccentText
					}
					ui.Text(c, font).Font(font).FontSize(apple.FontBody).
						SingleLine().TextColor(ink)
					ui.Row(c).Grow(1)
					if font == a.SelectedFont {
						ui.Icon(c, iconCheck).FontSize(11).TextColor(t.AccentText)
					}
				})
				if a.TypographyJustOpened && font == a.SelectedFont {
					row.ScrollIntoView()
				}
				if row.Clicked() && a.SelectedFont != font {
					a.SelectedFont = font
					a.Settings.FontFamily = font
					_ = a.Settings.Save()
				}
			}
		})
	})
	a.TypographyJustOpened = false
}

// fontMatches returns the fonts containing query, ignoring case, those
// starting with it first, and all of them for a blank query.
func fontMatches(fonts []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return fonts
	}
	var starts, contains []string
	for _, font := range fonts {
		l := strings.ToLower(font)
		switch {
		case strings.HasPrefix(l, q):
			starts = append(starts, font)
		case strings.Contains(l, q):
			contains = append(contains, font)
		}
	}
	return append(starts, contains...)
}
