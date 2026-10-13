# TODO - Musaik (actualizado)

## Completado recientemente

- [x] Queue movido a sidebar como sección dedicada.
- [x] Orden de sidebar de biblioteca ajustado.
- [x] Sección Playlists colapsable.
- [x] Playlists con icono + color/tag visual.
- [x] Selección de color de playlist (crear + context menu).
- [x] Navegación contextual desde barra de reproducción:
  - [x] título/caratula -> canción en lista
  - [x] artista -> detalle de artista
  - [x] álbum -> detalle de álbum
- [x] Limpieza de player bar:
  - [x] quitar “Hi-Res”
  - [x] quitar label de dispositivo
  - [x] botón mute funcional con icono dinámico
- [x] Optimización de memoria:
  - [x] límites de cache reducidos
  - [x] downscale de imágenes más agresivo
  - [x] paginación en vista de canciones
- [x] Instrumentación de profiling:
  - [x] pprof opcional por env vars
  - [x] log CSV periódico de heap + RSS app/ffplay/total

## Próximo (prioridad alta)

- [ ] Medición formal de RAM **en reproducción real** (no solo baseline idle) y validar objetivo ≤150MB p95.
- [ ] Ajustar estrategia de queue para evitar crecer sin límite en sesiones largas.
- [ ] Agregar acción “Quitar de queue” desde menú contextual en filas de queue.
- [ ] Persistir/recordar estado de página en vista Canciones por sección/filtro (UX).

## Técnico (prioridad media)

- [ ] Indexado incremental (watcher) para evitar reindexado completo.
- [ ] Soporte multi-root de biblioteca.
- [ ] Cache de metadata/artwork separada por disco con invalidación.
- [ ] Logging estructurado para diagnósticos.
- [ ] Tests de integración para: indexado, reproducción, persistencia, queue.

## Producto / Integraciones

- [ ] Scrobble con umbral estándar configurable (50% o 4min).
- [ ] Indicador visual de estado Last.fm.
- [ ] iPod: soporte `iTunesDB` real (además de `.m3u8`) para firmware stock.
