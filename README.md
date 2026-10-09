# Musaik

Musaik es una app de escritorio **nativa para macOS** (Go + Mygo UI nativa) para gestionar y reproducir música local.

## Estado actual del proyecto

Actualmente existe un MVP funcional con:

- Indexado recursivo de carpetas para: `mp3`, `mp4`, `flac`, `ogg`.
- Vistas principales:
  - **Librería** (todas las canciones indexadas)
  - **Artistas**
  - **Álbumes**
  - **Favoritos**
  - **Playlists** (manuales y dinámicas/smart)
- Controles base de reproducción:
  - previous, play/pause, next
  - repeat, shuffle, like
- Barra de progreso con tiempo actual/total y seek directo en UI.
- Persistencia local de estado (favoritos y playlists).
- Integración base con Last.fm:
  - update now playing
  - scrobble
  - consulta de `userplaycount` por canción

## Requisitos

- Go 1.27+
- Mygo CLI (como tool de Go)
- `ffplay` y `ffprobe` (incluidos en ffmpeg) para reproducción con progreso/seek.

## Desarrollo local

```bash
go mod tidy
go tool mygo dev
```

## Tests

Suite completa:

```bash
go test ./...
```

Test puntual:

```bash
go test ./internal/library -run TestSmartPlaylistFavoritesFilter -v
```

## Build

```bash
go tool mygo build
```

## Last.fm

Configura variables de entorno:

```bash
export LASTFM_API_KEY="..."
export LASTFM_API_SECRET="..."
export LASTFM_SESSION_KEY="..."
export LASTFM_USERNAME="..."
```

La obtención de `LASTFM_SESSION_KEY` aún no está integrada en UI; hoy se carga por variables de entorno.

## Pendientes

Los pendientes y próximos pasos están documentados en [`todo.md`](./todo.md).
