package library

import (
	"os"
	"path/filepath"
	"testing"

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

func writeEmpty(path string) error {
	return os.WriteFile(path, []byte(""), 0o644)
}
