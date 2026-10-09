package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
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
	sectionExplore   section = "Explorar"
	sectionRecent    section = "Recientes"
	sectionLibrary   section = "Librería"
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

	lastfmAPIKey     string
	lastfmAPISecret  string
	lastfmSessionKey string
	lastfmUsername   string
	lastfmAuthBusy   bool
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
}

func newApp() *app {
	a := &app{
		store:            library.NewStore(),
		player:           player.New(),
		section:          sectionExplore,
		rootInput:        defaultMusicDir(),
		searchQuery:      "",
		status:           "Listo para indexar música",
		selectedID:       "",
		playIndex:        -1,
		covers:           map[string]*ui.Bitmap{},
		coverLRU:         []string{},
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
	if len(a.store.Songs) > 0 {
		a.status = fmt.Sprintf("Biblioteca cargada: %d canciones", len(a.store.Songs))
	}
	a.refreshLastFMClient()
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
	case sectionExplore, sectionRecent, sectionLibrary, sectionArtists, sectionAlbums, sectionFavorites, sectionPlaylists, sectionSettings:
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
	ui.Column(c).Fill().Background(sonoraCanvas).Children(func() {
		a.viewTopBar(c)
		ui.Row(c).Grow(1).Padding(16).Gap(14).AlignItems(ui.Stretch).Children(func() {
			a.viewSidebar(c)
			ui.Column(c).Grow(1).Gap(12).Children(func() {
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
			ui.Row(c).Gap(6).Children(func() {
				ui.Button(c, "‹").Width(28)
				ui.Button(c, "›").Width(28)
			})
			ui.Box(c).Size(24, 24).Radius(6).Background(ui.RGBA(244, 63, 94, 0.35)).Children(func() {
				ui.Column(c).Fill().Center().Children(func() { ui.Text(c, "♪").FontSize(10) })
			})
			ui.Text(c, "Musaik").FontSize(28).Bold().TextColor(sonoraText)
			ui.TextInput(c, &a.searchQuery).
				Placeholder("Buscar en la biblioteca").
				Width(360)
			if strings.TrimSpace(a.searchQuery) != "" {
				if ui.Button(c, "Limpiar").Clicked() {
					a.searchQuery = ""
					_ = a.saveSettings()
				}
			}
			ui.Spacer(c)
			ui.Text(c, "Chord Hugo 2 DAC").FontSize(12).TextColor(sonoraTextMuted)
			ui.Text(c, "▮▮").TextColor(sonoraTextMuted)
			ui.Box(c).Size(24, 24).Radius(12).Background(ui.RGBA(255, 255, 255, 0.12))
		})
}

func (a *app) viewSidebar(c *ui.Context) {
	ui.Column(c).
		Width(172).
		Padding(12, 8).
		Gap(8).
		Background(sonoraSidebar).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Text(c, "Biblioteca").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			a.sidebarNavButton(c, sectionExplore, "◉", "Explorar")
			a.sidebarNavButton(c, sectionRecent, "◔", "Recientes")
			a.sidebarNavButton(c, sectionLibrary, "♪", "Canciones")
			ui.Box(c).Height(6)
			ui.Text(c, "Colección").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			a.sidebarNavButton(c, sectionAlbums, "◈", "Álbumes")
			a.sidebarNavButton(c, sectionArtists, "◉", "Artistas")
			a.sidebarNavButton(c, sectionFavorites, "♡", "Favoritos")
			a.sidebarNavButton(c, sectionPlaylists, "≡", "Playlists")
			ui.Box(c).Height(8)
			ui.Text(c, "Playlists").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			a.sidebarPlaylist(c, "≣", "Descubrimiento Audiófilo")
			a.sidebarPlaylist(c, "≣", "Late Night Lo-Fi")
			a.sidebarPlaylist(c, "≣", "Acoustic Masterpieces")
			a.sidebarPlaylist(c, "≣", "Synthwave Horizons")
			ui.Box(c).Height(8)
			ui.Text(c, "Sistema").FontSize(11).FontWeight(600).TextColor(sonoraTextTertiary)
			a.sidebarNavButton(c, sectionSettings, "⚙", "Ajustes")
			ui.Spacer(c)
			ui.Row(c).Gap(8).Children(func() {
				ui.Box(c).Size(28, 28).Radius(8).Background(ui.RGBA(6, 182, 212, 0.2)).Children(func() {
					ui.Column(c).Fill().Center().Children(func() { ui.Text(c, "≋").FontSize(10).TextColor(sonoraTertiary) })
				})
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, "Audio engine").FontSize(11).TextColor(sonoraTertiary)
					ui.Text(c, "Bit-Perfect 192kHz").FontSize(10).TextColor(sonoraTextMuted)
				})
			})
		})
}

func (a *app) sidebarNavButton(c *ui.Context, sec section, icon, label string) {
	btn := ui.Button(c, icon+" "+label)
	if a.section == sec {
		btn.Background(ui.RGBA(244, 63, 94, 0.14)).TextColor(sonoraText)
	} else {
		btn.Background(ui.RGBA(255, 255, 255, 0.00)).TextColor(sonoraTextMuted)
	}
	if btn.Clicked() {
		a.section = sec
		_ = a.saveSettings()
	}
}

func (a *app) sidebarPlaylist(c *ui.Context, icon, label string) {
	ui.Button(c, icon+" "+label).Background(ui.RGBA(255, 255, 255, 0.00)).TextColor(sonoraTextMuted)
}

func (a *app) viewMain(c *ui.Context) {
	ui.Column(c).Grow(1).Gap(12).Children(func() {
		switch a.section {
		case sectionExplore, sectionRecent:
			a.viewExplore(c)
		case sectionLibrary:
			a.viewHero(c)
			a.viewSongs(c, a.filteredSongs(a.store.Songs))
		case sectionArtists:
			a.viewHero(c)
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
			a.viewHero(c)
			a.viewAlbumsGrid(c)
		case sectionFavorites:
			a.viewHero(c)
			favs := make([]model.Song, 0, len(a.store.Songs))
			for _, song := range a.store.Songs {
				if a.store.Favorites[song.ID] {
					favs = append(favs, song)
				}
			}
			a.viewSongs(c, a.filteredSongs(favs))
		case sectionPlaylists:
			a.viewHero(c)
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
					_ = a.store.SaveState()
				}
			})
		case sectionSettings:
			a.viewSettings(c)
		}
	})
}

func (a *app) viewExplore(c *ui.Context) {
	ui.Scroll(c).Grow(1).Gap(12).Children(func() {
		a.viewHero(c)
		ui.Row(c).Gap(8).Children(func() {
			chips := []string{"Todos", "Electrónica", "Ambient", "Post-Rock", "Jazz", "Clásica"}
			for i, chip := range chips {
				btn := ui.Button(c, chip)
				if i == 0 {
					btn.Background(ui.RGBA(244, 63, 94, 0.18)).TextColor(sonoraText)
				} else {
					btn.Background(ui.RGBA(255, 255, 255, 0.05)).TextColor(sonoraTextMuted)
				}
			}
		})

		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "Añadidos recientemente a tu biblioteca local").FontSize(40).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Archivos de almacenamiento interno bit-perfect y sincronizados").TextColor(sonoraTextMuted)
			})
			ui.Text(c, "Ver todos los álbumes →").TextColor(sonoraAccent)
		})

		a.viewAlbumStrip(c, a.albumRepresentativeSongs(), 6)

		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "Álbumes en alta resolución").FontSize(34).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Remasterizaciones certificadas y grabaciones directas sin pérdida").TextColor(sonoraTextMuted)
			})
			ui.Text(c, "● DAC ACTIVO: BIT-PERFECT").Font("monospace").TextColor(sonoraTertiary)
		})
		a.viewHiResRows(c, a.albumRepresentativeSongs(), 3)

		ui.Row(c).Padding(12, 14).Background(ui.RGBA(255, 255, 255, 0.04)).Border(1, sonoraBorder).Radius(12).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "Ruta de señal directa (Bit-Perfect mode)").FontWeight(700)
				ui.Text(c, "El mezclador del sistema macOS está en modo de derivación por hardware.").TextColor(sonoraTextMuted)
			})
			ui.Button(c, "Configurar DSP")
		})
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

	ui.GridView(c, nil, len(albums), 170, 232, func(i int) {
		s := albums[i]
		ui.Column(c).
			Padding(8, 8).
			Gap(8).
			Background(ui.RGBA(255, 255, 255, 0.03)).
			Border(1, sonoraBorder).
			Radius(10).
			Children(func() {
				ui.Box(c).Size(160, 160).Radius(10).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(10)
					}
				})
				ui.Text(c, s.Album).SingleLine().TextColor(sonoraText)
				ui.Text(c, s.Artist).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
			})
	}).Grow(1)
}

func (a *app) viewSettings(c *ui.Context) {
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
			ui.Text(c, a.status).TextColor(sonoraTextMuted)
		})
}

func (a *app) indexLibrary() {
	if err := a.store.Index(a.rootInput); err != nil {
		a.status = "Error indexando: " + err.Error()
		return
	}
	a.covers = map[string]*ui.Bitmap{}
	a.coverLRU = nil
	a.status = fmt.Sprintf("Indexadas %d canciones", len(a.store.Songs))
	if len(a.store.Songs) > 0 {
		a.selectedID = a.store.Songs[0].ID
	}
	a.persistAll()
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
			ui.Box(c).Size(280, 220).Background(ui.RGBA(244, 63, 94, 0.16)).Radius(12).Children(func() {
				if hasSelected {
					if bmp := a.coverBitmap(selectedSong); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(12)
						return
					}
				}
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "MUSAIK").FontSize(16).FontWeight(700).TextColor(sonoraText)
				})
			})
			ui.Column(c).Grow(1).Gap(8).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "ÁLBUM DEL DÍA").FontSize(11).FontWeight(600).TextColor(sonoraTextMuted)
					ui.Text(c, "24-BIT / 96KHZ FLAC").FontSize(11).Font("monospace").TextColor(sonoraTertiary)
					ui.Text(c, "MASTER DIRECTO").FontSize(11).TextColor(sonoraTextTertiary)
				})
				ui.Text(c, title).FontSize(52).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, subtitle).FontSize(14).TextColor(sonoraTextMuted)
				ui.Row(c).Gap(10).Children(func() {
					if ui.PrimaryButton(c, "▶ Reproducir Álbum").Clicked() {
						a.playSelectedOrFirst()
					}
					if ui.Button(c, "⤮ Aleatorio").Clicked() {
						a.shuffle = !a.shuffle
					}
					if hasSelected && ui.Button(c, "♡ Like").Clicked() {
						a.store.ToggleFavorite(selectedSong.ID)
						_ = a.store.SaveState()
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

func (a *app) albumRepresentativeSongs() []model.Song {
	out := make([]model.Song, 0, 24)
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
		if len(out) >= 24 {
			break
		}
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
			ui.Column(c).Width(170).Gap(6).Children(func() {
				ui.Box(c).Size(170, 170).Radius(10).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(10)
					}
				})
				ui.Text(c, s.Album).SingleLine().TextColor(sonoraText)
				ui.Text(c, s.Artist).SingleLine().TextColor(sonoraTextMuted)
				ui.Text(c, strings.ToUpper(s.Format)).Font("monospace").FontSize(11).TextColor(sonoraTertiary)
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
						_ = a.store.SaveState()
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
					_ = a.saveSettings()
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
					_ = a.saveSettings()
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
