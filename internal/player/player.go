package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EventType string

const (
	EventTrackEnded EventType = "track-ended"
)

type Event struct {
	Type EventType
	Path string
	Err  error
}

type State struct {
	Path     string
	Playing  bool
	Paused   bool
	Position time.Duration
	Duration time.Duration
}

type Player struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	controlIn   io.WriteCloser
	nowPath     string
	isPaused    bool
	lastError   error
	duration    time.Duration
	seekBase    time.Duration
	startedAt   time.Time
	pausedAt    time.Time
	pauseAccum  time.Duration
	generation  uint64
	events      chan Event
	clockNow    func() time.Time
	disableExec bool
	volume      int
}

func New() *Player {
	return &Player{
		events:   make(chan Event, 16),
		clockNow: time.Now,
		volume:   80,
	}
}

func (p *Player) Play(path string, knownDuration time.Duration) error {
	return p.PlayFrom(path, knownDuration, 0)
}

func (p *Player) PlayFrom(path string, knownDuration time.Duration, offset time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopLocked()
	if knownDuration <= 0 {
		knownDuration = probeDuration(path)
	}

	if offset < 0 {
		offset = 0
	}
	if knownDuration > 0 && offset > knownDuration {
		offset = knownDuration
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd, controlIn, err := startBackend(ctx, path, offset, p.volume, p.disableExec)
	if err != nil {
		cancel()
		p.lastError = err
		return err
	}
	p.cmd = cmd
	p.cancel = cancel
	p.controlIn = controlIn
	p.nowPath = path
	p.isPaused = false
	p.duration = knownDuration
	p.seekBase = offset
	p.startedAt = p.clockNow()
	p.pauseAccum = 0
	p.pausedAt = time.Time{}
	p.lastError = nil
	p.generation++
	gen := p.generation

	go p.waitForTrackEnd(cmd, cancel, path, gen)

	return nil
}

func (p *Player) TogglePause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.nowPath == "" {
		return errors.New("nothing is playing")
	}

	if p.isPaused {
		ctx, cancel := context.WithCancel(context.Background())
		cmd, controlIn, err := startBackend(ctx, p.nowPath, p.seekBase, p.volume, p.disableExec)
		if err != nil {
			cancel()
			p.lastError = err
			return err
		}
		p.cmd = cmd
		p.cancel = cancel
		p.controlIn = controlIn
		p.startedAt = p.clockNow()
		p.pauseAccum = 0
		p.pausedAt = time.Time{}
		p.isPaused = false
		p.generation++
		gen := p.generation
		go p.waitForTrackEnd(cmd, cancel, p.nowPath, gen)
		return nil
	}

	if p.cmd == nil || p.cmd.Process == nil {
		p.lastError = errors.New("nothing is playing")
		return p.lastError
	}
	p.seekBase = p.positionLocked()
	p.stopProcessKeepTrackLocked()
	p.isPaused = true
	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	p.generation++
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.controlIn != nil {
		_ = p.controlIn.Close()
	}
	p.cmd = nil
	p.cancel = nil
	p.controlIn = nil
	p.nowPath = ""
	p.isPaused = false
	p.duration = 0
	p.seekBase = 0
	p.startedAt = time.Time{}
	p.pausedAt = time.Time{}
	p.pauseAccum = 0
}

func (p *Player) stopProcessKeepTrackLocked() {
	p.generation++
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.controlIn != nil {
		_ = p.controlIn.Close()
	}
	p.cmd = nil
	p.cancel = nil
	p.controlIn = nil
	p.startedAt = time.Time{}
	p.pausedAt = time.Time{}
	p.pauseAccum = 0
}

func (p *Player) NowPlaying() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nowPath
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPaused
}

func (p *Player) LastError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastError
}

func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()

	st := State{
		Path:     p.nowPath,
		Playing:  p.nowPath != "",
		Paused:   p.isPaused,
		Duration: p.duration,
	}
	st.Position = p.positionLocked()
	return st
}

func (p *Player) SeekTo(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.nowPath == "" {
		return errors.New("nothing is playing")
	}
	if pos < 0 {
		pos = 0
	}
	if p.duration > 0 && pos > p.duration {
		pos = p.duration
	}
	if p.isPaused && (p.cmd == nil || p.cmd.Process == nil) {
		p.seekBase = pos
		return nil
	}

	path := p.nowPath
	duration := p.duration
	p.stopProcessKeepTrackLocked()

	ctx, cancel := context.WithCancel(context.Background())
	cmd, controlIn, err := startBackend(ctx, path, pos, p.volume, p.disableExec)
	if err != nil {
		cancel()
		p.lastError = err
		return err
	}

	p.cmd = cmd
	p.cancel = cancel
	p.controlIn = controlIn
	p.nowPath = path
	p.isPaused = false
	p.duration = duration
	p.seekBase = pos
	p.startedAt = p.clockNow()
	p.pauseAccum = 0
	p.pausedAt = time.Time{}
	p.lastError = nil
	p.generation++
	gen := p.generation
	go p.waitForTrackEnd(cmd, cancel, path, gen)
	return nil
}

func (p *Player) Events() <-chan Event {
	return p.events
}

func (p *Player) Volume() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

func (p *Player) SetVolume(volume int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	if p.volume == volume {
		return nil
	}
	p.volume = volume

	if p.nowPath == "" || p.isPaused || p.cmd == nil {
		return nil
	}

	path := p.nowPath
	duration := p.duration
	pos := p.positionLocked()
	p.stopProcessKeepTrackLocked()

	ctx, cancel := context.WithCancel(context.Background())
	cmd, controlIn, err := startBackend(ctx, path, pos, p.volume, p.disableExec)
	if err != nil {
		cancel()
		p.lastError = err
		return err
	}

	p.cmd = cmd
	p.cancel = cancel
	p.controlIn = controlIn
	p.nowPath = path
	p.isPaused = false
	p.duration = duration
	p.seekBase = pos
	p.startedAt = p.clockNow()
	p.pauseAccum = 0
	p.pausedAt = time.Time{}
	p.lastError = nil
	p.generation++
	gen := p.generation
	go p.waitForTrackEnd(cmd, cancel, path, gen)

	return nil
}

func (p *Player) waitForTrackEnd(cmd *exec.Cmd, cancel context.CancelFunc, path string, gen uint64) {
	waitErr := cmd.Wait()
	cancel()

	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.generation {
		return
	}
	p.cmd = nil
	p.cancel = nil
	p.controlIn = nil
	p.isPaused = false
	p.pausedAt = time.Time{}
	p.pauseAccum = 0
	p.seekBase = 0
	p.startedAt = time.Time{}
	p.duration = 0
	p.nowPath = ""
	if waitErr != nil && !strings.Contains(waitErr.Error(), "signal: killed") {
		p.lastError = waitErr
	}

	select {
	case p.events <- Event{Type: EventTrackEnded, Path: path, Err: waitErr}:
	default:
	}
}

func (p *Player) positionLocked() time.Duration {
	if p.nowPath == "" {
		return 0
	}
	pos := p.seekBase
	if p.startedAt.IsZero() {
		return pos
	}

	if p.isPaused {
		pos += p.pausedAt.Sub(p.startedAt) - p.pauseAccum
	} else {
		pos += p.clockNow().Sub(p.startedAt) - p.pauseAccum
	}

	if pos < 0 {
		pos = 0
	}
	if p.duration > 0 && pos > p.duration {
		pos = p.duration
	}
	return pos
}

func startBackend(ctx context.Context, path string, offset time.Duration, volume int, disableExec bool) (*exec.Cmd, io.WriteCloser, error) {
	bin, err := findExecutable("ffplay")
	if err != nil {
		return nil, nil, errors.New("ffplay no está disponible (instala ffmpeg)")
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	offsetSec := strconv.FormatFloat(offset.Seconds(), 'f', 3, 64)
	args := []string{
		"-nodisp",
		"-autoexit",
		"-loglevel", "error",
		"-volume", strconv.Itoa(volume),
		"-ss", offsetSec,
		path,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdin, stdinErr := cmd.StdinPipe()
	if stdinErr != nil {
		return nil, nil, fmt.Errorf("opening ffplay stdin: %w", stdinErr)
	}
	if disableExec {
		return cmd, stdin, nil
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("starting ffplay: %w", err)
	}
	return cmd, stdin, nil
}

func probeDuration(path string) time.Duration {
	bin, err := findExecutable("ffprobe")
	if err != nil {
		return 0
	}
	out, err := exec.Command(
		bin,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0
	}
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

func findExecutable(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	paths := []string{
		filepath.Join("/opt/homebrew/bin", name),
		filepath.Join("/usr/local/bin", name),
		filepath.Join("/opt/local/bin", name),
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		return p, nil
	}
	return "", fmt.Errorf("%s not found", name)
}
