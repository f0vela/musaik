package main

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"musaik/internal/lyrics"
	"musaik/internal/model"
)

const (
	lyricsRowH       = 44  // every lyric line has this height, so the current one can be centered
	lyricsViewH      = 480 // height of the scrolling area
	lyricsManualHold = 5 * time.Second
	lyricsRetryAfter = 20 * time.Second
	lyricsKeep       = 30
)

type lyricsState int

const (
	lyricsLoading lyricsState = iota
	lyricsReady
	lyricsNone
	lyricsFailed
)

// lyricsResult is what is known about one song's lyrics.
type lyricsResult struct {
	state lyricsState
	data  *lyrics.Lyrics
	err   string
	at    time.Time
}

// lyricsPanel is the lyrics window: which song it shows, the lookups so far,
// and where it is scrolled. While synced lyrics play it keeps the current line
// in the middle, unless the user scrolled by hand a moment ago.
type lyricsPanel struct {
	open   bool
	client *lyrics.Client
	cache  map[string]*lyricsResult

	songID      string
	scroll      ui.ScrollState
	haveSet     bool
	lastSetY    float32
	lastCur     int
	recenter    bool
	manualUntil time.Time
}

func (a *app) toggleLyrics() {
	a.lyrics.open = !a.lyrics.open
	if a.lyrics.open {
		a.lyrics.songID = ""
	}
}

// lyricsSong is the song the player bar describes: the playing one, or else
// the selected one.
func (a *app) lyricsSong() (model.Song, bool) {
	if s, ok := a.currentPlayingSong(); ok {
		return s, true
	}
	_, s, ok := a.findSong(a.selectedID)
	return s, ok
}

// lyricsFor returns the lyrics of song and starts looking them up the first
// time it is asked (a failed lookup is tried again after a few seconds).
func (a *app) lyricsFor(song model.Song) lyricsResult {
	lp := &a.lyrics
	if lp.cache == nil {
		lp.cache = map[string]*lyricsResult{}
	}
	if r, ok := lp.cache[song.ID]; ok && (r.state != lyricsFailed || time.Since(r.at) < lyricsRetryAfter) {
		return *r
	}
	if lp.client == nil {
		lp.client = lyrics.NewClient("Musaik")
	}
	if len(lp.cache) >= lyricsKeep {
		lp.cache = map[string]*lyricsResult{}
	}
	pending := &lyricsResult{state: lyricsLoading, at: time.Now()}
	lp.cache[song.ID] = pending
	client := lp.client
	q := lyrics.Query{Title: song.Title, Artist: song.Artist, Album: song.Album, Duration: song.Duration, Path: song.Path}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		l, err := client.Fetch(ctx, q)
		a.updateUI(func() {
			done := &lyricsResult{at: time.Now()}
			switch {
			case err == nil:
				done.state, done.data = lyricsReady, l
			case errors.Is(err, lyrics.ErrNotFound):
				done.state = lyricsNone
			default:
				done.state, done.err = lyricsFailed, err.Error()
			}
			if lp.cache[song.ID] == pending {
				lp.cache[song.ID] = done
			}
		})
	}()
	return *pending
}

func lyricsButtonColor(open bool) ui.Color {
	if open {
		return sonoraAccent
	}
	return sonoraTextMuted
}

// viewLyricsModal is the lyrics window, opened from the player bar or with ⌘L.
func (a *app) viewLyricsModal(c *ui.Context) {
	ui.Modal(c, &a.lyrics.open, func() {
		song, hasSong := a.lyricsSong()
		ui.Column(c).
			Width(640).
			Height(620).
			Padding(12, 12).
			Gap(8).
			Background(sonoraCardAlt).
			Border(1, sonoraBorder).
			Radius(12).
			Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					if ui.Button(c, "✕").Width(28).Height(28).Padding(0, 0).Radius(14).Clicked() {
						a.lyrics.open = false
					}
					ui.Column(c).Grow(1).MinWidth(0).Clip().Children(func() {
						ui.Text(c, "Letra").FontSize(fs(22)).FontWeight(700).TextColor(sonoraText)
						if hasSong {
							sub := song.Title
							if song.Artist != "" {
								sub += " — " + song.Artist
							}
							ui.Text(c, sub).SingleLine().FontSize(fs(12)).TextColor(sonoraTextMuted)
						}
					})
				})
				ui.Divider(c)
				ui.Box(c).Height(lyricsViewH).Children(func() {
					a.viewLyricsBody(c, song, hasSong)
				})
				ui.Text(c, "Letras de LRCLIB · clic en una línea para ir a ese punto").
					SingleLine().FontSize(fs(11)).TextColor(sonoraTextTertiary)
			})
	})
}

func lyricsMessage(c *ui.Context, title string, detail ...string) {
	ui.Column(c).Fill().Center().Gap(6).Children(func() {
		ui.Text(c, title).FontSize(fs(16)).FontWeight(600).TextColor(sonoraText).TextAlign(ui.Center)
		for _, d := range detail {
			ui.Text(c, d).FontSize(fs(12)).TextColor(sonoraTextMuted).TextAlign(ui.Center)
		}
	})
}

func (a *app) viewLyricsBody(c *ui.Context, song model.Song, hasSong bool) {
	lp := &a.lyrics
	if !hasSong {
		lyricsMessage(c, "No hay canción")
		return
	}
	if lp.songID != song.ID {
		lp.songID, lp.haveSet, lp.recenter, lp.lastCur = song.ID, false, false, -2
		lp.manualUntil = time.Time{}
		lp.scroll.Y = 0
	}
	res := a.lyricsFor(song)
	switch res.state {
	case lyricsLoading:
		lyricsMessage(c, "Buscando letra…", "LRCLIB · lrclib.net")
		return
	case lyricsFailed:
		ui.Column(c).Fill().Center().Gap(8).Children(func() {
			ui.Text(c, "No se pudo buscar la letra").FontSize(fs(16)).FontWeight(600).TextColor(sonoraText)
			ui.Text(c, res.err).FontSize(fs(12)).TextColor(sonoraTextMuted).TextAlign(ui.Center)
			if ui.Button(c, "Reintentar").Clicked() {
				delete(lp.cache, song.ID)
			}
		})
		return
	case lyricsNone:
		ui.Column(c).Fill().Center().Gap(8).Children(func() {
			ui.Text(c, "No hay letra para esta canción").FontSize(fs(16)).FontWeight(600).TextColor(sonoraText)
			ui.Text(c, "Pon un archivo .lrc con el nombre del audio al lado,\no incrusta la letra en sus etiquetas.").
				FontSize(fs(12)).TextColor(sonoraTextMuted).TextAlign(ui.Center)
			if ui.Button(c, "Buscar de nuevo").Clicked() {
				delete(lp.cache, song.ID)
			}
		})
		return
	}
	l := res.data
	if l.Instrumental {
		lyricsMessage(c, "♪  Instrumental  ♪")
		return
	}

	synced := l.IsSynced()
	cur := l.Current(a.player.State().Position)
	lines := l.Lines()
	s := &lp.scroll

	// Let the user scroll: a change that is not ours holds the view for a while.
	if lp.haveSet {
		expect := min(lp.lastSetY, s.MaxY)
		if math.Abs(float64(s.Y-expect)) > 1 {
			lp.manualUntil = time.Now().Add(lyricsManualHold)
			lp.recenter = true
			lp.lastSetY = s.Y
		}
	}
	if synced && cur >= 0 && time.Now().After(lp.manualUntil) && (cur != lp.lastCur || lp.recenter) {
		// The spacers above and below let the first and last lines reach the middle.
		target := float32(cur * lyricsRowH)
		s.Y = target
		lp.lastSetY, lp.haveSet, lp.recenter = target, true, false
	}
	lp.lastCur = cur

	pad := float32(lyricsViewH-lyricsRowH) / 2
	ui.Scroll(c.Key("lyrics-scroll")).Fill().TrackScroll(s).Children(func() {
		ui.Box(c).Height(pad)
		for i, text := range lines {
			color := sonoraTextMuted
			weight := 500
			switch {
			case !synced:
				color = sonoraText
			case i == cur:
				color, weight = sonoraText, 700
			case i < cur:
				color = sonoraTextTertiary
			}
			label := text
			if label == "" && synced {
				label = "♪"
			}
			row := ui.Row(c).Height(lyricsRowH).AlignItems(ui.Center).Padding(0, 10).Radius(8)
			if synced && row.Hovered() {
				row.Background(adaptiveOverlay(c, 0.06, 0.05))
			}
			row.Children(func() {
				ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
					TextAlign(ui.Center).FontSize(fs(19)).FontWeight(weight).TextColor(color)
			})
			if synced && row.Clicked() {
				if err := a.player.SeekTo(min(l.Synced[i].At, max(song.Duration-time.Second, 0))); err != nil {
					a.status = "No se pudo mover la canción: " + err.Error()
				} else {
					lp.recenter, lp.manualUntil = true, time.Time{}
					a.updateNowPlaying()
				}
			}
		}
		ui.Box(c).Height(pad)
	})
}
