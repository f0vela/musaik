# Musaik

Musaik es una app de escritorio **nativa para macOS** (Go + Mygo) para reproducción de música local con biblioteca, playlists, queue y controles estilo app de audio moderna.

## Funcionalidades principales

- Biblioteca local con indexado recursivo para `mp3`, `mp4`, `flac`, `ogg`.
- Persistencia local:
  - `state.json`: canciones, favoritos, playlists, colores de playlist, playcount y última reproducción.
  - `settings.json`: sección activa, búsqueda, shuffle/repeat, Last.fm, iPod.
- Vistas:
  - **Canciones** (tabla con título, artista, álbum, plays, formato, duración, tamaño, like)
  - **Artistas** (listado simple y detalle por artista)
  - **Álbumes** (grid + detalle)
  - **Queue** (sección dedicada en sidebar con canción actual + siguientes)
  - **Favoritos** y **Recientes**
- Playlists:
  - crear/renombrar/eliminar playlists manuales
  - agregar canción por menú contextual o drag & drop al sidebar
  - color por playlist (tag estilo Finder en sidebar)
- Reproductor:
  - play/pause/next/previous, shuffle, repeat (off/all/one), seek
  - navegación contextual desde now-playing (título/caratula/artista/álbum)
  - toggle de mute en barra de volumen
- Last.fm:
  - auth automática desde ajustes
  - now playing, scrobble, `track.love`, sync de playcount/liked
- iPod Classic (modo disco):
  - detección en `/Volumes`
  - exportación de playlist `.m3u8` + copia de audio
- Action Bar (`Cmd+P`): búsqueda + acciones rápidas.

## Requisitos

- Go 1.27+
- Mygo CLI
- `ffplay` y `ffprobe`

Orden de resolución para `ffplay/ffprobe`:

1. `PATH`
2. `MUSAIK_FFMPEG_DIR`
3. rutas embebidas/empacadas (`./bin`, `<ejecutable>/bin`, `Contents/Resources/bin` en macOS)

## Desarrollo

```bash
go mod tidy
go tool mygo dev
```

## Tests

```bash
go test ./...
```

## Build

```bash
go tool mygo build
```

## Memoria y profiling (RAM)

Musaik incluye instrumentación opcional para pprof y muestreo de memoria:

```bash
export MUSAIK_ENABLE_PPROF=1
export MUSAIK_PPROF_ADDR="127.0.0.1:6060"      # default: 127.0.0.1:6060
export MUSAIK_PROFILE_LOG="/tmp/musaik-mem.csv"
export MUSAIK_PROFILE_INTERVAL_MS=1000          # mínimo: 250
```

Con estas variables:

- expone `http://127.0.0.1:6060/debug/pprof/`
- escribe CSV por intervalo con:
  - `heap_alloc_mb`
  - `heap_sys_mb`
  - `rss_app_mb`
  - `rss_player_mb` (proceso `ffplay`)
  - `rss_total_mb` (app + backend)

## Documentación del proyecto

- Especificación técnica: [`PROJECT-SPECS.md`](./PROJECT-SPECS.md)
- Roadmap y pendientes: [`todo.md`](./todo.md)
- Operación, debugging y guías: [`docs/OPERATIONS.md`](./docs/OPERATIONS.md)
