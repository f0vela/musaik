package main

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
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
