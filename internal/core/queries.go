package core

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"musaik/internal/model"
)

// Favorites are the liked songs, in library order.
func (e *Engine) Favorites() []model.Song {
	out := make([]model.Song, 0, len(e.Store.Favorites))
	for _, s := range e.Store.Songs {
		if e.Store.Favorites[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// Recent are the songs played in the last days, newest first.
func (e *Engine) Recent(days int) []model.Song {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	out := make([]model.Song, 0, 64)
	for _, s := range e.Store.Songs {
		if s.LastPlayedAt.IsZero() || s.LastPlayedAt.Before(cutoff) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastPlayedAt.After(out[j].LastPlayedAt) })
	return out
}

// FilterSongs keeps the songs whose title, artist or album contains q.
func FilterSongs(in []model.Song, q string) []model.Song {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return in
	}
	out := make([]model.Song, 0, len(in))
	for _, s := range in {
		if strings.Contains(strings.ToLower(s.Title), q) ||
			strings.Contains(strings.ToLower(s.Artist), q) ||
			strings.Contains(strings.ToLower(s.Album), q) {
			out = append(out, s)
		}
	}
	return out
}

// Artists are the artist names matching q, sorted without regard to case.
func (e *Engine) Artists(q string) []string {
	q = strings.TrimSpace(strings.ToLower(q))
	all := e.Store.Artists()
	out := make([]string, 0, len(all))
	for _, a := range all {
		if q == "" || strings.Contains(strings.ToLower(a), q) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// ArtistAlbumCount counts the distinct albums of an artist.
func (e *Engine) ArtistAlbumCount(artist string) int {
	albums := map[string]bool{}
	for _, s := range e.Store.Songs {
		if s.Artist != artist {
			continue
		}
		album := strings.TrimSpace(s.Album)
		if album == "" {
			album = "Unknown Album"
		}
		albums[strings.ToLower(album)] = true
	}
	return len(albums)
}

// ArtistSongCount counts the songs of an artist.
func (e *Engine) ArtistSongCount(artist string) int {
	n := 0
	for _, s := range e.Store.Songs {
		if s.Artist == artist {
			n++
		}
	}
	return n
}

// SongsForArtist are the songs of an artist, in library order.
func (e *Engine) SongsForArtist(artist string) []model.Song {
	out := make([]model.Song, 0, 64)
	for _, s := range e.Store.Songs {
		if s.Artist == artist {
			out = append(out, s)
		}
	}
	return out
}

// TopPlayedForArtist are an artist's most played songs.
func (e *Engine) TopPlayedForArtist(artist string, limit int) []model.Song {
	songs := e.SongsForArtist(artist)
	sort.Slice(songs, func(i, j int) bool {
		if songs[i].PlayCount != songs[j].PlayCount {
			return songs[i].PlayCount > songs[j].PlayCount
		}
		return strings.ToLower(songs[i].Title) < strings.ToLower(songs[j].Title)
	})
	if limit > 0 && len(songs) > limit {
		songs = songs[:limit]
	}
	return songs
}

// AlbumGroup is an album of an artist with its songs in track order.
type AlbumGroup struct {
	Album string
	Songs []model.Song
}

func trackLess(a, b model.Song) bool {
	ta, tb := a.TrackNum, b.TrackNum
	if ta > 0 && tb > 0 && ta != tb {
		return ta < tb
	}
	if ta > 0 && tb == 0 {
		return true
	}
	if tb > 0 && ta == 0 {
		return false
	}
	return strings.ToLower(a.Title) < strings.ToLower(b.Title)
}

// AlbumGroupsForArtist splits an artist's songs by album, albums by name.
func (e *Engine) AlbumGroupsForArtist(artist string) []AlbumGroup {
	songs := e.SongsForArtist(artist)
	sort.Slice(songs, func(i, j int) bool {
		ai := strings.ToLower(strings.TrimSpace(songs[i].Album))
		aj := strings.ToLower(strings.TrimSpace(songs[j].Album))
		if ai != aj {
			return ai < aj
		}
		return trackLess(songs[i], songs[j])
	})
	groups := make([]AlbumGroup, 0, 16)
	for _, s := range songs {
		album := strings.TrimSpace(s.Album)
		if album == "" {
			album = "Unknown Album"
		}
		if len(groups) == 0 || groups[len(groups)-1].Album != album {
			groups = append(groups, AlbumGroup{Album: album, Songs: []model.Song{s}})
			continue
		}
		groups[len(groups)-1].Songs = append(groups[len(groups)-1].Songs, s)
	}
	return groups
}

// AlbumIdentity is the key that groups songs into an album.
func AlbumIdentity(s model.Song) string {
	artist := strings.TrimSpace(strings.ToLower(s.Artist))
	album := strings.TrimSpace(strings.ToLower(s.Album))
	if artist == "" && album == "" {
		return strings.TrimSpace(strings.ToLower(s.Path))
	}
	return artist + "||" + album
}

// SongsForAlbum are the songs of an album in track order.
func (e *Engine) SongsForAlbum(identity string) []model.Song {
	out := make([]model.Song, 0, 32)
	for _, s := range e.Store.Songs {
		if AlbumIdentity(s) == identity {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return trackLess(out[i], out[j]) })
	return out
}

// AlbumSorts are the orders the album list supports.
func AlbumSorts() []string { return []string{"Artista", "Nombre", "Año"} }

// Albums returns one representative song per album of the songs matching q,
// ordered by sortBy ("Artista", "Nombre" or "Año", the file's modification time).
func (e *Engine) Albums(q, sortBy string) []model.Song {
	songs := FilterSongs(e.Store.Songs, q)
	out := make([]model.Song, 0, len(songs)/8+1)
	seen := map[string]bool{}
	for _, s := range songs {
		k := strings.ToLower(strings.TrimSpace(s.Artist + "||" + s.Album))
		if k == "||" {
			k = s.ID
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	switch sortBy {
	case "Nombre":
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Album) < strings.ToLower(out[j].Album) })
	case "Año":
		mod := make(map[string]time.Time, len(out))
		for _, s := range out {
			if info, err := os.Stat(s.Path); err == nil {
				mod[s.ID] = info.ModTime()
			}
		}
		sort.Slice(out, func(i, j int) bool {
			mi, oki := mod[out[i].ID]
			mj, okj := mod[out[j].ID]
			if !oki || !okj {
				return strings.ToLower(out[i].Album) < strings.ToLower(out[j].Album)
			}
			return mi.After(mj)
		})
	default:
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Artist) < strings.ToLower(out[j].Artist) })
	}
	return out
}

// VisiblePlaylists are the playlists listed in the sidebar (all but "Todos").
func (e *Engine) VisiblePlaylists() []model.Playlist {
	out := make([]model.Playlist, 0, len(e.Store.Playlists))
	for _, pl := range e.Store.Playlists {
		if pl.Name != "Todos" {
			out = append(out, pl)
		}
	}
	return out
}

// ManualPlaylistNames are the playlists a song can be added to.
func (e *Engine) ManualPlaylistNames() []string {
	out := make([]string, 0, len(e.Store.Playlists))
	for _, pl := range e.Store.Playlists {
		if pl.Name != "Todos" && pl.Smart == nil {
			out = append(out, pl.Name)
		}
	}
	return out
}

// PlaylistOptions are all the playlist names, "Todos" included.
func (e *Engine) PlaylistOptions() []string {
	out := make([]string, 0, len(e.Store.Playlists))
	for _, pl := range e.Store.Playlists {
		out = append(out, pl.Name)
	}
	if len(out) == 0 {
		out = append(out, "Todos")
	}
	return out
}

// SongsForPlaylist are the songs of a playlist; an unknown name gives the library.
func (e *Engine) SongsForPlaylist(name string) []model.Song {
	for _, pl := range e.Store.Playlists {
		if pl.Name == name {
			return e.Store.PlaylistSongs(pl)
		}
	}
	return e.Store.Songs
}

// SongFileSize is the size on disk of a song, cached.
func (e *Engine) SongFileSize(s model.Song) int64 {
	if v, ok := e.sizeCache[s.ID]; ok {
		return max(v, 0)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		e.sizeCache[s.ID] = -1
		return 0
	}
	e.sizeCache[s.ID] = info.Size()
	return info.Size()
}

// TotalLibrarySize is the size of all the songs, formatted.
func (e *Engine) TotalLibrarySize(songs []model.Song) string {
	var total int64
	for _, s := range songs {
		total += e.SongFileSize(s)
	}
	return FormatFileSizeMB(total)
}

// TotalDuration adds up the length of songs.
func TotalDuration(songs []model.Song) time.Duration {
	var d time.Duration
	for _, s := range songs {
		d += s.Duration
	}
	return d
}

// ---- formatting ---------------------------------------------------------------

// FormatDuration is mm:ss, or "--:--" for no duration.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "--:--"
	}
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// FormatDurationLong is "1h 2m 3s".
func FormatDurationLong(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	total := int(d.Round(time.Second).Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}

// FormatHoursMinutes is "12 horas 05 min".
func FormatHoursMinutes(d time.Duration) string {
	m := int(d.Minutes())
	return fmt.Sprintf("%d horas %02d min", m/60, m%60)
}

// FormatFileSizeMB is the size in MB, or "--" when unknown.
func FormatFileSizeMB(size int64) string {
	if size <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

// CompactTrackTitle prefixes the title with its track number when known.
func CompactTrackTitle(s model.Song) string {
	if s.TrackNum > 0 {
		return fmt.Sprintf("%02d. %s", s.TrackNum, s.Title)
	}
	return s.Title
}

// CompactFormat is the upper-case file format, or "--".
func CompactFormat(s model.Song) string {
	if s.Format == "" {
		return "--"
	}
	return strings.ToUpper(s.Format)
}

// QualityBadge describes the audio quality a format usually has.
func QualityBadge(s model.Song) string {
	switch strings.ToUpper(s.Format) {
	case "FLAC", "ALAC":
		return "24-bit / 96kHz"
	case "DSD":
		return "1-bit / 2.8MHz"
	}
	return "16-bit / 44.1kHz"
}

// IsLossless tells whether the format is one the app badges as lossless.
func IsLossless(format string) bool {
	switch strings.ToUpper(format) {
	case "FLAC", "ALAC", "DSD", "WAV":
		return true
	}
	return false
}
