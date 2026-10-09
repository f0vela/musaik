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
  - like (`track.love`) al marcar favoritos en la app
  - consulta de `userplaycount` por canción
  - conexión automática desde Ajustes para obtener y guardar `session_key`
- Integración iPod Classic (modo disco):
  - detección automática en `/Volumes`
  - exportación de playlist al iPod (copiado de archivos + `.m3u8`)

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

Puedes configurar Last.fm por variables de entorno o desde **Ajustes**.

Variables de entorno (opcional):

```bash
export LASTFM_API_KEY="..."
export LASTFM_API_SECRET="..."
export LASTFM_SESSION_KEY="..." # opcional si conectas desde Ajustes
export LASTFM_USERNAME="..."
```

En Ajustes existe un botón para autorización automática que abre navegador y guarda `session_key`.

## iPod Classic (5th gen / Video)

En **Ajustes**:

- `Detectar iPod Classic`: busca dispositivos montados en `/Volumes`.
- `Sincronizar playlist a iPod`: copia canciones a `iPod_Control/Music/F00` y genera playlist `.m3u8` en `Playlists/`.

Nota: la exportación actual no reescribe `iTunesDB`; según firmware/flujo del iPod puede requerir gestor externo para reflejar biblioteca Apple nativa.

## Pendientes

Los pendientes y próximos pasos están documentados en [`todo.md`](./todo.md).
