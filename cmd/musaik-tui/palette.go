package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
)

func (t *tui) openPalette() {
	t.pal = paletteState{open: true}
	t.pal.hits = t.actionHits()
}

func (t *tui) paletteKey(k tea.KeyPressMsg) {
	p := &t.pal
	switch k.String() {
	case "esc", "ctrl+p":
		p.open = false
	case "down", "ctrl+n", "tab":
		if n := len(p.hits); n > 0 {
			p.idx = (p.idx + 1) % n
		}
	case "up", "ctrl+k", "shift+tab":
		if n := len(p.hits); n > 0 {
			p.idx = (p.idx + n - 1) % n
		}
	case "enter":
		if p.idx >= 0 && p.idx < len(p.hits) {
			hit := p.hits[p.idx]
			p.open = false
			t.applyActionHit(hit)
		}
	default:
		if p.input.Handle(k) {
			p.hits = t.actionHits()
			p.idx = 0
		}
	}
}

// actionHits is what the command palette lists for the text typed so far:
// quick actions first, then artists, songs and albums.
func (t *tui) actionHits() []actionHit {
	q := strings.TrimSpace(strings.ToLower(t.pal.input.Value()))
	e := t.eng
	out := make([]actionHit, 0, 32)

	actions := []actionHit{
		{Kind: "action", ActionID: "play", Title: "Play", Subtitle: "Playback • space"},
		{Kind: "action", ActionID: "pause", Title: "Pause", Subtitle: "Playback • space"},
		{Kind: "action", ActionID: "next", Title: "Next", Subtitle: "Playback • n"},
		{Kind: "action", ActionID: "previous", Title: "Previous", Subtitle: "Playback • p"},
		{Kind: "action", ActionID: "stop", Title: "Stop", Subtitle: "Playback"},
		{Kind: "action", ActionID: "create-playlist", Title: "Create playlist", Subtitle: "Playlist • N"},
		{Kind: "action", ActionID: "add-current-song-to-playlist", Title: "Add current playing song to playlist", Subtitle: "Playlist"},
		{Kind: "action", ActionID: "like-current-song", Title: "Like current playing song", Subtitle: "Favorites • l"},
		{Kind: "action", ActionID: "sync-current-song-lastfm", Title: "Sync current song with Last.fm", Subtitle: "Last.fm • U"},
		{Kind: "action", ActionID: "reindex", Title: "Reindexar biblioteca", Subtitle: "Biblioteca • ctrl+r"},
		{Kind: "action", ActionID: "ipod", Title: "iPod: detectar / sincronizar", Subtitle: "iPod • I"},
		{Kind: "action", ActionID: "mini", Title: "Mini reproductor", Subtitle: "App • M"},
		{Kind: "action", ActionID: "lyrics", Title: "Mostrar u ocultar la letra", Subtitle: "Playback • L", Keywords: "lyrics letra lrc karaoke"},
		{Kind: "action", ActionID: "theme", Title: "Cambiar tema…", Subtitle: "App • T", Keywords: "theme tema colores dracula catppuccin tokyo rose pine gruvbox nord solarized"},
		{Kind: "action", ActionID: "settings", Title: "Ajustes", Subtitle: "App • ,"},
		{Kind: "action", ActionID: "toggle-sidebar", Title: "Mostrar/ocultar barra lateral", Subtitle: "App • b"},
		{Kind: "action", ActionID: "help", Title: "Atajos de teclado", Subtitle: "App • ?"},
		{Kind: "action", ActionID: "quit", Title: "Cerrar Musaik", Subtitle: "App • q", Keywords: "quit salir cerrar exit"},
	}
	for _, a := range actions {
		if q != "" && !strings.Contains(strings.ToLower(a.Title), q) && !strings.Contains(strings.ToLower(a.ActionID), q) && !strings.Contains(a.Keywords, q) {
			continue
		}
		out = append(out, a)
	}
	if q == "" {
		return out
	}

	artists := 0
	for _, artist := range e.Artists(q) {
		out = append(out, actionHit{Kind: "artist", ArtistName: artist, Title: artist, Subtitle: "Artista"})
		if artists++; artists >= 5 {
			break
		}
	}
	songs := 0
	for _, s := range core.FilterSongs(e.Store.Songs, q) {
		out = append(out, actionHit{Kind: "song", SongID: s.ID, Title: s.Title, Subtitle: fmt.Sprintf("%s • %s", s.Artist, s.Album)})
		if songs++; songs >= 10 {
			break
		}
	}
	albums := 0
	for _, s := range e.Albums(q, "Artista") {
		if strings.TrimSpace(s.Album) == "" {
			continue
		}
		out = append(out, actionHit{Kind: "album", AlbumID: core.AlbumIdentity(s), Title: s.Album, Subtitle: s.Artist})
		if albums++; albums >= 6 {
			break
		}
	}
	return out
}

func (t *tui) applyActionHit(hit actionHit) {
	switch hit.Kind {
	case "action":
		t.applyQuickAction(hit.ActionID)
	case "artist":
		t.artistOpen, t.albumOpen = false, false
		t.goSection(secArtists)
		t.artist = hit.ArtistName
		t.artistOpen = true
		t.artistRows = t.buildArtistRows()
		t.artistDet = listState{}
		for i, it := range t.artistRows {
			if it.isSong() {
				t.artistDet.cur = i
				break
			}
		}
		t.eng.Status = "Artista seleccionado: " + hit.ArtistName
	case "song":
		t.eng.PlaySongByID(hit.SongID)
		t.eng.Status = "Reproduciendo: " + hit.Title
	case "album":
		t.goSection(secAlbums)
		t.albumID = hit.AlbumID
		t.albumOpen = true
		t.albumRows = t.buildAlbumRows()
		t.albumDet = listState{}
		t.eng.Status = "Álbum seleccionado: " + hit.Title
	}
	t.rowsDirty = true
}

func (t *tui) applyQuickAction(id string) {
	e := t.eng
	switch id {
	case "play":
		e.Resume()
	case "pause":
		e.Pause()
	case "next":
		e.Next()
	case "previous":
		e.Previous()
	case "stop":
		e.Stop()
	case "create-playlist":
		t.openNewPlaylist("")
	case "add-current-song-to-playlist":
		song, ok := e.CurrentSong()
		if !ok {
			e.Status = "No hay canción reproduciéndose"
			return
		}
		t.openAddToPlaylist(song.ID)
	case "like-current-song":
		song, ok := e.CurrentSong()
		if !ok {
			e.Status = "No hay canción reproduciéndose"
			return
		}
		e.LikeSong(song)
	case "sync-current-song-lastfm":
		song, ok := e.CurrentSong()
		if !ok {
			e.Status = "No hay canción reproduciéndose"
			return
		}
		e.SyncFromLastFM(song, true)
	case "reindex":
		t.reindex()
	case "ipod":
		t.ipodAction()
	case "mini":
		t.mini = true
	case "theme":
		t.openThemePicker()
	case "lyrics":
		t.toggleLyrics()
	case "settings":
		t.openSettings()
	case "toggle-sidebar":
		t.toggleSidebar()
	case "help":
		t.help = true
	case "quit":
		t.quit()
	}
}
