package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"

	"musaik/internal/model"
)

var supportedExtensions = map[string]struct{}{
	".mp3":  {},
	".mp4":  {},
	".flac": {},
	".ogg":  {},
}

func isSupported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := supportedExtensions[ext]
	return ok
}

func readMetadata(path string) model.Song {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	song := model.Song{
		ID:     path,
		Path:   path,
		Title:  base,
		Artist: "Unknown Artist",
		Album:  "Unknown Album",
		Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
	}

	f, err := os.Open(path)
	if err != nil {
		return song
	}
	defer f.Close()

	meta, err := tag.ReadFrom(f)
	if err != nil {
		return song
	}

	if meta.Title() != "" {
		song.Title = meta.Title()
	}
	if meta.Artist() != "" {
		song.Artist = meta.Artist()
	}
	if meta.Album() != "" {
		song.Album = meta.Album()
	}
	if track, _ := meta.Track(); track > 0 {
		song.TrackNum = track
	}
	return song
}

func Scan(root string) ([]model.Song, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("accessing %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", root)
	}

	songs := make([]model.Song, 0, 512)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !isSupported(path) {
			return nil
		}
		songs = append(songs, readMetadata(path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %q: %w", root, err)
	}
	return songs, nil
}
