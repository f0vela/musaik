package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"musaik/internal/core"
)

// fitBg is fit with the padding painted on bg, so a panel has no holes.
func fitBg(s string, w int, bg color.Color) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return padBg(s, w, bg)
}

// panel wraps lines in a rounded border on the panel background. The lines are
// fitted to inner cells; paint is the style factory they were made with.
func (t *tui) panel(title string, lines []string, inner int) string {
	th := t.th
	bg := th.panel
	edge := lipgloss.NewStyle().Foreground(th.accent).Background(bg)
	side := edge.Render("│")
	var out []string
	head := ""
	if title != "" {
		head = " " + title + " "
	}
	pad := inner + 2 - ansi.StringWidth(head)
	out = append(out, edge.Render("╭")+lipgloss.NewStyle().Foreground(th.text).Background(bg).Bold(true).Render(head)+edge.Render(strings.Repeat("─", max(pad, 0))+"╮"))
	for _, l := range lines {
		out = append(out, side+lipgloss.NewStyle().Background(bg).Render(" ")+fitBg(l, inner, bg)+lipgloss.NewStyle().Background(bg).Render(" ")+side)
	}
	out = append(out, edge.Render("╰"+strings.Repeat("─", inner+2)+"╯"))
	return strings.Join(out, "\n")
}

func (t *tui) panelPaint() func(color.Color) lipgloss.Style {
	bg := t.th.panel
	return func(fg color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(fg).Background(bg) }
}

// ---- command palette ---------------------------------------------------------------------

func groupTitle(kind string) string {
	switch kind {
	case "action":
		return "ACCIONES RÁPIDAS"
	case "artist":
		return "ARTISTAS"
	case "song":
		return "CANCIONES"
	}
	return "ÁLBUMES"
}

func (t *tui) viewPalette() string {
	th := t.th
	pp := t.panelPaint()
	inner := min(max(t.w-10, 30), 72)
	maxRows := min(max(t.h-9, 4), 16)

	input := t.pal.input.View(inner-10, true, "Buscar artista o canción…", pp(th.text).Render, pp(th.faint).Render, reverse)
	lines := []string{pp(th.accent).Render("⌕ ") + input + pp(th.faint).Render(" esc"), pp(th.border).Render(strings.Repeat("─", inner))}

	var body []string
	selLine := 0
	last := ""
	for i, h := range t.pal.hits {
		if h.Kind != last {
			last = h.Kind
			body = append(body, pp(th.faint).Bold(true).Render(groupTitle(h.Kind)))
		}
		sel := i == t.pal.idx
		if sel {
			selLine = len(body)
		}
		paint := func(fg color.Color) lipgloss.Style {
			st := lipgloss.NewStyle().Foreground(fg).Background(th.panel)
			if sel {
				st = st.Background(th.selected)
			}
			return st
		}
		glyph := paint(th.muted).Render(" " + hitGlyph(h) + " ")
		if sel {
			glyph = lipgloss.NewStyle().Foreground(th.accentText).Background(th.accent).Render(" " + hitGlyph(h) + " ")
		}
		sub := h.Subtitle
		subW := min(ansi.StringWidth(sub), inner/2)
		titleW := inner - 4 - subW - 2
		row := glyph + paint(th.text).Render(" ") + paint(th.text).Bold(true).Render(fit(h.Title, max(titleW, 4))) + paint(th.muted).Render("  ") + paint(th.muted).Render(fitRight(sub, subW))
		body = append(body, fitBg(row, inner, th.panel))
	}
	if len(t.pal.hits) == 0 {
		body = append(body, pp(th.muted).Render("Sin resultados"))
	}
	start := 0
	if selLine >= maxRows {
		start = selLine - maxRows + 1
	}
	if len(body) > maxRows {
		body = body[start:min(start+maxRows, len(body))]
	}
	lines = append(lines, body...)
	lines = append(lines, pp(th.border).Render(strings.Repeat("─", inner)),
		pp(th.faint).Render(fit("↑/↓ navegar · enter ejecutar · esc cerrar", inner-14))+pp(th.faint).Render(fitRight(fmt.Sprintf("%d RESULTADOS", len(t.pal.hits)), 14)))
	return t.panel("Comandos", lines, inner)
}

func hitGlyph(h actionHit) string {
	switch h.Kind {
	case "artist":
		return "☺"
	case "song":
		return "♪"
	case "album":
		return "◉"
	}
	switch h.ActionID {
	case "play":
		return "▶"
	case "pause":
		return "❚"
	case "next":
		return "»"
	case "previous":
		return "«"
	case "stop":
		return "■"
	case "create-playlist":
		return "+"
	case "like-current-song":
		return "♥"
	}
	return "⌘"
}

// ---- modals --------------------------------------------------------------------------------

func (t *tui) viewModal() string {
	if t.modal.kind == modalTheme {
		return t.viewThemePicker()
	}
	th := t.th
	pp := t.panelPaint()
	m := &t.modal
	inner := min(max(t.w-12, 28), 52)
	var lines []string
	title := ""
	switch m.kind {
	case modalNewPlaylist, modalRenamePlaylist:
		title = "Nueva playlist"
		if m.kind == modalRenamePlaylist {
			title = "Renombrar playlist"
		}
		lines = append(lines, pp(th.muted).Render("Nombre"),
			pp(th.border).Render("[")+m.input.View(inner-2, true, "Mi playlist", pp(th.text).Render, pp(th.faint).Render, reverse)+pp(th.border).Render("]"))
		if m.kind == modalNewPlaylist {
			colors := core.PlaylistColors()
			var dots []string
			for i, c := range colors {
				dot := lipgloss.NewStyle().Foreground(th.playlistColor(c)).Background(th.panel).Render("●")
				if i == m.color {
					dot = lipgloss.NewStyle().Foreground(th.playlistColor(c)).Background(th.panel).Bold(true).Render("[●]")
				} else {
					dot = pp(th.muted).Render(" ") + dot + pp(th.muted).Render(" ")
				}
				dots = append(dots, dot)
			}
			lines = append(lines, "", pp(th.muted).Render("Color  ")+strings.Join(dots, "")+pp(th.muted).Render("  "+colors[m.color]))
		}
		if m.err != "" {
			lines = append(lines, "", pp(th.bad).Render(m.err))
		}
		hint := "enter crear · tab color · esc cancelar"
		if m.kind == modalRenamePlaylist {
			hint = "enter guardar · esc cancelar"
		}
		lines = append(lines, "", pp(th.faint).Render(hint))
	case modalDeletePlaylist:
		title = "Eliminar playlist"
		lines = append(lines, pp(th.text).Render("¿Eliminar la playlist “"+m.target+"”?"), pp(th.muted).Render("Las canciones no se borran de tu biblioteca."))
		if m.err != "" {
			lines = append(lines, pp(th.bad).Render(m.err))
		}
		lines = append(lines, "", pp(th.faint).Render("y eliminar · n cancelar"))
	case modalAddToPlaylist:
		title = "Agregar a playlist"
		for i, c := range m.choices {
			sel := i == m.idx
			st := lipgloss.NewStyle().Foreground(th.text).Background(th.panel)
			marker := "  "
			if sel {
				st = lipgloss.NewStyle().Foreground(th.accent).Background(th.selected).Bold(true)
				marker = "▸ "
			}
			lines = append(lines, st.Render(fit(marker+c, inner)))
		}
		lines = append(lines, "", pp(th.faint).Render("↑/↓ elegir · enter agregar · esc cancelar"))
	}
	return t.panel(title, lines, inner)
}

// ---- help ------------------------------------------------------------------------------------

func (t *tui) viewHelp() string {
	th := t.th
	pp := t.panelPaint()
	type kv struct{ k, v string }
	groups := []struct {
		name string
		keys []kv
	}{
		{"Reproducción", []kv{
			{"space", "play / pausa"}, {"n / p", "siguiente / anterior"},
			{"← / →", "−5 s / +5 s (shift: 30 s)"}, {"+ / -   m", "volumen · silencio"},
			{"s   r", "shuffle · repeat (off, all, one)"}, {"l", "like a lo que suena"},
		}},
		{"Navegación", []kv{
			{"↑ ↓ j k", "mover"}, {"pgup pgdn", "media página"}, {"g / G", "inicio / final"},
			{"enter", "reproducir / abrir"}, {"esc  ⌫", "volver / limpiar búsqueda"},
			{"tab", "foco barra lateral"}, {"1 … 7", "secciones y playlist"}, {"/", "buscar"},
		}},
		{"Sobre una canción", []kv{
			{"a", "agregar a la cola"}, {"A", "agregar a playlist"}, {"f", "like / quitar like"},
			{"u", "conteo desde Last.fm"}, {"U", "sincronizar la que suena"},
		}},
		{"Vistas", []kv{
			{"P  z", "reproducir todo · aleatorio (artista/álbum)"}, {"F", "like al álbum"},
			{"o", "ordenar álbumes"}, {"x  C", "quitar de la cola · limpiarla"},
			{"N  R  D  c", "playlist: nueva · renombrar · eliminar · color"},
		}},
		{"App", []kv{
			{"ctrl+p  :", "paleta de comandos"}, {",", "ajustes"}, {"b", "barra lateral"},
			{"M", "mini reproductor"}, {"T", "elegir tema"}, {"ctrl+r", "reindexar biblioteca"}, {"I", "iPod: detectar / sincronizar"},
			{"?", "esta ayuda"}, {"q", "salir"},
		}},
	}
	render := func(gs []int, width int) []string {
		var lines []string
		for n, gi := range gs {
			if n > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, pp(th.accent).Bold(true).Render(groups[gi].name))
			for _, e := range groups[gi].keys {
				lines = append(lines, pp(th.text).Bold(true).Render(fit("  "+e.k, 14))+pp(th.muted).Render(fit(e.v, width-14)))
			}
		}
		return lines
	}
	var lines []string
	inner := min(max(t.w-8, 30), 70)
	if t.w >= 104 {
		// Two columns: reproduction and navigation, then the rest.
		inner = min(t.w-8, 112)
		colW := (inner - 3) / 2
		left, right := render([]int{0, 1}, colW), render([]int{2, 3, 4}, inner-colW-3)
		for i := 0; i < max(len(left), len(right)); i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, fitBg(l, colW, th.panel)+pp(th.border).Render(" │ ")+r)
		}
	} else {
		lines = render([]int{0, 1, 2, 3, 4}, inner)
	}
	room := max(t.h-4, 6)
	if len(lines) > room {
		lines = append(lines[:room-1], pp(th.faint).Render("… agranda la terminal para ver todo"))
	}
	return t.panel("Atajos de teclado", lines, inner)
}
