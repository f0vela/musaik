package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// applyTheme builds the theme for themeID. Cell renderings that carry theme
// colors (the placeholder art) are dropped so they are drawn again.
func (t *tui) applyTheme() {
	t.th = buildTheme(t.themeID, t.termDark)
	for k := range t.coverCache {
		delete(t.coverCache, k)
	}
}

func themeIndex(id string) int {
	for i, d := range themeDefs {
		if d.id == id {
			return i
		}
	}
	return 0
}

// setTheme switches theme and remembers the choice.
func (t *tui) setTheme(id string) {
	t.themeID = id
	t.applyTheme()
	if err := (tuiPrefs{Theme: id}).save(); err != nil {
		t.eng.Status = "No se pudo guardar el tema: " + err.Error()
		return
	}
	t.eng.Status = "Tema: " + themeDefByID(id).label
}

// cycleTheme moves d themes along the list, applying and saving as it goes.
func (t *tui) cycleTheme(d int) {
	n := len(themeDefs)
	t.setTheme(themeDefs[(themeIndex(t.themeID)+d+n)%n].id)
}

// openThemePicker shows the themes; moving the cursor previews each one on the
// whole app, Enter keeps it and Esc goes back to the one in use.
func (t *tui) openThemePicker() {
	t.modal = modalState{kind: modalTheme, target: t.themeID, idx: themeIndex(t.themeID)}
}

func (t *tui) themeKey(s string) {
	m := &t.modal
	n := len(themeDefs)
	move := func(i int) {
		m.idx = (i + n) % n
		t.themeID = themeDefs[m.idx].id
		t.applyTheme()
	}
	switch s {
	case "up", "k":
		move(m.idx - 1)
	case "down", "j":
		move(m.idx + 1)
	case "pgup":
		move(max(m.idx-6, 0))
	case "pgdown":
		move(min(m.idx+6, n-1))
	case "home", "g":
		move(0)
	case "end", "G":
		move(n - 1)
	case "enter":
		id := themeDefs[m.idx].id
		t.modal = modalState{}
		t.setTheme(id)
	case "esc":
		back := m.target // read it before the modal is reset
		t.modal = modalState{}
		t.themeID = back
		t.applyTheme()
	}
}

func (t *tui) viewThemePicker() string {
	th := t.th
	pp := t.panelPaint()
	m := &t.modal
	inner := min(max(t.w-12, 34), 50)
	room := min(len(themeDefs), max(t.h-9, 4))
	start := min(max(m.idx-room/2, 0), len(themeDefs)-room)

	var lines []string
	for i := start; i < start+room; i++ {
		d := themeDefs[i]
		sel := i == m.idx
		bg := th.panel
		if sel {
			bg = th.selected
		}
		paint := func(fg color.Color) lipgloss.Style {
			return lipgloss.NewStyle().Foreground(fg).Background(bg)
		}
		marker := paint(th.faint).Render("  ")
		name := paint(th.text).Render(fit(d.label, inner-20))
		if sel {
			marker = paint(th.accent).Render("▸ ")
			name = paint(th.accent).Bold(true).Render(fit(d.label, inner-20))
		}
		preview := buildTheme(d.id, t.termDark)
		var dots strings.Builder
		for _, c := range d.swatch(preview) {
			dots.WriteString(lipgloss.NewStyle().Foreground(c).Background(bg).Render("●"))
		}
		kind := "claro"
		if d.dark || (d.id == themeAuto && t.termDark) {
			kind = "oscuro"
		}
		row := marker + name + paint(th.muted).Render(" ") + dots.String() + paint(th.faint).Render(" "+fit(kind, 6))
		lines = append(lines, fitBg(row, inner, bg))
	}
	lines = append(lines, "", pp(th.faint).Render("↑/↓ elegir (vista previa) · enter aplicar · esc cancelar"))
	return t.panel("Tema", lines, inner)
}
