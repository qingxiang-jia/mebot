package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFilterCleanReadingFontsDropsNotoVariants(t *testing.T) {
	fonts := []string{
		"Adwaita Sans",
		"Noto Kufi Arabic",
		"Noto Sans",
		"Noto Sans Arabic",
		"Noto Sans Bengali UI Medium",
		"Noto Sans Black",
		"Noto Sans Display",
		"Noto Sans Mono",
		"Noto Serif",
		"Noto Serif Ahom",
		"Noto Serif Display",
	}
	want := []string{
		"Adwaita Sans",
		"Noto Sans",
		"Noto Sans Display",
		"Noto Sans Mono",
		"Noto Serif",
		"Noto Serif Display",
	}
	if got := FilterCleanReadingFonts(fonts); !reflect.DeepEqual(got, want) {
		t.Errorf("FilterCleanReadingFonts = %q, want %q", got, want)
	}
}

func TestFontMatches(t *testing.T) {
	fonts := []string{"DejaVu Sans", "Noto Sans", "Noto Serif", "Open Sans"}
	got := fontMatches(fonts, "noto")
	want := []string{"Noto Sans", "Noto Serif"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fontMatches = %q, want %q", got, want)
	}
	if got := fontMatches(fonts, "  "); !reflect.DeepEqual(got, fonts) {
		t.Errorf("fontMatches with a blank query = %q, want all fonts", got)
	}
}

func TestFontComboboxOpensAtTheSelectedFont(t *testing.T) {
	var fonts []string
	for i := 0; i < 100; i++ {
		fonts = append(fonts, fmt.Sprintf("Font %02d", i))
	}
	selected := "Font 90"
	app := &App{
		AvailableFonts: fonts,
		SelectedFont:   selected,
		FontPickerText: selected,
		Settings:       &AppSettings{},
	}
	tt := ui.NewTester(func(c *ui.Context) { app.fontCombobox(c) }, 400, 300)
	if err := tt.Click("Font"); err != nil {
		t.Fatal(err)
	}
	r, ok := tt.Find(selected)
	if !ok {
		t.Fatalf("the selected font is not shown: %q", tt.Texts())
	}
	if r.Y < 0 || r.Y+r.H > 300 {
		t.Errorf("the selected font is at y=%v..%v, outside the 300 DIP window", r.Y, r.Y+r.H)
	}

	// Typing after opening replaces the choice and filters the fonts.
	tt.Type("90")
	if !tt.HasText("Font 90") || tt.HasText("Font 00") {
		t.Errorf("typing after opening shows %q", tt.Texts())
	}
}
