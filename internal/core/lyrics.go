package core

import (
	"context"
	"errors"
	"time"

	"musaik/internal/lyrics"
	"musaik/internal/model"
)

// LyricsState is where the lyrics of a song are.
type LyricsState int

const (
	LyricsLoading LyricsState = iota // being looked up
	LyricsReady                      // found: see Lyrics
	LyricsNone                       // nobody has them
	LyricsFailed                     // the lookup failed (network): see Err
)

// LyricsEntry is the answer LyricsFor gives.
type LyricsEntry struct {
	State  LyricsState
	Lyrics *lyrics.Lyrics
	Err    string
	// Saving is true while the lyrics are being written into the audio file.
	Saving bool
	at     time.Time
}

const (
	lyricsKeep       = 30
	lyricsRetryAfter = 20 * time.Second
)

// LyricsFor returns the lyrics of song, and starts looking them up the first
// time it is asked: the state is LyricsLoading until the lookup posts its
// answer back (the UI is redrawn by whatever Post sends). A failed lookup is
// tried again after a few seconds, when asked again. Nothing leaves the
// machine until this is called, which the UI does when the user opens the
// lyrics.
func (e *Engine) LyricsFor(song model.Song) LyricsEntry {
	if e.lyricsCache == nil {
		e.lyricsCache = map[string]*LyricsEntry{}
	}
	if ent, ok := e.lyricsCache[song.ID]; ok {
		if ent.State != LyricsFailed || time.Since(ent.at) < lyricsRetryAfter {
			return *ent
		}
	}
	ent := &LyricsEntry{State: LyricsLoading, at: time.Now()}
	if len(e.lyricsCache) >= lyricsKeep {
		e.lyricsCache = map[string]*LyricsEntry{}
	}
	e.lyricsCache[song.ID] = ent
	client := e.Lyrics
	if client == nil {
		client = lyrics.NewClient("Musaik")
	}
	q := lyricsQuery(song)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		l, err := client.Fetch(ctx, q)
		e.post(func() {
			done := &LyricsEntry{at: time.Now()}
			switch {
			case err == nil:
				done.State, done.Lyrics = LyricsReady, l
			case errors.Is(err, lyrics.ErrNotFound):
				done.State = LyricsNone
			default:
				done.State, done.Err = LyricsFailed, err.Error()
			}
			// Only if the song is still wanted: the cache may have been cleared.
			if e.lyricsCache[song.ID] == ent {
				e.lyricsCache[song.ID] = done
			}
		})
	}()
	return *ent
}

// ForgetLyrics drops what is known about song's lyrics, so the next
// LyricsFor looks again.
func (e *Engine) ForgetLyrics(songID string) { delete(e.lyricsCache, songID) }

func lyricsQuery(song model.Song) lyrics.Query {
	return lyrics.Query{Title: song.Title, Artist: song.Artist, Album: song.Album, Duration: song.Duration, Path: song.Path}
}

// SaveLyricsToTag writes the lyrics shown for song into the lyrics tag of its
// file (with times, as LRC, when it has them), so they are found there from
// now on and no longer depend on the cache or the network. Only lyrics that
// came from the network can be saved. The file is modified, so the UIs call
// this on the user's request only; the result arrives in Status.
func (e *Engine) SaveLyricsToTag(song model.Song) {
	ent := e.lyricsCache[song.ID]
	if ent == nil || ent.State != LyricsReady || !ent.Lyrics.CanSave() {
		e.Status = "No hay una letra descargada que guardar"
		return
	}
	if ent.Saving {
		return
	}
	ent.Saving = true
	l, q := ent.Lyrics, lyricsQuery(song)
	client := e.Lyrics
	go func() {
		err := lyrics.SaveToTag(song.Path, l)
		e.post(func() {
			ent.Saving = false
			if err != nil {
				e.Status = "No se pudo guardar la letra: " + err.Error()
				return
			}
			if client != nil {
				client.Forget(q)
			} else {
				lyrics.NewClient("Musaik").Forget(q)
			}
			saved := *l
			saved.Source = "tag"
			ent.Lyrics = &saved
			e.Status = "Letra guardada en las etiquetas de «" + song.Title + "»"
		})
	}()
}
