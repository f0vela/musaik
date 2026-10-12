# Musaik - Especificación técnica detallada (para porting)

Este documento define el comportamiento funcional/técnico actual de Musaik para poder portar la app a otro lenguaje o framework (Swift, Rust, C#, Electron, Tauri, etc.) manteniendo paridad.

## 1) Objetivo del producto

App de escritorio para macOS enfocada en:

- reproducir música local (`mp3`, `mp4`, `flac`, `ogg`)
- gestionar biblioteca musical
- visualizar artistas/álbumes/canciones/favoritos/playlists
- integrar Last.fm (auth, scrobble, likes, playcount)
- preparar sincronización hacia iPod Classic (modo disco)

## 2) Arquitectura actual (Go + Mygo)

- UI: `github.com/egoist/mygo/ui`
- App shell: `github.com/egoist/mygo`
- Reproductor: `internal/player` (backend `ffplay` + `ffprobe`)
- Biblioteca/indexado/persistencia: `internal/library`
- Last.fm client: `internal/lastfm`
- iPod classic (modo disco): `internal/ipod`
- Modelos comunes: `internal/model`

## 3) Modelo de datos (paridad requerida)

`Song` (campos activos):

- `ID string` (actualmente ruta del archivo)
- `Path string`
- `Title string`
- `Artist string`
- `Album string`
- `TrackNum int` (tag track number si existe)
- `Duration time.Duration`
- `Format string` (ext sin punto)
- `PlayCount int` (conteo local)
- `LastPlayedAt time.Time` (última fecha/hora de reproducción local)

`Playlist`:

- `Name string`
- `ManualSong []string` (IDs)
- `Smart *SmartPlaylistFilter`

`SmartPlaylistFilter`:

- `ArtistContains`
- `AlbumContains`
- `TitleContains`
- `FavoritesOnly`

Estado de store:

- `Root string`
- `Songs []Song`
- `Favorites map[string]bool`
- `Playlists []Playlist`

## 4) Persistencia local

Directorio base: `~/Library/Application Support`/config dir de usuario bajo `musaik` (vía `os.UserConfigDir()`).

Archivos:

1. `state.json` (biblioteca)
   - favoritos
   - playlists
   - root
   - songs
2. `settings.json` (preferencias UI/servicios)
   - sección activa
   - shuffle/repeat
   - rootInput/searchQuery
   - credenciales Last.fm
   - ajustes iPod (mount path + playlist elegida)

## 5) Reproducción (contrato funcional)

Backend:

- reproducción con `ffplay -nodisp -autoexit -ss <offset> <path>`
- duración con `ffprobe`

Comportamiento:

- `Play(path, duration)` y `PlayFrom(path, duration, offset)`
- `TogglePause()`:
  - pause: detiene proceso y guarda posición
  - resume: relanza proceso desde posición guardada
- `SeekTo(pos)` relanza backend desde nuevo offset
- `Stop()` detiene y limpia estado
- eventos de fin de pista para autoplay `next`/repeat/shuffle

Al cerrar ventana/app:

- detener player
- persistir estado y settings

## 6) UI y navegación

Secciones:

- Librería (Canciones: listado completo)
- Reproducidas recientemente (últimos 15 días)
- Favoritos (likes)
- Artistas
- Álbumes
- Ajustes

Topbar:

- búsqueda general por texto
- integrada al titlebar nativo de ventana (`TitleBarHiddenInset`) con insets dinámicos del área de traffic lights para aspecto/mac behavior nativo.

Player bar:

- progreso (slider)
- tiempos actual/total
- controles: shuffle, previous, play/pause, next, repeat
- bloque now playing (carátula + título + artista/álbum)

## 7) Vista Canciones (requisitos de comportamiento)

Listado con columnas:

- Nombre de pista
- Artista
- Álbum
- Reproducciones (local)
- Formato / tasa (visual)
- Duración
- Tamaño
- Like (♥/♡)

Interacción:

- primer click: selecciona fila
- click en fila seleccionada: reproduce canción
- fila activa: estado visual de reproduciendo

Menú contextual (right click) sobre tabla:

- agregar a playlist manual
- actualizar conteo desde Last.fm (solo habilitado si Last.fm conectado)

Filtros/vistas que usan el mismo listado:

- Canciones: todas las canciones
- Reproducidas recientemente: `LastPlayedAt >= now - 15 días`
- Favoritos: `Favorites[song.ID] == true`

Persistencia:

- Al reproducir una canción se incrementa `PlayCount` y se guarda `LastPlayedAt`.

## 8) Vista Álbumes (requisitos)

- Grid de álbumes con portada representativa.
- Detalle de álbum (estilo Sonora) con:
  - encabezado grande (portada, metadata principal y chips)
  - acciones: reproducir todo, aleatorio, like, menú
  - tabla de pistas por álbum con selección y estado de reproducción
  - menú contextual por pista para sync Last.fm

## 9) Vista Artistas (requisitos)

- panel izquierdo: lista de artistas clickeables
- panel derecho: artista seleccionado
  - agrupación por álbum
  - carátula por álbum
  - lista de canciones por álbum
  - orden preferente por `TrackNum`, fallback por título

## 10) Last.fm (contrato API)

Campos requeridos:

- `APIKey`
- `APISecret`
- `SessionKey`
- `Username`

Métodos implementados:

- `track.updateNowPlaying`
- `track.scrobble`
- `track.love`
- `track.getInfo` (`userplaycount`)
- `auth.getSession` (token -> session key)

Flujo auth integrado:

1. abrir URL `https://www.last.fm/api/auth/?api_key=...&cb=http://127.0.0.1:<port>/callback`
2. servidor local temporal recibe `token`
3. intercambio por `session_key`
4. guardar credenciales en settings

Regla actual:

- like local siempre se guarda; si Last.fm está activo también envía `track.love`

## 11) iPod Classic (modo disco actual)

Detección:

- scan de `/Volumes`
- dispositivo válido si existe:
  - `iPod_Control/`
  - `iPod_Control/iTunes/iTunesDB`

Sync actual:

- copia de canciones a `iPod_Control/Music/F00`
- escritura de playlist `.m3u8` en `Playlists/`

Limitación actual:

- no se reescribe `iTunesDB` (paridad iTunes/Finder no garantizada para firmware stock)

## 12) Action Bar (Cmd+P)

Modal flotante con input y resultados:

- artistas
- canciones
- álbumes
- acciones rápidas: play, pause, next, previous, settings, stop, quit

Atajos:

- `Cmd+P` abre/cierra
- `Esc` cierra
- `Enter` ejecuta primer resultado

## 13) Reglas de rendimiento/memoria

- no guardar binarios de portada dentro de `Song` persistido
- reducir tamaño de portada antes de crear bitmap (downscale)
- cache LRU limitada
- evitar renders innecesarios de artwork repetido

## 14) Criterios de compatibilidad para un port

El port se considera equivalente si cumple:

1. Misma semántica de estado (biblioteca, favoritos, playlists, settings).
2. Misma UX funcional en reproducción (seek/pause/resume/next/autoplay).
3. Mismas capacidades Last.fm (auth + now playing + scrobble + love + playcount).
4. Misma interacción de vistas clave (Canciones, Artistas, Álbumes, Ajustes).
5. Misma Action Bar (búsqueda + acciones).
6. Mismo contrato de sync iPod Classic actual (modo disco + m3u8).

## 15) Extensiones recomendadas para siguiente iteración

- soporte real de `iTunesDB` para iPod Classic
- biblioteca multi-root
- indexado incremental con watcher
- scrobble con umbral estándar
- logs estructurados y diagnósticos de crash/memoria
