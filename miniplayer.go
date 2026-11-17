package main

import (
	"bytes"
	"image"
	"math"
	"os"
	"sort"
	"time"

	"github.com/dhowden/tag"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"musaik/internal/model"
	"musaik/internal/player"
)

const (
	miniArtSize   = 256
	miniWidth     = 460
	miniHeight    = 218
	miniArtPoints = 124
	// miniOpacityMin keeps the controls readable over any desktop.
	miniOpacityMin = 30
)

var (
	iconMiniPlayer = mustIconoir("mini-player.svg")
	iconExpand     = mustIconoir("expand.svg")

	miniText      = ui.Hex("#FFFFFF")
	miniTextMuted = ui.RGBA(255, 255, 255, 0.70)
	miniTextDim   = ui.RGBA(255, 255, 255, 0.55)

	// miniFallbackPalette is used when the song has no embedded artwork.
	miniFallbackPalette = []ui.Color{ui.Hex("#243B6B"), ui.Hex("#5B3A8C"), ui.Hex("#1F6F8B"), ui.Hex("#8C3A5B")}
)

type miniArtwork struct {
	bmp    *ui.Bitmap
	colors []ui.Color
}

func (a *app) toggleMiniPlayer() {
	if a.miniWindow != nil {
		a.closeMiniPlayer()
		return
	}
	a.miniWindow = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Musaik",
		Width:             miniWidth,
		Height:            miniHeight,
		DisableResize:     true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		TitleBarStyle:     mygo.TitleBarHiddenInset,
		TitleBarHeight:    28,
		Transparent:       true,
		StateKey:          "mini",
		Content:           ui.View(a.viewMiniPlayer),
	})
	a.miniWindow.OnClosed(func() {
		a.miniWindow = nil
		a.showMainWindow()
	})
	// The main window stays hidden while the mini player is open.
	if a.window != nil {
		a.window.Hide()
	}
}

// showMainWindow brings the main window back, unless the app is going away.
func (a *app) showMainWindow() {
	if a.quitting || a.window == nil {
		return
	}
	a.window.Show()
	a.window.Focus()
}

func (a *app) closeMiniPlayer() {
	if a.miniWindow != nil {
		a.miniWindow.Close()
	}
}

// refreshMiniArtwork reloads the artwork and palette when the album changes.
func (a *app) refreshMiniArtwork(song model.Song, ok bool) {
	key := ""
	if ok {
		key = songCoverKey(song)
	}
	if a.miniReady && key == a.miniCoverKey {
		return
	}
	a.miniReady = true
	a.miniCoverKey = key
	art := miniArtwork{}
	if ok {
		art = loadMiniArtwork(song.Path)
	}
	if len(art.colors) == 0 {
		art.colors = miniFallbackPalette
	}
	a.miniArt = art.bmp
	a.miniColors = art.colors
}

// applyMiniTheme gives controls a white-on-glass look that works over any
// artwork color, whatever the system appearance is.
func applyMiniTheme(c *ui.Context) {
	t := *c.Theme()
	t.Dark = true
	t.Text = miniText
	t.TextMuted = miniTextMuted
	t.Accent = miniText
	t.AccentHover = miniText
	t.AccentPressed = ui.RGBA(255, 255, 255, 0.80)
	t.AccentText = ui.Hex("#111827")
	t.Surface = ui.RGBA(255, 255, 255, 0.26)
	t.SurfaceHover = ui.RGBA(255, 255, 255, 0.34)
	t.SurfacePressed = ui.RGBA(255, 255, 255, 0.40)
	t.Border = ui.RGBA(255, 255, 255, 0.30)
	t.Focus = ui.RGBA(255, 255, 255, 0.35)
	t.FontSize = themeFontSize(t.FontSize)
	c.SetTheme(&t)
}

func (a *app) viewMiniPlayer(c *ui.Context) {
	applyMiniTheme(c)
	a.consumePlayerEvents()
	c.Root().Background(ui.Transparent)
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyM, a.closeMiniPlayer)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.closeMiniPlayer)
	c.OnShortcut(0, ui.KeyEscape, a.closeMiniPlayer)
	c.OnShortcut(0, ui.KeySpace, a.togglePlayPause)

	song, hasSong := a.currentPlayingSong()
	a.refreshMiniArtwork(song, hasSong)

	state := a.player.State()
	playing := state.Playing && !state.Paused
	if state.Duration > 0 {
		a.miniSlider = state.Position.Seconds()
	} else {
		a.miniSlider = 0
	}

	title, artist := "Sin reproducción", "Musaik"
	liked := false
	if hasSong {
		title = song.Title
		artist = song.Artist
		if artist == "" {
			artist = "Unknown Artist"
		}
		liked = a.store.Favorites[song.ID]
	}

	ui.Column(c).Fill().Padding(8, 18, 14, 18).Gap(8).DragWindow().Draw(a.paintMiniBackground).Children(func() {
		// Top bar: leaves room for the window buttons on the left.
		ui.Row(c).Height(30).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			back := miniButton(c, 26, ui.Transparent, miniTextMuted, iconExpand, 14).Tooltip("Volver al reproductor (⇧⌘M)")
			if back.Clicked() {
				a.closeMiniPlayer()
			}
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			a.miniCover(c)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).SingleLine().FontSize(fs(19)).FontWeight(700).TextColor(miniText)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, artist).SingleLine().FontSize(fs(13)).TextColor(miniTextMuted).Grow(1)
					heart := miniText
					if liked {
						heart = ui.Hex("#FF5C9A")
					}
					likeBtn := miniButton(c, 28, ui.RGBA(255, 255, 255, 0.16), heart, iconHeart, 14)
					if likeBtn.Clicked() && hasSong {
						a.toggleFavorite(song)
					}
				})
				ui.Spacer(c)
				a.miniTransport(c, playing)
			})
		})
		a.miniProgress(c, state)
	})
}

func (a *app) miniCover(c *ui.Context) {
	box := ui.Box(c).Width(miniArtPoints).Height(miniArtPoints).Radius(14).
		Background(ui.RGBA(255, 255, 255, 0.12)).
		Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.35)).
		Clip()
	box.Children(func() {
		if a.miniArt != nil {
			ui.Image(c, a.miniArt).Fill().Fit(ui.Cover)
			return
		}
		ui.Column(c).Fill().Center().Children(func() {
			ui.Icon(c, iconMusic).Size(40, 40).TextColor(miniTextDim)
		})
	})
}

func (a *app) miniTransport(c *ui.Context, playing bool) {
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		shuffleBase, shuffleColor := ui.Transparent, miniTextDim
		if a.shuffle {
			shuffleBase, shuffleColor = ui.RGBA(255, 255, 255, 0.20), miniText
		}
		shuffle := miniButton(c, 28, shuffleBase, shuffleColor, iconShuffle, 14)
		if shuffle.Clicked() {
			a.shuffle = !a.shuffle
			_ = a.saveSettings()
		}

		prev := miniButton(c, 32, ui.Transparent, miniText, iconPrev, 17)
		if prev.Clicked() {
			a.playPrevious()
		}

		playIcon := iconPlay
		if playing {
			playIcon = iconPause
		}
		play := miniButton(c, 46, miniText, ui.Hex("#111827"), playIcon, 20)
		if play.Clicked() {
			a.togglePlayPause()
		}

		next := miniButton(c, 32, ui.Transparent, miniText, iconNext, 17)
		if next.Clicked() {
			a.playNext()
		}

		repeatBase, repeatColor, repeatIcon := ui.Transparent, miniTextDim, iconRepeat
		if a.repeatMode != repeatModeOff {
			repeatBase, repeatColor = ui.RGBA(255, 255, 255, 0.20), miniText
		}
		if a.repeatMode == repeatModeOne {
			repeatIcon = iconRepeat1
		}
		repeat := miniButton(c, 28, repeatBase, repeatColor, repeatIcon, 14)
		if repeat.Clicked() {
			a.cycleRepeatMode()
			_ = a.saveSettings()
		}
	})
}

func (a *app) miniProgress(c *ui.Context, state player.State) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, formatDuration(state.Position)).Font("monospace").FontSize(fs(11)).TextColor(miniTextMuted)
		if state.Duration > 0 {
			if ui.Slider(c.Key("mini-progress"), &a.miniSlider, 0, state.Duration.Seconds()).Step(1).Grow(1).Changed() {
				if err := a.player.SeekTo(time.Duration(a.miniSlider * float64(time.Second))); err != nil {
					a.status = "No se pudo mover la canción: " + err.Error()
				} else {
					a.updateNowPlaying()
				}
			}
		} else {
			ui.Box(c).Grow(1)
		}
		ui.Text(c, formatDuration(state.Duration)).Font("monospace").FontSize(fs(11)).TextColor(miniTextMuted)
	})
}

// miniButton is a round icon button of the mini player with hover and pressed
// states derived from base: transparent, translucent white or solid white.
func miniButton(c *ui.Context, size float32, base, fg ui.Color, icon *ui.SVG, iconSize float32) ui.Element {
	var hover, pressed ui.Color
	if base.A == 255 {
		hover, pressed = base.Mix(ui.RGB(0, 0, 0), 0.08), base.Mix(ui.RGB(0, 0, 0), 0.20)
	} else {
		a := float32(base.A) / 255
		hover = ui.RGBA(255, 255, 255, min(a+0.16, 1))
		pressed = ui.RGBA(255, 255, 255, min(a+0.28, 1))
	}
	return iconButton(c, size, base, hover, pressed, fg, icon, iconSize)
}

// paintMiniBackground draws soft blobs of the artwork's colors over a dark
// base. mygo has no radial gradient or blur primitive, but a shadow cast by a
// 1pt box with a large spread and blur is a blurred disc.
func (a *app) paintMiniBackground(p *ui.Painter, r ui.Rect) {
	cols := a.miniColors
	if len(cols) == 0 {
		cols = miniFallbackPalette
	}
	op := float32(a.miniOpacity) / 100
	if op <= 0 || op > 1 {
		op = 1
	}
	p.Fill(r, cols[0].Mix(ui.RGB(0, 0, 0), 0.74).Alpha(op), 0)
	blobs := []struct {
		fx, fy, size float32
		color        int
	}{
		{0.10, 0.20, 95, 0},
		{0.88, 0.12, 85, 1},
		{0.50, 0.55, 70, 2},
		{0.92, 0.95, 90, 3},
		{0.18, 0.98, 80, 1},
		{0.65, 0.05, 60, 3},
	}
	for _, b := range blobs {
		col := cols[b.color%len(cols)].Alpha(0.85 * op)
		cx := r.X + b.fx*r.W
		cy := r.Y + b.fy*r.H
		p.Shadow(ui.Rect{X: cx - 0.5, Y: cy - 0.5, W: 1, H: 1}, 0.5, 0, 0, b.size, b.size, col)
	}
	p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(0, 0, 0, 0.05*op), To: ui.RGBA(0, 0, 0, 0.38*op), Angle: 180}, 0)
}

func loadMiniArtwork(path string) miniArtwork {
	f, err := os.Open(path)
	if err != nil {
		return miniArtwork{}
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		return miniArtwork{}
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return miniArtwork{}
	}
	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return miniArtwork{}
	}
	scaled := scaleImageSmooth(img, miniArtSize)
	return miniArtwork{bmp: ui.NewBitmap(scaled), colors: paletteFromImage(scaled)}
}

func scaleImageSmooth(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || (w <= maxDim && h <= maxDim) {
		return src
	}
	nw, nh := maxDim, maxDim
	if w >= h {
		nh = max(1, h*maxDim/w)
	} else {
		nw = max(1, w*maxDim/h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

type colorBucket struct {
	r, g, b, n float64
	weight     float64
}

// paletteFromImage returns up to four distinct, vivid colors of img, toned
// down so white text stays readable on top of them.
func paletteFromImage(img image.Image) []ui.Color {
	b := img.Bounds()
	step := max(1, max(b.Dx(), b.Dy())/32)
	buckets := map[int]*colorBucket{}
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r, g, bl := float64(r16>>8), float64(g16>>8), float64(b16>>8)
			mx, mn := math.Max(r, math.Max(g, bl)), math.Min(r, math.Min(g, bl))
			if mx < 20 {
				continue
			}
			key := int(r)>>4<<8 | int(g)>>4<<4 | int(bl)>>4
			bk := buckets[key]
			if bk == nil {
				bk = &colorBucket{}
				buckets[key] = bk
			}
			bk.r += r
			bk.g += g
			bk.b += bl
			bk.n++
			bk.weight += 0.15 + (mx-mn)/mx
		}
	}
	list := make([]*colorBucket, 0, len(buckets))
	for _, bk := range buckets {
		list = append(list, bk)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].weight > list[j].weight })

	var picked []ui.Color
	for _, bk := range list {
		c := ui.RGB(uint8(bk.r/bk.n), uint8(bk.g/bk.n), uint8(bk.b/bk.n))
		far := true
		for _, p := range picked {
			if colorDistance(c, p) < 70 {
				far = false
				break
			}
		}
		if far {
			picked = append(picked, c)
		}
		if len(picked) == 4 {
			break
		}
	}
	if len(picked) == 0 {
		return nil
	}
	for i := 0; len(picked) < 4; i++ {
		base := picked[i%len(picked)]
		if i%2 == 0 {
			picked = append(picked, base.Mix(ui.RGB(0, 0, 0), 0.35))
		} else {
			picked = append(picked, base.Mix(ui.RGB(255, 255, 255), 0.25))
		}
	}
	for i := range picked {
		picked[i] = limitLuminance(picked[i], 0.62)
	}
	return picked
}

func colorDistance(a, b ui.Color) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func limitLuminance(c ui.Color, maxLum float64) ui.Color {
	lum := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	if lum <= maxLum {
		return c
	}
	return c.Mix(ui.RGB(0, 0, 0), float32(1-maxLum/lum))
}
