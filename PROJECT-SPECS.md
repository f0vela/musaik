# Musaik - Especificación técnica detallada (paridad de implementación)

Este documento describe el comportamiento actual para mantener paridad al portar Musaik a otro stack (Swift/AppKit, Rust, C#, Electron/Tauri, etc.).

## 1) Objetivo del producto

- Reproductor local para macOS.
- Gestión de biblioteca (canciones/artistas/álbumes/favoritos/recientes).
- Playlists manuales con color/tag en sidebar.
- Queue dedicada en navegación.
- Integración Last.fm.
- Exportación a iPod Classic (modo disco).

## 2) Arquitectura actual

- UI/App shell: `github.com/egoist/mygo` + `github.com/egoist/mygo/ui`
- Entrada app/UI principal: `main.go`
- Biblioteca/persistencia: `internal/library`
- Reproducción: `internal/player` (`ffplay` + `ffprobe`)
- Last.fm: `internal/lastfm`
- iPod: `internal/ipod`
- Modelos: `internal/model`

## 3) Modelo de datos

`Song`:

- `ID`, `Path`, `Title`, `Artist`, `Album`, `TrackNum`
- `Duration`, `Format`
- `PlayCount`, `LastPlayedAt`

`Playlist`:

- `Name`
- `Color` (string, valores lógicos: `none`, `purple`, `yellow`, `red`, `orange`, `green`, `blue`, `gray`)
- `ManualSong []string`
- `Smart *SmartPlaylistFilter`

`SmartPlaylistFilter`:

- `ArtistContains`, `AlbumContains`, `TitleContains`, `FavoritesOnly`

Store:

- `Root`, `Songs`, `Favorites`, `Playlists`

## 4) Persistencia local

Directorio: `os.UserConfigDir()/musaik`

- `state.json`:
  - favoritos
  - playlists (incluyendo color)
  - root
  - songs
- `settings.json`:
  - sección activa
  - shuffle/repeat
  - búsqueda
  - ajustes Last.fm
  - ajustes iPod

## 5) Reproductor (contrato funcional)

- Backend por proceso externo:
  - `ffplay -nodisp -autoexit -ss <offset>`
  - duración vía `ffprobe`
- Operaciones:
  - play/seek/next/prev
  - pause/resume (relanzando proceso desde offset)
  - volume slider + mute toggle
  - repeat: off/all/one
  - shuffle

Resolución de binarios `ffplay/ffprobe`:

1. `PATH`
2. `MUSAIK_FFMPEG_DIR`
3. rutas empaquetadas (`./bin`, `<exe>/bin`, `Contents/Resources/bin`)

## 6) UI y navegación

Sidebar (orden actual de biblioteca):

1. Canciones
2. Artistas
3. Álbumes
4. Queue
5. Favoritos
6. Recientes

Playlists:

- Sección colapsable.
- Item “Nueva playlist” con icono `+`.
- Playlists con icono de playlist + color tag visual.
- Context menu: renombrar, eliminar, cambiar color.
- Soporte drag&drop de canción a playlist.

Player bar:

- Carátula/título clickeables (navega a canción).
- Artista clickeable (detalle de artista).
- Álbum clickeable (detalle de álbum).
- Sin badge “Hi-Res” ni label de dispositivo al lado de volumen.
- Botón de mute funcional con icono dinámico.

## 7) Vistas clave

Canciones:

- Tabla con columnas: título, artista, álbum, plays, formato, duración, tamaño, like.
- Paginación activa para limitar memoria de render (`songsPerPage`).
- Menú contextual: add to queue, add to playlist, sync Last.fm.

Artistas:

- Lista simple (sin avatar): artista, número de álbumes, número de canciones.
- Click abre detalle de artista.

Álbumes:

- Grid + detalle de álbum.

Queue:

- Pantalla dedicada en sección sidebar.
- Muestra canción actual + siguientes.
- Auto-fill de siguientes hasta mínimo 10 en orden aleatorio cuando aplica.

## 8) Last.fm

Capacidades:

- Auth web callback local (`auth.getSession`)
- `track.updateNowPlaying`
- `track.scrobble`
- `track.love`
- `track.getInfo` (playcount + estado loved)

## 9) iPod Classic (modo disco)

- Detección de mounts válidos bajo `/Volumes`.
- Exporta audio + playlist `.m3u8`.
- No reescribe `iTunesDB` (limitación conocida).

## 10) Rendimiento y memoria

Medidas implementadas:

- Downscale de imágenes:
  - cover: `maxCoverSize=128`
  - avatar: `maxAvatarSize=96`
- LRU cache:
  - covers: `maxCovers=40`
  - artist avatars: `maxArtistAvatars=24`
- Lista de canciones paginada para reducir nodos UI concurrentes.

Diagnóstico en runtime:

- `MUSAIK_ENABLE_PPROF=1` + `MUSAIK_PPROF_ADDR` habilita `net/http/pprof`.
- `MUSAIK_PROFILE_LOG` habilita CSV periódico con heap y RSS (`app`, `ffplay`, `total`).

## 11) Criterios de paridad para port

1. Persistencia equivalente (`state/settings`) incluyendo color de playlist.
2. Semántica de reproducción equivalente (seek, pause/resume, repeat/shuffle, mute).
3. Navegación y UX equivalente en Canciones/Artistas/Álbumes/Queue/Playlists.
4. Integración Last.fm equivalente.
5. Contrato de iPod modo disco equivalente.
6. Capacidad de profiling de memoria equivalente (o mejor).
