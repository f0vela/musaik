package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
