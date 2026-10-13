package model

import "time"

type Song struct {
	ID           string
	Path         string
	Title        string
	Artist       string
	Album        string
	TrackNum     int
	Duration     time.Duration
	Format       string
	PlayCount    int
	LastPlayedAt time.Time
}

type SmartPlaylistFilter struct {
	ArtistContains string
	AlbumContains  string
	TitleContains  string
	FavoritesOnly  bool
}

type Playlist struct {
	Name       string
	Color      string
	ManualSong []string
	Smart      *SmartPlaylistFilter
}
