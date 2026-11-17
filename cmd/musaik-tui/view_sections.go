package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"musaik/internal/core"
	"musaik/internal/model"
)

// listHeight is how many rows the list on screen shows, as of the last frame.
func (t *tui) listHeight() int {
	if t.vis > 0 {
		return t.vis
	}
	return 10
}

// renderItems draws a list of songs and headings in room rows, keeping the
// cursor in view. number gives the number shown beside each song.
func (t *tui) renderItems(items []item, ls *listState, cols []column, w, room, x0, y0 int, number func(i int, it item) int) []string {
	th := t.th
	t.vis = room
	ls.fix(len(items), room)
	var out []string
	for i := ls.off; i < min(ls.off+room, len(items)); i++ {
		it := items[i]
		if !it.isSong() {
			if i > ls.off {
				out = append(out, "")
				if len(out) >= room {
					break
				}
			}
			out = append(out, fit(" "+th.accented().Render("▍ ")+th.strong().Render(it.heading), w))
			continue
		}
		selected := i == ls.cur && !t.focusSide
		playing := it.now || it.song.ID == t.eng.PlayingID
		t.zone(x0, y0+len(out), w, 1, "item", i)
		out = append(out, t.songRow(it.song, cols, number(i, it), selected, playing, w))
	}
	return out
}

// ---- artists ---------------------------------------------------------------------------

func (t *tui) viewArtists(w, h, x0, y0 int) []string {
	if t.artistOpen {
		return t.viewArtistDetail(w, h, x0, y0)
	}
	th := t.th
	out := []string{
		fit(" "+th.dim().Render(fit("Artista", max(w-24, 8)))+th.dim().Render(fitRight("Álbumes", 10))+th.dim().Render(fitRight("Pistas", 10)), w),
		th.fg(th.border).Render(strings.Repeat("─", w)),
	}
	room := max(h-len(out), 1)
	t.vis = room
	if len(t.artists) == 0 {
		msg := "No hay artistas"
		if t.query() != "" {
			msg = "Sin resultados"
		}
		return append(out, t.emptyList(room, w, msg, "")...)
	}
	t.artistList.fix(len(t.artists), room)
	for i := t.artistList.off; i < min(t.artistList.off+room, len(t.artists)); i++ {
		name := t.artists[i]
		selected := i == t.artistList.cur && !t.focusSide
		paint := t.painter(selected)
		nameCell := paint(th.text).Bold(true).Render(fit(name, max(w-24, 8)))
		albums := paint(th.muted).Render(fitRight(strconv.Itoa(t.eng.ArtistAlbumCount(name)), 10))
		songs := paint(th.muted).Render(fitRight(strconv.Itoa(t.eng.ArtistSongCount(name)), 10))
		row := paint(th.muted).Render(" ") + nameCell + albums + songs
		if pad := w - lipgloss.Width(row); pad > 0 {
			row += paint(th.muted).Render(strings.Repeat(" ", pad))
		}
		t.zone(x0, y0+len(out), w, 1, "artist", i)
		out = append(out, fit(row, w))
	}
	return out
}

func (t *tui) viewArtistDetail(w, h, x0, y0 int) []string {
	th := t.th
	e := t.eng
	songs := e.SongsForArtist(t.artist)
	var size int64
	for _, s := range songs {
		size += e.SongFileSize(s)
	}
	groups := e.AlbumGroupsForArtist(t.artist)
	avatarW, avatarH := 12, 6
	art := t.avatar(t.artist, avatarW, avatarH)
	info := []string{
		th.subtle().Bold(true).Render("ARTISTA"),
		th.strong().Render(t.artist),
		th.dim().Render(fmt.Sprintf("Biblioteca local · %d álbumes · %d pistas locales · %s", len(groups), len(songs), core.FormatFileSizeMB(size))),
		"",
		th.accented().Render("P") + th.dim().Render(" reproducir artista   ") + th.accented().Render("z") + th.dim().Render(" aleatorio   ") + th.accented().Render("esc") + th.dim().Render(" volver"),
	}
	var out []string
	for i := 0; i < avatarH; i++ {
		line := " " + art[i] + "  "
		if i < len(info) {
			line += info[i]
		}
		out = append(out, fit(line, w))
	}
	out = append(out, th.fg(th.border).Render(strings.Repeat("─", w)))
	room := max(h-len(out), 1)
	cols, _ := songColumns(w, true)
	if len(t.artistRows) == 0 {
		return append(out, t.emptyList(room, w, "Sin canciones", "")...)
	}
	out = append(out, t.renderItems(t.artistRows, &t.artistDet, cols, w, room, x0, y0+len(out), func(i int, it item) int {
		// Under "Más escuchadas" the number is the rank; under an album, the track.
		for j := i - 1; j >= 0; j-- {
			if !t.artistRows[j].isSong() {
				if t.artistRows[j].heading == "Más escuchadas" {
					return i - j
				}
				break
			}
		}
		return it.song.TrackNum
	})...)
	return out
}

// ---- albums ----------------------------------------------------------------------------

func (t *tui) viewAlbums(w, h, x0, y0 int) []string {
	if t.albumOpen {
		return t.viewAlbumDetail(w, h, x0, y0)
	}
	th := t.th
	count := th.dim().Render(fmt.Sprintf(" %d álbumes", len(t.albums)))
	sortHint := th.dim().Render("Ordenar por: ") + th.accented().Render(t.albumSort) + th.subtle().Render("  (o) ")
	gap := w - lipgloss.Width(count) - lipgloss.Width(sortHint)
	top := count
	if gap > 0 {
		top += strings.Repeat(" ", gap) + sortHint
	}
	artistW, fmtW := 24, 8
	if w < 70 {
		artistW = 16
	}
	titleW := max(w-artistW-fmtW-6, 10)
	out := []string{
		fit(top, w),
		fit(" "+th.dim().Render(fit("Álbum", titleW))+"  "+th.dim().Render(fit("Artista", artistW))+"  "+th.dim().Render(fit("Calidad", fmtW)), w),
		th.fg(th.border).Render(strings.Repeat("─", w)),
	}
	room := max(h-len(out), 1)
	t.vis = room
	if len(t.albums) == 0 {
		msg := "No hay álbumes"
		if t.query() != "" {
			msg = "Sin resultados"
		}
		return append(out, t.emptyList(room, w, msg, "")...)
	}
	t.albumList.fix(len(t.albums), room)
	for i := t.albumList.off; i < min(t.albumList.off+room, len(t.albums)); i++ {
		s := t.albums[i]
		selected := i == t.albumList.cur && !t.focusSide
		paint := t.painter(selected)
		playing := core.AlbumIdentity(s) == t.playingAlbum()
		titleStyle := paint(th.text).Bold(true)
		if playing {
			titleStyle = paint(th.accent).Bold(true)
		}
		q := core.CompactFormat(s)
		quality := paint(th.muted).Render(fit(q, fmtW))
		if core.IsLossless(q) {
			quality = lipgloss.NewStyle().Foreground(th.badgeFg).Background(th.badgeBg).Bold(true).Render(fit(" "+q, fmtW))
		}
		row := paint(th.muted).Render(" ") + titleStyle.Render(fit(s.Album, titleW)) + paint(th.muted).Render("  ") +
			paint(th.muted).Render(fit(s.Artist, artistW)) + paint(th.muted).Render("  ") + quality
		if pad := w - lipgloss.Width(row); pad > 0 {
			row += paint(th.muted).Render(strings.Repeat(" ", pad))
		}
		t.zone(x0, y0+len(out), w, 1, "album", i)
		out = append(out, fit(row, w))
	}
	return out
}

// playingAlbum is the identity of the album that is playing.
func (t *tui) playingAlbum() string {
	if s, ok := t.eng.CurrentSong(); ok {
		return core.AlbumIdentity(s)
	}
	return ""
}

func (t *tui) viewAlbumDetail(w, h, x0, y0 int) []string {
	th := t.th
	e := t.eng
	if len(t.albumRows) == 0 {
		return t.emptyList(h, w, "Álbum vacío", "")
	}
	first := t.albumRows[0].song
	dur := core.TotalDuration(songsOf(t.albumRows))
	coverW, coverH := 16, 8
	art := t.cover(first, coverW, coverH)
	heart := th.dim().Render("♡")
	if e.Store.Favorites[first.ID] {
		heart = lipgloss.NewStyle().Foreground(th.bad).Render("♥")
	}
	q := core.CompactFormat(first)
	chip := lipgloss.NewStyle().Foreground(th.badgeFg).Background(th.badgeBg).Bold(true).Render(" " + q + " ")
	info := []string{
		th.subtle().Bold(true).Render("ÁLBUM"),
		th.strong().Render(first.Album) + " " + heart,
		th.dim().Render(first.Artist),
		"",
		chip + " " + th.dim().Render(core.QualityBadge(first)),
		th.dim().Render(fmt.Sprintf("%d pistas · %s", len(t.albumRows), core.FormatDurationLong(dur))),
		"",
		th.accented().Render("P") + th.dim().Render(" reproducir todo   ") + th.accented().Render("z") + th.dim().Render(" aleatorio   ") + th.accented().Render("F") + th.dim().Render(" like   ") + th.accented().Render("esc") + th.dim().Render(" volver"),
	}
	var out []string
	for i := 0; i < coverH; i++ {
		line := " " + art[i] + "  "
		if i < len(info) {
			line += info[i]
		}
		out = append(out, fit(line, w))
	}
	out = append(out, th.fg(th.border).Render(strings.Repeat("─", w)))
	room := max(h-len(out), 1)
	cols, _ := songColumns(w, true)
	out = append(out, t.renderItems(t.albumRows, &t.albumDet, cols, w, room, x0, y0+len(out), func(i int, it item) int {
		if it.song.TrackNum > 0 {
			return it.song.TrackNum
		}
		return i + 1
	})...)
	return out
}

// ---- queue -----------------------------------------------------------------------------

func (t *tui) viewQueue(w, h, x0, y0 int) []string {
	th := t.th
	upcoming := len(t.eng.Queue)
	top := th.dim().Render(fmt.Sprintf(" %d siguientes", upcoming))
	hint := th.subtle().Render("x quitar · C limpiar · enter reproducir ")
	gap := w - lipgloss.Width(top) - lipgloss.Width(hint)
	if gap > 0 {
		top += strings.Repeat(" ", gap) + hint
	}
	artistW := 24
	if w < 70 {
		artistW = 0
	}
	cols := []column{{"num", 4}, {"title", max(w-4-6-2-6, 10)}, {"dur", 6}}
	if artistW > 0 {
		cols = []column{{"num", 4}, {"title", max(w-4-artistW-6-12, 10)}, {"artist", artistW}, {"dur", 6}}
	}
	out := []string{fit(top, w), t.songHeader(cols, w), th.fg(th.border).Render(strings.Repeat("─", w))}
	room := max(h-len(out), 1)
	t.vis = room
	if len(t.queueRows) == 0 {
		return append(out, t.emptyList(room, w, "No hay canciones siguientes", "Usa a sobre una canción para agregarla al queue.")...)
	}
	out = append(out, t.renderItems(t.queueRows, &t.queueList, cols, w, room, x0, y0+len(out), func(i int, it item) int {
		if it.now {
			return 0
		}
		if len(t.queueRows) > 0 && t.queueRows[0].now {
			return i
		}
		return i + 1
	})...)
	return out
}

func songsOf(items []item) []model.Song {
	out := make([]model.Song, 0, len(items))
	for _, it := range items {
		if it.isSong() {
			out = append(out, it.song)
		}
	}
	return out
}
