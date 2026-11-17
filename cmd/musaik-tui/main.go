// Command musaik-tui is Musaik in the terminal: the desktop app's library,
// playback, queue, playlists, Last.fm, iPod sync and settings, driven from the
// keyboard (or the mouse) with Bubble Tea.
package main

import (
	"flag"
	"fmt"
	"os"

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

	eng := core.New(player.New())
	if *root != "" {
		eng.Settings.RootInput = *root
	}
	t := newTUI(eng, *mini)
	t.setImageProto(detectImages(*images))
	prog := tea.NewProgram(t)
	eng.Post = func(fn func()) { prog.Send(funcMsg(fn)) }

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
