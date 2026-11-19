package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"musaik/internal/core"
)

func TestKittyPlaceholders(t *testing.T) {
	const id = 0x010203
	lines := kittyLines(id, 5, 3)
	if len(lines) != 3 {
		t.Fatalf("rows: %d", len(lines))
	}
	for y, l := range lines {
		if w := ansi.StringWidth(l); w != 5 {
			t.Fatalf("row %d is %d cells wide, want 5", y, w)
		}
		if !strings.Contains(l, "\x1b[38;2;1;2;3m") {
			t.Fatalf("the image id is the foreground color: %q", l)
		}
		for x := 0; x < 5; x++ {
			cell := string(kitty.Placeholder) + string(kitty.Diacritic(y)) + string(kitty.Diacritic(x))
			if !strings.Contains(l, cell) {
				t.Fatalf("row %d is missing the cell for column %d", y, x)
			}
		}
	}
}

func TestEncodeKitty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	seq, err := encodeKitty(kittyJob{id: 1001, img: img, cols: 16, rows: 8})
	if err != nil {
		t.Fatal(err)
	}
	apcs := regexp.MustCompile(`\x1b_G([^;\x1b]*)(?:;([^\x1b]*))?\x1b\\`).FindAllStringSubmatch(seq, -1)
	if len(apcs) < 3 {
		t.Fatalf("expected several chunks and a placement: %d", len(apcs))
	}
	first, last := apcs[0][1], apcs[len(apcs)-1][1]
	if !strings.HasPrefix(first, "a=t,f=100,i=1001") || !strings.Contains(first, "m=1") {
		t.Fatalf("first chunk: %q", first)
	}
	for _, a := range apcs[1 : len(apcs)-2] {
		if !strings.Contains(a[1], "m=1") || strings.Contains(a[1], "a=") {
			t.Fatalf("middle chunks carry only m and q: %q", a[1])
		}
	}
	if end := apcs[len(apcs)-2][1]; !strings.Contains(end, "m=0") {
		t.Fatalf("the last data chunk says it is the last: %q", end)
	}
	if !strings.Contains(last, "a=p") || !strings.Contains(last, "U=1") || !strings.Contains(last, "i=1001") || !strings.Contains(last, "c=16") || !strings.Contains(last, "r=8") {
		t.Fatalf("placement: %q", last)
	}
	var data strings.Builder
	for _, a := range apcs[:len(apcs)-1] {
		if len(a[2]) > kitty.MaxChunkSize {
			t.Fatalf("chunk of %d bytes", len(a[2]))
		}
		data.WriteString(a[2])
	}
	raw, err := base64.StdEncoding.DecodeString(data.String())
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the payload is a PNG: %v", err)
	}
	// 16 columns by 8 rows is square, so the picture is cropped to a square.
	if b := got.Bounds(); b.Dx() != b.Dy() {
		t.Fatalf("not square: %v", b)
	}
}

func TestKittyFlow(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.setImageProto(protoKitty)
	cover := testCover()
	for _, s := range tu.eng.Store.Songs {
		tu.art[core.CoverKey(s)] = &artEntry{img: cover, colors: core.PaletteFromImage(cover)}
	}
	tu.press("enter")

	first := tu.View().Content
	if strings.ContainsRune(first, kitty.Placeholder) {
		t.Fatal("before the image reaches the terminal there are no placeholders")
	}
	if !strings.Contains(first, "▀") {
		t.Fatal("meanwhile the cover is drawn with half blocks")
	}
	if len(tu.kitty.jobs) == 0 {
		t.Fatal("the cover is queued to be sent")
	}
	if cmd := tu.flushKitty(); cmd == nil {
		t.Fatal("flushing starts the encoding")
	}
	for _, j := range tu.kitty.jobs {
		t.Fatalf("job left after flush: %+v", j)
	}
	for key := range tu.kitty.imgs {
		tu.onKittySent(kittySentMsg{key: key})
	}
	second := tu.View().Content
	if !strings.ContainsRune(second, kitty.Placeholder) {
		t.Fatal("once sent, the cover is shown with placeholders")
	}
	// The placeholders keep every line inside the terminal.
	for i, l := range strings.Split(second, "\n") {
		if w := ansi.StringWidth(l); w > 110 {
			t.Fatalf("line %d is %d cells wide", i, w)
		}
	}

	// Dropping a cover frees it in the terminal.
	before := len(tu.kitty.imgs)
	tu.dropImages(core.CoverKey(tu.eng.Store.Songs[0]))
	if len(tu.kitty.imgs) >= before || len(tu.kitty.drop) == 0 {
		t.Fatalf("dropped images are forgotten and queued for deletion: %d -> %d, drop=%v", before, len(tu.kitty.imgs), tu.kitty.drop)
	}
	if got := kittyDelete(tu.kitty.drop); !strings.Contains(got, "a=d,d=I,i=") {
		t.Fatalf("delete: %q", got)
	}
}

func TestBlocksModeNeverUsesKitty(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	cover := testCover()
	for _, s := range tu.eng.Store.Songs {
		tu.art[core.CoverKey(s)] = &artEntry{img: cover}
	}
	tu.press("enter")
	if strings.ContainsRune(tu.View().Content, kitty.Placeholder) || len(tu.kitty.jobs) != 0 {
		t.Fatal("blocks mode sends nothing to the terminal")
	}
}

func TestDetectImages(t *testing.T) {
	clear := func() {
		for _, k := range []string{"TMUX", "STY", "KITTY_WINDOW_ID", "TERM", "TERM_PROGRAM", "GHOSTTY_RESOURCES_DIR"} {
			t.Setenv(k, "")
		}
	}
	cases := []struct {
		name string
		env  map[string]string
		mode string
		want imgProto
	}{
		{"plain terminal", nil, "auto", protoBlocks},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "3"}, "auto", protoKitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, "", protoKitty},
		{"ghostty by TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "ghostty"}, "auto", protoKitty},
		{"ghostty by TERM", map[string]string{"TERM": "xterm-ghostty"}, "auto", protoKitty},
		{"rex by TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "rex"}, "auto", protoKitty},
		{"rex by TERM", map[string]string{"TERM": "xterm-rex"}, "auto", protoKitty},
		{"inside tmux stays on blocks", map[string]string{"KITTY_WINDOW_ID": "3", "TMUX": "/tmp/tmux-1/default,1,0"}, "auto", protoBlocks},
		{"blocks wins", map[string]string{"KITTY_WINDOW_ID": "3"}, "blocks", protoBlocks},
		{"kitty can be forced", nil, "kitty", protoKitty},
	}
	for _, c := range cases {
		clear()
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		if got := detectImages(c.mode); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
