package core

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
	xdraw "golang.org/x/image/draw"

	"musaik/internal/lastfm"
	"musaik/internal/model"
)

// Artwork is a song's embedded cover and the colors that stand out in it.
type Artwork struct {
	Image  image.Image
	Colors []color.RGBA
}

// CoverKey groups songs that share a cover: the album, or the file.
func CoverKey(s model.Song) string {
	if s.Artist != "" && s.Album != "" {
		return strings.ToLower(s.Artist + "||" + s.Album)
	}
	return s.Path
}

// LoadArtwork reads the cover embedded in the file, scaled to fit maxDim, and
// works out its palette. It returns a zero Artwork when there is none.
func LoadArtwork(path string, maxDim int) Artwork {
	f, err := os.Open(path)
	if err != nil {
		return Artwork{}
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		return Artwork{}
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return Artwork{}
	}
	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return Artwork{}
	}
	scaled := ScaleImage(img, maxDim)
	return Artwork{Image: scaled, Colors: PaletteFromImage(scaled)}
}

// ScaleImage shrinks src, smoothly, so that neither side exceeds maxDim.
func ScaleImage(src image.Image, maxDim int) image.Image {
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

// PaletteFromImage returns up to four distinct, vivid colors of img, toned
// down so white text stays readable on top of them. It is nil for an image
// that is all black.
func PaletteFromImage(img image.Image) []color.RGBA {
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

	var picked []color.RGBA
	for _, bk := range list {
		c := color.RGBA{R: uint8(bk.r / bk.n), G: uint8(bk.g / bk.n), B: uint8(bk.b / bk.n), A: 255}
		far := true
		for _, p := range picked {
			if ColorDistance(c, p) < 70 {
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
			picked = append(picked, MixColor(base, color.RGBA{A: 255}, 0.35))
		} else {
			picked = append(picked, MixColor(base, color.RGBA{R: 255, G: 255, B: 255, A: 255}, 0.25))
		}
	}
	for i := range picked {
		picked[i] = LimitLuminance(picked[i], 0.62)
	}
	return picked
}

// ColorDistance is the Euclidean distance between two colors in RGB.
func ColorDistance(a, b color.RGBA) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// MixColor blends a toward b by t (0 is a, 1 is b).
func MixColor(a, b color.RGBA, t float64) color.RGBA {
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

// LimitLuminance darkens c until its luminance is at most maxLum (0-1).
func LimitLuminance(c color.RGBA, maxLum float64) color.RGBA {
	lum := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	if lum <= maxLum {
		return c
	}
	return MixColor(c, color.RGBA{A: 255}, 1-maxLum/lum)
}

// ---- artist avatars ---------------------------------------------------------------

// ArtistAvatarKey normalizes an artist name for caching.
func ArtistAvatarKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

// ArtistAvatarCachePath is where the desktop and terminal apps keep an artist's
// photo, so one fetch serves both.
func ArtistAvatarCachePath(artist string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "musaik", "artist-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(ArtistAvatarKey(artist)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".img"), nil
}

// LoadArtistAvatarFromDisk reads a cached artist photo, scaled to maxDim.
func LoadArtistAvatarFromDisk(artist string, maxDim int) image.Image {
	path, err := ArtistAvatarCachePath(artist)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return ScaleImage(img, maxDim)
}

// FetchArtistAvatar downloads an artist's photo through Last.fm and caches it.
// It blocks, so run it off the UI goroutine.
func FetchArtistAvatar(client *lastfm.Client, artist string, maxDim int) (image.Image, error) {
	if client == nil || strings.TrimSpace(client.APIKey) == "" {
		return nil, fmt.Errorf("sin API key de Last.fm")
	}
	imgURL, err := client.ArtistImageURL(artist)
	if err != nil || strings.TrimSpace(imgURL) == "" {
		return nil, fmt.Errorf("sin foto para %s", artist)
	}
	resp, err := client.HTTP.Get(imgURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("respuesta vacía")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if path, err := ArtistAvatarCachePath(artist); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
	return ScaleImage(img, maxDim), nil
}
