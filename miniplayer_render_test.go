package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"musaik/internal/library"
	"musaik/internal/player"
)

// TestRenderMiniPlayer writes PNG snapshots of the mini player for the first
// MUSAIK_SNAPSHOT_COUNT songs of the saved library that have artwork. It only
// runs when MUSAIK_SNAPSHOT_DIR is set.
func TestRenderMiniPlayer(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	store := library.NewStore()
	_ = store.LoadState()
	done := 0
	seen := map[string]bool{}
	for _, song := range store.Songs {
		if seen[songCoverKey(song)] {
			continue
		}
		seen[songCoverKey(song)] = true
		art := loadMiniArtwork(song.Path)
		if art.bmp == nil {
			continue
		}
		a := &app{store: store, player: player.New(), miniReady: true, miniArt: art.bmp, miniColors: art.colors}
		// The player is idle in tests, so the progress bar is rendered by hand.
		view := func(c *ui.Context) {
			applyMiniTheme(c)
			ui.Column(c).Fill().Padding(150, 18, 14, 18).Draw(a.paintMiniBackground).Children(func() {
				a.miniSlider = 84
				a.miniProgress(c, player.State{Position: 84 * time.Second, Duration: 232 * time.Second})
			})
		}
		img := ui.Render(a.viewMiniPlayer, miniWidth, miniHeight, 2)
		bar := ui.Render(view, miniWidth, miniHeight, 2)
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("mini-%d.png", done)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		fb, err := os.Create(filepath.Join(dir, fmt.Sprintf("mini-%d-bar.png", done)))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(fb, bar); err != nil {
			t.Fatal(err)
		}
		fb.Close()
		t.Logf("%d: %s — %s %v", done, song.Artist, song.Title, art.colors)
		if done++; done == 4 {
			break
		}
	}
	if done == 0 {
		t.Skip("no songs with artwork in the saved library")
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestRenderMiniPlayerStates writes snapshots of hover, pressed and a
// translucent background. It only runs when MUSAIK_SNAPSHOT_DIR is set.
func TestRenderMiniPlayerStates(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	store := library.NewStore()
	_ = store.LoadState()
	var art miniArtwork
	for _, song := range store.Songs {
		if art = loadMiniArtwork(song.Path); art.bmp != nil {
			break
		}
	}
	if art.bmp == nil {
		t.Skip("no songs with artwork in the saved library")
	}
	newApp := func(opacity float64) *app {
		return &app{store: store, player: player.New(), miniReady: true, miniArt: art.bmp, miniColors: art.colors,
			miniOpacity: opacity, repeatMode: repeatModeOff}
	}

	a := newApp(100)
	tt := ui.NewTester(a.viewMiniPlayer, miniWidth, miniHeight)
	tt.SetScale(2)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "state-idle.png"), tt.Image())
	tt.Move(269, 152)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "state-hover-play.png"), tt.Image())
	tt.Press(269, 152)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "state-pressed-play.png"), tt.Image())
	tt.Move(5, 5) // leave the button before releasing: a click would start playback
	tt.Release(5, 5)
	tt.Move(216, 152)
	tt.Frame()
	writePNG(t, filepath.Join(dir, "state-hover-prev.png"), tt.Image())

	b := newApp(50)
	img := ui.Render(b.viewMiniPlayer, miniWidth, miniHeight, 2)
	backdrop := image.NewRGBA(img.Bounds())
	draw.Draw(backdrop, backdrop.Bounds(), &image.Uniform{color.RGBA{R: 255, G: 0, B: 255, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(backdrop, backdrop.Bounds(), img, image.Point{}, draw.Over)
	writePNG(t, filepath.Join(dir, "state-opacity50.png"), backdrop)
}
