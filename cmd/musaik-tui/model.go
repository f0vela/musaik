package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
	"musaik/internal/model"
	"musaik/internal/player"
)

// version is shown in Settings > About; it follows the desktop app's mygo.json,
// and a build can set it: -ldflags "-X main.version=1.2.3".
var version = "0.4.7"

type sectionID int

const (
	secSongs sectionID = iota
	secArtists
	secAlbums
	secQueue
	secFavorites
	secRecent
	secPlaylist
	secSettings
)

// sectionNames are the names the desktop app saves in settings.json.
var sectionNames = map[sectionID]string{
	secSongs:     "Canciones",
	secArtists:   "Artistas",
	secAlbums:    "Álbumes",
	secQueue:     "Queue",
	secFavorites: "Favoritos",
	secRecent:    "Recientes",
	secPlaylist:  "Playlists",
	secSettings:  "Ajustes",
}

func sectionFromName(name string) (sectionID, bool) {
	for id, n := range sectionNames {
		if n == name {
			return id, true
		}
	}
	return secSongs, false
}

// item is one row of a song list: a song, or a heading that groups songs.
type item struct {
	song    model.Song
	heading string
	now     bool
}

func (i item) isSong() bool { return i.heading == "" }

// listState is a cursor and the first visible row of a scrolling list.
type listState struct{ cur, off int }

// fix keeps the cursor inside n rows and the window of h rows around it.
func (l *listState) fix(n, h int) {
	if n <= 0 {
		l.cur, l.off = 0, 0
		return
	}
	l.cur = min(max(l.cur, 0), n-1)
	if h <= 0 {
		return
	}
	if l.cur < l.off {
		l.off = l.cur
	}
	if l.cur >= l.off+h {
		l.off = l.cur - h + 1
	}
	l.off = min(max(l.off, 0), max(n-h, 0))
}

// move shifts the cursor by d rows, stepping over rows skip says to avoid.
func (l *listState) move(d, n int, skip func(int) bool) {
	if n <= 0 {
		return
	}
	step := 1
	if d < 0 {
		step = -1
	}
	for ; d != 0; d -= step {
		i := l.cur + step
		for i >= 0 && i < n && skip != nil && skip(i) {
			i += step
		}
		if i < 0 || i >= n {
			break
		}
		l.cur = i
	}
}

// ---- messages -------------------------------------------------------------------

type tickMsg time.Time

// playerEventMsg is something the player reported, delivered the moment it
// happens instead of found by polling.
type playerEventMsg player.Event

// funcMsg runs a function on the UI goroutine: background work reports back
// through the engine's Post.
type funcMsg func()

type artMsg struct {
	key string
	art core.Artwork
}

type avatarMsg struct {
	artist string
	img    image.Image
}

type artEntry struct {
	img    image.Image
	colors []color.RGBA
}

// ---- model ----------------------------------------------------------------------

type modalKind int

const (
	modalNone modalKind = iota
	modalNewPlaylist
	modalRenamePlaylist
	modalDeletePlaylist
	modalAddToPlaylist
	modalTheme
)

type modalState struct {
	kind    modalKind
	input   field
	color   int
	seed    string
	choices []string
	idx     int
	target  string
	err     string
}

type actionHit struct {
	Kind       string
	ActionID   string
	SongID     string
	ArtistName string
	AlbumID    string
	Title      string
	Subtitle   string
	// Keywords are extra words the search matches, such as other names for the action.
	Keywords string
}

type paletteState struct {
	open  bool
	input field
	hits  []actionHit
	idx   int
}

type tui struct {
	eng *core.Engine
	th  theme
	w   int
	h   int

	sec       sectionID
	prevSec   sectionID
	playlist  string
	focusSide bool
	sideCur   int
	mini      bool
	help      bool

	search    field
	searching bool

	rows      []model.Song
	rowsDirty bool
	list      listState

	artists    []string
	artistList listState
	artistOpen bool
	artist     string
	artistRows []item
	artistDet  listState

	albums    []model.Song
	albumList listState
	albumOpen bool
	albumID   string
	albumRows []item
	albumDet  listState
	albumSort string

	queueRows []item
	queueList listState

	set     settingsState
	lyr     lyricsState
	pal     paletteState
	modal   modalState
	remote  *core.Remote
	quiting bool

	art        map[string]*artEntry
	artLoading map[string]bool
	artOrder   []string
	avatars    map[string]image.Image
	avatarTry  map[string]bool
	coverCache map[string][]string

	lastPlaying string
	ticks       int
	zones       []zone
	vis         int
	stats       libStats

	proto imgProto
	artPx int // longest side, in pixels, that covers are decoded at
	kitty kittyState

	themeID  string
	termDark bool // whether the terminal's own background is dark, for the auto theme

	sideCache []sideEntry // nil: rebuild
	sideSel   []sideEntry
	statsKey  string // what stats were computed for

	// A frame is only rebuilt when something visible changed: a message other
	// than the clock tick always counts, a tick only if it moved the clock, the
	// song or the status. cacheViews turns this on (the app does; tests render
	// every time, so they can poke at the state directly).
	cacheViews bool
	dirty      bool
	view       tea.View
	viewOK     bool
	renders    int
	lastPos    int
	lastPlay   int
	lastIPod   time.Time
}

const (
	artKeep     = 12
	tickPlaying = 500 * time.Millisecond
	tickIdle    = time.Second
	ipodEvery   = 8 * time.Second
	rowsPerPg   = 10
	recentDays  = 15
)

func newTUI(eng *core.Engine, mini bool) *tui {
	t := &tui{
		eng:        eng,
		th:         newTheme(true),
		themeID:    loadPrefs().Theme,
		termDark:   true,
		w:          100,
		h:          30,
		sec:        secSongs,
		mini:       mini,
		albumSort:  "Artista",
		art:        map[string]*artEntry{},
		artLoading: map[string]bool{},
		avatars:    map[string]image.Image{},
		avatarTry:  map[string]bool{},
		coverCache: map[string][]string{},
		rowsDirty:  true,
		dirty:      true,
		artPx:      artPxBlocks,
		kitty:      kittyState{next: kittyFirst, imgs: map[string]*kittyImg{}},
	}
	if id, ok := sectionFromName(eng.Settings.Section); ok && id != secPlaylist && id != secSettings {
		t.sec = id
	}
	if strings.TrimSpace(eng.Settings.SearchQuery) != "" {
		t.search.Set(eng.Settings.SearchQuery)
	}
	t.applyTheme()
	t.initSettings()
	for _, pl := range eng.VisiblePlaylists() {
		t.playlist = pl.Name
		break
	}
	t.syncSideCursor()
	return t
}

func (t *tui) Init() tea.Cmd {
	t.refreshStats()
	return tea.Batch(tea.RequestBackgroundColor, tickCmd(false), t.artCmds())
}

// tickCmd schedules the next clock tick. The clock only moves while a song plays
// (the bar and the time change once a second), so an idle app wakes up rarely.
func tickCmd(playing bool) tea.Cmd {
	every := tickIdle
	if playing {
		every = tickPlaying
	}
	return tea.Tick(every, func(at time.Time) tea.Msg { return tickMsg(at) })
}

func (t *tui) isPlaying() bool {
	st := t.eng.Player.State()
	return st.Playing && !st.Paused
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if _, isTick := msg.(tickMsg); !isTick {
		t.dirty = true
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		t.termDark = msg.IsDark()
		t.applyTheme()
	case tickMsg:
		if t.onTick() {
			t.dirty = true
		}
		cmds = append(cmds, tickCmd(t.isPlaying()))
	case playerEventMsg:
		t.eng.HandleEvent(player.Event(msg))
		t.rowsDirty = true
	case funcMsg:
		msg()
		t.rowsDirty = true
	case artMsg:
		t.storeArt(msg)
	case kittyReadyMsg:
		cmds = append(cmds, t.onKittyReady(msg))
	case kittySentMsg:
		t.onKittySent(msg)
	case avatarMsg:
		if msg.img != nil {
			t.avatars[core.ArtistAvatarKey(msg.artist)] = msg.img
			t.dropImages("avatar:" + core.ArtistAvatarKey(msg.artist))
		}
	case tea.KeyPressMsg:
		if cmd := t.onKey(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.PasteMsg:
		t.onPaste(msg.Content)
	case tea.MouseClickMsg:
		if cmd := t.onClick(msg.Mouse()); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.MouseWheelMsg:
		t.onWheel(msg.Mouse())
	}
	t.refreshIfDirty()
	cmds = append(cmds, t.artCmds(), t.flushKitty())
	if t.quiting {
		return t, tea.Quit
	}
	return t, tea.Batch(cmds...)
}

// onTick does what has to be done now and then, and reports whether anything
// the screen shows has changed.
func (t *tui) onTick() (changed bool) {
	e := t.eng
	status, playing, detected, name := e.Status, e.PlayingID, e.IPodDetected, e.IPodName
	e.FlushIfDirty(false)
	if time.Since(t.lastIPod) >= ipodEvery {
		t.lastIPod = time.Now()
		e.DetectIPod(false)
	}
	if e.PlayingID != t.lastPlaying {
		t.lastPlaying = e.PlayingID
		t.rowsDirty = true
	}
	st := e.Player.State()
	state := 0
	if st.Playing {
		state = 1
		if st.Paused {
			state = 2
		}
	}
	pos := int(st.Position / time.Second)
	changed = pos != t.lastPos || state != t.lastPlay ||
		e.Status != status || e.PlayingID != playing || e.IPodDetected != detected || e.IPodName != name
	t.lastPos, t.lastPlay = pos, state
	return changed
}

// ---- data -------------------------------------------------------------------------

func (t *tui) query() string { return t.search.Value() }

func (t *tui) refreshIfDirty() {
	if !t.rowsDirty {
		return
	}
	t.rowsDirty = false
	t.refresh()
}

// refresh rebuilds what the current section shows.
func (t *tui) refresh() {
	q := t.query()
	e := t.eng
	switch t.sec {
	case secSongs:
		t.rows = core.FilterSongs(e.Store.Songs, q)
	case secFavorites:
		t.rows = core.FilterSongs(e.Favorites(), q)
	case secRecent:
		t.rows = core.FilterSongs(e.Recent(recentDays), q)
	case secPlaylist:
		t.rows = core.FilterSongs(e.SongsForPlaylist(t.playlist), q)
	case secArtists:
		t.artists = e.Artists(q)
		if t.artistOpen {
			t.artistRows = t.buildArtistRows()
		}
	case secAlbums:
		t.albums = e.Albums(q, t.albumSort)
		if t.albumOpen {
			t.albumRows = t.buildAlbumRows()
			if len(t.albumRows) == 0 {
				t.albumOpen = false
			}
		}
	case secQueue:
		e.EnsureRandomQueue(10)
		t.queueRows = t.buildQueueRows()
	}
	t.sideCache, t.sideSel = nil, nil
	// The library totals only change with the library: not on every search key.
	if key := strconv.Itoa(len(e.Store.Songs)) + "|" + e.Store.Root; key != t.statsKey {
		t.refreshStats()
	}
}

func (t *tui) buildQueueRows() []item {
	var out []item
	if song, ok := t.eng.CurrentSong(); ok {
		out = append(out, item{song: song, now: true})
	}
	for _, s := range t.eng.QueuedSongs() {
		out = append(out, item{song: s})
	}
	return out
}

func (t *tui) buildArtistRows() []item {
	var out []item
	top := t.eng.TopPlayedForArtist(t.artist, 5)
	if len(top) > 0 {
		out = append(out, item{heading: "Más escuchadas"})
		for _, s := range top {
			out = append(out, item{song: s})
		}
	}
	for _, g := range t.eng.AlbumGroupsForArtist(t.artist) {
		out = append(out, item{heading: g.Album})
		for _, s := range g.Songs {
			out = append(out, item{song: s})
		}
	}
	return out
}

func (t *tui) buildAlbumRows() []item {
	var out []item
	for _, s := range t.eng.SongsForAlbum(t.albumID) {
		out = append(out, item{song: s})
	}
	return out
}

// ---- artwork ------------------------------------------------------------------------

func (t *tui) storeArt(msg artMsg) {
	delete(t.artLoading, msg.key)
	t.art[msg.key] = &artEntry{img: msg.art.Image, colors: msg.art.Colors}
	t.artOrder = append(t.artOrder, msg.key)
	if len(t.artOrder) > artKeep {
		old := t.artOrder[0]
		t.artOrder = t.artOrder[1:]
		delete(t.art, old)
		t.dropImages(old)
	}
	t.dropImages(msg.key)
}

// dropImages forgets the cell renderings of one image, so they are redone.
func (t *tui) dropImages(id string) {
	for k := range t.coverCache {
		if strings.Contains(k, "|"+id+"|") {
			delete(t.coverCache, k)
		}
	}
	t.forgetKitty(id)
}

// artCmds loads the covers the screen needs and that are not cached yet: the
// playing song's, the open album's, and the open artist's photo.
func (t *tui) artCmds() tea.Cmd {
	var cmds []tea.Cmd
	px := t.artPx
	want := func(s model.Song) {
		key := core.CoverKey(s)
		if _, ok := t.art[key]; ok || t.artLoading[key] {
			return
		}
		t.artLoading[key] = true
		path := s.Path
		cmds = append(cmds, func() tea.Msg {
			art := core.LoadArtwork(path, px)
			releaseMemory()
			return artMsg{key: key, art: art}
		})
	}
	if s, ok := t.eng.CurrentSong(); ok {
		want(s)
	}
	if t.sec == secAlbums && t.albumOpen && len(t.albumRows) > 0 {
		want(t.albumRows[0].song)
	}
	if t.sec == secArtists && t.artistOpen {
		key := core.ArtistAvatarKey(t.artist)
		if _, ok := t.avatars[key]; !ok && !t.avatarTry[key] {
			t.avatarTry[key] = true
			if img := core.LoadArtistAvatarFromDisk(t.artist, t.artPx); img != nil {
				t.avatars[key] = img
			} else {
				artist, client := t.artist, t.eng.LastFM
				cmds = append(cmds, func() tea.Msg {
					img, _ := core.FetchArtistAvatar(client, artist, px)
					releaseMemory()
					return avatarMsg{artist: artist, img: img}
				})
			}
		}
	}
	return tea.Batch(cmds...)
}

// palette is the colors of the playing song's cover, or a neutral set.
func (t *tui) palette() []color.RGBA {
	if s, ok := t.eng.CurrentSong(); ok {
		if a := t.art[core.CoverKey(s)]; a != nil && len(a.colors) > 0 {
			return a.colors
		}
	}
	return []color.RGBA{{R: 0x24, G: 0x3B, B: 0x6B, A: 255}, {R: 0x5B, G: 0x3A, B: 0x8C, A: 255}, {R: 0x1F, G: 0x6F, B: 0x8B, A: 255}, {R: 0x8C, G: 0x3A, B: 0x5B, A: 255}}
}

// cover draws a song's cover in cols x rows cells.
func (t *tui) cover(s model.Song, cols, rows int) []string {
	id := core.CoverKey(s)
	return t.image(id, cols, rows, t.art[id])
}

// avatar draws an artist's photo in cols x rows cells.
func (t *tui) avatar(artist string, cols, rows int) []string {
	id := "avatar:" + core.ArtistAvatarKey(artist)
	var a *artEntry
	if img, ok := t.avatars[core.ArtistAvatarKey(artist)]; ok {
		a = &artEntry{img: img}
	}
	return t.image(id, cols, rows, a)
}

func (t *tui) image(id string, cols, rows int, a *artEntry) []string {
	if t.proto == protoKitty && a != nil && a.img != nil {
		// In the terminal at full resolution, or on its way: half blocks until then.
		if lines := t.kittyImage(id, cols, rows, a.img); lines != nil {
			return lines
		}
	}
	key := "img|" + id + "|" + strconv.Itoa(cols) + "x" + strconv.Itoa(rows)
	if lines, ok := t.coverCache[key]; ok {
		return lines
	}
	var lines []string
	if a != nil && a.img != nil {
		lines = halfBlocks(a.img, cols, rows)
	} else {
		lines = placeholderArt(cols, rows, t.th.faint, t.th.chip, "♪")
	}
	t.coverCache[key] = lines
	return lines
}
