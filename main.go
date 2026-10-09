package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	store   *library.Store
	player  *player.Player
	lastfm  *lastfm.Client
	section section

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
	ui.Column(c).Fill().Padding(14).Gap(12).Children(func() {
		ui.Text(c, "Musaik").FontSize(28).Bold()
		ui.Text(c, a.status).TextColor(t.TextMuted)
		a.viewSourceBar(c)
		a.viewSections(c)
		a.viewMain(c)
		a.viewControls(c)
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
			a.viewSongRow(c, i, song)
		}
		if len(songs) > limit {
			ui.Textf(c, "... y %d más", len(songs)-limit)
		}
	})
}

func (a *app) viewSongRow(c *ui.Context, idx int, song model.Song) {
	isSelected := song.ID == a.selectedID
	prefix := "  "
	if isSelected {
		prefix = "▶ "
	}
	title := fmt.Sprintf("%s%s — %s — %s [%s]", prefix, song.Title, song.Artist, song.Album, song.Format)

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "Play").Width(50).Clicked() {
			a.playSongAt(idx, song)
		}
		if ui.Button(c, "♥").Width(38).Clicked() {
			a.store.ToggleFavorite(song.ID)
		}
		if ui.Button(c, title).Clicked() {
			a.selectedID = song.ID
			a.playIndex = idx
		}
	})
}

func (a *app) viewControls(c *ui.Context) {
	isPaused := a.player.IsPaused()
	playLabel := "Play/Pause"
	if isPaused {
		playLabel = "Resume"
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "Previous").Clicked() {
			a.playPrevious()
		}
		if ui.PrimaryButton(c, playLabel).Clicked() {
			if err := a.player.TogglePause(); err != nil {
				a.status = "No se puede pausar/reanudar: " + err.Error()
			} else if isPaused {
				a.status = "Reproducción reanudada"
			} else {
				a.status = "Reproducción pausada"
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
}

func (a *app) playSongAt(index int, song model.Song) {
	if err := a.player.Play(song.Path); err != nil {
		a.status = "Error reproduciendo: " + err.Error()
		return
	}
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
		a.playIndex = 0
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
			a.playIndex = len(a.store.Songs) - 1
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
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Musaik",
			Width:     1100,
			Height:    780,
			MinWidth:  800,
			MinHeight: 520,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
