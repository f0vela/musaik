# Musaik TUI

Versión de terminal de Musaik, hecha con Go + [Bubble Tea v2](https://charm.land/bubbletea) + Lip Gloss v2.
Comparte con la app de escritorio la biblioteca (`state.json`), los ajustes (`settings.json`),
las playlists, los likes, las credenciales de Last.fm y la caché de fotos de artistas.

## Compilar y ejecutar

```bash
go build -ldflags "-X main.version=$(sed -n 's/.*"version": "\(.*\)".*/\1/p' mygo.json)" -o dist/musaik-tui ./cmd/musaik-tui
./dist/musaik-tui            # abre la biblioteca guardada
./dist/musaik-tui -root ~/Music   # indexa otra carpeta al arrancar
./dist/musaik-tui -mini      # arranca en modo mini
./dist/musaik-tui -images blocks  # fuerza el modo de bloques
```

Requiere `ffplay`/`ffprobe`, igual que la app (misma resolución: `PATH`, `MUSAIK_FFMPEG_DIR`, `./bin`).

## Carátulas

Hay dos modos, y `-images` (o `MUSAIK_IMAGES`) elige: `auto` (por defecto), `kitty` o `blocks`.

- **`kitty`** — imágenes nítidas a resolución completa con el protocolo gráfico de Kitty y *Unicode placeholders*:
  la imagen se envía una vez a la terminal (PNG, hasta 640 px) y en pantalla van celdas de texto que la terminal
  sustituye por la imagen. Al ser texto, se comportan bien con modales, scroll y el renderizador de Bubble Tea.
  `auto` lo activa en **kitty** y **Ghostty**. Requisitos: kitty ≥ 0.28 o Ghostty, y color de 24 bits (`COLORTERM=truecolor`,
  que ambas definen). Mientras la imagen llega se muestra la versión en bloques, y al salir se liberan todas.
- **`blocks`** — medio bloque (`▀`, dos píxeles por celda): funciona en cualquier terminal con color de 24 bits
  (iTerm2, WezTerm, Terminal.app reciente…) pero es de baja resolución, porque una carátula de 8×4 celdas son 8×8 píxeles.

Dentro de tmux o screen `auto` usa `blocks`, porque las secuencias gráficas necesitan *passthrough*; se puede forzar con
`-images kitty` si lo tienes configurado (`allow-passthrough on`). Se asume una celda el doble de alta que de ancha para
recortar la imagen; si la tuya difiere mucho, la carátula se estira un poco.

## Arquitectura

```
internal/core        motor sin UI: reproducción, cola, shuffle/repeat, historial, likes, playlists,
                     Last.fm, iPod, indexado, API remota, carátulas y paletas
internal/library     biblioteca y estado persistente      (compartido con la app)
internal/player      backend ffplay                        (compartido con la app)
internal/lastfm      cliente Last.fm                       (compartido con la app)
internal/ipod        sincronización iTunesDB               (compartido con la app)
cmd/musaik-tui       interfaz Bubble Tea
```

El motor es de un solo hilo: lo que ocurre en segundo plano (indexar, Last.fm, iPod, API remota)
vuelve a su goroutine con `Engine.Post`, así que no hay locks ni carreras (`go test -race` pasa).

## Paridad con la app de escritorio

| Función | App | TUI |
|---|---|---|
| Canciones, Artistas, Álbumes, Queue, Favoritos, Recientes (15 días) | ✓ | ✓ (`1`–`6`) |
| Playlists manuales e inteligentes, color, crear/renombrar/eliminar | ✓ | ✓ (`7`, `N` `R` `D` `c`) |
| Agregar a playlist / crear desde una canción | ✓ | ✓ (`A`) |
| Detalle de artista (foto, más escuchadas, álbumes) y de álbum (carátula, pistas) | ✓ | ✓ (`enter`, `P` `z` `F`) |
| Orden de álbumes: Artista / Nombre / Año | ✓ | ✓ (`o`) |
| Búsqueda en canciones, artistas y álbumes | ✓ | ✓ (`/`) |
| Action Bar (`⌘P`): acciones, artistas, canciones, álbumes | ✓ | ✓ (`ctrl+p`) |
| Play/pausa, siguiente/anterior (con historial), seek, volumen, mute | ✓ | ✓ |
| Shuffle y repeat off/all/one | ✓ | ✓ (`s` `r`) |
| Cola manual, relleno aleatorio a 10, quitar, limpiar | ✓ | ✓ (`a` `x` `C`) |
| Like local + `love` en Last.fm | ✓ | ✓ (`f` `l`) |
| Now playing, scrobble (15 s), conteo y likes desde Last.fm | ✓ | ✓ (`u` `U`) |
| Conexión automática a Last.fm (callback local + navegador) | ✓ | ✓ (Ajustes → Integraciones) |
| Indexado de la biblioteca, "Force Refresh", "Guardar estado" | ✓ | ✓ (`ctrl+r`, Ajustes) — en segundo plano |
| Detección de iPod y sincronización con iTunesDB | ✓ | ✓ (`I`, Ajustes) |
| API de control externo para Raycast/Tinycast | ✓ | ✓ — mismo protocolo, mismo puerto y `remote.json` |
| Barra lateral ocultable | ✓ `⌘B` | ✓ `b` |
| Reproductor adaptable al tamaño | ✓ | ✓ (3 niveles + modo compacto) |
| Mini reproductor | ventana flotante | pantalla compacta con los colores de la carátula (`M`, `-mini`) |
| Tema claro/oscuro del sistema | ✓ | ✓ (sigue el fondo de la terminal) |
| Ratón | ✓ | ✓ (filas, barra de progreso, volumen, botones, rueda) |

### Lo que no se porta

- **Teclas multimedia y "Now Playing" de macOS** (`MPNowPlayingInfoCenter`): una app de terminal no tiene acceso. Usa el atajo del launcher (API remota) o `space`/`n`/`p`.
- **Mini reproductor flotante, siempre encima y con opacidad**: es de la ventana nativa. En la TUI el mini es una pantalla compacta; para tenerlo flotante, ponlo en una terminal pequeña o en un popup de tmux (`tmux display-popup -E musaik-tui -mini`).
- **Paginación de álbumes y de canciones**: las listas hacen scroll.

## Teclas

`?` dentro de la app muestra esta misma tabla.

| Reproducción | |
|---|---|
| `space` | play / pausa |
| `n` / `p` | siguiente / anterior |
| `←` / `→` | −5 s / +5 s (`shift`: 30 s) |
| `+` / `-` , `m` | volumen, silencio |
| `s` , `r` | shuffle, repeat (off → all → one) |
| `l` | like a la canción que suena |

| Navegación | |
|---|---|
| `↑ ↓` / `j k`, `pgup pgdn`, `g G` | mover |
| `enter` | reproducir / abrir |
| `esc`, `⌫` | volver / limpiar búsqueda |
| `tab` | foco barra lateral ↔ contenido |
| `1`–`6`, `7` | secciones, primera playlist |
| `/` | buscar |

| Sobre una canción | |
|---|---|
| `a` / `A` | agregar a la cola / a una playlist |
| `f` | like / quitar like |
| `u` / `U` | conteo desde Last.fm / sincronizar la que suena |

| Vistas | |
|---|---|
| `P` `z` `F` | artista/álbum: reproducir todo, aleatorio, like al álbum |
| `o` | ordenar álbumes |
| `x` `C` | cola: quitar, limpiar |
| `N` `R` `D` `c` | playlist: nueva, renombrar, eliminar, color |

| App | |
|---|---|
| `ctrl+p` / `:` | paleta de comandos |
| `,` | ajustes (`[` `]` o `←` `→` cambian de pestaña) |
| `b` | barra lateral |
| `M` | mini reproductor |
| `ctrl+r` | reindexar |
| `I` | iPod: detectar / sincronizar |
| `q` | salir |

## Convivir con la app de escritorio

Ambas leen y escriben el mismo `state.json`. Funciona bien si usas una a la vez; con las dos abiertas,
gana la última que guarda (la TUI guarda los cambios a los 2 s). `settings.json` se fusiona clave a clave:
la TUI solo escribe las que son suyas y respeta el resto (por ejemplo `mini_opacity`).
La API remota usa el puerto 47821; si ya lo ocupa la otra app, toma uno libre y lo publica en `remote.json`.

## Pruebas

```bash
go test ./internal/core ./cmd/musaik-tui          # lógica y pantallas (reproductor falso, HOME temporal)
MUSAIK_SNAPSHOT_DIR=/tmp/snaps go test ./cmd/musaik-tui   # además vuelca las pantallas como texto y PNG
```

Los tests nunca tocan tu biblioteca real: usan un `HOME` temporal.
