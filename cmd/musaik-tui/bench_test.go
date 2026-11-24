package main

import (
	"os"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
	"musaik/internal/model"
	"musaik/internal/player"
)

// benchTUI loads the library in $MUSAIK_BENCH_HOME, a real-sized one, and skips
// the benchmark without it.
func benchTUI(b *testing.B) *tui {
	b.Helper()
	home := os.Getenv("MUSAIK_BENCH_HOME")
	if home == "" {
		b.Skip("MUSAIK_BENCH_HOME not set")
	}
	b.Setenv("HOME", home)
	fp := &fakePlayer{volume: 80, events: make(chan player.Event, 4)}
	tu := newTUI(core.New(fp), false)
	tu.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	tu.press("enter")
	return tu
}

// BenchmarkTick is what the app does four times a second while it sits idle.
func BenchmarkTick(b *testing.B) {
	tu := benchTUI(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tu.Update(tickMsg(time.Now()))
		_ = tu.View()
	}
}

func BenchmarkViewSongs(b *testing.B) {
	tu := benchTUI(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tu.View()
	}
}

func BenchmarkScrollDown(b *testing.B) {
	tu := benchTUI(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tu.press("j")
		_ = tu.View()
	}
}

func BenchmarkViewArtists(b *testing.B) {
	tu := benchTUI(b)
	tu.press("2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tu.View()
	}
}

func BenchmarkViewAlbums(b *testing.B) {
	tu := benchTUI(b)
	tu.press("3")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tu.View()
	}
}

// BenchmarkMiniVisualizerFrame is one frame of the mini player with the
// visualizer going: what the app does 20 times a second while it plays.
func BenchmarkMiniVisualizerFrame(b *testing.B) {
	for _, style := range []string{"bars", "mirror", "wave"} {
		b.Run(style, func(b *testing.B) {
			b.Setenv("HOME", b.TempDir())
			fp := &fakePlayer{volume: 80, events: make(chan player.Event, 4)}
			eng := core.New(fp)
			eng.Store.SetSongs("/music", benchSongs(40))
			tu := newTUI(eng, false)
			tu.Update(tea.WindowSizeMsg{Width: 100, Height: 36})
			tu.viz.src, tu.viz.style = &fakeViz{frame: frameOf(170)}, style
			tu.eng.PlaySongByID("id0")
			tu.press("M")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tu.Update(tickMsg(time.Now()))
				tu.dirty = true
				_ = tu.View()
			}
		})
	}
}

func benchSongs(n int) []model.Song {
	out := make([]model.Song, n)
	for i := range out {
		out[i] = model.Song{ID: "id" + strconv.Itoa(i), Path: "/music/" + strconv.Itoa(i) + ".flac", Title: "Song " + strconv.Itoa(i),
			Artist: "Artist", Album: "Album", TrackNum: i + 1, Duration: 3 * time.Minute}
	}
	return out
}
