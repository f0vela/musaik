package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"musaik/internal/core"
)

// palette is what a named color scheme gives: the background, the text, the
// "comment" gray and a handful of hues. Everything else the app needs (borders,
// selection, panels, chips) is mixed from these, so every scheme gets the same
// treatment and a typo in one color cannot leave a corner unreadable.
type palette struct {
	dark                                         bool
	bg, text, faint, accent                      string
	red, orange, yellow, green, cyan, blue, purp string
}

type themeDef struct {
	id, label string
	dark      bool
	pal       *palette // nil: the desktop app's own look
}

const themeAuto = "auto"

// themeDefs are the themes the picker offers, in order.
var themeDefs = []themeDef{
	{id: themeAuto, label: "Automático (sigue la terminal)"},
	{id: "sonora-dark", label: "Sonora Oscuro", dark: true},
	{id: "sonora-light", label: "Sonora Claro"},
	{id: "dracula", label: "Dracula", dark: true, pal: &palette{dark: true,
		bg: "#282a36", text: "#f8f8f2", faint: "#6272a4", accent: "#bd93f9",
		red: "#ff5555", orange: "#ffb86c", yellow: "#f1fa8c", green: "#50fa7b", cyan: "#8be9fd", blue: "#8be9fd", purp: "#bd93f9"}},
	{id: "catppuccin-mocha", label: "Catppuccin Mocha", dark: true, pal: &palette{dark: true,
		bg: "#1e1e2e", text: "#cdd6f4", faint: "#6c7086", accent: "#cba6f7",
		red: "#f38ba8", orange: "#fab387", yellow: "#f9e2af", green: "#a6e3a1", cyan: "#94e2d5", blue: "#89b4fa", purp: "#cba6f7"}},
	{id: "catppuccin-macchiato", label: "Catppuccin Macchiato", dark: true, pal: &palette{dark: true,
		bg: "#24273a", text: "#cad3f5", faint: "#6e738d", accent: "#c6a0f6",
		red: "#ed8796", orange: "#f5a97f", yellow: "#eed49f", green: "#a6da95", cyan: "#8bd5ca", blue: "#8aadf4", purp: "#c6a0f6"}},
	{id: "catppuccin-frappe", label: "Catppuccin Frappé", dark: true, pal: &palette{dark: true,
		bg: "#303446", text: "#c6d0f5", faint: "#737994", accent: "#ca9ee6",
		red: "#e78284", orange: "#ef9f76", yellow: "#e5c890", green: "#a6d189", cyan: "#81c8be", blue: "#8caaee", purp: "#ca9ee6"}},
	{id: "catppuccin-latte", label: "Catppuccin Latte", pal: &palette{
		bg: "#eff1f5", text: "#4c4f69", faint: "#9ca0b0", accent: "#8839ef",
		red: "#d20f39", orange: "#fe640b", yellow: "#df8e1d", green: "#40a02b", cyan: "#179299", blue: "#1e66f5", purp: "#8839ef"}},
	{id: "tokyonight", label: "Tokyo Night", dark: true, pal: &palette{dark: true,
		bg: "#1a1b26", text: "#c0caf5", faint: "#565f89", accent: "#7aa2f7",
		red: "#f7768e", orange: "#ff9e64", yellow: "#e0af68", green: "#9ece6a", cyan: "#7dcfff", blue: "#7aa2f7", purp: "#bb9af7"}},
	{id: "tokyonight-storm", label: "Tokyo Night Storm", dark: true, pal: &palette{dark: true,
		bg: "#24283b", text: "#c0caf5", faint: "#565f89", accent: "#7aa2f7",
		red: "#f7768e", orange: "#ff9e64", yellow: "#e0af68", green: "#9ece6a", cyan: "#7dcfff", blue: "#7aa2f7", purp: "#bb9af7"}},
	{id: "tokyonight-day", label: "Tokyo Night Day", pal: &palette{
		bg: "#e1e2e7", text: "#3760bf", faint: "#848cb5", accent: "#2e7de9",
		red: "#f52a65", orange: "#b15c00", yellow: "#8c6c3e", green: "#587539", cyan: "#007197", blue: "#2e7de9", purp: "#9854f1"}},
	{id: "rosepine", label: "Rosé Pine", dark: true, pal: &palette{dark: true,
		bg: "#191724", text: "#e0def4", faint: "#6e6a86", accent: "#ebbcba",
		red: "#eb6f92", orange: "#ebbcba", yellow: "#f6c177", green: "#9ccfd8", cyan: "#9ccfd8", blue: "#3e8fb0", purp: "#c4a7e7"}},
	{id: "rosepine-moon", label: "Rosé Pine Moon", dark: true, pal: &palette{dark: true,
		bg: "#232136", text: "#e0def4", faint: "#6e6a86", accent: "#ea9a97",
		red: "#eb6f92", orange: "#ea9a97", yellow: "#f6c177", green: "#9ccfd8", cyan: "#9ccfd8", blue: "#3e8fb0", purp: "#c4a7e7"}},
	{id: "rosepine-dawn", label: "Rosé Pine Dawn", pal: &palette{
		bg: "#faf4ed", text: "#575279", faint: "#9893a5", accent: "#b4637a",
		red: "#b4637a", orange: "#d7827e", yellow: "#ea9d34", green: "#56949f", cyan: "#56949f", blue: "#286983", purp: "#907aa9"}},
	{id: "gruvbox-dark", label: "Gruvbox Oscuro", dark: true, pal: &palette{dark: true,
		bg: "#282828", text: "#ebdbb2", faint: "#928374", accent: "#fabd2f",
		red: "#fb4934", orange: "#fe8019", yellow: "#fabd2f", green: "#b8bb26", cyan: "#8ec07c", blue: "#83a598", purp: "#d3869b"}},
	{id: "gruvbox-light", label: "Gruvbox Claro", pal: &palette{
		bg: "#fbf1c7", text: "#3c3836", faint: "#928374", accent: "#b57614",
		red: "#9d0006", orange: "#af3a03", yellow: "#b57614", green: "#79740e", cyan: "#427b58", blue: "#076678", purp: "#8f3f71"}},
	{id: "nord", label: "Nord", dark: true, pal: &palette{dark: true,
		bg: "#2e3440", text: "#d8dee9", faint: "#616e88", accent: "#88c0d0",
		red: "#bf616a", orange: "#d08770", yellow: "#ebcb8b", green: "#a3be8c", cyan: "#8fbcbb", blue: "#81a1c1", purp: "#b48ead"}},
	{id: "solarized-dark", label: "Solarized Oscuro", dark: true, pal: &palette{dark: true,
		bg: "#002b36", text: "#93a1a1", faint: "#586e75", accent: "#268bd2",
		red: "#dc322f", orange: "#cb4b16", yellow: "#b58900", green: "#859900", cyan: "#2aa198", blue: "#268bd2", purp: "#6c71c4"}},
	{id: "solarized-light", label: "Solarized Claro", pal: &palette{
		bg: "#fdf6e3", text: "#586e75", faint: "#93a1a1", accent: "#268bd2",
		red: "#dc322f", orange: "#cb4b16", yellow: "#b58900", green: "#859900", cyan: "#2aa198", blue: "#268bd2", purp: "#6c71c4"}},
	{id: "onedark", label: "One Dark", dark: true, pal: &palette{dark: true,
		bg: "#282c34", text: "#abb2bf", faint: "#5c6370", accent: "#61afef",
		red: "#e06c75", orange: "#d19a66", yellow: "#e5c07b", green: "#98c379", cyan: "#56b6c2", blue: "#61afef", purp: "#c678dd"}},
	{id: "kanagawa", label: "Kanagawa", dark: true, pal: &palette{dark: true,
		bg: "#1f1f28", text: "#dcd7ba", faint: "#727169", accent: "#7e9cd8",
		red: "#e46876", orange: "#ffa066", yellow: "#e6c384", green: "#98bb6c", cyan: "#7aa89f", blue: "#7e9cd8", purp: "#957fb8"}},
}

func themeDefByID(id string) themeDef {
	for _, d := range themeDefs {
		if d.id == id {
			return d
		}
	}
	return themeDefs[0]
}

// buildTheme makes the theme for an id. Auto follows the terminal's background
// (termDark) and leaves its colors alone.
func buildTheme(id string, termDark bool) theme {
	d := themeDefByID(id)
	switch {
	case d.pal != nil:
		return d.pal.theme()
	case d.id == "sonora-dark" || d.id == "sonora-light":
		th := sonoraTheme(d.dark)
		th.hues = sonoraHues
		if d.dark {
			th.bg = lipgloss.Color("#121318")
		} else {
			th.bg = lipgloss.Color("#FFFFFF")
		}
		return th
	}
	return newTheme(termDark)
}

// ---- deriving a theme from a palette --------------------------------------------------------

func hexRGBA(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	n, _ := strconv.ParseUint(s, 16, 32)
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}

func rgbHex(c color.RGBA) color.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// luminance is the WCAG relative luminance of a color.
func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrast is the WCAG contrast ratio of two colors, from 1 to 21.
func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func (p *palette) theme() theme {
	bg, text, faint, accent := hexRGBA(p.bg), hexRGBA(p.text), hexRGBA(p.faint), hexRGBA(p.accent)
	mix := func(a, b color.RGBA, t float64) color.Color { return rgbHex(core.MixColor(a, b, t)) }
	panelMix := 0.035
	if p.dark {
		panelMix = 0.06
	}
	// The text on an accent-colored background is whichever of the background
	// and the text color reads better on it.
	onAccent := bg
	if contrast(text, accent) > contrast(bg, accent) {
		onAccent = text
	}
	cyan := hexRGBA(p.cyan)
	return theme{
		dark:       p.dark,
		bg:         rgbHex(bg),
		text:       rgbHex(text),
		muted:      mix(text, faint, 0.45),
		faint:      rgbHex(faint),
		border:     mix(bg, text, 0.13),
		accent:     rgbHex(accent),
		accentText: rgbHex(onAccent),
		selected:   mix(bg, accent, 0.16),
		panel:      mix(bg, text, panelMix),
		chip:       mix(bg, text, 0.11),
		good:       lipgloss.Color(p.green),
		bad:        lipgloss.Color(p.red),
		warn:       lipgloss.Color(p.yellow),
		badgeFg:    lipgloss.Color(p.cyan),
		badgeBg:    mix(bg, cyan, 0.16),
		nowFg:      rgbHex(accent),
		nowBg:      mix(bg, accent, 0.24),
		hues: map[string]color.Color{
			"purple": lipgloss.Color(p.purp),
			"yellow": lipgloss.Color(p.yellow),
			"red":    lipgloss.Color(p.red),
			"orange": lipgloss.Color(p.orange),
			"green":  lipgloss.Color(p.green),
			"blue":   lipgloss.Color(p.blue),
			"gray":   rgbHex(faint),
		},
	}
}

// swatch is a few dots in the colors of a theme, to tell them apart in the picker.
func (d themeDef) swatch(t theme) []color.Color {
	return []color.Color{t.accent, t.good, t.bad, t.warn, t.badgeFg}
}
