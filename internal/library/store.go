package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"musaik/internal/model"
)

type Store struct {
	Root          string
	Songs         []model.Song
	Favorites     map[string]bool
	Playlists     []model.Playlist
	artistsCached []string
	albumsCached  []string
}

func NewStore() *Store {
	return &Store{
		Favorites: map[string]bool{},
		Playlists: []model.Playlist{
			{Name: "Todos"},
			{Name: "Favoritos", Smart: &model.SmartPlaylistFilter{FavoritesOnly: true}},
		},
	}
}

func (s *Store) Index(root string) error {
	songs, err := Scan(root)
	if err != nil {
		return err
	}
	s.Root = root
	s.Songs = songs
	s.rebuildIndexes()
	return nil
}

func (s *Store) ToggleFavorite(songID string) {
	if s.Favorites[songID] {
		delete(s.Favorites, songID)
		return
	}
	s.Favorites[songID] = true
}

func (s *Store) Artists() []string {
	return slices.Clone(s.artistsCached)
}

func (s *Store) Albums() []string {
	return slices.Clone(s.albumsCached)
}

func (s *Store) PlaylistSongs(p model.Playlist) []model.Song {
	if p.Smart == nil && len(p.ManualSong) == 0 {
		return slices.Clone(s.Songs)
	}

	songByID := make(map[string]model.Song, len(s.Songs))
	for _, song := range s.Songs {
		songByID[song.ID] = song
	}

	if p.Smart == nil {
		out := make([]model.Song, 0, len(p.ManualSong))
		for _, id := range p.ManualSong {
			if song, ok := songByID[id]; ok {
				out = append(out, song)
			}
		}
		return out
	}

	filter := p.Smart
	out := make([]model.Song, 0, len(s.Songs))
	for _, song := range s.Songs {
		if filter.FavoritesOnly && !s.Favorites[song.ID] {
			continue
		}
		if filter.ArtistContains != "" && !containsFold(song.Artist, filter.ArtistContains) {
			continue
		}
		if filter.AlbumContains != "" && !containsFold(song.Album, filter.AlbumContains) {
			continue
		}
		if filter.TitleContains != "" && !containsFold(song.Title, filter.TitleContains) {
			continue
		}
		out = append(out, song)
	}
	return out
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func (s *Store) AddSmartPlaylist(name string, filter model.SmartPlaylistFilter) {
	s.Playlists = append(s.Playlists, model.Playlist{
		Name:  name,
		Smart: &filter,
	})
}

func (s *Store) AddManualPlaylist(name string, songIDs []string) {
	ids := slices.Clone(songIDs)
	s.Playlists = append(s.Playlists, model.Playlist{
		Name:       name,
		ManualSong: ids,
	})
}

func (s *Store) rebuildIndexes() {
	artistsSet := map[string]struct{}{}
	albumsSet := map[string]struct{}{}

	for _, song := range s.Songs {
		artistsSet[song.Artist] = struct{}{}
		albumsSet[song.Album] = struct{}{}
	}

	s.artistsCached = mapToSortedSlice(artistsSet)
	s.albumsCached = mapToSortedSlice(albumsSet)
}

func mapToSortedSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

type persistedState struct {
	Favorites map[string]bool  `json:"favorites"`
	Playlists []model.Playlist `json:"playlists"`
	Root      string           `json:"root"`
	Songs     []model.Song     `json:"songs"`
}

func StatePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("reading user config dir: %w", err)
	}
	dir := filepath.Join(base, "musaik")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	return filepath.Join(dir, "state.json"), nil
}

func (s *Store) SaveState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	p := persistedState{
		Favorites: s.Favorites,
		Playlists: s.Playlists,
		Root:      s.Root,
		Songs:     s.Songs,
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	return nil
}

func (s *Store) LoadState() error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading state: %w", err)
	}
	var p persistedState
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("decoding state: %w", err)
	}
	if p.Favorites != nil {
		s.Favorites = p.Favorites
	}
	if len(p.Playlists) > 0 {
		s.Playlists = p.Playlists
	}
	if p.Root != "" {
		s.Root = p.Root
	}
	if len(p.Songs) > 0 {
		s.Songs = p.Songs
		s.rebuildIndexes()
	}
	return nil
}
