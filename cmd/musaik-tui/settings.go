package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var settingsTabs = []string{"General", "Apariencia", "Biblioteca", "Integraciones", "Acerca de"}

type settingsState struct {
	tab     int
	idx     int
	editing bool
	inputs  map[string]*field
}

type sfield struct {
	id    string
	kind  string // info, head, input, toggle, select, button
	label string
	text  string
}

func (t *tui) initSettings() {
	s := t.eng.Settings
	mk := func(v string, mask bool) *field {
		f := &field{mask: mask}
		f.Set(v)
		return f
	}
	t.set.inputs = map[string]*field{
		"root":       mk(s.RootInput, false),
		"lf_user":    mk(s.LastFMUsername, false),
		"lf_key":     mk(s.LastFMAPIKey, false),
		"lf_secret":  mk(s.LastFMAPISecret, true),
		"lf_session": mk(s.LastFMSessionKey, true),
		"ipod_mount": mk(s.IPodMountPath, false),
	}
	t.set.editing = false
	t.set.idx = 0
}

// applyInput copies an edited field into the settings, as the desktop app's
// bound text inputs do.
func (t *tui) applyInput(id string) {
	s := t.eng.Settings
	v := t.set.inputs[id].Value()
	switch id {
	case "root":
		s.RootInput = v
	case "lf_user":
		s.LastFMUsername = v
	case "lf_key":
		s.LastFMAPIKey = v
	case "lf_secret":
		s.LastFMAPISecret = v
	case "lf_session":
		s.LastFMSessionKey = v
	case "ipod_mount":
		s.IPodMountPath = v
	}
}

func (t *tui) settingsFields() []sfield {
	e := t.eng
	s := e.Settings
	switch t.set.tab {
	case 0:
		return []sfield{
			{kind: "head", label: "General"},
			{kind: "info", text: "ctrl+p comandos · , ajustes · b barra lateral · M mini reproductor · ? todos los atajos"},
			{kind: "head", label: "Library status"},
			{kind: "info", text: fmt.Sprintf("%d canciones indexadas", len(e.Store.Songs))},
			{kind: "info", text: t.stats.size},
		}
	case 1:
		mode := "Light"
		if t.th.dark {
			mode = "Dark"
		}
		sidebar := "visible"
		if s.SidebarHidden {
			sidebar = "oculta"
		}
		return []sfield{
			{kind: "head", label: "Appearance"},
			{kind: "info", text: fmt.Sprintf("Theme mode: %s (sigue el fondo de la terminal)", mode)},
			{kind: "toggle", id: "sidebar", label: "Barra lateral", text: sidebar},
			{kind: "head", label: "Mini reproductor"},
			{kind: "info", text: "Tecla M: tarjeta compacta con los colores de la carátula. La opacidad es cosa de la ventana de la app de escritorio; aquí la terminal manda."},
		}
	case 2:
		return []sfield{
			{kind: "head", label: "Discover & Scanning"},
			{kind: "info", text: "Selecciona la carpeta raíz e indexa cambios de tu colección."},
			{kind: "input", id: "root", label: "Carpeta raíz"},
			{kind: "button", id: "refresh", label: "Force Refresh"},
			{kind: "button", id: "save_state", label: "Guardar estado"},
			{kind: "head", label: "Watched folders"},
			{kind: "info", text: fmt.Sprintf("%s · %d tracks", strings.TrimSpace(s.RootInput), len(e.Store.Songs))},
		}
	case 3:
		remote := "desactivado"
		if s.RemoteControl {
			remote = "activado"
		}
		out := []sfield{
			{kind: "head", label: "Last.fm"},
			{kind: "info", text: "Now playing, scrobble y sincronización de reproducciones."},
			{kind: "input", id: "lf_user", label: "Usuario"},
			{kind: "input", id: "lf_key", label: "API key"},
			{kind: "input", id: "lf_secret", label: "Shared secret"},
			{kind: "input", id: "lf_session", label: "Session key"},
			{kind: "button", id: "lf_save", label: "Guardar credenciales"},
			{kind: "button", id: "lf_connect", label: "Conectar automáticamente"},
			{kind: "head", label: "Control externo"},
			{kind: "info", text: "API local en 127.0.0.1 para controlar Musaik desde Raycast. Desactivada por defecto."},
			{kind: "toggle", id: "remote", label: "Permitir control externo", text: remote},
		}
		if t.remote != nil {
			out = append(out, sfield{kind: "info", text: fmt.Sprintf("Activo en 127.0.0.1:%d", t.remote.Port)})
		}
		return append(out,
			sfield{kind: "head", label: "iPod Classic"},
			sfield{kind: "input", id: "ipod_mount", label: "Ruta de montaje iPod"},
			sfield{kind: "select", id: "ipod_playlist", label: "Playlist a sincronizar", text: s.IPodPlaylistName},
			sfield{kind: "button", id: "ipod_detect", label: "Detectar iPod"},
			sfield{kind: "button", id: "ipod_sync", label: "Sincronizar playlist"},
		)
	}
	return []sfield{
		{kind: "head", label: "About Musaik"},
		{kind: "info", text: "Musaik TUI " + version},
		{kind: "info", text: "Reproductor local Hi-Res para la terminal: misma biblioteca, playlists, queue y ajustes que la app de escritorio."},
	}
}

func selectableFields(fs []sfield) []int {
	var out []int
	for i, f := range fs {
		if f.kind != "info" && f.kind != "head" {
			out = append(out, i)
		}
	}
	return out
}

// settingsKey handles the keys of the settings view; it reports whether it used one.
func (t *tui) settingsKey(s string) bool {
	st := &t.set
	fs := t.settingsFields()
	sel := selectableFields(fs)
	var cur *sfield
	if len(sel) > 0 {
		st.idx = min(max(st.idx, 0), len(sel)-1)
		cur = &fs[sel[st.idx]]
	}
	switch s {
	case "esc":
		t.closeSettings()
	case "up", "k":
		st.idx = max(st.idx-1, 0)
	case "down", "j":
		st.idx = min(st.idx+1, max(len(sel)-1, 0))
	case "]":
		t.settingsTab(1)
	case "[":
		t.settingsTab(-1)
	case "left", "right":
		if cur != nil && cur.kind == "select" {
			t.cycleSelect(cur.id, map[string]int{"left": -1, "right": 1}[s])
			return true
		}
		t.settingsTab(map[string]int{"left": -1, "right": 1}[s])
	case "enter", "space":
		if cur == nil {
			return s == "enter"
		}
		if s == "space" && cur.kind == "input" {
			return false
		}
		t.activateField(*cur)
	default:
		return false
	}
	return true
}

func (t *tui) settingsTab(d int) {
	t.set.tab = (t.set.tab + d + len(settingsTabs)) % len(settingsTabs)
	t.set.idx = 0
}

func (t *tui) cycleSelect(id string, d int) {
	if id != "ipod_playlist" {
		return
	}
	opts := t.eng.PlaylistOptions()
	cur := 0
	for i, o := range opts {
		if o == t.eng.Settings.IPodPlaylistName {
			cur = i
		}
	}
	t.eng.Settings.IPodPlaylistName = opts[(cur+d+len(opts))%len(opts)]
	_ = t.eng.SaveSettings()
}

func (t *tui) activateField(f sfield) {
	e := t.eng
	switch f.kind {
	case "input":
		t.set.editing = true
	case "select":
		t.cycleSelect(f.id, 1)
	case "toggle":
		switch f.id {
		case "sidebar":
			t.toggleSidebar()
		case "remote":
			t.setRemote(!e.Settings.RemoteControl)
		}
	case "button":
		switch f.id {
		case "refresh":
			t.reindex()
		case "save_state":
			if err := e.Persist(); err != nil {
				e.Status = "Error guardando estado: " + err.Error()
			} else {
				e.Status = "Estado guardado"
			}
		case "lf_save":
			e.RefreshLastFM()
			if err := e.SaveSettings(); err != nil {
				e.Status = "No se guardaron credenciales Last.fm: " + err.Error()
			} else {
				e.Status = "Credenciales Last.fm guardadas"
			}
		case "lf_connect":
			e.RefreshLastFM()
			e.ConnectLastFM(openBrowser, func() {
				t.initSettings()
			})
		case "ipod_detect":
			e.DetectIPod(true)
			t.set.inputs["ipod_mount"].Set(e.Settings.IPodMountPath)
		case "ipod_sync":
			e.SyncIPodAsync(nil)
		}
	}
}

func (t *tui) settingsEditKey(k tea.KeyPressMsg) {
	fs := t.settingsFields()
	sel := selectableFields(fs)
	if len(sel) == 0 {
		t.set.editing = false
		return
	}
	f := fs[sel[min(max(t.set.idx, 0), len(sel)-1)]]
	switch k.String() {
	case "enter", "esc":
		t.set.editing = false
		_ = t.eng.SaveSettings()
		if f.id == "root" || strings.HasPrefix(f.id, "lf_") {
			t.eng.RefreshLastFM()
		}
	case "up", "down":
		t.set.editing = false
		_ = t.eng.SaveSettings()
		t.settingsKey(k.String())
	default:
		if in := t.set.inputs[f.id]; in != nil && in.Handle(k) {
			t.applyInput(f.id)
		}
	}
}

func (t *tui) settingsPaste(s string) {
	fs := t.settingsFields()
	sel := selectableFields(fs)
	if len(sel) == 0 {
		return
	}
	f := fs[sel[min(max(t.set.idx, 0), len(sel)-1)]]
	if in := t.set.inputs[f.id]; in != nil {
		in.Paste(s)
		t.applyInput(f.id)
	}
}

// setRemote turns the external control API on or off.
func (t *tui) setRemote(on bool) {
	e := t.eng
	e.Settings.RemoteControl = on
	_ = e.SaveSettings()
	if !on {
		t.remote.Close()
		t.remote = nil
		e.Status = "Control externo desactivado"
		return
	}
	if t.remote != nil {
		return
	}
	r, err := e.StartRemote()
	if err != nil {
		e.Settings.RemoteControl = false
		_ = e.SaveSettings()
		e.Status = "No se pudo iniciar el control externo: " + err.Error()
		return
	}
	t.remote = r
	e.Status = fmt.Sprintf("Control externo activo en 127.0.0.1:%d", r.Port)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

// viewSettings draws the settings in w x h cells.
func (t *tui) viewSettings(w, h int) []string {
	th := t.th
	st := &t.set
	var tabs []string
	for i, name := range settingsTabs {
		if i == st.tab {
			tabs = append(tabs, lipgloss.NewStyle().Foreground(th.accentText).Background(th.accent).Bold(true).Render(" "+name+" "))
		} else {
			tabs = append(tabs, th.dim().Render(" "+name+" "))
		}
	}
	lines := []string{" " + strings.Join(tabs, " "), ""}

	fs := t.settingsFields()
	sel := selectableFields(fs)
	st.idx = min(max(st.idx, 0), max(len(sel)-1, 0))
	cursor := -1
	if len(sel) > 0 {
		cursor = sel[st.idx]
	}
	labelW := 26
	valueW := max(w-labelW-6, 10)
	cursorLine := 0
	for i, f := range fs {
		focused := i == cursor
		if focused {
			cursorLine = len(lines)
		}
		switch f.kind {
		case "head":
			if len(lines) > 2 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+th.strong().Render(f.label))
		case "info":
			lines = append(lines, "  "+th.dim().Render(fit(f.text, w-4)))
		case "input":
			in := t.set.inputs[f.id]
			editing := focused && st.editing
			box := in.View(valueW, editing, "—", th.normal().Render, th.subtle().Render, reverse)
			marker := "  "
			if focused {
				marker = th.accented().Render("▸ ")
			}
			border := th.fg(th.border)
			if editing {
				border = th.fg(th.accent)
			}
			lines = append(lines, marker+th.dim().Render(fit(f.label, labelW))+border.Render("[")+box+border.Render("]"))
		case "toggle", "select":
			marker := "  "
			if focused {
				marker = th.accented().Render("▸ ")
			}
			val := f.text
			if f.kind == "select" {
				val = "‹ " + val + " ›"
			} else if f.text == "activado" || f.text == "visible" {
				val = th.fg(th.good).Render("● ") + th.normal().Render(f.text)
			} else {
				val = th.fg(th.faint).Render("○ ") + th.dim().Render(f.text)
			}
			lines = append(lines, marker+th.dim().Render(fit(f.label, labelW))+val)
		case "button":
			marker := "  "
			style := lipgloss.NewStyle().Foreground(th.text).Background(th.chip).Padding(0, 1)
			if focused {
				marker = th.accented().Render("▸ ")
				style = lipgloss.NewStyle().Foreground(th.accentText).Background(th.accent).Bold(true).Padding(0, 1)
			}
			lines = append(lines, marker+style.Render(f.label))
		}
	}
	if len(lines) > h {
		start := min(max(cursorLine-h/2, 0), len(lines)-h)
		lines = lines[start : start+h]
	}
	return lines
}
