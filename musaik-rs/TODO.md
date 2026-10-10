# TODO - Musaik RS

## Fase 1 - Base técnica

- [x] Crear proyecto Rust separado en `musaik-rs`
- [x] Definir modelos de dominio (`Song`, playlists, smart filters)
- [x] Implementar store + persistencia (`state.json`)
- [x] Implementar settings (`settings.json`)
- [x] Implementar cliente Last.fm base
- [x] Implementar soporte iPod Classic modo disco base
- [x] Crear ventana GPUI inicial

## Fase 2 - UI funcional equivalente

- [~] Sidebar y topbar equivalentes a Musaik actual (scaffold interactivo y navegación por sección listos)
- [ ] Vista Canciones (tabla, cover agrupado por álbum, track #, like, plays)
- [~] Vista Artistas (detalle por álbum/canciones en scaffold textual; falta layout final/carátulas)
- [~] Vista Álbumes en grid (lista scaffold; falta grid final con covers)
- [~] Ajustes (acciones base indexar/guardar/iPod; faltan formularios completos Last.fm/iPod)
- [ ] Barra de reproducción con now playing y controles
- [x] Lógica de secciones/filtros en `AppState` (sin wiring UI final)

## Fase 3 - Interacciones avanzadas

- [x] Action Bar `Cmd+P` (lógica de hits: artista/canción/álbum)
- [x] Action Bar acciones rápidas en `AppState` (play/pause/next/previous/stop/settings/quit)
- [x] Navegación next/previous en estado con soporte shuffle/repeat
- [~] Wiring UI real de Action Bar en GPUI (lista de hits clickeable visible; falta overlay Cmd+P y input)
- [ ] Menú contextual canciones (agregar playlist + sync playcount Last.fm)
- [ ] Enviar like a Last.fm al marcar favorito

## Fase 4 - Hardening

- [ ] Cache/dowscale de carátulas para controlar memoria
- [ ] Tests unitarios de store/player/lastfm
- [ ] Manejo de errores de red visible en UI
- [ ] Telemetría/logging local para debugging
