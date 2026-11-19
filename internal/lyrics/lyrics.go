// Package lyrics finds the lyrics of a song and follows them as it plays.
//
// They come from, in order: a .lrc file next to the audio file, the lyrics
// tag embedded in it, and LRCLIB (https://lrclib.net), a free community
// database that needs no key and has time-synced lyrics. Answers from LRCLIB
// are cached on disk, misses too (for a while), so a song is looked up once.
// Nothing is sent anywhere until Fetch is called, which the apps only do when
// the user opens the lyrics.
package lyrics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

// ErrNotFound is returned when no source has lyrics for the song.
var ErrNotFound = errors.New("no hay letra para esta canción")

// Line is one line of a synced lyric, sung At into the song.
type Line struct {
	At   time.Duration
	Text string
}

// Lyrics is what was found for a song: synced lines, or just text, or the
// knowledge that the song has no words.
type Lyrics struct {
	// Synced has the lines with their time, sorted; empty for plain lyrics.
	Synced []Line
	// Plain is the text without times (always set when there is any lyric).
	Plain string
	// Instrumental is true for a song known to have no lyrics.
	Instrumental bool
	// Source says where it came from: "lrclib", "lrc" (a sidecar file) or "tag".
	Source string
}

// IsSynced reports whether the lyrics carry times to follow along with.
func (l *Lyrics) IsSynced() bool { return l != nil && len(l.Synced) > 0 }

// Lines is the text to show, one entry per line: the synced lines or the
// plain text split on newlines.
func (l *Lyrics) Lines() []string {
	if l == nil {
		return nil
	}
	if len(l.Synced) > 0 {
		out := make([]string, len(l.Synced))
		for i, ln := range l.Synced {
			out[i] = ln.Text
		}
		return out
	}
	if strings.TrimSpace(l.Plain) == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(strings.TrimRight(l.Plain, "\r\n "), "\r\n", "\n"), "\n")
}

// Current is the index of the line being sung at pos: the last one that has
// started. It is -1 before the first line and for lyrics without times.
func (l *Lyrics) Current(pos time.Duration) int {
	if l == nil || len(l.Synced) == 0 {
		return -1
	}
	// First line that starts after pos, minus one.
	i := sort.Search(len(l.Synced), func(i int) bool { return l.Synced[i].At > pos })
	return i - 1
}

// Query says which song to look for. Path, when set, is also searched for a
// sidecar .lrc file and an embedded lyrics tag.
type Query struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Path     string
}

// ---- LRC ----------------------------------------------------------------------

var (
	lrcStamp  = regexp.MustCompile(`\[(\d+):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	lrcWord   = regexp.MustCompile(`<\d+:\d{1,2}(?:[.:]\d{1,3})?>`)
	lrcOffset = regexp.MustCompile(`(?mi)^\[offset:\s*([+-]?\d+)\s*\]`)
)

// ParseLRC reads LRC text ("[01:23.45] words", several stamps per line
// allowed, [offset:ms] honored) into lines sorted by time. It returns nil if
// the text has no timestamps.
func ParseLRC(text string) []Line {
	var offset time.Duration
	if m := lrcOffset.FindStringSubmatch(text); m != nil {
		if ms, err := strconv.Atoi(m[1]); err == nil {
			offset = time.Duration(ms) * time.Millisecond
		}
	}
	var out []Line
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if raw == "" {
			continue
		}
		stamps := lrcStamp.FindAllStringSubmatchIndex(raw, -1)
		if len(stamps) == 0 {
			continue
		}
		// The text is what follows the last leading stamp.
		end := 0
		for _, s := range stamps {
			if s[0] != end {
				break
			}
			end = s[1]
		}
		body := strings.TrimSpace(lrcWord.ReplaceAllString(raw[end:], ""))
		for _, s := range stamps {
			if s[1] > end {
				break
			}
			mins, _ := strconv.Atoi(raw[s[2]:s[3]])
			sec, _ := strconv.Atoi(raw[s[4]:s[5]])
			var frac time.Duration
			if s[6] >= 0 {
				digits := raw[s[6]:s[7]]
				n, _ := strconv.Atoi(digits)
				// "5" is 500 ms, "50" 500 ms, "500" 500 ms.
				frac = time.Duration(n) * time.Second / time.Duration(pow10(len(digits)))
			}
			at := time.Duration(mins)*time.Minute + time.Duration(sec)*time.Second + frac + offset
			out = append(out, Line{At: max(at, 0), Text: body})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func pow10(n int) int {
	p := 1
	for ; n > 0; n-- {
		p *= 10
	}
	return p
}

// fromText builds Lyrics from text that may be LRC or plain.
func fromText(text, source string) *Lyrics {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if lines := ParseLRC(text); len(lines) > 0 {
		return &Lyrics{Synced: lines, Plain: plainOf(lines), Source: source}
	}
	return &Lyrics{Plain: text, Source: source}
}

func plainOf(lines []Line) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = l.Text
	}
	return strings.Join(parts, "\n")
}

// ---- local sources -----------------------------------------------------------------

// Local looks for lyrics on the user's disk: a .lrc with the audio file's
// name, then the lyrics tag embedded in it. It returns nil when there is none.
func Local(path string) *Lyrics {
	if path == "" {
		return nil
	}
	if data, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".lrc"); err == nil {
		if l := fromText(string(data), "lrc"); l != nil {
			return l
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}
	return fromText(meta.Lyrics(), "tag")
}

// ---- LRCLIB -------------------------------------------------------------------------

// DefaultBaseURL is LRCLIB's API.
const DefaultBaseURL = "https://lrclib.net/api"

const (
	missTTL       = 7 * 24 * time.Hour
	requestTimout = 10 * time.Second
	// durationSlack is how far a search result's length may be from the
	// song's and still count as the same recording.
	durationSlack = 4 * time.Second
)

// Client looks lyrics up. The zero value works: it uses LRCLIB and no cache.
type Client struct {
	// UserAgent identifies the app to LRCLIB, which asks for it.
	UserAgent string
	// BaseURL overrides the API (tests).
	BaseURL string
	// CacheDir keeps answers on disk; "" turns the cache off.
	CacheDir string
	HTTP     *http.Client
}

// NewClient is a client that caches under the user's config directory, next
// to the rest of Musaik's data.
func NewClient(userAgent string) *Client {
	c := &Client{UserAgent: userAgent}
	if base, err := os.UserConfigDir(); err == nil {
		c.CacheDir = filepath.Join(base, "musaik", "lyrics-cache")
	}
	return c
}

type apiTrack struct {
	ID           int     `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// cacheEntry is what is stored for a song. A miss has neither text nor Instrumental.
type cacheEntry struct {
	Plain        string    `json:"plain,omitempty"`
	Synced       string    `json:"synced,omitempty"`
	Instrumental bool      `json:"instrumental,omitempty"`
	At           time.Time `json:"at"`
}

func (e cacheEntry) lyrics() *Lyrics {
	if e.Instrumental {
		return &Lyrics{Instrumental: true, Source: "lrclib"}
	}
	var l *Lyrics
	if e.Synced != "" {
		l = fromText(e.Synced, "lrclib")
	}
	if l == nil && e.Plain != "" {
		l = &Lyrics{Plain: e.Plain, Source: "lrclib"}
	}
	if l != nil && l.Plain == "" {
		l.Plain = e.Plain
	}
	return l
}

func (c *Client) cachePath(q Query) string {
	if c.CacheDir == "" {
		return ""
	}
	key := strings.ToLower(strings.Join([]string{q.Artist, q.Title, q.Album, strconv.Itoa(int(q.Duration / time.Second))}, "\x00"))
	sum := sha1.Sum([]byte(key))
	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readCache(q Query) (*Lyrics, bool) {
	path := c.cachePath(q)
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil {
		return nil, false
	}
	if l := e.lyrics(); l != nil {
		return l, true
	}
	// A cached miss only holds for a while: the database grows.
	return nil, time.Since(e.At) < missTTL
}

func (c *Client) writeCache(q Query, e cacheEntry) {
	path := c.cachePath(q)
	if path == "" {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	e.At = time.Now()
	if data, err := json.Marshal(e); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// Fetch finds the lyrics for q: local files first, then the cache, then
// LRCLIB. It returns ErrNotFound when nobody has them, and another error
// when the network failed (nothing is cached then, so it is tried again).
func (c *Client) Fetch(ctx context.Context, q Query) (*Lyrics, error) {
	if l := Local(q.Path); l != nil {
		return l, nil
	}
	if strings.TrimSpace(q.Title) == "" {
		return nil, ErrNotFound
	}
	if l, hit := c.readCache(q); hit {
		if l == nil {
			return nil, ErrNotFound
		}
		return l, nil
	}
	tr, err := c.lookup(ctx, q)
	if err != nil {
		return nil, err
	}
	if tr == nil {
		c.writeCache(q, cacheEntry{})
		return nil, ErrNotFound
	}
	e := cacheEntry{Plain: tr.PlainLyrics, Synced: tr.SyncedLyrics, Instrumental: tr.Instrumental}
	c.writeCache(q, e)
	l := e.lyrics()
	if l == nil {
		return nil, ErrNotFound
	}
	return l, nil
}

// lookup asks for an exact match and, failing that, searches. It returns nil
// for "no such song".
func (c *Client) lookup(ctx context.Context, q Query) (*apiTrack, error) {
	title := strings.TrimSpace(q.Title)
	v := url.Values{}
	v.Set("track_name", title)
	v.Set("artist_name", strings.TrimSpace(q.Artist))
	if q.Album != "" {
		v.Set("album_name", strings.TrimSpace(q.Album))
	}
	if q.Duration > 0 {
		v.Set("duration", strconv.Itoa(int(q.Duration.Round(time.Second)/time.Second)))
	}
	var tr apiTrack
	status, err := c.getJSON(ctx, "/get", v, &tr)
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK && hasContent(tr) {
		return &tr, nil
	}
	// Not an exact match: search with the title as it is, then without the
	// "(feat. …)" and "- Remastered" kind of suffix.
	tried := map[string]bool{}
	for _, name := range []string{title, cleanTitle(title)} {
		if name == "" || tried[name] {
			continue
		}
		tried[name] = true
		sv := url.Values{}
		sv.Set("track_name", name)
		if a := strings.TrimSpace(q.Artist); a != "" {
			sv.Set("artist_name", a)
		}
		var list []apiTrack
		if _, err := c.getJSON(ctx, "/search", sv, &list); err != nil {
			return nil, err
		}
		if best := pickBest(list, q); best != nil {
			return best, nil
		}
	}
	return nil, nil
}

func hasContent(t apiTrack) bool {
	return t.Instrumental || t.SyncedLyrics != "" || t.PlainLyrics != ""
}

var suffixRE = regexp.MustCompile(`(?i)\s*[\(\[][^\)\]]*(feat|ft\.|with|remaster|live|version|edit|mix)[^\)\]]*[\)\]]|\s+-\s+(.*(remaster|live|version|edit|mix|mono|stereo).*)$`)

// cleanTitle drops the parenthesized extras and "- Remastered 2011" tails.
func cleanTitle(t string) string { return strings.TrimSpace(suffixRE.ReplaceAllString(t, "")) }

// pickBest chooses among search results: same length (within a few seconds)
// as the song when it is known, synced before plain, and the artist matching.
func pickBest(list []apiTrack, q Query) *apiTrack {
	var best *apiTrack
	bestScore := -1
	for i := range list {
		t := &list[i]
		if !hasContent(*t) {
			continue
		}
		score := 0
		if q.Duration > 0 && t.Duration > 0 {
			diff := q.Duration - time.Duration(t.Duration*float64(time.Second))
			if diff < 0 {
				diff = -diff
			}
			if diff > durationSlack {
				continue
			}
			score += 4
		}
		if a := strings.ToLower(strings.TrimSpace(q.Artist)); a != "" && strings.ToLower(t.ArtistName) == a {
			score += 8
		}
		if t.SyncedLyrics != "" {
			score += 2
		}
		if score > bestScore {
			best, bestScore = t, score
		}
	}
	// Without an artist in common or a length to compare, a result is a guess.
	if best != nil && bestScore < 4 && q.Duration > 0 {
		return nil
	}
	return best
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, out any) (int, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+params.Encode(), nil)
	if err != nil {
		return 0, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = "Musaik"
	}
	req.Header.Set("User-Agent", ua)
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, fmt.Errorf("sin conexión con LRCLIB: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return resp.StatusCode, nil
	case resp.StatusCode != http.StatusOK:
		return resp.StatusCode, fmt.Errorf("LRCLIB respondió %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("respuesta inválida de LRCLIB: %w", err)
	}
	return resp.StatusCode, nil
}
