package main

import (
	"runtime/debug"
	"sync"
	"time"
)

const (
	releaseGap   = time.Second
	releaseAfter = 1500 * time.Millisecond
)

var release struct {
	sync.Mutex
	last  time.Time
	timer *time.Timer
}

// releaseMemory collects the garbage a heavy step left and gives the freed pages
// back to the system. Decoding a cover is tens of megabytes of pixels that are
// dropped as soon as the cover is scaled; Go would return them on its own, but
// slowly, and the process would keep holding what it only needed for a moment.
//
// It runs right away if the last release was a while ago, which keeps a burst of
// covers from piling up, and once more after the last call, so the garbage of the
// final step is not left behind. Call it off the UI goroutine, after the rare
// heavy steps.
func releaseMemory() {
	release.Lock()
	defer release.Unlock()
	now := time.Now()
	if now.Sub(release.last) >= releaseGap {
		release.last = now
		debug.FreeOSMemory()
	}
	if release.timer == nil {
		release.timer = time.AfterFunc(releaseAfter, debug.FreeOSMemory)
	} else {
		release.timer.Reset(releaseAfter)
	}
}
