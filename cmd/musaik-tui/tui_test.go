package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
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
}

func (f *fakePlayer) Play(path string, d time.Duration) error {
	f.path, f.paused, f.duration, f.pos = path, false, d, 83*time.Second
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

// newTestTUI builds the app over a throwaway home with a small library, so no
// test touches the real one.
func newTestTUI(t *testing.T, w, h int) (*tui, *fakePlayer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	fp := &fakePlayer{volume: 80, events: make(chan player.Event, 4)}
	eng := core.New(fp)
	artists := []string{"Arctic Monkeys", "Boards of Canada", "Caribou"}
	albums := []string{"AM", "Music Has the Right to Children", "Swim"}
	var songs []model.Song
	for i := 0; i < 30; i++ {
		a := i % 3
		songs = append(songs, model.Song{
			ID:       "id" + strconv.Itoa(i),
			Path:     "/music/" + strconv.Itoa(i) + ".flac",
			Title:    fmt.Sprintf("Track number %d with a fairly long title", i+1),
			Artist:   artists[a],
			Album:    albums[a],
			TrackNum: i/3 + 1,
			Format:   []string{"flac", "mp3", "flac"}[a],
			Duration: time.Duration(180+i) * time.Second,
		})
	}
	eng.Store.SetSongs("/music", songs)
	eng.Store.AddManualPlaylist("Road trip", []string{"id1", "id2"})
	_ = eng.Store.SetManualPlaylistColor("Road trip", "orange")
	tu := newTUI(eng, false)
	tu.send(tea.WindowSizeMsg{Width: w, Height: h})
	return tu, fp
}

func (t *tui) send(msg tea.Msg) {
	t.Update(msg)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+p":
		return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func (t *tui) press(keys ...string) {
	for _, k := range keys {
		t.send(key(k))
	}
}

func (t *tui) typeText(s string) {
	for _, r := range s {
		t.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// screen is what the app shows, without styling.
func (t *tui) screen() string {
	return ansi.Strip(t.View().Content)
}

func TestScreenFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {100, 30}, {80, 24}, {64, 18}, {50, 14}, {40, 10}} {
		tu, _ := newTestTUI(t, size[0], size[1])
		tu.press("enter")
		for _, sec := range []string{"1", "2", "3", "4", "5", "6", "7"} {
			tu.press(sec)
			lines := strings.Split(tu.screen(), "\n")
			if len(lines) != size[1] {
				t.Fatalf("%dx%d section %s: %d lines", size[0], size[1], sec, len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Fatalf("%dx%d section %s line %d is %d wide: %q", size[0], size[1], sec, i, w, l)
				}
			}
		}
	}
}

func TestMiniFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 16}, {64, 12}, {60, 20}, {44, 10}, {80, 9}} {
		tu, _ := newTestTUI(t, size[0], size[1])
		tu.press("enter", "M")
		lines := strings.Split(tu.screen(), "\n")
		if len(lines) != size[1] {
			t.Fatalf("mini %dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Fatalf("mini %dx%d line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}

func TestPlaybackKeys(t *testing.T) {
	tu, fp := newTestTUI(t, 100, 30)
	tu.press("enter")
	if tu.eng.PlayingID != "id0" || fp.path != "/music/0.flac" {
		t.Fatalf("enter plays the song under the cursor: %s", tu.eng.PlayingID)
	}
	tu.press("space")
	if !fp.paused {
		t.Fatal("space pauses")
	}
	tu.press("n")
	if tu.eng.PlayingID != "id1" {
		t.Fatalf("n: %s", tu.eng.PlayingID)
	}
	tu.press("p")
	if tu.eng.PlayingID != "id0" {
		t.Fatalf("p goes back through the history: %s", tu.eng.PlayingID)
	}
	tu.press("right")
	if fp.pos != 88*time.Second {
		t.Fatalf("right seeks 5s: %v", fp.pos)
	}
	tu.press("-", "-")
	if fp.volume != 70 {
		t.Fatalf("volume: %d", fp.volume)
	}
	tu.press("m")
	if fp.volume != 0 {
		t.Fatal("mute")
	}
	tu.press("s", "r")
	if !tu.eng.Shuffle || tu.eng.Repeat != core.RepeatAll {
		t.Fatal("shuffle and repeat")
	}
	tu.press("l")
	if !tu.eng.Store.Favorites["id0"] {
		t.Fatal("l likes what plays")
	}
}

func TestNavigationSearchAndDetail(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.press("down", "down", "j")
	if tu.list.cur != 3 {
		t.Fatalf("cursor: %d", tu.list.cur)
	}
	tu.press("/")
	tu.typeText("boards")
	if len(tu.rows) != 10 {
		t.Fatalf("search filters live: %d rows", len(tu.rows))
	}
	tu.press("esc")
	if tu.query() != "" || len(tu.rows) != 30 {
		t.Fatalf("esc clears the search: %q %d", tu.query(), len(tu.rows))
	}

	tu.press("2")
	if tu.sec != secArtists || len(tu.artists) != 3 {
		t.Fatalf("artists: %d", len(tu.artists))
	}
	tu.press("down", "enter")
	if !tu.artistOpen || tu.artist != "Boards of Canada" {
		t.Fatalf("artist detail: %v %q", tu.artistOpen, tu.artist)
	}
	tu.press("esc")
	if tu.artistOpen {
		t.Fatal("esc closes the detail")
	}

	tu.press("3")
	tu.press("enter")
	if !tu.albumOpen || len(tu.albumRows) != 10 {
		t.Fatalf("album detail: %v %d", tu.albumOpen, len(tu.albumRows))
	}
	tu.press("P")
	if tu.eng.PlayingID == "" {
		t.Fatal("P plays the album")
	}
}

func TestQueueAndPlaylists(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.press("a", "down", "a")
	if len(tu.eng.Queue) != 2 {
		t.Fatalf("a queues the song: %v", tu.eng.Queue)
	}
	tu.press("4")
	if tu.sec != secQueue {
		t.Fatal("queue section")
	}
	if len(tu.queueRows) < 10 {
		t.Fatalf("the queue tops itself up to 10: %d", len(tu.queueRows))
	}
	tu.press("x")
	if len(tu.eng.Queue) < 10 {
		t.Fatalf("x removes and the queue refills: %d", len(tu.eng.Queue))
	}
	tu.press("C")
	tu.press("7")
	if tu.sec != secPlaylist || tu.playlist != "Favoritos" {
		t.Fatalf("7 opens the first playlist, the smart one: %v %q", tu.sec, tu.playlist)
	}
	tu.goPlaylist("Road trip")
	tu.refreshIfDirty()
	if tu.sec != secPlaylist || tu.playlist != "Road trip" || len(tu.rows) != 2 {
		t.Fatalf("playlist: %v %q %d", tu.sec, tu.playlist, len(tu.rows))
	}

	tu.press("N")
	tu.typeText("Gym")
	tu.press("tab", "enter")
	if tu.playlist != "Gym" {
		t.Fatalf("new playlist: %q", tu.playlist)
	}
	tu.press("R")
	tu.press("ctrl+u")
	tu.typeText("Gym 2")
	tu.press("enter")
	if tu.playlist != "Gym 2" {
		t.Fatalf("rename: %q", tu.playlist)
	}
	tu.press("D")
	tu.press("y")
	if tu.playlist != "Favoritos" {
		t.Fatalf("delete falls back to the first playlist, as the desktop app does: %q", tu.playlist)
	}

	tu.press("1", "A")
	if tu.modal.kind != modalAddToPlaylist {
		t.Fatal("A opens the playlist picker")
	}
	tu.press("enter")
	if got := tu.eng.SongsForPlaylist("Road trip"); len(got) != 3 {
		t.Fatalf("added to the playlist: %d", len(got))
	}
}

func TestPaletteAndSettings(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.press("ctrl+p")
	if !tu.pal.open || len(tu.pal.hits) == 0 {
		t.Fatal("palette opens with the actions")
	}
	tu.typeText("ajustes")
	tu.press("enter")
	if tu.sec != secSettings {
		t.Fatal("the Ajustes action opens the settings view")
	}
	tu.press("]", "]")
	if tu.set.tab != 2 {
		t.Fatalf("tabs: %d", tu.set.tab)
	}
	tu.press("esc")
	if tu.sec != secSongs {
		t.Fatalf("esc leaves settings: %v", tu.sec)
	}
	tu.press("b")
	if !tu.eng.Settings.SidebarHidden {
		t.Fatal("b hides the sidebar")
	}
	if strings.Contains(tu.screen(), "BIBLIOTECA") {
		t.Fatal("the sidebar is gone")
	}
	tu.press("b")
	if !strings.Contains(tu.screen(), "BIBLIOTECA") {
		t.Fatal("the sidebar is back")
	}
}

func TestPaletteCloseApp(t *testing.T) {
	for _, word := range []string{"cerrar", "salir", "quit"} {
		tu, _ := newTestTUI(t, 110, 32)
		tu.press("ctrl+p")
		tu.typeText(word)
		if len(tu.pal.hits) == 0 || tu.pal.hits[0].ActionID != "quit" {
			t.Fatalf("%q should find the close action first: %+v", word, tu.pal.hits)
		}
		tu.press("enter")
		if !tu.quiting {
			t.Fatalf("%q then enter should close the app", word)
		}
	}
}

func TestMiniMode(t *testing.T) {
	tu, _ := newTestTUI(t, 60, 20)
	tu.press("enter")
	tu.press("M")
	if !tu.mini {
		t.Fatal("M enters mini mode")
	}
	lines := strings.Split(tu.screen(), "\n")
	if len(lines) != 20 {
		t.Fatalf("mini fits: %d lines", len(lines))
	}
	tu.press("space")
	tu.press("M")
	if tu.mini {
		t.Fatal("M leaves mini mode")
	}
}

// TestSnapshots writes the screens as text when MUSAIK_SNAPSHOT_DIR is set.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tu, _ := newTestTUI(t, 120, 34)
	tu.press("down", "down", "enter")
	tu.press("f")
	write("songs-120", tu.screen())
	tu.press("2", "down", "enter")
	write("artist-120", tu.screen())
	tu.press("esc", "3", "enter")
	write("album-120", tu.screen())
	tu.press("esc", "4")
	write("queue-120", tu.screen())
	tu.press("7")
	write("playlist-120", tu.screen())
	tu.press("ctrl+p")
	tu.typeText("a")
	write("palette-120", tu.screen())
	tu.press("esc", ",", "]", "]", "]")
	write("settings-120", tu.screen())
	tu.press("esc", "?")
	write("help-120", tu.screen())
	tu.press("esc", "N")
	write("modal-120", tu.screen())
	tu.press("esc", "M")
	write("mini-120", tu.screen())

	for _, size := range [][2]int{{80, 24}, {64, 18}} {
		tu, _ := newTestTUI(t, size[0], size[1])
		tu.press("enter")
		write(fmt.Sprintf("songs-%d", size[0]), tu.screen())
	}
}
