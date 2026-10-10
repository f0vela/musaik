# Musaik RS (Rust + GPUI)

Port separado de Musaik (Go/Mygo) dentro del mismo repo, en carpeta independiente:

- `musaik-rs/`

## Objetivo

Mantener paridad funcional con Musaik original usando:

- **Rust**
- **GPUI** para UI nativa desktop

## Estado actual del port

Base implementada:

- estructura modular (`model`, `library`, `settings`, `player`, `lastfm`, `ipod`, `app_state`)
- persistencia local (`state.json`, `settings.json`)
- indexado de biblioteca por carpeta
- reproductor shell con `ffplay` (play/pause/seek/stop)
- cliente Last.fm (auth/getSession, now playing, scrobble, love, playcount)
- sync iPod Classic modo disco (detección `/Volumes`, copia audio + `m3u8`)
- ventana GPUI inicial mostrando estado/cantidades
- scaffold UI interactivo:
  - navegación lateral clickeable por secciones
  - lista de canciones con acciones click (play / like)
  - botones rápidos play/pause/next/previous en cabecera
  - visualización de cantidad de hits de Action Bar
  - render condicional por sección (Library/Favorites/Artists/Albums/Playlists/Settings)
  - acciones de ajustes base (indexar, guardar, detectar iPod, sync Todos)
- lógica de vistas en `AppState`:
  - filtros de biblioteca/favoritos
  - agrupación artista -> álbum -> canciones
  - generación de hits para Action Bar (acciones/artistas/canciones/álbumes)
  - aplicación de quick actions en estado
  - navegación de reproducción en estado (play selected/first, next/previous, shuffle/repeat)

## Ejecutar (cuando red de crates esté disponible)

```bash
cd musaik-rs
cargo run
```

## Dependencias del sistema

- Rust stable reciente
- Xcode + command line tools (macOS + Metal para GPUI)
- ffmpeg (`ffplay`/`ffprobe`)

## Nota de build en esta sesión

Durante esta sesión hubo fallo de resolución DNS hacia `static.crates.io`, por lo que no se pudo completar `cargo check` end-to-end.

El código quedó estructurado y listo para compilar tan pronto se restablezca acceso a crates.
