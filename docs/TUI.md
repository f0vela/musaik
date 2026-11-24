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
  `auto` lo activa en **kitty**, **Ghostty** y **Rex** (por `TERM`/`TERM_PROGRAM`; en cualquier otra terminal que soporte el protocolo, `-images kitty`). Requisitos: kitty ≥ 0.28, Ghostty o Rex, y color de 24 bits (`COLORTERM=truecolor`,
  que ambas definen). Mientras la imagen llega se muestra la versión en bloques, y al salir se liberan todas.
- **`blocks`** — medio bloque (`▀`, dos píxeles por celda): funciona en cualquier terminal con color de 24 bits
  (iTerm2, WezTerm, Terminal.app reciente…) pero es de baja resolución, porque una carátula de 8×4 celdas son 8×8 píxeles.

Dentro de tmux o screen `auto` usa `blocks`, porque las secuencias gráficas necesitan *passthrough*; se puede forzar con
`-images kitty` si lo tienes configurado (`allow-passthrough on`). Se asume una celda el doble de alta que de ancha para
recortar la imagen; si la tuya difiere mucho, la carátula se estira un poco.

## Temas

`T` abre el selector (también en la paleta de comandos: «Cambiar tema…», y en Ajustes → Apariencia).
Al mover el cursor se previsualiza cada tema sobre toda la app; `enter` lo aplica y `esc` vuelve al anterior.
`←`/`→` sobre el campo «Tema» de Ajustes también los recorre.

| Familia | Variantes |
|---|---|
| Automático | sigue el fondo de la terminal (claro u oscuro) y no toca sus colores |
| Sonora | Oscuro, Claro — la paleta de la app de escritorio |
| Dracula | — |
| Catppuccin | Mocha, Macchiato, Frappé, Latte |
| Tokyo Night | Night, Storm, Day |
| Rosé Pine | Rosé Pine, Moon, Dawn |
| Gruvbox | Oscuro, Claro |
| Nord · One Dark · Kanagawa | — |
| Solarized | Oscuro, Claro |

Un tema con nombre **fija el fondo y el color de texto de la terminal mientras la app corre** (OSC 11/10) y los
restaura al salir; «Automático» no cambia nada. De cada paleta solo se definen el fondo, el texto, el gris de
comentarios, el acento y siete tonos; el resto (bordes, selección, paneles, chips) se mezcla a partir de ellos,
así todos los temas reciben el mismo tratamiento. Un test comprueba el contraste WCAG de cada uno.

La elección se guarda en `tui.json` (junto a `settings.json`), no en `settings.json`: la app de escritorio reescribe
ese archivo con las claves que conoce y borraría una desconocida. La app de escritorio sigue el tema del sistema.

## Letras

`L` (o «Mostrar u ocultar la letra» en la paleta) sustituye la vista principal por la letra de la canción que suena;
`L` o `esc` la cierran, y ir a otra sección también. La barra lateral, el reproductor y el resto de teclas
(`space`, `n`, `p`, `←` `→`, volumen…) siguen funcionando.

- **De dónde sale**, en este orden: la etiqueta de letra incrustada en el archivo (puede traer tiempos, en LRC),
  un archivo `.lrc` con el nombre del audio y al lado de él, y [LRCLIB](https://lrclib.net), una base comunitaria
  gratuita, sin clave. Si la etiqueta solo tiene texto plano y hay un `.lrc` con tiempos, gana el `.lrc`; con texto
  plano en la etiqueta y nada más, se usa ese y no se consulta LRCLIB. Si el nombre exacto no aparece en LRCLIB, busca
  por título (también sin «(feat. …)» ni «- Remastered») y elige el resultado que dura lo mismo que tu archivo (±4 s),
  del mismo artista y con tiempos.
- **Privacidad**: no se consulta nada hasta que abres la letra. LRCLIB recibe título, artista, álbum y duración de la
  canción. Las respuestas, y también los «no hay», se guardan **3 días** en `lyrics-cache/` (junto a `settings.json`),
  y las caducadas se borran solas. Si falla la red no se guarda nada y se reintenta a los 20 s (o con `R`).
- **Guardar en el archivo** (`S`): escribe la letra mostrada en la etiqueta de letra del audio (mp3, mp4, flac, ogg),
  con tiempos si los tiene, sin tocar las demás etiquetas. Desde entonces se lee de ahí, sin red, y su entrada de la
  caché se borra. Solo aparece para letras descargadas y **nunca es automático**, porque modifica tu archivo. Cierra
  el archivo en otras apps que lo editen; en mp3 TagLib puede reescribirlo entero si la etiqueta no tiene hueco.
- **Sincronizadas**: la línea actual va en el acento y centrada, las pasadas se atenúan. `↑` `↓` `pgup` `pgdn` `g` `G` o la rueda
  la desplazan a mano y sueltan el seguimiento; `c` lo retoma. **Un clic en una línea salta a ese punto de la canción.**
  Sin tiempos se muestra el texto, marcado «sin sincronizar».
- Mientras la letra está abierta, las teclas de las listas (`enter`, `a`, `f`, `N`…) no hacen nada, porque la lista no se ve.

## Visualizador

El mini reproductor (`M`) puede mostrar un espectro del audio en tiempo real, con los colores de la carátula.
`v` cambia de estilo (también «Visualizador: cambiar estilo» en la paleta de comandos y Ajustes → Apariencia):

| Estilo | Qué dibuja |
|---|---|
| Barras | barras que crecen desde abajo, en octavos de celda, con degradado de la paleta |
| Espejo | las barras crecen hacia arriba y hacia abajo desde una línea central |
| Onda | una curva en braille (dos columnas por cuatro filas de puntos por celda) que oscila alrededor del centro |
| Apagado | sin visualizador: no corre nada de lo que sigue |

La elección se guarda en `tui.json`.

**Cómo funciona.** `ffplay` reproduce sin dejar acceso al audio, así que Musaik decodifica el mismo archivo aparte
con `ffmpeg` (mono, 22 kHz), calcula una FFT de 2048 puntos y guarda 64 bandas logarítmicas (de 50 Hz a 10,5 kHz)
por cada treintavo de segundo: unos 2 KB por segundo de audio, solo de la canción que suena. El dibujo busca el
cuadro por la **posición real de reproducción**, así que respeta pausas y saltos (si te mueves fuera de lo analizado,
el análisis se reinicia ahí) y no depende de cómo toque `ffplay`. Se compensan 120 ms por lo que tarda en sonar.
Las barras suben rápido y bajan despacio, y el espectro se inclina 3 dB por octava hacia los agudos para que el lado
derecho no esté siempre plano.

**Lo que cuesta.** Nada, salvo con el mini abierto, un estilo distinto de «Apagado» y una canción sonando: fuera de eso no
existe el proceso `ffmpeg` ni el reloj rápido. Con todo activo, el análisis corre como mucho 6 veces más rápido que
la reproducción y el dibujo va a 15 fotogramas por segundo. En mis medidas (un pty simulado, 100×36) la TUI pasó de
~0,4 % a ~3 % de un núcleo, más ~1,5 % de `ffmpeg` y ~2,5 % del análisis mientras dura (unos 40 s por una canción de
4 minutos). Lo que más pesa no es dibujar (0,3 ms por fotograma) sino escribir al terminal, así que depende de cuán
rápido lo lea el tuyo. Si te importa, `v` hasta «Apagado».

Necesita `ffmpeg`, que se busca igual que `ffplay` (`PATH`, `MUSAIK_FFMPEG_DIR`, junto al ejecutable). Sin él, Ajustes
lo avisa y el mini sigue como antes.

## Arquitectura

```
internal/core        motor sin UI: reproducción, cola, shuffle/repeat, historial, likes, playlists,
                     Last.fm, iPod, indexado, API remota, carátulas y paletas
internal/library     biblioteca y estado persistente      (compartido con la app)
internal/player      backend ffplay                        (compartido con la app)
internal/lastfm      cliente Last.fm                       (compartido con la app)
internal/lyrics      LRCLIB, .lrc, etiquetas, parser, caché (compartido con la app)
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
| Visualizador de espectro en el mini | — | ✓ solo TUI (`v`): barras, espejo u onda |
| Tema claro/oscuro del sistema | ✓ | ✓ (sigue el fondo de la terminal) |
| Selector de temas (Dracula, Catppuccin, Tokyo Night, Rosé Pine…) | — | ✓ solo TUI (`T`) |
| Letras sincronizadas (etiqueta, `.lrc`, LRCLIB) y guardarlas en el archivo | ✓ `⌘L` | ✓ (`L`, `S`) |
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
| `L` / `S` | letra (ver «Letras») / guardarla en el archivo |

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
| `v` | visualizador del mini (barras, espejo, onda, apagado) |
| `T` | elegir tema |
| `ctrl+r` | reindexar |
| `I` | iPod: detectar / sincronizar |
| `q` | salir |

## Rendimiento

Medido con una biblioteca de ~3600 canciones y una terminal de 130×40, sobre un núcleo (macOS, arm64):

| | antes | ahora |
|---|---|---|
| CPU en reposo | 9,2 % | **0,4 %** |
| CPU reproduciendo (app + `ffplay`) | 11,1 % | **2,5 %** (1,6 es de `ffplay`) |
| CPU desplazando con una tecla sostenida | 32,6 % | **9,5 %** |
| Coste de dibujar un frame | 5,3 ms, 14 MB de basura | **0,45 ms, 122 KB** |
| Memoria con portadas de 3000 px (pico) | 95 MB bloques · 235 MB kitty | **50 MB · 100 MB** |

Qué lo explica:

- **Un contador por playlist en cada frame.** La barra lateral reconstruía las playlists (un mapa de las 3600
  canciones cada una) varias veces por frame: el 83 % del coste. Ahora se calcula una vez y se invalida cuando
  algo cambia. Lo mismo con los totales de la biblioteca.
- **Redibujar sin que nada cambie.** Antes había un reloj de 4 Hz que reconstruía la pantalla entera siempre.
  Ahora el reloj solo late cada 0,5 s mientras suena algo (cada 1 s en reposo), y un frame se reutiliza si el
  tick no movió el reloj, la canción ni el estado. Cualquier tecla, clic o evento sí lo reconstruye.
- **El fin de pista llega como mensaje**, no preguntando cada 250 ms: la siguiente canción arranca al instante.
- **La barra de progreso se pintaba celda a celda** con un estilo cada una; ahora son secuencias de color en tandas.
- **El tamaño total de la biblioteca se suma en segundo plano** (un `stat` por canción) para no frenar el arranque.
- **Memoria:** decodificar una portada de 3000 px son decenas de MB que se descartan al instante; se devuelven al sistema
  justo después, y la caché de portadas guarda 12 en vez de 40. Las búsquedas de canción por ID usan un índice.
- Renderizado limitado a 30 fps.

Lo que no se puede bajar desde aquí: `ffplay` pesa ~30 MB y ~1,6 % de CPU mientras suena, igual que en la app de
escritorio (probé `-vn` y `-nostats`: sin efecto medible). El pico con portadas enormes lo marca el decodificador
JPEG de la biblioteca estándar, que no puede decodificar a tamaño reducido.

Para medir tú mismo:

```bash
# coste por frame de cada vista (con una biblioteca real: HOME apunta a una copia)
MUSAIK_BENCH_HOME=/ruta/a/un/home go test ./cmd/musaik-tui -run '^$' -bench . -benchmem
# CPU de toda una sesión, escrito al salir con normalidad (q)
MUSAIK_CPU_PROFILE=/tmp/cpu.prof ./dist/musaik-tui     # luego: go tool pprof dist/musaik-tui /tmp/cpu.prof
# heap en vivo de una sesión: kill -USR1 <pid> escribe el perfil
MUSAIK_HEAP_PROFILE=/tmp/heap.prof ./dist/musaik-tui   # luego: go tool pprof -sample_index=inuse_space dist/musaik-tui /tmp/heap.prof
```

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
