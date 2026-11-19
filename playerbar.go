package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"musaik/internal/model"
)

const sidebarWidth = 210

type playerBarLayout int

const (
	// playerBarFull has the song, the controls and the volume on one row.
	playerBarFull playerBarLayout = iota
	// playerBarMedium shrinks the song and the volume to keep one row.
	playerBarMedium
	// playerBarCompact puts the progress bar on its own row below the rest.
	playerBarCompact
)

// contentWidth is the width left to the main column, beside the sidebar.
func (a *app) contentWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	if !a.sidebarHidden {
		w -= sidebarWidth
	}
	return w
}

func (a *app) playerBarLayoutFor(c *ui.Context) playerBarLayout {
	w := a.contentWidth(c)
	switch {
	case w <= 0 || w >= 1080:
		return playerBarFull
	case w >= 720:
		return playerBarMedium
	}
	return playerBarCompact
}

func (a *app) viewControls(c *ui.Context) {
	state := a.player.State()
	playIcon := iconPlay
	if state.Playing && !state.Paused {
		playIcon = iconPause
	}

	duration := state.Duration
	position := state.Position
	_, song, hasSong := a.findSong(a.selectedID)
	if duration > 0 {
		a.sliderAt = position.Seconds()
	} else {
		a.sliderAt = 0
	}

	bar := func() ui.Element {
		return ui.Row(c).
			Padding(10, 14).
			Gap(12).
			Background(sonoraToolbar).
			BorderWidth(1, 0, 0, 0).
			BorderColor(sonoraBorder).
			AlignItems(ui.Center)
	}

	switch a.playerBarLayoutFor(c) {
	case playerBarFull:
		bar().Children(func() {
			ui.Row(c).Width(300).Gap(10).AlignItems(ui.Center).Children(func() {
				a.controlsNowPlaying(c, song, hasSong)
			})
			ui.Spacer(c)
			ui.Column(c).Width(470).Gap(5).Children(func() {
				a.controlsTransport(c, playIcon, 16)
				a.controlsProgress(c, position, duration)
			})
			ui.Spacer(c)
			ui.Row(c).Width(220).Gap(8).AlignItems(ui.Center).Children(func() {
				a.controlsVolume(c, -1)
			})
		})
	case playerBarMedium:
		bar().Children(func() {
			ui.Row(c).Width(210).Gap(10).AlignItems(ui.Center).Children(func() {
				a.controlsNowPlaying(c, song, hasSong)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				a.controlsTransport(c, playIcon, 10)
				a.controlsProgress(c, position, duration)
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				a.controlsVolume(c, 64)
			})
		})
	default:
		ui.Column(c).
			Padding(10, 14).
			Gap(8).
			Background(sonoraToolbar).
			BorderWidth(1, 0, 0, 0).
			BorderColor(sonoraBorder).
			Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Grow(1).MinWidth(0).Gap(10).AlignItems(ui.Center).Children(func() {
						a.controlsNowPlaying(c, song, hasSong)
					})
					a.controlsTransport(c, playIcon, 8)
					ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
						a.controlsVolume(c, 0)
					})
				})
				a.controlsProgress(c, position, duration)
			})
	}
}

// controlsNowPlaying draws the cover and the song's title, artist and format.
func (a *app) controlsNowPlaying(c *ui.Context, song model.Song, hasSong bool) {
	coverBtn := ui.Button(c, "").Width(48).Height(48).Padding(0, 0).Radius(8).Background(adaptiveOverlay(c, 0.10, 0.08)).Children(func() {
		if hasSong {
			if bmp := a.coverBitmap(song); bmp != nil {
				ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(8)
				return
			}
		}
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "RAM").FontSize(fs(11)).FontWeight(700).TextColor(ui.Hex("#F2C94C"))
		})
	})
	if coverBtn.Clicked() && hasSong {
		a.navigateToSongInLibrary(song)
	}
	ui.Column(c).Grow(1).MinWidth(0).Clip().Gap(1).Children(func() {
		title := "Sin reproducción"
		subtitle := "Musaik"
		quality := ""
		liked := false
		if hasSong {
			title = song.Title
			subtitle = fmt.Sprintf("%s — %s", song.Artist, song.Album)
			quality = fmt.Sprintf("%s  /  %s", strings.ToUpper(song.Format), "Lossless")
			liked = a.store.Favorites[song.ID]
		}
		ui.Row(c).MinWidth(0).Gap(6).Children(func() {
			// The title is clipped in its own box so a long one never covers
			// the like button or the controls next to the song.
			ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() {
				if hasSong {
					titleBtn := ui.Button(c, "").MinWidth(0).Padding(0, 0).Background(ui.Transparent).TextColor(sonoraText).Children(func() {
						ui.Text(c, title).SingleLine().FontSize(fs(13)).FontWeight(600).TextColor(sonoraText)
					})
					if titleBtn.Clicked() {
						a.navigateToSongInLibrary(song)
					}
				} else {
					ui.Text(c, title).SingleLine().FontSize(fs(13)).FontWeight(600).TextColor(sonoraText)
				}
			})
			likeBtn := ui.Button(c, "").Width(22).Height(22).Padding(0, 0).Radius(11).Background(ui.Transparent).TextColor(sonoraTextMuted)
			if liked {
				likeBtn.TextColor(ui.Hex("#F43F5E"))
			}
			likeBtn.Children(func() {
				ui.Column(c).Fill().Center().Children(func() {
					if liked {
						ui.Text(c, "♥").FontSize(fs(12))
						return
					}
					ui.Text(c, "♡").FontSize(fs(12))
				})
			})
			if likeBtn.Clicked() && hasSong {
				a.toggleFavorite(song)
			}
		})
		if hasSong {
			displayArtist := strings.TrimSpace(song.Artist)
			if displayArtist == "" {
				displayArtist = "Unknown Artist"
			}
			displayAlbum := strings.TrimSpace(song.Album)
			if displayAlbum == "" {
				displayAlbum = "Unknown Album"
			}
			ui.Row(c).MinWidth(0).Clip().Gap(4).Children(func() {
				artistBtn := ui.Button(c, "").MinWidth(0).Padding(0, 0).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
					ui.Text(c, displayArtist).SingleLine().FontSize(fs(12)).TextColor(sonoraTextMuted)
				})
				if artistBtn.Clicked() {
					a.navigateToArtistDetail(song.Artist)
				}
				ui.Text(c, "—").FontSize(fs(12)).TextColor(sonoraTextMuted)
				albumBtn := ui.Button(c, "").MinWidth(0).Padding(0, 0).Background(ui.Transparent).TextColor(sonoraTextMuted).Children(func() {
					ui.Text(c, displayAlbum).SingleLine().FontSize(fs(12)).TextColor(sonoraTextMuted)
				})
				if albumBtn.Clicked() {
					a.navigateToAlbumDetail(song)
				}
			})
		} else {
			ui.Text(c, subtitle).SingleLine().FontSize(fs(12)).TextColor(sonoraTextMuted)
		}
		ui.Text(c, quality).SingleLine().FontSize(fs(11)).TextColor(sonoraTextTertiary)
	})
}

// controlsTransport draws shuffle, previous, play/pause, next and repeat.
func (a *app) controlsTransport(c *ui.Context, playIcon *ui.SVG, gap float32) {
	ui.Row(c).Center().Gap(gap).Children(func() {
		shuffleBG := ui.Transparent
		shuffleColor := sonoraTextMuted
		if a.shuffle {
			shuffleBG = adaptiveOverlay(c, 0.12, 0.10)
			shuffleColor = sonoraAccent
		}
		shuffleBtn := barIconButton(c, 24, shuffleBG, shuffleColor, iconShuffle, 13)
		if shuffleBtn.Clicked() {
			a.shuffle = !a.shuffle
			_ = a.saveSettings()
		}

		prevBtn := barIconButton(c, 24, ui.Transparent, sonoraTextMuted, iconPrev, 13)
		if prevBtn.Clicked() {
			a.playPrevious()
		}

		playBtn := barIconButton(c, 36, sonoraText, sonoraCanvas, playIcon, 14)
		if playBtn.Clicked() {
			a.togglePlayPause()
		}

		nextBtn := barIconButton(c, 24, ui.Transparent, sonoraTextMuted, iconNext, 13)
		if nextBtn.Clicked() {
			a.playNext()
		}

		repeatBG := ui.Transparent
		repeatColor := sonoraTextMuted
		repeatIcon := iconRepeat
		if a.repeatMode != repeatModeOff {
			repeatBG = adaptiveOverlay(c, 0.12, 0.10)
			repeatColor = sonoraAccent
		}
		if a.repeatMode == repeatModeOne {
			repeatIcon = iconRepeat1
		}
		repeatBtn := barIconButton(c, 24, repeatBG, repeatColor, repeatIcon, 13)
		if repeatBtn.Clicked() {
			a.cycleRepeatMode()
			_ = a.saveSettings()
		}
	})
}

// controlsProgress draws the elapsed time, the seek slider and the duration.
func (a *app) controlsProgress(c *ui.Context, position, duration time.Duration) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, formatDuration(position)).Font("monospace").FontSize(fs(11)).TextColor(sonoraTextMuted)
		if duration > 0 {
			if ui.Slider(c.Key("progress"), &a.sliderAt, 0, duration.Seconds()).Step(1).Grow(1).Changed() {
				if err := a.player.SeekTo(time.Duration(a.sliderAt * float64(time.Second))); err != nil {
					a.status = "No se pudo mover la canción: " + err.Error()
				} else {
					a.updateNowPlaying()
				}
			}
		} else {
			ui.Box(c).Grow(1)
		}
		ui.Text(c, formatDuration(duration)).Font("monospace").FontSize(fs(11)).TextColor(sonoraTextMuted)
	})
}

// controlsVolume draws the mini player and mute buttons and, unless slider is
// 0, the volume slider: slider > 0 is its width and slider < 0 makes it grow.
func (a *app) controlsVolume(c *ui.Context, slider float32) {
	lyricsBtn := barIconButton(c, 24, ui.Transparent, lyricsButtonColor(a.lyrics.open), iconLyrics, 13).Tooltip("Letra (⌘L)")
	if lyricsBtn.Clicked() {
		a.toggleLyrics()
	}
	miniBtn := barIconButton(c, 24, ui.Transparent, sonoraTextMuted, iconMiniPlayer, 13).Tooltip("Mini reproductor (⇧⌘M)")
	if miniBtn.Clicked() {
		a.toggleMiniPlayer()
	}
	isMuted := a.volumeLevel <= 0
	muteIcon := iconVolume
	if isMuted {
		muteIcon = iconVolumeOff
	}
	muteBtn := barIconButton(c, 24, ui.Transparent, sonoraTextMuted, muteIcon, 13)
	if muteBtn.Clicked() {
		if isMuted {
			restore := a.volumeBeforeMute
			if restore <= 0 {
				restore = 65
			}
			a.volumeLevel = restore
		} else {
			a.volumeBeforeMute = a.volumeLevel
			a.volumeLevel = 0
		}
		if err := a.player.SetVolume(int(a.volumeLevel)); err != nil {
			a.status = "No se pudo ajustar volumen: " + err.Error()
		}
	}
	if slider == 0 {
		return
	}
	vol := ui.Slider(c.Key("volume"), &a.volumeLevel, 0, 100).Step(1)
	if slider > 0 {
		vol.Width(slider)
	} else {
		vol.Grow(1)
	}
	if vol.Changed() {
		if a.volumeLevel > 0 {
			a.volumeBeforeMute = a.volumeLevel
		}
		if err := a.player.SetVolume(int(a.volumeLevel)); err != nil {
			a.status = "No se pudo ajustar volumen: " + err.Error()
		}
	}
}
