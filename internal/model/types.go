package model

import "time"

type Song struct {
	ID       string
	Path     string
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Format   string
}

type SmartPlaylistFilter struct {
	ArtistContains string
	AlbumContains  string
	TitleContains  string
	FavoritesOnly  bool
}

type Playlist struct {
	Name       string
	ManualSong []string
	Smart      *SmartPlaylistFilter
}
