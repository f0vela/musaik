# TODO - Musaik (estado actual + próximos pasos)

## Hecho

- [x] Reproducción estable con `ffplay` + `ffprobe` (play/pause/resume/next/prev/seek/progress).
- [x] Guardado de estado de biblioteca y preferencias (`state.json` + `settings.json`).
- [x] Corrección de cierre de app (detener audio al cerrar).
- [x] UI base alineada con diseño de referencia (sidebar/topbar/playerbar).
- [x] Tabla de canciones con:
  - [x] número de track desde metadata
  - [x] plays locales
  - [x] like
  - [x] carátula de álbum (sin repetición en filas del mismo álbum)
- [x] Listado de canciones rediseñado (estilo oscuro Sonora) con columnas:
  - [x] nombre de pista
  - [x] artista
  - [x] álbum
  - [x] reproducciones
  - [x] formato/tasa
  - [x] duración
  - [x] tamaño
  - [x] like
- [x] Vistas de biblioteca:
  - [x] Canciones (todas)
  - [x] Reproducidas recientemente (últimos 15 días)
  - [x] Favoritos (like)
- [x] Persistencia de última reproducción por canción (`LastPlayedAt`) para vista de recientes.
- [x] Optimización de memoria de carátulas (downscale + caché LRU reducida).
- [x] Menú contextual en canciones:
  - [x] agregar a playlist
  - [x] actualizar playcount desde Last.fm (si conectado)
- [x] Detalle de álbum rediseñado en estilo Sonora (header + tabla de tracks + acciones).
- [x] Integración de topbar con titlebar nativo (`TitleBarHiddenInset`) para look/UX macOS.
- [x] Last.fm completo para uso diario:
  - [x] auth automática en Ajustes
  - [x] now playing + scrobble
  - [x] enviar like a Last.fm
- [x] Vista Artistas avanzada (click artista -> canciones por álbum con carátula por álbum).
- [x] Action Bar (`Cmd+P`) flotante con:
  - [x] búsqueda de artista/canción/álbum
  - [x] acciones play/pause/next/previous/stop/settings/quit
- [x] iPod Classic modo disco:
  - [x] detección en `/Volumes`
  - [x] exportación de playlist a `.m3u8` y copia de audio

## Pendiente prioritario (v0.6)

- [ ] Sync iPod Classic con `iTunesDB` real (no solo `.m3u8`) para compatibilidad total con firmware stock.
- [ ] Crear/editar/eliminar playlists manuales completamente desde UI (pantalla dedicada).
- [ ] Añadir acción “agregar a playlist nueva…” desde menú contextual.
- [ ] Mejorar scrobble: enviar solo al cumplir umbral (ej. 50% o 4 min).
- [ ] Mostrar indicador visual de estado Last.fm (conectado/error/pendiente) en topbar o settings.

## Pendiente técnico (v0.7)

- [ ] Soporte multi-carpeta raíz para la biblioteca.
- [ ] Detección incremental de cambios en carpetas (sin reindexado full).
- [ ] Dedupe de canciones por hash/metadata al indexar.
- [ ] Logging estructurado para diagnóstico (`logs/` por sesión).
- [ ] Tests de integración para indexado + persistencia + reproducción.

## Pendiente UX/UI (v0.8)

- [ ] Pulido pixel-perfect final contra `screen.png` (spacing/tipo/tamaños exactos en todas las vistas, especialmente álbumes/canciones).
- [ ] Estado vacío y skeletons de carga en cada vista.
- [ ] Atajos globales configurables (media keys, cmd+shift+f, etc.).
- [ ] Mini player / compact mode opcional.
