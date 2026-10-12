# Musaik

Musaik es una app de escritorio **nativa para macOS** (Go + Mygo UI nativa) para gestionar y reproducir música local con enfoque visual tipo Sonora/screen reference.

## Estado actual

Implementado y funcional:

- Indexado recursivo de carpetas para `mp3`, `mp4`, `flac`, `ogg`.
- Persistencia local de biblioteca y ajustes:
  - `state.json`: canciones indexadas, favoritos, playlists, playcount y última reproducción por canción.
  - `settings.json`: sección activa, búsqueda, shuffle/repeat, Last.fm, iPod.
- Vistas:
  - **Canciones/Librería** (listado completo con plays, formato, duración, tamaño y like)
  - **Reproducidas recientemente** (solo canciones reproducidas en los últimos 15 días)
  - **Favoritos** (canciones con like/corazón)
  - **Álbumes**:
    - grid de álbumes
    - detalle de álbum estilo Sonora (header grande + tabla de pistas)
  - **Artistas** (detalle por artista agrupado por álbum, con carátula por álbum y canciones)
  - **Ajustes** (configuración Last.fm e iPod)
- Reproducción:
  - previous / play-pause / next
  - shuffle / repeat
  - seek y barra de progreso
  - like directo de la canción en reproducción (♥)
  - now playing en barra inferior (carátula + título + artista/álbum)
  - stop correcto al cerrar app
- Interacciones de listado:
  - clic en canción: seleccionar
  - segundo clic en canción seleccionada: reproducir
  - clic derecho por canción: actualizar playcount desde Last.fm y agregar a playlist
- Shell/UI de ventana:
  - top bar integrada con titlebar nativo (`TitleBarHiddenInset`) para look macOS más cercano al diseño.
- Last.fm:
  - auth desde UI (flujo automático con callback local)
  - update now playing
  - scrobble
  - like remoto (`track.love`) al dar like local
  - sync manual de playcount por canción (`track.getInfo`)
- iPod Classic (modo disco):
  - detección en `/Volumes`
  - sync de playlist (copia de archivos + `.m3u8`)
- Action Bar:
  - `Cmd+P` abre barra flotante
  - búsqueda de artista / canción / álbum
  - acciones rápidas: play, pause, next, previous, settings, stop, quit

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

## Documento de especificación

Para porting del proyecto a otro lenguaje/framework, ver:

- [`PROJECT-SPECS.md`](./PROJECT-SPECS.md)

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
