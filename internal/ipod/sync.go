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
	return SyncDevice(deviceMountPath, playlistName, songs, nil)
}

// SyncDevice copies songs to the iPod, writes a real iTunesDB containing a
// Library playlist with all tracks plus the named playlist, and a .m3u8 copy
// for reference. WARNING: it rewrites iTunesDB — tracks previously synced by
// iTunes that are not in `songs` disappear from the iPod menus (files remain
// on the device). A backup of the old database is kept at
// iTunes/iTunesDB.musaik.bak.
// extraPlaylists are additional playlists written into the DB whose songs
// must already be present in `songs`.
func SyncDevice(deviceMountPath string, playlistName string, songs []model.Song, extraPlaylists []PlaylistEntry) (copied int, m3uPath string, err error) {
	if playlistName == "" {
		return 0, "", fmt.Errorf("playlist name is required")
	}
	playlistsDir := filepath.Join(deviceMountPath, "Playlists")
	if err := os.MkdirAll(playlistsDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("create playlists directory: %w", err)
	}

	lines := make([]string, 0, len(songs)+1)
	lines = append(lines, "#EXTM3U")

	trackIDs := make([]uint32, len(songs))
	ipodPaths := map[uint32]string{}
	fileSizes := map[uint32]int64{}
	usedNames := map[string]int{}

	for i, song := range songs {
		ext := strings.ToLower(filepath.Ext(song.Path))
		if ext == "" {
			ext = "." + strings.ToLower(song.Format)
		}
		if ext == "." || ext == "" {
			ext = ".mp3"
		}
		// Location strings must stay under ~56 UTF-16 chars.
		name := safeFileName(fmt.Sprintf("%s-%s%s", song.Artist, song.Title, ext))
		if utf16Len(name) > 50 {
			name = truncateUTF16(name, 50-len(ext)) + ext
		}
		if c := usedNames[name]; c > 0 {
			name = safeFileName(fmt.Sprintf("%s-%s-%d%s", song.Artist, song.Title, c+1, ext))
		}
		usedNames[name]++

		folder := musicFolder(song.ID)
		musicDir := filepath.Join(deviceMountPath, "iPod_Control", "Music", folder)
		if err := os.MkdirAll(musicDir, 0o755); err != nil {
			return copied, "", fmt.Errorf("create music directory: %w", err)
		}
		dst := filepath.Join(musicDir, name)
		wrote, err := copyFileIfNeeded(song.Path, dst)
		if err != nil {
			return copied, "", fmt.Errorf("copy %q: %w", song.Path, err)
		}
		if wrote {
			copied++
		}
		st, err := os.Stat(dst)
		if err != nil {
			return copied, "", fmt.Errorf("stat %q: %w", dst, err)
		}

		trackIDs[i] = uint32(i + 1)
		ipodPaths[trackIDs[i]] = iPodMusicPath(folder, name)
		fileSizes[trackIDs[i]] = st.Size()

		rel, err := filepath.Rel(deviceMountPath, dst)
		if err != nil {
			return copied, "", fmt.Errorf("relative playlist path: %w", err)
		}
		lines = append(lines, filepath.ToSlash(rel))
	}

	m3uName := safeFileName(playlistName) + ".m3u8"
	m3uPath = filepath.Join(playlistsDir, m3uName)
	if err := writeLines(m3uPath, lines); err != nil {
		return copied, "", fmt.Errorf("write playlist: %w", err)
	}

	pls := []PlaylistEntry{{Name: playlistName, Tracks: trackIDs}}
	pls = append(pls, extraPlaylists...)
	if err := WriteITunesDB(deviceMountPath, songs, trackIDs, ipodPaths, fileSizes, pls); err != nil {
		return copied, "", fmt.Errorf("write iTunesDB: %w", err)
	}
	return copied, m3uPath, nil
}

func utf16Len(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func truncateUTF16(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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
