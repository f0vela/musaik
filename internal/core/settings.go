// Package core is the headless engine behind Musaik: playback orchestration,
// queue, favorites, playlists, Last.fm, iPod sync and the remote control API.
// The terminal app drives it; it holds no UI.
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Settings are the preferences shared with the desktop app, stored in the same
// settings.json. Only the keys this struct owns are written back: anything the
// desktop app keeps there (mini player opacity, ...) is left as it is on disk.
type Settings struct {
	Section          string
	Shuffle          bool
	RepeatMode       string
	RootInput        string
	SearchQuery      string
	LastFMAPIKey     string
	LastFMAPISecret  string
	LastFMSessionKey string
	LastFMUsername   string
	IPodMountPath    string
	IPodPlaylistName string
	RemoteControl    bool
	SidebarHidden    bool
}

// SettingsPath is where the desktop and terminal apps keep their preferences.
func SettingsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "musaik")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

type settingsFile struct {
	Section          string `json:"section"`
	Shuffle          bool   `json:"shuffle"`
	Repeat           bool   `json:"repeat"`
	RepeatMode       string `json:"repeat_mode"`
	RootInput        string `json:"root_input"`
	SearchQuery      string `json:"search_query"`
	LastFMAPIKey     string `json:"lastfm_api_key"`
	LastFMAPISecret  string `json:"lastfm_api_secret"`
	LastFMSessionKey string `json:"lastfm_session_key"`
	LastFMUsername   string `json:"lastfm_username"`
	IPodMountPath    string `json:"ipod_mount_path"`
	IPodPlaylistName string `json:"ipod_playlist_name"`
	RemoteControl    bool   `json:"remote_control"`
	SidebarHidden    bool   `json:"sidebar_hidden"`
}

// LoadSettings reads settings.json; a missing file gives the defaults.
func LoadSettings() (*Settings, error) {
	s := &Settings{}
	path, err := SettingsPath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	var f settingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return s, err
	}
	s.Section = f.Section
	s.Shuffle = f.Shuffle
	s.RepeatMode = f.RepeatMode
	if s.RepeatMode == "" && f.Repeat {
		s.RepeatMode = string(RepeatAll)
	}
	s.RootInput = f.RootInput
	s.SearchQuery = f.SearchQuery
	s.LastFMAPIKey = f.LastFMAPIKey
	s.LastFMAPISecret = f.LastFMAPISecret
	s.LastFMSessionKey = f.LastFMSessionKey
	s.LastFMUsername = f.LastFMUsername
	s.IPodMountPath = f.IPodMountPath
	s.IPodPlaylistName = f.IPodPlaylistName
	s.RemoteControl = f.RemoteControl
	s.SidebarHidden = f.SidebarHidden
	return s, nil
}

// Save merges the keys this struct owns into settings.json, re-reading the file
// first so keys written meanwhile by the desktop app survive.
func (s *Settings) Save() error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	doc := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &doc)
	}
	mine, _ := json.Marshal(settingsFile{
		Section:          s.Section,
		Shuffle:          s.Shuffle,
		Repeat:           s.RepeatMode != "" && s.RepeatMode != string(RepeatOff),
		RepeatMode:       s.RepeatMode,
		RootInput:        s.RootInput,
		SearchQuery:      s.SearchQuery,
		LastFMAPIKey:     s.LastFMAPIKey,
		LastFMAPISecret:  s.LastFMAPISecret,
		LastFMSessionKey: s.LastFMSessionKey,
		LastFMUsername:   s.LastFMUsername,
		IPodMountPath:    s.IPodMountPath,
		IPodPlaylistName: s.IPodPlaylistName,
		RemoteControl:    s.RemoteControl,
		SidebarHidden:    s.SidebarHidden,
	})
	owned := map[string]json.RawMessage{}
	_ = json.Unmarshal(mine, &owned)
	for k, v := range owned {
		doc[k] = v
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func trim(s string) string { return strings.TrimSpace(s) }
