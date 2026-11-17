package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
)

func rgbaOfColor(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

func TestThemeCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range themeDefs {
		if d.id == "" || d.label == "" || seen[d.id] {
			t.Fatalf("bad or repeated theme: %+v", d)
		}
		seen[d.id] = true
	}
	for _, id := range []string{"dracula", "catppuccin-mocha", "catppuccin-latte", "tokyonight", "rosepine", "gruvbox-dark", "nord", "solarized-dark", "onedark"} {
		if !seen[id] {
			t.Errorf("missing theme %s", id)
		}
	}
	if themeDefByID("nope").id != themeAuto {
		t.Fatal("an unknown id falls back to auto")
	}
}

// TestThemesAreReadable checks every palette with the WCAG contrast ratio: the
// text, the secondary text, the accent and the text on an accent button.
func TestThemesAreReadable(t *testing.T) {
	for _, d := range themeDefs {
		th := buildTheme(d.id, true)
		if th.bg == nil {
			continue // auto: the terminal's own colors
		}
		bg := rgbaOfColor(th.bg)
		checks := []struct {
			name   string
			fg, bg color.RGBA
			min    float64
		}{
			// Text on the background itself must be fully readable; on the tinted
			// surfaces (selection, panels) a little less is fair, since the official
			// light palettes are tuned for their own background; the decorative chips
			// are bold, short labels.
			{"text", rgbaOfColor(th.text), bg, 4.5},
			{"muted", rgbaOfColor(th.muted), bg, 3.0},
			{"faint", rgbaOfColor(th.faint), bg, 2.0},
			{"accent", rgbaOfColor(th.accent), bg, 3.0},
			{"accent button", rgbaOfColor(th.accentText), rgbaOfColor(th.accent), 3.0},
			{"text on selection", rgbaOfColor(th.text), rgbaOfColor(th.selected), 3.5},
			{"accent on selection", rgbaOfColor(th.accent), rgbaOfColor(th.selected), 2.5},
			{"text on panel", rgbaOfColor(th.text), rgbaOfColor(th.panel), 3.5},
			{"badge", rgbaOfColor(th.badgeFg), rgbaOfColor(th.badgeBg), 2.2},
			{"now chip", rgbaOfColor(th.nowFg), rgbaOfColor(th.nowBg), 2.2},
		}
		for _, c := range checks {
			if got := contrast(c.fg, c.bg); got < c.min {
				t.Errorf("%s: %s contrast is %.2f, want at least %.1f", d.id, c.name, got, c.min)
			}
		}
		if len(th.hues) < 7 {
			t.Errorf("%s: playlist hues missing", d.id)
		}
	}
}

func TestThemePicker(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	if tu.themeID != themeAuto {
		t.Fatalf("starts on auto: %s", tu.themeID)
	}
	tu.press("T")
	if tu.modal.kind != modalTheme {
		t.Fatal("T opens the picker")
	}
	tu.press("down", "down", "down")
	chosen := themeDefs[3].id
	if tu.themeID != chosen || tu.th.bg == nil {
		t.Fatalf("moving previews the theme: %s", tu.themeID)
	}
	if !strings.Contains(tu.screen(), themeDefs[3].label) {
		t.Fatal("the picker lists the themes")
	}
	tu.press("esc")
	if tu.themeID != themeAuto || tu.th.bg != nil {
		t.Fatalf("esc goes back to the theme in use: %s", tu.themeID)
	}
	if _, err := os.Stat(prefsFile(t)); err == nil {
		t.Fatal("cancelling saves nothing")
	}

	tu.press("T", "down", "down", "down", "enter")
	if tu.themeID != chosen || tu.modal.kind != modalNone {
		t.Fatalf("enter keeps the theme: %s", tu.themeID)
	}
	// It is remembered: a new app starts on it.
	again := newTUI(tu.eng, false)
	if again.themeID != chosen {
		t.Fatalf("the choice is saved: %s", again.themeID)
	}
	if !strings.Contains(string(readFile(t, prefsFile(t))), chosen) {
		t.Fatal("tui.json holds the theme")
	}
	// settings.json, which the desktop app rewrites, is not where it lives.
	sp, _ := core.SettingsPath()
	if data, err := os.ReadFile(sp); err == nil && strings.Contains(string(data), chosen) {
		t.Fatal("the theme must not go in settings.json")
	}
}

func TestThemeFromPaletteAndSettings(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.press("ctrl+p")
	tu.typeText("tema")
	if len(tu.pal.hits) == 0 || tu.pal.hits[0].ActionID != "theme" {
		t.Fatalf("the palette finds the theme action: %+v", tu.pal.hits)
	}
	tu.press("enter")
	if tu.modal.kind != modalTheme {
		t.Fatal("the action opens the picker")
	}
	tu.press("esc", ",", "]")
	if !strings.Contains(tu.screen(), "Tema") {
		t.Fatal("Apariencia has a theme field")
	}
	tu.press("enter")
	if tu.modal.kind != modalTheme {
		t.Fatal("enter on the field opens the picker")
	}
	tu.press("esc")
	tu.press("right")
	if tu.themeID != themeDefs[1].id {
		t.Fatalf("left/right on the field changes the theme: %s", tu.themeID)
	}
}

func TestThemeBackground(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	if v := tu.View(); v.BackgroundColor != nil {
		t.Fatal("auto leaves the terminal's background alone")
	}
	tu.setTheme("dracula")
	v := tu.View()
	if v.BackgroundColor == nil || rgbaOfColor(v.BackgroundColor) != hexRGBA("#282a36") {
		t.Fatalf("a named theme sets its background: %v", v.BackgroundColor)
	}
}

func TestEveryThemeFits(t *testing.T) {
	for _, d := range themeDefs {
		tu, _ := newTestTUI(t, 100, 30)
		tu.setTheme(d.id)
		tu.press("enter")
		for _, k := range []string{"1", "2", "3", "4", "7"} {
			tu.press(k)
			for i, l := range strings.Split(tu.screen(), "\n") {
				if w := ansi.StringWidth(l); w > 100 {
					t.Fatalf("%s: line %d is %d wide", d.id, i, w)
				}
			}
		}
		tu.press("M")
		_ = tu.screen()
	}
}

func prefsFile(t *testing.T) string {
	t.Helper()
	p, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestRenderThemes writes a PNG of the app in each theme when MUSAIK_SNAPSHOT_DIR is set.
func TestRenderThemes(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	cover := testCover()
	for _, d := range themeDefs {
		tu, _ := newTestTUI(t, 110, 28)
		for _, s := range tu.eng.Store.Songs {
			tu.art[core.CoverKey(s)] = &artEntry{img: cover, colors: core.PaletteFromImage(cover)}
		}
		tu.setTheme(d.id)
		tu.press("down", "enter", "f", "down")
		bg := color.RGBA{R: 12, G: 12, B: 14, A: 255}
		if tu.th.bg != nil {
			bg = rgbaOfColor(tu.th.bg)
		}
		writePNGFile(t, filepath.Join(dir, "theme-"+d.id+".png"), ansiToImage(tu.View().Content, 8, 16, bg))
	}
	tu, _ := newTestTUI(t, 110, 28)
	tu.setTheme("dracula")
	tu.press("T", "down", "down")
	bg := rgbaOfColor(tu.th.bg)
	writePNGFile(t, filepath.Join(dir, "theme-picker.png"), ansiToImage(tu.View().Content, 8, 16, bg))
}
