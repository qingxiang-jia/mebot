package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// fontCombobox shows the font picker: a field that filters fonts as the
// user types, with the current font scrolled into view when the popup
// opens.
func (a *App) fontCombobox(c *ui.Context) {
	t := c.Theme()
	wasOpen := a.FontPickerOpen

	var cb ui.ComboboxParts
	var arrow ui.Element
	f := ui.Row(c).Key("fontFamilySelector").
		AlignItems(ui.Center).Gap(t.Space(1.5)).
		Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).
		Background(t.Surface).Cursor(ui.CursorText).Width(160)
	f.Children(func() {
		cb = ui.ComboboxBase(c, &a.FontPickerText)
		cb.Input.Grow(1).MinWidth(t.Space(25)).Label("Font")
		arrow = ui.Box(c).Size(t.Space(2.5), t.Space(2.5)).Shrink(0).KeepFocus()
		arrow.Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.MoveTo(r.X+r.W*0.1, r.Y+r.H*0.3).
				LineTo(r.X+r.W*0.5, r.Y+r.H*0.7).
				LineTo(r.X+r.W*0.9, r.Y+r.H*0.3)
			p.StrokePath(&path, 1.5, t.TextMuted)
		})
	})

	switch {
	case cb.Input.Focused():
		f.Border(1, t.Accent)
	case f.Hovered():
		f.Border(1, t.Border.Mix(t.Text, 0.25))
	default:
		f.Border(1, t.Border)
	}

	openedByClick := false
	if arrow.Clicked() {
		open := !cb.Open()
		cb.SetOpen(open)
		cb.Input.Focus()
		openedByClick = open
	}
	if !wasOpen && cb.Open() && cb.Input.Clicked() {
		openedByClick = true
	}
	if f.Clicked() {
		cb.Input.Focus()
	}
	if openedByClick {
		// As choosing a font does: typing replaces the choice, and
		// filters from its start.
		cb.Input.SetTextSelection(0, len(a.FontPickerText))
	}

	shown := a.AvailableFonts
	if cb.Filtering() {
		shown = fontMatches(a.AvailableFonts, a.FontPickerText)
	}

	cb.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).
			Radius(t.Radius+2).Background(t.Background).
			Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Scroll(c).WidthPercent(100).MaxHeight(t.Space(70)).Children(func() {
			for _, font := range shown {
				item := cb.Item(font).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
				switch {
				case item.Highlighted():
					item.Background(t.Accent).TextColor(t.AccentText)
				case font == a.SelectedFont:
					item.Background(t.Surface)
				}
				if openedByClick && font == a.SelectedFont {
					item.ScrollIntoView()
				}
				item.Children(func() { ui.Text(c, font).SingleLine() })
			}
		})
	})

	if font, ok := cb.Chosen(); ok {
		if a.SelectedFont != font {
			a.SelectedFont = font
			a.Settings.FontFamily = font
			_ = a.Settings.Save()
		}
		a.FontPickerText = font
	} else if !cb.Input.Focused() && !cb.Open() && a.FontPickerText != a.SelectedFont {
		a.FontPickerText = a.SelectedFont
	}
	a.FontPickerOpen = cb.Open()
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
