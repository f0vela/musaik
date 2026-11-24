package core

import (
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"musaik/internal/player"
)

func sine(freq, amp float64, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*freq*float64(i)/vizRate))
	}
	return out
}

func argmax(b []uint8) int {
	best := 0
	for i, v := range b {
		if v > b[best] {
			best = i
		}
	}
	return best
}

// bandOf is the band a frequency should land in.
func bandOf(freq float64) int {
	return int(float64(VizBands) * math.Log(freq/vizFMin) / math.Log(vizFMax/vizFMin))
}

func TestSpectrumPutsEachFrequencyInItsBand(t *testing.T) {
	sp := newSpectrum()
	for _, f := range []float64{100, 220, 440, 1000, 2500, 4000, 8000} {
		fr := sp.frame(sine(f, 0.8, vizWindow))
		got, want := argmax(fr[:]), bandOf(f)
		if d := got - want; d < -1 || d > 1 {
			t.Errorf("%.0f Hz: loudest band %d, want about %d", f, got, want)
		}
	}
}

func TestSpectrumLevels(t *testing.T) {
	sp := newSpectrum()
	silent := sp.frame(make([]float32, vizWindow))
	for i, v := range silent {
		if v != 0 {
			t.Fatalf("silence must be flat, band %d is %d", i, v)
		}
	}
	loud := sp.frame(sine(1000, 0.9, vizWindow))
	quiet := sp.frame(sine(1000, 0.02, vizWindow))  // about -34 dBFS
	faint := sp.frame(sine(1000, 0.001, vizWindow)) // about -60 dBFS
	b := bandOf(1000)
	if loud[b] < 200 {
		t.Fatalf("a loud tone fills its band: %d", loud[b])
	}
	if !(faint[b] < quiet[b] && quiet[b] < loud[b]) {
		t.Fatalf("the level grows with the loudness: faint %d, quiet %d, loud %d", faint[b], quiet[b], loud[b])
	}
	if faint[b] > 90 {
		t.Fatalf("a very faint tone barely shows: %d", faint[b])
	}
	// A band far from the tone stays low.
	if far := loud[bandOf(8000)]; far > 60 {
		t.Fatalf("leakage into a distant band: %d", far)
	}
}

func TestFFTAgainstDirectTransform(t *testing.T) {
	sp := newSpectrum()
	x := make([]float32, vizWindow)
	for i := range x {
		x[i] = float32(math.Sin(float64(i)*0.05) + 0.5*math.Cos(float64(i)*0.31))
	}
	sp.frame(x)
	for _, k := range []int{3, 17, 80, 300} {
		var re, im float64
		for n := 0; n < vizWindow; n++ {
			v := float64(x[n]) * sp.window[n]
			a := -2 * math.Pi * float64(k) * float64(n) / vizWindow
			re += v * math.Cos(a)
			im += v * math.Sin(a)
		}
		if d := math.Hypot(sp.re[k]-re, sp.im[k]-im); d > 1e-6 {
			t.Fatalf("bin %d differs from the direct transform by %g", k, d)
		}
	}
}

// fakeFFmpeg is a script that ignores its arguments and prints a file of audio,
// as ffmpeg would print the decoded song.
func fakeFFmpeg(t *testing.T, pcm []float32) string {
	t.Helper()
	dir := t.TempDir()
	raw := make([]byte, 2*len(pcm))
	for i, v := range pcm {
		binary.LittleEndian.PutUint16(raw[2*i:], uint16(int16(v*32767)))
	}
	data := filepath.Join(dir, "audio.raw")
	if err := os.WriteFile(data, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec cat '"+data+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAnalyzerFollowsThePosition(t *testing.T) {
	a := NewAnalyzerWith(fakeFFmpeg(t, sine(1000, 0.8, 3*vizRate)))
	defer a.Stop()
	a.Follow("song.mp3", 0)
	waitFor(t, "the whole song to be analyzed", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.finished
	})
	if err := a.Err(); err != nil {
		t.Fatal(err)
	}
	fr, ok := a.At(1500 * time.Millisecond)
	if !ok || len(fr) != VizBands {
		t.Fatalf("a frame in the middle of the song: %v %d", ok, len(fr))
	}
	if d := argmax(fr) - bandOf(1000); d < -1 || d > 1 {
		t.Fatalf("the tone shows in band %d, want about %d", argmax(fr), bandOf(1000))
	}
	if _, ok := a.At(60 * time.Second); ok {
		t.Fatal("there is nothing past the end")
	}
	n := len(a.frames) / VizBands
	if n < 85 || n > 91 {
		t.Fatalf("3 seconds are about 90 frames: %d", n)
	}
}

func TestAnalyzerRestartsWhereTheMusicIs(t *testing.T) {
	a := NewAnalyzerWith(fakeFFmpeg(t, sine(440, 0.8, 2*vizRate)))
	defer a.Stop()
	a.Follow("a.mp3", 10*time.Second) // joining a song in the middle
	if a.from != 10*VizFPS {
		t.Fatalf("it starts at the position, not at the beginning: %d", a.from)
	}
	gen := a.gen
	a.Follow("a.mp3", 10*time.Second+500*time.Millisecond)
	if a.gen != gen {
		t.Fatal("inside what is covered nothing restarts")
	}
	a.Follow("a.mp3", 4*time.Second) // seek back
	if a.from != 4*VizFPS || a.gen == gen {
		t.Fatalf("a seek back restarts there: from=%d", a.from)
	}
	gen = a.gen
	a.Follow("a.mp3", 120*time.Second) // seek far ahead of what is known
	if a.from != 120*VizFPS || a.gen == gen {
		t.Fatalf("a seek far ahead restarts there: from=%d", a.from)
	}
	gen = a.gen
	a.Follow("b.mp3", 0)
	if a.path != "b.mp3" || a.gen == gen {
		t.Fatal("a new song restarts")
	}
	a.Stop()
	if _, ok := a.At(0); ok || a.path != "" {
		t.Fatal("stopping forgets the spectrum")
	}
}

func TestAnalyzerReportsAMissingFFmpeg(t *testing.T) {
	a := NewAnalyzerWith("/nonexistent/ffmpeg")
	defer a.Stop()
	a.Follow("x.mp3", 0)
	waitFor(t, "the failure", func() bool { return a.Err() != nil })
	if _, ok := a.At(0); ok {
		t.Fatal("no frames without ffmpeg")
	}
}

// With the real ffmpeg: it decodes a real file to the same result.
func TestAnalyzerWithRealFFmpeg(t *testing.T) {
	ff, err := player.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	wav := filepath.Join(t.TempDir(), "tone.wav")
	if out, err := exec.Command(ff, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=2000:duration=3", "-ar", "44100", wav).CombinedOutput(); err != nil {
		t.Fatalf("making the tone: %v %s", err, out)
	}
	a := NewAnalyzerWith(ff)
	defer a.Stop()
	a.Follow(wav, time.Second) // from the middle: -ss is honored
	waitFor(t, "frames", func() bool { _, ok := a.At(1500 * time.Millisecond); return ok })
	fr, _ := a.At(1500 * time.Millisecond)
	if d := argmax(fr) - bandOf(2000); d < -1 || d > 1 {
		t.Fatalf("a 2 kHz tone shows in band %d, want about %d", argmax(fr), bandOf(2000))
	}
}

// A sweep whose pitch at every instant is known checks the time alignment as
// well as the frequency: the bar that is lit must be the one for that moment.
func TestAnalyzerKeepsTimeWithASweep(t *testing.T) {
	ff, err := player.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	// sin(2π(150+1500t)t) has the instantaneous frequency 150+3000t.
	wav := filepath.Join(t.TempDir(), "sweep.wav")
	if out, err := exec.Command(ff, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "aevalsrc=0.8*sin(2*PI*(150+1500*t)*t):d=3:s=44100", wav).CombinedOutput(); err != nil {
		t.Fatalf("making the sweep: %v %s", err, out)
	}
	a := NewAnalyzerWith(ff)
	defer a.Stop()
	a.Follow(wav, 0)
	waitFor(t, "the sweep to be analyzed", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.finished
	})
	last := -1
	for _, sec := range []float64{0.3, 0.8, 1.3, 1.8, 2.3, 2.7} {
		fr, ok := a.At(time.Duration(sec * float64(time.Second)))
		if !ok {
			t.Fatalf("no frame at %.1f s", sec)
		}
		got, want := argmax(fr), bandOf(150+3000*sec)
		if d := got - want; d < -3 || d > 3 {
			t.Errorf("at %.1f s the pitch is %.0f Hz (band %d) but band %d is lit", sec, 150+3000*sec, want, got)
		}
		if got <= last {
			t.Errorf("the lit band must climb with the sweep: %d after %d at %.1f s", got, last, sec)
		}
		last = got
	}
}
