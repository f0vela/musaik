package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
	"musaik/internal/model"
)

const (
	seekStep      = 5 * time.Second
	seekStepLarge = 30 * time.Second
	volumeStep    = 5
)

func (t *tui) quit() { t.quiting = true }

func (t *tui) onKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		t.quit()
		return nil
	}
	switch {
	case t.modal.kind != modalNone:
		t.modalKey(k)
		return nil
	case t.help:
		if s == "esc" || s == "?" || s == "q" || s == "enter" {
			t.help = false
		}
		return nil
	case t.pal.open:
		t.paletteKey(k)
		return nil
	case t.searching:
		t.searchKey(k)
		return nil
	case t.sec == secSettings && t.set.editing:
		t.settingsEditKey(k)
		return nil
	}
	if t.mini {
		t.miniKey(s)
		return nil
	}
	if t.sec == secSettings && t.settingsKey(s) {
		return nil
	}
	if t.lyr.open && !t.focusSide {
		// The lists behind the lyrics are not shown: their keys do nothing.
		if !t.lyricsKey(s) {
			t.globalKey(s)
		}
		return nil
	}
	if t.globalKey(s) {
		return nil
	}
	if t.focusSide && t.sidebarVisible() {
		t.sideKey(s)
		return nil
	}
	t.sectionKey(s)
	return nil
}

func (t *tui) onPaste(s string) {
	switch {
	case t.modal.kind == modalNewPlaylist || t.modal.kind == modalRenamePlaylist:
		t.modal.input.Paste(s)
	case t.pal.open:
		t.pal.input.Paste(s)
		t.pal.hits, t.pal.idx = t.actionHits(), 0
	case t.searching:
		t.search.Paste(s)
		t.rowsDirty = true
	case t.sec == secSettings && t.set.editing:
		t.settingsPaste(s)
	}
}

// globalKey handles the keys that work in every view; it reports whether it
// used the key.
func (t *tui) globalKey(s string) bool {
	e := t.eng
	switch s {
	case "q":
		t.quit()
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
		} else {
			e.Status = "No hay canción reproduciéndose"
		}
		t.rowsDirty = true
	case "U":
		if song, ok := e.CurrentSong(); ok {
			e.SyncFromLastFM(song, true)
		} else {
			e.Status = "No hay canción reproduciéndose"
		}
	case "ctrl+p", ":":
		t.openPalette()
	case "?":
		t.help = true
	case "T":
		t.openThemePicker()
	case "L":
		t.toggleLyrics()
	case "b":
		t.toggleSidebar()
	case "M":
		t.mini = true
	case ",":
		t.openSettings()
	case "tab":
		if t.sidebarVisible() {
			t.focusSide = !t.focusSide
			if t.focusSide {
				t.syncSideCursor()
			}
		}
	case "/":
		if t.sec != secQueue && t.sec != secSettings {
			t.searching = true
			t.focusSide = false
		}
	case "ctrl+r":
		t.reindex()
	case "I":
		t.ipodAction()
	case "N":
		t.openNewPlaylist("")
	case "1", "2", "3", "4", "5", "6":
		order := []sectionID{secSongs, secArtists, secAlbums, secQueue, secFavorites, secRecent}
		t.goSection(order[int(s[0]-'1')])
	case "7":
		if pls := e.VisiblePlaylists(); len(pls) > 0 {
			t.goPlaylist(pls[0].Name)
		}
	default:
		return false
	}
	return true
}

func (t *tui) toggleSidebar() {
	t.eng.Settings.SidebarHidden = !t.eng.Settings.SidebarHidden
	if t.eng.Settings.SidebarHidden {
		t.focusSide = false
	}
	_ = t.eng.SaveSettings()
}

// ---- sections ------------------------------------------------------------------------

func (t *tui) goSection(sec sectionID) {
	t.lyr.open = false
	if sec != t.sec {
		t.artistOpen, t.albumOpen = false, false
	}
	if sec == secSettings && t.sec != secSettings {
		t.prevSec = t.sec
	}
	t.sec = sec
	t.list = listState{}
	t.focusSide = false
	t.rowsDirty = true
	t.syncSideCursor()
	if sec != secSettings {
		t.eng.Settings.Section = sectionNames[sec]
		_ = t.eng.SaveSettings()
	}
	if sec == secQueue {
		t.queueList = listState{}
	}
}

func (t *tui) goPlaylist(name string) {
	t.lyr.open = false
	t.artistOpen, t.albumOpen = false, false
	t.sec = secPlaylist
	t.playlist = name
	t.list = listState{}
	t.focusSide = false
	t.rowsDirty = true
	t.syncSideCursor()
	t.eng.Settings.Section = sectionNames[secPlaylist]
	_ = t.eng.SaveSettings()
}

func (t *tui) openSettings() {
	t.lyr.open = false
	if t.sec != secSettings {
		t.prevSec = t.sec
	}
	t.initSettings()
	t.sec = secSettings
	t.focusSide = false
	t.syncSideCursor()
}

func (t *tui) closeSettings() {
	t.sec = t.prevSec
	if t.sec == secSettings {
		t.sec = secSongs
	}
	t.rowsDirty = true
	t.syncSideCursor()
}

func (t *tui) reindex() {
	root := strings.TrimSpace(t.eng.Settings.RootInput)
	if root == "" {
		t.eng.Status = "Define la carpeta raíz en Ajustes"
		return
	}
	t.eng.IndexAsync(root, func(error) { t.rowsDirty = true })
}

func (t *tui) ipodAction() {
	if t.eng.IPodDetected {
		t.eng.SyncIPodAsync(nil)
		return
	}
	t.eng.DetectIPod(true)
}

// ---- lists ------------------------------------------------------------------------------

// active is the list the keys move through, with its row count.
func (t *tui) active() (n int, ls *listState, skip func(int) bool) {
	switch t.sec {
	case secArtists:
		if t.artistOpen {
			return len(t.artistRows), &t.artistDet, func(i int) bool { return !t.artistRows[i].isSong() }
		}
		return len(t.artists), &t.artistList, nil
	case secAlbums:
		if t.albumOpen {
			return len(t.albumRows), &t.albumDet, nil
		}
		return len(t.albums), &t.albumList, nil
	case secQueue:
		return len(t.queueRows), &t.queueList, nil
	case secSettings:
		return 0, &t.list, nil
	}
	return len(t.rows), &t.list, nil
}

// selectedSong is the song under the cursor of the current view.
func (t *tui) selectedSong() (model.Song, bool) {
	n, ls, _ := t.active()
	if n == 0 {
		return model.Song{}, false
	}
	switch t.sec {
	case secArtists:
		if t.artistOpen && ls.cur < len(t.artistRows) && t.artistRows[ls.cur].isSong() {
			return t.artistRows[ls.cur].song, true
		}
	case secAlbums:
		if t.albumOpen {
			if ls.cur < len(t.albumRows) {
				return t.albumRows[ls.cur].song, true
			}
		} else if ls.cur < len(t.albums) {
			return t.albums[ls.cur], true
		}
	case secQueue:
		if ls.cur < len(t.queueRows) {
			return t.queueRows[ls.cur].song, true
		}
	case secSettings:
	default:
		if ls.cur < len(t.rows) {
			return t.rows[ls.cur], true
		}
	}
	return model.Song{}, false
}

func (t *tui) sectionKey(s string) {
	n, ls, skip := t.active()
	h := t.listHeight()
	switch s {
	case "up", "k":
		ls.move(-1, n, skip)
	case "down", "j":
		ls.move(1, n, skip)
	case "pgup", "ctrl+u":
		ls.move(-max(h/2, 1), n, skip)
	case "pgdown", "ctrl+d":
		ls.move(max(h/2, 1), n, skip)
	case "home", "g":
		ls.cur = 0
		ls.move(0, n, skip)
		if skip != nil && n > 0 && skip(0) {
			ls.move(1, n, skip)
		}
	case "end", "G":
		ls.cur = max(n-1, 0)
	case "enter":
		t.activate()
	case "esc":
		switch {
		case t.query() != "":
			t.search.Reset()
			t.rowsDirty = true
		case t.artistOpen || t.albumOpen:
			t.closeDetail()
		}
	case "backspace":
		if t.artistOpen || t.albumOpen {
			t.closeDetail()
		}
	case "a":
		t.withSong(func(song model.Song) {
			if t.eng.Enqueue(song.ID) {
				t.eng.Status = fmt.Sprintf("%s agregado al queue", song.Title)
			} else {
				t.eng.Status = "No se pudo agregar al queue"
			}
			t.rowsDirty = true
		})
	case "A":
		t.withSong(func(song model.Song) { t.openAddToPlaylist(song.ID) })
	case "f":
		t.withSong(func(song model.Song) {
			t.eng.ToggleFavorite(song)
			t.rowsDirty = true
		})
	case "u":
		t.withSong(func(song model.Song) { t.eng.SyncFromLastFM(song, false) })
	case "o":
		if t.sec == secAlbums && !t.albumOpen {
			sorts := core.AlbumSorts()
			for i, v := range sorts {
				if v == t.albumSort {
					t.albumSort = sorts[(i+1)%len(sorts)]
					break
				}
			}
			t.rowsDirty = true
			t.albumList = listState{}
		}
	case "P":
		t.playAll(false)
	case "z":
		t.playAll(true)
	case "F":
		if t.sec == secAlbums && t.albumOpen && len(t.albumRows) > 0 {
			t.eng.ToggleFavorite(t.albumRows[0].song)
			t.rowsDirty = true
		}
	case "x", "delete":
		if t.sec == secQueue {
			t.removeQueued()
		}
	case "C":
		if t.sec == secQueue {
			t.eng.ClearQueue()
			t.rowsDirty = true
		}
	case "R":
		if pl, ok := t.currentManualPlaylist(); ok {
			t.openRenamePlaylist(pl)
		}
	case "D":
		if pl, ok := t.currentManualPlaylist(); ok {
			t.openDeletePlaylist(pl)
		}
	case "c":
		if pl, ok := t.currentManualPlaylist(); ok {
			t.cyclePlaylistColor(pl)
		}
	}
	if s == "up" || s == "down" || s == "k" || s == "j" || s == "pgup" || s == "pgdown" || s == "home" || s == "end" || s == "g" || s == "G" || s == "ctrl+u" || s == "ctrl+d" {
		if song, ok := t.selectedSong(); ok && t.sec != secQueue {
			t.eng.SelectedID = song.ID
		}
	}
}

func (t *tui) withSong(fn func(model.Song)) {
	if song, ok := t.selectedSong(); ok {
		fn(song)
	}
}

func (t *tui) closeDetail() {
	t.artistOpen, t.albumOpen = false, false
	t.rowsDirty = true
}

// activate does what Enter does on the row under the cursor.
func (t *tui) activate() {
	e := t.eng
	switch t.sec {
	case secArtists:
		if !t.artistOpen {
			if t.artistList.cur < len(t.artists) {
				t.artist = t.artists[t.artistList.cur]
				t.artistOpen = true
				t.artistRows = t.buildArtistRows()
				t.artistDet = listState{}
				for i, it := range t.artistRows {
					if it.isSong() {
						t.artistDet.cur = i
						break
					}
				}
			}
			return
		}
	case secAlbums:
		if !t.albumOpen {
			if t.albumList.cur < len(t.albums) {
				t.albumID = core.AlbumIdentity(t.albums[t.albumList.cur])
				t.albumOpen = true
				t.albumRows = t.buildAlbumRows()
				t.albumDet = listState{}
			}
			return
		}
	case secQueue:
		i := t.queueList.cur
		if i < len(t.queueRows) {
			if t.queueRows[i].now {
				e.TogglePlayPause()
				return
			}
			idx := i
			if len(t.queueRows) > 0 && t.queueRows[0].now {
				idx--
			}
			e.PlayFromQueue(idx)
			t.rowsDirty = true
		}
		return
	}
	if song, ok := t.selectedSong(); ok {
		e.PlaySongByID(song.ID)
		t.rowsDirty = true
	}
}

// playAll plays an artist or an album from its first song, or one at random.
func (t *tui) playAll(random bool) {
	var songs []model.Song
	switch {
	case t.sec == secArtists && t.artistOpen:
		songs = t.eng.SongsForArtist(t.artist)
		if !random {
			if top := t.eng.TopPlayedForArtist(t.artist, 1); len(top) > 0 {
				songs = top
			}
		}
	case t.sec == secAlbums && t.albumOpen:
		for _, it := range t.albumRows {
			songs = append(songs, it.song)
		}
	case t.sec == secSongs || t.sec == secFavorites || t.sec == secRecent || t.sec == secPlaylist:
		songs = t.rows
	}
	if len(songs) == 0 {
		return
	}
	pick := songs[0]
	if random {
		pick = songs[int(time.Now().UnixNano()%int64(len(songs)))]
	}
	t.eng.PlaySongByID(pick.ID)
	t.rowsDirty = true
}

func (t *tui) removeQueued() {
	i := t.queueList.cur
	if i >= len(t.queueRows) || t.queueRows[i].now {
		return
	}
	idx := i
	if len(t.queueRows) > 0 && t.queueRows[0].now {
		idx--
	}
	t.eng.RemoveQueued(idx)
	t.rowsDirty = true
}

// ---- search ------------------------------------------------------------------------------

func (t *tui) searchKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "enter":
		t.searching = false
	case "esc":
		t.searching = false
		t.search.Reset()
		t.rowsDirty = true
		t.list, t.artistList, t.albumList = listState{}, listState{}, listState{}
	case "up", "down":
		t.searching = false
		t.sectionKey(k.String())
	default:
		if t.search.Handle(k) {
			t.rowsDirty = true
			t.list, t.artistList, t.albumList = listState{}, listState{}, listState{}
		}
	}
	t.eng.Settings.SearchQuery = t.search.Value()
}

// ---- playlists ----------------------------------------------------------------------------

func (t *tui) currentManualPlaylist() (string, bool) {
	if t.sec != secPlaylist {
		return "", false
	}
	for _, name := range t.eng.ManualPlaylistNames() {
		if name == t.playlist {
			return name, true
		}
	}
	return "", false
}

func (t *tui) openNewPlaylist(seed string) {
	t.modal = modalState{kind: modalNewPlaylist, seed: seed}
}

func (t *tui) openRenamePlaylist(name string) {
	t.modal = modalState{kind: modalRenamePlaylist, target: name}
	t.modal.input.Set(name)
}

func (t *tui) openDeletePlaylist(name string) {
	t.modal = modalState{kind: modalDeletePlaylist, target: name}
}

func (t *tui) openAddToPlaylist(songID string) {
	names := t.eng.ManualPlaylistNames()
	if len(names) == 0 {
		t.openNewPlaylist(songID)
		t.eng.Status = "No hay playlists manuales. Crea una para agregar la canción."
		return
	}
	t.modal = modalState{kind: modalAddToPlaylist, seed: songID, choices: append(names, "+ Crear nueva playlist…")}
}

func (t *tui) cyclePlaylistColor(name string) {
	colors := core.PlaylistColors()
	cur := "none"
	for _, pl := range t.eng.Store.Playlists {
		if pl.Name == name {
			cur = core.NormalizePlaylistColor(pl.Color)
		}
	}
	next := colors[0]
	for i, c := range colors {
		if c == cur {
			next = colors[(i+1)%len(colors)]
		}
	}
	if err := t.eng.SetPlaylistColor(name, next); err != nil {
		t.eng.Status = "No se pudo cambiar el color: " + err.Error()
		return
	}
	t.eng.Status = fmt.Sprintf("Color de %s: %s", name, next)
}

func (t *tui) modalKey(k tea.KeyPressMsg) {
	m := &t.modal
	s := k.String()
	if m.kind == modalTheme {
		t.themeKey(s)
		return
	}
	if s == "esc" {
		t.modal = modalState{}
		return
	}
	e := t.eng
	switch m.kind {
	case modalNewPlaylist, modalRenamePlaylist:
		colors := core.PlaylistColors()
		switch s {
		case "enter":
			var err error
			if m.kind == modalNewPlaylist {
				name := strings.TrimSpace(m.input.Value())
				if err = e.CreatePlaylist(name, m.seed, colors[m.color]); err == nil {
					t.modal = modalState{}
					t.goPlaylist(name)
					e.Status = "Playlist creada: " + name
					return
				}
			} else {
				newName := strings.TrimSpace(m.input.Value())
				if err = e.RenamePlaylist(m.target, newName); err == nil {
					if t.playlist == m.target {
						t.playlist = newName
					}
					t.modal = modalState{}
					t.rowsDirty = true
					e.Status = "Playlist renombrada: " + newName
					return
				}
			}
			m.err = err.Error()
		case "tab", "down":
			if m.kind == modalNewPlaylist {
				m.color = (m.color + 1) % len(colors)
			}
		case "shift+tab", "up":
			if m.kind == modalNewPlaylist {
				m.color = (m.color + len(colors) - 1) % len(colors)
			}
		default:
			if m.input.Handle(k) {
				m.err = ""
			}
		}
	case modalDeletePlaylist:
		switch s {
		case "y", "Y", "enter":
			if err := e.DeletePlaylist(m.target); err != nil {
				m.err = err.Error()
				return
			}
			e.Status = "Playlist eliminada: " + m.target
			if t.playlist == m.target {
				t.playlist = ""
				if pls := e.VisiblePlaylists(); len(pls) > 0 {
					t.playlist = pls[0].Name
				} else {
					t.goSection(secSongs)
				}
			}
			t.modal = modalState{}
			t.rowsDirty = true
			t.syncSideCursor()
		case "n", "N":
			t.modal = modalState{}
		}
	case modalAddToPlaylist:
		switch s {
		case "up", "k":
			m.idx = (m.idx + len(m.choices) - 1) % len(m.choices)
		case "down", "j":
			m.idx = (m.idx + 1) % len(m.choices)
		case "enter":
			if m.idx == len(m.choices)-1 {
				seed := m.seed
				t.modal = modalState{}
				t.openNewPlaylist(seed)
				return
			}
			name := m.choices[m.idx]
			if err := e.AddToPlaylist(name, m.seed); err != nil {
				e.Status = "No se pudo agregar a playlist: " + err.Error()
			} else {
				title := m.seed
				if _, song, ok := e.FindSong(m.seed); ok {
					title = song.Title
				}
				e.Status = fmt.Sprintf("%s agregado a playlist %s", title, name)
			}
			t.modal = modalState{}
			t.rowsDirty = true
		}
	}
}
