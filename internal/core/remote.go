package core

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
	// RemoteDefaultPort is tried first; another free port is used if it is taken.
	RemoteDefaultPort = 47821
	remoteUITimeout   = 3 * time.Second
	remoteSearchMax   = 50
	remoteSearchDef   = 20
	remoteQueueMax    = 50
)

// Remote is the opt-in local HTTP API that launcher extensions (Raycast) use.
// It listens on loopback only and wants a bearer token that is new on every
// start and published in remote.json (mode 0600). It speaks the same protocol
// as the desktop app's server.
type Remote struct {
	srv   *http.Server
	Port  int
	token string
	path  string
}

// RemoteSong is a song as the API describes it.
type RemoteSong struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	Favorite bool   `json:"favorite"`
}

// RemoteState is the playback state as the API describes it.
type RemoteState struct {
	Playing  bool        `json:"playing"`
	Paused   bool        `json:"paused"`
	Position int         `json:"position"`
	Duration int         `json:"duration"`
	Volume   int         `json:"volume"`
	Shuffle  bool        `json:"shuffle"`
	Repeat   string      `json:"repeat"`
	Queued   int         `json:"queued"`
	Song     *RemoteSong `json:"song,omitempty"`
}

// StartRemote starts the API; stop it with Close.
func (e *Engine) StartRemote() (*Remote, error) {
	path, err := RemoteDiscoveryPath()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", RemoteDefaultPort))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
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

	r := &Remote{Port: port, token: token, path: path}
	r.srv = &http.Server{
		Handler:           e.remoteHandler(r),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() { _ = r.srv.Serve(ln) }()
	return r, nil
}

// Close stops the server and removes remote.json.
func (r *Remote) Close() {
	if r == nil {
		return
	}
	_ = os.Remove(r.path)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = r.srv.Shutdown(ctx)
}

// RemoteDiscoveryPath is the file that tells extensions the port and token.
func RemoteDiscoveryPath() (string, error) {
	sp, err := SettingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(sp), "remote.json"), nil
}

// onUI runs fn on the engine's goroutine and waits for its result; the
// buffered channel keeps fn from blocking if the wait times out.
func onUI[T any](e *Engine, fn func() T) (T, bool) {
	ch := make(chan T, 1)
	e.post(func() { ch <- fn() })
	select {
	case v := <-ch:
		return v, true
	case <-time.After(remoteUITimeout):
		var zero T
		return zero, false
	}
}

func (e *Engine) remoteHandler(r *Remote) http.Handler {
	mux := http.NewServeMux()
	wantHosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", r.Port): true,
		fmt.Sprintf("localhost:%d", r.Port): true,
	}

	read := func(pattern string, fn func(req *http.Request) any) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
			v, ok := onUI(e, func() any { return fn(req) })
			if !ok {
				remoteError(w, http.StatusServiceUnavailable, "la app no respondió")
				return
			}
			remoteJSON(w, v)
		})
	}
	read("GET /state", func(*http.Request) any { return e.RemoteState() })
	read("GET /search", func(req *http.Request) any {
		limit := remoteLimit(req.URL.Query().Get("limit"), remoteSearchDef, remoteSearchMax)
		return e.RemoteSearch(req.URL.Query().Get("q"), limit)
	})
	read("GET /queue", func(*http.Request) any {
		out := []RemoteSong{}
		for _, s := range e.QueuedSongs() {
			if len(out) >= remoteQueueMax {
				break
			}
			out = append(out, e.toRemoteSong(s))
		}
		return out
	})

	action := func(pattern string, fn func(req *http.Request) string) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
			type result struct {
				st  RemoteState
				err string
			}
			res, ok := onUI(e, func() result {
				msg := fn(req)
				return result{st: e.RemoteState(), err: msg}
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
	action("POST /toggle", func(*http.Request) string { e.TogglePlayPause(); return "" })
	action("POST /play", func(*http.Request) string { e.Resume(); return "" })
	action("POST /pause", func(*http.Request) string { e.Pause(); return "" })
	action("POST /next", func(*http.Request) string { e.Next(); return "" })
	action("POST /prev", func(*http.Request) string { e.Previous(); return "" })
	action("POST /favorite", func(*http.Request) string {
		song, ok := e.CurrentSong()
		if !ok {
			return "no hay canción en reproducción"
		}
		e.ToggleFavorite(song)
		return ""
	})
	action("POST /volume", func(req *http.Request) string {
		level, err := strconv.Atoi(req.URL.Query().Get("level"))
		if err != nil || level < 0 || level > 100 {
			return "level debe ser un entero entre 0 y 100"
		}
		e.SetVolume(level)
		return ""
	})
	action("POST /play/{id}", func(req *http.Request) string {
		id := req.PathValue("id")
		if _, _, ok := e.FindSong(id); !ok {
			return "canción no encontrada"
		}
		e.PlaySongByID(id)
		return ""
	})
	action("POST /queue/{id}", func(req *http.Request) string {
		if !e.Enqueue(req.PathValue("id")) {
			return "canción no encontrada"
		}
		return ""
	})

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// The Host check blocks DNS rebinding from web pages; the bearer
		// token blocks local processes that do not read remote.json.
		if !wantHosts[req.Host] {
			remoteError(w, http.StatusForbidden, "host no permitido")
			return
		}
		got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(r.token)) != 1 {
			remoteError(w, http.StatusUnauthorized, "token inválido")
			return
		}
		mux.ServeHTTP(w, req)
	})
}

func (e *Engine) toRemoteSong(s model.Song) RemoteSong {
	return RemoteSong{
		ID:       s.ID,
		Title:    s.Title,
		Artist:   s.Artist,
		Album:    s.Album,
		Duration: int(s.Duration / time.Second),
		Favorite: e.Store.Favorites[s.ID],
	}
}

// RemoteState describes the playback for the API.
func (e *Engine) RemoteState() RemoteState {
	ps := e.Player.State()
	st := RemoteState{
		Playing:  ps.Playing && !ps.Paused,
		Paused:   ps.Paused,
		Position: int(ps.Position / time.Second),
		Duration: int(ps.Duration / time.Second),
		Volume:   e.Volume,
		Shuffle:  e.Shuffle,
		Repeat:   string(e.Repeat),
		Queued:   len(e.Queue),
	}
	if song, ok := e.CurrentSong(); ok {
		rs := e.toRemoteSong(song)
		st.Song = &rs
		if st.Duration == 0 {
			st.Duration = rs.Duration
		}
	}
	return st
}

// RemoteSearch lists up to limit songs matching q for the API.
func (e *Engine) RemoteSearch(q string, limit int) []RemoteSong {
	q = strings.TrimSpace(strings.ToLower(q))
	out := []RemoteSong{}
	for _, s := range e.Store.Songs {
		if q != "" &&
			!strings.Contains(strings.ToLower(s.Title), q) &&
			!strings.Contains(strings.ToLower(s.Artist), q) &&
			!strings.Contains(strings.ToLower(s.Album), q) {
			continue
		}
		out = append(out, e.toRemoteSong(s))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func remoteLimit(raw string, def, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return min(n, max)
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
