package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"musaik/internal/library"
	"musaik/internal/player"
)

// TestRenderPlayerBarStates writes snapshots of the player bar buttons. It
// only runs when MUSAIK_SNAPSHOT_DIR is set.
func TestRenderPlayerBarStates(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	a := &app{store: library.NewStore(), player: player.New(), repeatMode: repeatModeOff, volumeLevel: 80}
	view := func(c *ui.Context) {
		a.applyTheme(c)
		ui.Column(c).Fill().Background(sonoraCanvas).Children(func() {
			ui.Spacer(c)
			a.viewControls(c)
		})
	}
	tt := ui.NewTester(view, 1100, 120)
	tt.SetScale(2)
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "bar-idle.png"), tt.Image())
	for name, x := range map[string]float32{"prev": 543, "play": 589, "next": 635, "mini": 877} {
		tt.Move(x, 74)
		tt.Frame()
		writePNG(t, filepath.Join(dir, "bar-hover-"+name+".png"), tt.Image())
	}
	tt.Press(589, 74)
	tt.Move(5, 5) // leave before releasing: a click would start playback
	tt.Release(5, 5)
	tt.Move(589, 74)
	tt.Press(589, 74)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "bar-pressed-play.png"), tt.Image())
	tt.Move(5, 5)
	tt.Release(5, 5)
}
