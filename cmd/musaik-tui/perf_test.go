package main

import (
	"strings"
	"testing"
	"time"

	"musaik/internal/player"
)

func TestIdleTickReusesTheFrame(t *testing.T) {
	tu, fp := newTestTUI(t, 110, 32)
	tu.cacheViews = true
	tu.View()
	renders := tu.renders
	for i := 0; i < 5; i++ {
		tu.Update(tickMsg(time.Now()))
		tu.View()
	}
	if tu.renders != renders {
		t.Fatalf("idle ticks changed nothing, so the frame is reused: %d extra renders", tu.renders-renders)
	}

	tu.press("n") // any key redraws
	tu.View()
	if tu.renders != renders+1 {
		t.Fatalf("a key redraws: %d", tu.renders-renders)
	}

	// While a song plays the clock moves: a tick that moves it redraws, one that does not, does not.
	tu.eng.PlaySongByID("id0")
	tu.Update(tickMsg(time.Now()))
	tu.View()
	before := tu.renders
	fp.pos += 3 * time.Second
	tu.Update(tickMsg(time.Now()))
	tu.View()
	if tu.renders != before+1 {
		t.Fatal("a moved clock redraws")
	}
	tu.Update(tickMsg(time.Now()))
	tu.View()
	if tu.renders != before+1 {
		t.Fatal("the same second does not")
	}
	fp.paused = true
	tu.Update(tickMsg(time.Now()))
	tu.View()
	if tu.renders != before+2 || !strings.Contains(tu.screen(), "▶") {
		t.Fatal("pausing is noticed on the next tick")
	}
}

func TestPlayerEventsArriveAsMessages(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.eng.PlaySongByID("id0")
	tu.Update(playerEventMsg(player.Event{Type: player.EventTrackEnded}))
	if tu.eng.PlayingID != "id1" {
		t.Fatalf("a finished track moves on to the next one: %s", tu.eng.PlayingID)
	}
	if !tu.dirty {
		t.Fatal("and the screen is redrawn")
	}
}

func TestTickSlowsDownWhenIdle(t *testing.T) {
	tu, fp := newTestTUI(t, 110, 32)
	if tu.isPlaying() {
		t.Fatal("nothing plays yet")
	}
	tu.eng.PlaySongByID("id0")
	if !tu.isPlaying() {
		t.Fatal("playing")
	}
	fp.paused = true
	if tu.isPlaying() {
		t.Fatal("paused is idle")
	}
	if tickIdle <= tickPlaying {
		t.Fatal("the idle tick is the slow one")
	}
}

func TestSizeIsCountedInTheBackground(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	posted := make(chan func(), 1)
	tu.eng.Post = func(fn func()) { posted <- fn }
	tu.refreshStats()
	if tu.stats.size != "…" {
		t.Fatalf("the size is pending, not computed on the spot: %q", tu.stats.size)
	}
	select {
	case fn := <-posted:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("the total arrives through Post")
	}
	if tu.stats.size == "…" {
		t.Fatal("the pending mark is replaced")
	}
	// A result for a library that has since changed is dropped.
	tu.refreshStats()
	tu.statsKey = "another library"
	(<-posted)()
	if tu.stats.size != "…" {
		t.Fatal("a stale total must not overwrite the current one")
	}
}

func TestFindSongIndex(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	e := tu.eng
	for _, id := range []string{"id0", "id17", "id29"} {
		i, s, ok := e.FindSong(id)
		if !ok || s.ID != id || e.Store.Songs[i].ID != id {
			t.Fatalf("%s: %d %v", id, i, ok)
		}
	}
	if _, _, ok := e.FindSong("nope"); ok {
		t.Fatal("unknown ids are not found")
	}
	// A library swapped for another of the same size must not return stale rows.
	songs := append(e.Store.Songs[:0:0], e.Store.Songs...)
	for i := range songs {
		songs[i].ID = "new" + songs[i].ID
	}
	e.Store.SetSongs("/music", songs)
	if _, s, ok := e.FindSong("newid5"); !ok || s.ID != "newid5" {
		t.Fatal("the index follows a replaced library")
	}
	if _, _, ok := e.FindSong("id5"); ok {
		t.Fatal("and forgets the old one")
	}
}
