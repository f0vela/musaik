package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"musaik/internal/model"
	"musaik/internal/player"
)

type fakePlayer struct {
	path     string
	paused   bool
	duration time.Duration
	pos      time.Duration
	volume   int
	events   chan player.Event
	plays    []string
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{volume: 80, events: make(chan player.Event, 8)}
}

func (f *fakePlayer) Play(path string, d time.Duration) error {
	f.path, f.paused, f.duration, f.pos = path, false, d, 0
	f.plays = append(f.plays, path)
	return nil
}
func (f *fakePlayer) TogglePause() error { f.paused = !f.paused; return nil }
func (f *fakePlayer) Stop()              { f.path = "" }
func (f *fakePlayer) State() player.State {
	return player.State{Path: f.path, Playing: f.path != "", Paused: f.paused, Position: f.pos, Duration: f.duration}
}
func (f *fakePlayer) SeekTo(p time.Duration) error { f.pos = p; return nil }
func (f *fakePlayer) Volume() int                  { return f.volume }
func (f *fakePlayer) SetVolume(v int) error        { f.volume = v; return nil }
func (f *fakePlayer) Events() <-chan player.Event  { return f.events }

// newTestEngine builds an engine over a throwaway home, so it never reads or
// writes the real library.
func newTestEngine(t *testing.T, n int) (*Engine, *fakePlayer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	fp := newFakePlayer()
	e := New(fp)
	var songs []model.Song
	for i := 0; i < n; i++ {
		songs = append(songs, model.Song{
			ID:       "id" + strconv.Itoa(i),
			Path:     "/music/" + strconv.Itoa(i) + ".flac",
			Title:    "Song " + strconv.Itoa(i),
			Artist:   "Artist " + strconv.Itoa(i%2),
			Album:    "Album " + strconv.Itoa(i%3),
			TrackNum: i + 1,
			Duration: 3 * time.Minute,
		})
	}
	e.Store.SetSongs("/music", songs)
	return e, fp
}

func TestNextPreviousAndHistory(t *testing.T) {
	e, fp := newTestEngine(t, 4)
	e.PlaySongByID("id0")
	e.Next()
	e.Next()
	if e.PlayingID != "id2" {
		t.Fatalf("after two nexts: %s", e.PlayingID)
	}
	e.Previous()
	if e.PlayingID != "id1" {
		t.Fatalf("previous should return to id1, got %s", e.PlayingID)
	}
	if len(e.Queue) != 1 || e.Queue[0] != "id2" {
		t.Fatalf("going back should put the song we left on the queue: %v", e.Queue)
	}
	e.Next()
	if e.PlayingID != "id2" {
		t.Fatalf("next should take the queued song, got %s", e.PlayingID)
	}
	if fp.path != "/music/2.flac" {
		t.Fatalf("player path %s", fp.path)
	}
}

func TestEndOfLibraryAndRepeat(t *testing.T) {
	e, _ := newTestEngine(t, 2)
	e.PlaySongByID("id1")
	e.Next()
	if e.PlayingID != "id1" || e.Status != "Fin de la cola" {
		t.Fatalf("at the end: %s %q", e.PlayingID, e.Status)
	}
	e.Repeat = RepeatAll
	e.Next()
	if e.PlayingID != "id0" {
		t.Fatalf("repeat all should wrap, got %s", e.PlayingID)
	}
	e.CycleRepeat()
	if e.Repeat != RepeatOne {
		t.Fatalf("cycle all -> one, got %s", e.Repeat)
	}
	e.CycleRepeat()
	if e.Repeat != RepeatOff {
		t.Fatalf("cycle one -> off, got %s", e.Repeat)
	}
}

func TestAutoAdvanceOnTrackEnd(t *testing.T) {
	e, fp := newTestEngine(t, 3)
	e.PlaySongByID("id0")
	fp.events <- player.Event{Type: player.EventTrackEnded}
	e.ConsumePlayerEvents()
	if e.PlayingID != "id1" {
		t.Fatalf("track end should advance, got %s", e.PlayingID)
	}
	e.Repeat = RepeatOne
	fp.events <- player.Event{Type: player.EventTrackEnded}
	e.ConsumePlayerEvents()
	if e.PlayingID != "id1" || len(fp.plays) != 3 {
		t.Fatalf("repeat one should replay: %s plays=%d", e.PlayingID, len(fp.plays))
	}
}

func TestQueueRules(t *testing.T) {
	e, _ := newTestEngine(t, 6)
	if e.Enqueue("") || e.Enqueue("nope") {
		t.Fatal("bad ids must not queue")
	}
	e.Enqueue("id4")
	e.Enqueue("id2")
	e.PlaySongByID("id0")
	e.Next()
	if e.PlayingID != "id4" {
		t.Fatalf("queue first: %s", e.PlayingID)
	}
	e.PlayFromQueue(0)
	if e.PlayingID != "id2" || len(e.Queue) != 0 {
		t.Fatalf("play from queue: %s %v", e.PlayingID, e.Queue)
	}
	e.EnsureRandomQueue(3)
	if len(e.Queue) != 3 {
		t.Fatalf("random fill: %v", e.Queue)
	}
	for _, id := range e.Queue {
		if id == "id2" {
			t.Fatal("the playing song must not be queued")
		}
	}
	e.ClearQueue()
	if len(e.Queue) != 0 {
		t.Fatal("clear")
	}
}

func TestVolumeMuteSeek(t *testing.T) {
	e, fp := newTestEngine(t, 1)
	e.PlaySongByID("id0")
	e.SetVolume(150)
	if e.Volume != 100 || fp.volume != 100 {
		t.Fatalf("clamp: %d %d", e.Volume, fp.volume)
	}
	e.ToggleMute()
	if fp.volume != 0 {
		t.Fatal("mute")
	}
	e.ToggleMute()
	if fp.volume != 100 {
		t.Fatalf("unmute restores: %d", fp.volume)
	}
	e.SeekTo(time.Hour)
	if fp.pos != 3*time.Minute-time.Second {
		t.Fatalf("seek clamps to the end: %v", fp.pos)
	}
	e.SeekBy(-time.Hour)
	if fp.pos != 0 {
		t.Fatalf("seek clamps to the start: %v", fp.pos)
	}
}

func TestFavoritesAndPlaylists(t *testing.T) {
	e, _ := newTestEngine(t, 3)
	s := e.Store.Songs[1]
	e.ToggleFavorite(s)
	if !e.Store.Favorites[s.ID] || len(e.Favorites()) != 1 {
		t.Fatal("like")
	}
	e.ToggleFavorite(s)
	if e.Store.Favorites[s.ID] {
		t.Fatal("unlike")
	}
	e.LikeSong(s)
	e.LikeSong(s)
	if !e.Store.Favorites[s.ID] {
		t.Fatal("LikeSong never unlikes")
	}

	if err := e.CreatePlaylist(" ", "", "red"); err == nil {
		t.Fatal("blank name")
	}
	if err := e.CreatePlaylist("Rock", "id0", "red"); err != nil {
		t.Fatal(err)
	}
	if err := e.CreatePlaylist("rock", "", "none"); err == nil {
		t.Fatal("duplicate names are refused whatever the case")
	}
	if err := e.AddToPlaylist("Rock", "id2"); err != nil {
		t.Fatal(err)
	}
	if got := e.SongsForPlaylist("Rock"); len(got) != 2 {
		t.Fatalf("playlist songs: %d", len(got))
	}
	if err := e.RenamePlaylist("Rock", "Metal"); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPlaylistColor("Metal", "bogus"); err != nil {
		t.Fatal(err)
	}
	if names := e.ManualPlaylistNames(); len(names) != 1 || names[0] != "Metal" {
		t.Fatalf("manual names: %v", names)
	}
	if err := e.DeletePlaylist("Metal"); err != nil {
		t.Fatal(err)
	}
	if len(e.ManualPlaylistNames()) != 0 {
		t.Fatal("delete")
	}
}

func TestQueries(t *testing.T) {
	e, _ := newTestEngine(t, 6)
	if got := e.Artists("artist 1"); len(got) != 1 {
		t.Fatalf("artists: %v", got)
	}
	if n := e.ArtistSongCount("Artist 0"); n != 3 {
		t.Fatalf("song count %d", n)
	}
	albums := e.Albums("", "Nombre")
	if len(albums) != 6 {
		t.Fatalf("albums are per artist+album pair: %d", len(albums))
	}
	if got := FilterSongs(e.Store.Songs, "song 3"); len(got) != 1 {
		t.Fatalf("filter: %d", len(got))
	}
	groups := e.AlbumGroupsForArtist("Artist 0")
	if len(groups) == 0 || groups[0].Songs[0].TrackNum > groups[0].Songs[len(groups[0].Songs)-1].TrackNum {
		t.Fatalf("groups: %+v", groups)
	}
	e.Store.Songs[2].LastPlayedAt = time.Now()
	e.Store.Songs[3].LastPlayedAt = time.Now().Add(-48 * time.Hour)
	e.Store.Songs[4].LastPlayedAt = time.Now().Add(-30 * 24 * time.Hour)
	rec := e.Recent(15)
	if len(rec) != 2 || rec[0].ID != "id2" {
		t.Fatalf("recent is newest first and limited to 15 days: %+v", rec)
	}
}

func TestSettingsKeepDesktopKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := SettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mini_opacity": 55, "shuffle": false, "future_key": {"a": 1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	s.Shuffle = true
	s.RepeatMode = "all"
	s.SidebarHidden = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var future map[string]int
	_ = json.Unmarshal(doc["future_key"], &future)
	if string(doc["mini_opacity"]) != "55" || future["a"] != 1 {
		t.Fatalf("keys of the desktop app must survive: %s", data)
	}
	if string(doc["shuffle"]) != "true" || string(doc["repeat"]) != "true" || string(doc["sidebar_hidden"]) != "true" {
		t.Fatalf("own keys must be written: %s", data)
	}
	if filepath.Base(path) != "settings.json" {
		t.Fatal("same file as the desktop app")
	}
}

func TestRemoteAPI(t *testing.T) {
	e, _ := newTestEngine(t, 3)
	r := &Remote{Port: 4000, token: "secret"}
	h := e.remoteHandler(r)
	do := func(method, target, host, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		req.Host = host
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := do("GET", "/state", "127.0.0.1:4000", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := do("GET", "/state", "evil.example:4000", "secret"); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: %d", rec.Code)
	}
	if rec := do("GET", "/state", "localhost:4000", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("ok: %d", rec.Code)
	}
	var got []RemoteSong
	rec := do("GET", "/search?q=artist+1&limit=1", "127.0.0.1:4000", "secret")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].Artist != "Artist 1" {
		t.Fatalf("search: %v %s", err, rec.Body.String())
	}
	if rec := do("POST", "/queue/id2", "127.0.0.1:4000", "secret"); rec.Code != http.StatusOK || len(e.Queue) != 1 {
		t.Fatalf("queue: %d %v", rec.Code, e.Queue)
	}
	if rec := do("POST", "/queue/nope", "127.0.0.1:4000", "secret"); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown id: %d", rec.Code)
	}
	if rec := do("POST", "/volume?level=500", "127.0.0.1:4000", "secret"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad volume: %d", rec.Code)
	}
	if rec := do("POST", "/play/id1", "127.0.0.1:4000", "secret"); rec.Code != http.StatusOK || e.PlayingID != "id1" {
		t.Fatalf("play: %d %s", rec.Code, e.PlayingID)
	}
	if rec := do("GET", "/play", "127.0.0.1:4000", "secret"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET on an action: %d", rec.Code)
	}
}
