package main

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
)

type fakeViz struct {
	frame   []uint8
	follows []string
	stops   int
}

func (f *fakeViz) Follow(path string, _ time.Duration) { f.follows = append(f.follows, path) }
func (f *fakeViz) At(time.Duration) ([]uint8, bool) {
	if f.frame == nil {
		return nil, false
	}
	return append([]uint8(nil), f.frame...), true
}
func (f *fakeViz) Stop() { f.stops++ }

func frameOf(v uint8) []uint8 {
	out := make([]uint8, core.VizBands)
	for i := range out {
		out[i] = v
	}
	return out
}

func levelsOf(v float64) []float64 {
	out := make([]float64, core.VizBands)
	for i := range out {
		out[i] = v
	}
	return out
}

var (
	ca = color.RGBA{R: 200, G: 80, B: 80, A: 255}
	cb = color.RGBA{R: 80, G: 80, B: 200, A: 255}
	bd = color.RGBA{R: 20, G: 20, B: 30, A: 255}
)

func plain(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestRenderersFillExactlyTheirBox(t *testing.T) {
	for _, width := range []int{20, 31, 40, 72, 100} {
		for _, rows := range []int{2, 3, 5, 8} {
			for name, render := range map[string]func([]float64, int, int, color.RGBA, color.RGBA, color.Color) []string{
				"bars": renderBars, "mirror": renderMirror, "wave": renderWave,
			} {
				for _, lv := range []float64{0, 0.37, 1} {
					lines := render(levelsOf(lv), width, rows, ca, cb, bd)
					if len(lines) != rows {
						t.Fatalf("%s %dx%d: %d rows", name, width, rows, len(lines))
					}
					for i, l := range lines {
						if w := ansi.StringWidth(l); w != width {
							t.Fatalf("%s %dx%d level %.2f row %d is %d cells wide", name, width, rows, lv, i, w)
						}
					}
				}
			}
		}
	}
}

func TestBarsGrowFromTheBottom(t *testing.T) {
	silent := plain(renderBars(levelsOf(0), 40, 4, ca, cb, bd))
	for _, l := range silent {
		if strings.TrimSpace(l) != "" {
			t.Fatalf("silence draws nothing: %q", l)
		}
	}
	full := plain(renderBars(levelsOf(1), 40, 4, ca, cb, bd))
	for i, l := range full {
		if !strings.Contains(l, "█") {
			t.Fatalf("a full level fills every row, row %d: %q", i, l)
		}
	}
	// Half the height: the two bottom rows are full, the top two are empty.
	half := plain(renderBars(levelsOf(0.5), 40, 4, ca, cb, bd))
	if strings.TrimSpace(half[0]) != "" || strings.TrimSpace(half[1]) != "" ||
		!strings.Contains(half[2], "█") || !strings.Contains(half[3], "█") {
		t.Fatalf("half height: %q", half)
	}
	// A level between cells uses a partial block.
	part := plain(renderBars(levelsOf(0.2), 40, 4, ca, cb, bd))
	if !strings.ContainsAny(part[3], "▁▂▃▄▅▆▇█") || strings.TrimSpace(part[2]) != "" && !strings.ContainsAny(part[2], "▁▂▃▄▅▆▇") {
		t.Fatalf("partial blocks: %q", part)
	}
	// Louder is taller: more lit cells.
	count := func(lv float64) int {
		n := 0
		for _, l := range plain(renderBars(levelsOf(lv), 40, 8, ca, cb, bd)) {
			n += strings.Count(l, "█")
		}
		return n
	}
	if !(count(0.25) < count(0.5) && count(0.5) < count(0.9)) {
		t.Fatal("the bars rise with the level")
	}
}

func TestEachBarFollowsItsBand(t *testing.T) {
	lv := levelsOf(0)
	for i := 0; i < 8; i++ {
		lv[i] = 1 // only the lowest bands
	}
	lines := plain(renderBars(lv, 40, 4, ca, cb, bd))
	top := []rune(lines[0])
	left, right := strings.TrimSpace(string(top[:10])), strings.TrimSpace(string(top[len(top)-10:]))
	if left == "" || right != "" {
		t.Fatalf("the bass is on the left only: left %q right %q", left, right)
	}
}

func TestMirrorIsSymmetric(t *testing.T) {
	lines := plain(renderMirror(levelsOf(0.6), 40, 6, ca, cb, bd))
	swap := strings.NewReplacer("▀", "▄", "▄", "▀")
	for i := 0; i < len(lines)/2; i++ {
		if swap.Replace(lines[i]) != lines[len(lines)-1-i] {
			t.Fatalf("row %d does not mirror row %d:\n%q\n%q", i, len(lines)-1-i, lines[i], lines[len(lines)-1-i])
		}
	}
	silent := plain(renderMirror(levelsOf(0), 40, 6, ca, cb, bd))
	for _, l := range silent {
		if strings.TrimSpace(l) != "" {
			t.Fatalf("silence draws nothing: %q", l)
		}
	}
	// A louder frame lights more half rows, outward from the middle.
	n := func(lv float64) int {
		c := 0
		for _, l := range plain(renderMirror(levelsOf(lv), 40, 8, ca, cb, bd)) {
			c += strings.Count(l, "█") + strings.Count(l, "▀") + strings.Count(l, "▄")
		}
		return c
	}
	if !(n(0.2) < n(0.6) && n(0.6) < n(1)) {
		t.Fatal("louder reaches further from the middle")
	}
}

func TestWaveDrawsBrailleAndSwingsWithTheLevel(t *testing.T) {
	flat := plain(renderWave(levelsOf(0), 40, 4, ca, cb, bd))
	big := plain(renderWave(levelsOf(1), 40, 4, ca, cb, bd))
	braille := func(lines []string) (cells, rowsUsed int) {
		for _, l := range lines {
			n := 0
			for _, r := range l {
				if r > 0x2800 && r <= 0x28FF {
					n++
				}
			}
			cells += n
			if n > 0 {
				rowsUsed++
			}
		}
		return
	}
	_, flatRows := braille(flat)
	cells, bigRows := braille(big)
	if cells == 0 {
		t.Fatal("the wave is drawn with braille dots")
	}
	if bigRows <= flatRows {
		t.Fatalf("a loud wave spreads over more rows than a flat one: %d vs %d", bigRows, flatRows)
	}
}

func TestVisualizerSmoothing(t *testing.T) {
	tu, fp := newTestTUI(t, 100, 30)
	src := &fakeViz{frame: frameOf(255)}
	tu.viz.src, tu.viz.style = src, "bars"
	tu.setMini(true)
	tu.eng.PlaySongByID("id0")

	if !tu.updateViz() {
		t.Fatal("a loud frame moves the bars")
	}
	first := tu.viz.levels[10]
	if first < 0.4 || first >= 1 {
		t.Fatalf("they rise fast but not at once: %.2f", first)
	}
	for i := 0; i < 8; i++ {
		tu.updateViz()
	}
	if tu.viz.levels[10] < 0.95 {
		t.Fatalf("and reach the level: %.2f", tu.viz.levels[10])
	}
	if len(src.follows) == 0 || src.follows[0] != "/music/0.flac" {
		t.Fatalf("the analysis is pointed at the song that plays: %v", src.follows)
	}

	// The music stops: they fall slowly and then rest.
	src.frame = frameOf(0)
	tu.updateViz()
	if v := tu.viz.levels[10]; v >= 0.95 || v < 0.8 {
		t.Fatalf("a fall takes a few frames, not one: %.2f", v)
	}
	steps := 0
	for tu.updateViz() {
		steps++
		if steps > 100 {
			t.Fatal("they never settle")
		}
	}
	if tu.viz.levels[10] != 0 || tu.vizHasLevels() {
		t.Fatalf("at rest they are zero: %.3f", tu.viz.levels[10])
	}

	// Paused: nothing is analyzed and the bars fall to zero.
	src.frame = frameOf(255)
	tu.updateViz()
	fp.paused = true
	follows := len(src.follows)
	for i := 0; i < 40; i++ {
		tu.updateViz()
	}
	if tu.vizHasLevels() || len(src.follows) != follows {
		t.Fatal("while paused the bars fall and the analysis is left alone")
	}
}

func TestVisualizerTickOnlyWhenItIsWatched(t *testing.T) {
	tu, fp := newTestTUI(t, 100, 30)
	tu.viz.src, tu.viz.style = &fakeViz{frame: frameOf(200)}, "bars"
	tu.eng.PlaySongByID("id0")
	if got := tu.tickInterval(); got != tickPlaying {
		t.Fatalf("outside the mini player the tick is the normal one: %v", got)
	}
	tu.setMini(true)
	if got := tu.tickInterval(); got != vizTick {
		t.Fatalf("with the mini player and a song, the visualizer's frame rate: %v", got)
	}
	tu.viz.style = "off"
	if got := tu.tickInterval(); got != tickPlaying {
		t.Fatalf("switched off, the normal tick: %v", got)
	}
	tu.viz.style = "wave"
	fp.paused = true
	tu.viz.levels[3] = 0.5
	if got := tu.tickInterval(); got != vizTick {
		t.Fatalf("while the bars fall it keeps the fast tick: %v", got)
	}
	tu.viz.levels[3] = 0
	if got := tu.tickInterval(); got != tickIdle {
		t.Fatalf("paused and at rest, the idle tick: %v", got)
	}
}

func TestLeavingTheMiniPlayerStopsTheAnalysis(t *testing.T) {
	tu, _ := newTestTUI(t, 100, 30)
	src := &fakeViz{frame: frameOf(255)}
	tu.viz.src, tu.viz.style = src, "bars"
	tu.eng.PlaySongByID("id0")
	tu.press("M")
	tu.updateViz()
	if !tu.vizHasLevels() {
		t.Fatal("the bars are up")
	}
	tu.press("M")
	if src.stops != 1 || tu.vizHasLevels() {
		t.Fatalf("closing the mini player stops the analysis and clears the bars: stops=%d", src.stops)
	}
}

func TestVisualizerStyleKeyAndPreferences(t *testing.T) {
	tu, _ := newTestTUI(t, 100, 30)
	tu.viz.src = &fakeViz{}
	tu.press("M")
	if tu.viz.style != "bars" {
		t.Fatalf("bars by default: %s", tu.viz.style)
	}
	for _, want := range []string{"mirror", "wave", "off", "bars"} {
		tu.press("v")
		if tu.viz.style != want {
			t.Fatalf("v goes to %s, got %s", want, tu.viz.style)
		}
	}
	tu.press("v", "v") // mirror, wave
	again := newTUI(tu.eng, false)
	if again.viz.style != "wave" {
		t.Fatalf("the style is remembered: %s", again.viz.style)
	}
	// Choosing the theme must not undo it.
	tu.setTheme("dracula")
	if again = newTUI(tu.eng, false); again.viz.style != "wave" || again.themeID != "dracula" {
		t.Fatalf("theme and visualizer are kept together: %s %s", again.viz.style, again.themeID)
	}
	if validVizStyle("nonsense") != "bars" {
		t.Fatal("an unknown style falls back to bars")
	}
}

func TestVisualizerFromThePaletteAndSettings(t *testing.T) {
	tu, _ := newTestTUI(t, 110, 32)
	tu.viz.src = &fakeViz{}
	tu.press("ctrl+p")
	tu.typeText("visualiz")
	if len(tu.pal.hits) == 0 || tu.pal.hits[0].ActionID != "visualizer" {
		t.Fatalf("the palette finds it: %+v", tu.pal.hits)
	}
	tu.press("enter")
	if tu.viz.style != "mirror" {
		t.Fatalf("the action cycles the style: %s", tu.viz.style)
	}
	tu.press(",", "]")
	if !strings.Contains(tu.screen(), "Visualizador") {
		t.Fatal("Apariencia has a visualizer field")
	}
}

func TestMiniShowsTheVisualizer(t *testing.T) {
	tu, _ := newTestTUI(t, 100, 30)
	tu.viz.src, tu.viz.style = &fakeViz{frame: frameOf(255)}, "bars"
	tu.eng.PlaySongByID("id0")
	tu.press("M")
	for i := 0; i < 12; i++ {
		tu.updateViz()
	}
	tu.dirty = true
	with := tu.screen()
	if !strings.Contains(with, "█") {
		t.Fatal("the bars are on screen")
	}
	tu.viz.style = "off"
	tu.dirty = true
	if strings.Contains(tu.screen(), "█") {
		t.Fatal("switched off, the visualizer takes no room")
	}
	tu.viz.style = "bars"
	tu.viz.src = nil
	tu.dirty = true
	if strings.Contains(tu.screen(), "█") {
		t.Fatal("without ffmpeg nothing is drawn")
	}
}

// With the visualizer on, the mini player still fits every terminal and its
// controls still sit under their click zones.
func TestMiniWithVisualizerFitsAndKeepsItsControls(t *testing.T) {
	sizes := [][2]int{{120, 40}, {120, 34}, {100, 30}, {100, 26}, {100, 22}, {100, 16}, {80, 24}, {80, 18}, {64, 14}, {64, 12}, {60, 20}, {44, 14}, {40, 10}}
	for _, style := range []string{"bars", "mirror", "wave"} {
		for _, sz := range sizes {
			tu, _ := newTestTUI(t, sz[0], sz[1])
			tu.viz.src, tu.viz.style = &fakeViz{frame: frameOf(180)}, style
			tu.eng.PlaySongByID("id0")
			tu.press("M")
			for i := 0; i < 6; i++ {
				tu.updateViz()
			}
			tu.dirty = true
			lines := strings.Split(tu.screen(), "\n")
			if len(lines) != sz[1] {
				t.Fatalf("%s %dx%d: %d lines", style, sz[0], sz[1], len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > sz[0] {
					t.Fatalf("%s %dx%d line %d is %d wide", style, sz[0], sz[1], i, w)
				}
			}
			for _, kind := range transportOrder {
				z, ok := zoneOf(tu, kind)
				if !ok {
					t.Fatalf("%s %dx%d: no zone for %s", style, sz[0], sz[1], kind)
				}
				row := []rune(lines[z.y])
				if z.x+z.w > len(row) {
					continue
				}
				cellText := string(row[z.x : z.x+z.w])
				found := false
				for _, sym := range transportSymbols[kind] {
					found = found || strings.Contains(cellText, sym)
				}
				if !found {
					t.Fatalf("%s %dx%d: the %s zone covers %q, not its symbol", style, sz[0], sz[1], kind, cellText)
				}
			}
		}
	}
}
