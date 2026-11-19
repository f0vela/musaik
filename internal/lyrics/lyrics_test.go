package lyrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseLRC(t *testing.T) {
	text := "[ar:Someone]\n[ti:Song]\n[00:12.50] first\n[01:02.5][00:30.00]  twice <00:31.00>word\n[00:05]\n[03:00:25] last\nno stamp\n"
	got := ParseLRC(text)
	want := []Line{
		{5 * time.Second, ""},
		{12500 * time.Millisecond, "first"},
		{30 * time.Second, "twice word"},
		{62500 * time.Millisecond, "twice word"},
		{3*time.Minute + 250*time.Millisecond, "last"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseLRCOffset(t *testing.T) {
	got := ParseLRC("[offset:+500]\n[00:10.00]a\n")
	if len(got) != 1 || got[0].At != 10500*time.Millisecond {
		t.Fatalf("offset not applied: %v", got)
	}
	if ParseLRC("just words\nmore words") != nil {
		t.Fatal("plain text parsed as synced")
	}
}

func TestCurrent(t *testing.T) {
	l := &Lyrics{Synced: []Line{{2 * time.Second, "a"}, {5 * time.Second, "b"}, {9 * time.Second, "c"}}}
	cases := map[time.Duration]int{0: -1, 1900 * time.Millisecond: -1, 2 * time.Second: 0, 4 * time.Second: 0, 5 * time.Second: 1, time.Minute: 2}
	for pos, want := range cases {
		if got := l.Current(pos); got != want {
			t.Errorf("Current(%v) = %d, want %d", pos, got, want)
		}
	}
	if (&Lyrics{Plain: "x"}).Current(time.Second) != -1 {
		t.Error("plain lyrics have no current line")
	}
}

func TestLines(t *testing.T) {
	if got := (&Lyrics{Plain: "a\r\nb\n"}).Lines(); len(got) != 2 || got[1] != "b" {
		t.Fatalf("plain lines: %q", got)
	}
	if (&Lyrics{}).Lines() != nil {
		t.Fatal("empty lyrics have lines")
	}
}

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"Song (feat. Other)":     "Song",
		"Song - Remastered 2011": "Song",
		"Song [Live]":            "Song",
		"Plain Title":            "Plain Title",
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalSidecar(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "a.mp3")
	if err := os.WriteFile(audio, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.lrc"), []byte("[00:01.00]hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := Local(audio)
	if l == nil || !l.IsSynced() || l.Source != "lrc" || l.Synced[0].Text != "hola" {
		t.Fatalf("sidecar not read: %+v", l)
	}
	if Local(filepath.Join(dir, "none.mp3")) != nil {
		t.Fatal("lyrics out of nowhere")
	}
}

type server struct {
	*httptest.Server
	gets, searches atomic.Int32
}

func newServer(t *testing.T, get func(w http.ResponseWriter, r *http.Request), search func() []apiTrack) *server {
	t.Helper()
	s := &server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		s.gets.Add(1)
		if r.Header.Get("User-Agent") != "Musaik/test" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		get(w, r)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		s.searches.Add(1)
		_ = json.NewEncoder(w).Encode(search())
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestFetchExactAndCache(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("track_name") != "Song" || q.Get("artist_name") != "Band" || q.Get("duration") != "200" {
			t.Errorf("query = %v", q)
		}
		_ = json.NewEncoder(w).Encode(apiTrack{SyncedLyrics: "[00:01.00]one\n[00:02.00]two", PlainLyrics: "one\ntwo"})
	}, func() []apiTrack { return nil })
	c := &Client{UserAgent: "Musaik/test", BaseURL: srv.URL, CacheDir: t.TempDir()}
	q := Query{Title: "Song", Artist: "Band", Album: "LP", Duration: 200 * time.Second}
	for i := 0; i < 2; i++ {
		l, err := c.Fetch(context.Background(), q)
		if err != nil || !l.IsSynced() || len(l.Synced) != 2 || l.Source != "lrclib" {
			t.Fatalf("fetch %d: %+v, %v", i, l, err)
		}
	}
	if srv.gets.Load() != 1 {
		t.Fatalf("second fetch hit the network: %d requests", srv.gets.Load())
	}
}

func TestFetchSearchFallbackAndMissCache(t *testing.T) {
	notFound := func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
	srv := newServer(t, notFound, func() []apiTrack {
		return []apiTrack{
			{ArtistName: "Other", Duration: 201, PlainLyrics: "wrong artist"},
			{ArtistName: "Band", Duration: 400, SyncedLyrics: "[00:01.00]too long"},
			{ArtistName: "Band", Duration: 201, PlainLyrics: "plain"},
			{ArtistName: "Band", Duration: 199, SyncedLyrics: "[00:01.00]right", PlainLyrics: "right"},
		}
	})
	c := &Client{UserAgent: "Musaik/test", BaseURL: srv.URL, CacheDir: t.TempDir()}
	l, err := c.Fetch(context.Background(), Query{Title: "Song (feat. X)", Artist: "Band", Duration: 200 * time.Second})
	if err != nil || !l.IsSynced() || l.Synced[0].Text != "right" {
		t.Fatalf("search pick: %+v, %v", l, err)
	}

	empty := newServer(t, notFound, func() []apiTrack { return nil })
	c2 := &Client{UserAgent: "Musaik/test", BaseURL: empty.URL, CacheDir: t.TempDir()}
	q := Query{Title: "Nothing", Artist: "Nobody", Duration: 100 * time.Second}
	for i := 0; i < 2; i++ {
		if _, err := c2.Fetch(context.Background(), q); err != ErrNotFound {
			t.Fatalf("fetch %d err = %v, want ErrNotFound", i, err)
		}
	}
	if empty.gets.Load() != 1 {
		t.Fatalf("a cached miss hit the network again: %d", empty.gets.Load())
	}
}

func TestFetchInstrumentalAndErrors(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiTrack{Instrumental: true})
	}, func() []apiTrack { return nil })
	c := &Client{UserAgent: "Musaik/test", BaseURL: srv.URL}
	l, err := c.Fetch(context.Background(), Query{Title: "Intro", Artist: "Band"})
	if err != nil || !l.Instrumental {
		t.Fatalf("instrumental: %+v, %v", l, err)
	}

	bad := newServer(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }, func() []apiTrack { return nil })
	dir := t.TempDir()
	c = &Client{UserAgent: "Musaik/test", BaseURL: bad.URL, CacheDir: dir}
	if _, err := c.Fetch(context.Background(), Query{Title: "X", Artist: "Y"}); err == nil || err == ErrNotFound {
		t.Fatalf("server error err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("a failed request was cached")
	}
	if _, err := c.Fetch(context.Background(), Query{}); err != ErrNotFound {
		t.Fatalf("no title err = %v", err)
	}
}
