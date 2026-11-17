package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"musaik/internal/ipod"
	"musaik/internal/lastfm"
	"musaik/internal/library"
	"musaik/internal/model"
)

// IndexAsync scans root in the background and, when done, swaps the songs in
// through Post. Play counts and last-played dates carry over. done may be nil.
func (e *Engine) IndexAsync(root string, done func(err error)) {
	if e.Indexing {
		return
	}
	root = strings.TrimSpace(root)
	e.Indexing = true
	e.Status = "Indexando " + root + "…"
	snapshot := slices.Clone(e.Store.Songs)
	go func() {
		tmp := library.NewStore()
		tmp.Songs = snapshot
		err := tmp.Index(root)
		e.post(func() {
			e.Indexing = false
			if err != nil {
				e.Status = "Error indexando: " + err.Error()
			} else {
				e.Store.SetSongs(root, tmp.Songs)
				e.sizeCache = map[string]int64{}
				e.Settings.RootInput = root
				if len(e.Store.Songs) > 0 {
					e.SelectedID = e.Store.Songs[0].ID
				}
				e.Status = fmt.Sprintf("Indexadas %d canciones", len(e.Store.Songs))
				_ = e.Persist()
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// ---- iPod ---------------------------------------------------------------------

// DetectIPod looks for a mounted iPod Classic and, with report, says so in the
// status line and remembers its mount path.
func (e *Engine) DetectIPod(report bool) {
	devices, err := ipod.DetectClassicMounts("/Volumes")
	if err != nil {
		if report {
			e.Status = "Error detectando iPod: " + err.Error()
		}
		return
	}
	if len(devices) == 0 {
		e.IPodDetected, e.IPodName = false, ""
		if report {
			e.Status = "No se detectó iPod Classic montado en /Volumes"
		}
		return
	}
	e.IPodDetected = true
	e.IPodName = devices[0].Name
	if report || strings.TrimSpace(e.Settings.IPodMountPath) == "" {
		e.Settings.IPodMountPath = devices[0].MountPath
	}
	if report {
		_ = e.SaveSettings()
		e.Status = "iPod detectado: " + devices[0].Name
	}
}

// SyncIPodAsync copies the chosen playlist to the iPod and rewrites its
// iTunesDB in the background. done may be nil.
func (e *Engine) SyncIPodAsync(done func()) {
	if e.IPodBusy {
		return
	}
	mount := strings.TrimSpace(e.Settings.IPodMountPath)
	if mount == "" {
		e.Status = "Define la ruta de montaje del iPod"
		return
	}
	playlist := strings.TrimSpace(e.Settings.IPodPlaylistName)
	if playlist == "" {
		playlist = "Todos"
	}
	songs := slices.Clone(e.SongsForPlaylist(playlist))
	if len(songs) == 0 {
		e.Status = "La playlist está vacía"
		return
	}
	e.IPodBusy = true
	e.Status = fmt.Sprintf("Sincronizando %d canciones hacia iPod…", len(songs))
	_ = e.SaveSettings()
	go func(songs []model.Song) {
		copied, _, err := ipod.SyncPlaylist(mount, playlist, songs)
		e.post(func() {
			e.IPodBusy = false
			switch {
			case err != nil:
				e.Status = "Falló sincronización iPod: " + err.Error()
			case copied == 0:
				e.Status = "iPod al día: iTunesDB reescrita, sin archivos nuevos que copiar"
			default:
				e.Status = fmt.Sprintf("iPod sincronizado: %d copias nuevas, iTunesDB reescrita (backup: iTunesDB.musaik.bak)", copied)
			}
			if done != nil {
				done()
			}
		})
	}(songs)
}

// ---- Last.fm authorization ------------------------------------------------------

// ConnectLastFM runs the Last.fm web authorization: it listens for the callback
// on a local port, asks openURL to show the authorization page, and stores the
// session it gets back. done may be nil.
func (e *Engine) ConnectLastFM(openURL func(string) error, done func()) {
	s := e.Settings
	if s.LastFMAPIKey == "" || s.LastFMAPISecret == "" {
		e.Status = "Falta API key/shared secret de Last.fm"
		return
	}
	if e.LastFMAuthBusy {
		e.Status = "Autorización Last.fm en progreso"
		return
	}
	e.LastFMAuthBusy = true
	e.Status = "Abriendo autorización de Last.fm en tu navegador…"
	apiKey, apiSecret, user := s.LastFMAPIKey, s.LastFMAPISecret, s.LastFMUsername

	finish := func(msg string, apply func()) {
		e.post(func() {
			e.LastFMAuthBusy = false
			if apply != nil {
				apply()
			}
			e.Status = msg
			if done != nil {
				done()
			}
		})
	}

	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			finish("No se pudo iniciar callback local de Last.fm: "+err.Error(), nil)
			return
		}
		client := lastfm.New(apiKey, apiSecret, "", user)
		authURL := client.AuthURL("http://" + listener.Addr().String() + "/callback")

		tokenCh := make(chan string, 1)
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		})}
		go func() { _ = server.Serve(listener) }()

		if err := openURL(authURL); err != nil {
			_ = server.Close()
			finish("No se pudo abrir navegador para Last.fm: "+err.Error()+" — abre "+authURL, nil)
			return
		}
		select {
		case token := <-tokenCh:
			sessionKey, username, err := client.SessionFromToken(token)
			_ = server.Close()
			if err != nil {
				finish("Falló auth.getSession de Last.fm: "+err.Error(), nil)
				return
			}
			finish("Last.fm conectado correctamente", func() {
				e.Settings.LastFMSessionKey = sessionKey
				if username != "" {
					e.Settings.LastFMUsername = username
				}
				e.RefreshLastFM()
				_ = e.Persist()
			})
		case <-time.After(5 * time.Minute):
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = server.Shutdown(ctx)
			cancel()
			finish("Tiempo de espera agotado en autorización Last.fm", nil)
		}
	}()
}
