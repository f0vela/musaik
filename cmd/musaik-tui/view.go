package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"musaik/internal/core"
	"musaik/internal/model"
)

type libStats struct {
	albums int
	size   string
}

func (t *tui) computeStats() libStats {
	return libStats{albums: len(t.eng.Albums("", "")), size: t.eng.TotalLibrarySize(t.eng.Store.Songs)}
}

func (t *tui) View() tea.View {
	t.zones = t.zones[:0]
	if t.stats.size == "" {
		t.stats = t.computeStats()
	}
	var content string
	switch {
	case t.w < 40 || t.h < 10:
		content = block([]string{"", center("Musaik", t.w), center(t.th.dim().Render("Agranda la terminal"), t.w)}, t.w, t.h)
	case t.mini:
		content = t.viewMini()
	default:
		content = t.viewScreen()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "Musaik"
	switch {
	case t.mini:
		v.BackgroundColor = t.miniBackdrop()
	case t.th.bg != nil:
		// A named theme brings its own background; auto leaves the terminal's.
		v.BackgroundColor = t.th.bg
		v.ForegroundColor = t.th.text
	}
	return v
}

func (t *tui) viewScreen() string {
	l := t.layout()
	th := t.th
	bodyH := t.h - l.playerH - l.stat

	main := t.viewMainColumn(l)
	mainLines := strings.Split(block(main, l.mw, bodyH), "\n")

	var rows []string
	if l.side {
		side := strings.Split(block(t.viewSidebar(l.sw, bodyH), l.sw, bodyH), "\n")
		div := th.fg(th.border).Render("│")
		for i := 0; i < bodyH; i++ {
			rows = append(rows, side[i]+div+mainLines[i])
		}
	} else {
		rows = mainLines
	}
	rows = append(rows, t.viewPlayer(l)...)
	rows = append(rows, t.viewStatus())
	screen := strings.Join(rows, "\n")

	switch {
	case t.help:
		screen = t.overlayCentered(screen, t.viewHelp())
	case t.modal.kind != modalNone:
		screen = t.overlayCentered(screen, t.viewModal())
	case t.pal.open:
		screen = t.overlayAt(screen, t.viewPalette(), 3)
	}
	return screen
}

func (t *tui) overlayCentered(base, box string) string {
	bw := lipgloss.Width(strings.Split(box, "\n")[0])
	bh := strings.Count(box, "\n") + 1
	return overlay(base, box, max((t.w-bw)/2, 0), max((t.h-bh)/2, 0))
}

func (t *tui) overlayAt(base, box string, y int) string {
	bw := lipgloss.Width(strings.Split(box, "\n")[0])
	return overlay(base, box, max((t.w-bw)/2, 0), y)
}

// viewMainColumn is the top bar and the view of the current section.
func (t *tui) viewMainColumn(l layout) []string {
	x0 := 0
	if l.side {
		x0 = l.sw + 1
	}
	var body []string
	switch t.sec {
	case secArtists:
		body = t.viewArtists(l.mw, l.contentH, x0, l.topH)
	case secAlbums:
		body = t.viewAlbums(l.mw, l.contentH, x0, l.topH)
	case secQueue:
		body = t.viewQueue(l.mw, l.contentH, x0, l.topH)
	case secSettings:
		body = t.viewSettings(l.mw, l.contentH)
	default:
		body = t.viewSongList(l.mw, l.contentH, x0, l.topH)
	}
	return append(t.viewTopBar(l.mw), body...)
}

// title is the heading of the current section.
func (t *tui) title() string {
	switch t.sec {
	case secSongs:
		return "Todas las canciones"
	case secPlaylist:
		if t.playlist != "" {
			return t.playlist
		}
	case secArtists:
		if t.artistOpen {
			return t.artist
		}
	}
	return sectionNames[t.sec]
}

func (t *tui) viewTopBar(w int) []string {
	th := t.th
	left := " " + th.strong().Render(t.title())
	if t.sec == secSongs && w >= 70 {
		left += th.subtle().Render("  │  ") + th.dim().Render(fmt.Sprintf("%d pistas", len(t.eng.Store.Songs))) +
			th.subtle().Render(" • ") + th.dim().Render(core.FormatHoursMinutes(core.TotalDuration(t.eng.Store.Songs)))
	}
	var right string
	if t.sec != secQueue && t.sec != secSettings {
		bw := min(max(w/3, 18), 34)
		box := t.search.View(bw-4, t.searching, "Buscar… (/)", th.normal().Render, th.subtle().Render, reverse)
		icon := th.dim().Render("⌕ ")
		if t.searching {
			icon = th.accented().Render("⌕ ")
		}
		right = icon + box + " "
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	line := left
	if gap > 0 {
		line += strings.Repeat(" ", gap) + right
	}
	return []string{fit(line, w), th.fg(th.border).Render(strings.Repeat("─", w))}
}

func (t *tui) viewStatus() string {
	th := t.th
	hints := th.subtle().Render("? ayuda · ctrl+p comandos · q salir")
	hw := lipgloss.Width(hints)
	status := t.eng.Status
	if status == "" {
		status = " "
	}
	left := th.dim().Render(fit(" "+status, max(t.w-hw-2, 1)))
	if t.w < 70 {
		return fit(left, t.w)
	}
	return fit(left+"  "+hints, t.w)
}

// ---- song lists -------------------------------------------------------------------------

type column struct {
	id string
	w  int
}

// songColumns chooses the columns that fit in w cells, dropping the secondary
// ones (size, plays, format, album, artist) before the title gets cramped.
func songColumns(w int, detail bool) ([]column, int) {
	base := []column{{"num", 4}, {"dur", 6}, {"like", 2}}
	optional := []column{{"artist", 20}, {"album", 22}, {"fmt", 6}, {"plays", 5}, {"size", 9}}
	if detail {
		base = []column{{"num", 4}, {"dur", 6}, {"like", 2}}
		optional = []column{{"plays", 5}}
	}
	const titleMin = 24
	cols := base
	sum := func(cs []column) int {
		n := 2 // left and right margin
		for _, c := range cs {
			n += c.w + 2
		}
		return n // the title's own gap included
	}
	for _, c := range optional {
		if w-sum(append(cols, c)) < titleMin {
			break
		}
		cols = append(cols, c)
	}
	titleW := w - sum(cols)
	// Present them in reading order.
	order := []string{"num", "title", "artist", "album", "plays", "fmt", "dur", "size", "like"}
	cols = append(cols, column{"title", titleW})
	var out []column
	for _, id := range order {
		for _, c := range cols {
			if c.id == id {
				out = append(out, c)
			}
		}
	}
	return out, titleW
}

var columnHeads = map[string]string{
	"num": "#", "title": "Nombre de Pista", "artist": "Artista", "album": "Álbum",
	"plays": "Repr.", "fmt": "Formato", "dur": "Durac.", "size": "Tamaño", "like": "♡",
}

func (t *tui) songHeader(cols []column, w int) string {
	th := t.th
	var parts []string
	for _, c := range cols {
		txt := columnHeads[c.id]
		switch c.id {
		case "plays", "dur", "size":
			parts = append(parts, th.dim().Render(fitRight(txt, c.w)))
		default:
			parts = append(parts, th.dim().Render(fit(txt, c.w)))
		}
	}
	return fit(" "+strings.Join(parts, "  "), w)
}

// songRow draws one song. num is the number shown for it (0 shows none).
func (t *tui) songRow(s model.Song, cols []column, num int, selected, playing bool, w int) string {
	th := t.th
	e := t.eng
	paint := t.painter(selected)
	gap := paint(th.muted).Render("  ")
	var parts []string
	for _, c := range cols {
		var cell string
		switch c.id {
		case "num":
			switch {
			case playing:
				cell = paint(th.accent).Render(fit("♪", c.w))
			case num > 0:
				cell = paint(th.faint).Render(fit(strconv.Itoa(num), c.w))
			default:
				cell = paint(th.faint).Render(fit("", c.w))
			}
		case "title":
			title := core.CompactTrackTitle(s)
			if playing {
				chip := lipgloss.NewStyle().Foreground(th.nowFg).Background(th.nowBg).Bold(true).Render(" NOW ")
				room := c.w - 6
				if room < 6 {
					cell = paint(th.accent).Bold(true).Render(fit(title, c.w))
				} else {
					cell = paint(th.accent).Bold(true).Render(fit(title, room)) + paint(th.muted).Render(" ") + chip
				}
			} else {
				cell = paint(th.text).Bold(true).Render(fit(title, c.w))
			}
		case "artist":
			cell = paint(th.text).Render(fit(s.Artist, c.w))
		case "album":
			cell = paint(th.muted).Render(fit(s.Album, c.w))
		case "plays":
			cell = paint(th.muted).Render(fitRight(strconv.Itoa(s.PlayCount), c.w))
		case "fmt":
			f := core.CompactFormat(s)
			if core.IsLossless(f) {
				cell = lipgloss.NewStyle().Foreground(th.badgeFg).Background(th.badgeBg).Bold(true).Render(fit(" "+f, c.w))
			} else {
				cell = paint(th.muted).Render(fit(" "+f, c.w))
			}
		case "dur":
			cell = paint(th.muted).Render(fitRight(core.FormatDuration(s.Duration), c.w))
		case "size":
			cell = paint(th.muted).Render(fitRight(core.FormatFileSizeMB(e.SongFileSize(s)), c.w))
		case "like":
			if e.Store.Favorites[s.ID] {
				cell = paint(th.bad).Render(fit("♥", c.w))
			} else {
				cell = paint(th.muted).Render(fit("", c.w))
			}
		}
		parts = append(parts, cell)
	}
	row := paint(th.muted).Render(" ") + strings.Join(parts, gap)
	// Pad with the row's own background so a highlight reaches the edge.
	if pad := w - lipgloss.Width(row); pad > 0 {
		row += paint(th.muted).Render(strings.Repeat(" ", pad))
	}
	return fit(row, w)
}

func (t *tui) emptyList(h int, w int, msg, hint string) []string {
	th := t.th
	out := make([]string, 0, h)
	out = append(out, "", "  "+th.strong().Render(msg))
	if hint != "" {
		out = append(out, "  "+th.dim().Render(hint))
	}
	return out
}

// viewSongList draws a list of songs: Canciones, Favoritos, Recientes, a playlist.
func (t *tui) viewSongList(w, h, x0, y0 int) []string {
	th := t.th
	rows := t.rows
	var out []string
	if t.sec == secPlaylist {
		out = append(out, t.playlistHeader(w)...)
	}
	cols, _ := songColumns(w, false)
	out = append(out, t.songHeader(cols, w), th.fg(th.border).Render(strings.Repeat("─", w)))
	room := max(h-len(out), 1)
	t.vis = room
	if len(rows) == 0 {
		msg, hint := "No hay canciones", ""
		switch {
		case len(t.eng.Store.Songs) == 0:
			msg, hint = "No se encontró música local", "Ve a Ajustes (,) y usa “Force Refresh”, o pulsa ctrl+r para indexar tu carpeta."
		case t.query() != "":
			msg, hint = "Sin resultados", "Esc limpia la búsqueda."
		case t.sec == secFavorites:
			hint = "Marca canciones con f (o l mientras suenan)."
		case t.sec == secRecent:
			hint = "Aquí aparecen las canciones que reproduces."
		}
		return append(out, t.emptyList(room, w, msg, hint)...)
	}
	t.list.fix(len(rows), room)
	for i := t.list.off; i < min(t.list.off+room, len(rows)); i++ {
		s := rows[i]
		selected := i == t.list.cur && !t.focusSide
		y := y0 + len(out)
		out = append(out, t.songRow(s, cols, i+1, selected, s.ID == t.eng.PlayingID, w))
		t.zone(x0, y, w, 1, "row", i)
	}
	return out
}

// playlistHeader is the heading of a playlist view.
func (t *tui) playlistHeader(w int) []string {
	th := t.th
	color := "none"
	kind := "manual"
	for _, pl := range t.eng.Store.Playlists {
		if pl.Name == t.playlist {
			color = pl.Color
			if pl.Smart != nil {
				kind = "inteligente"
			}
		}
	}
	dot := lipgloss.NewStyle().Foreground(th.playlistColor(core.NormalizePlaylistColor(color))).Render("●")
	hint := "N nueva"
	if _, ok := t.currentManualPlaylist(); ok {
		hint = "R renombrar · D eliminar · c color · N nueva"
	}
	left := " " + dot + " " + th.subtle().Render("PLAYLIST ") + th.dim().Render(kind)
	right := th.subtle().Render(hint + " ")
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return []string{fit(left, w)}
	}
	return []string{left + strings.Repeat(" ", gap) + right}
}
