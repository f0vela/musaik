package main

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
)

// lyricsState is the lyrics view: it takes the place of the main column while
// open. With synced lyrics it keeps the line being sung in the middle (follow);
// scrolling by hand lets go of it until `c` (or a new song) takes it back.
type lyricsState struct {
	open   bool
	follow bool
	top    int // first display row shown
	maxTop int // as far as top goes, as of the last frame
	songID string
}

func (t *tui) openLyrics() {
	t.lyr = lyricsState{open: true, follow: true}
	t.focusSide = false
	if song, ok := t.nowSong(); ok {
		t.eng.LyricsFor(song) // start looking now, not at the first frame
	} else {
		t.eng.Status = "No hay canción para mostrar la letra"
	}
}

func (t *tui) closeLyrics() { t.lyr.open = false }

func (t *tui) toggleLyrics() {
	if t.lyr.open {
		t.closeLyrics()
		return
	}
	t.openLyrics()
}

// lyricsKey handles the keys of the open lyrics view; it reports whether it
// used the key, and the rest (play, next, volume…) work as everywhere.
func (t *tui) lyricsKey(s string) bool {
	half := max(t.listHeight()/2, 1)
	switch s {
	case "L", "esc":
		t.closeLyrics()
	case "/":
		t.closeLyrics() // search is a list's: show the list
		return false
	case "up", "k":
		t.scrollLyrics(-1)
	case "down", "j":
		t.scrollLyrics(1)
	case "pgup", "ctrl+u":
		t.scrollLyrics(-half)
	case "pgdown", "ctrl+d":
		t.scrollLyrics(half)
	case "home", "g":
		t.lyr.follow, t.lyr.top = false, 0
	case "end", "G":
		t.lyr.follow, t.lyr.top = false, t.lyr.maxTop
	case "c":
		t.lyr.follow = true
	case "R":
		if song, ok := t.nowSong(); ok {
			t.eng.ForgetLyrics(song.ID)
		}
	case "S":
		if song, ok := t.nowSong(); ok {
			t.eng.SaveLyricsToTag(song)
		}
	default:
		return false
	}
	return true
}

func (t *tui) scrollLyrics(d int) {
	t.lyr.follow = false
	t.lyr.top = min(max(t.lyr.top+d, 0), t.lyr.maxTop)
}

// lyricsHeader is the first two lines of the view: what it is, whose song,
// the keys, and a rule. hints are the keys by priority: as many as fit are
// shown, and the title gives way to them down to a minimum.
func (t *tui) lyricsHeader(w int, chip, subtitle string, hints ...string) []string {
	th := t.th
	left := " " + th.accented().Bold(true).Render("♬ Letra")
	if chip != "" {
		left += th.subtle().Render("  │  ") + th.fg(th.warn).Render(chip)
	}
	if subtitle != "" {
		left += th.subtle().Render("  │  ") + th.dim().Render(subtitle)
	}
	line := fit(left, w)
	for n := len(hints); n > 0; n-- {
		right := th.subtle().Render(strings.Join(hints[:n], " · ") + " ")
		if rw := lipgloss.Width(right); rw+24 <= w {
			line = fit(left, w-rw) + right
			break
		}
	}
	return []string{line, th.fg(th.border).Render(strings.Repeat("─", w))}
}

// viewLyrics draws the lyrics view as h lines of w cells; x0 and y0 are where
// it sits on the screen, for the clickable lines.
func (t *tui) viewLyrics(w, h, x0, y0 int) []string {
	th := t.th
	song, ok := t.nowSong()
	if t.lyr.songID != song.ID {
		t.lyr.songID, t.lyr.follow, t.lyr.top = song.ID, true, 0
	}
	subtitle := ""
	if ok {
		subtitle = song.Title
		if song.Artist != "" {
			subtitle += " — " + song.Artist
		}
	}
	message := func(hints []string, msg string, detail ...string) []string {
		out := t.lyricsHeader(w, "", subtitle, hints...)
		out = append(out, "", center(th.strong().Render(msg), w))
		for _, d := range detail {
			out = append(out, center(th.dim().Render(d), w))
		}
		return out
	}
	closeHint := []string{"L cerrar"}
	if !ok {
		return message(closeHint, "No hay canción")
	}
	ent := t.eng.LyricsFor(song)
	switch ent.State {
	case core.LyricsLoading:
		return message(closeHint, "Buscando letra…", "LRCLIB · lrclib.net")
	case core.LyricsFailed:
		return message([]string{"L cerrar", "R reintentar"}, "No se pudo buscar la letra", ent.Err, "Se reintenta solo en unos segundos.")
	case core.LyricsNone:
		return message([]string{"L cerrar", "R buscar de nuevo"}, "No hay letra para esta canción",
			"Pon un archivo .lrc con el nombre del audio al lado, o incrusta la letra en sus etiquetas.")
	}
	l := ent.Lyrics
	if l.Instrumental {
		return message(closeHint, "♪  Instrumental  ♪")
	}

	bodyH := max(h-2, 1)
	textW := max(w-6, 10)
	lines := l.Lines()
	cur := l.Current(t.eng.Player.State().Position)

	// Wrap every line into display rows, remembering where each one starts.
	type span struct{ start, n int }
	var rows []string
	var line []int // which lyric line each display row belongs to
	spans := make([]span, len(lines))
	for i, text := range lines {
		wrapped := []string{""}
		if strings.TrimSpace(text) != "" {
			wrapped = strings.Split(ansi.Wrap(text, textW, ""), "\n")
		} else if l.IsSynced() {
			wrapped = []string{"♪"}
		}
		spans[i] = span{len(rows), len(wrapped)}
		for _, r := range wrapped {
			rows = append(rows, r)
			line = append(line, i)
		}
	}
	t.lyr.maxTop = max(len(rows)-bodyH, 0)
	if l.IsSynced() && t.lyr.follow && cur >= 0 {
		mid := spans[cur].start + spans[cur].n/2
		t.lyr.top = mid - bodyH/2
	} else if t.lyr.follow {
		t.lyr.top = 0
	}
	t.lyr.top = min(max(t.lyr.top, 0), t.lyr.maxTop)

	hints := []string{"L cerrar"}
	var chips []string
	if !l.IsSynced() {
		chips = append(chips, "sin sincronizar")
	}
	switch {
	case ent.Saving:
		chips = append(chips, "guardando en el archivo…")
	case l.CanSave():
		hints = append(hints, "S guardar en el archivo")
	case l.Source == "tag":
		chips = append(chips, "guardada en el archivo")
	case l.Source == "lrc":
		chips = append(chips, "del archivo .lrc")
	}
	hints = append(hints, "↑↓ mover")
	if l.IsSynced() {
		hints = append(hints, "c seguir", "clic: ir")
	}
	chip := strings.Join(chips, " · ")
	out := t.lyricsHeader(w, chip, subtitle, hints...)
	for r := t.lyr.top; r < min(t.lyr.top+bodyH, len(rows)); r++ {
		i := line[r]
		style := th.dim()
		switch {
		case !l.IsSynced():
			style = th.normal()
		case i == cur:
			style = th.accented().Bold(true)
		case i < cur:
			style = th.subtle()
		}
		out = append(out, center(style.Render(rows[r]), w))
		if l.IsSynced() {
			t.zone(x0, y0+len(out)-1, w, 1, "lyric", i)
		}
	}
	return out
}

// seekToLyric jumps to the start of synced line i.
func (t *tui) seekToLyric(i int) {
	song, ok := t.nowSong()
	if !ok {
		return
	}
	ent := t.eng.LyricsFor(song)
	if ent.State != core.LyricsReady || !ent.Lyrics.IsSynced() || i < 0 || i >= len(ent.Lyrics.Synced) {
		return
	}
	if st := t.eng.Player.State(); st.Playing {
		t.eng.SeekTo(min(ent.Lyrics.Synced[i].At, max(st.Duration-time.Second, 0)))
		t.lyr.follow = true
	}
}
