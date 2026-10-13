package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"musaik/internal/model"
)

func TestScanFiltersSupportedFiles(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"a.mp3",
		"b.mp4",
		"c.flac",
		"d.ogg",
		"e.txt",
	}
	for _, f := range files {
		if err := writeEmpty(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}

	songs, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 4 {
		t.Fatalf("expected 4 supported songs, got %d", len(songs))
	}
}

func TestSmartPlaylistFavoritesFilter(t *testing.T) {
	s := NewStore()
	s.Songs = []model.Song{
		{ID: "1", Title: "Rock One", Artist: "A"},
		{ID: "2", Title: "Jazz One", Artist: "B"},
	}
	s.Favorites["1"] = true

	pl := model.Playlist{
		Name: "FavRock",
		Smart: &model.SmartPlaylistFilter{
			FavoritesOnly: true,
			TitleContains: "rock",
		},
	}

	out := s.PlaylistSongs(pl)
	if len(out) != 1 || out[0].ID != "1" {
		t.Fatalf("unexpected playlist output: %+v", out)
	}
}

func TestPlayCountOperations(t *testing.T) {
	s := NewStore()
	s.Songs = []model.Song{
		{ID: "1", Title: "Song A"},
	}

	count, ok := s.IncrementPlayCount("1")
	if !ok || count != 1 {
		t.Fatalf("expected increment count 1, got count=%d ok=%t", count, ok)
	}
	if !s.SetPlayCount("1", 7) {
		t.Fatal("expected SetPlayCount to succeed")
	}
	if s.Songs[0].PlayCount != 7 {
		t.Fatalf("expected play count 7, got %d", s.Songs[0].PlayCount)
	}
	when := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	if !s.MarkPlayedAt("1", when) {
		t.Fatal("expected MarkPlayedAt to succeed")
	}
	if !s.Songs[0].LastPlayedAt.Equal(when) {
		t.Fatalf("expected LastPlayedAt %v, got %v", when, s.Songs[0].LastPlayedAt)
	}
}

func TestAddSongToManualPlaylist(t *testing.T) {
	s := NewStore()
	s.AddManualPlaylist("Ruta", nil)
	if err := s.AddSongToManualPlaylist("Ruta", "song-1"); err != nil {
		t.Fatalf("unexpected add song error: %v", err)
	}
	if err := s.AddSongToManualPlaylist("Ruta", "song-1"); err != nil {
		t.Fatalf("expected duplicate add to be ignored, got error: %v", err)
	}
	if len(s.Playlists) == 0 || len(s.Playlists[len(s.Playlists)-1].ManualSong) != 1 {
		t.Fatalf("expected one song in manual playlist, got %+v", s.Playlists)
	}
	if err := s.AddSongToManualPlaylist("Favoritos", "song-1"); err == nil {
		t.Fatal("expected smart playlist add to fail")
	}
}

func TestRenameManualPlaylist(t *testing.T) {
	s := NewStore()
	s.AddManualPlaylist("Ruta", nil)

	if err := s.RenameManualPlaylist("Ruta", "Ruta Nocturna"); err != nil {
		t.Fatalf("unexpected rename error: %v", err)
	}
	if s.Playlists[len(s.Playlists)-1].Name != "Ruta Nocturna" {
		t.Fatalf("expected playlist renamed, got %+v", s.Playlists[len(s.Playlists)-1])
	}

	if err := s.RenameManualPlaylist("Favoritos", "X"); err == nil {
		t.Fatal("expected smart playlist rename to fail")
	}
	if err := s.RenameManualPlaylist("Ruta Nocturna", "Todos"); err == nil {
		t.Fatal("expected duplicate rename to fail")
	}
}

func TestDeleteManualPlaylist(t *testing.T) {
	s := NewStore()
	s.AddManualPlaylist("Temporal", nil)

	if err := s.DeleteManualPlaylist("Temporal"); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	for _, pl := range s.Playlists {
		if pl.Name == "Temporal" {
			t.Fatal("expected playlist to be removed")
		}
	}

	if err := s.DeleteManualPlaylist("Favoritos"); err == nil {
		t.Fatal("expected smart playlist delete to fail")
	}
}

func TestSetManualPlaylistColor(t *testing.T) {
	s := NewStore()
	s.AddManualPlaylist("Ruta", nil)

	if err := s.SetManualPlaylistColor("Ruta", "purple"); err != nil {
		t.Fatalf("unexpected color update error: %v", err)
	}
	if got := s.Playlists[len(s.Playlists)-1].Color; got != "purple" {
		t.Fatalf("expected playlist color purple, got %q", got)
	}

	if err := s.SetManualPlaylistColor("Favoritos", "yellow"); err == nil {
		t.Fatal("expected smart playlist color update to fail")
	}
}

func writeEmpty(path string) error {
	return os.WriteFile(path, []byte(""), 0o644)
}
