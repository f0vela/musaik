package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestRenderWindowLayouts writes snapshots of the whole window at several
// widths. It only runs when MUSAIK_SNAPSHOT_DIR is set.
func TestRenderWindowLayouts(t *testing.T) {
	dir := os.Getenv("MUSAIK_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("MUSAIK_SNAPSHOT_DIR not set")
	}
	cases := []struct {
		name          string
		width         int
		sidebarHidden bool
	}{
		{"1200", 1200, false},
		{"1400", 1400, false},
		{"900", 900, false},
		{"800", 800, false},
		{"640-nosidebar", 640, true},
	}
	for _, tc := range cases {
		a := newApp()
		a.sidebarHidden = tc.sidebarHidden
		for _, s := range a.store.Songs {
			if len(s.Title) > 18 {
				a.selectedID = s.ID
				break
			}
		}
		img := ui.Render(a.view, tc.width, 520, 2)
		writePNG(t, filepath.Join(dir, fmt.Sprintf("layout-%s.png", tc.name)), img)
	}
}
