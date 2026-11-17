package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"musaik/internal/core"
)

type layout struct {
	side                bool
	sw, mw              int
	topH, playerH, stat int
	contentH            int
}

// layout divides the terminal between the sidebar, the main column, the player
// bar and the status line.
func (t *tui) layout() layout {
	l := layout{topH: 2, stat: 1}
	l.side = t.sidebarVisible()
	if l.side {
		l.sw = 24
		if t.w < 100 {
			l.sw = 20
		}
	}
	l.mw = t.w - l.sw
	if l.side {
		l.mw-- // the divider
	}
	l.playerH = 5
	if t.h < 20 {
		l.playerH = 4
	}
	l.contentH = max(t.h-l.topH-l.playerH-l.stat, 1)
	return l
}

// sidebarVisible: the sidebar can be hidden by choice, and it gives way on a
// terminal too narrow for it.
func (t *tui) sidebarVisible() bool {
	return !t.eng.Settings.SidebarHidden && t.w >= 64 && !t.mini
}

type sideEntry struct {
	kind  string // head, nav, playlist, new, ipod, settings
	label string
	sec   sectionID
	name  string
	color string
	count int
}

func (t *tui) sideEntries() []sideEntry {
	e := t.eng
	out := []sideEntry{
		{kind: "head", label: "BIBLIOTECA"},
		{kind: "nav", label: "Canciones", sec: secSongs, count: len(e.Store.Songs)},
		{kind: "nav", label: "Artistas", sec: secArtists},
		{kind: "nav", label: "Álbumes", sec: secAlbums},
		{kind: "nav", label: "Queue", sec: secQueue},
		{kind: "nav", label: "Favoritos", sec: secFavorites},
		{kind: "nav", label: "Recientes", sec: secRecent},
		{kind: "head", label: "PLAYLISTS"},
	}
	for _, pl := range e.VisiblePlaylists() {
		out = append(out, sideEntry{kind: "playlist", label: pl.Name, name: pl.Name, color: pl.Color, count: len(e.SongsForPlaylist(pl.Name))})
	}
	out = append(out,
		sideEntry{kind: "new", label: "+ Nueva playlist"},
		sideEntry{kind: "ipod"},
		sideEntry{kind: "settings", label: "Ajustes", sec: secSettings},
	)
	return out
}

func (t *tui) sideSelectable() []sideEntry {
	var out []sideEntry
	for _, e := range t.sideEntries() {
		if e.kind != "head" {
			out = append(out, e)
		}
	}
	return out
}

// isCurrent tells whether the entry is the view on screen.
func (t *tui) isCurrent(e sideEntry) bool {
	switch e.kind {
	case "nav", "settings":
		return t.sec == e.sec
	case "playlist":
		return t.sec == secPlaylist && t.playlist == e.name
	}
	return false
}

func (t *tui) syncSideCursor() {
	for i, e := range t.sideSelectable() {
		if t.isCurrent(e) {
			t.sideCur = i
			return
		}
	}
}

func (t *tui) sideKey(s string) {
	sel := t.sideSelectable()
	switch s {
	case "up", "k":
		t.sideCur = max(t.sideCur-1, 0)
	case "down", "j":
		t.sideCur = min(t.sideCur+1, len(sel)-1)
	case "home", "g":
		t.sideCur = 0
	case "end", "G":
		t.sideCur = len(sel) - 1
	case "enter":
		if t.sideCur >= 0 && t.sideCur < len(sel) {
			t.activateSide(sel[t.sideCur])
		}
	case "esc":
		t.focusSide = false
	}
}

func (t *tui) activateSide(e sideEntry) {
	switch e.kind {
	case "nav":
		t.goSection(e.sec)
	case "playlist":
		t.goPlaylist(e.name)
	case "new":
		t.openNewPlaylist("")
	case "ipod":
		t.ipodAction()
	case "settings":
		t.openSettings()
	}
}

// viewSidebar draws the sidebar as h lines of w cells.
func (t *tui) viewSidebar(w, h int) []string {
	th := t.th
	var lines []string
	lines = append(lines, "  "+th.accented().Bold(true).Render("♪ ")+th.strong().Render("Musaik"), "")

	entries := t.sideEntries()
	selIdx := -1
	cursorLine := 0
	body := make([]string, 0, len(entries))
	bodySel := make([]int, 0, len(entries)) // selectable index of each body line, -1 for blanks and headings
	for _, e := range entries {
		if e.kind == "head" {
			body = append(body, "", "  "+th.subtle().Bold(true).Render(e.label))
			bodySel = append(bodySel, -1, -1)
			continue
		}
		selIdx++
		if e.kind == "ipod" || e.kind == "settings" {
			continue // drawn in the footer
		}
		onCursor := t.focusSide && selIdx == t.sideCur
		if onCursor {
			cursorLine = len(body)
		}
		body = append(body, t.sideRow(e, w, onCursor))
		bodySel = append(bodySel, selIdx)
	}
	// Keep the cursor in view when there are more playlists than lines.
	room := max(h-len(lines)-4, 3)
	start := 0
	if cursorLine >= room {
		start = cursorLine - room + 1
	}
	if len(body) > room {
		body = body[start:min(start+room, len(body))]
		bodySel = bodySel[start:min(start+room, len(bodySel))]
	}
	if len(body) > 0 && body[0] == "" {
		body, bodySel = body[1:], bodySel[1:]
	}
	for i, sel := range bodySel {
		if sel >= 0 {
			t.zone(0, len(lines)+i, w, 1, "side", sel)
		}
	}
	lines = append(lines, body...)
	return pinBottom(lines, t.sidebarFooter(w), h)
}

func pinBottom(top, bottom []string, h int) []string {
	if len(top)+len(bottom) >= h {
		return append(top[:max(h-len(bottom), 0)], bottom...)
	}
	out := append([]string{}, top...)
	for len(out) < h-len(bottom) {
		out = append(out, "")
	}
	return append(out, bottom...)
}

// painter returns a style factory: every piece of a highlighted row carries the
// row's background, so the highlight has no gaps between differently colored pieces.
func (t *tui) painter(highlight bool) func(color.Color) lipgloss.Style {
	return func(fg color.Color) lipgloss.Style {
		st := lipgloss.NewStyle().Foreground(fg)
		if highlight {
			st = st.Background(t.th.selected)
		}
		return st
	}
}

func (t *tui) sideRow(e sideEntry, w int, onCursor bool) string {
	th := t.th
	if e.kind == "ipod" {
		return ""
	}
	paint := t.painter(onCursor)
	current := t.isCurrent(e)
	marker := paint(th.accent).Render(" ")
	switch {
	case current:
		marker = paint(th.accent).Render("▌")
	case onCursor:
		marker = paint(th.accent).Render("▸")
	}
	lead := paint(th.muted).Render("  ")
	switch e.kind {
	case "playlist":
		lead = paint(th.playlistColor(core.NormalizePlaylistColor(e.color))).Render("●") + paint(th.muted).Render(" ")
	case "settings":
		lead = paint(th.muted).Render("⚙ ")
	}
	count := ""
	if e.count > 0 {
		count = strconv.Itoa(e.count)
	}
	labelW := w - 5
	if count != "" {
		labelW -= len(count) + 1
	}
	fg := th.muted
	switch {
	case onCursor, current:
		fg = th.text
	case e.kind == "new":
		fg = th.accent
	}
	label := paint(fg).Render(fit(e.label, max(labelW, 1)))
	if current {
		label = paint(fg).Bold(true).Render(fit(e.label, max(labelW, 1)))
	}
	row := marker + paint(th.muted).Render(" ") + lead + label
	if count != "" {
		row += paint(th.faint).Render(" " + count)
	}
	return " " + row
}

// sidebarFooter is the iPod panel above the Settings entry.
func (t *tui) sidebarFooter(w int) []string {
	th := t.th
	e := t.eng
	onIPod, onSettings := false, false
	for i, s := range t.sideSelectable() {
		if t.focusSide && i == t.sideCur {
			onIPod, onSettings = s.kind == "ipod", s.kind == "settings"
		}
	}
	paint := t.painter(onIPod)
	dotColor, title := th.faint, "iPod no detectado"
	if e.IPodDetected {
		dotColor, title = th.good, "iPod "+e.IPodName
		if e.IPodBusy {
			title += " …"
		}
	}
	head := paint(th.muted).Render(" ") + paint(dotColor).Render("●") + paint(th.muted).Render(" ") +
		paint(th.text).Bold(true).Render(fit(title, w-4)) + paint(th.muted).Render(" ")
	statsText := fmt.Sprintf("%d álbumes · %s", t.stats.albums, t.stats.size)
	stats := paint(th.muted).Render("   ") + paint(th.muted).Render(fit(statsText, w-4)) + paint(th.muted).Render(" ")
	settings := sideEntry{kind: "settings", label: "Ajustes", sec: secSettings}
	t.zone(0, t.h-t.layout().playerH-t.layout().stat-3, w, 2, "side", len(t.sideSelectable())-2)
	t.zone(0, t.h-t.layout().playerH-t.layout().stat-1, w, 1, "side", len(t.sideSelectable())-1)
	return []string{
		" " + th.fg(th.border).Render(strings.Repeat("─", w-2)),
		fit(head, w),
		fit(stats, w),
		t.sideRow(settings, w, onSettings),
	}
}
