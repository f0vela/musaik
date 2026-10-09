# TODO - Musaik (Roadmap por fases)

## Fase v0.2 - Reproducción sólida + UX básica

Objetivo: convertir el MVP en un player usable día a día.

- [ ] Reemplazar backend de reproducción básico (`afplay/ffplay`) por un motor con mejor control de estado, progreso y seek.
- [ ] Implementar barra de progreso real de reproducción con tiempo actual / tiempo total.
- [ ] Implementar **seek** real (arrastrar barra para adelantar/retroceder pista).
- [ ] Sincronizar de forma confiable eventos de reproducción (play/pause/stop/track end) para actualizar UI en tiempo real.
- [ ] Añadir indicador visual claro de canción en reproducción y estado de pausa.
- [ ] Añadir tests unitarios para módulo de reproducción (estado, cola, navegación).

## Fase v0.3 - Librería navegable + playlists útiles

Objetivo: mejorar exploración y organización musical.

- [ ] Mejorar extracción de metadatos (duración, track number, year, genre) según formato.
- [ ] Mejorar tabla/listado de librería (ordenar por título/artista/álbum/duración).
- [ ] Añadir búsqueda global y filtros rápidos.
- [ ] Crear/editar/eliminar playlists manuales desde UI.
- [ ] Añadir/remover canciones a playlists manuales desde librería.
- [ ] Editor de reglas para smart playlists (AND/OR, múltiples condiciones).
- [ ] Recalcular smart playlists al cambiar favoritos o metadatos.

## Fase v0.4 - Last.fm completo + visual premium

Objetivo: integrar ecosistema externo y reforzar experiencia visual.

- [ ] Implementar flujo de autenticación dentro de la app para obtener/renovar `session_key`.
- [ ] Guardar credenciales/token de Last.fm de forma persistente y segura.
- [ ] Mostrar en UI el playcount de Last.fm por canción en librería/detalle.
- [ ] Mejorar estrategia de scrobble (umbral de tiempo reproducido antes de enviar).
- [ ] Vista de álbumes con carátulas reales (no solo texto).
- [ ] Vista de artistas con agrupación navegable.

## Fase v0.5 - Escala, sincronización y estabilidad

Objetivo: preparar Musaik para colecciones grandes y operación estable.

- [ ] Detectar y manejar cambios en carpeta (archivos agregados/eliminados) sin reindexado total manual.
- [ ] Evitar duplicados de canciones al indexar múltiples rutas.
- [ ] Soportar múltiples carpetas raíz para una sola librería.
- [ ] Añadir tests de integración para indexado + persistencia.
- [ ] Manejo de errores más explícito en UI para fallos de reproducción y red.
- [ ] Definir estrategia de logging para diagnóstico en desarrollo y producción.
