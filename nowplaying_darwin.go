//go:build darwin

package main

import (
	"log"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/dhowden/tag"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"musaik/internal/model"
)

// Now Playing integration with macOS through MPNowPlayingInfoCenter and
// MPRemoteCommandCenter, driven over the Objective-C runtime with purego
// (the same approach MyGo uses internally). Registering the remote command
// center is also what makes the keyboard media keys reach the app.

const (
	npPlaybackStateStopped = 0
	npPlaybackStatePlaying = 1
	npPlaybackStatePaused  = 2
)

// CGSize mirrors CoreGraphics' CGSize; the type name matters because the
// Objective-C block signature is derived from it ({CGSize=dd}).
type CGSize struct{ Width, Height float64 }

type npArtworkEntry struct {
	artwork objc.ID
	block   objc.Block
}

type npState struct {
	once      sync.Once
	available bool
	center    objc.ID
	blocks    []objc.Block
	artworks  map[string]npArtworkEntry
	sels      map[string]objc.SEL
	classes   map[string]objc.ID
	poolPush  func() uintptr
	poolPop   func(uintptr)
}

var np = &npState{
	artworks: map[string]npArtworkEntry{},
	sels:     map[string]objc.SEL{},
	classes:  map[string]objc.ID{},
}

func npSel(name string) objc.SEL {
	if s, ok := np.sels[name]; ok {
		return s
	}
	s := objc.RegisterName(name)
	np.sels[name] = s
	return s
}

func npClass(name string) objc.ID {
	if c, ok := np.classes[name]; ok {
		return c
	}
	c := objc.ID(objc.GetClass(name))
	np.classes[name] = c
	return c
}

func npSend[T any](receiver objc.ID, selector string, args ...any) T {
	return objc.Send[T](receiver, npSel(selector), args...)
}

func nsString(s string) objc.ID {
	return npSend[objc.ID](npClass("NSString"), "stringWithUTF8String:", s)
}

func nsNumberDouble(v float64) objc.ID {
	return npSend[objc.ID](npClass("NSNumber"), "numberWithDouble:", v)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nsNumberInt(v int) objc.ID {
	return npSend[objc.ID](npClass("NSNumber"), "numberWithInteger:", v)
}

// withPool runs fn inside an autorelease pool so autoreleased objects created
// on goroutines without one are released.
func withPool(fn func()) {
	if np.poolPush == nil {
		fn()
		return
	}
	pool := np.poolPush()
	defer np.poolPop(pool)
	fn()
}

func npInit() {
	np.once.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				np.available = false
			}
		}()
		libObjC, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err == nil {
			purego.RegisterLibFunc(&np.poolPush, libObjC, "objc_autoreleasePoolPush")
			purego.RegisterLibFunc(&np.poolPop, libObjC, "objc_autoreleasePoolPop")
		}
		if _, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
			return
		}
		center := npSend[objc.ID](npClass("MPNowPlayingInfoCenter"), "defaultCenter")
		if center == 0 {
			return
		}
		np.center = center
		np.available = true
	})
}

// setupNowPlaying registers MPRemoteCommandCenter handlers so the OS media
// controls (keyboard media keys, Control Center, lock screen) drive the app.
// It returns true when macOS now-playing integration is active.
func (a *app) setupNowPlaying() bool {
	npInit()
	if !np.available {
		return false
	}
	withPool(func() {
		rcc := npSend[objc.ID](npClass("MPRemoteCommandCenter"), "sharedCommandCenter")
		bind := func(prop string, fn func()) {
			cmd := npSend[objc.ID](rcc, prop)
			npSend[objc.ID](cmd, "setEnabled:", true)
			blk := objc.NewBlock(func(_ objc.Block, _ objc.ID) int {
				a.updateUI(fn)
				return 0 // MPRemoteCommandHandlerStatusSuccess
			})
			np.blocks = append(np.blocks, blk)
			npSend[objc.ID](cmd, "addTargetWithHandler:", blk)
		}
		bind("togglePlayPauseCommand", a.togglePlayPause)
		bind("playCommand", func() {
			state := a.player.State()
			if state.Paused {
				_ = a.player.TogglePause()
			} else if !state.Playing {
				a.playSelectedOrFirst()
			}
		})
		bind("pauseCommand", func() {
			state := a.player.State()
			if state.Playing && !state.Paused {
				_ = a.player.TogglePause()
			}
		})
		bind("nextTrackCommand", a.playNext)
		bind("previousTrackCommand", a.playPrevious)

		seekCmd := npSend[objc.ID](rcc, "changePlaybackPositionCommand")
		npSend[objc.ID](seekCmd, "setEnabled:", true)
		seekBlk := objc.NewBlock(func(_ objc.Block, ev objc.ID) int {
			pos := npSend[float64](ev, "positionTime")
			a.updateUI(func() {
				if err := a.player.SeekTo(time.Duration(pos * float64(time.Second))); err == nil {
					a.sliderAt = pos
					a.updateNowPlaying()
				}
			})
			return 0
		})
		np.blocks = append(np.blocks, seekBlk)
		npSend[objc.ID](seekCmd, "addTargetWithHandler:", seekBlk)

		npSend[objc.ID](np.center, "setPlaybackState:", npPlaybackStateStopped)
	})
	log.Printf("nowplaying: remote commands registered")
	return true
}

func embeddedCoverData(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	meta, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}
	pic := meta.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil
	}
	return pic.Data
}

// npArtworkFor returns an MPMediaItemArtwork for the song's embedded cover,
// cached per album so the artwork object (and its handler block) stays alive.
func npArtworkFor(song model.Song) objc.ID {
	key := songCoverKey(song)
	if entry, ok := np.artworks[key]; ok {
		return entry.artwork
	}
	data := embeddedCoverData(song.Path)
	if len(data) == 0 {
		data, _ = os.ReadFile("assets/icono_musaik.png")
	}
	if len(data) == 0 {
		np.artworks[key] = npArtworkEntry{}
		return 0
	}
	img := npSend[objc.ID](npSend[objc.ID](npClass("NSImage"), "alloc"), "initWithData:",
		npSend[objc.ID](npClass("NSData"), "dataWithBytes:length:",
			uintptr(unsafe.Pointer(&data[0])), len(data)))
	if img == 0 {
		np.artworks[key] = npArtworkEntry{}
		return 0
	}
	blk := objc.NewBlock(func(_ objc.Block, _ CGSize) objc.ID { return img })
	artwork := npSend[objc.ID](npSend[objc.ID](npClass("MPMediaItemArtwork"), "alloc"),
		"initWithBoundsSize:requestHandler:", CGSize{Width: 512, Height: 512}, blk)
	np.artworks[key] = npArtworkEntry{artwork: artwork, block: blk}
	return artwork
}

// updateNowPlaying publishes the current track and playback state to macOS.
// Call it on the main thread whenever the track, pause state or position changes.
func (a *app) updateNowPlaying() {
	if !np.available {
		return
	}
	song, hasSong := a.currentPlayingSong()
	state := a.player.State()
	withPool(func() {
		if !hasSong || !state.Playing {
			npSend[objc.ID](np.center, "setNowPlayingInfo:", objc.ID(0))
			npSend[objc.ID](np.center, "setPlaybackState:", npPlaybackStateStopped)
			return
		}
		dict := npSend[objc.ID](npClass("NSMutableDictionary"), "dictionary")
		set := func(value objc.ID, key string) {
			npSend[objc.ID](dict, "setObject:forKey:", value, nsString(key))
		}
		// Keys are the constant *values* (e.g. MPMediaItemPropertyTitle is the
		// string "title"), not the constant names — mediaremoted only maps the
		// real values.
		set(nsString(song.Title), "title")
		if song.Artist != "" {
			set(nsString(song.Artist), "artist")
		}
		if song.Album != "" {
			set(nsString(song.Album), "albumTitle")
		}
		set(nsNumberInt(1), "mediaType")
		if song.ID != "" {
			set(nsString(song.ID), "persistentID")
		}
		duration := state.Duration
		if duration <= 0 && song.Duration > 0 {
			duration = song.Duration
		}
		set(nsNumberDouble(duration.Seconds()), "playbackDuration")
		set(nsNumberDouble(state.Position.Seconds()), "elapsedPlaybackTime")
		rate := 0.0
		playback := npPlaybackStatePaused
		if !state.Paused {
			rate = 1.0
			playback = npPlaybackStatePlaying
		}
		set(nsNumberDouble(rate), "playbackRate")
		set(nsNumberDouble(1.0), "defaultPlaybackRate")
		set(nsNumberInt(boolInt(duration <= 0)), "isLiveStream")
		if artwork := npArtworkFor(song); artwork != 0 {
			set(artwork, "artwork")
		}
		npSend[objc.ID](np.center, "setNowPlayingInfo:", dict)
		npSend[objc.ID](np.center, "setPlaybackState:", playback)
		if os.Getenv("MUSAIK_DEBUG_NP") != "" {
			info := npSend[objc.ID](np.center, "nowPlayingInfo")
			log.Printf("nowplaying: info=%v count=%d state=%d title=%q", info,
				npSend[int](info, "count"), playback, song.Title)
		}
	})
}

// clearNowPlaying removes Musaik from the system Now Playing UI.
func (a *app) clearNowPlaying() {
	if !np.available {
		return
	}
	withPool(func() {
		npSend[objc.ID](np.center, "setNowPlayingInfo:", objc.ID(0))
		npSend[objc.ID](np.center, "setPlaybackState:", npPlaybackStateStopped)
	})
}
