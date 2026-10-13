package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
	sectionQueue     section = "Queue"
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
	iconHome      = mustIconoir("home-simple.svg")
	iconList      = mustIconoir("list.svg")
	iconProfile   = mustIconoir("profile-circle.svg")
	iconAlbum     = mustIconoir("album.svg")
	iconMusic     = mustIconoir("music-note.svg")
	iconPlaylist  = mustIconoir("playlist.svg")
	iconPlus      = mustIconoir("plus.svg")
	iconHeart     = mustIconoir("heart.svg")
	iconClock     = mustIconoir("clock.svg")
	iconBell      = mustIconoir("bell.svg")
	iconPlay      = mustIconoir("play.svg")
	iconPause     = mustIconoir("pause.svg")
	iconPrev      = mustIconoir("fast-arrow-left.svg")
	iconNext      = mustIconoir("fast-arrow-right.svg")
	iconRepeat    = mustIconoir("repeat.svg")
	iconRepeat1   = mustIconoir("repeat-once.svg")
	iconVolume    = mustIconoir("sound-high.svg")
	iconVolumeOff = mustIconoir("sound-off.svg")
	iconShuffle   = mustIconoir("shuffle.svg")
)

type repeatMode string

const (
	repeatModeOff repeatMode = "off"
	repeatModeAll repeatMode = "all"
	repeatModeOne repeatMode = "one"
)

type app struct {
	store             *library.Store
	player            *player.Player
	lastfm            *lastfm.Client
	window            *mygo.Window
	settingsWindow    *mygo.Window
	appIcon           *ui.Bitmap
	section           section
	sliderAt          float64
	covers            map[string]*ui.Bitmap
	coverLRU          []string
	sizeByID          map[string]int64
	artistAvatars     map[string]*ui.Bitmap
	artistAvatarLRU   []string
	artistAvatarFetch map[string]bool

	libraryTable ui.ListState

	rootInput                 string
	searchQuery               string
	actionQuery               string
	actionPrevQ               string
	actionOpen                bool
	actionIndex               int
	actionAddToPlaylistOpen   bool
	actionAddSongID           string
	actionAddPlaylistName     string
	queueOpen                 bool
	queueSongIDs              []string
	playlistCreateOpen        bool
	playlistCreateName        string
	playlistCreateSeedSongID  string
	playlistCreateColor       string
	playlistRenameOpen        bool
	playlistRenameOriginal    string
	playlistRenameName        string
	playlistDeleteConfirmOpen bool
	playlistDeleteTarget      string
	playlistsOpen             bool
	selectedPlaylistName      string
	status                    string
	selectedID                string
	selectedArtist            string
	artistDetailOn            bool
	selectedAlbum             string
	albumDetailOn             bool
	volumeLevel               float64
	volumeBeforeMute          float64
	playIndex                 int
	albumPage                 int
	songsPage                 int
	albumSort                 string
	settingsTab               settingsTab
	shuffle                   bool
	repeatMode                repeatMode

	lastfmAPIKey     string
	lastfmAPISecret  string
	lastfmSessionKey string
	lastfmUsername   string
	lastfmAuthBusy   bool

	ipodMountPath    string
	ipodPlaylistName string
	ipodSyncBusy     bool
}

type draggedSong struct {
	SongID string
}

type settingsTab string

const (
	settingsTabGeneral      settingsTab = "general"
	settingsTabAppearance   settingsTab = "appearance"
	settingsTabLibrary      settingsTab = "library"
	settingsTabIntegrations settingsTab = "integrations"
	settingsTabAbout        settingsTab = "about"
)

type appSettings struct {
	Section          string `json:"section"`
	Shuffle          bool   `json:"shuffle"`
	Repeat           bool   `json:"repeat"`
	RepeatMode       string `json:"repeat_mode"`
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
		store:               library.NewStore(),
		player:              player.New(),
		appIcon:             appIcon,
		section:             sectionSongs,
		rootInput:           defaultMusicDir(),
		searchQuery:         "",
		status:              "Listo para indexar música",
		selectedID:          "",
		playIndex:           -1,
		volumeLevel:         80,
		volumeBeforeMute:    80,
		albumPage:           0,
		songsPage:           0,
		albumSort:           "Artista",
		playlistsOpen:       true,
		settingsTab:         settingsTabLibrary,
		playlistCreateColor: "none",
		repeatMode:          repeatModeOff,
		covers:              map[string]*ui.Bitmap{},
		coverLRU:            []string{},
		sizeByID:            map[string]int64{},
		artistAvatars:       map[string]*ui.Bitmap{},
		artistAvatarLRU:     []string{},
		artistAvatarFetch:   map[string]bool{},
		lastfmAPIKey:        os.Getenv("LASTFM_API_KEY"),
		lastfmAPISecret:     os.Getenv("LASTFM_API_SECRET"),
		lastfmSessionKey:    os.Getenv("LASTFM_SESSION_KEY"),
		lastfmUsername:      os.Getenv("LASTFM_USERNAME"),
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
	if len(a.store.Playlists) > 0 {
		for _, pl := range a.store.Playlists {
			if pl.Name == "Todos" {
				continue
			}
			a.selectedPlaylistName = pl.Name
			break
		}
		if a.selectedPlaylistName == "" {
			a.selectedPlaylistName = a.store.Playlists[0].Name
		}
	}
	a.refreshLastFMClient()
	a.volumeLevel = float64(a.player.Volume())
	if a.volumeLevel > 0 {
		a.volumeBeforeMute = a.volumeLevel
	}
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
		Repeat:           a.repeatMode != repeatModeOff,
		RepeatMode:       string(a.repeatMode),
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
	case sectionExplore, sectionRecent, sectionLibrary, sectionSongs, sectionArtists, sectionAlbums, sectionQueue, sectionFavorites, sectionPlaylists, sectionSettings:
		a.section = section(s.Section)
	}
	a.shuffle = s.Shuffle
	switch repeatMode(s.RepeatMode) {
	case repeatModeOff, repeatModeAll, repeatModeOne:
		a.repeatMode = repeatMode(s.RepeatMode)
	default:
		if s.Repeat {
			a.repeatMode = repeatModeAll
		} else {
			a.repeatMode = repeatModeOff
		}
	}
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
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() {
		a.openSettingsWindow()
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if a.actionOpen {
			a.actionOpen = false
		}
	})
	rootBg := sonoraCanvas
	if c.Vibrancy() {
		rootBg = ui.Transparent
	}
	ui.Column(c).Fill().Background(rootBg).Children(func() {
		a.viewTopBar(c)
		ui.Row(c).Grow(1).Gap(0).AlignItems(ui.Stretch).Children(func() {
			a.viewSidebar(c)
			ui.Column(c).Grow(1).Background(sonoraCanvas).Children(func() {
				a.viewMain(c)
			})
		})
		a.viewControls(c)
	})
	a.viewActionBar(c)
	a.viewActionAddToPlaylistModal(c)
	a.viewPlaylistCreateModal(c)
	a.viewPlaylistRenameModal(c)
	a.viewPlaylistDeleteModal(c)
}

func (a *app) applyTheme(c *ui.Context) {
	dark := c.Theme().Dark
	if dark {
		sonoraCanvas = ui.Hex("#121318")
		sonoraToolbar = ui.RGBA(28, 30, 42, 0.85)
		sonoraSidebar = ui.RGBA(22, 24, 34, 0.82)
		sonoraCard = ui.RGBA(30, 32, 44, 0.80)
		sonoraCardAlt = ui.RGBA(26, 29, 41, 0.82)
		sonoraBorder = ui.RGBA(255, 255, 255, 0.08)
		sonoraText = ui.Hex("#F8FAFC")
		sonoraTextMuted = ui.RGBA(248, 250, 252, 0.64)
		sonoraTextTertiary = ui.RGBA(248, 250, 252, 0.38)
		sonoraAccent = ui.Hex("#F43F5E")
		sonoraSecondary = ui.Hex("#8B5CF6")
		sonoraTertiary = ui.Hex("#06B6D4")
	} else {
		sonoraCanvas = ui.Hex("#F2F4F8")
		sonoraToolbar = ui.RGBA(247, 249, 253, 0.88)
		sonoraSidebar = ui.RGBA(241, 244, 248, 0.90)
		sonoraCard = ui.RGBA(255, 255, 255, 0.94)
		sonoraCardAlt = ui.RGBA(252, 253, 255, 0.94)
		sonoraBorder = ui.RGBA(16, 24, 40, 0.12)
		sonoraText = ui.Hex("#1A1F2B")
		sonoraTextMuted = ui.RGBA(26, 31, 43, 0.66)
		sonoraTextTertiary = ui.RGBA(26, 31, 43, 0.48)
		sonoraAccent = ui.Hex("#D63A57")
		sonoraSecondary = ui.Hex("#6C4DDC")
		sonoraTertiary = ui.Hex("#0C8E9B")
	}

	t := *c.Theme()
	t.Dark = dark
	t.Background = sonoraCanvas
	t.Surface = sonoraCard
	t.SurfaceHover = sonoraCard.Mix(sonoraText, 0.05)
	t.SurfacePressed = sonoraCard.Mix(sonoraText, 0.10)
	t.Border = sonoraBorder
	t.Text = sonoraText
	t.TextMuted = sonoraTextMuted
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

func adaptiveOverlay(c *ui.Context, darkAlpha, lightAlpha float32) ui.Color {
	if c.Theme().Dark {
		return ui.RGBA(255, 255, 255, darkAlpha)
	}
	return ui.RGBA(0, 0, 0, lightAlpha)
}

func (a *app) viewTopBar(c *ui.Context) {
	bar := c.TitleBar()
	barHeight := float32(54)
	if bar.Height > barHeight {
		barHeight = bar.Height
	}
	topBarBg := ui.Hex("#222225")
	if c.Vibrancy() {
		if c.Theme().Dark {
			topBarBg = ui.RGBA(34, 34, 37, 0.72)
		} else {
			topBarBg = ui.RGBA(248, 249, 253, 0.72)
		}
	} else {
		topBarBg = sonoraToolbar
	}
	ui.Row(c).
		Height(barHeight).
		Padding(10, max(12.0, bar.Right+12), 10, max(12.0, bar.Left+12)).
		Background(topBarBg).
		BorderWidth(0, 0, 1, 0).
		BorderColor(sonoraBorder).
		AlignItems(ui.Center).
		DragWindow().
		Gap(10).
		Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(20, 20).Radius(4).Background(adaptiveOverlay(c, 0.08, 0.08)).Children(func() {
					if a.appIcon != nil {
						ui.Image(c, a.appIcon).Fill().Fit(ui.Cover).Radius(4)
						return
					}
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "♪").FontSize(11).TextColor(sonoraTextMuted)
					})
				})
				ui.Text(c, "Musaik").FontSize(20).FontWeight(700).TextColor(sonoraText)
			})
			ui.Spacer(c).DragWindow()
			ui.TextInput(c, &a.searchQuery).
				Placeholder("Buscar pista, artista.").
				Width(220)
			ui.MenuButton(c, "⋯", func(m *ui.Menu) {
				settingsItem := m.Item("Ajustes")
				if settingsItem.Chosen() {
					a.openSettingsWindow()
				}
			})
		})
}

func (a *app) openSettingsWindow() {
	if a.settingsWindow != nil {
		a.settingsWindow.Show()
		a.settingsWindow.Focus()
		return
	}
	parent := a.window
	a.settingsWindow = mygo.NewWindow(mygo.WindowOptions{
		Title:     "Ajustes",
		Width:     820,
		Height:    700,
		MinWidth:  680,
		MinHeight: 520,
		Parent:    parent,
		Modal:     true,
		Vibrancy:  mygo.VibrancyWindow,
		Content:   ui.View(a.viewSettingsModal),
	})
	a.settingsWindow.OnClosed(func() {
		a.settingsWindow = nil
	})
}

func (a *app) closeSettingsWindow() {
	if a.settingsWindow != nil {
		a.settingsWindow.Close()
	}
}

func (a *app) viewSettingsModal(c *ui.Context) {
	a.applyTheme(c)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		a.closeSettingsWindow()
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		a.closeSettingsWindow()
	})
	ui.Column(c).Fill().Background(sonoraCanvas).Children(func() {
		ui.Row(c).
			Height(56).
			Padding(10, 14).
			Gap(12).
			AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).
			BorderColor(sonoraBorder).
			Children(func() {
				closeBtn := ui.Button(c, "✕").
					Width(32).
					Height(32).
					Padding(0, 0).
					Radius(16).
					Background(adaptiveOverlay(c, 0.12, 0.08)).
					TextColor(sonoraTextMuted)
				if closeBtn.Clicked() {
					a.closeSettingsWindow()
				}
				ui.Text(c, "Settings").FontSize(14).FontWeight(700).TextColor(sonoraText)
				ui.Spacer(c)
			})
		ui.Scroll(c).Grow(1).Padding(14, 14).Children(func() {
			a.viewSettings(c)
		})
	})
}

func (a *app) viewSidebar(c *ui.Context) {
	ui.Column(c).
		Width(210).
		Padding(8, 8).
		Gap(6).
		Background(sonoraSidebar).
		BorderWidth(0, 0, 0, 1).
		BorderColor(sonoraBorder).
		Children(func() {
			selected := a.sidebarSelectionID()
			sidebar := ui.Sidebar(c, &selected, func() {
				ui.SidebarSection(c, "Biblioteca", nil, func() {
					ui.SidebarItem(c, "lib:songs", iconMusic, "Canciones")
					ui.SidebarItem(c, "lib:artists", iconProfile, "Artistas")
					ui.SidebarItem(c, "lib:albums", iconAlbum, "Álbumes")
					ui.SidebarItem(c, "lib:queue", iconList, "Queue")
					ui.SidebarItem(c, "lib:favorites", iconHeart, "Favoritos")
					ui.SidebarItem(c, "lib:recent", iconClock, "Recientes")
				})
				ui.SidebarSection(c, "Playlists", &a.playlistsOpen, func() {
					ui.SidebarItem(c, "playlist:new", iconPlus, "Nueva playlist")
					for _, pl := range a.visiblePlaylists() {
						playlistName := pl.Name
						label := playlistName
						if dot := playlistColorDot(pl.Color); dot != "" {
							label = dot + " " + label
						}
						if pl.Smart != nil {
							label = "◌ " + playlistName
						}
						item := ui.SidebarItem(c, "playlist:"+playlistName, iconPlaylist, label)
						if pl.Smart == nil {
							item.ContextMenu(func(m *ui.Menu) {
								renameItem := m.Item("Editar nombre")
								if renameItem.Chosen() {
									a.playlistRenameOpen = true
									a.playlistRenameOriginal = playlistName
									a.playlistRenameName = playlistName
								}
								colorMenu := m.Submenu("Color", func(cm *ui.Menu) {
									for _, color := range playlistColorOptions() {
										colorValue := color
										item := cm.Item(playlistColorLabel(colorValue))
										if item.Chosen() {
											if err := a.setManualPlaylistColor(playlistName, colorValue); err != nil {
												a.status = "No se pudo actualizar color: " + err.Error()
											} else {
												a.status = "Color actualizado para " + playlistName
											}
										}
									}
								})
								colorMenu.Disabled(false)
								deleteItem := m.Item("Eliminar playlist")
								if deleteItem.Chosen() {
									a.playlistDeleteConfirmOpen = true
									a.playlistDeleteTarget = playlistName
								}
							})
							if dragSong, over := ui.DragOver[*draggedSong](item); over {
								if dragSong != nil {
									a.status = "Suelta para agregar a playlist " + playlistName
								}
							}
							if dropped, ok := ui.Drop[*draggedSong](item); ok && dropped != nil {
								if err := a.store.AddSongToManualPlaylist(playlistName, dropped.SongID); err != nil {
									a.status = "No se pudo agregar a playlist: " + err.Error()
								} else {
									_ = a.store.SaveState()
									a.status = "Canción agregada a playlist " + playlistName
									a.section = sectionPlaylists
									a.selectedPlaylistName = playlistName
								}
							}
						}
					}
				})
			}).
				Width(194).
				Grow(1)
			if sidebar.Changed() {
				a.applySidebarSelection(selected)
			}
			ui.Row(c).Padding(6, 8).Children(func() {
				ui.Text(c, fmt.Sprintf("%d álbumes listos", len(a.albumRepresentativeSongs()))).FontSize(11).TextColor(sonoraTextMuted)
				ui.Spacer(c)
				ui.Text(c, totalLibrarySize(a.store.Songs, a)).FontSize(11).TextColor(sonoraTextTertiary)
			})
		})
}

func (a *app) sidebarSelectionID() string {
	if a.section == sectionPlaylists && strings.TrimSpace(a.selectedPlaylistName) != "" {
		return "playlist:" + a.selectedPlaylistName
	}
	switch a.section {
	case sectionRecent:
		return "lib:recent"
	case sectionFavorites:
		return "lib:favorites"
	case sectionQueue:
		return "lib:queue"
	case sectionAlbums:
		return "lib:albums"
	case sectionArtists:
		return "lib:artists"
	default:
		return "lib:songs"
	}
}

func (a *app) applySidebarSelection(id string) {
	if id != "lib:artists" {
		a.artistDetailOn = false
	}
	if id != "lib:albums" {
		a.albumDetailOn = false
	}
	switch {
	case id == "playlist:new":
		a.playlistCreateOpen = true
		a.playlistCreateName = ""
		a.playlistCreateSeedSongID = ""
		a.playlistCreateColor = "none"
	case strings.HasPrefix(id, "playlist:"):
		name := strings.TrimPrefix(id, "playlist:")
		if strings.TrimSpace(name) == "" {
			return
		}
		a.section = sectionPlaylists
		a.selectedPlaylistName = name
	case id == "lib:recent":
		a.section = sectionRecent
	case id == "lib:favorites":
		a.section = sectionFavorites
	case id == "lib:queue":
		a.section = sectionQueue
		a.ensureRandomQueue(10)
	case id == "lib:albums":
		a.section = sectionAlbums
	case id == "lib:artists":
		a.section = sectionArtists
	default:
		a.section = sectionSongs
	}
	_ = a.saveSettings()
}

func (a *app) sidebarLibraryItem(c *ui.Context, label string, sec section) {
	row := ui.Row(c).Width(186).Padding(7, 6).Radius(7).TextColor(sonoraTextMuted)
	if a.section == sec {
		row.Background(adaptiveOverlay(c, 0.08, 0.10)).TextColor(sonoraText)
	} else if row.Hovered() {
		row.Background(adaptiveOverlay(c, 0.04, 0.05))
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
		ui.Text(c, label).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
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
		case sectionQueue:
			ui.Column(c).Grow(1).Padding(12, 12).Children(func() {
				a.viewQueuePage(c)
			})
		case sectionPlaylists:
			a.viewPlaylists(c)
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

func (a *app) viewPlaylists(c *ui.Context) {
	playlists := a.visiblePlaylists()
	if len(playlists) == 0 {
		ui.Column(c).
			Grow(1).
			Padding(18, 18).
			AlignItems(ui.Center).
			Children(func() {
				ui.Box(c).Grow(1)
				ui.Text(c, "No hay playlists todavía").FontSize(18).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Crea una playlist manual desde la barra lateral o con clic derecho en una canción.").FontSize(12).TextColor(sonoraTextMuted)
				ui.Box(c).Grow(1)
			})
		return
	}
	exists := false
	for _, pl := range playlists {
		if pl.Name == a.selectedPlaylistName {
			exists = true
			break
		}
	}
	if !exists {
		a.selectedPlaylistName = playlists[0].Name
	}
	ui.Column(c).Grow(1).Children(func() {
		ui.Row(c).
			Padding(14, 16).
			AlignItems(ui.Center).
			Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "PLAYLIST").FontSize(11).FontWeight(700).TextColor(sonoraTextTertiary)
					ui.Text(c, a.selectedPlaylistName).FontSize(22).FontWeight(700).TextColor(sonoraText)
				})
				ui.Box(c).Grow(1)
				if ui.Button(c, "+ Nueva playlist").Clicked() {
					a.playlistCreateOpen = true
					a.playlistCreateName = ""
					a.playlistCreateSeedSongID = ""
					a.playlistCreateColor = "none"
				}
			})
		a.viewSongs(c, a.filteredSongs(a.songsForPlaylist(a.selectedPlaylistName)))
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
	switch a.albumSort {
	case "Nombre":
		sort.Slice(albums, func(i, j int) bool {
			return strings.ToLower(albums[i].Album) < strings.ToLower(albums[j].Album)
		})
	case "Año":
		sort.Slice(albums, func(i, j int) bool {
			ii, _ := os.Stat(albums[i].Path)
			jj, _ := os.Stat(albums[j].Path)
			if ii == nil || jj == nil {
				return strings.ToLower(albums[i].Album) < strings.ToLower(albums[j].Album)
			}
			return ii.ModTime().After(jj.ModTime())
		})
	default:
		sort.Slice(albums, func(i, j int) bool {
			return strings.ToLower(albums[i].Artist) < strings.ToLower(albums[j].Artist)
		})
	}
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

	const pageSize = 28
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

	ui.Column(c).Grow(1).Gap(12).Background(sonoraCanvas).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Álbumes / Biblioteca local").FontSize(13).TextColor(sonoraTextMuted)
			ui.Spacer(c)
			ui.Text(c, "FLAC / DSD Nativo").FontSize(11).Padding(5, 12).Radius(12).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
		})
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Colección de álbumes").FontSize(31).FontWeight(700).TextColor(sonoraText)
			ui.Textf(c, "%d Items", total).FontSize(11).Padding(4, 8).Radius(8).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
			ui.Spacer(c)
			ui.Text(c, "Ordenar por:").FontSize(12).TextColor(sonoraTextMuted)
			ui.Select(c, &a.albumSort, []string{"Artista", "Nombre", "Año"}).Width(120)
		})
		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
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

		ui.GridView(c, nil, len(pageAlbums), 156, 232, func(i int) {
			s := pageAlbums[i]
			selected := albumIdentity(s) == a.selectedAlbum && a.albumDetailOn
			card := ui.Column(c).
				Padding(10, 10).
				Gap(8).
				Background(adaptiveOverlay(c, 0.03, 0.03)).
				Border(1, sonoraBorder).
				Radius(10)
			if selected {
				card.Border(1, ui.RGBA(84, 172, 255, 0.90)).Background(ui.RGBA(30, 111, 232, 0.20))
			} else if card.Hovered() {
				card.Background(adaptiveOverlay(c, 0.06, 0.06))
			}
			card.Children(func() {
				ui.Box(c).Size(136, 136).Radius(8).Clip().Background(adaptiveOverlay(c, 0.06, 0.05)).Children(func() {
					if bmp := a.coverBitmap(s); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
					}
					ui.Text(c, qualityBadge(s)).
						Absolute().
						Top(6).
						Right(6).
						FontSize(10).
						Padding(2, 6).
						Radius(6).
						Background(ui.RGBA(0, 0, 0, 0.68)).
						TextColor(ui.Hex("#71E1BE"))
				})
				ui.Text(c, s.Album).SingleLine().FontSize(14).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, s.Artist).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
				ui.Text(c, "Catálogo local").SingleLine().FontSize(11).TextColor(sonoraTextTertiary)
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
		Background(sonoraCanvas).
		Children(func() {
			ui.Column(c).
				Padding(12, 14).
				Gap(10).
				BorderWidth(0, 0, 1, 0).
				BorderColor(sonoraBorder).
				Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						if ui.Button(c, "← Volver").Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraText).Clicked() {
							a.albumDetailOn = false
						}
					})
					ui.Row(c).Gap(18).Children(func() {
						ui.Box(c).Size(182, 182).Radius(10).Background(adaptiveOverlay(c, 0.06, 0.05)).Children(func() {
							if bmp := a.coverBitmap(songs[0]); bmp != nil {
								ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(10)
							}
						})
						ui.Column(c).Grow(1).Gap(10).Children(func() {
							ui.Text(c, "ÁLBUM DE ESTUDIO").FontSize(12).FontWeight(700).TextColor(sonoraTextTertiary)
							ui.Text(c, album).FontSize(48).FontWeight(700).TextColor(sonoraText)
							ui.Textf(c, "%s  ·  Remaster  ·  Catálogo Local", artist).FontSize(23).TextColor(sonoraTextMuted)
							ui.Row(c).Gap(8).Children(func() {
								chip("⚡ "+compactFormat(songs[0])+" 96kHz / 24-bit", ui.RGBA(30, 111, 232, 0.22), ui.Hex("#8FC4FF"))
								chip("Bit-Perfect Direct Out", ui.RGBA(16, 185, 129, 0.22), ui.Hex("#66E3BB"))
								chip(fmt.Sprintf("%d pistas · %s", len(songs), formatDurationLong(totalDuration)), adaptiveOverlay(c, 0.10, 0.08), sonoraText)
							})
							ui.Row(c).Gap(8).Children(func() {
								if ui.PrimaryButton(c, "▶ Reproducir Todo").Background(ui.Hex("#1E6FE8")).TextColor(ui.Hex("#EFF5FF")).Clicked() {
									a.playSongByID(songs[0].ID)
								}
								if ui.Button(c, "⤬ Aleatorio").Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraText).Clicked() {
									a.playSongByID(songs[time.Now().UnixNano()%int64(len(songs))].ID)
								}
								like := "♡"
								if albumLiked {
									like = "♥"
								}
								likeBtn := ui.Button(c, like).Width(38).Background(adaptiveOverlay(c, 0.10, 0.08))
								if albumLiked {
									likeBtn.TextColor(ui.Hex("#F43F5E"))
								}
								if likeBtn.Clicked() {
									a.toggleFavorite(songs[0])
								}
								ui.Button(c, "⋯").Width(38).Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraTextMuted)
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
				BorderColor(sonoraBorder).
				TextColor(sonoraTextMuted).
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
					primary := sonoraText
					secondary := sonoraTextMuted
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
						BorderColor(sonoraBorder.Alpha(0.5))
					row.ContextMenu(func(m *ui.Menu) {
						queueItem := m.Item("Agregar a queue")
						if queueItem.Chosen() {
							if a.enqueueSong(songCopy.ID) {
								a.status = fmt.Sprintf("%s agregado al queue", songCopy.Title)
							} else {
								a.status = "No se pudo agregar al queue"
							}
						}
						m.Separator()
						lastfmItem := m.Item("Actualizar conteo desde Last.fm")
						lastfmItem.Disabled(a.lastfm == nil || !a.lastfm.Enabled())
						if lastfmItem.Chosen() {
							a.syncSongPlayCountFromLastFM(songCopy)
						}
					})
					if !selected && row.Hovered() {
						row.Background(adaptiveOverlay(c, 0.05, 0.04))
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
		Gap(12).
		Background(sonoraCanvas).
		Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Artistas / Biblioteca local").FontSize(13).TextColor(sonoraTextMuted)
				ui.Spacer(c)
				ui.Text(c, "FLAC / DSD Nativo").FontSize(11).Padding(5, 12).Radius(12).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Colección de artistas").FontSize(31).FontWeight(700).TextColor(sonoraText)
				ui.Textf(c, "%d Items", len(artists)).FontSize(11).Padding(4, 8).Radius(8).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
				ui.Spacer(c)
			})
			ui.Divider(c)

			colAlbums := float32(130)
			colSongs := float32(130)
			ui.Row(c).
				Height(36).
				Padding(8, 12).
				AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).
				BorderColor(sonoraBorder).
				TextColor(sonoraTextMuted).
				Children(func() {
					ui.Text(c, "Artista").Grow(1).FontSize(12)
					ui.Text(c, "Álbumes").Width(colAlbums).FontSize(12)
					ui.Text(c, "Canciones").Width(colSongs).FontSize(12)
				})

			ui.Scroll(c.Key("artists-rows")).Grow(1).Children(func() {
				for _, artist := range artists {
					artistName := artist
					albumCount := a.artistAlbumCount(artistName)
					songCount := a.artistSongCount(artistName)
					selected := artistName == a.selectedArtist
					rowBG := ui.RGBA(255, 255, 255, 0.0)
					primary := sonoraText
					secondary := sonoraTextMuted
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
						BorderColor(sonoraBorder.Alpha(0.5))
					if !selected && row.Hovered() {
						row.Background(adaptiveOverlay(c, 0.05, 0.04))
					}
					row.Children(func() {
						ui.Text(c, artistName).Grow(1).SingleLine().TextColor(primary)
						ui.Textf(c, "%d", albumCount).Width(colAlbums).SingleLine().Font("monospace").TextColor(secondary)
						ui.Textf(c, "%d", songCount).Width(colSongs).SingleLine().Font("monospace").TextColor(secondary)
					})
					if row.Clicked() {
						a.selectedArtist = artistName
						a.artistDetailOn = true
						a.fetchArtistAvatarAsync(artistName, artistAvatarKey(artistName))
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

func (a *app) artistSongCount(artist string) int {
	count := 0
	for _, s := range a.store.Songs {
		if s.Artist == artist {
			count++
		}
	}
	return count
}

func (a *app) artistAvatarBitmap(artist string) *ui.Bitmap {
	key := artistAvatarKey(artist)
	if bmp, ok := a.artistAvatars[key]; ok {
		return bmp
	}

	if bmp := loadArtistAvatarFromDisk(artist); bmp != nil {
		a.cacheArtistAvatar(key, bmp)
		return bmp
	}

	a.cacheArtistAvatar(key, nil)
	return nil
}

func (a *app) fetchArtistAvatarAsync(artist, key string) {
	if a.lastfm == nil || strings.TrimSpace(a.lastfm.APIKey) == "" {
		return
	}
	if a.artistAvatarFetch[key] {
		return
	}
	a.artistAvatarFetch[key] = true

	go func(artist, key string) {
		finalize := func() {
			a.updateUI(func() {
				delete(a.artistAvatarFetch, key)
			})
		}
		defer finalize()

		imgURL, err := a.lastfm.ArtistImageURL(artist)
		if err != nil || strings.TrimSpace(imgURL) == "" {
			return
		}
		resp, err := a.lastfm.HTTP.Get(imgURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil || len(data) == 0 {
			return
		}
		bmp := decodeArtistAvatarBitmap(data)
		if bmp == nil {
			return
		}
		if path, err := artistAvatarCachePath(artist); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
		a.updateUI(func() {
			a.cacheArtistAvatar(key, bmp)
		})
	}(artist, key)
}

func artistAvatarKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

func artistAvatarCachePath(artist string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "musaik", "artist-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(artistAvatarKey(artist)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".img"), nil
}

func loadArtistAvatarFromDisk(artist string) *ui.Bitmap {
	path, err := artistAvatarCachePath(artist)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	return decodeArtistAvatarBitmap(data)
}

func decodeArtistAvatarBitmap(data []byte) *ui.Bitmap {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err == nil && img != nil {
		const maxAvatarSize = 96
		scaled := downscaleImage(img, maxAvatarSize)
		return ui.NewBitmap(scaled)
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return bmp
}

func artistInitials(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "♪"
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) == 0 {
			return "♪"
		}
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string([]rune{r[0], r[1]}))
	}
	first := []rune(parts[0])
	last := []rune(parts[len(parts)-1])
	if len(first) == 0 || len(last) == 0 {
		return "♪"
	}
	return strings.ToUpper(string([]rune{first[0], last[0]}))
}

func (a *app) viewArtistDetail(c *ui.Context, artist string) {
	a.fetchArtistAvatarAsync(artist, artistAvatarKey(artist))
	groups := a.groupSongsByAlbumForArtist(artist)
	artistSongs := a.songsForArtist(artist)
	if len(artistSongs) == 0 {
		ui.Row(c).Grow(1).Center().Children(func() {
			ui.Text(c, "Sin canciones para este artista").TextColor(sonoraTextMuted)
		})
		return
	}
	totalSize := int64(0)
	for _, s := range artistSongs {
		totalSize += a.songFileSize(s)
	}
	topSongs := a.topPlayedSongsForArtist(artist, 14)
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].Songs) > len(groups[j].Songs)
	})
	artistCover := artistSongs[0]

	ui.Column(c).Grow(1).Background(sonoraCanvas).Children(func() {
		ui.Column(c).
			Padding(14, 16).
			Gap(12).
			BorderWidth(0, 0, 1, 0).
			BorderColor(sonoraBorder).
			Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					if ui.Button(c, "← Artistas").Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraText).Clicked() {
						a.artistDetailOn = false
					}
				})

				ui.Row(c).Gap(16).Children(func() {
					ui.Box(c).Size(116, 116).Radius(12).Background(adaptiveOverlay(c, 0.06, 0.05)).Children(func() {
						if bmp := a.coverBitmap(artistCover); bmp != nil {
							ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(12)
						}
					})
					ui.Column(c).Grow(1).Gap(8).Children(func() {
						ui.Row(c).Gap(8).Children(func() {
							ui.Text(c, "BIBLIOTECA LOCAL SONORA").FontSize(12).FontWeight(700).TextColor(ui.RGBA(142, 197, 255, 0.90))
							ui.Text(c, "•").FontSize(12).TextColor(sonoraTextTertiary)
							ui.Text(c, "HI-RES LOSSLESS").FontSize(12).FontWeight(700).TextColor(ui.RGBA(163, 255, 220, 0.84))
						})
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, artist).FontSize(53).FontWeight(700).TextColor(sonoraText)
							ui.Text(c, artist).FontSize(12).Padding(3, 7).Radius(6).Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraTextMuted)
						})
						ui.Textf(c, "Biblioteca local  ·  %d Álbumes  ·  %d pistas locales  ·  %s almacenados", len(groups), len(artistSongs), formatFileSizeMB(totalSize)).
							FontSize(24).
							TextColor(sonoraTextMuted)
						ui.Row(c).Gap(8).Children(func() {
							if ui.PrimaryButton(c, "▶ Reproducir Artista").Background(ui.Hex("#1E6FE8")).TextColor(ui.Hex("#EFF5FF")).Clicked() {
								a.playSongByID(topSongs[0].ID)
							}
							if ui.Button(c, "⤬ Aleatorio").Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraText).Clicked() {
								s := artistSongs[time.Now().UnixNano()%int64(len(artistSongs))]
								a.playSongByID(s.ID)
							}
							ui.Button(c, "+ Añadir a Playlist").Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraText)
							ui.Button(c, "⋯").Width(36).Background(adaptiveOverlay(c, 0.10, 0.08)).TextColor(sonoraTextMuted)
						})
					})
				})
			})

		ui.Column(c).Grow(1).Padding(12, 14).Gap(12).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, "Álbumes").FontSize(12).Padding(5, 12).Radius(8).Background(adaptiveOverlay(c, 0.12, 0.10)).TextColor(sonoraText)
				ui.Text(c, "Pistas Populares").FontSize(12).Padding(5, 12).Radius(8).Background(adaptiveOverlay(c, 0.04, 0.04)).TextColor(sonoraTextMuted)
				ui.Text(c, "Sencillos y EPs").FontSize(12).Padding(5, 12).Radius(8).Background(adaptiveOverlay(c, 0.04, 0.04)).TextColor(sonoraTextMuted)
				ui.Text(c, "En Vivo & Remezclas").FontSize(12).Padding(5, 12).Radius(8).Background(adaptiveOverlay(c, 0.04, 0.04)).TextColor(sonoraTextMuted)
				ui.Spacer(c)
				ui.Text(c, "Ordenado por: Reproducciones (Descendente)").FontSize(12).TextColor(sonoraTextMuted)
			})

			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "ÁLBUMES DE ESTUDIO").FontSize(13).FontWeight(700).TextColor(sonoraTextTertiary)
				ui.Spacer(c)
				ui.Textf(c, "Ver todos (%d)", len(groups)).FontSize(12).TextColor(ui.RGBA(134, 184, 255, 0.90))
			})
			ui.Row(c).Gap(12).Children(func() {
				for _, group := range groups[:min(5, len(groups))] {
					groupCopy := group
					card := ui.Column(c).
						Width(190).
						Padding(8, 8).
						Gap(8).
						Background(adaptiveOverlay(c, 0.03, 0.03)).
						Border(1, sonoraBorder).
						Radius(10)
					card.Children(func() {
						ui.Box(c).Size(174, 174).Radius(8).Background(adaptiveOverlay(c, 0.06, 0.05)).Children(func() {
							if len(groupCopy.Songs) > 0 {
								if bmp := a.coverBitmap(groupCopy.Songs[0]); bmp != nil {
									ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
								}
							}
						})
						ui.Text(c, groupCopy.Album).SingleLine().FontSize(14).TextColor(sonoraText)
						ui.Textf(c, "%d pistas", len(groupCopy.Songs)).FontSize(12).TextColor(sonoraTextMuted)
					})
					if card.Clicked() && len(groupCopy.Songs) > 0 {
						a.selectedAlbum = albumIdentity(groupCopy.Songs[0])
						a.albumDetailOn = true
						a.section = sectionAlbums
					}
				}
			})

			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "PISTAS MÁS REPRODUCIDAS EN TU MAC").FontSize(13).FontWeight(700).TextColor(sonoraTextTertiary)
				ui.Spacer(c)
				ui.Text(c, "Tasa de bits promedio: 1411 kbps").FontSize(12).TextColor(sonoraTextTertiary)
			})

			colNum := float32(42)
			colAlbum := float32(250)
			colQuality := float32(160)
			colDur := float32(82)
			colLike := float32(36)
			ui.Column(c).Grow(1).Background(adaptiveOverlay(c, 0.02, 0.03)).Border(1, sonoraBorder).Radius(8).Children(func() {
				ui.Row(c).Height(34).Padding(8, 12).BorderWidth(0, 0, 1, 0).BorderColor(sonoraBorder).Children(func() {
					ui.Text(c, "#").Width(colNum).FontSize(12).TextColor(sonoraTextMuted)
					ui.Text(c, "Título").Grow(1).FontSize(12).TextColor(sonoraTextMuted)
					ui.Text(c, "Álbum").Width(colAlbum).FontSize(12).TextColor(sonoraTextMuted)
					ui.Text(c, "Calidad").Width(colQuality).FontSize(12).TextColor(sonoraTextMuted)
					ui.Text(c, "Duración").Width(colDur).FontSize(12).TextColor(sonoraTextMuted)
					ui.Text(c, "").Width(colLike)
				})
				ui.Scroll(c.Key("artist-popular-scroll")).Grow(1).Children(func() {
					for i, song := range topSongs {
						songCopy := song
						idx := i + 1
						selected := songCopy.ID == a.selectedID
						rowBG := ui.RGBA(255, 255, 255, 0)
						primary := sonoraText
						secondary := sonoraTextMuted
						if selected {
							rowBG = ui.RGBA(22, 96, 201, 0.38)
							primary = ui.Hex("#F5F8FF")
							secondary = ui.RGBA(245, 248, 255, 0.88)
						}
						row := ui.Row(c).Height(44).Padding(9, 12).AlignItems(ui.Center).Background(rowBG).BorderWidth(0, 0, 1, 0).BorderColor(sonoraBorder.Alpha(0.5))
						row.ContextMenu(func(m *ui.Menu) {
							queueItem := m.Item("Agregar a queue")
							if queueItem.Chosen() {
								if a.enqueueSong(songCopy.ID) {
									a.status = fmt.Sprintf("%s agregado al queue", songCopy.Title)
								} else {
									a.status = "No se pudo agregar al queue"
								}
							}
							m.Separator()
							lastfmItem := m.Item("Actualizar conteo desde Last.fm")
							lastfmItem.Disabled(a.lastfm == nil || !a.lastfm.Enabled())
							if lastfmItem.Chosen() {
								a.syncSongPlayCountFromLastFM(songCopy)
							}
						})
						if !selected && row.Hovered() {
							row.Background(adaptiveOverlay(c, 0.05, 0.04))
						}
						row.Children(func() {
							ui.Textf(c, "%d", idx).Width(colNum).Font("monospace").TextColor(secondary)
							ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Children(func() {
								if selected {
									ui.Text(c, "◑").FontSize(11).TextColor(ui.Hex("#EAF3FF"))
								}
								ui.Text(c, songCopy.Title).Grow(1).SingleLine().TextColor(primary)
								if selected {
									ui.Text(c, "Master").FontSize(10).Padding(2, 6).Radius(6).Background(ui.RGBA(255, 255, 255, 0.20)).TextColor(ui.Hex("#EAF3FF"))
								}
							})
							ui.Text(c, songCopy.Album).Width(colAlbum).SingleLine().TextColor(secondary)
							ui.Text(c, qualityBadge(songCopy)).Width(colQuality).SingleLine().TextColor(ui.Hex("#71E1BE"))
							ui.Text(c, formatDuration(songCopy.Duration)).Width(colDur).SingleLine().Font("monospace").TextColor(secondary)
							like := "♡"
							if a.store.Favorites[songCopy.ID] {
								like = "♥"
							}
							color := secondary
							if like == "♥" {
								color = ui.Hex("#F43F5E")
							}
							ui.Text(c, like).Width(colLike).SingleLine().TextColor(color)
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

func (a *app) songsForArtist(artist string) []model.Song {
	out := make([]model.Song, 0, 64)
	for _, s := range a.store.Songs {
		if s.Artist == artist {
			out = append(out, s)
		}
	}
	return out
}

func (a *app) topPlayedSongsForArtist(artist string, limit int) []model.Song {
	songs := a.songsForArtist(artist)
	sort.Slice(songs, func(i, j int) bool {
		if songs[i].PlayCount != songs[j].PlayCount {
			return songs[i].PlayCount > songs[j].PlayCount
		}
		return strings.ToLower(songs[i].Title) < strings.ToLower(songs[j].Title)
	})
	if limit > 0 && len(songs) > limit {
		songs = songs[:limit]
	}
	return songs
}

func qualityBadge(song model.Song) string {
	switch strings.ToUpper(song.Format) {
	case "FLAC", "ALAC":
		return "24-bit / 96kHz"
	case "DSD":
		return "1-bit / 2.8MHz"
	case "AIFF", "WAV":
		return "16-bit / 44.1kHz"
	default:
		return "16-bit / 44.1kHz"
	}
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
		Padding(10, 10).
		Gap(12).
		Children(func() {
			ui.Row(c).
				Padding(4, 4).
				Gap(4).
				Background(adaptiveOverlay(c, 0.04, 0.04)).
				Border(1, sonoraBorder).
				Radius(18).
				Children(func() {
					a.viewSettingsTabButton(c, settingsTabGeneral, "General")
					a.viewSettingsTabButton(c, settingsTabAppearance, "Appearance")
					a.viewSettingsTabButton(c, settingsTabLibrary, "Library")
					a.viewSettingsTabButton(c, settingsTabIntegrations, "Integrations")
					a.viewSettingsTabButton(c, settingsTabAbout, "About")
				})
			ui.Box(c).Height(2)
			switch a.settingsTab {
			case settingsTabGeneral:
				a.viewSettingsGeneral(c)
			case settingsTabAppearance:
				a.viewSettingsAppearance(c)
			case settingsTabIntegrations:
				a.viewSettingsIntegrations(c, playlistOptions)
			case settingsTabAbout:
				a.viewSettingsAbout(c)
			default:
				a.viewSettingsLibrary(c)
			}
			ui.Text(c, a.status).FontSize(11).TextColor(sonoraTextMuted)
		})
}

func (a *app) viewSettingsTabButton(c *ui.Context, tab settingsTab, label string) {
	active := a.settingsTab == tab
	btn := ui.Button(c, label).
		Padding(7, 12).
		Background(ui.Transparent).
		TextColor(sonoraTextMuted).
		Radius(14)
	if active {
		btn.Background(ui.Hex("#1E6FE8")).TextColor(ui.Hex("#EEF5FF"))
	}
	if btn.Clicked() {
		a.settingsTab = tab
	}
}

func settingsPanel(c *ui.Context, title string, body func()) {
	ui.Column(c).
		Padding(14, 14).
		Gap(10).
		Background(adaptiveOverlay(c, 0.05, 0.03)).
		Border(1, sonoraBorder).
		Radius(12).
		Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(700).TextColor(sonoraText)
			body()
		})
}

func (a *app) viewSettingsGeneral(c *ui.Context) {
	ui.Column(c).Gap(12).Children(func() {
		settingsPanel(c, "General", func() {
			ui.Text(c, "Atajos principales: Cmd+P (acciones), Cmd+, (ajustes), Espacio (play/pausa).").FontSize(12).TextColor(sonoraTextMuted)
		})
		settingsPanel(c, "Library status", func() {
			ui.Textf(c, "%d canciones indexadas", len(a.store.Songs)).FontSize(12).TextColor(sonoraText)
			ui.Text(c, totalLibrarySize(a.store.Songs, a)).FontSize(12).TextColor(sonoraTextMuted)
		})
	})
}

func (a *app) viewSettingsAppearance(c *ui.Context) {
	mode := "Light"
	if c.Theme().Dark {
		mode = "Dark"
	}
	settingsPanel(c, "Appearance", func() {
		ui.Textf(c, "Theme mode: %s (sigue el sistema operativo)", mode).FontSize(12).TextColor(sonoraTextMuted)
	})
}

func (a *app) viewSettingsLibrary(c *ui.Context) {
	ui.Column(c).Gap(12).Children(func() {
		settingsPanel(c, "Discover & Scanning", func() {
			ui.Text(c, "Selecciona la carpeta raíz e indexa cambios de tu colección.").FontSize(12).TextColor(sonoraTextMuted)
			ui.TextInput(c, &a.rootInput).Label("Carpeta raíz").Width(680)
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Force Refresh").Clicked() {
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
		})

		settingsPanel(c, "Watched folders", func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, strings.TrimSpace(a.rootInput)).SingleLine().Grow(1).FontSize(12).TextColor(sonoraText)
				ui.Textf(c, "%d tracks", len(a.store.Songs)).FontSize(12).TextColor(sonoraTextMuted)
			})
		})
	})
}

func (a *app) viewSettingsIntegrations(c *ui.Context, playlistOptions []string) {
	if len(playlistOptions) == 0 {
		playlistOptions = []string{"Todos"}
	}
	ui.Column(c).Gap(12).Children(func() {
		settingsPanel(c, "Last.fm", func() {
			ui.Text(c, "Now playing, scrobble y sincronización de reproducciones.").FontSize(12).TextColor(sonoraTextMuted)
			ui.TextInput(c, &a.lastfmUsername).Label("Usuario").Width(680)
			ui.TextInput(c, &a.lastfmAPIKey).Label("API key").Width(680)
			ui.TextInput(c, &a.lastfmAPISecret).Label("Shared secret").Width(680)
			ui.TextInput(c, &a.lastfmSessionKey).Label("Session key").Width(680)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "Guardar credenciales").Clicked() {
					a.refreshLastFMClient()
					if err := a.saveSettings(); err != nil {
						a.status = "No se guardaron credenciales Last.fm: " + err.Error()
					} else {
						a.status = "Credenciales Last.fm guardadas"
					}
				}
				if ui.PrimaryButton(c, "Conectar automáticamente").Disabled(a.lastfmAuthBusy).Clicked() {
					a.startLastFMAutoConnect()
				}
			})
		})

		settingsPanel(c, "iPod Classic", func() {
			ui.TextInput(c, &a.ipodMountPath).Label("Ruta de montaje iPod").Width(680)
			ui.Select(c, &a.ipodPlaylistName, playlistOptions).Label("Playlist a sincronizar").Width(360)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "Detectar iPod").Disabled(a.ipodSyncBusy).Clicked() {
					a.detectIPodClassic()
				}
				if ui.PrimaryButton(c, "Sincronizar playlist").Disabled(a.ipodSyncBusy || strings.TrimSpace(a.ipodMountPath) == "").Clicked() {
					a.syncPlaylistToIPod()
				}
				if ui.Button(c, "Guardar ajustes").Clicked() {
					if err := a.saveSettings(); err != nil {
						a.status = "No se guardaron ajustes iPod: " + err.Error()
					} else {
						a.status = "Ajustes iPod guardados"
					}
				}
			})
		})
	})
}

func (a *app) viewSettingsAbout(c *ui.Context) {
	settingsPanel(c, "About Musaik", func() {
		ui.Text(c, "Musaik 0.1.0").FontSize(12).TextColor(sonoraText)
		ui.Text(c, "Reproductor local Hi-Res para macOS con biblioteca, playlists y queue.").FontSize(12).TextColor(sonoraTextMuted)
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
						Background(adaptiveOverlay(c, 0.03, 0.03)).
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
							row.Background(ui.RGBA(30, 111, 232, 0.16))
							row.Border(1, ui.RGBA(86, 170, 255, 0.55))
						} else {
							row.Background(adaptiveOverlay(c, 0.03, 0.03))
							if row.Hovered() {
								row.Background(adaptiveOverlay(c, 0.07, 0.06))
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
		{Kind: "action", ActionID: "play", Title: "Play", Subtitle: "Playback"},
		{Kind: "action", ActionID: "pause", Title: "Pause", Subtitle: "Playback"},
		{Kind: "action", ActionID: "next", Title: "Next", Subtitle: "Playback"},
		{Kind: "action", ActionID: "previous", Title: "Previous", Subtitle: "Playback"},
		{Kind: "action", ActionID: "stop", Title: "Stop", Subtitle: "Playback"},
		{Kind: "action", ActionID: "create-playlist", Title: "Create playlist", Subtitle: "Playlist"},
		{Kind: "action", ActionID: "add-current-song-to-playlist", Title: "Add current playing song to playlist", Subtitle: "Playlist"},
		{Kind: "action", ActionID: "like-current-song", Title: "Like current playing song", Subtitle: "Favorites"},
		{Kind: "action", ActionID: "sync-current-song-lastfm", Title: "Sync current song with Last.fm", Subtitle: "Last.fm"},
		{Kind: "action", ActionID: "settings", Title: "Ajustes", Subtitle: "App"},
		{Kind: "action", ActionID: "quit", Title: "Quit", Subtitle: "App"},
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
	case "create-playlist":
		a.playlistCreateOpen = true
		a.playlistCreateName = ""
		a.playlistCreateSeedSongID = ""
		a.playlistCreateColor = "none"
		a.status = "Creación de playlist"
	case "add-current-song-to-playlist":
		a.openAddCurrentSongToPlaylist()
	case "like-current-song":
		song, ok := a.currentPlayingSong()
		if !ok {
			a.status = "No hay canción reproduciéndose"
			return
		}
		a.likeSong(song)
	case "sync-current-song-lastfm":
		song, ok := a.currentPlayingSong()
		if !ok {
			a.status = "No hay canción reproduciéndose"
			return
		}
		a.syncSongFromLastFM(song, true)
	case "settings":
		a.section = sectionSettings
		a.status = "Ajustes"
	case "quit":
		a.player.Stop()
		a.persistAll()
		mygo.App.Quit()
	}
}

func (a *app) manualPlaylistNames() []string {
	out := make([]string, 0, len(a.store.Playlists))
	for _, pl := range a.store.Playlists {
		if pl.Name == "Todos" || pl.Smart != nil {
			continue
		}
		out = append(out, pl.Name)
	}
	return out
}

func (a *app) currentPlayingSong() (model.Song, bool) {
	state := a.player.State()
	if strings.TrimSpace(state.Path) == "" {
		return model.Song{}, false
	}
	for _, song := range a.store.Songs {
		if song.Path == state.Path {
			return song, true
		}
	}
	return model.Song{}, false
}

func (a *app) openAddCurrentSongToPlaylist() {
	song, ok := a.currentPlayingSong()
	if !ok {
		a.status = "No hay canción reproduciéndose"
		return
	}
	manualPlaylists := a.manualPlaylistNames()
	if len(manualPlaylists) == 0 {
		a.playlistCreateOpen = true
		a.playlistCreateName = ""
		a.playlistCreateSeedSongID = song.ID
		a.playlistCreateColor = "none"
		a.status = "No hay playlists manuales. Crea una para agregar la canción actual."
		return
	}

	a.actionAddSongID = song.ID
	if !containsString(manualPlaylists, a.actionAddPlaylistName) {
		a.actionAddPlaylistName = manualPlaylists[0]
	}
	a.actionAddToPlaylistOpen = true
}

func (a *app) visiblePlaylists() []model.Playlist {
	out := make([]model.Playlist, 0, len(a.store.Playlists))
	for _, pl := range a.store.Playlists {
		if pl.Name == "Todos" {
			continue
		}
		out = append(out, pl)
	}
	return out
}

func playlistColorOptions() []string {
	return []string{"none", "purple", "yellow", "red", "orange", "green", "blue", "gray"}
}

func playlistColorLabel(color string) string {
	switch strings.TrimSpace(strings.ToLower(color)) {
	case "purple":
		return "🟣 Purple"
	case "yellow":
		return "🟡 Yellow"
	case "red":
		return "🔴 Red"
	case "orange":
		return "🟠 Orange"
	case "green":
		return "🟢 Green"
	case "blue":
		return "🔵 Blue"
	case "gray":
		return "⚪ Gray"
	default:
		return "Sin color"
	}
}

func playlistColorDot(color string) string {
	switch strings.TrimSpace(strings.ToLower(color)) {
	case "purple":
		return "🟣"
	case "yellow":
		return "🟡"
	case "red":
		return "🔴"
	case "orange":
		return "🟠"
	case "green":
		return "🟢"
	case "blue":
		return "🔵"
	case "gray":
		return "⚪"
	default:
		return ""
	}
}

func normalizePlaylistColor(color string) string {
	color = strings.TrimSpace(strings.ToLower(color))
	for _, option := range playlistColorOptions() {
		if color == option {
			return color
		}
	}
	return "none"
}

func (a *app) createManualPlaylist(name string, seedSongID string, color string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return fmt.Errorf("define un nombre para la playlist")
	}
	for _, pl := range a.store.Playlists {
		if strings.EqualFold(strings.TrimSpace(pl.Name), n) {
			return fmt.Errorf("ya existe una playlist con ese nombre")
		}
	}
	seed := []string{}
	if strings.TrimSpace(seedSongID) != "" {
		seed = append(seed, seedSongID)
	}
	a.store.AddManualPlaylist(n, seed)
	if err := a.store.SetManualPlaylistColor(n, normalizePlaylistColor(color)); err != nil {
		return err
	}
	if err := a.store.SaveState(); err != nil {
		return err
	}
	a.selectedPlaylistName = n
	a.section = sectionPlaylists
	return nil
}

func (a *app) setManualPlaylistColor(name, color string) error {
	if err := a.store.SetManualPlaylistColor(name, normalizePlaylistColor(color)); err != nil {
		return err
	}
	if err := a.store.SaveState(); err != nil {
		return err
	}
	return nil
}

func (a *app) renameManualPlaylist(oldName, newName string) error {
	if err := a.store.RenameManualPlaylist(oldName, newName); err != nil {
		return err
	}
	if err := a.store.SaveState(); err != nil {
		return err
	}
	if a.selectedPlaylistName == oldName {
		a.selectedPlaylistName = strings.TrimSpace(newName)
	}
	return nil
}

func (a *app) deleteManualPlaylist(name string) error {
	if err := a.store.DeleteManualPlaylist(name); err != nil {
		return err
	}
	if err := a.store.SaveState(); err != nil {
		return err
	}
	if a.selectedPlaylistName == name {
		next := ""
		for _, pl := range a.visiblePlaylists() {
			next = pl.Name
			break
		}
		a.selectedPlaylistName = next
		if next == "" {
			a.section = sectionSongs
		}
	}
	return nil
}

func (a *app) viewActionAddToPlaylistModal(c *ui.Context) {
	manualPlaylists := a.manualPlaylistNames()
	if len(manualPlaylists) > 0 && !containsString(manualPlaylists, a.actionAddPlaylistName) {
		a.actionAddPlaylistName = manualPlaylists[0]
	}
	ui.Modal(c, &a.actionAddToPlaylistOpen, func() {
		ui.Column(c).
			Width(460).
			Padding(14, 14).
			Gap(10).
			Background(sonoraCardAlt).
			Border(1, ui.RGBA(255, 255, 255, 0.10)).
			Radius(12).
			Children(func() {
				ui.Text(c, "Add current song to playlist").FontSize(18).FontWeight(700).TextColor(sonoraText)
				if len(manualPlaylists) == 0 {
					ui.Text(c, "No manual playlists found. Create one and we'll add the current song to it.").FontSize(12).TextColor(sonoraTextMuted)
					ui.Row(c).Gap(8).Children(func() {
						if ui.Button(c, "Cancel").Clicked() {
							a.actionAddToPlaylistOpen = false
							a.actionAddSongID = ""
						}
						if ui.PrimaryButton(c, "Create playlist").Clicked() {
							a.playlistCreateOpen = true
							a.playlistCreateName = ""
							a.playlistCreateSeedSongID = a.actionAddSongID
							a.playlistCreateColor = "none"
							a.actionAddToPlaylistOpen = false
							a.actionAddSongID = ""
						}
					})
					return
				}
				ui.Select(c, &a.actionAddPlaylistName, manualPlaylists).Label("Playlist").Width(420)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Cancel").Clicked() {
						a.actionAddToPlaylistOpen = false
						a.actionAddSongID = ""
					}
					if ui.PrimaryButton(c, "Add song").Clicked() {
						if strings.TrimSpace(a.actionAddSongID) == "" {
							a.status = "No hay canción seleccionada"
							return
						}
						target := strings.TrimSpace(a.actionAddPlaylistName)
						if target == "" {
							a.status = "Selecciona una playlist"
							return
						}
						if err := a.store.AddSongToManualPlaylist(target, a.actionAddSongID); err != nil {
							a.status = "No se pudo agregar a playlist: " + err.Error()
							return
						}
						_ = a.store.SaveState()
						a.status = "Canción actual agregada a " + target
						a.selectedPlaylistName = target
						a.section = sectionPlaylists
						a.actionAddToPlaylistOpen = false
						a.actionAddSongID = ""
					}
				})
			})
	})
}

func (a *app) viewQueueModal(c *ui.Context) {
	ui.Modal(c, &a.queueOpen, func() {
		current, hasCurrent := a.currentPlayingSong()
		upcoming := a.queuedSongs()
		if len(upcoming) < 10 {
			a.ensureRandomQueue(10)
			upcoming = a.queuedSongs()
		}
		totalTracks := len(upcoming)
		if hasCurrent {
			totalTracks++
		}
		ui.Column(c).
			Width(640).
			Height(620).
			Padding(12, 12).
			Gap(8).
			Background(sonoraCardAlt).
			Border(1, sonoraBorder).
			Radius(12).
			Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					closeBtn := ui.Button(c, "✕").Width(28).Height(28).Padding(0, 0).Radius(14)
					if closeBtn.Clicked() {
						a.queueOpen = false
					}
					ui.Text(c, "Play Queue").FontSize(28).FontWeight(700).TextColor(sonoraText)
					ui.Spacer(c)
					ui.Textf(c, "%d tracks", totalTracks).FontSize(13).TextColor(sonoraTextMuted)
					ui.Box(c).Width(8)
					if ui.Button(c, "Limpiar siguientes").Disabled(len(a.queueSongIDs) == 0).Clicked() {
						a.queueSongIDs = nil
						a.status = "Queue limpiado"
					}
				})
				ui.Divider(c)
				ui.Box(c).Grow(1).Children(func() {
					ui.Scroll(c.Key("queue-scroll")).Fill().Children(func() {
						if hasCurrent {
							ui.Row(c).Padding(10, 12).Gap(10).Background(ui.Hex("#3A74DF")).Border(1, ui.RGBA(120, 178, 255, 0.85)).Radius(10).Children(func() {
								ui.Text(c, "▮▮").FontSize(14).FontWeight(700).TextColor(ui.Hex("#F0F6FF"))
								ui.Column(c).Grow(1).Gap(2).Children(func() {
									ui.Text(c, current.Title).SingleLine().FontSize(14).FontWeight(700).TextColor(ui.Hex("#F8FBFF"))
									ui.Text(c, current.Artist).SingleLine().FontSize(11).TextColor(ui.RGBA(248, 251, 255, 0.88))
								})
								ui.Text(c, formatDuration(current.Duration)).SingleLine().FontSize(13).TextColor(ui.Hex("#F8FBFF"))
							})
						} else {
							ui.Row(c).Padding(8, 10).Gap(8).Background(adaptiveOverlay(c, 0.05, 0.04)).Border(1, sonoraBorder).Radius(8).Children(func() {
								ui.Text(c, "Sin reproducción actual").TextColor(sonoraTextMuted)
							})
						}

						ui.Box(c).Height(8)
						if len(upcoming) == 0 {
							ui.Row(c).Padding(8, 10).Background(adaptiveOverlay(c, 0.03, 0.03)).Border(1, sonoraBorder).Radius(8).Children(func() {
								ui.Text(c, "No hay canciones siguientes. Usa clic derecho en una canción → Agregar a queue.").TextColor(sonoraTextMuted)
							})
							return
						}
						for i, song := range upcoming {
							songCopy := song
							idx := i + 1
							row := ui.Row(c).Padding(8, 10).Gap(10).Background(ui.Transparent).BorderWidth(0, 0, 1, 0).BorderColor(sonoraBorder.Alpha(0.55))
							if row.Hovered() {
								row.Background(adaptiveOverlay(c, 0.06, 0.05))
							}
							row.Children(func() {
								ui.Textf(c, "%d", idx).Width(34).Font("monospace").FontSize(20).TextColor(sonoraTextMuted)
								ui.Column(c).Grow(1).Gap(2).Children(func() {
									ui.Text(c, songCopy.Title).SingleLine().FontSize(15).TextColor(sonoraText)
									ui.Text(c, songCopy.Artist).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
								})
								ui.Text(c, formatDuration(songCopy.Duration)).Width(58).SingleLine().FontSize(13).TextColor(sonoraTextMuted)
							})
							if row.Clicked() {
								a.playSongByID(songCopy.ID)
								if idx-1 < len(a.queueSongIDs) {
									a.queueSongIDs = append(a.queueSongIDs[:idx-1], a.queueSongIDs[idx:]...)
								}
							}
						}
					})
				})
			})
	})
}

func (a *app) viewQueuePage(c *ui.Context) {
	current, hasCurrent := a.currentPlayingSong()
	upcoming := a.queuedSongs()
	if len(upcoming) < 10 {
		a.ensureRandomQueue(10)
		upcoming = a.queuedSongs()
	}
	totalTracks := len(upcoming)
	if hasCurrent {
		totalTracks++
	}

	ui.Column(c).
		Grow(1).
		Gap(10).
		Background(sonoraCanvas).
		Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Queue / Biblioteca local").FontSize(13).TextColor(sonoraTextMuted)
				ui.Spacer(c)
				ui.Text(c, "FLAC / DSD Nativo").FontSize(11).Padding(5, 12).Radius(12).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
			})
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Play Queue").FontSize(25).FontWeight(700).TextColor(sonoraText)
				ui.Textf(c, "%d Items", totalTracks).FontSize(11).Padding(4, 8).Radius(8).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
				ui.Spacer(c)
				if ui.Button(c, "Limpiar siguientes").Disabled(len(a.queueSongIDs) == 0).Clicked() {
					a.queueSongIDs = nil
					a.status = "Queue limpiado"
				}
			})
			ui.Divider(c)
			ui.Scroll(c.Key("queue-page-scroll")).Grow(1).Children(func() {
				if hasCurrent {
					ui.Row(c).Padding(10, 12).Gap(10).Background(ui.Hex("#3A74DF")).Border(1, ui.RGBA(120, 178, 255, 0.85)).Radius(10).Children(func() {
						ui.Text(c, "▮▮").FontSize(12).FontWeight(700).TextColor(ui.Hex("#F0F6FF"))
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, current.Title).SingleLine().FontSize(13).FontWeight(700).TextColor(ui.Hex("#F8FBFF"))
							ui.Text(c, current.Artist).SingleLine().FontSize(10).TextColor(ui.RGBA(248, 251, 255, 0.88))
						})
						ui.Text(c, formatDuration(current.Duration)).SingleLine().FontSize(12).TextColor(ui.Hex("#F8FBFF"))
					})
				} else {
					ui.Row(c).Padding(8, 10).Gap(8).Background(adaptiveOverlay(c, 0.05, 0.04)).Border(1, sonoraBorder).Radius(8).Children(func() {
						ui.Text(c, "Sin reproducción actual").TextColor(sonoraTextMuted)
					})
				}

				ui.Box(c).Height(8)
				if len(upcoming) == 0 {
					ui.Row(c).Padding(8, 10).Background(adaptiveOverlay(c, 0.03, 0.03)).Border(1, sonoraBorder).Radius(8).Children(func() {
						ui.Text(c, "No hay canciones siguientes. Usa clic derecho en una canción → Agregar a queue.").TextColor(sonoraTextMuted)
					})
					return
				}
				for i, song := range upcoming {
					songCopy := song
					idx := i + 1
					row := ui.Row(c).Padding(8, 10).Gap(10).Background(ui.Transparent).BorderWidth(0, 0, 1, 0).BorderColor(sonoraBorder.Alpha(0.55))
					if row.Hovered() {
						row.Background(adaptiveOverlay(c, 0.06, 0.05))
					}
					row.Children(func() {
						ui.Textf(c, "%d", idx).Width(30).Font("monospace").FontSize(16).TextColor(sonoraTextMuted)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, songCopy.Title).SingleLine().FontSize(13).TextColor(sonoraText)
							ui.Text(c, songCopy.Artist).SingleLine().FontSize(11).TextColor(sonoraTextMuted)
						})
						ui.Text(c, formatDuration(songCopy.Duration)).Width(52).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
					})
					if row.Clicked() {
						a.playSongByID(songCopy.ID)
						if idx-1 < len(a.queueSongIDs) {
							a.queueSongIDs = append(a.queueSongIDs[:idx-1], a.queueSongIDs[idx:]...)
						}
					}
				}
			})
		})
}

func (a *app) viewPlaylistCreateModal(c *ui.Context) {
	colorOptions := playlistColorOptions()
	if !containsString(colorOptions, a.playlistCreateColor) {
		a.playlistCreateColor = "none"
	}
	ui.Modal(c, &a.playlistCreateOpen, func() {
		ui.Column(c).
			Width(420).
			Padding(14, 14).
			Gap(10).
			Background(sonoraCardAlt).
			Border(1, ui.RGBA(255, 255, 255, 0.10)).
			Radius(12).
			Children(func() {
				ui.Text(c, "Nueva playlist").FontSize(18).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Crear playlist manual y agregar canción inicial (opcional).").FontSize(12).TextColor(sonoraTextMuted)
				ui.TextInput(c, &a.playlistCreateName).
					Placeholder("Nombre de playlist").
					Width(380)
				ui.Select(c, &a.playlistCreateColor, colorOptions).Label("Color tag").Width(220)
				ui.Text(c, playlistColorLabel(a.playlistCreateColor)).FontSize(12).TextColor(sonoraTextMuted)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Cancelar").Clicked() {
						a.playlistCreateOpen = false
						a.playlistCreateName = ""
						a.playlistCreateSeedSongID = ""
						a.playlistCreateColor = "none"
					}
					if ui.PrimaryButton(c, "Crear playlist").Clicked() {
						if err := a.createManualPlaylist(a.playlistCreateName, a.playlistCreateSeedSongID, a.playlistCreateColor); err != nil {
							a.status = "No se pudo crear playlist: " + err.Error()
							return
						}
						a.status = "Playlist creada: " + strings.TrimSpace(a.playlistCreateName)
						a.playlistCreateOpen = false
						a.playlistCreateName = ""
						a.playlistCreateSeedSongID = ""
						a.playlistCreateColor = "none"
					}
				})
			})
	})
}

func (a *app) viewPlaylistRenameModal(c *ui.Context) {
	ui.Modal(c, &a.playlistRenameOpen, func() {
		ui.Column(c).
			Width(420).
			Padding(14, 14).
			Gap(10).
			Background(sonoraCardAlt).
			Border(1, ui.RGBA(255, 255, 255, 0.10)).
			Radius(12).
			Children(func() {
				ui.Text(c, "Editar nombre de playlist").FontSize(18).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Nombre actual: "+a.playlistRenameOriginal).FontSize(12).TextColor(sonoraTextMuted)
				ui.TextInput(c, &a.playlistRenameName).
					Placeholder("Nuevo nombre").
					Width(380)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Cancelar").Clicked() {
						a.playlistRenameOpen = false
						a.playlistRenameOriginal = ""
						a.playlistRenameName = ""
					}
					if ui.PrimaryButton(c, "Guardar").Clicked() {
						if err := a.renameManualPlaylist(a.playlistRenameOriginal, a.playlistRenameName); err != nil {
							a.status = "No se pudo editar playlist: " + err.Error()
							return
						}
						a.status = "Playlist renombrada"
						a.playlistRenameOpen = false
						a.playlistRenameOriginal = ""
						a.playlistRenameName = ""
					}
				})
			})
	})
}

func (a *app) viewPlaylistDeleteModal(c *ui.Context) {
	ui.Modal(c, &a.playlistDeleteConfirmOpen, func() {
		ui.Column(c).
			Width(420).
			Padding(14, 14).
			Gap(10).
			Background(sonoraCardAlt).
			Border(1, ui.RGBA(255, 255, 255, 0.10)).
			Radius(12).
			Children(func() {
				ui.Text(c, "Eliminar playlist").FontSize(18).FontWeight(700).TextColor(sonoraText)
				ui.Text(c, "Se eliminará \""+a.playlistDeleteTarget+"\" y no se puede deshacer.").FontSize(12).TextColor(sonoraTextMuted)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "Cancelar").Clicked() {
						a.playlistDeleteConfirmOpen = false
						a.playlistDeleteTarget = ""
					}
					if ui.PrimaryButton(c, "Eliminar").Clicked() {
						target := strings.TrimSpace(a.playlistDeleteTarget)
						if err := a.deleteManualPlaylist(target); err != nil {
							a.status = "No se pudo eliminar playlist: " + err.Error()
							return
						}
						a.status = "Playlist eliminada: " + target
						a.playlistDeleteConfirmOpen = false
						a.playlistDeleteTarget = ""
					}
				})
			})
	})
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
	a.artistAvatars = map[string]*ui.Bitmap{}
	a.artistAvatarLRU = nil
	a.artistAvatarFetch = map[string]bool{}
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
	const songsPerPage = 220
	totalSongs := len(songs)
	totalPages := (totalSongs + songsPerPage - 1) / songsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if a.songsPage < 0 {
		a.songsPage = 0
	}
	if a.songsPage >= totalPages {
		a.songsPage = totalPages - 1
	}
	start := a.songsPage * songsPerPage
	end := min(start+songsPerPage, totalSongs)
	pageSongs := songs[start:end]

	colArtist := float32(126)
	colAlbum := float32(126)
	colPlays := float32(78)
	colFormat := float32(84)
	colDuration := float32(64)
	colSize := float32(86)
	colLike := float32(46)

	ui.Column(c).Grow(1).Background(sonoraCanvas).Children(func() {
		if a.section == sectionSongs {
			ui.Column(c).Padding(12, 14).Gap(10).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Canciones / Biblioteca local").FontSize(13).TextColor(sonoraTextMuted)
					ui.Spacer(c)
					ui.Text(c, "FLAC / DSD Nativo").FontSize(11).Padding(5, 12).Radius(12).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
				})
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Colección de canciones").FontSize(31).FontWeight(700).TextColor(sonoraText)
					ui.Textf(c, "%d Items", len(songs)).FontSize(11).Padding(4, 8).Radius(8).Background(adaptiveOverlay(c, 0.08, 0.08)).TextColor(sonoraTextMuted)
					ui.Spacer(c)
				})
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Textf(c, "Mostrando %d-%d de %d", start+1, end, totalSongs).FontSize(11).TextColor(sonoraTextMuted)
					ui.Spacer(c)
					prev := ui.Button(c, "←").Disabled(a.songsPage == 0)
					if prev.Clicked() && a.songsPage > 0 {
						a.songsPage--
					}
					ui.Textf(c, "Página %d/%d", a.songsPage+1, totalPages).FontSize(11).TextColor(sonoraTextMuted)
					next := ui.Button(c, "→").Disabled(a.songsPage >= totalPages-1)
					if next.Clicked() && a.songsPage < totalPages-1 {
						a.songsPage++
					}
				})
				ui.Divider(c)
			})
		}
		ui.Row(c).
			Height(36).
			Padding(8, 12).
			AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).
			BorderColor(sonoraBorder).
			TextColor(sonoraTextMuted).
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
			for _, song := range pageSongs {
				songCopy := song
				selected := songCopy.ID == a.selectedID
				rowBG := ui.RGBA(255, 255, 255, 0)
				primary := sonoraText
				secondary := sonoraTextMuted
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
					BorderColor(sonoraBorder.Alpha(0.5))
				row = row.Drag(&draggedSong{SongID: songCopy.ID})
				row.ContextMenu(func(m *ui.Menu) {
					selectedSong := songCopy
					queueItem := m.Item("Agregar a queue")
					if queueItem.Chosen() {
						if a.enqueueSong(selectedSong.ID) {
							a.status = fmt.Sprintf("%s agregado al queue", selectedSong.Title)
						} else {
							a.status = "No se pudo agregar al queue"
						}
					}
					m.Separator()
					menuPlaylist := m.Submenu("Agregar a playlist", func(pm *ui.Menu) {
						anyManual := false
						for _, pl := range a.store.Playlists {
							if pl.Smart != nil || pl.Name == "Todos" {
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
						pm.Separator()
						createItem := pm.Item("Crear nueva playlist…")
						if createItem.Chosen() {
							a.playlistCreateOpen = true
							a.playlistCreateName = ""
							a.playlistCreateSeedSongID = selectedSong.ID
							a.playlistCreateColor = "none"
						}
						if !anyManual {
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
					row.Background(adaptiveOverlay(c, 0.05, 0.04))
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

			ghostRows := max(0, 10-len(pageSongs))
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
	const maxCoverSize = 128
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
	const maxCovers = 40
	if len(a.coverLRU) <= maxCovers {
		return
	}
	evict := a.coverLRU[0]
	a.coverLRU = a.coverLRU[1:]
	delete(a.covers, evict)
}

func (a *app) cacheArtistAvatar(key string, bmp *ui.Bitmap) {
	if _, ok := a.artistAvatars[key]; ok {
		return
	}
	a.artistAvatars[key] = bmp
	a.artistAvatarLRU = append(a.artistAvatarLRU, key)
	const maxArtistAvatars = 24
	if len(a.artistAvatarLRU) <= maxArtistAvatars {
		return
	}
	evict := a.artistAvatarLRU[0]
	a.artistAvatarLRU = a.artistAvatarLRU[1:]
	delete(a.artistAvatars, evict)
}

func (a *app) cycleRepeatMode() {
	switch a.repeatMode {
	case repeatModeOff:
		a.repeatMode = repeatModeAll
	case repeatModeAll:
		a.repeatMode = repeatModeOne
	default:
		a.repeatMode = repeatModeOff
	}
}

func (a *app) navigateToSongInLibrary(song model.Song) {
	if song.ID == "" {
		return
	}
	a.section = sectionSongs
	a.artistDetailOn = false
	a.albumDetailOn = false
	a.searchQuery = ""
	a.selectedID = song.ID
	if idx, _, ok := a.findSong(song.ID); ok {
		a.playIndex = idx
	}
}

func (a *app) navigateToArtistDetail(artist string) {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return
	}
	a.section = sectionArtists
	a.selectedArtist = artist
	a.artistDetailOn = true
	a.albumDetailOn = false
	a.fetchArtistAvatarAsync(artist, artistAvatarKey(artist))
}

func (a *app) navigateToAlbumDetail(song model.Song) {
	identity := albumIdentity(song)
	if identity == "" {
		return
	}
	a.section = sectionAlbums
	a.selectedAlbum = identity
	a.albumDetailOn = true
	a.artistDetailOn = false
}

func drawCenteredButtonIcon(c *ui.Context, icon *ui.SVG, size float32) {
	ui.Column(c).Fill().Center().Children(func() {
		ui.Icon(c, icon).Size(size, size)
	})
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
		Background(sonoraToolbar).
		BorderWidth(1, 0, 0, 0).
		BorderColor(sonoraBorder).
		AlignItems(ui.Center).
		Children(func() {
			ui.Row(c).Width(300).Gap(10).AlignItems(ui.Center).Children(func() {
				coverBtn := ui.Button(c, "").Width(40).Height(40).Padding(0, 0).Radius(4).Background(adaptiveOverlay(c, 0.10, 0.08)).Children(func() {
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
				if coverBtn.Clicked() && hasCurrent {
					a.navigateToSongInLibrary(currentSong)
				}
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
						if hasCurrent {
							titleBtn := ui.Button(c, "").Padding(0, 0).Background(ui.Transparent).TextColor(sonoraText).Children(func() {
								ui.Text(c, title).SingleLine().FontSize(15).FontWeight(600).TextColor(sonoraText)
							})
							if titleBtn.Clicked() {
								a.navigateToSongInLibrary(currentSong)
							}
						} else {
							ui.Text(c, title).SingleLine().FontSize(15).FontWeight(600).TextColor(sonoraText)
						}
						likeBtn := ui.Button(c, "").Width(22).Height(22).Padding(0, 0).Radius(11).Background(ui.Transparent).TextColor(sonoraTextMuted)
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
					if hasCurrent {
						displayArtist := strings.TrimSpace(currentSong.Artist)
						if displayArtist == "" {
							displayArtist = "Unknown Artist"
						}
						displayAlbum := strings.TrimSpace(currentSong.Album)
						if displayAlbum == "" {
							displayAlbum = "Unknown Album"
						}
						ui.Row(c).Gap(4).Children(func() {
							artistBtn := ui.Button(c, "").Padding(0, 0).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
								ui.Text(c, displayArtist).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
							})
							if artistBtn.Clicked() {
								a.navigateToArtistDetail(currentSong.Artist)
							}
							ui.Text(c, "—").FontSize(12).TextColor(sonoraTextMuted)
							albumBtn := ui.Button(c, "").Padding(0, 0).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
								ui.Text(c, displayAlbum).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
							})
							if albumBtn.Clicked() {
								a.navigateToAlbumDetail(currentSong)
							}
						})
					} else {
						ui.Text(c, subtitle).SingleLine().FontSize(12).TextColor(sonoraTextMuted)
					}
					ui.Text(c, quality).SingleLine().FontSize(11).TextColor(sonoraTextTertiary)
				})
			})

			ui.Spacer(c)

			center := ui.Column(c).Width(470).Gap(5)
			center.Children(func() {
				ui.Row(c).Center().Gap(16).Children(func() {
					shuffleBG := ui.Transparent
					shuffleColor := sonoraTextMuted
					if a.shuffle {
						shuffleBG = adaptiveOverlay(c, 0.12, 0.10)
						if c.Theme().Dark {
							shuffleColor = ui.Hex("#EAF3FF")
						} else {
							shuffleColor = ui.Hex("#1E6FE8")
						}
					}
					shuffleBtn := ui.Button(c, "").Width(24).Height(24).Padding(0, 0).Radius(12).Background(shuffleBG).TextColor(shuffleColor).Children(func() {
						drawCenteredButtonIcon(c, iconShuffle, 13)
					})
					if shuffleBtn.Clicked() {
						a.shuffle = !a.shuffle
						_ = a.saveSettings()
					}

					prevBtn := ui.Button(c, "").Width(24).Height(24).Padding(0, 0).Radius(12).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
						drawCenteredButtonIcon(c, iconPrev, 13)
					})
					if prevBtn.Clicked() {
						a.playPrevious()
					}

					playBtn := ui.Button(c, "").Width(34).Height(34).Padding(0, 0).Radius(17).Background(ui.Hex("#F3F3F3")).TextColor(ui.Hex("#1F1F22")).Children(func() {
						drawCenteredButtonIcon(c, playIcon, 14)
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

					nextBtn := ui.Button(c, "").Width(24).Height(24).Padding(0, 0).Radius(12).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
						drawCenteredButtonIcon(c, iconNext, 13)
					})
					if nextBtn.Clicked() {
						a.playNext()
					}

					repeatBG := ui.Transparent
					repeatColor := sonoraTextMuted
					repeatIcon := iconRepeat
					if a.repeatMode != repeatModeOff {
						repeatBG = adaptiveOverlay(c, 0.12, 0.10)
						if c.Theme().Dark {
							repeatColor = ui.Hex("#EAF3FF")
						} else {
							repeatColor = ui.Hex("#1E6FE8")
						}
					}
					if a.repeatMode == repeatModeOne {
						repeatIcon = iconRepeat1
					}
					repeatBtn := ui.Button(c, "").Width(24).Height(24).Padding(0, 0).Radius(12).Background(repeatBG).TextColor(repeatColor).Children(func() {
						drawCenteredButtonIcon(c, repeatIcon, 13)
					})
					if repeatBtn.Clicked() {
						a.cycleRepeatMode()
						_ = a.saveSettings()
					}
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, formatDuration(position)).Font("monospace").FontSize(12).TextColor(sonoraTextMuted)
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
					ui.Text(c, "-"+formatDuration(rem)).Font("monospace").FontSize(12).TextColor(sonoraTextMuted)
				})
			})

			ui.Spacer(c)
			ui.Row(c).Width(220).Gap(8).AlignItems(ui.Center).Children(func() {
				isMuted := a.volumeLevel <= 0
				muteIcon := iconVolume
				if isMuted {
					muteIcon = iconVolumeOff
				}
				muteBtn := ui.Button(c, "").Width(24).Height(24).Padding(0, 0).Radius(12).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
					drawCenteredButtonIcon(c, muteIcon, 13)
				})
				if muteBtn.Clicked() {
					if isMuted {
						restore := a.volumeBeforeMute
						if restore <= 0 {
							restore = 65
						}
						a.volumeLevel = restore
					} else {
						a.volumeBeforeMute = a.volumeLevel
						a.volumeLevel = 0
					}
					if err := a.player.SetVolume(int(a.volumeLevel)); err != nil {
						a.status = "No se pudo ajustar volumen: " + err.Error()
					}
				}
				if ui.Slider(c.Key("volume"), &a.volumeLevel, 0, 100).Step(1).Grow(1).Changed() {
					if a.volumeLevel > 0 {
						a.volumeBeforeMute = a.volumeLevel
					}
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

func (a *app) enqueueSong(songID string) bool {
	if strings.TrimSpace(songID) == "" {
		return false
	}
	if _, _, ok := a.findSong(songID); !ok {
		return false
	}
	a.queueSongIDs = append(a.queueSongIDs, songID)
	return true
}

func (a *app) dequeueNextQueuedSong() (model.Song, bool) {
	for len(a.queueSongIDs) > 0 {
		nextID := a.queueSongIDs[0]
		a.queueSongIDs = a.queueSongIDs[1:]
		_, song, ok := a.findSong(nextID)
		if ok {
			return song, true
		}
	}
	return model.Song{}, false
}

func (a *app) queuedSongs() []model.Song {
	out := make([]model.Song, 0, len(a.queueSongIDs))
	for _, id := range a.queueSongIDs {
		_, song, ok := a.findSong(id)
		if ok {
			out = append(out, song)
		}
	}
	return out
}

func (a *app) naturalUpcomingSongs(limit int) []model.Song {
	if limit <= 0 || len(a.store.Songs) == 0 || a.playIndex < 0 || a.playIndex >= len(a.store.Songs) {
		return nil
	}
	if a.shuffle {
		return nil
	}
	out := make([]model.Song, 0, limit)
	for i := a.playIndex + 1; i < len(a.store.Songs) && len(out) < limit; i++ {
		out = append(out, a.store.Songs[i])
	}
	if a.repeatMode == repeatModeAll {
		for i := 0; i <= a.playIndex && len(out) < limit; i++ {
			out = append(out, a.store.Songs[i])
		}
	}
	return out
}

func (a *app) ensureRandomQueue(minCount int) {
	if minCount <= 0 {
		return
	}
	if len(a.queueSongIDs) >= minCount {
		return
	}
	if len(a.store.Songs) == 0 {
		return
	}

	currentID := ""
	if song, ok := a.currentPlayingSong(); ok {
		currentID = song.ID
	}

	exists := make(map[string]bool, len(a.queueSongIDs)+1)
	for _, id := range a.queueSongIDs {
		exists[id] = true
	}
	if currentID != "" {
		exists[currentID] = true
	}

	candidates := make([]string, 0, len(a.store.Songs))
	for _, song := range a.store.Songs {
		if exists[song.ID] {
			continue
		}
		candidates = append(candidates, song.ID)
	}
	if len(candidates) == 0 {
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	need := minCount - len(a.queueSongIDs)
	if need > len(candidates) {
		need = len(candidates)
	}
	a.queueSongIDs = append(a.queueSongIDs, candidates[:need]...)
}

func (a *app) syncSongPlayCountFromLastFM(song model.Song) {
	a.syncSongFromLastFM(song, false)
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

func (a *app) likeSong(song model.Song) {
	alreadyLiked := a.store.Favorites[song.ID]
	if !alreadyLiked {
		a.store.ToggleFavorite(song.ID)
		_ = a.store.SaveState()
		a.status = fmt.Sprintf("Like guardado: %s", song.Title)
	} else {
		a.status = fmt.Sprintf("La canción ya tiene like: %s", song.Title)
	}

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

func (a *app) syncSongFromLastFM(song model.Song, syncLoved bool) {
	if a.lastfm == nil || !a.lastfm.Enabled() {
		a.status = "Last.fm no está conectado"
		return
	}
	a.status = "Consultando metadata en Last.fm…"
	go func(song model.Song, syncLoved bool) {
		count, loved, err := a.lastfm.UserTrackInfo(song)
		if err != nil {
			a.updateUI(func() {
				a.status = "No se pudo leer datos de Last.fm: " + err.Error()
			})
			return
		}
		a.updateUI(func() {
			if !a.store.SetPlayCount(song.ID, count) {
				a.status = "Canción no encontrada para actualizar datos"
				return
			}
			if syncLoved {
				if loved {
					a.store.Favorites[song.ID] = true
				} else {
					delete(a.store.Favorites, song.ID)
				}
			}
			_ = a.store.SaveState()
			if syncLoved {
				likeState := "not liked"
				if loved {
					likeState = "liked"
				}
				a.status = fmt.Sprintf("Last.fm sync: %s (%d plays, %s)", song.Title, count, likeState)
				return
			}
			a.status = fmt.Sprintf("Conteo actualizado desde Last.fm: %s (%d)", song.Title, count)
		})
	}(song, syncLoved)
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
	if queuedSong, ok := a.dequeueNextQueuedSong(); ok {
		if idx, _, found := a.findSong(queuedSong.ID); found {
			a.playSongAt(idx, queuedSong)
			return
		}
	}
	if a.repeatMode == repeatModeOne {
		song := a.store.Songs[a.playIndex]
		a.playSongAt(a.playIndex, song)
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
		if a.repeatMode != repeatModeAll {
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
		if a.repeatMode != repeatModeAll {
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
	if queuedSong, ok := a.dequeueNextQueuedSong(); ok {
		if idx, _, found := a.findSong(queuedSong.ID); found {
			a.playSongAt(idx, queuedSong)
			return
		}
	}
	if a.shuffle {
		a.playIndex = int(time.Now().UnixNano() % int64(len(a.store.Songs)))
	} else if a.playIndex >= len(a.store.Songs)-1 {
		if a.repeatMode == repeatModeAll {
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

func startDiagnostics(a *app) {
	if enabled, addr := diagnosticsPprofConfig(); enabled {
		go func() {
			log.Printf("pprof activo en http://%s/debug/pprof/", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				log.Printf("pprof detenido: %v", err)
			}
		}()
	}
	startMemorySampler(a)
}

func diagnosticsPprofConfig() (bool, string) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MUSAIK_ENABLE_PPROF")))
	if raw == "" || raw == "0" || raw == "false" || raw == "off" {
		return false, ""
	}
	addr := strings.TrimSpace(os.Getenv("MUSAIK_PPROF_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:6060"
	}
	return true, addr
}

func startMemorySampler(a *app) {
	logPath := strings.TrimSpace(os.Getenv("MUSAIK_PROFILE_LOG"))
	if logPath == "" {
		return
	}
	interval := 2 * time.Second
	if raw := strings.TrimSpace(os.Getenv("MUSAIK_PROFILE_INTERVAL_MS")); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 250 {
			interval = time.Duration(ms) * time.Millisecond
		}
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("no se pudo abrir log de memoria %q: %v", logPath, err)
		return
	}
	if info, statErr := f.Stat(); statErr == nil && info.Size() == 0 {
		_, _ = fmt.Fprintln(f, "ts,heap_alloc_mb,heap_sys_mb,rss_app_mb,rss_player_mb,rss_total_mb")
	}

	go func() {
		defer f.Close()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			appRSS := processRSSMB(os.Getpid())
			playerRSS := processRSSMB(a.player.BackendPID())
			totalRSS := appRSS + playerRSS
			_, _ = fmt.Fprintf(
				f,
				"%s,%.2f,%.2f,%.2f,%.2f,%.2f\n",
				time.Now().Format(time.RFC3339Nano),
				bytesToMB(m.HeapAlloc),
				bytesToMB(m.HeapSys),
				appRSS,
				playerRSS,
				totalRSS,
			)
		}
	}()
}

func bytesToMB(b uint64) float64 {
	return float64(b) / (1024 * 1024)
}

func processRSSMB(pid int) float64 {
	if pid <= 0 {
		return 0
	}
	out, err := execCommand("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	if err != nil {
		return 0
	}
	rssKB, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || rssKB <= 0 {
		return 0
	}
	return float64(rssKB) / 1024.0
}

var execCommand = func(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).Output()
}

func main() {
	a := newApp()
	startDiagnostics(a)
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
