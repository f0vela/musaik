package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"musaik/internal/library"
	"musaik/internal/lyrics"
	"musaik/internal/model"
	"musaik/internal/player"
)

// lyricsApp is an app with one song selected and the given lookup result, so
// the lyrics window can be drawn without the network (the lookup itself is
// tested in internal/lyrics and internal/core).
func lyricsApp(res *lyricsResult) *app {
	store := library.NewStore()
	store.SetSongs("/music", []model.Song{{ID: "s1", Path: "/music/a.flac", Title: "Canción", Artist: "Banda", Album: "Disco", Duration: 200 * time.Second}})
	a := &app{store: store, player: player.New(), selectedID: "s1", volumeLevel: 80}
	a.lyrics = lyricsPanel{open: true, cache: map[string]*lyricsResult{}}
	if res != nil {
		a.lyrics.cache["s1"] = res
	}
	return a
}

func lyricsTester(a *app) *ui.Tester {
	view := func(c *ui.Context) {
		a.applyTheme(c)
		ui.Column(c).Fill().Background(sonoraCanvas)
		a.viewLyricsModal(c)
	}
	tt := ui.NewTester(view, 900, 760)
	tt.SetDark(true)
	tt.Frame()
	return tt
}

func TestLyricsWindowStates(t *testing.T) {
	var lines []lyrics.Line
	for i := 0; i < 40; i++ {
		lines = append(lines, lyrics.Line{At: time.Duration(i*5) * time.Second, Text: fmt.Sprintf("Verso número %d", i)})
	}
	cases := []struct {
		name string
		res  *lyricsResult
		want []string
	}{
		{"synced", &lyricsResult{state: lyricsReady, data: &lyrics.Lyrics{Synced: lines}}, []string{"Letra", "Canción — Banda", "Verso número 0", "Verso número 1"}},
		{"plain", &lyricsResult{state: lyricsReady, data: &lyrics.Lyrics{Plain: "Primera línea\n\nSegunda línea"}}, []string{"Primera línea", "Segunda línea"}},
		{"instrumental", &lyricsResult{state: lyricsReady, data: &lyrics.Lyrics{Instrumental: true}}, []string{"Instrumental"}},
		{"none", &lyricsResult{state: lyricsNone}, []string{"No hay letra para esta canción", "Buscar de nuevo"}},
		{"failed", &lyricsResult{state: lyricsFailed, err: "sin conexión", at: time.Now()}, []string{"No se pudo buscar la letra", "sin conexión", "Reintentar"}},
		{"loading", &lyricsResult{state: lyricsLoading, at: time.Now()}, []string{"Buscando letra…"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := lyricsTester(lyricsApp(tc.res))
			for _, want := range tc.want {
				if !tt.HasText(want) {
					t.Errorf("missing %q in %q", want, strings.Join(tt.Texts(), " | "))
				}
			}
			if dir := os.Getenv("MUSAIK_SNAPSHOT_DIR"); dir != "" {
				writePNG(t, filepath.Join(dir, "lyrics-"+tc.name+".png"), tt.Image())
			}
		})
	}
}

func TestLyricsRetryForgetsTheFailure(t *testing.T) {
	a := lyricsApp(&lyricsResult{state: lyricsFailed, err: "sin conexión", at: time.Now()})
	tt := lyricsTester(a)
	if err := tt.Click("Reintentar"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if r := a.lyrics.cache["s1"]; r != nil && r.state == lyricsFailed {
		t.Fatalf("retry kept the failed result: %+v", r)
	}
}

func TestLyricsToggleAndAction(t *testing.T) {
	a := lyricsApp(nil)
	a.lyrics.open = false
	a.applyQuickAction("lyrics")
	if !a.lyrics.open {
		t.Fatal("the lyrics action did not open the window")
	}
	a.toggleLyrics()
	if a.lyrics.open {
		t.Fatal("toggling did not close it")
	}
	a.actionQuery = "letra"
	found := false
	for _, h := range a.actionHits() {
		if h.ActionID == "lyrics" {
			found = true
		}
	}
	if !found {
		t.Fatal("the action bar does not offer the lyrics")
	}
}
