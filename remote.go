package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"musaik/internal/model"
)

const (
	remoteDefaultPort  = 47821
	remoteUITimeout    = 3 * time.Second
	remoteSearchMax    = 50
	remoteSearchDef    = 20
	remoteQueueMax     = 50
	remoteDiscoveryDoc = "remote.json"
)

// remoteServer is the opt-in local HTTP API used by launcher extensions
// (Raycast). It only listens on loopback and requires a bearer token that is
// regenerated on every start and published in remote.json (mode 0600).
type remoteServer struct {
	srv   *http.Server
	port  int
	token string
	path  string
}

type remoteSong struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	Favorite bool   `json:"favorite"`
}

type remoteState struct {
	Playing  bool        `json:"playing"`
	Paused   bool        `json:"paused"`
	Position int         `json:"position"`
	Duration int         `json:"duration"`
	Volume   int         `json:"volume"`
	Shuffle  bool        `json:"shuffle"`
	Repeat   string      `json:"repeat"`
	Queued   int         `json:"queued"`
	Song     *remoteSong `json:"song,omitempty"`
}

func (a *app) setRemoteEnabled(on bool) {
	a.remoteEnabled = on
	if err := a.saveSettings(); err != nil {
		a.status = "No se guardó el ajuste de control externo: " + err.Error()
	}
	a.applyRemoteControl()
}

// applyRemoteControl starts or stops the server so it matches a.remoteEnabled.
// Must be called from the UI thread.
func (a *app) applyRemoteControl() {
	if !a.remoteEnabled {
		if a.remote != nil {
			a.stopRemote()
			a.status = "Control externo desactivado"
		}
		return
	}
	if a.remote != nil {
		return
	}
	rs, err := a.startRemote()
	if err != nil {
		a.remoteEnabled = false
		a.status = "No se pudo iniciar el control externo: " + err.Error()
		return
	}
	a.remote = rs
	a.status = fmt.Sprintf("Control externo activo en 127.0.0.1:%d", rs.port)
}

func (a *app) stopRemote() {
	if a.remote == nil {
		return
	}
	rs := a.remote
	a.remote = nil
	_ = os.Remove(rs.path)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = rs.srv.Shutdown(ctx)
}

func (a *app) startRemote() (*remoteServer, error) {
	path, err := remoteDiscoveryPath()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", remoteDefaultPort))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
	}
	port := ln.Addr().(*net.TCPAddr).Port

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		_ = ln.Close()
		return nil, err
	}
	token := hex.EncodeToString(raw)

	doc, _ := json.Marshal(map[string]any{"port": port, "token": token})
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}

	rs := &remoteServer{port: port, token: token, path: path}
	rs.srv = &http.Server{
		Handler:           a.remoteHandler(rs),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() { _ = rs.srv.Serve(ln) }()
	return rs, nil
}

func remoteDiscoveryPath() (string, error) {
	sp, err := settingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(sp), remoteDiscoveryDoc), nil
}

func (a *app) remoteHandler(rs *remoteServer) http.Handler {
	mux := http.NewServeMux()
	wantHosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", rs.port): true,
		fmt.Sprintf("localhost:%d", rs.port): true,
	}

	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		st, ok := onUIValue(a, a.remoteState)
		if !ok {
			remoteError(w, http.StatusServiceUnavailable, "la app no respondió")
			return
		}
		remoteJSON(w, st)
	})

	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limit := remoteLimit(r.URL.Query().Get("limit"), remoteSearchDef, remoteSearchMax)
		songs, ok := onUIValue(a, func() []remoteSong { return a.remoteSearch(q, limit) })
		if !ok {
			remoteError(w, http.StatusServiceUnavailable, "la app no respondió")
			return
		}
		remoteJSON(w, songs)
	})

	mux.HandleFunc("GET /queue", func(w http.ResponseWriter, r *http.Request) {
		songs, ok := onUIValue(a, func() []remoteSong {
			out := []remoteSong{}
			for _, s := range a.queuedSongs() {
				if len(out) >= remoteQueueMax {
					break
				}
				out = append(out, a.toRemoteSong(s))
			}
			return out
		})
		if !ok {
			remoteError(w, http.StatusServiceUnavailable, "la app no respondió")
			return
		}
		remoteJSON(w, songs)
	})

	action := func(pattern string, fn func(r *http.Request) string) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			type result struct {
				st  remoteState
				err string
			}
			res, ok := onUIValue(a, func() result {
				errMsg := fn(r)
				return result{st: a.remoteState(), err: errMsg}
			})
			if !ok {
				remoteError(w, http.StatusServiceUnavailable, "la app no respondió")
				return
			}
			if res.err != "" {
				remoteError(w, http.StatusBadRequest, res.err)
				return
			}
			remoteJSON(w, res.st)
		})
	}

	action("POST /toggle", func(*http.Request) string {
		a.togglePlayPause()
		return ""
	})
	action("POST /play", func(*http.Request) string {
		st := a.player.State()
		switch {
		case !st.Playing:
			a.playSelectedOrFirst()
		case st.Paused:
			a.togglePlayPause()
		}
		return ""
	})
	action("POST /pause", func(*http.Request) string {
		if st := a.player.State(); st.Playing && !st.Paused {
			a.togglePlayPause()
		}
		return ""
	})
	action("POST /next", func(*http.Request) string {
		a.playNext()
		return ""
	})
	action("POST /prev", func(*http.Request) string {
		a.playPrevious()
		return ""
	})
	action("POST /favorite", func(*http.Request) string {
		song, ok := a.currentPlayingSong()
		if !ok {
			return "no hay canción en reproducción"
		}
		a.toggleFavorite(song)
		return ""
	})
	action("POST /volume", func(r *http.Request) string {
		level, err := strconv.Atoi(r.URL.Query().Get("level"))
		if err != nil || level < 0 || level > 100 {
			return "level debe ser un entero entre 0 y 100"
		}
		a.volumeLevel = float64(level)
		if level > 0 {
			a.volumeBeforeMute = a.volumeLevel
		}
		if err := a.player.SetVolume(level); err != nil {
			return "no se pudo ajustar el volumen: " + err.Error()
		}
		return ""
	})
	action("POST /play/{id}", func(r *http.Request) string {
		id := r.PathValue("id")
		if _, _, ok := a.findSong(id); !ok {
			return "canción no encontrada"
		}
		a.playSongByID(id)
		return ""
	})
	action("POST /queue/{id}", func(r *http.Request) string {
		if !a.enqueueSong(r.PathValue("id")) {
			return "canción no encontrada"
		}
		return ""
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host check blocks DNS-rebinding from web pages; the bearer token
		// blocks other local processes that don't read remote.json.
		if !wantHosts[r.Host] {
			remoteError(w, http.StatusForbidden, "host no permitido")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(rs.token)) != 1 {
			remoteError(w, http.StatusUnauthorized, "token inválido")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *app) toRemoteSong(s model.Song) remoteSong {
	return remoteSong{
		ID:       s.ID,
		Title:    s.Title,
		Artist:   s.Artist,
		Album:    s.Album,
		Duration: int(s.Duration / time.Second),
		Favorite: a.store.Favorites[s.ID],
	}
}

func (a *app) remoteState() remoteState {
	ps := a.player.State()
	st := remoteState{
		Playing:  ps.Playing && !ps.Paused,
		Paused:   ps.Paused,
		Position: int(ps.Position / time.Second),
		Duration: int(ps.Duration / time.Second),
		Volume:   int(a.volumeLevel),
		Shuffle:  a.shuffle,
		Repeat:   string(a.repeatMode),
		Queued:   len(a.queueSongIDs),
	}
	if song, ok := a.currentPlayingSong(); ok {
		rs := a.toRemoteSong(song)
		st.Song = &rs
		if st.Duration == 0 {
			st.Duration = rs.Duration
		}
	}
	return st
}

func (a *app) remoteSearch(q string, limit int) []remoteSong {
	q = strings.TrimSpace(strings.ToLower(q))
	out := []remoteSong{}
	for _, s := range a.store.Songs {
		if q != "" &&
			!strings.Contains(strings.ToLower(s.Title), q) &&
			!strings.Contains(strings.ToLower(s.Artist), q) &&
			!strings.Contains(strings.ToLower(s.Album), q) {
			continue
		}
		out = append(out, a.toRemoteSong(s))
		if len(out) >= limit {
			break
		}
	}
	return out
}

// onUIValue runs fn on the UI thread (all app state is owned by it) and waits
// for the result. The buffered channel keeps fn from blocking if we time out.
func onUIValue[T any](a *app, fn func() T) (T, bool) {
	ch := make(chan T, 1)
	a.updateUI(func() { ch <- fn() })
	select {
	case v := <-ch:
		return v, true
	case <-time.After(remoteUITimeout):
		var zero T
		return zero, false
	}
}

func remoteLimit(raw string, def, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func remoteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func remoteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
