package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"musaik/internal/lastfm"
	"musaik/internal/library"
	"musaik/internal/model"
	"musaik/internal/player"
)

type section string

const (
	sectionLibrary   section = "Librería"
	sectionArtists   section = "Artistas"
	sectionAlbums    section = "Álbumes"
	sectionFavorites section = "Favoritos"
	sectionPlaylists section = "Playlists"
)

var (
	sonoraCanvas       = ui.Hex("#121318")
	sonoraToolbar      = ui.RGBA(28, 30, 42, 0.85)
	sonoraSidebar      = ui.RGBA(22, 24, 34, 0.82)
	sonoraCard         = ui.RGBA(30, 32, 44, 0.80)
	sonoraCardAlt      = ui.RGBA(26, 29, 41, 0.82)
	sonoraBorder       = ui.RGBA(255, 255, 255, 0.08)
	sonoraText         = ui.Hex("#F8FAFC")
	sonoraTextMuted    = ui.RGBA(248, 250, 252, 0.64)
	sonoraTextTertiary = ui.RGBA(248, 250, 252, 0.38)
	sonoraAccent       = ui.Hex("#F43F5E")
	sonoraSecondary    = ui.Hex("#8B5CF6")
	sonoraTertiary     = ui.Hex("#06B6D4")
)

type app struct {
	store    *library.Store
	player   *player.Player
	lastfm   *lastfm.Client
	window   *mygo.Window
	section  section
	sliderAt float64
	covers   map[string]*ui.Bitmap
	coverLRU []string

	libraryTable ui.ListState

	rootInput   string
	searchQuery string
	status      string
	selectedID  string
	playIndex   int
	shuffle     bool
	repeat      bool
}

func newApp() *app {
	a := &app{
		store:       library.NewStore(),
		player:      player.New(),
		section:     sectionLibrary,
		rootInput:   defaultMusicDir(),
		searchQuery: "",
		status:      "Listo para indexar música",
		selectedID:  "",
		playIndex:   -1,
		covers:      map[string]*ui.Bitmap{},
		coverLRU:    []string{},
		lastfm: lastfm.New(
			os.Getenv("LASTFM_API_KEY"),
			os.Getenv("LASTFM_API_SECRET"),
			os.Getenv("LASTFM_SESSION_KEY"),
			os.Getenv("LASTFM_USERNAME"),
		),
	}
	_ = a.store.LoadState()
	return a
}

func defaultMusicDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Music")
}

func (a *app) view(c *ui.Context) {
	a.applyTheme(c)
	a.consumePlayerEvents()
	ui.Column(c).Fill().Background(sonoraCanvas).Children(func() {
		a.viewTopBar(c)
		ui.Row(c).Grow(1).Padding(16).Gap(14).AlignItems(ui.Stretch).Children(func() {
			a.viewSidebar(c)
			ui.Column(c).Grow(1).Gap(12).Children(func() {
				a.viewSections(c)
				a.viewMain(c)
			})
		})
		a.viewControls(c)
	})
}

func (a *app) applyTheme(c *ui.Context) {
	t := *c.Theme()
	t.Dark = true
	t.Background = sonoraCanvas
	t.Surface = sonoraCard
	t.SurfaceHover = sonoraCardAlt
	t.SurfacePressed = ui.RGBA(52, 55, 73, 0.94)
	t.Border = sonoraBorder
	t.Text = sonoraText
	t.TextMuted = sonoraTextMuted
	t.Accent = sonoraAccent
	t.AccentHover = ui.Hex("#FB7185")
	t.AccentPressed = ui.Hex("#E11D48")
	t.AccentText = ui.Hex("#33000D")
	t.Focus = ui.RGBA(244, 63, 94, 0.58)
	t.Selection = ui.RGBA(244, 63, 94, 0.28)
	t.Radius = 10
	t.Spacing = 4
	c.SetTheme(&t)
}

func (a *app) viewTopBar(c *ui.Context) {
	ui.Row(c).
		Height(52).
		Padding(10, 14).
		Background(sonoraToolbar).
		BorderWidth(0, 0, 1, 0).
		BorderColor(sonoraBorder).
		AlignItems(ui.Center).
		Gap(12).
		Children(func() {
			ui.Text(c, "Musaik").FontSize(24).Bold().TextColor(sonoraText)
			ui.TextInput(c, &a.searchQuery).
				Placeholder("Buscar en la biblioteca").
				Width(360)
			if strings.TrimSpace(a.searchQuery) != "" {
				if ui.Button(c, "Limpiar").Clicked() {
					a.searchQuery = ""
				}
			}
			ui.Spacer(c)
			ui.Text(c, "Chord Hugo 2 DAC").FontSize(12).TextColor(sonoraTextMuted)
		})
}

func (a *app) viewSidebar(c *ui.Context) {
	ui.Column(c).
		Width(248).
		Grow(1).
		Padding(14, 12).
		Gap(10).
		Background(sonoraSidebar).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Text(c, "Origen").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			ui.TextInput(c, &a.rootInput).Placeholder("/Users/.../Music")
			if ui.PrimaryButton(c, "Indexar biblioteca").Width(220).Clicked() {
				if err := a.store.Index(a.rootInput); err != nil {
					a.status = "Error indexando: " + err.Error()
				} else {
					a.covers = map[string]*ui.Bitmap{}
					a.coverLRU = nil
					a.status = fmt.Sprintf("Indexadas %d canciones", len(a.store.Songs))
					if len(a.store.Songs) > 0 {
						a.selectedID = a.store.Songs[0].ID
					}
				}
			}
			if ui.Button(c, "Guardar estado").Width(220).Clicked() {
				if err := a.store.SaveState(); err != nil {
					a.status = "Error guardando estado: " + err.Error()
				} else {
					a.status = "Estado guardado"
				}
			}

			ui.Box(c).Height(8)
			ui.Text(c, "Biblioteca").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			a.sidebarNavButton(c, sectionLibrary, "Canciones")
			a.sidebarNavButton(c, sectionArtists, "Artistas")
			a.sidebarNavButton(c, sectionAlbums, "Álbumes")
			a.sidebarNavButton(c, sectionFavorites, "Favoritos")
			a.sidebarNavButton(c, sectionPlaylists, "Playlists")
			ui.Spacer(c)
			ui.Text(c, a.status).FontSize(11).TextColor(sonoraTextMuted)
		})
}

func (a *app) sidebarNavButton(c *ui.Context, sec section, label string) {
	btn := ui.Button(c, label).Width(220)
	if a.section == sec {
		btn.Background(ui.RGBA(244, 63, 94, 0.22)).TextColor(sonoraText)
	} else {
		btn.Background(ui.RGBA(255, 255, 255, 0.03)).TextColor(sonoraTextMuted)
	}
	if btn.Clicked() {
		a.section = sec
	}
}

func (a *app) viewSourceBar(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &a.rootInput).Label("Carpeta raíz").Width(560)
		if ui.PrimaryButton(c, "Indexar").Clicked() {
			if err := a.store.Index(a.rootInput); err != nil {
				a.status = "Error indexando: " + err.Error()
			} else {
				a.status = fmt.Sprintf("Indexadas %d canciones", len(a.store.Songs))
				if len(a.store.Songs) > 0 {
					a.selectedID = a.store.Songs[0].ID
				}
			}
		}
		if ui.Button(c, "Guardar estado").Clicked() {
			if err := a.store.SaveState(); err != nil {
				a.status = "Error guardando estado: " + err.Error()
			} else {
				a.status = "Estado guardado"
			}
		}
	})
}

func (a *app) viewSections(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		a.sectionButton(c, sectionLibrary)
		a.sectionButton(c, sectionArtists)
		a.sectionButton(c, sectionAlbums)
		a.sectionButton(c, sectionFavorites)
		a.sectionButton(c, sectionPlaylists)
	})
}

func (a *app) sectionButton(c *ui.Context, sec section) {
	label := string(sec)
	btn := ui.Button(c, label)
	if a.section == sec {
		btn.Background(ui.RGBA(244, 63, 94, 0.18)).TextColor(sonoraText)
	} else {
		btn.Background(ui.RGBA(255, 255, 255, 0.04)).TextColor(sonoraTextMuted)
	}
	if btn.Clicked() {
		a.section = sec
	}
}

func (a *app) viewMain(c *ui.Context) {
	ui.Column(c).Grow(1).Gap(12).Children(func() {
		a.viewHero(c)
		switch a.section {
		case sectionLibrary:
			a.viewSongs(c, a.filteredSongs(a.store.Songs))
		case sectionArtists:
			ui.Scroll(c).Grow(1).Gap(8).Children(func() {
				for _, artist := range a.store.Artists() {
					ui.Row(c).
						Padding(8, 10).
						Background(ui.RGBA(255, 255, 255, 0.03)).
						Border(1, sonoraBorder).
						Radius(10).
						Children(func() {
							ui.Text(c, artist).TextColor(sonoraText)
						})
				}
			})
		case sectionAlbums:
			ui.Scroll(c).Grow(1).Gap(8).Children(func() {
				for _, album := range a.store.Albums() {
					ui.Row(c).
						Padding(8, 10).
						Background(ui.RGBA(255, 255, 255, 0.03)).
						Border(1, sonoraBorder).
						Radius(10).
						Children(func() {
							ui.Text(c, "💿 "+album).TextColor(sonoraText)
						})
				}
			})
		case sectionFavorites:
			favs := make([]model.Song, 0, len(a.store.Songs))
			for _, song := range a.store.Songs {
				if a.store.Favorites[song.ID] {
					favs = append(favs, song)
				}
			}
			a.viewSongs(c, a.filteredSongs(favs))
		case sectionPlaylists:
			ui.Scroll(c).Grow(1).Gap(10).Children(func() {
				for _, pl := range a.store.Playlists {
					count := len(a.store.PlaylistSongs(pl))
					ui.Row(c).
						Padding(8, 10).
						Background(ui.RGBA(255, 255, 255, 0.03)).
						Border(1, sonoraBorder).
						Radius(10).
						Children(func() {
							ui.Textf(c, "%s (%d)", pl.Name, count).TextColor(sonoraText)
						})
				}
				if ui.Button(c, "Crear smart playlist: Favoritos Rock").Clicked() {
					a.store.AddSmartPlaylist("Favoritos Rock", model.SmartPlaylistFilter{
						TitleContains: "rock",
						FavoritesOnly: true,
					})
					a.status = "Playlist dinámica creada"
				}
			})
		}
	})
}

func (a *app) viewHero(c *ui.Context) {
	state := a.player.State()
	_, selectedSong, hasSelected := a.findSong(a.selectedID)
	title := "Tu biblioteca local"
	subtitle := "Colección indexada para reproducción bit-perfect en macOS"
	if hasSelected {
		title = selectedSong.Title
		subtitle = fmt.Sprintf("%s • %s", selectedSong.Artist, selectedSong.Album)
	}
	if state.Playing && hasSelected {
		subtitle += " • reproduciendo"
	}

	ui.Row(c).
		Padding(16, 18).
		Gap(16).
		Background(sonoraCardAlt).
		Border(1, sonoraBorder).
		Radius(14).
		Children(func() {
			ui.Box(c).Size(140, 140).Background(ui.RGBA(244, 63, 94, 0.22)).Radius(12).Children(func() {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "MUSAIK").FontSize(12).FontWeight(700).TextColor(sonoraText)
				})
			})
			ui.Column(c).Grow(1).Gap(8).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "ÁLBUM DEL DÍA").FontSize(11).FontWeight(600).TextColor(sonoraTextMuted)
					ui.Text(c, "24-BIT / 96KHZ FLAC").FontSize(11).Font("monospace").TextColor(sonoraTertiary)
				})
				ui.Text(c, title).FontSize(42).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, subtitle).FontSize(14).TextColor(sonoraTextMuted)
				ui.Row(c).Gap(10).Children(func() {
					if ui.PrimaryButton(c, "▶ Reproducir").Clicked() {
						a.playSelectedOrFirst()
					}
					if ui.Button(c, "↺ Aleatorio").Clicked() {
						a.shuffle = !a.shuffle
					}
					if hasSelected && ui.Button(c, "♡ Like").Clicked() {
						a.store.ToggleFavorite(selectedSong.ID)
					}
				})
			})
		})
}

func (a *app) filteredSongs(in []model.Song) []model.Song {
	q := strings.TrimSpace(strings.ToLower(a.searchQuery))
	if q == "" {
		return in
	}
	out := make([]model.Song, 0, len(in))
	for _, s := range in {
		if strings.Contains(strings.ToLower(s.Title), q) ||
			strings.Contains(strings.ToLower(s.Artist), q) ||
			strings.Contains(strings.ToLower(s.Album), q) {
			out = append(out, s)
		}
	}
	return out
}

func (a *app) viewSongs(c *ui.Context, songs []model.Song) {
	if len(songs) == 0 {
		ui.Row(c).
			Padding(14, 16).
			Background(ui.RGBA(255, 255, 255, 0.03)).
			Border(1, sonoraBorder).
			Radius(10).
			Children(func() {
				ui.Text(c, "No hay canciones").TextColor(sonoraTextMuted)
			})
		return
	}
	cols := []ui.TableColumn{
		{Title: "Cover", ID: "cover", Width: 56, MinWidth: 56, MaxWidth: 56, Fixed: true},
		{Title: "Título", ID: "title", Sortable: true},
		{Title: "Artista", ID: "artist", Width: 190, Sortable: true},
		{Title: "Álbum", ID: "album", Width: 220, Sortable: true},
		{Title: "Formato", ID: "format", Width: 90, Align: ui.End},
		{Title: "Duración", ID: "duration", Width: 90, Align: ui.End},
		{Title: "Like", ID: "like", Width: 56, Align: ui.Center},
	}

	selected := -1
	for i := range songs {
		if songs[i].ID == a.selectedID {
			selected = i
			break
		}
	}

	a.libraryTable.Key = func(i int) any { return songs[i].ID }
	a.libraryTable.Label = func(i int) string { return songs[i].Title }
	a.libraryTable.Selected = &selected

	ui.Column(c).
		Padding(8, 8).
		Grow(1).
		Background(sonoraCard).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Textf(c, "Canciones: %d", len(songs)).
				Padding(6, 8).
				FontSize(12).
				TextColor(sonoraTextMuted)

			table := ui.Table(c, &a.libraryTable, cols, len(songs), func(row, col int) {
				s := songs[row]
				switch col {
				case 0:
					ui.Box(c).Size(34, 34).Radius(6).Background(ui.RGBA(255, 255, 255, 0.07)).Children(func() {
						if bmp := a.coverBitmap(s); bmp != nil {
							ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(6)
						} else {
							ui.Column(c).Fill().Center().Children(func() {
								ui.Text(c, "♪").FontSize(10).TextColor(sonoraTextMuted)
							})
						}
					})
				case 1:
					label := s.Title
					if s.ID == a.selectedID {
						label = "▶ " + label
					}
					ui.Text(c, label).SingleLine().TextColor(sonoraText)
				case 2:
					ui.Text(c, s.Artist).SingleLine().TextColor(sonoraTextMuted)
				case 3:
					ui.Text(c, s.Album).SingleLine().TextColor(sonoraTextMuted)
				case 4:
					ui.Text(c, strings.ToUpper(s.Format)).SingleLine().Font("monospace").TextColor(sonoraSecondary)
				case 5:
					ui.Text(c, formatDuration(s.Duration)).SingleLine().Font("monospace").TextColor(sonoraTextTertiary)
				case 6:
					icon := "♡"
					if a.store.Favorites[s.ID] {
						icon = "♥"
					}
					if ui.Button(c, icon).Width(36).Clicked() {
						a.store.ToggleFavorite(s.ID)
					}
				}
			}).Grow(1)

			if table.Changed() && selected >= 0 && selected < len(songs) {
				a.selectedID = songs[selected].ID
				if idx, _, ok := a.findSong(a.selectedID); ok {
					a.playIndex = idx
				}
			}
			if table.Submitted() && selected >= 0 && selected < len(songs) {
				a.playSongByID(songs[selected].ID)
			}
		})
}

func (a *app) coverBitmap(song model.Song) *ui.Bitmap {
	key := songCoverKey(song)
	if b, ok := a.covers[key]; ok {
		return b
	}
	b, err := decodeEmbeddedCover(song.Path)
	if err != nil {
		a.cacheCover(key, nil)
		return nil
	}
	a.cacheCover(key, b)
	return b
}

func songCoverKey(song model.Song) string {
	if song.Artist != "" && song.Album != "" {
		return strings.ToLower(song.Artist + "||" + song.Album)
	}
	return song.Path
}

func decodeEmbeddedCover(path string) (*ui.Bitmap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, fmt.Errorf("no artwork")
	}
	return ui.DecodeBitmap(pic.Data)
}

func (a *app) cacheCover(key string, bmp *ui.Bitmap) {
	if _, ok := a.covers[key]; ok {
		return
	}
	a.covers[key] = bmp
	a.coverLRU = append(a.coverLRU, key)
	const maxCovers = 220
	if len(a.coverLRU) <= maxCovers {
		return
	}
	evict := a.coverLRU[0]
	a.coverLRU = a.coverLRU[1:]
	delete(a.covers, evict)
}

func (a *app) viewControls(c *ui.Context) {
	state := a.player.State()
	isPaused := state.Paused
	playLabel := "▶"
	if isPaused {
		playLabel = "⏵"
	}
	if state.Playing && !isPaused {
		playLabel = "⏸"
	}

	duration := state.Duration
	position := state.Position
	if duration > 0 {
		if a.sliderAt <= 0 || a.sliderAt > duration.Seconds() {
			a.sliderAt = position.Seconds()
		}
	}

	ui.Column(c).
		Padding(10, 18).
		Gap(8).
		Background(ui.RGBA(20, 22, 30, 0.95)).
		BorderWidth(1, 0, 0, 0).
		BorderColor(sonoraBorder).
		Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, formatDuration(position)).Font("monospace").TextColor(sonoraTextMuted)
				if duration > 0 {
					if ui.Slider(c.Key("progress"), &a.sliderAt, 0, duration.Seconds()).Step(1).Grow(1).Changed() {
						if err := a.player.SeekTo(time.Duration(a.sliderAt * float64(time.Second))); err != nil {
							a.status = "No se pudo mover la canción: " + err.Error()
						} else {
							a.status = "Posición actualizada"
						}
					}
				} else {
					ui.Text(c, "Sin duración disponible").Grow(1).TextColor(sonoraTextTertiary)
				}
				ui.Text(c, formatDuration(duration)).Font("monospace").TextColor(sonoraTextMuted)
			})

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "⇄").Clicked() {
					a.shuffle = !a.shuffle
				}
				if ui.Button(c, "⏮").Clicked() {
					a.playPrevious()
				}
				if ui.PrimaryButton(c, playLabel).Width(44).Clicked() {
					if state.Playing {
						if err := a.player.TogglePause(); err != nil {
							a.status = "No se puede pausar/reanudar: " + err.Error()
						} else if isPaused {
							a.status = "Reproducción reanudada"
						} else {
							a.status = "Reproducción pausada"
						}
					} else {
						a.playSelectedOrFirst()
					}
				}
				if ui.Button(c, "⏭").Clicked() {
					a.playNext()
				}
				if ui.Button(c, "↻").Clicked() {
					a.repeat = !a.repeat
				}
				ui.Spacer(c)
				ui.Textf(c, "shuffle %t", a.shuffle).FontSize(11).TextColor(sonoraTextTertiary)
				ui.Textf(c, "repeat %t", a.repeat).FontSize(11).TextColor(sonoraTextTertiary)
			})
		})
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "--:--"
	}
	total := int(d.Round(time.Second).Seconds())
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%02d:%02d", min, sec)
}

func (a *app) playSongByID(songID string) {
	index, song, ok := a.findSong(songID)
	if !ok {
		a.status = "Canción no encontrada"
		return
	}
	a.playSongAt(index, song)
}

func (a *app) playSelectedOrFirst() {
	if a.selectedID != "" {
		a.playSongByID(a.selectedID)
		return
	}
	if len(a.store.Songs) > 0 {
		a.playSongAt(0, a.store.Songs[0])
	}
}

func (a *app) findSong(songID string) (int, model.Song, bool) {
	for i, s := range a.store.Songs {
		if s.ID == songID {
			return i, s, true
		}
	}
	return -1, model.Song{}, false
}

func (a *app) consumePlayerEvents() {
	for {
		select {
		case ev := <-a.player.Events():
			if ev.Type == player.EventTrackEnded {
				if ev.Err != nil && !strings.Contains(ev.Err.Error(), "signal: killed") {
					a.status = "Error de reproducción: " + ev.Err.Error()
					return
				}
				a.playNextFromAutoEnd()
				return
			}
		default:
			return
		}
	}
}

func (a *app) playNextFromAutoEnd() {
	if len(a.store.Songs) == 0 || a.playIndex < 0 {
		a.status = "Reproducción finalizada"
		return
	}
	if a.shuffle {
		a.playIndex = int(time.Now().UnixNano() % int64(len(a.store.Songs)))
		song := a.store.Songs[a.playIndex]
		a.playSongAt(a.playIndex, song)
		return
	}
	next := a.playIndex + 1
	if next >= len(a.store.Songs) {
		if !a.repeat {
			a.status = "Fin de la cola"
			return
		}
		next = 0
	}
	song := a.store.Songs[next]
	a.playSongAt(next, song)
}

func (a *app) startUIHeartbeat() {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}()
}

func (a *app) playSongAt(index int, song model.Song) {
	if err := a.player.Play(song.Path, song.Duration); err != nil {
		a.status = "Error reproduciendo: " + err.Error()
		return
	}
	a.sliderAt = 0
	a.playIndex = index
	a.selectedID = song.ID
	a.status = fmt.Sprintf("Reproduciendo: %s — %s", song.Artist, song.Title)

	if err := a.lastfm.UpdateNowPlaying(song); err != nil {
		a.status = "Reproduciendo (Last.fm falló): " + err.Error()
	}

	go func(song model.Song) {
		_ = a.lastfm.Scrobble(song, time.Now().Add(-10*time.Second))
	}(song)
}

func (a *app) playPrevious() {
	if len(a.store.Songs) == 0 {
		return
	}
	if a.playIndex <= 0 {
		if !a.repeat {
			a.playIndex = 0
		} else {
			a.playIndex = len(a.store.Songs) - 1
		}
	} else {
		a.playIndex--
	}
	song := a.store.Songs[a.playIndex]
	a.playSongAt(a.playIndex, song)
}

func (a *app) playNext() {
	if len(a.store.Songs) == 0 {
		return
	}
	if a.shuffle {
		a.playIndex = int(time.Now().UnixNano() % int64(len(a.store.Songs)))
	} else if a.playIndex >= len(a.store.Songs)-1 {
		if a.repeat {
			a.playIndex = 0
		} else {
			a.status = "Fin de la cola"
			return
		}
	} else {
		a.playIndex++
	}
	song := a.store.Songs[a.playIndex]
	a.playSongAt(a.playIndex, song)
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Musaik",
			Width:     1100,
			Height:    780,
			MinWidth:  800,
			MinHeight: 520,
			Vibrancy:  mygo.VibrancySidebar,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.window = win
		a.startUIHeartbeat()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
