package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
	"musaik/internal/model"
)

// nowSong is the song the player bar describes: the one that plays, or else
// the one selected, as the desktop app's bar does.
func (t *tui) nowSong() (model.Song, bool) {
	if s, ok := t.eng.CurrentSong(); ok {
		return s, true
	}
	if _, s, ok := t.eng.FindSong(t.eng.SelectedID); ok {
		return s, true
	}
	return model.Song{}, false
}

type playback struct {
	pos, dur    float64
	playing     bool
	paused      bool
	hasDuration bool
	frac        float64
	posS, durS  string
}

func (t *tui) playback() playback {
	st := t.eng.Player.State()
	p := playback{playing: st.Playing && !st.Paused, paused: st.Paused}
	p.pos, p.dur = st.Position.Seconds(), st.Duration.Seconds()
	p.hasDuration = st.Duration > 0
	if p.hasDuration {
		p.frac = min(max(p.pos/p.dur, 0), 1)
	}
	p.posS = core.FormatDuration(st.Position)
	if st.Position <= 0 {
		p.posS = "00:00"
	}
	p.durS = core.FormatDuration(st.Duration)
	return p
}

// progressBar draws a seek bar of w cells, filled up to frac, with its colors
// running from a to b, and makes it clickable.
func (t *tui) progressBar(w int, frac float64, a, b color.Color, bg color.Color, x, y int) string {
	if w <= 0 {
		return ""
	}
	filled := int(frac * float64(w))
	if frac > 0 && filled == 0 {
		filled = 1
	}
	if filled > w {
		filled = w
	}
	ar, ag, ab, _ := a.RGBA()
	br, bgc, bb, _ := b.RGBA()
	ca := color.RGBA{R: uint8(ar >> 8), G: uint8(ag >> 8), B: uint8(ab >> 8), A: 255}
	cb := color.RGBA{R: uint8(br >> 8), G: uint8(bgc >> 8), B: uint8(bb >> 8), A: 255}
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}
	var sb strings.Builder
	for i := 0; i < w; i++ {
		switch {
		case i < filled-1 || (i == filled-1 && filled == w):
			c := core.MixColor(ca, cb, float64(i)/float64(max(w-1, 1)))
			sb.WriteString(base.Foreground(c).Render("━"))
		case i == filled-1:
			sb.WriteString(base.Foreground(t.th.text).Render("●"))
		default:
			sb.WriteString(base.Foreground(t.th.faint).Render("─"))
		}
	}
	t.zone(x, y, w, 1, "seek", w)
	return sb.String()
}

// transport draws shuffle, previous, play/pause, next and repeat, and makes
// each button clickable. It returns the text and its width.
func (t *tui) transport(paint func(color.Color) lipgloss.Style, accent color.Color, playBg color.Color, x, y int) (string, int) {
	th := t.th
	e := t.eng
	pb := t.playback()
	type btn struct {
		kind, text string
		style      lipgloss.Style
	}
	shuffle := btn{"shuffle", "⇄", paint(th.muted)}
	if e.Shuffle {
		shuffle.style = paint(accent).Bold(true)
	}
	play := "▶"
	if pb.playing {
		play = "❚❚"
	}
	repeat := btn{"repeat", "↻", paint(th.muted)}
	switch e.Repeat {
	case core.RepeatAll:
		repeat.style = paint(accent).Bold(true)
	case core.RepeatOne:
		repeat = btn{"repeat", "↻1", paint(accent).Bold(true)}
	}
	btns := []btn{
		shuffle,
		{"prev", "◀◀", paint(th.text)},
		{"play", play, lipgloss.NewStyle().Foreground(th.accentText).Background(playBg).Bold(true)},
		{"next", "▶▶", paint(th.text)},
		repeat,
	}
	var sb strings.Builder
	cx := x
	for i, b := range btns {
		txt := " " + b.text + " "
		if b.kind == "play" {
			txt = " " + b.text + " "
		}
		w := lipgloss.Width(txt)
		sb.WriteString(b.style.Render(txt))
		t.zone(cx, y, w, 1, b.kind, 0)
		cx += w
		if i < len(btns)-1 {
			sb.WriteString(paint(th.muted).Render(" "))
			cx++
		}
	}
	return sb.String(), cx - x
}

// volume draws a small volume bar with its percentage and makes it clickable.
func (t *tui) volume(paint func(color.Color) lipgloss.Style, accent color.Color, barW int, x, y int) (string, int) {
	th := t.th
	e := t.eng
	label := fmt.Sprintf("%3d%%", e.Volume)
	icon := "vol"
	if e.Volume <= 0 {
		icon = "mut"
	}
	filled := e.Volume * barW / 100
	var sb strings.Builder
	sb.WriteString(paint(th.muted).Render(icon + " "))
	t.zone(x, y, 3, 1, "mute", 0)
	for i := 0; i < barW; i++ {
		if i < filled {
			sb.WriteString(paint(accent).Render("━"))
		} else {
			sb.WriteString(paint(th.faint).Render("─"))
		}
	}
	t.zone(x+4, y, barW, 1, "vol", barW)
	sb.WriteString(paint(th.muted).Render(" " + label))
	return sb.String(), 4 + barW + 1 + len(label)
}

// trunc cuts s to w cells without padding it.
func trunc(s string, w int) string {
	if ansi.StringWidth(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s
}

// titleLine is the title followed by the like heart, and artistLine the
// artist and album; both are as wide as their text, up to w cells.
func (t *tui) titleLine(song model.Song, has bool, w int, paint func(color.Color) lipgloss.Style) string {
	th := t.th
	if !has {
		return paint(th.text).Bold(true).Render("Sin reproducción")
	}
	heart := paint(th.muted).Render("♡")
	if t.eng.Store.Favorites[song.ID] {
		heart = paint(th.bad).Render("♥")
	}
	return paint(th.text).Bold(true).Render(trunc(song.Title, max(w-2, 1))) + paint(th.muted).Render(" ") + heart
}

func (t *tui) artistLine(song model.Song, has bool, w int, paint func(color.Color) lipgloss.Style) string {
	if !has {
		return paint(t.th.muted).Render("Musaik")
	}
	artist, album := strings.TrimSpace(song.Artist), strings.TrimSpace(song.Album)
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}
	return paint(t.th.muted).Render(trunc(artist+" — "+album, w))
}

func (t *tui) qualityLine(song model.Song, has bool, paint func(color.Color) lipgloss.Style) string {
	if !has {
		return ""
	}
	return paint(t.th.faint).Render(strings.ToUpper(song.Format) + "  /  Lossless")
}

// spread puts left and right on one line of w cells, right against the edge.
func spread(left, right string, w int) string {
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fit(left, max(w-ansi.StringWidth(right)-1, 1)) + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// viewPlayer draws the player bar: l.playerH lines. The song is on the left,
// the controls on the right and the progress bar underneath; the narrower it
// is the less it shows, but never the controls.
func (t *tui) viewPlayer(l layout) []string {
	th := t.th
	w := t.w
	top := t.h - l.stat - l.playerH
	song, has := t.nowSong()
	pb := t.playback()
	plain := t.painter(false)
	out := []string{th.fg(th.border).Render(strings.Repeat("─", w))}

	coverW := 0
	if l.playerH == 5 && w >= 70 {
		coverW = 8
	}
	rx := 1
	var art []string
	if coverW > 0 {
		if has {
			art = t.cover(song, coverW, 4)
		} else {
			art = placeholderArt(coverW, 4, th.faint, th.chip, "♪")
		}
		rx = 1 + coverW + 2
	}
	rw := w - rx - 1
	rows := l.playerH - 1
	y := func(row int) int { return top + 1 + row }
	body := make([]string, rows)

	progress := func(row int) string {
		barW := max(rw-ansi.StringWidth(pb.posS)-ansi.StringWidth(pb.durS)-2, 4)
		return plain(th.muted).Render(pb.posS+" ") + t.progressBar(barW, pb.frac, th.accent, th.accent, nil, rx+ansi.StringWidth(pb.posS)+1, y(row)) + plain(th.muted).Render(" "+pb.durS)
	}
	controls := func(row, x int, withVolume bool) (string, int) {
		tr, trW := t.transport(plain, th.accent, th.accent, x, y(row))
		if !withVolume {
			return tr, trW
		}
		vol, volW := t.volume(plain, th.accent, 8, x+trW+3, y(row))
		return tr + "   " + vol, trW + 3 + volW
	}

	switch {
	case rows >= 4 && rw >= 84:
		// song | transport, volume
		trW := 5*4 + 4 + 4
		volW := 4 + 10 + 1 + 4
		infoW := rw - trW - 4
		tr, _ := t.transport(plain, th.accent, th.accent, rx+rw-trW, y(0))
		vol, _ := t.volume(plain, th.accent, 10, rx+rw-volW, y(1))
		body[0] = spread(fit(t.titleLine(song, has, infoW, plain), infoW), tr, rw)
		body[1] = spread(fit(t.artistLine(song, has, infoW, plain), infoW), vol, rw)
		body[2] = t.qualityLine(song, has, plain)
		body[3] = progress(3)
		t.zone(rx, y(0), infoW, 1, "like", 0)
	case rows >= 4:
		ctl, _ := controls(2, rx, rw >= 62)
		body[0] = t.titleLine(song, has, rw, plain)
		body[1] = t.artistLine(song, has, rw, plain)
		body[2] = ctl
		body[3] = progress(3)
	case rows == 3:
		ctl, _ := controls(1, rx, rw >= 62)
		body[0] = t.titleLine(song, has, rw*2/3, plain) + plain(th.muted).Render("  ") + t.artistLine(song, has, max(rw/3-2, 1), plain)
		body[1] = ctl
		body[2] = progress(2)
	}
	for i := 0; i < rows; i++ {
		if coverW > 0 && i < len(art) {
			out = append(out, " "+art[i]+"  "+fit(body[i], rw)+" ")
		} else {
			out = append(out, " "+fit(body[i], rw)+" ")
		}
	}
	return out
}

func rgbaOf(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

// miniBackdrop is the artwork's main color, darkened as the desktop mini
// player's background is.
func (t *tui) miniBackdrop() color.Color {
	p := t.palette()
	return core.MixColor(p[0], color.RGBA{A: 255}, 0.74)
}

func (t *tui) miniKey(s string) {
	e := t.eng
	switch s {
	case "q":
		t.quit()
	case "M", "esc":
		t.mini = false
	case "space":
		e.TogglePlayPause()
	case "n":
		e.Next()
	case "p":
		e.Previous()
	case "right":
		e.SeekBy(seekStep)
	case "left":
		e.SeekBy(-seekStep)
	case "shift+right":
		e.SeekBy(seekStepLarge)
	case "shift+left":
		e.SeekBy(-seekStepLarge)
	case "+", "=":
		e.SetVolume(e.Volume + volumeStep)
	case "-", "_":
		e.SetVolume(e.Volume - volumeStep)
	case "m":
		e.ToggleMute()
	case "s":
		e.ToggleShuffle()
	case "r":
		e.CycleRepeat()
	case "l":
		if song, ok := e.CurrentSong(); ok {
			e.ToggleFavorite(song)
		}
	case "?":
		t.help = true
	}
}

// viewMini draws the compact now-playing card over a backdrop tinted by the
// cover, the terminal's version of the desktop mini player. A wide, short
// terminal puts the cover beside the text; any other puts it above.
func (t *tui) viewMini() string {
	th := t.th
	w, h := t.w, t.h
	pal := t.palette()
	bd := t.miniBackdrop()
	paint := func(fg color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(fg).Background(bd) }
	bright := func(c color.RGBA) color.RGBA {
		return core.MixColor(c, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.35)
	}
	a1, a2 := bright(pal[1%len(pal)]), bright(pal[2%len(pal)])
	accent := pal[1%len(pal)]

	song, has := t.nowSong()
	pb := t.playback()
	name, artist := "Sin reproducción", "Musaik"
	if has {
		name, artist = song.Title, song.Artist
	}
	heart := "♡"
	if has && t.eng.Store.Favorites[song.ID] {
		heart = "♥"
	}

	wide := w >= 64 && h < 22
	var coverCols, coverRows, cardW, cardH, textW int
	if wide {
		coverRows = min(h-4, 12)
		coverCols = coverRows * 2
		textW = min(w-coverCols-8, 44)
		cardW, cardH = coverCols+3+textW, coverRows
	} else {
		coverCols = max(min(w-8, (h-11)*2, 44), 8)
		coverRows = coverCols / 2
		textW = max(coverCols, min(w-4, 36))
		cardW, cardH = textW, coverRows+7
	}
	cx, cy := max((w-cardW)/2, 0), max((h-cardH)/2, 0)

	var art []string
	if has {
		art = t.cover(song, coverCols, coverRows)
	} else {
		art = placeholderArt(coverCols, coverRows, th.faint, th.chip, "♪")
	}

	ctlW := 5*4 + 4
	progress := func(x, y, width int) string {
		barW := max(width-ansi.StringWidth(pb.posS)-ansi.StringWidth(pb.durS)-2, 4)
		return paint(th.muted).Render(pb.posS+" ") + t.progressBar(barW, pb.frac, a1, a2, bd, x+ansi.StringWidth(pb.posS)+1, y) + paint(th.muted).Render(" "+pb.durS)
	}
	text := func(s string, align string, width int) string {
		if align == "center" {
			return center(s, width)
		}
		return fit(s, width)
	}

	var card []string
	if wide {
		tx := cx + coverCols + 3
		top := (coverRows - 6) / 2
		info := make([]string, coverRows)
		set := func(i int, s string) {
			if i >= 0 && i < len(info) {
				info[i] = s
			}
		}
		set(top, paint(th.text).Bold(true).Render(text(name, "", textW)))
		set(top+1, paint(th.muted).Render(text(artist+"  "+heart, "", textW)))
		set(top+3, progress(tx, cy+top+3, textW))
		tr, _ := t.transport(paint, bright(accent), accent, tx, cy+top+5)
		set(top+5, tr)
		for i := 0; i < coverRows; i++ {
			card = append(card, art[i]+paint(th.text).Render("   ")+padBg(info[i], textW, bd))
		}
	} else {
		pad := func(s string, width int) string {
			left := max((cardW-width)/2, 0)
			return padBg(paint(th.text).Render(strings.Repeat(" ", left))+s, cardW, bd)
		}
		for i := 0; i < coverRows; i++ {
			card = append(card, pad(art[i], coverCols))
		}
		card = append(card, padBg("", cardW, bd))
		card = append(card, padBg(paint(th.text).Bold(true).Render(text(name, "center", cardW)), cardW, bd))
		card = append(card, padBg(paint(th.muted).Render(text(artist+"  "+heart, "center", cardW)), cardW, bd))
		card = append(card, padBg("", cardW, bd))
		card = append(card, padBg(progress(cx, cy+coverRows+4, cardW), cardW, bd))
		card = append(card, padBg("", cardW, bd))
		tr, _ := t.transport(paint, bright(accent), accent, cx+(cardW-ctlW)/2, cy+coverRows+6)
		card = append(card, pad(tr, ctlW))
	}

	blank := paint(th.text).Render(strings.Repeat(" ", w))
	lines := make([]string, h)
	for i := range lines {
		lines[i] = blank
	}
	for i, line := range card {
		if y := cy + i; y >= 0 && y < h {
			lines[y] = padBg(paint(th.text).Render(strings.Repeat(" ", cx))+line, w, bd)
		}
	}
	hint := "M volver · espacio · n/p · ←/→ · +/- · l ♥"
	lines[h-1] = padBg(paint(th.faint).Render(center(hint, w)), w, bd)
	return strings.Join(lines, "\n")
}

func padBg(s string, w int, bd color.Color) string {
	n := w - lipgloss.Width(s)
	if n <= 0 {
		return s
	}
	return s + lipgloss.NewStyle().Background(bd).Render(strings.Repeat(" ", n))
}
