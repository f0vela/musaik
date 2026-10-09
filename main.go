package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type app struct {
	store    *library.Store
	player   *player.Player
	lastfm   *lastfm.Client
	window   *mygo.Window
	section  section
	sliderAt float64

	rootInput  string
	status     string
	selectedID string
	playIndex  int
	shuffle    bool
	repeat     bool
}

func newApp() *app {
	a := &app{
		store:      library.NewStore(),
		player:     player.New(),
		section:    sectionLibrary,
		rootInput:  defaultMusicDir(),
		status:     "Listo para indexar música",
		selectedID: "",
		playIndex:  -1,
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
	t := c.Theme()
	a.consumePlayerEvents()
	ui.Column(c).Fill().Padding(14).Gap(12).Children(func() {
		ui.Text(c, "Musaik").FontSize(28).Bold()
		ui.Text(c, a.status).TextColor(t.TextMuted)
		a.viewControls(c)
		a.viewSourceBar(c)
		a.viewSections(c)
		ui.Scroll(c).Grow(1).Children(func() {
			a.viewMain(c)
		})
	})
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
	if a.section == sec {
		label = "• " + label
	}
	if ui.Button(c, label).Clicked() {
		a.section = sec
	}
}

func (a *app) viewMain(c *ui.Context) {
	switch a.section {
	case sectionLibrary:
		a.viewSongs(c, a.store.Songs)
	case sectionArtists:
		ui.Column(c).Gap(6).Children(func() {
			for _, artist := range a.store.Artists() {
				ui.Text(c, artist)
			}
		})
	case sectionAlbums:
		ui.Column(c).Gap(6).Children(func() {
			for _, album := range a.store.Albums() {
				ui.Text(c, "💿 "+album)
			}
		})
	case sectionFavorites:
		favs := make([]model.Song, 0, len(a.store.Songs))
		for _, song := range a.store.Songs {
			if a.store.Favorites[song.ID] {
				favs = append(favs, song)
			}
		}
		a.viewSongs(c, favs)
	case sectionPlaylists:
		ui.Column(c).Gap(10).Children(func() {
			for _, pl := range a.store.Playlists {
				count := len(a.store.PlaylistSongs(pl))
				ui.Textf(c, "%s (%d)", pl.Name, count)
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
}

func (a *app) viewSongs(c *ui.Context, songs []model.Song) {
	if len(songs) == 0 {
		ui.Text(c, "No hay canciones")
		return
	}
	limit := len(songs)
	if limit > 80 {
		limit = 80
	}
	ui.Column(c).Gap(4).Children(func() {
		for i := 0; i < limit; i++ {
			song := songs[i]
			a.viewSongRow(c, song)
		}
		if len(songs) > limit {
			ui.Textf(c, "... y %d más", len(songs)-limit)
		}
	})
}

func (a *app) viewSongRow(c *ui.Context, song model.Song) {
	isSelected := song.ID == a.selectedID
	prefix := "  "
	if isSelected {
		prefix = "▶ "
	}
	title := fmt.Sprintf("%s%s — %s — %s [%s]", prefix, song.Title, song.Artist, song.Album, song.Format)

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "Play").Width(50).Clicked() {
			a.playSongByID(song.ID)
		}
		if ui.Button(c, "♥").Width(38).Clicked() {
			a.store.ToggleFavorite(song.ID)
		}
		if ui.Button(c, title).Clicked() {
			a.selectedID = song.ID
			if idx, _, ok := a.findSong(song.ID); ok {
				a.playIndex = idx
			}
		}
	})
}

func (a *app) viewControls(c *ui.Context) {
	state := a.player.State()
	isPaused := state.Paused
	playLabel := "Play/Pause"
	if isPaused {
		playLabel = "Resume"
	}

	duration := state.Duration
	position := state.Position
	if duration > 0 {
		if a.sliderAt <= 0 || a.sliderAt > duration.Seconds() {
			a.sliderAt = position.Seconds()
		}
	}

	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, formatDuration(position))
			if duration > 0 {
				if ui.Slider(c.Key("progress"), &a.sliderAt, 0, duration.Seconds()).Step(1).Width(500).Changed() {
					if err := a.player.SeekTo(time.Duration(a.sliderAt * float64(time.Second))); err != nil {
						a.status = "No se pudo mover la canción: " + err.Error()
					} else {
						a.status = "Posición actualizada"
					}
				}
			} else {
				ui.Text(c, "Sin duración disponible")
			}
			ui.Text(c, formatDuration(duration))
		})

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "Previous").Clicked() {
				a.playPrevious()
			}
			if ui.PrimaryButton(c, playLabel).Clicked() {
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
			if ui.Button(c, "Next").Clicked() {
				a.playNext()
			}
			if ui.Button(c, "Repeat").Clicked() {
				a.repeat = !a.repeat
			}
			if ui.Button(c, "Shuffle").Clicked() {
				a.shuffle = !a.shuffle
			}
			ui.Textf(c, "repeat=%t shuffle=%t", a.repeat, a.shuffle)
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
