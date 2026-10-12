package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"musaik/internal/ipod"
	"musaik/internal/lastfm"
	"musaik/internal/library"
	"musaik/internal/model"
	"musaik/internal/player"
)

type section string

const (
	sectionExplore   section = "Explorar"
	sectionRecent    section = "Recientes"
	sectionLibrary   section = "Librería"
	sectionSongs     section = "Canciones"
	sectionArtists   section = "Artistas"
	sectionAlbums    section = "Álbumes"
	sectionFavorites section = "Favoritos"
	sectionPlaylists section = "Playlists"
	sectionSettings  section = "Ajustes"
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

//go:embed assets/iconoir/*.svg
var iconoirFS embed.FS

func mustIconoir(name string) *ui.SVG {
	data, err := iconoirFS.ReadFile("assets/iconoir/" + name)
	if err != nil {
		panic("missing icon asset: " + name)
	}
	return ui.MustParseSVG(data)
}

var (
	iconHome     = mustIconoir("home-simple.svg")
	iconList     = mustIconoir("list.svg")
	iconProfile  = mustIconoir("profile-circle.svg")
	iconAlbum    = mustIconoir("album.svg")
	iconMusic    = mustIconoir("music-note.svg")
	iconPlaylist = mustIconoir("playlist.svg")
	iconHeart    = mustIconoir("heart.svg")
	iconClock    = mustIconoir("clock.svg")
	iconNavLeft  = mustIconoir("nav-arrow-left.svg")
	iconNavRight = mustIconoir("nav-arrow-right.svg")
	iconBell     = mustIconoir("bell.svg")
	iconPlay     = mustIconoir("play.svg")
	iconPause    = mustIconoir("pause.svg")
	iconPrev     = mustIconoir("fast-arrow-left.svg")
	iconNext     = mustIconoir("fast-arrow-right.svg")
	iconRepeat   = mustIconoir("refresh-circle.svg")
	iconVolume   = mustIconoir("sound-high.svg")
	iconMenu     = mustIconoir("menu.svg")
	iconShuffle  = mustIconoir("arrow-separate.svg")
)

type app struct {
	store    *library.Store
	player   *player.Player
	lastfm   *lastfm.Client
	window   *mygo.Window
	appIcon  *ui.Bitmap
	section  section
	sliderAt float64
	covers   map[string]*ui.Bitmap
	coverLRU []string
	sizeByID map[string]int64

	libraryTable ui.ListState

	rootInput      string
	searchQuery    string
	actionQuery    string
	actionPrevQ    string
	actionOpen     bool
	actionIndex    int
	status         string
	selectedID     string
	selectedArtist string
	artistDetailOn bool
	selectedAlbum  string
	albumDetailOn  bool
	volumeLevel    float64
	playIndex      int
	albumPage      int
	shuffle        bool
	repeat         bool

	lastfmAPIKey     string
	lastfmAPISecret  string
	lastfmSessionKey string
	lastfmUsername   string
	lastfmAuthBusy   bool

	ipodMountPath    string
	ipodPlaylistName string
	ipodSyncBusy     bool
}

type appSettings struct {
	Section          string `json:"section"`
	Shuffle          bool   `json:"shuffle"`
	Repeat           bool   `json:"repeat"`
	RootInput        string `json:"root_input"`
	SearchQuery      string `json:"search_query"`
	LastFMAPIKey     string `json:"lastfm_api_key"`
	LastFMAPISecret  string `json:"lastfm_api_secret"`
	LastFMSessionKey string `json:"lastfm_session_key"`
	LastFMUsername   string `json:"lastfm_username"`
	IPodMountPath    string `json:"ipod_mount_path"`
	IPodPlaylistName string `json:"ipod_playlist_name"`
}

func newApp() *app {
	var appIcon *ui.Bitmap
	if data, err := os.ReadFile("assets/icono_musaik.png"); err == nil {
		appIcon, _ = ui.DecodeBitmap(data)
	}

	a := &app{
		store:            library.NewStore(),
		player:           player.New(),
		appIcon:          appIcon,
		section:          sectionSongs,
		rootInput:        defaultMusicDir(),
		searchQuery:      "",
		status:           "Listo para indexar música",
		selectedID:       "",
		playIndex:        -1,
		volumeLevel:      80,
		albumPage:        0,
		covers:           map[string]*ui.Bitmap{},
		coverLRU:         []string{},
		sizeByID:         map[string]int64{},
		lastfmAPIKey:     os.Getenv("LASTFM_API_KEY"),
		lastfmAPISecret:  os.Getenv("LASTFM_API_SECRET"),
		lastfmSessionKey: os.Getenv("LASTFM_SESSION_KEY"),
		lastfmUsername:   os.Getenv("LASTFM_USERNAME"),
	}
	_ = a.store.LoadState()
	if a.store.Root != "" {
		a.rootInput = a.store.Root
	}
	_ = a.loadSettings()
	if a.rootInput == "" {
		a.rootInput = defaultMusicDir()
	}
	if a.ipodPlaylistName == "" {
		a.ipodPlaylistName = "Todos"
	}
	if len(a.store.Songs) > 0 {
		a.status = fmt.Sprintf("Biblioteca cargada: %d canciones", len(a.store.Songs))
	}
	a.refreshLastFMClient()
	a.volumeLevel = float64(a.player.Volume())
	return a
}

func (a *app) refreshLastFMClient() {
	a.lastfm = lastfm.New(
		a.lastfmAPIKey,
		a.lastfmAPISecret,
		a.lastfmSessionKey,
		a.lastfmUsername,
	)
}

func defaultMusicDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Music")
}

func settingsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "musaik")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func (a *app) saveSettings() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	s := appSettings{
		Section:          string(a.section),
		Shuffle:          a.shuffle,
		Repeat:           a.repeat,
		RootInput:        a.rootInput,
		SearchQuery:      a.searchQuery,
		LastFMAPIKey:     a.lastfmAPIKey,
		LastFMAPISecret:  a.lastfmAPISecret,
		LastFMSessionKey: a.lastfmSessionKey,
		LastFMUsername:   a.lastfmUsername,
		IPodMountPath:    a.ipodMountPath,
		IPodPlaylistName: a.ipodPlaylistName,
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (a *app) loadSettings() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var s appSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch section(s.Section) {
	case sectionExplore, sectionRecent, sectionLibrary, sectionSongs, sectionArtists, sectionAlbums, sectionFavorites, sectionPlaylists, sectionSettings:
		a.section = section(s.Section)
	}
	a.shuffle = s.Shuffle
	a.repeat = s.Repeat
	if s.RootInput != "" {
		a.rootInput = s.RootInput
	}
	a.searchQuery = s.SearchQuery
	if s.LastFMAPIKey != "" {
		a.lastfmAPIKey = s.LastFMAPIKey
	}
	if s.LastFMAPISecret != "" {
		a.lastfmAPISecret = s.LastFMAPISecret
	}
	if s.LastFMSessionKey != "" {
		a.lastfmSessionKey = s.LastFMSessionKey
	}
	if s.LastFMUsername != "" {
		a.lastfmUsername = s.LastFMUsername
	}
	if s.IPodMountPath != "" {
		a.ipodMountPath = s.IPodMountPath
	}
	if s.IPodPlaylistName != "" {
		a.ipodPlaylistName = s.IPodPlaylistName
	}
	return nil
}

func (a *app) persistAll() {
	_ = a.store.SaveState()
	_ = a.saveSettings()
}

func (a *app) startLastFMAutoConnect() {
	if a.lastfmAPIKey == "" || a.lastfmAPISecret == "" {
		a.status = "Falta API key/shared secret de Last.fm"
		return
	}
	if a.lastfmAuthBusy {
		a.status = "Autorización Last.fm en progreso"
		return
	}
	a.lastfmAuthBusy = true
	a.status = "Abriendo autorización de Last.fm en tu navegador…"

	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			a.updateUI(func() {
				a.lastfmAuthBusy = false
				a.status = "No se pudo iniciar callback local de Last.fm: " + err.Error()
			})
			return
		}

		callbackURL := "http://" + listener.Addr().String() + "/callback"
		client := lastfm.New(a.lastfmAPIKey, a.lastfmAPISecret, "", a.lastfmUsername)
		authURL := client.AuthURL(callbackURL)

		tokenCh := make(chan string, 1)
		server := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				token := r.URL.Query().Get("token")
				if token == "" {
					http.Error(w, "Token missing in callback", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte("Last.fm conectado. Puedes cerrar esta pestaña y volver a Musaik."))
				select {
				case tokenCh <- token:
				default:
				}
			}),
		}

		go func() {
			_ = server.Serve(listener)
		}()

		if err := mygo.Shell.OpenExternal(authURL); err != nil {
			_ = server.Close()
			a.updateUI(func() {
				a.lastfmAuthBusy = false
				a.status = "No se pudo abrir navegador para Last.fm: " + err.Error()
			})
			return
		}

		select {
		case token := <-tokenCh:
			sessionKey, username, err := client.SessionFromToken(token)
			_ = server.Close()
			if err != nil {
				a.updateUI(func() {
					a.lastfmAuthBusy = false
					a.status = "Falló auth.getSession de Last.fm: " + err.Error()
				})
				return
			}
			a.updateUI(func() {
				a.lastfmSessionKey = sessionKey
				if username != "" {
					a.lastfmUsername = username
				}
				a.refreshLastFMClient()
				a.persistAll()
				a.lastfmAuthBusy = false
				a.status = "Last.fm conectado correctamente"
			})
		case <-time.After(5 * time.Minute):
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = server.Shutdown(ctx)
			cancel()
			a.updateUI(func() {
				a.lastfmAuthBusy = false
				a.status = "Tiempo de espera agotado en autorización Last.fm"
			})
		}
	}()
}

func (a *app) updateUI(fn func()) {
	if a.window != nil {
		a.window.Update(fn)
		return
	}
	fn()
}

func (a *app) view(c *ui.Context) {
	a.applyTheme(c)
	a.consumePlayerEvents()
	c.OnShortcut(ui.Cmd, ui.KeyP, func() {
		a.actionOpen = !a.actionOpen
		if a.actionOpen {
			a.actionQuery = ""
			a.actionPrevQ = ""
			a.actionIndex = 0
		}
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.actionOpen {
			a.actionOpen = false
		}
	})
	ui.Column(c).Fill().Background(ui.Hex("#161616")).Children(func() {
		a.viewTopBar(c)
		ui.Row(c).Grow(1).Gap(0).AlignItems(ui.Stretch).Children(func() {
			a.viewSidebar(c)
			ui.Column(c).Grow(1).Children(func() {
				a.viewMain(c)
			})
		})
		a.viewControls(c)
	})
	a.viewActionBar(c)
}

func (a *app) applyTheme(c *ui.Context) {
	t := *c.Theme()
	t.Dark = true
	t.Background = ui.Hex("#161616")
	t.Surface = ui.Hex("#212124")
	t.SurfaceHover = ui.Hex("#2A2A2E")
	t.SurfacePressed = ui.Hex("#34343A")
	t.Border = ui.RGBA(255, 255, 255, 0.10)
	t.Text = ui.Hex("#EBEBEF")
	t.TextMuted = ui.RGBA(235, 235, 239, 0.62)
	t.Accent = ui.Hex("#1E6FE8")
	t.AccentHover = ui.Hex("#2A7BF0")
	t.AccentPressed = ui.Hex("#145FC8")
	t.AccentText = ui.Hex("#EEF5FF")
	t.Focus = ui.RGBA(30, 111, 232, 0.45)
	t.Selection = ui.RGBA(30, 111, 232, 0.24)
	t.Radius = 8
	t.Spacing = 4
	c.SetTheme(&t)
}

func (a *app) viewTopBar(c *ui.Context) {
	bar := c.TitleBar()
	barHeight := float32(54)
	if bar.Height > barHeight {
		barHeight = bar.Height
	}
	ui.Row(c).
		Height(barHeight).
		Padding(10, max(12.0, bar.Right+12), 10, max(12.0, bar.Left+12)).
		Background(ui.Hex("#222225")).
		BorderWidth(0, 0, 1, 0).
		BorderColor(ui.RGBA(255, 255, 255, 0.10)).
		AlignItems(ui.Center).
		Gap(10).
		Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "").Width(24).Height(24).Radius(5).Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(sonoraTextMuted).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Icon(c, iconNavLeft).Size(13, 13)
					})
				})
				ui.Button(c, "").Width(24).Height(24).Radius(5).Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(sonoraTextMuted).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Icon(c, iconNavRight).Size(13, 13)
					})
				})
			})
			ui.Text(c, "♪  Música Local / Hi-Res Master").FontSize(23).FontWeight(600).TextColor(ui.Hex("#E9E9ED"))
			ui.Spacer(c).DragWindow()
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "").Width(26).Height(22).Radius(5).Background(ui.RGBA(255, 255, 255, 0.06)).TextColor(sonoraTextMuted).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "◫").FontSize(10)
					})
				})
				ui.Button(c, "").Width(26).Height(22).Radius(5).Background(ui.RGBA(255, 255, 255, 0.15)).TextColor(ui.Hex("#F3F3F5")).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "☰").FontSize(10)
					})
				})
				ui.Button(c, "").Width(26).Height(22).Radius(5).Background(ui.RGBA(255, 255, 255, 0.06)).TextColor(sonoraTextMuted).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "▦").FontSize(10)
					})
				})
			})
			ui.TextInput(c, &a.searchQuery).
				Placeholder("Buscar pista, artista.").
				Width(220)
			ui.Button(c, "").Width(26).Height(22).Radius(5).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "⋯").FontSize(12)
				})
			})
		})
}

func (a *app) viewSidebar(c *ui.Context) {
	ui.Column(c).
		Width(210).
		Padding(10, 12).
		Gap(8).
		AlignItems(ui.Start).
		Background(ui.Hex("#1B1B1D")).
		BorderWidth(0, 0, 0, 1).
		BorderColor(ui.RGBA(255, 255, 255, 0.08)).
		Children(func() {
			ui.Text(c, "BIBLIOTECA").FontSize(11).FontWeight(700).TextColor(ui.RGBA(235, 235, 239, 0.32))
			a.sidebarLibraryItem(c, "♪  Canciones", sectionSongs)
			a.sidebarLibraryItem(c, "◔  Reproducidas recientemente", sectionRecent)
			a.sidebarLibraryItem(c, "♥  Favoritos", sectionFavorites)
			a.sidebarLibraryItem(c, "◎  Álbumes", sectionAlbums)
			a.sidebarLibraryItem(c, "◉  Artistas", sectionArtists)
			ui.Box(c).Height(10)

			ui.Text(c, "PLAYLISTS").FontSize(11).FontWeight(700).TextColor(ui.RGBA(235, 235, 239, 0.32))
			a.sidebarLibraryItem(c, "#  Audiophile Benchmark", sectionPlaylists)
			a.sidebarLibraryItem(c, "#  Late Night Lo-Fi Rips", sectionPlaylists)
			a.sidebarLibraryItem(c, "#  Direct Stream Digital", sectionPlaylists)
			ui.Box(c).Height(10)

			ui.Text(c, "ETIQUETAS").FontSize(11).FontWeight(700).TextColor(ui.RGBA(235, 235, 239, 0.32))
			a.sidebarTag(c, ui.Hex("#A855F7"), "Favoritas Audiófilo")
			a.sidebarTag(c, ui.Hex("#FBBF24"), "Vinilos 24-bit/96kHz")
			a.sidebarTag(c, ui.Hex("#10B981"), "DSD Master Direct")
			a.sidebarTag(c, ui.Hex("#F43F5E"), "Studio Reference")
			ui.Spacer(c)
			ui.Row(c).Width(186).Padding(8, 0).Children(func() {
				ui.Text(c, fmt.Sprintf("%d álbumes listos", len(a.albumRepresentativeSongs()))).FontSize(11).TextColor(ui.RGBA(235, 235, 239, 0.52))
				ui.Spacer(c)
				ui.Text(c, totalLibrarySize(a.store.Songs, a)).FontSize(11).TextColor(ui.RGBA(235, 235, 239, 0.40))
			})
		})
}

func (a *app) sidebarLibraryItem(c *ui.Context, label string, sec section) {
	row := ui.Row(c).Width(186).Padding(7, 6).Radius(7).TextColor(ui.RGBA(235, 235, 239, 0.86))
	if a.section == sec {
		row.Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.Hex("#F4F4F6"))
	} else if row.Hovered() {
		row.Background(ui.RGBA(255, 255, 255, 0.04))
	}
	row.Children(func() {
		ui.Text(c, label).SingleLine().FontSize(12)
	})
	if row.Clicked() {
		a.section = sec
		if sec != sectionArtists {
			a.artistDetailOn = false
		}
		if sec != sectionAlbums {
			a.albumDetailOn = false
		}
		_ = a.saveSettings()
	}
}

func (a *app) sidebarTag(c *ui.Context, color ui.Color, label string) {
	ui.Row(c).Width(186).Padding(6, 5).Gap(8).Children(func() {
		ui.Box(c).Size(9, 9).Radius(5).Background(color)
		ui.Text(c, label).SingleLine().FontSize(12).TextColor(ui.RGBA(235, 235, 239, 0.82))
	})
}

func (a *app) sidebarNavButton(c *ui.Context, sec section, icon *ui.SVG, label string) {
	row := ui.Row(c).Width(160).Padding(7, 5).Gap(8).Radius(7).Background(ui.Transparent)
	isActive := a.section == sec
	if isActive {
		row.Background(ui.RGBA(55, 218, 200, 0.20)).TextColor(ui.Hex("#D9FFFA"))
	} else {
		row.TextColor(sonoraTextMuted)
		if row.Hovered() {
			row.Background(ui.RGBA(255, 255, 255, 0.05)).TextColor(sonoraText)
		}
	}
	row.Children(func() {
		ui.Icon(c, icon).Size(13, 13)
		ui.Text(c, label).FontSize(12)
	})
	if row.Clicked() {
		a.section = sec
		if sec == sectionArtists {
			a.artistDetailOn = false
		}
		if sec != sectionAlbums {
			a.albumDetailOn = false
		}
		_ = a.saveSettings()
	}
}

func (a *app) sidebarPlaylist(c *ui.Context, icon, label string) {
	row := ui.Row(c).Width(156).Padding(6, 4).Gap(8).Radius(6).TextColor(sonoraTextMuted)
	if row.Hovered() {
		row.TextColor(sonoraText)
	}
	row.Children(func() {
		ui.Text(c, icon).FontSize(12)
		ui.Text(c, label).FontSize(12)
	})
}

func (a *app) viewMain(c *ui.Context) {
	ui.Column(c).Grow(1).Children(func() {
		switch a.section {
		case sectionSongs:
			a.viewSongs(c, a.filteredSongs(a.store.Songs))
		case sectionRecent:
			a.viewSongs(c, a.filteredSongs(a.recentSongsLastNDays(15)))
		case sectionFavorites:
			a.viewSongs(c, a.filteredSongs(a.favoriteSongs()))
		case sectionSettings:
			ui.Column(c).Grow(1).Padding(14, 14).Children(func() {
				a.viewSettings(c)
			})
		case sectionArtists:
			ui.Column(c).Grow(1).Padding(12, 12).Children(func() {
				a.viewArtists(c)
			})
		case sectionAlbums:
			ui.Column(c).Grow(1).Padding(12, 12).Children(func() {
				if a.albumDetailOn && a.selectedAlbum != "" {
					a.viewAlbumDetail(c, a.selectedAlbum)
				} else {
					a.viewAlbumsGrid(c)
				}
			})
		default:
			a.viewSongs(c, a.filteredSongs(a.store.Songs))
		}
	})
}

func (a *app) favoriteSongs() []model.Song {
	out := make([]model.Song, 0, len(a.store.Songs))
	for _, song := range a.store.Songs {
		if a.store.Favorites[song.ID] {
			out = append(out, song)
		}
	}
	return out
}

func (a *app) recentSongsLastNDays(days int) []model.Song {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	out := make([]model.Song, 0, len(a.store.Songs))
	for _, song := range a.store.Songs {
		if song.LastPlayedAt.IsZero() || song.LastPlayedAt.Before(cutoff) {
			continue
		}
		out = append(out, song)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastPlayedAt.After(out[j].LastPlayedAt)
	})
	return out
}

func (a *app) viewExplore(c *ui.Context) {
	ui.Scroll(c).Grow(1).Gap(10).Children(func() {
		a.viewHero(c)
		ui.Column(c).
			Padding(12, 12).
			Background(ui.RGBA(10, 16, 24, 0.80)).
			Border(1, ui.RGBA(255, 255, 255, 0.08)).
			Radius(12).
			Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Escuchado recientemente").FontSize(34).FontWeight(700).TextColor(sonoraText)
					ui.Spacer(c)
					ui.Text(c, "Ver todo").FontSize(12).TextColor(ui.RGBA(103, 226, 213, 0.95))
				})
				a.viewAlbumStrip(c, a.albumRepresentativeSongs(), 6)
			})
		if len(a.filteredSongs(a.store.Songs)) == 0 {
			ui.Row(c).Padding(12, 12).Background(ui.RGBA(255, 255, 255, 0.03)).Border(1, sonoraBorder).Radius(10).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "No se encontró música local").FontWeight(700)
					ui.Text(c, "Ve a Ajustes y usa “Indexar Biblioteca” para cargar tus canciones.").FontSize(12).TextColor(sonoraTextMuted)
				})
			})
		}
	})
}

func (a *app) viewAlbumsGrid(c *ui.Context) {
	albums := a.albumRepresentativeSongs()
	if len(albums) == 0 {
		ui.Row(c).
			Padding(14, 16).
			Background(ui.RGBA(255, 255, 255, 0.03)).
			Border(1, sonoraBorder).
			Radius(10).
			Children(func() {
				ui.Text(c, "No hay álbumes").TextColor(sonoraTextMuted)
			})
		return
	}

	const pageSize = 50
	total := len(albums)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if a.albumPage >= totalPages {
		a.albumPage = totalPages - 1
	}
	if a.albumPage < 0 {
		a.albumPage = 0
	}

	start := a.albumPage * pageSize
	end := min(start+pageSize, total)
	pageAlbums := albums[start:end]

	ui.Column(c).Grow(1).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "Álbumes (%d)", total).FontSize(13).TextColor(sonoraTextMuted)
			ui.Spacer(c)
			prev := ui.Button(c, "← Anterior").Disabled(a.albumPage == 0)
			if prev.Clicked() && a.albumPage > 0 {
				a.albumPage--
			}
			ui.Textf(c, "Página %d de %d", a.albumPage+1, totalPages).FontSize(12).TextColor(sonoraTextMuted)
			next := ui.Button(c, "Siguiente →").Disabled(a.albumPage >= totalPages-1)
			if next.Clicked() && a.albumPage < totalPages-1 {
				a.albumPage++
			}
		})

		ui.GridView(c, nil, len(pageAlbums), 110, 168, func(i int) {
			s := pageAlbums[i]
			card := ui.Column(c).
				Padding(4, 4).
				Gap(6).
				Background(ui.RGBA(255, 255, 255, 0.03)).
				Border(1, sonoraBorder).
				Radius(10)
			card.Children(func() {
				ui.Box(c).Size(100, 100).Radius(8).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
					}
				})
				ui.Text(c, s.Album).SingleLine().TextColor(sonoraText)
				ui.Text(c, s.Artist).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
			})
			if card.Clicked() {
				a.selectedAlbum = albumIdentity(s)
				a.albumDetailOn = true
			}
		}).Grow(1)
	})
}

func albumIdentity(s model.Song) string {
	artist := strings.TrimSpace(strings.ToLower(s.Artist))
	album := strings.TrimSpace(strings.ToLower(s.Album))
	if artist == "" && album == "" {
		return strings.TrimSpace(strings.ToLower(s.Path))
	}
	return artist + "||" + album
}

func (a *app) songsForAlbum(identity string) []model.Song {
	out := make([]model.Song, 0, 32)
	for _, s := range a.store.Songs {
		if albumIdentity(s) == identity {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].TrackNum, out[j].TrackNum
		if ti > 0 && tj > 0 && ti != tj {
			return ti < tj
		}
		if ti > 0 && tj == 0 {
			return true
		}
		if tj > 0 && ti == 0 {
			return false
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out
}

func (a *app) viewAlbumDetail(c *ui.Context, identity string) {
	songs := a.songsForAlbum(identity)
	if len(songs) == 0 {
		a.albumDetailOn = false
		a.selectedAlbum = ""
		a.viewAlbumsGrid(c)
		return
	}
	album := strings.TrimSpace(songs[0].Album)
	if album == "" {
		album = "Unknown Album"
	}
	artist := strings.TrimSpace(songs[0].Artist)
	if artist == "" {
		artist = "Unknown Artist"
	}
	var totalDuration time.Duration
	for _, s := range songs {
		totalDuration += s.Duration
	}
	albumLiked := a.store.Favorites[songs[0].ID]
	chip := func(label string, bg ui.Color, fg ui.Color) {
		ui.Text(c, label).FontSize(12).Padding(4, 9).Radius(8).Background(bg).TextColor(fg)
	}
	ui.Column(c).
		Grow(1).
		Background(ui.Hex("#1F1F21")).
		Children(func() {
			ui.Column(c).
				Padding(12, 14).
				Gap(10).
				BorderWidth(0, 0, 1, 0).
				BorderColor(ui.RGBA(255, 255, 255, 0.08)).
				Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						if ui.Button(c, "← Volver").Background(ui.RGBA(255, 255, 255, 0.08)).TextColor(ui.RGBA(235, 235, 239, 0.84)).Clicked() {
							a.albumDetailOn = false
						}
					})
					ui.Row(c).Gap(18).Children(func() {
						ui.Box(c).Size(182, 182).Radius(10).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
							if bmp := a.coverBitmap(songs[0]); bmp != nil {
								ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(10)
							}
						})
						ui.Column(c).Grow(1).Gap(10).Children(func() {
							ui.Text(c, "ÁLBUM DE ESTUDIO").FontSize(12).FontWeight(700).TextColor(ui.RGBA(235, 235, 239, 0.40))
							ui.Text(c, album).FontSize(48).FontWeight(700).TextColor(ui.Hex("#F3F3F5"))
							ui.Textf(c, "%s  ·  Remaster  ·  Catálogo Local", artist).FontSize(23).TextColor(ui.RGBA(235, 235, 239, 0.70))
							ui.Row(c).Gap(8).Children(func() {
								chip("⚡ "+compactFormat(songs[0])+" 96kHz / 24-bit", ui.RGBA(30, 111, 232, 0.22), ui.Hex("#8FC4FF"))
								chip("Bit-Perfect Direct Out", ui.RGBA(16, 185, 129, 0.22), ui.Hex("#66E3BB"))
								chip(fmt.Sprintf("%d pistas · %s", len(songs), formatDurationLong(totalDuration)), ui.RGBA(255, 255, 255, 0.10), ui.RGBA(235, 235, 239, 0.84))
							})
							ui.Row(c).Gap(8).Children(func() {
								if ui.PrimaryButton(c, "▶ Reproducir Todo").Background(ui.Hex("#1E6FE8")).TextColor(ui.Hex("#EFF5FF")).Clicked() {
									a.playSongByID(songs[0].ID)
								}
								if ui.Button(c, "⤬ Aleatorio").Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(235, 235, 239, 0.82)).Clicked() {
									a.playSongByID(songs[time.Now().UnixNano()%int64(len(songs))].ID)
								}
								like := "♡"
								if albumLiked {
									like = "♥"
								}
								likeBtn := ui.Button(c, like).Width(38).Background(ui.RGBA(255, 255, 255, 0.10))
								if albumLiked {
									likeBtn.TextColor(ui.Hex("#F43F5E"))
								}
								if likeBtn.Clicked() {
									a.toggleFavorite(songs[0])
								}
								ui.Button(c, "⋯").Width(38).Background(ui.RGBA(255, 255, 255, 0.10)).TextColor(ui.RGBA(235, 235, 239, 0.72))
							})
						})
					})
				})

			colTrack := float32(56)
			colArtist := float32(170)
			colFormat := float32(190)
			colSize := float32(120)
			colDuration := float32(82)
			ui.Row(c).
				Height(38).
				Padding(10, 12).
				BorderWidth(0, 0, 1, 0).
				BorderColor(ui.RGBA(255, 255, 255, 0.08)).
				TextColor(ui.RGBA(235, 235, 239, 0.56)).
				Children(func() {
					ui.Text(c, "#").Width(colTrack).FontSize(12)
					ui.Text(c, "Título de Pista").Grow(1).FontSize(12)
					ui.Text(c, "Artista").Width(colArtist).FontSize(12)
					ui.Text(c, "Formato / Tasa").Width(colFormat).FontSize(12)
					ui.Text(c, "Tamaño").Width(colSize).FontSize(12)
					ui.Text(c, "Duración").Width(colDuration).FontSize(12)
				})

			ui.Scroll(c.Key("album-tracks-scroll")).Grow(1).Children(func() {
				for i, song := range songs {
					songCopy := song
					track := i + 1
					if songCopy.TrackNum > 0 {
						track = songCopy.TrackNum
					}
					selected := songCopy.ID == a.selectedID
					rowBG := ui.RGBA(255, 255, 255, 0)
					primary := ui.RGBA(235, 235, 239, 0.88)
					secondary := ui.RGBA(235, 235, 239, 0.56)
					if selected {
						rowBG = ui.Hex("#1660C8")
						primary = ui.Hex("#F5F8FF")
						secondary = ui.RGBA(245, 248, 255, 0.88)
					}
					row := ui.Row(c).
						Height(40).
						Padding(8, 12).
						AlignItems(ui.Center).
						Background(rowBG).
						BorderWidth(0, 0, 1, 0).
						BorderColor(ui.RGBA(255, 255, 255, 0.04))
					row.ContextMenu(func(m *ui.Menu) {
						lastfmItem := m.Item("Actualizar conteo desde Last.fm")
						lastfmItem.Disabled(a.lastfm == nil || !a.lastfm.Enabled())
						if lastfmItem.Chosen() {
							a.syncSongPlayCountFromLastFM(songCopy)
						}
					})
					if !selected && row.Hovered() {
						row.Background(ui.RGBA(255, 255, 255, 0.05))
					}
					row.Children(func() {
						ui.Textf(c, "%02d", track).Width(colTrack).Font("monospace").TextColor(secondary)
						ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
							if selected {
								ui.Text(c, "•").FontSize(11).TextColor(ui.Hex("#EAF3FF"))
							}
							ui.Text(c, songCopy.Title).Grow(1).SingleLine().TextColor(primary)
							if selected {
								ui.Text(c, "REPRODUCIENDO").FontSize(10).FontWeight(700).Padding(2, 6).Radius(6).Background(ui.RGBA(255, 255, 255, 0.20)).TextColor(ui.Hex("#EAF3FF"))
							}
						})
						ui.Text(c, songCopy.Artist).Width(colArtist).SingleLine().TextColor(secondary)
						ui.Text(c, compactFormat(songCopy)+" · 96kHz / 24b").Width(colFormat).SingleLine().TextColor(secondary)
						ui.Text(c, formatFileSizeMB(a.songFileSize(songCopy))).Width(colSize).SingleLine().Font("monospace").TextColor(secondary)
						ui.Text(c, formatDuration(songCopy.Duration)).Width(colDuration).SingleLine().Font("monospace").TextColor(secondary)
					})
					if row.Clicked() {
						if a.selectedID == songCopy.ID {
							a.playSongByID(songCopy.ID)
						}
						a.selectedID = songCopy.ID
						if idx, _, ok := a.findSong(songCopy.ID); ok {
							a.playIndex = idx
						}
					}
				}
			})
		})
}

type artistAlbumGroup struct {
	Album string
	Songs []model.Song
}

func (a *app) viewArtists(c *ui.Context) {
	artists := a.filteredArtists()
	if len(artists) == 0 {
		ui.Row(c).
			Padding(14, 16).
			Background(ui.RGBA(255, 255, 255, 0.03)).
			Border(1, sonoraBorder).
			Radius(10).
			Children(func() {
				ui.Text(c, "No hay artistas").TextColor(sonoraTextMuted)
			})
		return
	}
	if a.selectedArtist == "" || !containsString(artists, a.selectedArtist) {
		a.selectedArtist = artists[0]
	}

	if a.artistDetailOn {
		a.viewArtistDetail(c, a.selectedArtist)
		return
	}

	ui.Column(c).
		Grow(1).
		Padding(12, 12).
		Gap(10).
		Background(ui.RGBA(255, 255, 255, 0.03)).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Textf(c, "Artistas (%d)", len(artists)).FontSize(14).FontWeight(700).TextColor(sonoraText)
			ui.Text(c, "Listado alfabético · click para abrir detalle del artista").FontSize(11).TextColor(sonoraTextTertiary)
			ui.Scroll(c.Key("artists-directory-scroll")).Grow(1).Gap(6).Children(func() {
				for _, artist := range artists {
					artistName := artist
					albumCount := a.artistAlbumCount(artistName)
					row := ui.Row(c).
						Padding(8, 10).
						Gap(8).
						Background(ui.RGBA(255, 255, 255, 0.02)).
						Border(1, ui.RGBA(255, 255, 255, 0.06)).
						Radius(10)
					if row.Hovered() {
						row.Background(ui.RGBA(255, 255, 255, 0.07))
					}
					row.Children(func() {
						ui.Text(c, artistName).Grow(1).SingleLine().TextColor(sonoraText)
						ui.Textf(c, "%d álbumes", albumCount).FontSize(12).TextColor(sonoraTextMuted)
					})
					if row.Clicked() {
						a.selectedArtist = artistName
						a.artistDetailOn = true
					}
				}
			})
		})
}

func (a *app) filteredArtists() []string {
	artists := a.store.Artists()
	q := strings.TrimSpace(strings.ToLower(a.searchQuery))
	out := make([]string, 0, len(artists))
	for _, artist := range artists {
		if q == "" || strings.Contains(strings.ToLower(artist), q) {
			out = append(out, artist)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func (a *app) artistAlbumCount(artist string) int {
	albums := map[string]bool{}
	for _, s := range a.store.Songs {
		if s.Artist != artist {
			continue
		}
		album := strings.TrimSpace(s.Album)
		if album == "" {
			album = "Unknown Album"
		}
		albums[strings.ToLower(album)] = true
	}
	return len(albums)
}

func (a *app) viewArtistDetail(c *ui.Context, artist string) {
	groups := a.groupSongsByAlbumForArtist(artist)
	ui.Column(c).
		Grow(1).
		Padding(12, 12).
		Gap(10).
		Background(ui.RGBA(255, 255, 255, 0.03)).
		Border(1, sonoraBorder).
		Radius(12).
		Clip().
		Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				if ui.Button(c, "← Volver a artistas").Clicked() {
					a.artistDetailOn = false
				}
				ui.Text(c, artist).FontSize(28).FontWeight(700).TextColor(sonoraText)
				ui.Textf(c, "%d álbumes · %d canciones", len(groups), countSongsInGroups(groups)).FontSize(12).TextColor(sonoraTextMuted)
			})

			ui.Row(c).Gap(10).Children(func() {
				preview := min(3, len(groups))
				for i := 0; i < preview; i++ {
					group := groups[i]
					ui.Column(c).
						Width(170).
						Padding(8, 8).
						Gap(6).
						Background(ui.RGBA(255, 255, 255, 0.03)).
						Border(1, sonoraBorder).
						Radius(10).
						Children(func() {
							ui.Box(c).Size(150, 150).Radius(10).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
								if len(group.Songs) > 0 {
									if bmp := a.coverBitmap(group.Songs[0]); bmp != nil {
										ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(10)
									}
								}
							})
							ui.Text(c, group.Album).SingleLine().TextColor(sonoraText)
							ui.Textf(c, "%d canciones", len(group.Songs)).FontSize(11).TextColor(sonoraTextMuted)
						})
				}
			})

			ui.Scroll(c.Key("artist-detail-scroll")).
				Grow(1).
				Gap(10).
				Children(func() {
					if len(groups) == 0 {
						ui.Text(c, "Sin canciones para este artista").TextColor(sonoraTextMuted)
						return
					}
					for _, group := range groups {
						groupCopy := group
						ui.Column(c).
							Padding(10, 10).
							Gap(8).
							Background(ui.RGBA(255, 255, 255, 0.03)).
							Border(1, sonoraBorder).
							Radius(12).
							Children(func() {
								ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
									ui.Text(c, groupCopy.Album).FontSize(17).FontWeight(700).TextColor(sonoraText)
									ui.Textf(c, "%d canciones", len(groupCopy.Songs)).FontSize(12).TextColor(sonoraTextMuted)
								})
								for _, song := range groupCopy.Songs {
									songCopy := song
									trackLabel := "—"
									if songCopy.TrackNum > 0 {
										trackLabel = fmt.Sprintf("%d", songCopy.TrackNum)
									}
									ui.Row(c).
										Padding(6, 8).
										Gap(10).
										Background(ui.RGBA(255, 255, 255, 0.02)).
										Border(1, ui.RGBA(255, 255, 255, 0.04)).
										Radius(8).
										AlignItems(ui.Center).
										Children(func() {
											ui.Text(c, trackLabel).Font("monospace").Width(28).TextColor(sonoraTextTertiary)
											ui.Text(c, songCopy.Title).Grow(1).SingleLine().TextColor(sonoraText)
											ui.Text(c, formatDuration(songCopy.Duration)).Font("monospace").TextColor(sonoraTextMuted)
											if ui.Button(c, "▶").Width(30).Clicked() {
												a.playSongByID(songCopy.ID)
											}
										})
								}
							})
					}
				})
		})
}

func countSongsInGroups(groups []artistAlbumGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Songs)
	}
	return total
}

func (a *app) groupSongsByAlbumForArtist(artist string) []artistAlbumGroup {
	songs := make([]model.Song, 0, 64)
	for _, s := range a.store.Songs {
		if s.Artist == artist {
			songs = append(songs, s)
		}
	}
	sort.Slice(songs, func(i, j int) bool {
		ai := strings.ToLower(strings.TrimSpace(songs[i].Album))
		aj := strings.ToLower(strings.TrimSpace(songs[j].Album))
		if ai != aj {
			return ai < aj
		}
		ti, tj := songs[i].TrackNum, songs[j].TrackNum
		if ti > 0 && tj > 0 && ti != tj {
			return ti < tj
		}
		if ti > 0 && tj == 0 {
			return true
		}
		if tj > 0 && ti == 0 {
			return false
		}
		return strings.ToLower(songs[i].Title) < strings.ToLower(songs[j].Title)
	})

	groups := make([]artistAlbumGroup, 0, 16)
	for _, s := range songs {
		album := strings.TrimSpace(s.Album)
		if album == "" {
			album = "Unknown Album"
		}
		if len(groups) == 0 || groups[len(groups)-1].Album != album {
			groups = append(groups, artistAlbumGroup{Album: album, Songs: []model.Song{s}})
			continue
		}
		last := len(groups) - 1
		groups[last].Songs = append(groups[last].Songs, s)
	}
	return groups
}

func (a *app) viewSettings(c *ui.Context) {
	playlistOptions := a.playlistOptions()
	if len(playlistOptions) > 0 && !containsString(playlistOptions, a.ipodPlaylistName) {
		a.ipodPlaylistName = playlistOptions[0]
	}

	ui.Column(c).
		Padding(14, 16).
		Gap(12).
		Background(sonoraCard).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Text(c, "Ajustes de biblioteca").FontSize(20).FontWeight(700).TextColor(sonoraText)
			ui.Text(c, "Selecciona la carpeta raíz e indexa cuando cambie tu colección.").TextColor(sonoraTextMuted)
			ui.TextInput(c, &a.rootInput).Label("Carpeta raíz").Width(640)
			ui.Row(c).Gap(10).Children(func() {
				if ui.PrimaryButton(c, "Indexar biblioteca").Clicked() {
					a.indexLibrary()
				}
				if ui.Button(c, "Guardar estado").Clicked() {
					if err := a.store.SaveState(); err != nil {
						a.status = "Error guardando estado: " + err.Error()
					} else {
						_ = a.saveSettings()
						a.status = "Estado guardado"
					}
				}
			})
			ui.Box(c).Height(10)
			ui.Text(c, "Integración Last.fm").FontSize(18).FontWeight(700).TextColor(sonoraText)
			ui.TextInput(c, &a.lastfmUsername).Label("Usuario").Width(640)
			ui.TextInput(c, &a.lastfmAPIKey).Label("API key").Width(640)
			ui.TextInput(c, &a.lastfmAPISecret).Label("Shared secret").Width(640)
			ui.TextInput(c, &a.lastfmSessionKey).Label("Session key").Width(640)
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, "Guardar credenciales Last.fm").Clicked() {
					a.refreshLastFMClient()
					if err := a.saveSettings(); err != nil {
						a.status = "No se guardaron credenciales Last.fm: " + err.Error()
					} else {
						a.status = "Credenciales Last.fm guardadas"
					}
				}
				if ui.PrimaryButton(c, "Conectar Last.fm automáticamente").Disabled(a.lastfmAuthBusy).Clicked() {
					a.startLastFMAutoConnect()
				}
			})
			if a.lastfm != nil && a.lastfm.Enabled() {
				ui.Text(c, "Last.fm activo para now playing y scrobble").TextColor(sonoraTertiary)
			} else {
				ui.Text(c, "Last.fm parcial: falta session key para scrobble/now playing").TextColor(sonoraTextMuted)
			}
			ui.Box(c).Height(10)
			ui.Text(c, "iPod Classic (5th Gen / Video)").FontSize(18).FontWeight(700).TextColor(sonoraText)
			ui.Text(c, "Detecta un iPod en /Volumes y exporta playlist en modo disco.").TextColor(sonoraTextMuted)
			ui.TextInput(c, &a.ipodMountPath).Label("Ruta de montaje iPod").Width(640)
			if len(playlistOptions) == 0 {
				playlistOptions = []string{"Todos"}
			}
			ui.Select(c, &a.ipodPlaylistName, playlistOptions).Label("Playlist a sincronizar").Width(320)
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, "Detectar iPod Classic").Disabled(a.ipodSyncBusy).Clicked() {
					a.detectIPodClassic()
				}
				if ui.PrimaryButton(c, "Sincronizar playlist a iPod").Disabled(a.ipodSyncBusy || strings.TrimSpace(a.ipodMountPath) == "").Clicked() {
					a.syncPlaylistToIPod()
				}
				if ui.Button(c, "Guardar ajustes iPod").Clicked() {
					if err := a.saveSettings(); err != nil {
						a.status = "No se guardaron ajustes iPod: " + err.Error()
					} else {
						a.status = "Ajustes iPod guardados"
					}
				}
			})
			ui.Text(c, "Nota: se exporta como archivos + M3U; para firmware Apple puede requerir gestor externo para iTunesDB.").FontSize(11).TextColor(sonoraTextTertiary)
			ui.Text(c, a.status).TextColor(sonoraTextMuted)
		})
}

type actionHit struct {
	Kind       string
	ActionID   string
	SongID     string
	ArtistName string
	AlbumName  string
	Title      string
	Subtitle   string
}

func (a *app) viewActionBar(c *ui.Context) {
	if !a.actionOpen {
		return
	}
	hits := a.actionHits()
	if len(hits) > 12 {
		hits = hits[:12]
	}
	if len(hits) == 0 {
		a.actionIndex = 0
	} else if a.actionIndex < 0 || a.actionIndex >= len(hits) {
		a.actionIndex = len(hits) - 1
	}
	if a.actionQuery != a.actionPrevQ {
		a.actionIndex = 0
		a.actionPrevQ = a.actionQuery
	}
	ui.Modal(c, &a.actionOpen, func() {
		ui.Column(c).
			Width(720).
			MaxWidth(720).
			Padding(12, 12).
			Gap(10).
			Background(sonoraCardAlt).
			Border(1, sonoraBorder).
			Radius(12).
			Children(func() {
				input := ui.TextInput(c, &a.actionQuery).
					Placeholder("Buscar artista o canción…").
					AutoFocus().
					Width(696)
				if len(hits) > 0 {
					input.HandleInput(func(ev ui.InputEvent) bool {
						if ev.Kind != ui.InputKeyDown {
							return false
						}
						switch ev.Key {
						case ui.KeyDown:
							a.actionIndex = (a.actionIndex + 1) % len(hits)
							return true
						case ui.KeyUp:
							a.actionIndex = (a.actionIndex + len(hits) - 1) % len(hits)
							return true
						case ui.KeyEnter:
							if a.actionIndex < 0 || a.actionIndex >= len(hits) {
								a.actionIndex = 0
							}
							a.applyActionHit(hits[a.actionIndex])
							return true
						default:
							return false
						}
					})
				}
				if input.Submitted() && len(hits) > 0 {
					a.applyActionHit(hits[a.actionIndex])
				}
				ui.Text(c, "Cmd+P abrir/cerrar · Esc cerrar · ↑/↓ seleccionar · Enter ejecutar").FontSize(11).TextColor(sonoraTextTertiary)
				if len(hits) == 0 {
					ui.Row(c).
						Padding(10, 10).
						Background(ui.RGBA(255, 255, 255, 0.03)).
						Border(1, sonoraBorder).
						Radius(8).
						Children(func() {
							ui.Text(c, "Sin resultados").TextColor(sonoraTextMuted)
						})
					return
				}
				ui.Column(c).Gap(6).Children(func() {
					for i, hit := range hits {
						h := hit
						idx := i
						row := ui.Row(c).
							Padding(8, 10).
							Gap(8).
							Border(1, sonoraBorder).
							Radius(8)
						if idx == a.actionIndex {
							row.Background(ui.RGBA(244, 63, 94, 0.16))
						} else {
							row.Background(ui.RGBA(255, 255, 255, 0.03))
							if row.Hovered() {
								row.Background(ui.RGBA(255, 255, 255, 0.08))
							}
						}
						row.Children(func() {
							ui.Column(c).Grow(1).Gap(2).Children(func() {
								ui.Text(c, h.Title).SingleLine().TextColor(sonoraText)
								ui.Text(c, h.Subtitle).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
							})
						})
						if row.Clicked() {
							a.actionIndex = idx
							a.applyActionHit(h)
						}
					}
				})
			})
	})
}

func (a *app) actionHits() []actionHit {
	q := strings.TrimSpace(strings.ToLower(a.actionQuery))
	artists := a.store.Artists()
	out := make([]actionHit, 0, 24)

	actions := []actionHit{
		{Kind: "action", ActionID: "play", Title: "Play", Subtitle: "Acción"},
		{Kind: "action", ActionID: "pause", Title: "Pause", Subtitle: "Acción"},
		{Kind: "action", ActionID: "next", Title: "Next", Subtitle: "Acción"},
		{Kind: "action", ActionID: "previous", Title: "Previous", Subtitle: "Acción"},
		{Kind: "action", ActionID: "stop", Title: "Stop", Subtitle: "Acción"},
		{Kind: "action", ActionID: "settings", Title: "Ajustes", Subtitle: "Acción"},
		{Kind: "action", ActionID: "quit", Title: "Quit", Subtitle: "Acción"},
	}
	for _, action := range actions {
		if q != "" &&
			!strings.Contains(strings.ToLower(action.Title), q) &&
			!strings.Contains(strings.ToLower(action.ActionID), q) {
			continue
		}
		out = append(out, action)
	}

	for _, artist := range artists {
		if q != "" && !strings.Contains(strings.ToLower(artist), q) {
			continue
		}
		out = append(out, actionHit{
			Kind:       "artist",
			ArtistName: artist,
			Title:      artist,
			Subtitle:   "Artista",
		})
		if len(out) >= 8 {
			break
		}
	}
	for _, song := range a.store.Songs {
		if q != "" &&
			!strings.Contains(strings.ToLower(song.Title), q) &&
			!strings.Contains(strings.ToLower(song.Artist), q) &&
			!strings.Contains(strings.ToLower(song.Album), q) {
			continue
		}
		out = append(out, actionHit{
			Kind:     "song",
			SongID:   song.ID,
			Title:    song.Title,
			Subtitle: fmt.Sprintf("%s • %s", song.Artist, song.Album),
		})
		if len(out) >= 24 {
			break
		}
	}
	seenAlbums := map[string]bool{}
	for _, song := range a.store.Songs {
		album := strings.TrimSpace(song.Album)
		if album == "" {
			continue
		}
		key := strings.ToLower(song.Artist + "||" + album)
		if seenAlbums[key] {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(album), q) &&
			!strings.Contains(strings.ToLower(song.Artist), q) {
			continue
		}
		seenAlbums[key] = true
		out = append(out, actionHit{
			Kind:      "album",
			AlbumName: album,
			Title:     album,
			Subtitle:  "Álbum • " + song.Artist,
		})
		if len(out) >= 36 {
			break
		}
	}
	return out
}

func (a *app) applyActionHit(hit actionHit) {
	switch hit.Kind {
	case "action":
		a.applyQuickAction(hit.ActionID)
	case "artist":
		a.section = sectionArtists
		a.selectedArtist = hit.ArtistName
		a.artistDetailOn = true
		a.status = "Artista seleccionado: " + hit.ArtistName
	case "song":
		a.section = sectionLibrary
		a.playSongByID(hit.SongID)
	case "album":
		a.section = sectionAlbums
		a.searchQuery = hit.AlbumName
		a.status = "Álbum seleccionado: " + hit.AlbumName
	}
	a.actionOpen = false
	_ = a.saveSettings()
}

func (a *app) applyQuickAction(actionID string) {
	state := a.player.State()
	switch actionID {
	case "play":
		if state.Playing && state.Paused {
			if err := a.player.TogglePause(); err != nil {
				a.status = "No se pudo reanudar: " + err.Error()
				return
			}
			a.status = "Reproducción reanudada"
			return
		}
		if state.Playing {
			a.status = "Ya está reproduciendo"
			return
		}
		a.playSelectedOrFirst()
	case "pause":
		if !state.Playing || state.Paused {
			a.status = "No hay reproducción activa para pausar"
			return
		}
		if err := a.player.TogglePause(); err != nil {
			a.status = "No se pudo pausar: " + err.Error()
			return
		}
		a.status = "Reproducción pausada"
	case "next":
		a.playNext()
	case "previous":
		a.playPrevious()
	case "stop":
		a.player.Stop()
		a.status = "Reproducción detenida"
	case "settings":
		a.section = sectionSettings
		a.status = "Ajustes"
	case "quit":
		a.player.Stop()
		a.persistAll()
		mygo.App.Quit()
	}
}

func (a *app) playlistOptions() []string {
	out := make([]string, 0, len(a.store.Playlists))
	for _, pl := range a.store.Playlists {
		out = append(out, pl.Name)
	}
	if len(out) == 0 {
		out = append(out, "Todos")
	}
	return out
}

func (a *app) songsForPlaylist(name string) []model.Song {
	for _, pl := range a.store.Playlists {
		if pl.Name == name {
			return a.store.PlaylistSongs(pl)
		}
	}
	return a.store.Songs
}

func (a *app) detectIPodClassic() {
	devices, err := ipod.DetectClassicMounts("/Volumes")
	if err != nil {
		a.status = "Error detectando iPod: " + err.Error()
		return
	}
	if len(devices) == 0 {
		a.status = "No se detectó iPod Classic montado en /Volumes"
		return
	}
	a.ipodMountPath = devices[0].MountPath
	_ = a.saveSettings()
	a.status = "iPod detectado: " + devices[0].Name
}

func (a *app) syncPlaylistToIPod() {
	if a.ipodSyncBusy {
		return
	}
	mount := strings.TrimSpace(a.ipodMountPath)
	if mount == "" {
		a.status = "Define la ruta de montaje del iPod"
		return
	}
	playlist := strings.TrimSpace(a.ipodPlaylistName)
	if playlist == "" {
		playlist = "Todos"
	}
	songs := a.songsForPlaylist(playlist)
	if len(songs) == 0 {
		a.status = "La playlist está vacía"
		return
	}

	a.ipodSyncBusy = true
	a.status = fmt.Sprintf("Sincronizando %d canciones hacia iPod…", len(songs))
	_ = a.saveSettings()
	go func(mount, playlist string, songs []model.Song) {
		copied, m3uPath, err := ipod.SyncPlaylist(mount, playlist, songs)
		a.updateUI(func() {
			a.ipodSyncBusy = false
			if err != nil {
				a.status = "Falló sincronización iPod: " + err.Error()
				return
			}
			a.status = fmt.Sprintf("iPod sincronizado: %d copias nuevas/actualizadas, playlist en %s", copied, m3uPath)
		})
	}(mount, playlist, songs)
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func (a *app) indexLibrary() {
	if err := a.store.Index(a.rootInput); err != nil {
		a.status = "Error indexando: " + err.Error()
		return
	}
	a.covers = map[string]*ui.Bitmap{}
	a.coverLRU = nil
	a.sizeByID = map[string]int64{}
	a.status = fmt.Sprintf("Indexadas %d canciones", len(a.store.Songs))
	if len(a.store.Songs) > 0 {
		a.selectedID = a.store.Songs[0].ID
	}
	a.persistAll()
}

func (a *app) viewHero(c *ui.Context) {
	ui.Row(c).
		Padding(16, 16).
		Gap(14).
		Background(ui.RGBA(10, 17, 26, 0.86)).
		Border(1, ui.RGBA(255, 255, 255, 0.08)).
		Radius(12).
		Children(func() {
			ui.Column(c).Grow(1).Gap(9).Children(func() {
				ui.Box(c).Height(2)
				ui.Text(c, "Tu música,\nsin límites.").FontSize(58).FontWeight(800).TextColor(sonoraText)
				ui.Text(c, "Escucha. Descubre. Vive.").FontSize(22).TextColor(sonoraTextMuted)
				ui.Row(c).Gap(10).Children(func() {
					play := ui.PrimaryButton(c, "▶ Reproducir todo").Background(ui.Hex("#2DE5D3")).TextColor(ui.Hex("#06201C"))
					if play.Clicked() {
						a.playSelectedOrFirst()
					}
					browse := ui.Button(c, "◌ Biblioteca").Background(ui.RGBA(255, 255, 255, 0.04)).Border(1, ui.RGBA(255, 255, 255, 0.18))
					if browse.Clicked() {
						a.section = sectionLibrary
					}
				})
			})
			ui.Box(c).Size(370, 220).Radius(12).Clip().Background(ui.RGBA(8, 13, 21, 0.95)).Children(func() {
				if a.appIcon != nil {
					ui.Image(c, a.appIcon).Fill().Fit(ui.Cover)
					return
				}
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "MUSAIK").FontSize(18).FontWeight(700).TextColor(sonoraText)
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

func (a *app) albumRepresentativeSongs() []model.Song {
	out := make([]model.Song, 0, len(a.store.Songs))
	seen := map[string]bool{}
	songs := a.filteredSongs(a.store.Songs)
	for _, s := range songs {
		k := strings.ToLower(strings.TrimSpace(s.Artist + "||" + s.Album))
		if k == "||" {
			k = s.ID
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

func (a *app) viewAlbumStrip(c *ui.Context, albums []model.Song, limit int) {
	if len(albums) == 0 {
		return
	}
	if limit > len(albums) {
		limit = len(albums)
	}
	ui.Row(c).Gap(12).Children(func() {
		for i := 0; i < limit; i++ {
			s := albums[i]
			ui.Column(c).Width(132).Gap(5).Children(func() {
				ui.Box(c).Size(132, 86).Radius(8).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
					}
				})
				ui.Text(c, s.Title).SingleLine().FontSize(12).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, s.Artist).SingleLine().FontSize(11).TextColor(sonoraTextMuted)
			})
		}
	})
}

func (a *app) viewHiResRows(c *ui.Context, albums []model.Song, limit int) {
	if len(albums) == 0 {
		return
	}
	if limit > len(albums) {
		limit = len(albums)
	}
	ui.Row(c).Gap(12).Children(func() {
		for i := 0; i < limit; i++ {
			s := albums[i]
			ui.Row(c).Grow(1).Padding(10, 10).Gap(10).Background(ui.RGBA(255, 255, 255, 0.04)).Border(1, sonoraBorder).Radius(12).Children(func() {
				ui.Box(c).Size(84, 84).Radius(8).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
					}
				})
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					ui.Text(c, s.Album).SingleLine().TextColor(sonoraText)
					ui.Text(c, s.Artist).SingleLine().TextColor(sonoraTextMuted)
					ui.Text(c, "HI-RES / LOSSLESS").Font("monospace").FontSize(11).TextColor(sonoraTertiary)
				})
				ui.Button(c, "▶").Width(34)
			})
		}
	})
}

func totalLibrarySize(songs []model.Song, a *app) string {
	var total int64
	for _, song := range songs {
		total += a.songFileSize(song)
	}
	if total <= 0 {
		return "--"
	}
	return formatFileSizeMB(total)
}

func (a *app) songFileSize(song model.Song) int64 {
	if v, ok := a.sizeByID[song.ID]; ok {
		if v < 0 {
			return 0
		}
		return v
	}
	info, err := os.Stat(song.Path)
	if err != nil {
		a.sizeByID[song.ID] = -1
		return 0
	}
	a.sizeByID[song.ID] = info.Size()
	return info.Size()
}

func formatFileSizeMB(size int64) string {
	if size <= 0 {
		return "--"
	}
	mb := float64(size) / (1024 * 1024)
	return fmt.Sprintf("%.1f MB", mb)
}

func compactTrackTitle(song model.Song) string {
	if song.TrackNum > 0 {
		return fmt.Sprintf("%02d. %s", song.TrackNum, song.Title)
	}
	return song.Title
}

func compactFormat(song model.Song) string {
	if song.Format == "" {
		return "--"
	}
	return strings.ToUpper(song.Format)
}

func (a *app) viewSongs(c *ui.Context, songs []model.Song) {
	if len(songs) == 0 {
		ui.Row(c).
			Padding(14, 16).
			Background(ui.RGBA(255, 255, 255, 0.03)).
			Grow(1).
			Children(func() {
				ui.Text(c, "No hay canciones").TextColor(sonoraTextMuted)
			})
		return
	}

	colArtist := float32(126)
	colAlbum := float32(126)
	colPlays := float32(78)
	colFormat := float32(84)
	colDuration := float32(64)
	colSize := float32(86)
	colLike := float32(46)

	ui.Column(c).Grow(1).Background(ui.Hex("#1F1F21")).Children(func() {
		ui.Row(c).
			Height(36).
			Padding(8, 12).
			AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).
			BorderColor(ui.RGBA(255, 255, 255, 0.08)).
			TextColor(ui.RGBA(235, 235, 239, 0.56)).
			Children(func() {
				ui.Text(c, "Nombre de Pista").Grow(1).FontSize(12)
				ui.Text(c, "Artista").Width(colArtist).FontSize(12)
				ui.Text(c, "Álbum").Width(colAlbum).FontSize(12)
				ui.Text(c, "Repr.").Width(colPlays).FontSize(12)
				ui.Text(c, "Formato").Width(colFormat).FontSize(12)
				ui.Text(c, "Dura...").Width(colDuration).FontSize(12)
				ui.Text(c, "Tamaño").Width(colSize).FontSize(12)
				ui.Text(c, "♥").Width(colLike).FontSize(12)
			})

		ui.Scroll(c.Key("songs-rows")).Grow(1).Children(func() {
			for _, song := range songs {
				songCopy := song
				selected := songCopy.ID == a.selectedID
				rowBG := ui.RGBA(255, 255, 255, 0)
				primary := ui.RGBA(235, 235, 239, 0.88)
				secondary := ui.RGBA(235, 235, 239, 0.56)
				if selected {
					rowBG = ui.Hex("#1660C8")
					primary = ui.Hex("#F5F8FF")
					secondary = ui.RGBA(245, 248, 255, 0.88)
				}
				row := ui.Row(c).
					Height(42).
					Padding(8, 12).
					Gap(10).
					AlignItems(ui.Center).
					Background(rowBG).
					BorderWidth(0, 0, 1, 0).
					BorderColor(ui.RGBA(255, 255, 255, 0.04))
				row.ContextMenu(func(m *ui.Menu) {
					selectedSong := songCopy
					menuPlaylist := m.Submenu("Agregar a playlist", func(pm *ui.Menu) {
						anyManual := false
						for _, pl := range a.store.Playlists {
							if pl.Smart != nil || pl.Name == "Todos" {
								pm.Item(pl.Name + " (smart)").Disabled(true)
								continue
							}
							anyManual = true
							item := pm.Item(pl.Name)
							if item.Chosen() {
								if err := a.store.AddSongToManualPlaylist(pl.Name, selectedSong.ID); err != nil {
									a.status = "No se pudo agregar a playlist: " + err.Error()
								} else {
									_ = a.store.SaveState()
									a.status = fmt.Sprintf("%s agregado a playlist %s", selectedSong.Title, pl.Name)
								}
							}
						}
						if !anyManual {
							pm.Separator()
							pm.Item("No hay playlists manuales").Disabled(true)
						}
					})
					menuPlaylist.Disabled(false)

					lastfmItem := m.Item("Actualizar conteo desde Last.fm")
					lastfmItem.Disabled(a.lastfm == nil || !a.lastfm.Enabled())
					if lastfmItem.Chosen() {
						a.syncSongPlayCountFromLastFM(selectedSong)
					}
				})
				if !selected && row.Hovered() {
					row.Background(ui.RGBA(255, 255, 255, 0.05))
				}
				row.Children(func() {
					ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
						noteColor := ui.RGBA(59, 181, 255, 0.95)
						if selected {
							noteColor = ui.Hex("#D7E8FF")
						}
						ui.Text(c, "♪").FontSize(12).TextColor(noteColor)
						ui.Text(c, compactTrackTitle(songCopy)).Grow(1).SingleLine().TextColor(primary)
						if selected {
							ui.Text(c, "REPRODUCIENDO").
								FontSize(10).
								FontWeight(700).
								Padding(2, 6).
								Radius(6).
								Background(ui.RGBA(255, 255, 255, 0.20)).
								TextColor(ui.Hex("#EAF3FF"))
						}
					})
					ui.Text(c, songCopy.Artist).Width(colArtist).SingleLine().TextColor(secondary)
					ui.Text(c, songCopy.Album).Width(colAlbum).SingleLine().TextColor(secondary)
					ui.Textf(c, "%d", songCopy.PlayCount).Width(colPlays).SingleLine().Font("monospace").TextColor(secondary)
					ui.Text(c, compactFormat(songCopy)).Width(colFormat).SingleLine().TextColor(secondary)
					ui.Text(c, formatDuration(songCopy.Duration)).Width(colDuration).SingleLine().Font("monospace").TextColor(secondary)
					ui.Text(c, formatFileSizeMB(a.songFileSize(songCopy))).Width(colSize).SingleLine().Font("monospace").TextColor(secondary)
					like := "♡"
					if a.store.Favorites[songCopy.ID] {
						like = "♥"
					}
					ui.Text(c, like).Width(colLike).SingleLine().TextColor(secondary)
				})
				if row.Clicked() {
					if a.selectedID == songCopy.ID {
						a.playSongByID(songCopy.ID)
					}
					a.selectedID = songCopy.ID
					if idx, _, ok := a.findSong(songCopy.ID); ok {
						a.playIndex = idx
					}
				}
			}

			ghostRows := max(0, 10-len(songs))
			for i := 0; i < ghostRows; i++ {
				ui.Row(c).
					Height(42).
					Padding(8, 12).
					Background(ui.RGBA(255, 255, 255, 0.01)).
					BorderWidth(0, 0, 1, 0).
					BorderColor(ui.RGBA(255, 255, 255, 0.03))
			}
		})
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

func albumGroupKey(song model.Song) string {
	if song.Artist != "" || song.Album != "" {
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
	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return ui.DecodeBitmap(pic.Data)
	}
	const maxCoverSize = 192
	scaled := downscaleImage(img, maxCoverSize)
	return ui.NewBitmap(scaled), nil
}

func downscaleImage(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 || maxDim <= 0 {
		return src
	}
	if w <= maxDim && h <= maxDim {
		return src
	}

	var nw, nh int
	if w >= h {
		nw = maxDim
		nh = int(float64(h) * (float64(maxDim) / float64(w)))
	} else {
		nh = maxDim
		nw = int(float64(w) * (float64(maxDim) / float64(h)))
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + int(float64(y)*float64(h)/float64(nh))
		if sy >= b.Max.Y {
			sy = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)*float64(w)/float64(nw))
			if sx >= b.Max.X {
				sx = b.Max.X - 1
			}
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func (a *app) cacheCover(key string, bmp *ui.Bitmap) {
	if _, ok := a.covers[key]; ok {
		return
	}
	a.covers[key] = bmp
	a.coverLRU = append(a.coverLRU, key)
	const maxCovers = 80
	if len(a.coverLRU) <= maxCovers {
		return
	}
	evict := a.coverLRU[0]
	a.coverLRU = a.coverLRU[1:]
	delete(a.covers, evict)
}

func (a *app) viewControls(c *ui.Context) {
	state := a.player.State()
	playIcon := iconPlay
	if state.Playing && !state.Paused {
		playIcon = iconPause
	}

	duration := state.Duration
	position := state.Position
	_, currentSong, hasCurrent := a.findSong(a.selectedID)
	if duration > 0 {
		a.sliderAt = position.Seconds()
	} else {
		a.sliderAt = 0
	}

	ui.Row(c).
		Padding(10, 14).
		Gap(12).
		Background(ui.Hex("#242427")).
		BorderWidth(1, 0, 0, 0).
		BorderColor(ui.RGBA(255, 255, 255, 0.10)).
		AlignItems(ui.Center).
		Children(func() {
			ui.Row(c).Width(300).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(40, 40).Radius(4).Background(ui.RGBA(255, 255, 255, 0.10)).Children(func() {
					if hasCurrent {
						if bmp := a.coverBitmap(currentSong); bmp != nil {
							ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(4)
							return
						}
					}
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "RAM").FontSize(11).FontWeight(700).TextColor(ui.Hex("#F2C94C"))
					})
				})
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					title := "Sin reproducción"
					subtitle := "Musaik"
					quality := ""
					liked := false
					if hasCurrent {
						title = currentSong.Title
						subtitle = fmt.Sprintf("%s — %s", currentSong.Artist, currentSong.Album)
						quality = fmt.Sprintf("%s  /  %s", strings.ToUpper(currentSong.Format), "Lossless")
						liked = a.store.Favorites[currentSong.ID]
					}
					ui.Row(c).Gap(6).Children(func() {
						ui.Text(c, title).SingleLine().FontSize(15).FontWeight(600).TextColor(ui.Hex("#F4F4F6"))
						if hasCurrent {
							ui.Text(c, "Hi-Res").FontSize(10).Padding(1, 5).Radius(5).Background(ui.RGBA(30, 111, 232, 0.25)).TextColor(ui.Hex("#9FD0FF"))
						}
						likeBtn := ui.Button(c, "").Width(22).Height(22).Radius(11).Background(ui.Transparent).TextColor(ui.RGBA(235, 235, 239, 0.86))
						if liked {
							likeBtn.TextColor(ui.Hex("#F43F5E"))
						}
						likeBtn.Children(func() {
							ui.Column(c).Fill().Center().Children(func() {
								if liked {
									ui.Text(c, "♥").FontSize(12)
									return
								}
								ui.Text(c, "♡").FontSize(12)
							})
						})
						if likeBtn.Clicked() && hasCurrent {
							a.toggleFavorite(currentSong)
						}
					})
					ui.Text(c, subtitle).SingleLine().FontSize(12).TextColor(ui.RGBA(235, 235, 239, 0.56))
					ui.Text(c, quality).SingleLine().FontSize(11).TextColor(ui.RGBA(235, 235, 239, 0.42))
				})
			})

			ui.Spacer(c)

			center := ui.Column(c).Width(470).Gap(5)
			center.Children(func() {
				ui.Row(c).Center().Gap(16).Children(func() {
					shuffleBG := ui.Transparent
					shuffleColor := ui.RGBA(235, 235, 239, 0.72)
					if a.shuffle {
						shuffleBG = ui.RGBA(255, 255, 255, 0.12)
						shuffleColor = ui.Hex("#EAF3FF")
					}
					shuffleBtn := ui.Button(c, "").Width(24).Height(24).Radius(12).Background(shuffleBG).TextColor(shuffleColor).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Icon(c, iconShuffle).Size(13, 13)
						})
					})
					if shuffleBtn.Clicked() {
						a.shuffle = !a.shuffle
						_ = a.saveSettings()
					}

					prevBtn := ui.Button(c, "").Width(24).Height(24).Radius(12).Background(ui.Transparent).TextColor(ui.RGBA(235, 235, 239, 0.72)).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Icon(c, iconPrev).Size(13, 13)
						})
					})
					if prevBtn.Clicked() {
						a.playPrevious()
					}

					playBtn := ui.Button(c, "").Width(34).Height(34).Radius(17).Background(ui.Hex("#F3F3F3")).TextColor(ui.Hex("#1F1F22")).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Icon(c, playIcon).Size(14, 14)
						})
					})
					if playBtn.Clicked() {
						if state.Playing {
							if err := a.player.TogglePause(); err != nil {
								a.status = "No se puede pausar/reanudar: " + err.Error()
							}
						} else {
							a.playSelectedOrFirst()
						}
					}

					nextBtn := ui.Button(c, "").Width(24).Height(24).Radius(12).Background(ui.Transparent).TextColor(ui.RGBA(235, 235, 239, 0.72)).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Icon(c, iconNext).Size(13, 13)
						})
					})
					if nextBtn.Clicked() {
						a.playNext()
					}

					repeatBG := ui.Transparent
					repeatColor := ui.RGBA(235, 235, 239, 0.72)
					if a.repeat {
						repeatBG = ui.RGBA(255, 255, 255, 0.12)
						repeatColor = ui.Hex("#EAF3FF")
					}
					repeatBtn := ui.Button(c, "").Width(24).Height(24).Radius(12).Background(repeatBG).TextColor(repeatColor).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Icon(c, iconRepeat).Size(13, 13)
						})
					})
					if repeatBtn.Clicked() {
						a.repeat = !a.repeat
						_ = a.saveSettings()
					}
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, formatDuration(position)).Font("monospace").FontSize(12).TextColor(ui.RGBA(235, 235, 239, 0.56))
					if duration > 0 {
						if ui.Slider(c.Key("progress"), &a.sliderAt, 0, duration.Seconds()).Step(1).Grow(1).Changed() {
							if err := a.player.SeekTo(time.Duration(a.sliderAt * float64(time.Second))); err != nil {
								a.status = "No se pudo mover la canción: " + err.Error()
							}
						}
					} else {
						ui.Box(c).Grow(1)
					}
					rem := duration - position
					if rem < 0 {
						rem = 0
					}
					ui.Text(c, "-"+formatDuration(rem)).Font("monospace").FontSize(12).TextColor(ui.RGBA(235, 235, 239, 0.56))
				})
			})

			ui.Spacer(c)
			ui.Row(c).Width(220).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "🎧 AudioQuest DAC").SingleLine().FontSize(12).TextColor(ui.RGBA(235, 235, 239, 0.64))
				ui.Button(c, "").Width(18).Height(18).Radius(9).Background(ui.Transparent).TextColor(ui.RGBA(235, 235, 239, 0.48)).Children(func() {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "◂").FontSize(10)
					})
				})
				ui.Icon(c, iconVolume).Size(13, 13).TextColor(ui.RGBA(235, 235, 239, 0.56))
				if ui.Slider(c.Key("volume"), &a.volumeLevel, 0, 100).Step(1).Grow(1).Changed() {
					if err := a.player.SetVolume(int(a.volumeLevel)); err != nil {
						a.status = "No se pudo ajustar volumen: " + err.Error()
					}
				}
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

func formatDurationLong(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	total := int(d.Round(time.Second).Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
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

func (a *app) selectedSongForMenu(songs []model.Song, selected int) (model.Song, bool) {
	if selected >= 0 && selected < len(songs) {
		return songs[selected], true
	}
	if a.selectedID != "" {
		for _, s := range songs {
			if s.ID == a.selectedID {
				return s, true
			}
		}
	}
	return model.Song{}, false
}

func (a *app) syncSongPlayCountFromLastFM(song model.Song) {
	if a.lastfm == nil || !a.lastfm.Enabled() {
		a.status = "Last.fm no está conectado"
		return
	}
	a.status = "Consultando reproducciones en Last.fm…"
	go func(song model.Song) {
		count, err := a.lastfm.UserPlayCount(song)
		if err != nil {
			a.updateUI(func() {
				a.status = "No se pudo leer conteo de Last.fm: " + err.Error()
			})
			return
		}
		a.updateUI(func() {
			if !a.store.SetPlayCount(song.ID, count) {
				a.status = "Canción no encontrada para actualizar conteo"
				return
			}
			_ = a.store.SaveState()
			a.status = fmt.Sprintf("Conteo actualizado desde Last.fm: %s (%d)", song.Title, count)
		})
	}(song)
}

func (a *app) toggleFavorite(song model.Song) {
	wasFavorite := a.store.Favorites[song.ID]
	a.store.ToggleFavorite(song.ID)
	_ = a.store.SaveState()

	if wasFavorite {
		a.status = fmt.Sprintf("Quitaste like: %s", song.Title)
		return
	}

	a.status = fmt.Sprintf("Like guardado: %s", song.Title)
	if a.lastfm == nil || !a.lastfm.Enabled() {
		return
	}
	go func(song model.Song) {
		if err := a.lastfm.LoveTrack(song); err != nil {
			a.updateUI(func() {
				a.status = "Like local guardado, pero falló Last.fm: " + err.Error()
			})
			return
		}
		a.updateUI(func() {
			a.status = fmt.Sprintf("Like enviado a Last.fm: %s", song.Title)
		})
	}(song)
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
	now := time.Now()
	if count, ok := a.store.IncrementPlayCount(song.ID); ok {
		_ = a.store.MarkPlayedAt(song.ID, now)
		_ = a.store.SaveState()
		song.PlayCount = count
		song.LastPlayedAt = now
	}
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
			Title:          "Musaik",
			Width:          1100,
			Height:         780,
			MinWidth:       800,
			MinHeight:      520,
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: 54,
			Vibrancy:       mygo.VibrancySidebar,
			StateKey:       "main",
			Content:        ui.View(a.view),
		})
		a.window = win
		win.OnClose(func(e *mygo.CloseEvent) {
			a.player.Stop()
			a.persistAll()
		})
		win.OnClosed(func() {
			a.player.Stop()
		})
		a.startUIHeartbeat()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
