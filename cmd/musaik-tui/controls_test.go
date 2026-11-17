package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
)

var transportSymbols = map[string][]string{
	"shuffle": {"⇄"},
	"prev":    {"◀◀"},
	"play":    {"▶", "❚❚"},
	"next":    {"▶▶"},
	"repeat":  {"↻"},
}

var transportOrder = []string{"shuffle", "prev", "play", "next", "repeat"}

func zoneOf(tu *tui, kind string) (zone, bool) {
	for _, z := range tu.zones {
		if z.kind == kind {
			return z, true
		}
	}
	return zone{}, false
}

func click(tu *tui, z zone) {
	tu.Update(tea.MouseClickMsg{X: z.x + z.w/2, Y: z.y, Button: tea.MouseLeft})
}

// TestControlsAreWhereTheyAreDrawn checks, at every size and in the mini player,
// that the cells of each button's click zone hold that button's symbol: a zone
// that drifts from its drawing makes "next" press "repeat".
func TestControlsAreWhereTheyAreDrawn(t *testing.T) {
	type size struct {
		w, h int
		mini bool
	}
	sizes := []size{
		{160, 44, false}, {140, 40, false}, {120, 34, false}, {100, 30, false}, {90, 28, false},
		{80, 24, false}, {70, 22, false}, {64, 20, false}, {60, 18, false}, {50, 14, false}, {44, 12, false},
		{120, 34, true}, {100, 16, true}, {64, 12, true}, {60, 20, true}, {44, 14, true},
	}
	for _, sz := range sizes {
		tu, _ := newTestTUI(t, sz.w, sz.h)
		tu.press("enter")
		if sz.mini {
			tu.press("M")
		}
		lines := strings.Split(tu.screen(), "\n")
		prevEnd := -1
		for _, kind := range transportOrder {
			z, ok := zoneOf(tu, kind)
			if !ok {
				t.Fatalf("%dx%d mini=%v: no zone for %s", sz.w, sz.h, sz.mini, kind)
			}
			if z.y >= len(lines) {
				t.Fatalf("%dx%d: %s zone is below the screen", sz.w, sz.h, kind)
			}
			row := []rune(lines[z.y])
			if z.x+z.w > len(row) {
				// cut off by a terminal narrower than the controls: nothing to check
				continue
			}
			cell := string(row[z.x : z.x+z.w])
			found := false
			for _, sym := range transportSymbols[kind] {
				found = found || strings.Contains(cell, sym)
			}
			if !found {
				t.Fatalf("%dx%d mini=%v: the %s zone covers %q, not its symbol (zone x=%d w=%d y=%d)", sz.w, sz.h, sz.mini, kind, cell, z.x, z.w, z.y)
			}
			if z.x < prevEnd {
				t.Fatalf("%dx%d: the %s zone overlaps the one before", sz.w, sz.h, kind)
			}
			prevEnd = z.x + z.w
		}
	}
}

// TestControlsDoOnlyTheirOwnThing clicks each button and checks the one thing it
// should change.
func TestControlsDoOnlyTheirOwnThing(t *testing.T) {
	for _, size := range [][2]int{{140, 40}, {100, 30}, {80, 24}, {60, 18}, {50, 14}} {
		tu, fp := newTestTUI(t, size[0], size[1])
		tu.eng.PlaySongByID("id5")
		tu.View()

		z, _ := zoneOf(tu, "next")
		click(tu, z)
		if tu.eng.PlayingID != "id6" || tu.eng.Repeat != core.RepeatOff || tu.eng.Shuffle {
			t.Fatalf("%v: next must only skip: playing=%s repeat=%s shuffle=%v", size, tu.eng.PlayingID, tu.eng.Repeat, tu.eng.Shuffle)
		}
		tu.View()
		z, _ = zoneOf(tu, "prev")
		click(tu, z)
		if tu.eng.PlayingID != "id5" || tu.eng.Repeat != core.RepeatOff || tu.eng.Shuffle {
			t.Fatalf("%v: previous must only go back: playing=%s", size, tu.eng.PlayingID)
		}
		tu.View()
		z, _ = zoneOf(tu, "play")
		click(tu, z)
		if tu.eng.PlayingID != "id5" || !fp.paused {
			t.Fatalf("%v: play/pause must pause and stay on the song: playing=%s paused=%v", size, tu.eng.PlayingID, fp.paused)
		}
		tu.View()
		z, _ = zoneOf(tu, "play")
		click(tu, z)
		if fp.paused || tu.eng.PlayingID != "id5" {
			t.Fatalf("%v: and press again to resume", size)
		}
		tu.View()
		z, _ = zoneOf(tu, "repeat")
		click(tu, z)
		if tu.eng.Repeat != core.RepeatAll || tu.eng.Shuffle || tu.eng.PlayingID != "id5" {
			t.Fatalf("%v: repeat must only cycle repeat: %s", size, tu.eng.Repeat)
		}
		tu.View()
		z, _ = zoneOf(tu, "shuffle")
		click(tu, z)
		if !tu.eng.Shuffle || tu.eng.Repeat != core.RepeatAll || tu.eng.PlayingID != "id5" {
			t.Fatalf("%v: shuffle must only toggle shuffle: shuffle=%v repeat=%s", size, tu.eng.Shuffle, tu.eng.Repeat)
		}
	}
}

// The buttons keep their place whatever the player is doing: ▶ and ❚❚ have the
// same slot, and "↻1" fits where "↻" does.
func TestControlsDoNotMove(t *testing.T) {
	tu, fp := newTestTUI(t, 120, 34)
	tu.eng.PlaySongByID("id0")
	tu.View()
	positions := func() map[string][2]int {
		out := map[string][2]int{}
		for _, k := range transportOrder {
			z, _ := zoneOf(tu, k)
			out[k] = [2]int{z.x, z.w}
		}
		return out
	}
	base := positions()
	fp.paused = true
	tu.eng.Repeat = core.RepeatOne
	tu.eng.Shuffle = true
	tu.dirty = true
	tu.View()
	if got := positions(); got != nil {
		for k, v := range base {
			if got[k] != v {
				t.Fatalf("%s moved from %v to %v when the state changed", k, v, got[k])
			}
		}
	}
	if w := base["repeat"][0] + base["repeat"][1] - base["shuffle"][0]; w != transportW {
		t.Fatalf("the row is %d cells wide, want %d", w, transportW)
	}
}

// Only the heart toggles the like; the title next to it is not a button.
func TestOnlyTheHeartLikes(t *testing.T) {
	tu, _ := newTestTUI(t, 120, 34)
	tu.eng.PlaySongByID("id0")
	tu.View()
	heart, ok := zoneOf(tu, "like")
	if !ok || heart.w != 1 {
		t.Fatalf("the heart is a one-cell zone: %+v", heart)
	}
	tu.Update(tea.MouseClickMsg{X: heart.x - 3, Y: heart.y, Button: tea.MouseLeft})
	if tu.eng.Store.Favorites["id0"] {
		t.Fatal("clicking the title must not like the song")
	}
	click(tu, heart)
	if !tu.eng.Store.Favorites["id0"] {
		t.Fatal("clicking the heart likes it")
	}
}
