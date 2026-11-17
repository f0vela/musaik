package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// zone is a clickable rectangle of the last frame.
type zone struct {
	x, y, w, h int
	kind       string
	arg        int
}

func (t *tui) zone(x, y, w, h int, kind string, arg int) {
	t.zones = append(t.zones, zone{x, y, w, h, kind, arg})
}

func (t *tui) zoneAt(x, y int) (zone, bool) {
	for i := len(t.zones) - 1; i >= 0; i-- {
		z := t.zones[i]
		if x >= z.x && x < z.x+z.w && y >= z.y && y < z.y+z.h {
			return z, true
		}
	}
	return zone{}, false
}

func (t *tui) onClick(m tea.Mouse) tea.Cmd {
	if m.Button != tea.MouseLeft || t.modal.kind != modalNone || t.pal.open {
		return nil
	}
	if t.help {
		t.help = false
		return nil
	}
	z, ok := t.zoneAt(m.X, m.Y)
	if !ok {
		return nil
	}
	e := t.eng
	switch z.kind {
	case "side":
		sel := t.sideSelectable()
		if z.arg >= 0 && z.arg < len(sel) {
			t.sideCur = z.arg
			t.activateSide(sel[z.arg])
		}
	case "row", "item", "artist", "album":
		t.clickRow(z)
	case "seek":
		if st := e.Player.State(); st.Playing && st.Duration > 0 && z.arg > 0 {
			frac := float64(m.X-z.x) / float64(z.arg-1)
			e.SeekTo(time.Duration(frac * float64(st.Duration)))
		}
	case "vol":
		if z.arg > 1 {
			e.SetVolume((m.X - z.x + 1) * 100 / z.arg)
		}
	case "mute":
		e.ToggleMute()
	case "play":
		e.TogglePlayPause()
	case "next":
		e.Next()
	case "prev":
		e.Previous()
	case "shuffle":
		e.ToggleShuffle()
	case "repeat":
		e.CycleRepeat()
	case "like":
		if song, ok := e.CurrentSong(); ok {
			e.ToggleFavorite(song)
			t.rowsDirty = true
		}
	}
	return nil
}

// clickRow selects the row; clicking the selected row again opens or plays it,
// as it does in the desktop app.
func (t *tui) clickRow(z zone) {
	_, ls, skip := t.active()
	t.focusSide = false
	if z.arg == ls.cur {
		t.activate()
		return
	}
	if skip != nil && skip(z.arg) {
		return
	}
	ls.cur = z.arg
	if song, ok := t.selectedSong(); ok && t.sec != secQueue {
		t.eng.SelectedID = song.ID
	}
}

func (t *tui) onWheel(m tea.Mouse) {
	if t.modal.kind != modalNone || t.pal.open || t.help || t.sec == secSettings {
		return
	}
	n, ls, skip := t.active()
	switch m.Button {
	case tea.MouseWheelUp:
		ls.move(-3, n, skip)
	case tea.MouseWheelDown:
		ls.move(3, n, skip)
	}
}
