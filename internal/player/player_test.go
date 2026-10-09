package player

import (
	"testing"
	"time"
)

func TestPositionWhilePlaying(t *testing.T) {
	base := time.Unix(1000, 0)
	p := &Player{
		nowPath:    "/tmp/song.mp3",
		seekBase:   10 * time.Second,
		startedAt:  base,
		duration:   300 * time.Second,
		pauseAccum: 0,
		clockNow: func() time.Time {
			return base.Add(15 * time.Second)
		},
	}
	pos := p.positionLocked()
	if pos != 25*time.Second {
		t.Fatalf("expected 25s, got %v", pos)
	}
}

func TestPositionWhilePaused(t *testing.T) {
	base := time.Unix(1000, 0)
	p := &Player{
		nowPath:    "/tmp/song.mp3",
		seekBase:   0,
		startedAt:  base,
		duration:   300 * time.Second,
		isPaused:   true,
		pausedAt:   base.Add(20 * time.Second),
		pauseAccum: 5 * time.Second,
		clockNow: func() time.Time {
			return base.Add(50 * time.Second)
		},
	}
	pos := p.positionLocked()
	if pos != 15*time.Second {
		t.Fatalf("expected 15s, got %v", pos)
	}
}

func TestPositionClampToDuration(t *testing.T) {
	base := time.Unix(1000, 0)
	p := &Player{
		nowPath:   "/tmp/song.mp3",
		seekBase:  40 * time.Second,
		startedAt: base,
		duration:  45 * time.Second,
		clockNow: func() time.Time {
			return base.Add(20 * time.Second)
		},
	}
	pos := p.positionLocked()
	if pos != 45*time.Second {
		t.Fatalf("expected clamp to 45s, got %v", pos)
	}
}
