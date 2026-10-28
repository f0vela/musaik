package ipod

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"musaik/internal/model"
)

func TestSyncDeviceWritesITunesDB(t *testing.T) {
	dir := t.TempDir()
	songPath := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(songPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	songs := []model.Song{{
		ID: "s1", Path: songPath, Title: "Canción",
		Artist: "Telecast", Album: "Album",
		TrackNum: 3, Duration: 3 * time.Minute, Format: "mp3", PlayCount: 5,
	}}
	copied, _, err := SyncPlaylist(dir, "Rock", songs)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 {
		t.Fatalf("copied = %d, want 1", copied)
	}

	data, err := os.ReadFile(filepath.Join(dir, "iPod_Control", "iTunes", "iTunesDB"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "mhbd" {
		t.Fatalf("db tag = %q", data[:4])
	}
	total := binary.LittleEndian.Uint32(data[8:])
	if int(total) != len(data) {
		t.Fatalf("mhbd total = %d, file = %d", total, len(data))
	}

	// The location mhod must contain the ':'-separated iPod path in UTF-16.
	u16 := utf16.Encode([]rune(":/iPod_Control/Music/"))
	var needle []byte
	for _, c := range u16 {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], c)
		needle = append(needle, b[:]...)
	}
	if !bytes.Contains(data, needle) {
		t.Fatal("location path not found in UTF-16 in iTunesDB")
	}
}

func TestSyncDeviceSignsHash58(t *testing.T) {
	dir := t.TempDir()
	devDir := filepath.Join(dir, "iPod_Control", "Device")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "SysInfo"),
		[]byte("FirewireGuid: 0x000A270012345678\nModelNumStr: MB147\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	songPath := filepath.Join(dir, "s.mp3")
	if err := os.WriteFile(songPath, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	songs := []model.Song{{ID: "s1", Path: songPath, Title: "T", Artist: "A", Duration: time.Minute, Format: "mp3"}}
	if _, _, err := SyncPlaylist(dir, "P", songs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "iPod_Control", "iTunes", "iTunesDB"))
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint16(data[0x30:]); got != 1 {
		t.Fatalf("hashing_scheme = %d, want 1", got)
	}
	if bytes.Equal(data[0x58:0x6c], make([]byte, 20)) {
		t.Fatal("hash58 signature is zero")
	}

	// Recompute the signature from scratch and compare.
	fwid := []byte{0x00, 0x0A, 0x27, 0x00, 0x12, 0x34, 0x56, 0x78}
	signed := make([]byte, len(data))
	copy(signed, data)
	var b18 [8]byte
	var b32 [20]byte
	copy(b18[:], signed[0x18:0x20])
	copy(b32[:], signed[0x32:0x46])
	for i := 0x18; i < 0x20; i++ {
		signed[i] = 0
	}
	for i := 0x32; i < 0x46; i++ {
		signed[i] = 0
	}
	for i := 0x58; i < 0x6c; i++ {
		signed[i] = 0
	}
	digest := hash58Digest(fwid, signed)
	if !bytes.Equal(data[0x58:0x6c], digest[:]) {
		t.Fatal("hash58 mismatch")
	}
}
