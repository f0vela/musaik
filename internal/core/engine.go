package core

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"musaik/internal/lastfm"
	"musaik/internal/library"
	"musaik/internal/model"
	"musaik/internal/player"
)

// RepeatMode is what happens when the queue ends.
type RepeatMode string

const (
	RepeatOff RepeatMode = "off"
	RepeatAll RepeatMode = "all"
	RepeatOne RepeatMode = "one"
)

// Player is what the engine needs from the audio backend; *player.Player
// satisfies it and tests use a fake.
type Player interface {
	Play(path string, knownDuration time.Duration) error
	TogglePause() error
	Stop()
	State() player.State
	SeekTo(pos time.Duration) error
	Volume() int
	SetVolume(volume int) error
	Events() <-chan player.Event
}

const (
	historyLimit   = 200
	scrobbleAfter  = 15 * time.Second
	saveStateDelay = 2 * time.Second
)

// Engine owns the library, the player and the playback state. It is not safe
// for concurrent use: call it from one goroutine and use Post to get back onto
// that goroutine from background work.
type Engine struct {
	Store    *library.Store
	Player   Player
	LastFM   *lastfm.Client
	Settings *Settings

	// Queue holds the IDs of the songs that play next, before the library
	// order continues.
	Queue      []string
	PlayIndex  int
	PlayingID  string
	SelectedID string
	History    []string
	Shuffle    bool
	Repeat     RepeatMode
	// Volume is 0-100; VolumeBeforeMute is what unmuting restores.
	Volume           int
	VolumeBeforeMute int
	// Status is the last message for the user, as the desktop app's status line.
	Status string

	// IPodDetected, IPodName and IPodBusy describe the mounted iPod and a
	// running sync; LastFMAuthBusy a running Last.fm authorization; Indexing a
	// running library scan.
	IPodDetected   bool
	IPodName       string
	IPodBusy       bool
	LastFMAuthBusy bool
	Indexing       bool

	// Post runs fn on the goroutine that owns the engine. Background work
	// reports back through it. Nil runs fn at once (tests).
	Post func(fn func())

	goingBack  bool
	songIdx    map[string]int
	sizeCache  map[string]int64
	dirty      bool
	dirtySince time.Time
	rng        *rand.Rand
}

// New builds an engine on p with the saved library and settings loaded.
func New(p Player) *Engine {
	e := &Engine{
		Store:     library.NewStore(),
		Player:    p,
		PlayIndex: -1,
		Repeat:    RepeatOff,
		Volume:    80,
		sizeCache: map[string]int64{},
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	_ = e.Store.LoadState()
	e.Settings, _ = LoadSettings()
	if e.Settings == nil {
		e.Settings = &Settings{}
	}
	s := e.Settings
	if s.LastFMAPIKey == "" {
		s.LastFMAPIKey = os.Getenv("LASTFM_API_KEY")
	}
	if s.LastFMAPISecret == "" {
		s.LastFMAPISecret = os.Getenv("LASTFM_API_SECRET")
	}
	if s.LastFMSessionKey == "" {
		s.LastFMSessionKey = os.Getenv("LASTFM_SESSION_KEY")
	}
	if s.LastFMUsername == "" {
		s.LastFMUsername = os.Getenv("LASTFM_USERNAME")
	}
	if s.RootInput == "" {
		s.RootInput = e.Store.Root
	}
	if s.RootInput == "" {
		s.RootInput = DefaultMusicDir()
	}
	if s.IPodPlaylistName == "" {
		s.IPodPlaylistName = "Todos"
	}
	e.Shuffle = s.Shuffle
	switch RepeatMode(s.RepeatMode) {
	case RepeatAll, RepeatOne:
		e.Repeat = RepeatMode(s.RepeatMode)
	}
	if p != nil {
		e.Volume = p.Volume()
	}
	if e.Volume > 0 {
		e.VolumeBeforeMute = e.Volume
	}
	if len(e.Store.Songs) > 0 {
		e.Status = fmt.Sprintf("Biblioteca cargada: %d canciones", len(e.Store.Songs))
	} else {
		e.Status = "Listo para indexar música"
	}
	e.RefreshLastFM()
	return e
}

// DefaultMusicDir is ~/Music.
func DefaultMusicDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home + string(os.PathSeparator) + "Music"
}

// RefreshLastFM rebuilds the Last.fm client from the current credentials.
func (e *Engine) RefreshLastFM() {
	s := e.Settings
	e.LastFM = lastfm.New(s.LastFMAPIKey, s.LastFMAPISecret, s.LastFMSessionKey, s.LastFMUsername)
}

func (e *Engine) post(fn func()) {
	if e.Post != nil {
		e.Post(fn)
		return
	}
	fn()
}

// notify sets the status from a background goroutine.
func (e *Engine) notify(msg string) {
	e.post(func() { e.Status = msg })
}

// ---- persistence ------------------------------------------------------------

// SyncSettings copies the playback preferences into Settings.
func (e *Engine) SyncSettings() {
	e.Settings.Shuffle = e.Shuffle
	e.Settings.RepeatMode = string(e.Repeat)
}

// SaveSettings writes the preferences shared with the desktop app.
func (e *Engine) SaveSettings() error {
	e.SyncSettings()
	return e.Settings.Save()
}

// Persist writes the library state and the settings at once.
func (e *Engine) Persist() error {
	e.dirty = false
	err := e.Store.SaveState()
	if serr := e.SaveSettings(); err == nil {
		err = serr
	}
	return err
}

// MarkDirty notes that the library state changed; FlushIfDirty writes it
// shortly after, so a burst of plays or likes costs one write.
func (e *Engine) MarkDirty() {
	if !e.dirty {
		e.dirty = true
		e.dirtySince = time.Now()
	}
}

// FlushIfDirty writes the library state if it changed and has waited long enough.
func (e *Engine) FlushIfDirty(force bool) {
	if !e.dirty {
		return
	}
	if !force && time.Since(e.dirtySince) < saveStateDelay {
		return
	}
	e.dirty = false
	if err := e.Store.SaveState(); err != nil {
		e.Status = "Error guardando estado: " + err.Error()
	}
}

// ---- songs ------------------------------------------------------------------

// FindSong looks a song up by ID and returns its index in the library. It goes
// through an index of the IDs, rebuilt when the library changes; a hit is checked
// against the song itself and a miss falls back to a scan, so a stale index can
// slow a lookup down but never make it wrong.
func (e *Engine) FindSong(id string) (int, model.Song, bool) {
	songs := e.Store.Songs
	if len(e.songIdx) != len(songs) {
		e.songIdx = make(map[string]int, len(songs))
		for i, s := range songs {
			e.songIdx[s.ID] = i
		}
	}
	if i, ok := e.songIdx[id]; ok && i < len(songs) && songs[i].ID == id {
		return i, songs[i], true
	}
	for i, s := range songs {
		if s.ID == id {
			return i, s, true
		}
	}
	return -1, model.Song{}, false
}

// CurrentSong is the song the player has loaded, if it is in the library.
func (e *Engine) CurrentSong() (model.Song, bool) {
	path := strings.TrimSpace(e.Player.State().Path)
	if path == "" {
		return model.Song{}, false
	}
	if e.PlayingID != "" {
		if _, s, ok := e.FindSong(e.PlayingID); ok && s.Path == path {
			return s, true
		}
	}
	for _, s := range e.Store.Songs {
		if s.Path == path {
			return s, true
		}
	}
	return model.Song{}, false
}

// ---- playback ---------------------------------------------------------------

// PlaySongByID plays a library song.
func (e *Engine) PlaySongByID(id string) {
	index, song, ok := e.FindSong(id)
	if !ok {
		e.Status = "Canción no encontrada"
		return
	}
	e.PlaySongAt(index, song)
}

// PlaySongAt plays song, which sits at index in the library.
func (e *Engine) PlaySongAt(index int, song model.Song) {
	if err := e.Player.Play(song.Path, song.Duration); err != nil {
		e.Status = "Error reproduciendo: " + err.Error()
		e.goingBack = false
		return
	}
	if !e.goingBack && e.PlayingID != "" && e.PlayingID != song.ID {
		e.History = append(e.History, e.PlayingID)
		if len(e.History) > historyLimit {
			e.History = e.History[1:]
		}
	}
	e.goingBack = false
	e.PlayingID = song.ID
	e.PlayIndex = index
	e.SelectedID = song.ID
	now := time.Now()
	if count, ok := e.Store.IncrementPlayCount(song.ID); ok {
		_ = e.Store.MarkPlayedAt(song.ID, now)
		e.MarkDirty()
		song.PlayCount = count
		song.LastPlayedAt = now
	}
	e.Status = fmt.Sprintf("Reproduciendo: %s — %s", song.Artist, song.Title)

	if e.LastFM != nil && e.LastFM.Enabled() {
		client := e.LastFM
		go func() {
			if err := client.UpdateNowPlaying(song); err != nil {
				e.notify("Reproduciendo (Last.fm falló): " + err.Error())
			}
		}()
		startedAt := now
		go func() {
			time.Sleep(scrobbleAfter)
			state := e.Player.State()
			if !state.Playing || state.Path != song.Path {
				return
			}
			_ = client.Scrobble(song, startedAt)
		}()
	}
}

// TogglePlayPause pauses or resumes, or starts the selected song when idle.
func (e *Engine) TogglePlayPause() {
	if e.Player.State().Playing {
		if err := e.Player.TogglePause(); err != nil {
			e.Status = "No se puede pausar/reanudar: " + err.Error()
		}
		return
	}
	e.PlaySelectedOrFirst()
}

// Pause pauses playback if it is running.
func (e *Engine) Pause() {
	if st := e.Player.State(); st.Playing && !st.Paused {
		e.TogglePlayPause()
	}
}

// Resume resumes a paused song, or starts one when nothing is loaded.
func (e *Engine) Resume() {
	st := e.Player.State()
	switch {
	case !st.Playing:
		e.PlaySelectedOrFirst()
	case st.Paused:
		e.TogglePlayPause()
	}
}

// Stop stops playback.
func (e *Engine) Stop() {
	e.Player.Stop()
	e.Status = "Reproducción detenida"
}

// PlaySelectedOrFirst plays the selected song, or the first one of the library.
func (e *Engine) PlaySelectedOrFirst() {
	if e.SelectedID != "" {
		e.PlaySongByID(e.SelectedID)
		return
	}
	if len(e.Store.Songs) > 0 {
		e.PlaySongAt(0, e.Store.Songs[0])
	}
}

// CycleRepeat goes off -> all -> one -> off.
func (e *Engine) CycleRepeat() {
	switch e.Repeat {
	case RepeatOff:
		e.Repeat = RepeatAll
	case RepeatAll:
		e.Repeat = RepeatOne
	default:
		e.Repeat = RepeatOff
	}
	_ = e.SaveSettings()
}

// ToggleShuffle turns shuffle on or off.
func (e *Engine) ToggleShuffle() {
	e.Shuffle = !e.Shuffle
	_ = e.SaveSettings()
}

func (e *Engine) randomIndex() int {
	return e.rng.Intn(len(e.Store.Songs))
}

// Next plays the next queued song, or the next one in the library order.
func (e *Engine) Next() {
	if len(e.Store.Songs) == 0 {
		return
	}
	if queued, ok := e.DequeueNext(); ok {
		if idx, _, found := e.FindSong(queued.ID); found {
			e.PlaySongAt(idx, queued)
			return
		}
	}
	switch {
	case e.Shuffle:
		e.PlayIndex = e.randomIndex()
	case e.PlayIndex >= len(e.Store.Songs)-1:
		if e.Repeat != RepeatAll {
			e.Status = "Fin de la cola"
			return
		}
		e.PlayIndex = 0
	default:
		e.PlayIndex++
	}
	e.PlaySongAt(e.PlayIndex, e.Store.Songs[e.PlayIndex])
}

// Previous goes back through the play history, then through the library order.
func (e *Engine) Previous() {
	if len(e.Store.Songs) == 0 {
		return
	}
	for len(e.History) > 0 {
		id := e.History[len(e.History)-1]
		e.History = e.History[:len(e.History)-1]
		if idx, song, ok := e.FindSong(id); ok {
			if e.PlayingID != "" {
				e.Queue = append([]string{e.PlayingID}, e.Queue...)
			}
			e.goingBack = true
			e.PlaySongAt(idx, song)
			return
		}
	}
	if e.PlayIndex <= 0 {
		if e.Repeat != RepeatAll {
			e.PlayIndex = 0
		} else {
			e.PlayIndex = len(e.Store.Songs) - 1
		}
	} else {
		e.PlayIndex--
	}
	e.PlaySongAt(e.PlayIndex, e.Store.Songs[e.PlayIndex])
}

// HandleEvent reacts to one thing the player reported: when a track ended, it
// moves on to the next. A caller that waits on Player.Events itself uses this to
// react at once instead of polling ConsumePlayerEvents.
func (e *Engine) HandleEvent(ev player.Event) {
	if ev.Type != player.EventTrackEnded {
		return
	}
	if ev.Err != nil && !strings.Contains(ev.Err.Error(), "signal: killed") {
		e.Status = "Error de reproducción: " + ev.Err.Error()
		return
	}
	e.autoAdvance()
}

// ConsumePlayerEvents handles what the player reported since the last call,
// moving on when a track ended.
func (e *Engine) ConsumePlayerEvents() {
	for {
		select {
		case ev := <-e.Player.Events():
			if ev.Type == player.EventTrackEnded {
				e.HandleEvent(ev)
				return
			}
		default:
			return
		}
	}
}

func (e *Engine) autoAdvance() {
	if len(e.Store.Songs) == 0 || e.PlayIndex < 0 {
		e.Status = "Reproducción finalizada"
		return
	}
	if queued, ok := e.DequeueNext(); ok {
		if idx, _, found := e.FindSong(queued.ID); found {
			e.PlaySongAt(idx, queued)
			return
		}
	}
	if e.PlayIndex >= len(e.Store.Songs) {
		e.PlayIndex = len(e.Store.Songs) - 1
	}
	switch {
	case e.Repeat == RepeatOne:
	case e.Shuffle:
		e.PlayIndex = e.randomIndex()
	default:
		next := e.PlayIndex + 1
		if next >= len(e.Store.Songs) {
			if e.Repeat != RepeatAll {
				e.Status = "Fin de la cola"
				return
			}
			next = 0
		}
		e.PlayIndex = next
	}
	e.PlaySongAt(e.PlayIndex, e.Store.Songs[e.PlayIndex])
}

// SeekBy moves the playback position by delta, clamped to the song.
func (e *Engine) SeekBy(delta time.Duration) {
	st := e.Player.State()
	if !st.Playing {
		return
	}
	e.SeekTo(st.Position + delta)
}

// SeekTo moves the playback position to pos, clamped to the song.
func (e *Engine) SeekTo(pos time.Duration) {
	st := e.Player.State()
	if !st.Playing {
		return
	}
	if pos < 0 {
		pos = 0
	}
	if st.Duration > 0 && pos > st.Duration-time.Second {
		pos = max(st.Duration-time.Second, 0)
	}
	if err := e.Player.SeekTo(pos); err != nil {
		e.Status = "No se pudo mover la canción: " + err.Error()
	}
}

// SetVolume sets the volume, 0-100.
func (e *Engine) SetVolume(level int) {
	level = min(max(level, 0), 100)
	e.Volume = level
	if level > 0 {
		e.VolumeBeforeMute = level
	}
	if err := e.Player.SetVolume(level); err != nil {
		e.Status = "No se pudo ajustar volumen: " + err.Error()
	}
}

// ToggleMute mutes, or restores the volume from before the mute.
func (e *Engine) ToggleMute() {
	if e.Volume <= 0 {
		restore := e.VolumeBeforeMute
		if restore <= 0 {
			restore = 65
		}
		e.SetVolume(restore)
		return
	}
	e.VolumeBeforeMute = e.Volume
	e.SetVolume(0)
}

// ---- queue ------------------------------------------------------------------

// Enqueue adds a song to the end of the queue.
func (e *Engine) Enqueue(id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	if _, _, ok := e.FindSong(id); !ok {
		return false
	}
	e.Queue = append(e.Queue, id)
	return true
}

// DequeueNext pops the first queued song that still exists.
func (e *Engine) DequeueNext() (model.Song, bool) {
	for len(e.Queue) > 0 {
		id := e.Queue[0]
		e.Queue = e.Queue[1:]
		if _, song, ok := e.FindSong(id); ok {
			return song, true
		}
	}
	return model.Song{}, false
}

// QueuedSongs are the songs waiting in the queue.
func (e *Engine) QueuedSongs() []model.Song {
	out := make([]model.Song, 0, len(e.Queue))
	for _, id := range e.Queue {
		if _, song, ok := e.FindSong(id); ok {
			out = append(out, song)
		}
	}
	return out
}

// ClearQueue empties the queue.
func (e *Engine) ClearQueue() {
	e.Queue = nil
	e.Status = "Queue limpiado"
}

// RemoveQueued drops the i-th queued entry.
func (e *Engine) RemoveQueued(i int) {
	if i >= 0 && i < len(e.Queue) {
		e.Queue = append(e.Queue[:i], e.Queue[i+1:]...)
	}
}

// PlayFromQueue plays the i-th queued song and takes it off the queue.
func (e *Engine) PlayFromQueue(i int) {
	if i < 0 || i >= len(e.Queue) {
		return
	}
	id := e.Queue[i]
	e.PlaySongByID(id)
	e.RemoveQueued(i)
}

// NaturalUpcoming are the songs that follow the current one in library order.
func (e *Engine) NaturalUpcoming(limit int) []model.Song {
	n := len(e.Store.Songs)
	if limit <= 0 || n == 0 || e.PlayIndex < 0 || e.PlayIndex >= n || e.Shuffle {
		return nil
	}
	out := make([]model.Song, 0, limit)
	for i := e.PlayIndex + 1; i < n && len(out) < limit; i++ {
		out = append(out, e.Store.Songs[i])
	}
	if e.Repeat == RepeatAll {
		for i := 0; i <= e.PlayIndex && len(out) < limit; i++ {
			out = append(out, e.Store.Songs[i])
		}
	}
	return out
}

// EnsureRandomQueue tops the queue up to minCount random songs.
func (e *Engine) EnsureRandomQueue(minCount int) {
	if minCount <= 0 || len(e.Queue) >= minCount || len(e.Store.Songs) == 0 {
		return
	}
	exists := make(map[string]bool, len(e.Queue)+1)
	for _, id := range e.Queue {
		exists[id] = true
	}
	if song, ok := e.CurrentSong(); ok {
		exists[song.ID] = true
	}
	candidates := make([]string, 0, len(e.Store.Songs))
	for _, s := range e.Store.Songs {
		if !exists[s.ID] {
			candidates = append(candidates, s.ID)
		}
	}
	if len(candidates) == 0 {
		return
	}
	e.rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	need := min(minCount-len(e.Queue), len(candidates))
	e.Queue = append(e.Queue, candidates[:need]...)
}

// ---- favorites and Last.fm ---------------------------------------------------

// ToggleFavorite likes or unlikes a song; a new like is also sent to Last.fm.
func (e *Engine) ToggleFavorite(song model.Song) {
	was := e.Store.Favorites[song.ID]
	e.Store.ToggleFavorite(song.ID)
	e.MarkDirty()
	if was {
		e.Status = fmt.Sprintf("Quitaste like: %s", song.Title)
		return
	}
	e.Status = fmt.Sprintf("Like guardado: %s", song.Title)
	e.loveOnLastFM(song)
}

// LikeSong likes a song (it never unlikes) and sends the like to Last.fm.
func (e *Engine) LikeSong(song model.Song) {
	if e.Store.Favorites[song.ID] {
		e.Status = fmt.Sprintf("La canción ya tiene like: %s", song.Title)
	} else {
		e.Store.ToggleFavorite(song.ID)
		e.MarkDirty()
		e.Status = fmt.Sprintf("Like guardado: %s", song.Title)
	}
	e.loveOnLastFM(song)
}

func (e *Engine) loveOnLastFM(song model.Song) {
	if e.LastFM == nil || !e.LastFM.Enabled() {
		return
	}
	client := e.LastFM
	go func() {
		if err := client.LoveTrack(song); err != nil {
			e.notify("Like local guardado, pero falló Last.fm: " + err.Error())
			return
		}
		e.notify(fmt.Sprintf("Like enviado a Last.fm: %s", song.Title))
	}()
}

// SyncFromLastFM reads the song's play count (and, with syncLoved, whether it
// is loved) from Last.fm and applies it to the library.
func (e *Engine) SyncFromLastFM(song model.Song, syncLoved bool) {
	if e.LastFM == nil || !e.LastFM.Enabled() {
		e.Status = "Last.fm no está conectado"
		return
	}
	e.Status = "Consultando metadata en Last.fm…"
	client := e.LastFM
	go func() {
		count, loved, err := client.UserTrackInfo(song)
		e.post(func() {
			if err != nil {
				e.Status = "No se pudo leer datos de Last.fm: " + err.Error()
				return
			}
			e.applyLastFMInfo(song, count, loved, syncLoved)
		})
	}()
}

func (e *Engine) applyLastFMInfo(song model.Song, count int, loved, syncLoved bool) {
	if !e.Store.SetPlayCount(song.ID, count) {
		e.Status = "Canción no encontrada para actualizar datos"
		return
	}
	if syncLoved {
		if loved {
			e.Store.Favorites[song.ID] = true
		} else {
			delete(e.Store.Favorites, song.ID)
		}
	}
	e.MarkDirty()
	if !syncLoved {
		e.Status = fmt.Sprintf("Conteo actualizado desde Last.fm: %s (%d)", song.Title, count)
		return
	}
	state := "not liked"
	if loved {
		state = "liked"
	}
	e.Status = fmt.Sprintf("Last.fm sync: %s (%d plays, %s)", song.Title, count, state)
}

// ---- playlists --------------------------------------------------------------

// PlaylistColors are the colors a manual playlist can have.
func PlaylistColors() []string {
	return []string{"none", "purple", "yellow", "red", "orange", "green", "blue", "gray"}
}

// NormalizePlaylistColor maps anything unknown to "none".
func NormalizePlaylistColor(color string) string {
	color = strings.TrimSpace(strings.ToLower(color))
	for _, c := range PlaylistColors() {
		if color == c {
			return color
		}
	}
	return "none"
}

// CreatePlaylist makes a manual playlist, optionally seeded with a song.
func (e *Engine) CreatePlaylist(name, seedSongID, color string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return fmt.Errorf("define un nombre para la playlist")
	}
	for _, pl := range e.Store.Playlists {
		if strings.EqualFold(strings.TrimSpace(pl.Name), n) {
			return fmt.Errorf("ya existe una playlist con ese nombre")
		}
	}
	seed := []string{}
	if strings.TrimSpace(seedSongID) != "" {
		seed = append(seed, seedSongID)
	}
	e.Store.AddManualPlaylist(n, seed)
	if err := e.Store.SetManualPlaylistColor(n, NormalizePlaylistColor(color)); err != nil {
		return err
	}
	return e.Store.SaveState()
}

// SetPlaylistColor recolors a manual playlist.
func (e *Engine) SetPlaylistColor(name, color string) error {
	if err := e.Store.SetManualPlaylistColor(name, NormalizePlaylistColor(color)); err != nil {
		return err
	}
	return e.Store.SaveState()
}

// RenamePlaylist renames a manual playlist.
func (e *Engine) RenamePlaylist(oldName, newName string) error {
	if err := e.Store.RenameManualPlaylist(oldName, newName); err != nil {
		return err
	}
	return e.Store.SaveState()
}

// DeletePlaylist removes a manual playlist.
func (e *Engine) DeletePlaylist(name string) error {
	if err := e.Store.DeleteManualPlaylist(name); err != nil {
		return err
	}
	return e.Store.SaveState()
}

// AddToPlaylist adds a song to a manual playlist.
func (e *Engine) AddToPlaylist(name, songID string) error {
	if err := e.Store.AddSongToManualPlaylist(name, songID); err != nil {
		return err
	}
	return e.Store.SaveState()
}
