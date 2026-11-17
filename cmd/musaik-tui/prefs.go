package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"musaik/internal/core"
)

// tuiPrefs are the preferences only the terminal app has. They live in their
// own file because the desktop app rewrites settings.json from the keys it
// knows and would drop one it does not.
type tuiPrefs struct {
	Theme string `json:"theme"`
}

func prefsPath() (string, error) {
	sp, err := core.SettingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(sp), "tui.json"), nil
}

func loadPrefs() tuiPrefs {
	p := tuiPrefs{Theme: themeAuto}
	path, err := prefsPath()
	if err != nil {
		return p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	_ = json.Unmarshal(data, &p)
	if themeDefByID(p.Theme).id != p.Theme {
		p.Theme = themeAuto
	}
	return p
}

func (p tuiPrefs) save() error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
