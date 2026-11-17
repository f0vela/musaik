// Command musaik-tui is Musaik in the terminal: the desktop app's library,
// playback, queue, playlists, Last.fm, iPod sync and settings, driven from the
// keyboard (or the mouse) with Bubble Tea.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"musaik/internal/core"
	"musaik/internal/player"
)

func main() {
	mini := flag.Bool("mini", false, "start in mini player mode")
	root := flag.String("root", "", "music folder to index (default: the saved one)")
	images := flag.String("images", os.Getenv("MUSAIK_IMAGES"), "how covers are drawn: auto, kitty or blocks (default auto: kitty graphics in kitty and Ghostty)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("musaik-tui", version)
		return
	}

	if path := os.Getenv("MUSAIK_HEAP_PROFILE"); path != "" {
		// A heap profile on demand, to find where memory goes: kill -USR1 <pid>.
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGUSR1)
		go func() {
			for range sigs {
				runtime.GC()
				if f, err := os.Create(path); err == nil {
					_ = pprof.WriteHeapProfile(f)
					f.Close()
				}
			}
		}()
	}

	eng := core.New(player.New())
	if *root != "" {
		eng.Settings.RootInput = *root
	}
	t := newTUI(eng, *mini)
	t.setImageProto(detectImages(*images))
	t.cacheViews = true
	// 30 frames a second is plenty for a terminal, and when a key is held down the
	// renderer coalesces the updates in between instead of drawing every one.
	prog := tea.NewProgram(t, tea.WithFPS(30))
	eng.Post = func(fn func()) { prog.Send(funcMsg(fn)) }
	// The player reports when a track ends; waiting for it here means the next
	// song starts at once and the app does not have to ask on a timer.
	go func() {
		for ev := range eng.Player.Events() {
			prog.Send(playerEventMsg(ev))
		}
	}()

	if eng.Settings.RemoteControl {
		t.setRemote(true)
	}
	if len(eng.Store.Songs) == 0 || *root != "" {
		if root := eng.Settings.RootInput; root != "" {
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				eng.IndexAsync(root, func(error) { t.rowsDirty = true })
			}
		}
	}

	_, err := prog.Run()
	if t.proto == protoKitty {
		fmt.Print(kittyDeleteAll()) // no image outlives the program
	}
	t.remote.Close()
	eng.Player.Stop()
	eng.FlushIfDirty(true)
	_ = eng.Persist()
	if err != nil {
		fmt.Fprintln(os.Stderr, "musaik-tui:", err)
		os.Exit(1)
	}
}
