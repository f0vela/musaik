package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"musaik/internal/lyrics"
)

// lyricsTUI is a TUI playing id0 (83 s in) whose lyrics come from a fake LRCLIB.
// answer delivers the lookup's result to the UI, as the program would.
func lyricsTUI(t *testing.T, body string) (tu *tui, fp *fakePlayer, answer func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	tu, fp = newTestTUI(t, 110, 30)
	tu.eng.Lyrics = &lyrics.Client{BaseURL: srv.URL}
	posted := make(chan func(), 1)
	tu.eng.Post = func(fn func()) { posted <- fn }
	answer = func() {
		t.Helper()
		select {
		case fn := <-posted:
			tu.send(funcMsg(fn))
		case <-time.After(5 * time.Second):
			t.Fatal("no answer from the lyrics lookup")
		}
	}
	tu.eng.PlaySongByID("id0")
	return tu, fp, answer
}

const syncedBody = `{"syncedLyrics":"[01:00.00]Line sixty\n[01:20.00]Line eighty\n[01:30.00]Line ninety\n[02:00.00]Line one-twenty","plainLyrics":"x"}`

func TestLyricsViewFollowsTheSong(t *testing.T) {
	tu, fp, answer := lyricsTUI(t, syncedBody)
	tu.press("L")
	if !tu.lyr.open {
		t.Fatal("L did not open the lyrics")
	}
	if s := tu.screen(); !strings.Contains(s, "Buscando letra") {
		t.Fatalf("no loading message:\n%s", s)
	}
	answer()
	s := tu.screen()
	for _, want := range []string{"♬ Letra", "Line sixty", "Line eighty", "Line one-twenty", "Track number 1"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q:\n%s", want, s)
		}
	}
	// The sidebar and player bar stay; the song list is gone.
	if strings.Contains(s, "Nombre de Pista") {
		t.Fatalf("the song list shows behind the lyrics:\n%s", s)
	}

	// The current line is the one at 83 s; moving the song moves the highlight.
	cur := func() int { return lyricsCur(tu) }
	if cur() != 1 {
		t.Fatalf("current line = %d at 83 s, want 1", cur())
	}
	fp.pos = 95 * time.Second
	tu.send(tickMsg(time.Now()))
	if cur() != 2 {
		t.Fatalf("current line = %d at 95 s, want 2", cur())
	}

	// Clicking a line seeks to it.
	tu.screen()
	var lineZone zone
	for _, z := range tu.zones {
		if z.kind == "lyric" && z.arg == 3 {
			lineZone = z
		}
	}
	if lineZone.kind == "" {
		t.Fatal("no clickable zone for line 3")
	}
	tu.send(tea.MouseClickMsg{X: lineZone.x + 2, Y: lineZone.y, Button: tea.MouseLeft})
	if fp.pos != 2*time.Minute {
		t.Fatalf("click seeked to %v, want 2m", fp.pos)
	}

	tu.press("esc")
	if tu.lyr.open {
		t.Fatal("esc did not close the lyrics")
	}
	if !strings.Contains(tu.screen(), "Nombre de Pista") {
		t.Fatal("the song list did not come back")
	}
}

func lyricsCur(tu *tui) int {
	song, _ := tu.nowSong()
	ent := tu.eng.LyricsFor(song)
	return ent.Lyrics.Current(tu.eng.Player.State().Position)
}

func TestLyricsScrollingAndKeysPassThrough(t *testing.T) {
	var lines []string
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("[%02d:%02d.00]Verse %d", i/60, i%60, i))
	}
	tu, fp, answer := lyricsTUI(t, fmt.Sprintf(`{"syncedLyrics":%q}`, strings.Join(lines, "\n")))
	tu.press("L")
	answer()
	tu.screen()
	if !tu.lyr.follow || tu.lyr.top == 0 || tu.lyr.maxTop == 0 {
		t.Fatalf("following the song at 83 s should scroll: %+v", tu.lyr)
	}
	// 83 s is past every line: the last one is current and the view ends there.
	if tu.lyr.top != tu.lyr.maxTop {
		t.Fatalf("top %d, want the end %d", tu.lyr.top, tu.lyr.maxTop)
	}
	tu.press("k", "k", "k")
	if tu.lyr.follow || tu.lyr.top != tu.lyr.maxTop-3 {
		t.Fatalf("scrolling up by hand: %+v", tu.lyr)
	}
	tu.screen()
	if tu.lyr.top != tu.lyr.maxTop-3 {
		t.Fatalf("a new frame moved the view the user scrolled: %+v", tu.lyr)
	}
	tu.press("c")
	tu.screen()
	if !tu.lyr.follow || tu.lyr.top != tu.lyr.maxTop {
		t.Fatalf("c did not follow again: %+v", tu.lyr)
	}
	tu.press("g")
	if tu.lyr.top != 0 {
		t.Fatalf("g: top %d", tu.lyr.top)
	}

	// Playback keys still work; list keys do nothing to what is hidden.
	tu.press("space")
	if !fp.paused {
		t.Fatal("space did not pause with the lyrics open")
	}
	before := tu.eng.PlayingID
	tu.press("enter")
	if tu.eng.PlayingID != before || !tu.lyr.open {
		t.Fatal("enter acted on the hidden list")
	}
	tu.press("2")
	if tu.lyr.open {
		t.Fatal("going to a section left the lyrics open")
	}
}

func TestLyricsPlainNoneAndFailure(t *testing.T) {
	tu, _, answer := lyricsTUI(t, `{"plainLyrics":"Just words\nand more words"}`)
	tu.press("L")
	answer()
	s := tu.screen()
	if !strings.Contains(s, "Just words") || !strings.Contains(s, "sin sincronizar") {
		t.Fatalf("plain lyrics:\n%s", s)
	}
	for _, z := range tu.zones {
		if z.kind == "lyric" {
			t.Fatal("plain lyrics have clickable lines")
		}
	}

	tu, _, answer = lyricsTUI(t, "")
	tu.press("L")
	answer()
	if s := tu.screen(); !strings.Contains(s, "No hay letra para esta canción") {
		t.Fatalf("no lyrics:\n%s", s)
	}
}

func TestLyricsWithNothingPlayingAndPalette(t *testing.T) {
	tu, _ := newTestTUI(t, 100, 30)
	tu.press("L")
	tu.eng.Status = ""
	if s := tu.screen(); !strings.Contains(s, "No hay canción") {
		t.Fatalf("nothing playing:\n%s", s)
	}
	tu.press("esc")
	tu.openPalette()
	tu.pal.input.Set("letra")
	hits := tu.actionHits()
	if len(hits) == 0 || hits[0].ActionID != "lyrics" {
		t.Fatalf("palette does not offer the lyrics: %+v", hits)
	}
	tu.pal.open = false
	tu.applyActionHit(hits[0])
	if !tu.lyr.open {
		t.Fatal("the palette action did not open the lyrics")
	}
}

func TestLyricsSaveToTheFile(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "a.mp3")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono", "-t", "1", path).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg: %v %s", err, out)
	}
	tu, _, answer := lyricsTUI(t, syncedBody)
	tu.eng.Store.Songs[0].Path = path
	tu.eng.Lyrics.CacheDir = t.TempDir()
	tu.eng.PlaySongByID("id0")
	tu.press("L")
	answer()
	s := tu.screen()
	if !strings.Contains(s, "S guardar en el archivo") {
		t.Fatalf("no hint to save:\n%s", s)
	}
	tu.press("S")
	if s := tu.screen(); !strings.Contains(s, "guardando en el archivo") {
		t.Fatalf("not saving:\n%s", s)
	}
	answer()
	s = tu.screen()
	if !strings.Contains(s, "guardada en el archivo") || strings.Contains(s, "S guardar") {
		t.Fatalf("after saving:\n%s", s)
	}
	if !strings.Contains(s, "Letra guardada en las etiquetas") {
		t.Fatalf("no status message:\n%s", s)
	}
	if l := lyrics.Local(path); l == nil || l.Source != "tag" || len(l.Synced) != 4 {
		t.Fatalf("the file has no lyrics: %+v", l)
	}
	// Pressing S again has nothing to save.
	tu.press("S")
	if !strings.Contains(tu.eng.Status, "No hay una letra descargada") {
		t.Fatalf("status: %q", tu.eng.Status)
	}
}

func TestLyricsHeaderKeepsTheMostImportantKeys(t *testing.T) {
	tu, _ := newTestTUI(t, 100, 30)
	long := "Una canción con un título bastante largo — Un artista"
	for _, w := range []int{60, 80, 100} {
		line := ansi.Strip(tu.lyricsHeader(w, "", long, "L cerrar", "S guardar en el archivo", "↑↓ mover", "c seguir")[0])
		if got := lipgloss.Width(line); got != w {
			t.Fatalf("w=%d: header is %d wide", w, got)
		}
		if w >= 80 && !strings.Contains(line, "S guardar en el archivo") {
			t.Fatalf("w=%d: the save key was dropped: %q", w, line)
		}
		if w >= 100 && !strings.Contains(line, "c seguir") {
			t.Fatalf("w=%d: all keys fit but were dropped: %q", w, line)
		}
	}
}
