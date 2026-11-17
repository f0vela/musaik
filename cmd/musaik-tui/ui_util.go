package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	xdraw "golang.org/x/image/draw"
)

// fit cuts s to w cells (ending in an ellipsis) or pads it with spaces, so that
// it is exactly w cells wide whatever styling it carries.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw > w {
		s = ansi.Truncate(s, w, "…")
		sw = ansi.StringWidth(s)
	}
	if sw < w {
		s += strings.Repeat(" ", w-sw)
	}
	return s
}

// fitRight is fit with the text pushed to the right edge.
func fitRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw > w {
		return fit(s, w)
	}
	return strings.Repeat(" ", w-sw) + s
}

// center pads s on both sides to w cells.
func center(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw >= w {
		return fit(s, w)
	}
	left := (w - sw) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-sw-left)
}

// block makes lines exactly w x h: each line is fitted, missing ones are blank.
func block(lines []string, w, h int) string {
	out := make([]string, h)
	blank := strings.Repeat(" ", max(w, 0))
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = blank
		}
	}
	return strings.Join(out, "\n")
}

// overlay draws box over base with its top-left corner at (x, y). Both are
// blocks of equal-width lines.
func overlay(base, box string, x, y int) string {
	bl := strings.Split(base, "\n")
	ol := strings.Split(box, "\n")
	for i, line := range ol {
		r := y + i
		if r < 0 || r >= len(bl) {
			continue
		}
		w := ansi.StringWidth(line)
		left := ansi.Truncate(bl[r], x, "")
		left += strings.Repeat(" ", max(x-ansi.StringWidth(left), 0))
		bl[r] = left + line + "\x1b[0m" + ansi.TruncateLeft(bl[r], x+w, "")
	}
	return strings.Join(bl, "\n")
}

// halfBlocks draws img in cols x rows cells, two pixels per cell: the upper
// half block takes the top pixel as its color and the bottom one as its
// background.
func halfBlocks(img image.Image, cols, rows int) []string {
	if img == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, cols, rows*2))
	// Crop to the cell grid's aspect ratio, as the desktop app's Cover fit does.
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, cropRect(img.Bounds(), cols, rows), xdraw.Src, nil)
	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		var sb strings.Builder
		for x := 0; x < cols; x++ {
			top := dst.RGBAAt(x, y*2)
			bot := dst.RGBAAt(x, y*2+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", top.R, top.G, top.B, bot.R, bot.G, bot.B)
		}
		sb.WriteString("\x1b[0m")
		lines[y] = sb.String()
	}
	return lines
}

// placeholderArt is what shows where a song has no cover.
func placeholderArt(cols, rows int, fg, bg color.Color, glyph string) []string {
	lines := make([]string, rows)
	fr, fg2, fb, _ := fg.RGBA()
	br, bg2, bb, _ := bg.RGBA()
	style := fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", fr>>8, fg2>>8, fb>>8, br>>8, bg2>>8, bb>>8)
	for y := range lines {
		row := strings.Repeat(" ", cols)
		if y == rows/2 {
			row = center(glyph, cols)
		}
		lines[y] = style + row + "\x1b[0m"
	}
	return lines
}

// hexOf formats a color as #rrggbb.
func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// ---- text field -------------------------------------------------------------

// field is a single-line text input: enough for the search box, the palette
// and the settings, without the blinking cursor of a full widget.
type field struct {
	value []rune
	cur   int
	mask  bool
}

func (f *field) Value() string { return string(f.value) }

func (f *field) Set(s string) {
	f.value = []rune(s)
	f.cur = len(f.value)
}

func (f *field) Reset() { f.value, f.cur = nil, 0 }

func (f *field) insert(s string) {
	rs := []rune(s)
	out := make([]rune, 0, len(f.value)+len(rs))
	out = append(out, f.value[:f.cur]...)
	out = append(out, rs...)
	out = append(out, f.value[f.cur:]...)
	f.value = out
	f.cur += len(rs)
}

// Handle edits the field for a key and reports whether the text changed. Keys
// it does not use (enter, esc, up, down, tab) are left to the caller.
func (f *field) Handle(k tea.KeyPressMsg) (changed bool) {
	switch s := k.String(); s {
	case "left":
		f.cur = max(f.cur-1, 0)
	case "right":
		f.cur = min(f.cur+1, len(f.value))
	case "home", "ctrl+a":
		f.cur = 0
	case "end", "ctrl+e":
		f.cur = len(f.value)
	case "backspace", "ctrl+h":
		if f.cur > 0 {
			f.value = append(f.value[:f.cur-1], f.value[f.cur:]...)
			f.cur--
			return true
		}
	case "delete", "ctrl+d":
		if f.cur < len(f.value) {
			f.value = append(f.value[:f.cur], f.value[f.cur+1:]...)
			return true
		}
	case "ctrl+u":
		if f.cur > 0 {
			f.value = f.value[f.cur:]
			f.cur = 0
			return true
		}
	case "ctrl+k":
		if f.cur < len(f.value) {
			f.value = f.value[:f.cur]
			return true
		}
	case "ctrl+w", "alt+backspace":
		i := f.cur
		for i > 0 && unicode.IsSpace(f.value[i-1]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(f.value[i-1]) {
			i--
		}
		if i < f.cur {
			f.value = append(f.value[:i], f.value[f.cur:]...)
			f.cur = i
			return true
		}
	default:
		if k.Text != "" && !strings.ContainsAny(k.Text, "\x1b\r\n\t") && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			f.insert(k.Text)
			return true
		}
	}
	return false
}

// Paste inserts pasted text, flattened to one line.
func (f *field) Paste(s string) {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	f.insert(s)
}

// View draws the field in w cells; with focus, the character under the cursor
// is shown reversed.
func (f *field) View(w int, focused bool, placeholder string, normal, dim, cursor func(...string) string) string {
	if w <= 0 {
		return ""
	}
	if len(f.value) == 0 {
		if focused {
			return fit(cursor(" ")+dim(placeholder), w)
		}
		return fit(dim(placeholder), w)
	}
	shown := f.value
	if f.mask {
		shown = []rune(strings.Repeat("•", len(f.value)))
	}
	// Scroll so the cursor stays inside the window.
	start := 0
	if f.cur >= w-1 {
		start = f.cur - (w - 2)
	}
	end := min(start+w, len(shown))
	var sb strings.Builder
	for i := start; i < end; i++ {
		ch := string(shown[i])
		if focused && i == f.cur {
			sb.WriteString(cursor(ch))
		} else {
			sb.WriteString(normal(ch))
		}
	}
	if focused && f.cur >= len(shown) && end-start < w {
		sb.WriteString(cursor(" "))
	}
	return fit(sb.String(), w)
}

// reverse draws the character under a text field's cursor.
var reverse = lipgloss.NewStyle().Reverse(true).Render
