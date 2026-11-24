package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"musaik/internal/player"
)

// The visualizer needs the audio itself, and the player is an external ffplay
// that gives no access to it. So the Analyzer decodes the same file on its own
// with ffmpeg, runs a Fourier transform over it and keeps the spectrum of each
// thirtieth of a second. What is drawn is looked up by the playback position, so
// it follows pauses and seeks and does not depend on how ffplay plays.
const (
	// VizBands is how many frequency bands a frame has, spaced like a piano,
	// logarithmically, from about 50 Hz to 10 kHz.
	VizBands = 64
	// VizFPS is how many frames there are per second of audio.
	VizFPS = 30

	vizRate   = 22050
	vizWindow = 2048
	vizHop    = vizRate / VizFPS // 735 samples: exactly one frame
	vizFMin   = 50.0
	vizFMax   = 10500.0
	// The analysis runs this many times faster than playback at most: fast enough
	// to stay well ahead, slow enough that it never hogs a core. The work is the
	// same at any speed; a lower one spreads it out.
	vizSpeed = 6
	// Seeking further than this beyond what has been analyzed restarts it there.
	vizReach = 3 * VizFPS
)

// spectrum turns windows of audio into band levels.
type spectrum struct {
	window   [vizWindow]float64
	cos, sin [vizWindow / 2]float64
	rev      [vizWindow]int
	re, im   [vizWindow]float64
	lo, hi   [VizBands]int
	tilt     [VizBands]float64
}

func newSpectrum() *spectrum {
	s := &spectrum{}
	for i := range s.window {
		s.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(vizWindow-1)) // Hann
	}
	for i := range s.cos {
		a := -2 * math.Pi * float64(i) / float64(vizWindow)
		s.cos[i], s.sin[i] = math.Cos(a), math.Sin(a)
	}
	bits := 0
	for 1<<bits < vizWindow {
		bits++
	}
	for i := range s.rev {
		r := 0
		for b := 0; b < bits; b++ {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		s.rev[i] = r
	}
	binHz := float64(vizRate) / vizWindow
	edge := func(k int) float64 { return vizFMin * math.Pow(vizFMax/vizFMin, float64(k)/VizBands) }
	for k := 0; k < VizBands; k++ {
		lo := int(math.Round(edge(k) / binHz))
		hi := int(math.Round(edge(k+1) / binHz))
		if hi <= lo {
			hi = lo + 1
		}
		s.lo[k], s.hi[k] = min(lo, vizWindow/2-1), min(hi, vizWindow/2)
		center := math.Sqrt(edge(k) * edge(k+1))
		// Music falls off about 3 dB per octave toward the treble; give that back
		// so the right side of the display is not always flat.
		s.tilt[k] = 3 * math.Log2(center/1000)
	}
	return s
}

// frame computes the levels, 0-255, of vizWindow samples in -1..1.
func (s *spectrum) frame(samples []float32) [VizBands]uint8 {
	for i := 0; i < vizWindow; i++ {
		s.re[s.rev[i]] = float64(samples[i]) * s.window[i]
		s.im[s.rev[i]] = 0
	}
	for size := 2; size <= vizWindow; size <<= 1 {
		half, step := size/2, vizWindow/size
		for start := 0; start < vizWindow; start += size {
			for j := 0; j < half; j++ {
				c, sn := s.cos[j*step], s.sin[j*step]
				a, b := start+j, start+j+half
				tr := s.re[b]*c - s.im[b]*sn
				ti := s.re[b]*sn + s.im[b]*c
				s.re[b], s.im[b] = s.re[a]-tr, s.im[a]-ti
				s.re[a], s.im[a] = s.re[a]+tr, s.im[a]+ti
			}
		}
	}
	var out [VizBands]uint8
	for k := 0; k < VizBands; k++ {
		peak := 0.0
		for bin := s.lo[k]; bin < s.hi[k]; bin++ {
			peak = math.Max(peak, math.Hypot(s.re[bin], s.im[bin]))
		}
		// A full-scale sine peaks at vizWindow/4 with this window: that is 0 dB.
		db := 20*math.Log10(peak/(vizWindow/4)+1e-9) + s.tilt[k]
		v := (db + 70) / 62
		out[k] = uint8(math.Min(math.Max(v, 0), 1) * 255)
	}
	return out
}

// Analyzer keeps the spectrum of the song that plays. It is safe for concurrent
// use: the analysis runs in its own goroutine.
type Analyzer struct {
	ffmpeg string

	mu       sync.Mutex
	path     string
	from     int     // the frame of frames[0]
	frames   []uint8 // VizBands bytes per frame
	finished bool
	failed   error
	gen      int
	cancel   context.CancelFunc
}

// NewAnalyzer finds ffmpeg; without it there is no visualizer.
func NewAnalyzer() (*Analyzer, error) {
	path, err := player.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	return &Analyzer{ffmpeg: path}, nil
}

// NewAnalyzerWith uses the given ffmpeg.
func NewAnalyzerWith(ffmpeg string) *Analyzer { return &Analyzer{ffmpeg: ffmpeg} }

func frameAt(pos time.Duration) int {
	return int(math.Round(pos.Seconds() * VizFPS))
}

// Follow makes sure the analysis covers the playback position of path. It does
// nothing when it already does, so it is fine to call at every tick; it starts a
// new analysis when the song changes or the position jumps outside what is known.
func (a *Analyzer) Follow(path string, pos time.Duration) {
	f := frameAt(pos)
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case a.path != path:
	case f < a.from:
	case !a.finished && f > a.from+len(a.frames)/VizBands+vizReach:
	default:
		return
	}
	a.start(path, f)
}

// start must be called with the lock held.
func (a *Analyzer) start(path string, from int) {
	if a.cancel != nil {
		a.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.path, a.from, a.frames = cancel, path, max(from, 0), nil
	a.finished, a.failed = false, nil
	a.gen++
	go a.run(ctx, a.gen, path, a.from)
}

// At returns the levels of the frame for a playback position, if it has been
// analyzed.
func (a *Analyzer) At(pos time.Duration) ([]uint8, bool) {
	f := frameAt(pos)
	a.mu.Lock()
	defer a.mu.Unlock()
	i := f - a.from
	if i < 0 || (i+1)*VizBands > len(a.frames) {
		return nil, false
	}
	return append([]uint8(nil), a.frames[i*VizBands:(i+1)*VizBands]...), true
}

// Err is why the analysis stopped without a result, if it did.
func (a *Analyzer) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failed
}

// Stop ends the analysis and forgets the spectrum.
func (a *Analyzer) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.path, a.frames, a.finished, a.failed = "", nil, false, nil
	a.gen++
}

func (a *Analyzer) run(ctx context.Context, gen int, path string, from int) {
	args := []string{"-nostdin", "-v", "error"}
	if from > 0 {
		args = append(args, "-ss", strconv.FormatFloat(float64(from)/VizFPS, 'f', 3, 64))
	}
	args = append(args, "-i", path, "-vn", "-sn", "-dn", "-ac", "1", "-ar", strconv.Itoa(vizRate), "-f", "s16le", "-")
	cmd := exec.CommandContext(ctx, a.ffmpeg, args...)
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		a.finish(gen, err)
		return
	}
	defer func() { _ = cmd.Wait() }()

	sp := newSpectrum()
	// Half a window of silence in front centers the first frame on its time.
	buf := make([]float32, vizWindow/2, 4*vizWindow)
	r := bufio.NewReaderSize(out, 1<<16)
	raw := make([]byte, 8192)
	began := time.Now()
	produced := 0
	var carry []byte
	for {
		n, rerr := r.Read(raw)
		data := append(carry, raw[:n]...)
		whole := len(data) &^ 1
		for i := 0; i < whole; i += 2 {
			buf = append(buf, float32(int16(binary.LittleEndian.Uint16(data[i:])))/32768)
		}
		carry = append(carry[:0], data[whole:]...)
		var batch []uint8
		for len(buf) >= vizWindow {
			f := sp.frame(buf)
			batch = append(batch, f[:]...)
			buf = append(buf[:0], buf[vizHop:]...)
			produced++
		}
		if len(batch) > 0 {
			a.mu.Lock()
			if a.gen != gen {
				a.mu.Unlock()
				return
			}
			a.frames = append(a.frames, batch...)
			a.mu.Unlock()
			// Do not run ahead of the music by more than vizSpeed times.
			ahead := time.Duration(float64(produced)/VizFPS/vizSpeed*float64(time.Second)) - time.Since(began)
			if ahead > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(ahead):
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				rerr = nil
			}
			a.finish(gen, rerr)
			return
		}
	}
}

func (a *Analyzer) finish(gen int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.gen != gen {
		return
	}
	a.finished, a.failed = true, err
}
