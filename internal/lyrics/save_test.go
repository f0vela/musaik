package lyrics

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.senan.xyz/taglib"
)

func TestLRCRoundTrip(t *testing.T) {
	in := &Lyrics{Synced: []Line{{0, "intro"}, {12340 * time.Millisecond, "uno"}, {3*time.Minute + 5*time.Second + 70*time.Millisecond, ""}}}
	text := in.LRC()
	if text != "[00:00.00]intro\n[00:12.34]uno\n[03:05.07]" {
		t.Fatalf("LRC() = %q", text)
	}
	out := ParseLRC(text)
	if len(out) != 3 || out[1] != in.Synced[1] || out[2].At != in.Synced[2].At {
		t.Fatalf("round trip: %v", out)
	}
	if got := (&Lyrics{Plain: " hola\nmundo \n"}).LRC(); got != "hola\nmundo" {
		t.Fatalf("plain LRC() = %q", got)
	}
	if (*Lyrics)(nil).LRC() != "" || (*Lyrics)(nil).CanSave() {
		t.Fatal("nil lyrics")
	}
}

func TestCanSave(t *testing.T) {
	cases := []struct {
		l    *Lyrics
		want bool
	}{
		{&Lyrics{Plain: "x", Source: "lrclib"}, true},
		{&Lyrics{Plain: "x", Source: "tag"}, false}, // already in the file
		{&Lyrics{Plain: "x", Source: "lrc"}, false},
		{&Lyrics{Instrumental: true, Source: "lrclib"}, false},
		{&Lyrics{Source: "lrclib"}, false},
	}
	for i, c := range cases {
		if got := c.l.CanSave(); got != c.want {
			t.Errorf("case %d: CanSave() = %v, want %v", i, got, c.want)
		}
	}
}

// makeAudio writes a one-second silent file in the format of ext, with a title
// and artist tag, using ffmpeg; the test is skipped without it.
func makeAudio(t *testing.T, dir, ext string) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(dir, "song"+ext)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1",
		"-metadata", "title=Original Title", "-metadata", "artist=Original Artist"}
	switch ext {
	case ".mp4":
		args = append(args, "-c:a", "aac")
	case ".ogg":
		args = append(args, "-c:a", "vorbis", "-strict", "-2") // the native encoder: not every ffmpeg has libvorbis
	}
	if out, err := exec.Command(ffmpeg, append(args, path)...).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot make %s: %v %s", ext, err, out)
	}
	return path
}

func TestSaveToTagInEveryFormat(t *testing.T) {
	synced := &Lyrics{Source: "lrclib", Synced: []Line{{time.Second, "primera"}, {4 * time.Second, "segunda"}}}
	for _, ext := range []string{".mp3", ".flac", ".mp4", ".ogg"} {
		t.Run(ext, func(t *testing.T) {
			path := makeAudio(t, t.TempDir(), ext)
			if Local(path) != nil {
				t.Fatal("lyrics before saving")
			}
			if err := SaveToTag(path, synced); err != nil {
				t.Fatal(err)
			}
			got := Local(path)
			if got == nil || got.Source != "tag" || !got.IsSynced() || len(got.Synced) != 2 || got.Synced[1].Text != "segunda" || got.Synced[1].At != 4*time.Second {
				t.Fatalf("read back: %+v", got)
			}
			tags, err := taglib.ReadTags(path)
			if err != nil {
				t.Fatal(err)
			}
			if tags[taglib.Title][0] != "Original Title" || tags[taglib.Artist][0] != "Original Artist" {
				t.Fatalf("other tags changed: %v", tags)
			}
			// Saving again replaces them instead of adding a second copy.
			if err := SaveToTag(path, &Lyrics{Source: "lrclib", Plain: "solo texto"}); err != nil {
				t.Fatal(err)
			}
			got = Local(path)
			if got == nil || got.IsSynced() || got.Plain != "solo texto" {
				t.Fatalf("after replacing: %+v", got)
			}
		})
	}
}

func TestSaveToTagErrors(t *testing.T) {
	if err := SaveToTag("/nonexistent/x.mp3", &Lyrics{Plain: "x"}); err == nil {
		t.Fatal("writing to a missing file worked")
	}
	if err := SaveToTag("/nonexistent/x.mp3", &Lyrics{Instrumental: true}); err == nil {
		t.Fatal("saving an instrumental worked")
	}
}

func TestLocalPrefersTheTagThenTheLRCFile(t *testing.T) {
	path := makeAudio(t, t.TempDir(), ".flac")
	lrc := filepath.Join(filepath.Dir(path), "song.lrc")
	if err := os.WriteFile(lrc, []byte("[00:01.00]del archivo lrc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if l := Local(path); l == nil || l.Source != "lrc" {
		t.Fatalf("only the .lrc exists: %+v", l)
	}
	// Plain text in the tag does not beat a synced .lrc…
	if err := taglib.WriteTags(path, map[string][]string{taglib.Lyrics: {"texto plano de la etiqueta"}}, 0); err != nil {
		t.Fatal(err)
	}
	if l := Local(path); l == nil || l.Source != "lrc" || !l.IsSynced() {
		t.Fatalf("synced .lrc over plain tag: %+v", l)
	}
	// …but synced text in the tag comes first.
	if err := taglib.WriteTags(path, map[string][]string{taglib.Lyrics: {"[00:02.00]de la etiqueta"}}, 0); err != nil {
		t.Fatal(err)
	}
	if l := Local(path); l == nil || l.Source != "tag" || l.Synced[0].Text != "de la etiqueta" {
		t.Fatalf("synced tag first: %+v", l)
	}
	// With no .lrc, plain text in the tag is used.
	_ = os.Remove(lrc)
	if err := taglib.WriteTags(path, map[string][]string{taglib.Lyrics: {"solo plano"}}, 0); err != nil {
		t.Fatal(err)
	}
	if l := Local(path); l == nil || l.Source != "tag" || l.Plain != "solo plano" {
		t.Fatalf("plain tag alone: %+v", l)
	}
}

func TestFetchUsesTheTagBeforeTheNetwork(t *testing.T) {
	path := makeAudio(t, t.TempDir(), ".mp3")
	if err := SaveToTag(path, &Lyrics{Plain: "ya en la etiqueta"}); err != nil {
		t.Fatal(err)
	}
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the network was asked although the file has the lyrics")
		http.NotFound(w, r)
	}, func() []apiTrack { return nil })
	c := &Client{UserAgent: "Musaik/test", BaseURL: srv.URL, CacheDir: t.TempDir()}
	l, err := c.Fetch(context.Background(), Query{Title: "T", Artist: "A", Path: path})
	if err != nil || l.Source != "tag" || l.Plain != "ya en la etiqueta" {
		t.Fatalf("fetch: %+v, %v", l, err)
	}
	if srv.gets.Load() != 0 {
		t.Fatal("requests made")
	}
}

func TestCacheExpiresAfterThreeDays(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiTrack{PlainLyrics: "letra"})
	}, func() []apiTrack { return nil })
	dir := t.TempDir()
	c := &Client{UserAgent: "Musaik/test", BaseURL: srv.URL, CacheDir: dir}
	q := Query{Title: "T", Artist: "A", Duration: 100 * time.Second}
	if _, err := c.Fetch(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	age := func(d time.Duration) {
		t.Helper()
		data, err := os.ReadFile(c.cachePath(q))
		if err != nil {
			t.Fatal(err)
		}
		var e cacheEntry
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		e.At = time.Now().Add(-d)
		data, _ = json.Marshal(e)
		if err := os.WriteFile(c.cachePath(q), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	age(71 * time.Hour)
	if _, err := c.Fetch(context.Background(), q); err != nil || srv.gets.Load() != 1 {
		t.Fatalf("at 71 h the cache should hold: err %v, %d requests", err, srv.gets.Load())
	}
	age(73 * time.Hour)
	if _, err := c.Fetch(context.Background(), q); err != nil || srv.gets.Load() != 2 {
		t.Fatalf("at 73 h the answer should be asked again: err %v, %d requests", err, srv.gets.Load())
	}

	// A miss expires the same way.
	miss := newServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }, func() []apiTrack { return nil })
	c = &Client{UserAgent: "Musaik/test", BaseURL: miss.URL, CacheDir: t.TempDir()}
	_, _ = c.Fetch(context.Background(), q)
	age(73 * time.Hour)
	_, _ = c.Fetch(context.Background(), q)
	if miss.gets.Load() != 2 {
		t.Fatalf("an old miss was not retried: %d", miss.gets.Load())
	}
}

func TestSweepAndForget(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	fresh := filepath.Join(dir, "fresh.json")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-4 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	c := &Client{CacheDir: dir}
	c.writeCache(Query{Title: "x"}, cacheEntry{Plain: "y"}) // the first write sweeps
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("an expired file survived the sweep")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh file was swept")
	}
	q := Query{Title: "x"}
	if _, err := os.Stat(c.cachePath(q)); err != nil {
		t.Fatal("entry not written")
	}
	c.Forget(q)
	if _, err := os.Stat(c.cachePath(q)); !os.IsNotExist(err) {
		t.Fatal("Forget kept the entry")
	}
}
