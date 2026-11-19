package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	xdraw "golang.org/x/image/draw"
)

// imgProto is how covers reach the screen.
type imgProto int

const (
	// protoBlocks paints the image with half blocks: two pixels per cell, so
	// it is blocky, but it works in any terminal with 24-bit color.
	protoBlocks imgProto = iota
	// protoKitty sends the image to the terminal once, in full resolution,
	// and shows it with Unicode placeholders: ordinary text cells that the
	// terminal replaces with the image. Being text, they scroll, get covered by
	// panels and survive Bubble Tea's cell renderer. kitty, Ghostty and Rex draw them.
	protoKitty
)

const (
	artPxBlocks = 160
	artPxKitty  = 640
	// kittyCellPx is the width, in pixels, that a cell of the image is given
	// when it is sent: enough for a sharp picture without sending more than the
	// terminal can show.
	kittyCellPx = 22
	kittyFirst  = 1000 // image ids start here, away from small ids other programs use
)

// detectImages picks the image protocol: mode is "auto", "kitty" or "blocks".
// Auto turns kitty graphics on for kitty, Ghostty and Rex (which is built on
// libghostty); inside tmux or screen the escape sequences would need
// passthrough, so it keeps to blocks there.
func detectImages(mode string) imgProto {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "blocks", "off", "none", "0":
		return protoBlocks
	case "kitty", "on", "1":
		return protoKitty
	}
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" {
		return protoBlocks
	}
	term, prog := os.Getenv("TERM"), os.Getenv("TERM_PROGRAM")
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "",
		strings.Contains(term, "kitty"),
		strings.Contains(term, "ghostty"),
		strings.EqualFold(prog, "ghostty"),
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "",
		strings.EqualFold(prog, "rex"),
		strings.HasPrefix(term, "xterm-rex"):
		return protoKitty
	}
	return protoBlocks
}

func (t *tui) setImageProto(p imgProto) {
	t.proto = p
	t.artPx = artPxBlocks
	if p == protoKitty {
		t.artPx = artPxKitty
	}
}

type kittyImg struct {
	id   uint32
	sent bool
}

type kittyJob struct {
	key        string
	id         uint32
	img        image.Image
	cols, rows int
}

// kittyState tracks the images sent to the terminal.
type kittyState struct {
	next uint32
	imgs map[string]*kittyImg
	jobs []kittyJob
	drop []uint32
}

// kittyReadyMsg carries an encoded image to write; kittySentMsg says it was.
type (
	kittyReadyMsg struct {
		key string
		seq string
	}
	kittySentMsg struct{ key string }
)

// kittyLines is the text that stands for an image already sent: a grid of
// placeholder cells whose color is the image id and whose accents say which
// row and column of the image each one shows.
func kittyLines(id uint32, cols, rows int) []string {
	r, g, b := byte(id>>16), byte(id>>8), byte(id)
	lines := make([]string, rows)
	for y := 0; y < rows; y++ {
		var sb strings.Builder
		sb.WriteString("\x1b[38;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b)) + "m")
		for x := 0; x < cols; x++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(kitty.Diacritic(y))
			sb.WriteRune(kitty.Diacritic(x))
		}
		sb.WriteString("\x1b[39m")
		lines[y] = sb.String()
	}
	return lines
}

// cropRect is the largest centered part of b with the aspect of a cols x rows
// block of cells, taking a cell to be twice as tall as it is wide.
func cropRect(b image.Rectangle, cols, rows int) image.Rectangle {
	srcW, srcH := b.Dx(), b.Dy()
	want := float64(cols) / float64(rows*2)
	if float64(srcW)/float64(srcH) > want {
		nw := int(float64(srcH) * want)
		x := b.Min.X + (srcW-nw)/2
		return image.Rect(x, b.Min.Y, x+nw, b.Max.Y)
	}
	nh := int(float64(srcW) / want)
	y := b.Min.Y + (srcH-nh)/2
	return image.Rect(b.Min.X, y, b.Max.X, y+nh)
}

// encodeKitty makes the escape sequences that send img to the terminal as image
// j.id and place it, virtually, over j.cols x j.rows cells.
func encodeKitty(j kittyJob) (string, error) {
	r := cropRect(j.img.Bounds(), j.cols, j.rows)
	px := min(j.cols*kittyCellPx, artPxKitty)
	h := max(int(float64(px)*float64(r.Dy())/float64(r.Dx())), 1)
	dst := image.NewRGBA(image.Rect(0, 0, px, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), j.img, r, xdraw.Src, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	id := strconv.FormatUint(uint64(j.id), 10)
	var sb strings.Builder
	for i := 0; i < len(data); i += kitty.MaxChunkSize {
		end := min(i+kitty.MaxChunkSize, len(data))
		more := "1"
		if end == len(data) {
			more = "0"
		}
		opts := []string{"m=" + more, "q=2"}
		if i == 0 {
			opts = []string{"a=t", "f=100", "i=" + id, "q=2", "m=" + more}
		}
		sb.WriteString(ansi.KittyGraphics([]byte(data[i:end]), opts...))
	}
	sb.WriteString(ansi.KittyGraphics(nil, "a=p", "U=1", "i="+id, "c="+strconv.Itoa(j.cols), "r="+strconv.Itoa(j.rows), "q=2"))
	return sb.String(), nil
}

// kittyDelete frees images in the terminal.
func kittyDelete(ids []uint32) string {
	var sb strings.Builder
	for _, id := range ids {
		sb.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", "i="+strconv.FormatUint(uint64(id), 10), "q=2"))
	}
	return sb.String()
}

// kittyDeleteAll is what to write on exit, so no image outlives the program.
func kittyDeleteAll() string { return ansi.KittyGraphics(nil, "a=d", "d=A", "q=2") }

// kittyImage returns the placeholders for an image when it is in the terminal.
// Until it is, it queues the image to be sent and returns nil, and the caller
// draws it with half blocks meanwhile.
func (t *tui) kittyImage(id string, cols, rows int, img image.Image) []string {
	key := id + "|" + strconv.Itoa(cols) + "x" + strconv.Itoa(rows)
	k := &t.kitty
	if im := k.imgs[key]; im != nil {
		if im.sent {
			return kittyLines(im.id, cols, rows)
		}
		return nil
	}
	k.next++
	k.imgs[key] = &kittyImg{id: k.next}
	k.jobs = append(k.jobs, kittyJob{key: key, id: k.next, img: img, cols: cols, rows: rows})
	return nil
}

// forgetKitty frees the terminal's copies of the images of one cover.
func (t *tui) forgetKitty(artID string) {
	for key, im := range t.kitty.imgs {
		if strings.HasPrefix(key, artID+"|") {
			if im.sent {
				t.kitty.drop = append(t.kitty.drop, im.id)
			}
			delete(t.kitty.imgs, key)
		}
	}
}

// flushKitty starts encoding what View asked for and frees what was dropped.
func (t *tui) flushKitty() tea.Cmd {
	if t.proto != protoKitty {
		return nil
	}
	var cmds []tea.Cmd
	for _, j := range t.kitty.jobs {
		cmds = append(cmds, func() tea.Msg {
			seq, err := encodeKitty(j)
			if err != nil {
				return nil
			}
			releaseMemory()
			return kittyReadyMsg{key: j.key, seq: seq}
		})
	}
	t.kitty.jobs = nil
	if len(t.kitty.drop) > 0 {
		cmds = append(cmds, tea.Raw(kittyDelete(t.kitty.drop)))
		t.kitty.drop = nil
	}
	return tea.Batch(cmds...)
}

// onKittyReady writes an encoded image; once it has gone out, the image can be
// shown, so the next frame swaps the half blocks for the placeholders.
func (t *tui) onKittyReady(msg kittyReadyMsg) tea.Cmd {
	return tea.Sequence(tea.Raw(msg.seq), func() tea.Msg { return kittySentMsg{key: msg.key} })
}

func (t *tui) onKittySent(msg kittySentMsg) {
	if im := t.kitty.imgs[msg.key]; im != nil {
		im.sent = true
	}
}
