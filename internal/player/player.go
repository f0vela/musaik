package player

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

type Player struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	nowPath   string
	isPaused  bool
	lastError error
}

func New() *Player {
	return &Player{}
}

func (p *Player) Play(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopLocked()

	bin, args, err := backendFor(path)
	if err != nil {
		p.lastError = err
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, append(args, path)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		p.lastError = fmt.Errorf("starting %s: %w", bin, err)
		return p.lastError
	}

	p.cmd = cmd
	p.cancel = cancel
	p.nowPath = path
	p.isPaused = false
	p.lastError = nil

	go func(cmd *exec.Cmd, cancel context.CancelFunc) {
		_ = cmd.Wait()
		cancel()
	}(cmd, cancel)

	return nil
}

func (p *Player) TogglePause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("nothing is playing")
	}

	var sig syscall.Signal
	if p.isPaused {
		sig = syscall.SIGCONT
	} else {
		sig = syscall.SIGSTOP
	}
	if err := p.cmd.Process.Signal(sig); err != nil {
		p.lastError = fmt.Errorf("toggling pause: %w", err)
		return p.lastError
	}
	p.isPaused = !p.isPaused
	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
	p.cancel = nil
	p.nowPath = ""
	p.isPaused = false
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

func backendFor(path string) (string, []string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	afplayPath, afplayErr := exec.LookPath("afplay")
	ffplayPath, ffplayErr := exec.LookPath("ffplay")

	switch ext {
	case ".mp3", ".mp4":
		if afplayErr == nil {
			return afplayPath, nil, nil
		}
		if ffplayErr == nil {
			return ffplayPath, []string{"-nodisp", "-autoexit", "-loglevel", "error"}, nil
		}
	case ".flac", ".ogg":
		if ffplayErr == nil {
			return ffplayPath, []string{"-nodisp", "-autoexit", "-loglevel", "error"}, nil
		}
		if afplayErr == nil {
			return afplayPath, nil, nil
		}
	default:
		return "", nil, fmt.Errorf("unsupported format: %s", ext)
	}
	return "", nil, errors.New("no audio backend found (install ffmpeg for ffplay)")
}
