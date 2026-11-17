package main

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// theme mirrors the desktop app's palette ("sonora"): the same colors, with the
// translucent ones pre-blended because a terminal has no alpha.
type theme struct {
	dark                           bool
	text, muted, faint             color.Color
	border, accent, accentText     color.Color
	selected, panel, chip          color.Color
	good, bad, warn                color.Color
	badgeFg, badgeBg, nowFg, nowBg color.Color
}

func newTheme(dark bool) theme {
	if dark {
		return theme{
			dark:       true,
			text:       lipgloss.Color("#F1F2F6"),
			muted:      lipgloss.Color("#9C9DA2"),
			faint:      lipgloss.Color("#67686C"),
			border:     lipgloss.Color("#25262A"),
			accent:     lipgloss.Color("#4C8DFF"),
			accentText: lipgloss.Color("#FFFFFF"),
			selected:   lipgloss.Color("#181F2F"),
			panel:      lipgloss.Color("#1C1D24"),
			chip:       lipgloss.Color("#26272E"),
			good:       lipgloss.Color("#10B981"),
			bad:        lipgloss.Color("#F43F5E"),
			warn:       lipgloss.Color("#F2C94C"),
			badgeFg:    lipgloss.Color("#22D3EE"),
			badgeBg:    lipgloss.Color("#16313A"),
			nowFg:      lipgloss.Color("#4C8DFF"),
			nowBg:      lipgloss.Color("#1B2A47"),
		}
	}
	return theme{
		dark:       false,
		text:       lipgloss.Color("#111827"),
		muted:      lipgloss.Color("#6B7079"),
		faint:      lipgloss.Color("#A0A3A9"),
		border:     lipgloss.Color("#EAEAEC"),
		accent:     lipgloss.Color("#2563EB"),
		accentText: lipgloss.Color("#FFFFFF"),
		selected:   lipgloss.Color("#E9EFFD"),
		panel:      lipgloss.Color("#F4F5F7"),
		chip:       lipgloss.Color("#E8E9EC"),
		good:       lipgloss.Color("#059669"),
		bad:        lipgloss.Color("#E11D48"),
		warn:       lipgloss.Color("#B7791F"),
		badgeFg:    lipgloss.Color("#0E8A6B"),
		badgeBg:    lipgloss.Color("#DDF3EC"),
		nowFg:      lipgloss.Color("#2563EB"),
		nowBg:      lipgloss.Color("#DCE6FB"),
	}
}

func (t theme) fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func (t theme) normal() lipgloss.Style   { return t.fg(t.text) }
func (t theme) dim() lipgloss.Style      { return t.fg(t.muted) }
func (t theme) subtle() lipgloss.Style   { return t.fg(t.faint) }
func (t theme) strong() lipgloss.Style   { return t.fg(t.text).Bold(true) }
func (t theme) accented() lipgloss.Style { return t.fg(t.accent) }

// playlistColor is the dot color of a playlist, as in the desktop sidebar.
func (t theme) playlistColor(name string) color.Color {
	switch name {
	case "purple":
		return lipgloss.Color("#8B5CF6")
	case "yellow":
		return lipgloss.Color("#F59E0B")
	case "red":
		return lipgloss.Color("#EF4444")
	case "orange":
		return lipgloss.Color("#F97316")
	case "green":
		return lipgloss.Color("#10B981")
	case "blue":
		return lipgloss.Color("#3B82F6")
	}
	return lipgloss.Color("#9CA3AF")
}
