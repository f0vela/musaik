package ipod

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"musaik/internal/model"
)

// iTunesDB writer for hard-drive iPods (5g Video through Classic 7g).
// Format documented at ipodlinux.org/ITunesDB. Classic/Nano devices also
// need the hash58 signature, applied here when the FireWire GUID is found.

const (
	dbVersion      = 0x19 // iTunes 7.4; required for hash58-signed devices
	macEpochOffset = 2082844800
	mhbdHeaderSize = 0xBC
	mhsdHeaderSize = 0x60
	mhltHeaderSize = 0x5c
	mhlpHeaderSize = 0x5c
	mhitHeaderSize = 0x184
	mhypHeaderSize = 0x6c
	mhipHeaderSize = 0x24
	mhodHeaderSize = 0x18
	mhodStringLen  = 40 // header + string fields before data
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

type chunkWriter struct{ bytes.Buffer }

func (w *chunkWriter) u8(v uint8)   { w.WriteByte(v) }
func (w *chunkWriter) u16(v uint16) { binary.Write(w, binary.LittleEndian, v) }
func (w *chunkWriter) u32(v uint32) { binary.Write(w, binary.LittleEndian, v) }
func (w *chunkWriter) u64(v uint64) { binary.Write(w, binary.LittleEndian, v) }
func (w *chunkWriter) f32(v float32) {
	binary.Write(w, binary.LittleEndian, v)
}
func (w *chunkWriter) tag(t string) { w.WriteString(t) }
func (w *chunkWriter) pad(n int)    { w.Write(make([]byte, n)) }
func (w *chunkWriter) fixU32(off int, v uint32) {
	binary.LittleEndian.PutUint32(w.Bytes()[off:], v)
}

func macTime(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	return uint32(t.Unix() + macEpochOffset)
}

func utf16Bytes(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	return b
}

// mhodString writes a string-type mhod (types < 15). position must be 1;
// location mhods (type 2) will not play on the iPod if it is zero.
func mhodString(mhodType uint32, s string) []byte {
	data := utf16Bytes(s)
	var w chunkWriter
	w.tag("mhod")
	w.u32(mhodHeaderSize)
	w.u32(mhodStringLen + uint32(len(data)))
	w.u32(mhodType)
	w.u32(0)
	w.u32(0)
	w.u32(1) // position
	w.u32(uint32(len(data)))
	w.u32(1) // encoding flag: recent iTunesDBs use 1 even for UTF-16
	w.u32(0)
	w.Write(data)
	return w.Bytes()
}

// mhodPosition writes the type-100 position indicator that follows each mhip.
func mhodPosition(pos uint32) []byte {
	var w chunkWriter
	w.tag("mhod")
	w.u32(mhodHeaderSize)
	w.u32(mhodStringLen)
	w.u32(100)
	w.u32(0)
	w.u32(0)
	w.u32(pos)
	w.pad(mhodStringLen - mhodHeaderSize - 4)
	return w.Bytes()
}

var filetypeMap = map[string][4]byte{
	".mp3":  {'M', 'P', '3', ' '},
	".m4a":  {'M', '4', 'A', ' '},
	".m4b":  {'M', '4', 'B', ' '},
	".m4p":  {'M', '4', 'P', ' '},
	".m4v":  {'M', '4', 'V', ' '},
	".aac":  {'A', 'A', 'C', ' '},
	".wav":  {'W', 'A', 'V', 'E'},
	".aif":  {'A', 'I', 'F', ' '},
	".aiff": {'A', 'I', 'F', ' '},
	".alac": {'A', 'L', 'A', 'C'},
}

func filetypeFor(path string) [4]byte {
	if ft, ok := filetypeMap[strings.ToLower(filepath.Ext(path))]; ok {
		return ft
	}
	return [4]byte{'M', 'P', '3', ' '}
}

// mhit writes one Track Item with its child string mhods.
// ipodPath is the device path in ':' notation (e.g. :/iPod_Control/Music/F00/x.mp3).
func mhit(trackID uint32, song model.Song, ipodPath string, fileSize int64) []byte {
	var w chunkWriter
	w.tag("mhit")
	w.u32(mhitHeaderSize)
	totalOff := w.Len()
	w.u32(0) // total size, patched at end
	w.u32(uint32(mhodCount(song)))
	w.u32(trackID) // unique id (referenced by mhip)
	w.u32(1)       // visible
	ft := filetypeFor(song.Path)
	w.Write(ft[:]) // filetype
	ext := strings.ToLower(filepath.Ext(song.Path))
	isAAC := ext == ".m4a" || ext == ".m4b" || ext == ".m4p" || ext == ".aac" || ext == ".alac"
	isWave := ext == ".wav" || ext == ".aif" || ext == ".aiff"
	if isAAC {
		w.u8(0)
		w.u8(0)
	} else {
		w.u8(0)
		w.u8(1)
	}
	w.u8(0)                                         // compilation
	w.u8(0)                                         // rating
	w.u32(macTime(time.Now()))                      // last modified
	w.u32(uint32(fileSize))                         // size
	w.u32(uint32(song.Duration / time.Millisecond)) // length ms
	w.u32(uint32(song.TrackNum))
	w.u32(0) // total tracks
	w.u32(0) // year
	bitrate := 0
	if secs := song.Duration.Seconds(); secs > 0 && fileSize > 0 {
		bitrate = int(float64(fileSize) * 8 / secs / 1000)
	}
	w.u32(uint32(bitrate))
	const sampleRate = 44100
	w.u32(sampleRate << 16) // sample rate fixed point
	w.u32(0)                // volume adjust
	w.u32(0)                // start time
	w.u32(0)                // stop time (0 = play to end)
	w.u32(0)                // soundcheck
	w.u32(uint32(song.PlayCount))
	w.u32(uint32(song.PlayCount))
	w.u32(macTime(song.LastPlayedAt))
	w.u32(0)                   // disc number
	w.u32(0)                   // total discs
	w.u32(0)                   // user id
	w.u32(macTime(time.Now())) // date added
	w.u32(0)                   // bookmark time
	dbid := rng.Uint64()
	w.u64(dbid) // dbid
	w.u8(0)     // checked
	w.u8(0)     // application rating
	w.u16(0)    // BPM
	w.u16(0)    // artwork count
	if isWave {
		w.u16(0)
	} else {
		w.u16(0xffff) // unk9
	}
	w.u32(0) // artwork size
	w.u32(0) // unk11
	w.f32(sampleRate)
	w.u32(macTime(time.Now())) // date released
	switch {
	case isAAC:
		w.u16(0x0033)
		w.u16(1)
	case isWave:
		w.u16(0)
		w.u16(0)
	default:
		w.u16(0x000c) // MPEG-1 Layer 3
		w.u16(0)
	}
	w.u32(0)    // unk15
	w.u32(0)    // unk16
	w.u32(0)    // skip count
	w.u32(0)    // last skipped
	w.u8(2)     // has_artwork = 0x02 (none)
	w.u8(0)     // skip when shuffling
	w.u8(0)     // remember position
	w.u8(0)     // flag4 / podcast
	w.u64(dbid) // dbid2
	w.u8(0)     // lyrics flag
	w.u8(0)     // movie flag
	w.u8(1)     // played_mark (0x01 for audio)
	w.u8(0)     // unk17
	w.u32(0)    // unk21
	w.u32(0)    // pregap
	w.u64(0)    // sample count
	w.u32(0)    // unk25
	w.u32(0)    // postgap
	if !isAAC && !isWave {
		w.u32(1) // unk27: 1 for mp3-encoder files
	} else {
		w.u32(0)
	}
	w.u32(1) // media type: 1 = audio
	w.pad(mhitHeaderSize - w.Len())

	// String mhods: location first (required), then title/album/artist/filetype.
	w.Write(mhodString(2, ipodPath))
	w.Write(mhodString(1, song.Title))
	if song.Album != "" {
		w.Write(mhodString(3, song.Album))
	}
	if song.Artist != "" {
		w.Write(mhodString(4, song.Artist))
	}
	w.Write(mhodString(6, song.Format))

	w.fixU32(totalOff, uint32(w.Len()))
	return w.Bytes()
}

func mhodCount(song model.Song) int {
	n := 3 // location + title + filetype
	if song.Album != "" {
		n++
	}
	if song.Artist != "" {
		n++
	}
	return n
}

// mhip writes a Playlist Item referencing a track.
func mhip(trackID uint32) []byte {
	var w chunkWriter
	w.tag("mhip")
	w.u32(mhipHeaderSize)
	w.u32(mhipHeaderSize + mhodStringLen) // total: header + position mhod
	w.u32(1)                              // mhod children
	w.u16(0)                              // podcast grouping flag
	w.u8(0)
	w.u8(0)
	w.u32(0)       // group id
	w.u32(trackID) // track id
	w.u32(macTime(time.Now()))
	w.u32(0) // podcast grouping reference
	w.Write(mhodPosition(0))
	return w.Bytes()
}

// mhodIndex52 writes a Library Playlist Index mhod (type 52): the positions
// of all mhits sorted by one browse field, used by the iPod's Browse menus.
func mhodIndex52(indexType uint32, order []int) []byte {
	var w chunkWriter
	w.tag("mhod")
	w.u32(mhodHeaderSize)
	w.u32(72 + uint32(len(order))*4)
	w.u32(52)
	w.u32(0)
	w.u32(0)
	w.u32(indexType)
	w.u32(uint32(len(order)))
	w.pad(40)
	for _, i := range order {
		w.u32(uint32(i))
	}
	return w.Bytes()
}

func sortByField(songs []model.Song, key func(model.Song) string) []int {
	idx := make([]int, len(songs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return key(songs[idx[a]]) < key(songs[idx[b]])
	})
	return idx
}

// mhyp writes a Playlist with its name mhod and mhip items.
func mhyp(name string, trackIDs []uint32, master bool, songs []model.Song) []byte {
	var w chunkWriter
	w.tag("mhyp")
	w.u32(mhypHeaderSize)
	totalOff := w.Len()
	w.u32(0)
	mhodCount := 1 // name mhod
	if master {
		mhodCount += 3 // type-52 indexes: title, album, artist
	}
	w.u32(uint32(mhodCount))
	w.u32(uint32(len(trackIDs)))
	if master {
		w.u8(1)
	} else {
		w.u8(0)
	}
	w.pad(3)
	w.u32(macTime(time.Now()))
	w.u64(rng.Uint64())
	w.u32(0)
	w.u16(1) // string mhod count
	w.u16(0) // podcast flag
	w.u32(1) // sort order: playlist order
	w.pad(mhypHeaderSize - w.Len())

	w.Write(mhodString(1, name))
	if master {
		w.Write(mhodIndex52(0x03, sortByField(songs, func(s model.Song) string { return s.Title })))
		w.Write(mhodIndex52(0x04, sortByField(songs, func(s model.Song) string { return s.Album })))
		w.Write(mhodIndex52(0x05, sortByField(songs, func(s model.Song) string { return s.Artist })))
	}
	for _, id := range trackIDs {
		w.Write(mhip(id))
	}
	w.fixU32(totalOff, uint32(w.Len()))
	return w.Bytes()
}

// PlaylistEntry pairs a playlist name with the tracks it contains.
type PlaylistEntry struct {
	Name   string
	Tracks []uint32
}

// WriteITunesDB builds and writes iPod_Control/iTunes/iTunesDB.
// ipodPaths maps each trackID to its device path in ':' notation.
func WriteITunesDB(mountPath string, songs []model.Song, trackIDs []uint32, ipodPaths map[uint32]string, fileSizes map[uint32]int64, playlists []PlaylistEntry) error {
	var tracks chunkWriter
	tracks.tag("mhlt")
	tracks.u32(mhltHeaderSize)
	tracks.u32(uint32(len(songs)))
	tracks.pad(mhltHeaderSize - tracks.Len())
	for i, song := range songs {
		tracks.Write(mhit(trackIDs[i], song, ipodPaths[trackIDs[i]], fileSizes[trackIDs[i]]))
	}
	var dsTracks chunkWriter
	dsTracks.tag("mhsd")
	dsTracks.u32(mhsdHeaderSize)
	dsTracks.u32(mhsdHeaderSize + uint32(tracks.Len()))
	dsTracks.u32(1) // track list
	dsTracks.pad(mhsdHeaderSize - dsTracks.Len())
	dsTracks.Write(tracks.Bytes())

	all := make([]uint32, len(trackIDs))
	copy(all, trackIDs)
	var plist chunkWriter
	plist.tag("mhlp")
	plist.u32(mhlpHeaderSize)
	plist.u32(uint32(len(playlists) + 1)) // + library playlist
	plist.pad(mhlpHeaderSize - plist.Len())
	plist.Write(mhyp("iPod", all, true, songs))
	for _, pl := range playlists {
		plist.Write(mhyp(pl.Name, pl.Tracks, false, nil))
	}
	var dsPlist chunkWriter
	dsPlist.tag("mhsd")
	dsPlist.u32(mhsdHeaderSize)
	dsPlist.u32(mhsdHeaderSize + uint32(plist.Len()))
	dsPlist.u32(2) // playlist list
	dsPlist.pad(mhsdHeaderSize - dsPlist.Len())
	dsPlist.Write(plist.Bytes())

	total := mhbdHeaderSize + dsTracks.Len() + dsPlist.Len()
	var db chunkWriter
	db.tag("mhbd")
	db.u32(mhbdHeaderSize)
	db.u32(uint32(total))
	db.u32(1) // unk1
	db.u32(dbVersion)
	db.u32(2)            // children: tracks + playlists
	db.u64(rng.Uint64()) // db_id @0x18
	db.u16(2)            // platform @0x20
	db.u16(0)            // unk_0x22
	db.u64(rng.Uint64()) // id_0x24
	db.u32(0)            // unk_0x2c
	db.u16(0)            // hashing_scheme @0x30 (set to 1 when signing)
	db.pad(20)           // unk_0x32
	db.WriteByte('e')
	db.WriteByte('n')    // language @0x46
	db.u64(rng.Uint64()) // db_persistent_id @0x48
	db.u32(0)            // unk_0x50
	db.u32(0)            // unk_0x54
	db.pad(20)           // hash58 @0x58 (filled when signing)
	db.pad(mhbdHeaderSize - db.Len())
	db.Write(dsTracks.Bytes())
	db.Write(dsPlist.Bytes())

	if fwid := FirewireGUID(mountPath); fwid != nil {
		signHash58(db.Bytes(), fwid)
	}

	itunesDir := filepath.Join(mountPath, "iPod_Control", "iTunes")
	if err := os.MkdirAll(itunesDir, 0o755); err != nil {
		return err
	}
	dbPath := filepath.Join(itunesDir, "iTunesDB")
	if st, err := os.Stat(dbPath); err == nil && st.Size() > 0 {
		if err := os.Rename(dbPath, dbPath+".musaik.bak"); err != nil {
			return fmt.Errorf("backup iTunesDB: %w", err)
		}
	}
	tmp := dbPath + ".tmp"
	if err := os.WriteFile(tmp, db.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dbPath)
}

// iPodMusicPath returns the device path in ':' notation for a file under
// iPod_Control/Music/<folder>/<name>.
func iPodMusicPath(folder, name string) string {
	return ":/iPod_Control/Music/" + folder + "/" + name
}

func musicFolder(id string) string {
	return fmt.Sprintf("F%02d", crc32.ChecksumIEEE([]byte(id))%50)
}
