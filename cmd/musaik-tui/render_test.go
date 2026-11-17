package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
)

// ansiToImage draws a screen cell by cell: a cell is cw x ch pixels, filled with
// its background; the upper half block shows its foreground on the top half; any
// other glyph is marked with a small block of its foreground color. It lets a
// test look at colors, which text snapshots lose.
func ansiToImage(s string, cw, ch int, defBg color.RGBA) *image.RGBA {
	lines := strings.Split(s, "\n")
	width := 0
	for _, l := range lines {
		width = max(width, ansi.StringWidth(l))
	}
	img := image.NewRGBA(image.Rect(0, 0, width*cw, len(lines)*ch))
	draw.Draw(img, img.Bounds(), &image.Uniform{defBg}, image.Point{}, draw.Src)
	defFg := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	for y, line := range lines {
		fg, bg := defFg, defBg
		x := 0
		rs := []rune(line)
		for i := 0; i < len(rs); i++ {
			if rs[i] == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
				j := i + 2
				for j < len(rs) && !(rs[j] >= '@' && rs[j] <= '~') {
					j++
				}
				if j < len(rs) && rs[j] == 'm' {
					fg, bg = applySGR(string(rs[i+2:j]), fg, bg, defFg, defBg)
				}
				i = j
				continue
			}
			w := ansi.StringWidth(string(rs[i]))
			if w == 0 {
				continue
			}
			cell := image.Rect(x*cw, y*ch, (x+w)*cw, (y+1)*ch)
			draw.Draw(img, cell, &image.Uniform{bg}, image.Point{}, draw.Src)
			switch rs[i] {
			case '▀':
				draw.Draw(img, image.Rect(cell.Min.X, cell.Min.Y, cell.Max.X, cell.Min.Y+ch/2), &image.Uniform{fg}, image.Point{}, draw.Src)
			case ' ':
			default:
				draw.Draw(img, image.Rect(cell.Min.X+cw/4, cell.Min.Y+ch/4, cell.Max.X-cw/4, cell.Max.Y-ch/4), &image.Uniform{fg}, image.Point{}, draw.Src)
			}
			x += w
		}
	}
	return img
}

func applySGR(params string, fg, bg, defFg, defBg color.RGBA) (color.RGBA, color.RGBA) {
	if params == "" {
		return defFg, defBg
	}
	p := strings.Split(params, ";")
	num := func(i int) uint8 {
		if i >= len(p) {
			return 0
		}
		n, _ := strconv.Atoi(p[i])
		return uint8(n)
	}
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case "0":
			fg, bg = defFg, defBg
		case "39":
			fg = defFg
		case "49":
			bg = defBg
		case "38", "48":
			if i+4 < len(p) && p[i+1] == "2" {
				c := color.RGBA{R: num(i + 2), G: num(i + 3), B: num(i + 4), A: 255}
				if p[i] == "38" {
					fg = c
				} else {
					bg = c
				}
				i += 4
			}
		case "7":
			fg, bg = bg, fg
		}
	}
	return fg, bg
}

// testCover is a cover with four clearly different quadrants.
func testCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	cols := []color.RGBA{{R: 220, G: 60, B: 50, A: 255}, {R: 240, G: 190, B: 40, A: 255}, {R: 40, G: 120, B: 220, A: 255}, {R: 60, G: 180, B: 120, A: 255}}
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			c := cols[(y/100)*2+x/100]
			if (x-100)*(x-100)+(y-100)*(y-100) < 30*30 {
				c = color.RGBA{R: 250, G: 250, B: 250, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func TestRenderColors(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	save := func(name string, img image.Image) {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	black := color.RGBA{R: 12, G: 12, B: 14, A: 255}

	tu, _ := newTestTUI(t, 120, 34)
	cover := testCover()
	for _, s := range tu.eng.Store.Songs {
		tu.art[core.CoverKey(s)] = &artEntry{img: cover, colors: core.PaletteFromImage(cover)}
	}
	tu.press("down", "enter")
	save("color-songs", ansiToImage(ansi.Strip("")+tu.View().Content, 9, 18, black))
	tu.press("3", "enter")
	save("color-album", ansiToImage(tu.View().Content, 9, 18, black))
	tu.press("esc", "M")
	save("color-mini", ansiToImage(tu.View().Content, 9, 18, black))

	tuL, _ := newTestTUI(t, 110, 30)
	tuL.th = newTheme(false)
	for _, s := range tuL.eng.Store.Songs {
		tuL.art[core.CoverKey(s)] = &artEntry{img: cover, colors: core.PaletteFromImage(cover)}
	}
	tuL.press("down", "enter", "f")
	save("color-light", ansiToImage(tuL.View().Content, 9, 18, color.RGBA{R: 255, G: 255, B: 255, A: 255}))

	tu2, _ := newTestTUI(t, 100, 16)
	for _, s := range tu2.eng.Store.Songs {
		tu2.art[core.CoverKey(s)] = &artEntry{img: cover, colors: core.PaletteFromImage(cover)}
	}
	tu2.press("enter", "M")
	save("color-mini-wide", ansiToImage(tu2.View().Content, 9, 18, black))
}

func writePNGFile(t *testing.T, path string, img image.Image) {
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
