package main

import (
	"image"
	"image/color"
	"testing"
)

func TestPaletteFromImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			switch {
			case x < 32 && y < 32:
				img.Set(x, y, color.RGBA{R: 230, G: 40, B: 40, A: 255})
			case x >= 32 && y < 32:
				img.Set(x, y, color.RGBA{R: 40, G: 60, B: 230, A: 255})
			default:
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	cols := paletteFromImage(img)
	if len(cols) != 4 {
		t.Fatalf("want 4 colors, got %d", len(cols))
	}
	// The two vivid colors outrank white, which is toned down and not first.
	if cols[0].R < 150 && cols[0].B < 150 {
		t.Fatalf("first color should be vivid red/blue: %+v", cols[0])
	}
	for _, c := range cols {
		lum := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
		if lum > 0.63 {
			t.Fatalf("color too bright for white text: %+v (%.2f)", c, lum)
		}
	}
}

func TestPaletteFromBlackImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	if cols := paletteFromImage(img); cols != nil {
		t.Fatalf("black image should fall back, got %v", cols)
	}
}
