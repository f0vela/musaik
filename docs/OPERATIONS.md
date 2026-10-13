# Musaik - Operación y mantenimiento

Este documento concentra tareas operativas para desarrollo, debugging y diagnóstico de rendimiento/memoria.

## 1) Comandos de uso frecuente

Desarrollo:

```bash
go mod tidy
go tool mygo dev
```

Tests:

```bash
go test ./...
```

Build:

```bash
go tool mygo build
```

## 2) Estado y configuración local

Musaik guarda datos en `os.UserConfigDir()/musaik`:

- `state.json`: biblioteca, canciones, playlists, favoritos, playcount.
- `settings.json`: preferencias de UI, Last.fm, iPod.

Si necesitas reset controlado:

1. cerrar app
2. respaldar esos archivos
3. editar o borrar según necesidad

## 3) Profiling de memoria (heap + RSS)

Variables:

```bash
export MUSAIK_ENABLE_PPROF=1
export MUSAIK_PPROF_ADDR=127.0.0.1:6060
export MUSAIK_PROFILE_LOG=/tmp/musaik-mem.csv
export MUSAIK_PROFILE_INTERVAL_MS=1000
```

Con eso:

- pprof queda en `http://127.0.0.1:6060/debug/pprof/`
- se genera CSV con:
  - `heap_alloc_mb`
  - `heap_sys_mb`
  - `rss_app_mb`
  - `rss_player_mb`
  - `rss_total_mb`

Captura manual de heap profile:

```bash
curl -s http://127.0.0.1:6060/debug/pprof/heap > /tmp/musaik-heap.pb.gz
go tool pprof -top /tmp/musaik-heap.pb.gz
```

## 4) Flujo recomendado para medir RAM objetivo

Objetivo: `rss_total_mb` p95 <= 150 MB durante reproducción.

1. iniciar app con variables de profiling.
2. reproducir 10-15 min (navegando Songs/Albums/Artists/Queue).
3. exportar heap profile en tres puntos:
   - inicio
   - mitad
   - final
4. resumir CSV:
   - promedio/p95/máximo de `rss_total_mb`.
5. comparar contra baseline previo.

## 5) Señales comunes de regresión de memoria

- crecimiento sostenido en `rss_total_mb` sin liberar al cambiar de sección.
- aumento progresivo de `rss_player_mb` al cambiar muchas pistas.
- `image.NewRGBA` y nodos de UI dominando inuse heap en pprof.

## 6) Puntos de control en código

- `main.go`
  - caches de artwork/avatar y límites LRU
  - paginación de canciones
  - sampler de memoria y activación de pprof
- `internal/player/player.go`
  - ciclo de vida del proceso `ffplay`
  - `BackendPID()` para muestreo RSS de backend
- `internal/library/*`
  - tamaño de estructuras persistidas y rutas de carga

