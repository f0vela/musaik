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
)

// version is shown in Settings > About; it follows the desktop app's mygo.json,
// and a build can set it: -ldflags "-X main.version=1.2.3".
var version = "0.4.1"

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
}

const (
	artMax     = 160
	artKeep    = 40
	tickEvery  = 250 * time.Millisecond
	rowsPerPg  = 10
	recentDays = 15
)

func newTUI(eng *core.Engine, mini bool) *tui {
	t := &tui{
		eng:        eng,
		th:         newTheme(true),
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
	}
	if id, ok := sectionFromName(eng.Settings.Section); ok && id != secPlaylist && id != secSettings {
		t.sec = id
	}
	if strings.TrimSpace(eng.Settings.SearchQuery) != "" {
		t.search.Set(eng.Settings.SearchQuery)
	}
	t.initSettings()
	for _, pl := range eng.VisiblePlaylists() {
		t.playlist = pl.Name
		break
	}
	t.syncSideCursor()
	return t
}

func (t *tui) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tickCmd(), t.artCmds())
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickEvery, func(at time.Time) tea.Msg { return tickMsg(at) })
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		t.th = newTheme(msg.IsDark())
	case tickMsg:
		t.onTick()
		cmds = append(cmds, tickCmd())
	case funcMsg:
		msg()
		t.rowsDirty = true
	case artMsg:
		t.storeArt(msg)
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
	cmds = append(cmds, t.artCmds())
	if t.quiting {
		return t, tea.Quit
	}
	return t, tea.Batch(cmds...)
}

// onTick runs the things that need doing a few times a second.
func (t *tui) onTick() {
	t.eng.ConsumePlayerEvents()
	t.eng.FlushIfDirty(false)
	t.ticks++
	if t.ticks%8 == 0 {
		t.eng.DetectIPod(false)
	}
	if t.eng.PlayingID != t.lastPlaying {
		t.lastPlaying = t.eng.PlayingID
		t.rowsDirty = true
	}
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
	t.stats = t.computeStats()
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
}

// artCmds loads the covers the screen needs and that are not cached yet: the
// playing song's, the open album's, and the open artist's photo.
func (t *tui) artCmds() tea.Cmd {
	var cmds []tea.Cmd
	want := func(s model.Song) {
		key := core.CoverKey(s)
		if _, ok := t.art[key]; ok || t.artLoading[key] {
			return
		}
		t.artLoading[key] = true
		path := s.Path
		cmds = append(cmds, func() tea.Msg { return artMsg{key: key, art: core.LoadArtwork(path, artMax)} })
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
			if img := core.LoadArtistAvatarFromDisk(t.artist, artMax); img != nil {
				t.avatars[key] = img
			} else {
				artist, client := t.artist, t.eng.LastFM
				cmds = append(cmds, func() tea.Msg {
					img, _ := core.FetchArtistAvatar(client, artist, artMax)
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
