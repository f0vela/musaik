package ipod

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"musaik/internal/model"
)

type Device struct {
	Name      string
	MountPath string
}

func DetectClassicMounts(volumesRoot string) ([]Device, error) {
	entries, err := os.ReadDir(volumesRoot)
	if err != nil {
		return nil, fmt.Errorf("read volumes root: %w", err)
	}
	out := make([]Device, 0, 2)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mount := filepath.Join(volumesRoot, e.Name())
		controlDir := filepath.Join(mount, "iPod_Control")
		info, err := os.Stat(controlDir)
		if err != nil || !info.IsDir() {
			continue
		}
		db := filepath.Join(controlDir, "iTunes", "iTunesDB")
		if st, err := os.Stat(db); err != nil || st.IsDir() {
			continue
		}
		out = append(out, Device{
			Name:      e.Name(),
			MountPath: mount,
		})
	}
	return out, nil
}

func SyncPlaylist(deviceMountPath string, playlistName string, songs []model.Song) (copied int, m3uPath string, err error) {
	if playlistName == "" {
		return 0, "", fmt.Errorf("playlist name is required")
	}
	musicDir := filepath.Join(deviceMountPath, "iPod_Control", "Music", "F00")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("create music directory: %w", err)
	}
	playlistsDir := filepath.Join(deviceMountPath, "Playlists")
	if err := os.MkdirAll(playlistsDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("create playlists directory: %w", err)
	}

	lines := make([]string, 0, len(songs)+1)
	lines = append(lines, "#EXTM3U")
	for i, song := range songs {
		ext := strings.ToLower(filepath.Ext(song.Path))
		if ext == "" {
			ext = "." + strings.ToLower(song.Format)
		}
		if ext == "." || ext == "" {
			ext = ".mp3"
		}
		name := safeFileName(fmt.Sprintf("%03d-%s-%s%s", i+1, song.Artist, song.Title, ext))
		dst := filepath.Join(musicDir, name)
		wrote, err := copyFileIfNeeded(song.Path, dst)
		if err != nil {
			return copied, "", fmt.Errorf("copy %q: %w", song.Path, err)
		}
		if wrote {
			copied++
		}
		rel, err := filepath.Rel(deviceMountPath, dst)
		if err != nil {
			return copied, "", fmt.Errorf("relative playlist path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		lines = append(lines, rel)
	}

	m3uName := safeFileName(playlistName) + ".m3u8"
	m3uPath = filepath.Join(playlistsDir, m3uName)
	if err := writeLines(m3uPath, lines); err != nil {
		return copied, "", fmt.Errorf("write playlist: %w", err)
	}
	return copied, m3uPath, nil
}

func copyFileIfNeeded(src, dst string) (bool, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	if !srcInfo.Mode().IsRegular() {
		return false, fmt.Errorf("source is not a regular file")
	}

	if dstInfo, err := os.Stat(dst); err == nil {
		if dstInfo.Mode().IsRegular() && dstInfo.Size() == srcInfo.Size() {
			return false, nil
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return false, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}

func writeLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

func safeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "untitled"
	}
	repl := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	s = repl.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 90 {
		s = s[:90]
	}
	return s
}
