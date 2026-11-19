package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"musaik/internal/lyrics"
)

func TestLyricsForLooksUpOnceAndPostsTheAnswer(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"syncedLyrics": "[00:01.00]hola", "plainLyrics": "hola"})
	}))
	defer srv.Close()

	e, _ := newTestEngine(t, 2)
	e.Lyrics = &lyrics.Client{BaseURL: srv.URL, CacheDir: t.TempDir()}
	done := make(chan func(), 1)
	e.Post = func(fn func()) { done <- fn }

	song := e.Store.Songs[0]
	if got := e.LyricsFor(song); got.State != LyricsLoading {
		t.Fatalf("first ask: state %v, want loading", got.State)
	}
	if got := e.LyricsFor(song); got.State != LyricsLoading {
		t.Fatalf("asking again while loading: %v", got.State)
	}
	select {
	case fn := <-done:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("the lookup never posted back")
	}
	got := e.LyricsFor(song)
	if got.State != LyricsReady || !got.Lyrics.IsSynced() || got.Lyrics.Synced[0].Text != "hola" {
		t.Fatalf("after the answer: %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("%d requests, want 1", hits)
	}
}

func TestLyricsForReportsNoneAndFailures(t *testing.T) {
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		http.Error(w, "x", status)
	}))
	defer srv.Close()
	e, _ := newTestEngine(t, 2)
	e.Lyrics = &lyrics.Client{BaseURL: srv.URL}
	posted := make(chan func(), 1)
	e.Post = func(fn func()) { posted <- fn }

	answer := func(song int, want LyricsState) LyricsEntry {
		t.Helper()
		s := e.Store.Songs[song]
		e.LyricsFor(s)
		select {
		case fn := <-posted:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("the lookup never posted back")
		}
		ent := e.LyricsFor(s)
		if ent.State != want {
			t.Fatalf("song %d: state %v, want %v", song, ent.State, want)
		}
		return ent
	}
	answer(0, LyricsNone)

	status = http.StatusInternalServerError
	if ent := answer(1, LyricsFailed); ent.Err == "" {
		t.Fatal("a failure has no message")
	}
	e.ForgetLyrics("id1")
	if _, ok := e.lyricsCache["id1"]; ok {
		t.Fatal("ForgetLyrics kept it")
	}
}

func TestSaveLyricsToTagWritesTheFileAndDropsTheCache(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"syncedLyrics": "[00:01.00]uno\n[00:02.50]dos"})
	}))
	defer srv.Close()

	e, _ := newTestEngine(t, 1)
	path := filepath.Join(t.TempDir(), "a.mp3")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", "1", path).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg: %v %s", err, out)
	}
	song := e.Store.Songs[0]
	song.Path = path
	cacheDir := t.TempDir()
	e.Lyrics = &lyrics.Client{BaseURL: srv.URL, CacheDir: cacheDir}
	posted := make(chan func(), 1)
	e.Post = func(fn func()) { posted <- fn }
	run := func() {
		t.Helper()
		select {
		case fn := <-posted:
			fn()
		case <-time.After(10 * time.Second):
			t.Fatal("nothing posted back")
		}
	}

	e.SaveLyricsToTag(song) // nothing to save yet
	if e.Status != "No hay una letra descargada que guardar" {
		t.Fatalf("status: %q", e.Status)
	}
	e.LyricsFor(song)
	run()
	if entries, _ := os.ReadDir(cacheDir); len(entries) != 1 {
		t.Fatalf("the download was not cached: %d files", len(entries))
	}
	e.SaveLyricsToTag(song)
	if !e.LyricsFor(song).Saving {
		t.Fatal("not marked as saving")
	}
	e.SaveLyricsToTag(song) // a second request while saving does nothing
	run()
	got := e.LyricsFor(song)
	if got.Saving || got.Lyrics.Source != "tag" || !strings.Contains(e.Status, "Letra guardada") {
		t.Fatalf("after saving: %+v, status %q", got, e.Status)
	}
	if entries, _ := os.ReadDir(cacheDir); len(entries) != 0 {
		t.Fatalf("the cache kept %d files after saving to the tag", len(entries))
	}
	if l := lyrics.Local(path); l == nil || l.Source != "tag" || len(l.Synced) != 2 || l.Synced[1].At != 2500*time.Millisecond {
		t.Fatalf("the file does not have the lyrics: %+v", l)
	}
	e.SaveLyricsToTag(song) // already in the file
	if e.Status != "No hay una letra descargada que guardar" {
		t.Fatalf("saving what is saved: %q", e.Status)
	}
}
