package main

import (
	"image/color"
	"math"
	"strings"
	"time"

	"musaik/internal/core"
)

// vizSource is what the visualizer needs from the audio analysis.
type vizSource interface {
	Follow(path string, pos time.Duration)
	At(pos time.Duration) ([]uint8, bool)
	Stop()
}

const (
	// vizTick is the frame time of the visualizer, 15 a second: smooth enough for
	// bars, and cheap, because it only runs while the mini player is open and a
	// song plays. Most of what a frame costs is the terminal output, not the
	// drawing, so the frame rate is what matters.
	vizTick = 66 * time.Millisecond
	// vizLatency is how far ahead of what is heard the playback position runs:
	// ffplay takes a moment to start the sound. The spectrum is looked up that
	// much earlier so the bars move with the music, not before it.
	vizLatency = 120 * time.Millisecond
	// Bars rise fast and fall slowly, as they do on a stereo.
	vizAttack = 0.6
	vizFall   = 0.08
	// A bar that moves less than this is not redrawn: the tallest bar has about
	// 60 steps, so a smaller change cannot be seen.
	vizVisible = 1.0 / 80
)

var (
	vizStyles = []string{"bars", "mirror", "wave", "off"}
	vizLabels = map[string]string{"bars": "Barras", "mirror": "Espejo", "wave": "Onda", "off": "Apagado"}
)

type vizState struct {
	src    vizSource
	style  string
	levels [core.VizBands]float64
	note   string // why there is no visualizer
}

// enableVisualizer looks for ffmpeg, which the analysis needs.
func (t *tui) enableVisualizer() {
	a, err := core.NewAnalyzer()
	if err != nil {
		t.viz.note = "Requiere ffmpeg, que no se encontró."
		return
	}
	t.viz.src = a
}

func validVizStyle(s string) string {
	for _, v := range vizStyles {
		if v == s {
			return s
		}
	}
	return "bars"
}

// vizEnabled: the mini player is open, the style is not "off" and there is an analyzer.
func (t *tui) vizEnabled() bool {
	return t.mini && t.viz.style != "off" && t.viz.src != nil
}

func (t *tui) vizHasLevels() bool {
	for _, v := range t.viz.levels {
		if v > 0 {
			return true
		}
	}
	return false
}

// vizBusy is when the visualizer needs the fast tick: while it plays, and while
// the bars are still falling after it stopped.
func (t *tui) vizBusy() bool {
	return t.vizEnabled() && (t.isPlaying() || t.vizHasLevels())
}

// updateViz moves the bars one frame toward the spectrum at the playback
// position and reports whether any of them moved.
func (t *tui) updateViz() bool {
	v := &t.viz
	var target []uint8
	if st := t.eng.Player.State(); st.Playing && !st.Paused {
		if song, ok := t.eng.CurrentSong(); ok {
			v.src.Follow(song.Path, st.Position)
			target, _ = v.src.At(max(st.Position-vizLatency, 0))
		}
	}
	moved := false
	for i := range v.levels {
		goal := 0.0
		if len(target) > i {
			goal = float64(target[i]) / 255
		}
		old := v.levels[i]
		next := math.Max(goal, old-vizFall)
		if goal > old {
			next = old + (goal-old)*vizAttack
		}
		if next < 0.004 {
			next = 0
		}
		v.levels[i] = next
		moved = moved || math.Abs(next-old) > vizVisible || (next == 0) != (old == 0)
	}
	return moved
}

// setMini opens or closes the mini player. Closing it stops the analysis, so a
// visualizer nobody is looking at costs nothing.
func (t *tui) setMini(on bool) {
	t.mini = on
	if !on && t.viz.src != nil {
		t.viz.src.Stop()
		t.viz.levels = [core.VizBands]float64{}
	}
}

func (t *tui) setVizStyle(style string) {
	t.viz.style = validVizStyle(style)
	t.prefs.Visualizer = t.viz.style
	if err := t.prefs.save(); err != nil {
		t.eng.Status = "No se pudo guardar el visualizador: " + err.Error()
		return
	}
	if t.viz.style == "off" && t.viz.src != nil {
		t.viz.src.Stop()
	}
	t.eng.Status = "Visualizador: " + vizLabels[t.viz.style]
}

// cycleViz moves d styles along the list.
func (t *tui) cycleViz(d int) {
	cur := 0
	for i, s := range vizStyles {
		if s == t.viz.style {
			cur = i
		}
	}
	t.setVizStyle(vizStyles[(cur+d+len(vizStyles))%len(vizStyles)])
}

// miniVizRows is how many rows the visualizer gets in the mini player. The
// cover gives up the room, and keeps at least five rows of its own.
func (t *tui) miniVizRows(h int, wide bool, coverRows int) int {
	if !t.vizEnabled() {
		return 0
	}
	var rows int
	if wide {
		rows = min(coverRows-7, 5)
	} else {
		rows = min(h-17, 6)
		if h >= 26 {
			rows = min(h-17, 8)
		}
	}
	if rows < 2 {
		return 0
	}
	return rows
}

// vizLines draws the visualizer in rows lines of width cells, each exactly
// width wide, over the backdrop bd, with the colors running from a to b.
func (t *tui) vizLines(width, rows int, a, b color.RGBA, bd color.Color) []string {
	lv := t.viz.levels[:]
	switch t.viz.style {
	case "mirror":
		return renderMirror(lv, width, rows, a, b, bd)
	case "wave":
		return renderWave(lv, width, rows, a, b, bd)
	}
	return renderBars(lv, width, rows, a, b, bd)
}

// ---- rendering ---------------------------------------------------------------------------

var (
	eighths = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	white   = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

// resampleLevels fits the bands to n columns: a column shows mostly the loudest
// band it covers, so a narrow display still moves with the peaks.
func resampleLevels(levels []float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		lo := i * len(levels) / n
		hi := max((i+1)*len(levels)/n, lo+1)
		peak, sum := 0.0, 0.0
		for _, v := range levels[lo:min(hi, len(levels))] {
			peak = math.Max(peak, v)
			sum += v
		}
		out[i] = 0.6*peak + 0.4*sum/float64(hi-lo)
	}
	return out
}

// gradient is the color at column i of n, in twelve steps: a smooth blend over
// thirty columns is as good to the eye with a dozen colors, and neighbors that
// share one share an escape sequence.
func gradient(a, b color.RGBA, i, n int) color.RGBA {
	if n <= 1 {
		return a
	}
	const steps = 11
	return core.MixColor(a, b, math.Round(float64(i)/float64(n-1)*steps)/steps)
}

// row builds one line of the visualizer. Each cell sets its colors and the
// backdrop (there are no resets in between), but the escape sequence is only
// written when it differs from the last one: the terminal output of the whole
// screen is parsed on every frame, and this line is most of it.
type row struct {
	sb   strings.Builder
	last string
	bd   color.Color
	used int
}

func (r *row) cell(fg color.RGBA, glyph string) {
	if s := sgr(fg, r.bd); s != r.last {
		r.sb.WriteString(s)
		r.last = s
	}
	r.sb.WriteString(glyph)
	r.used++
}

// pad writes n blank cells on the backdrop.
func (r *row) pad(n int) {
	if n > 0 {
		r.cell(color.RGBA{}, strings.Repeat(" ", n))
		r.used += n - 1
	}
}

func (r *row) done(width int) string {
	r.pad(width - r.used)
	r.sb.WriteString("\x1b[0m")
	return r.sb.String()
}

func columnsFor(width int) (barW, gap, n int) {
	barW, gap = 1, 1
	if width >= 72 {
		barW = 2
	}
	return barW, gap, max((width+gap)/(barW+gap), 1)
}

// renderBars draws the levels as bars that grow from the bottom, in eighths of a
// cell so a few rows still move smoothly.
func renderBars(levels []float64, width, rows int, a, b color.RGBA, bd color.Color) []string {
	barW, gap, n := columnsFor(width)
	vals := resampleLevels(levels, n)
	used := n*barW + (n-1)*gap
	left := (width - used) / 2
	heights := make([]int, n)
	for i, v := range vals {
		heights[i] = int(math.Min(math.Max(v, 0), 1)*float64(rows*8) + 0.5)
	}
	out := make([]string, rows)
	for y := 0; y < rows; y++ {
		ln := &row{bd: bd}
		rb := rows - 1 - y // the row counted from the bottom
		ln.pad(left)
		// Taller rows are lighter, in four steps.
		light := 0.25 * float64(rb*4/rows) / 4
		for i := 0; i < n; i++ {
			fill := heights[i] - rb*8
			glyph := eighths[min(max(fill, 0), 8)]
			col := core.MixColor(gradient(a, b, i, n), white, light)
			for k := 0; k < barW; k++ {
				ln.cell(col, glyph)
			}
			if i < n-1 {
				ln.cell(col, " ")
			}
		}
		out[y] = ln.done(width)
	}
	return out
}

// renderMirror draws the bars growing both ways from a line through the middle,
// in half rows.
func renderMirror(levels []float64, width, rows int, a, b color.RGBA, bd color.Color) []string {
	barW, gap, n := columnsFor(width)
	vals := resampleLevels(levels, n)
	used := n*barW + (n-1)*gap
	left := (width - used) / 2
	reach := make([]int, n) // half rows lit on each side of the middle
	for i, v := range vals {
		reach[i] = int(math.Min(math.Max(v, 0), 1)*float64(rows) + 0.5)
	}
	// lit says whether the half row j, counted from the top of 2*rows of them, is lit.
	lit := func(i, j int) bool {
		k := j - rows
		if j < rows {
			k = rows - 1 - j
		}
		return k < reach[i]
	}
	out := make([]string, rows)
	for y := 0; y < rows; y++ {
		ln := &row{bd: bd}
		ln.pad(left)
		for i := 0; i < n; i++ {
			glyph := " "
			switch up, down := lit(i, 2*y), lit(i, 2*y+1); {
			case up && down:
				glyph = "█"
			case up:
				glyph = "▀"
			case down:
				glyph = "▄"
			}
			col := core.MixColor(gradient(a, b, i, n), white, 0.2)
			for k := 0; k < barW; k++ {
				ln.cell(col, glyph)
			}
			if i < n-1 {
				ln.cell(col, " ")
			}
		}
		out[y] = ln.done(width)
	}
	return out
}

// brailleBit is the bit of a braille character for the dot at column x (0-1) and
// row y (0-3) of its cell.
var brailleBit = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// renderWave draws the spectrum as a curve that swings above and below a middle
// line, in braille dots: two columns and four rows of them per cell, the finest
// picture a terminal can make out of text.
func renderWave(levels []float64, width, rows int, a, b color.RGBA, bd color.Color) []string {
	dotsW, dotsH := width*2, rows*4
	// A little smoothing, so the curve is a curve and not a fence.
	smooth := make([]float64, len(levels))
	for i := range levels {
		sum, n := 0.0, 0.0
		for d := -2; d <= 2; d++ {
			if j := i + d; j >= 0 && j < len(levels) {
				w := float64(3 - int(math.Abs(float64(d)))) // 1..3
				sum += levels[j] * w
				n += w
			}
		}
		smooth[i] = sum / n
	}
	at := func(x int) float64 {
		u := float64(x) / float64(max(dotsW-1, 1)) * float64(len(smooth)-1)
		i := int(u)
		if i >= len(smooth)-1 {
			return smooth[len(smooth)-1]
		}
		f := u - float64(i)
		return smooth[i]*(1-f) + smooth[i+1]*f
	}
	grid := make([][]bool, dotsH)
	for y := range grid {
		grid[y] = make([]bool, dotsW)
	}
	center := float64(dotsH-1) / 2
	prevTop, prevBot := -1, -1
	for x := 0; x < dotsW; x++ {
		amp := math.Min(math.Max(at(x), 0), 1) * center
		top, bot := int(math.Round(center-amp)), int(math.Round(center+amp))
		span := func(from, to int) {
			if from < 0 {
				from = to
			}
			for y := min(from, to); y <= max(from, to); y++ {
				grid[y][x] = true
			}
		}
		span(prevTop, top)
		span(prevBot, bot)
		// A light stipple between the two curves gives the wave a body.
		for y := top + 1; y < bot; y++ {
			if (x+y)%2 == 0 {
				grid[y][x] = true
			}
		}
		prevTop, prevBot = top, bot
	}
	out := make([]string, rows)
	for y := 0; y < rows; y++ {
		ln := &row{bd: bd}
		for c := 0; c < width; c++ {
			bits := rune(0)
			for dx := 0; dx < 2; dx++ {
				for dy := 0; dy < 4; dy++ {
					if grid[y*4+dy][c*2+dx] {
						bits |= brailleBit[dx][dy]
					}
				}
			}
			glyph := " "
			if bits != 0 {
				glyph = string(0x2800 + bits)
			}
			ln.cell(core.MixColor(gradient(a, b, c, width), white, 0.2), glyph)
		}
		out[y] = ln.done(width)
	}
	return out
}
