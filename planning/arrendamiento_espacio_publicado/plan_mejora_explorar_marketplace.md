# Plan de mejora de Explorar y Mapa · Marketplace público de EspaciGo

> **Estado: Fases 0, 1, 2, 3 y 4 implementadas y verificadas en desarrollo local.** Listas para verificación final del usuario. La sección 6 contiene el tablero de tareas.

**Revisión del 23 de noviembre de 2026.** Rama `codex/marketplace/feature/contratos-1-modulo-contratos`, base `fa5b16f`, con cambios en el worktree. Este documento define el plan ejecutable con decisiones cerradas para el agente de desarrollo. El alcance cubre desde la **creación/edición de ubicación del espacio (fuente de verdad en mapa)** hasta la búsqueda pública en **Explorar**, ficha y entrada a cotización. La política de ubicación se coordina con el [plan de contratos](plan_contratos_marketplace.md).

---

## 0. Regla crítica previa (Bug bloqueante): Mapa como fuente de verdad al crear/editar espacios

Hoy existe un desfase en la creación de espacios: el mapa y el formulario no se sincronizan correctamente, lo que permite guardar coordenadas inconsistentes con la comuna/región o sin sincronización real. **Esto rompe la búsqueda geográfica en Explorar** y debe corregirse antes de las mejoras de búsqueda.

### Decisiones cerradas para creación y edición de ubicación

1. **Prioridad absoluta del mapa (flujo unidireccional Mapa → Formulario):**
   - El pin en el mapa es la **única fuente de verdad** para `lat`, `lng`, `region` y `comuna`.
   - Al hacer clic en el mapa, arrastrar el pin, usar "Mi ubicación" o seleccionar un resultado en el buscador del mapa, se disparan automáticamente:
     1. Actualización de `lat` y `lng` en el estado del formulario.
     2. Geocodificación inversa (reverse geocoding) + resolución contra el catálogo oficial de **345 comunas y regiones de Chile**.
     3. Relleno automático de `region`, `comuna` y propuesta inicial de `direccion`.
2. **Campos bloqueados vs. editables en el formulario:**
   - **`región` y `comuna`: SOLO LECTURA (no editables manualmente).** Se muestran claramente rellenados desde el mapa (estado `readOnly`/`disabled` visualmente limpio con etiqueta *"Se completa automáticamente desde el mapa. Mueve el pin para cambiarla"*). El usuario **no puede** cambiar región ni comuna manualmente para evitar descalces con las coordenadas.
   - **`dirección` (calle, numeración, referencia/depto/piso): EDITABLE.** El mapa propone la dirección detectada al mover el pin, pero el anfitrión puede editar el texto libremente para corregir numeración, agregar oficina/torre o indicaciones de acceso.
   - **Protección contra sobreescritura accidental:** Si el usuario editó manualmente la dirección y luego mueve el pin a otro lugar significativamente distinto (> 30 m), actualizar la dirección sugerida (o mostrar un aviso rápido *"Dirección actualizada según el mapa"*). Si solo edita el texto de dirección, **no** se modifican `lat`, `lng`, `region` ni `comuna`.
3. **Estado de carga y manejo de errores en geocodificación inversa:**
   - Mientras resuelve la ubicación del pin, mostrar indicador breve en los campos `región` y `comuna` (*"Detectando comuna..."*) y cancelar peticiones previas con `AbortController` para evitar condiciones de carrera si el usuario arrastra el pin varias veces seguidas.
   - Si el pin cae fuera de cobertura o falla la geocodificación inversa (ej. mar o error de red), limpiar `region`/`comuna`, mostrar mensaje claro (*"Ubica el pin dentro de una zona válida en Chile"*) e impedir avanzar/guardar hasta tener `lat`, `lng`, `region` y `comuna` válidos.
4. **Validación en Backend (Go):**
   - El endpoint de creación/actualización de espacio debe validar que `lat`, `lng`, `region` y `comuna` vengan presentes y sean coherentes entre sí al confirmar la ubicación del espacio.

---

## 1. Estado actual y decisiones cerradas por área

| Área | Estado actual | Decisión cerrada para implementar |
| --- | --- | --- |
| Creación de espacio (ubicación) | El mapa no sincroniza bien con el formulario y permite descalces entre mapa, dirección, comuna y región. | **Fase 0 (P0):** El mapa es la fuente de verdad. Mover/colocar el pin rellena `lat`, `lng`, `region`, `comuna` y sugiere `direccion`. **`region` y `comuna` son solo lectura**; solo `direccion` es editable manualmente. |
| Entrada y navegación | `/explorar` y `/explorar/mapa` son vistas separadas que comparten filtros por URL y recuperan scroll mediante `sessionStorage`. | Conservar además **cámara (`center`, `zoom`)**, `selectedSpaceId` y `visitedSpaceIds` (máx. 50) en `sessionStorage` al volver desde una ficha. Mantener la URL limpia y compartible solo con filtros de búsqueda. |
| Modelo geográfico (Búsqueda) | Filtros administrativos (comuna/región), radio y rectángulo (`bbox`) pueden cruzarse y vaciar resultados por accidente. | **Modos espaciales mutuamente excluyentes:** `areaMode = 'admin' \| 'radius' \| 'bbox'`. Al usar **Buscar en esta zona**, se limpian comuna/región y pasa a `bbox`. Al elegir comuna, GPS o punto + radio, se reemplaza `bbox` y se ajusta la cámara. |
| Búsqueda y filtros | Texto libre, categoría, comuna, región, superficie, tarifa, precio, fecha/hora en formato RFC 3339 y atributos por categoría. | Separar **Qué buscas** de **Dónde**. Reemplazar inputs RFC 3339 por controles amigables de fecha + hora inicio/fin en zona horaria **`America/Santiago`**. Agregar filtro uniforme `min_capacity` en Go y Frontend. |
| Actualización del mapa | **Buscar en esta zona** aplica el rectángulo visible; también existe búsqueda por radio. | Mantener **solo botón manual "Buscar en esta zona"** en Fases 1 y 2 (sin auto-búsqueda al arrastrar). Mostrar indicador visual claro cuando la cámara se movió y los resultados aún pertenecen al área anterior. |
| Ordenamiento por precio | Puede comparar tarifas de unidades distintas (hora vs. día vs. mes) como si fueran equivalentes. | Permitir ordenar por precio **solo** cuando haya unidad seleccionada (`hora`, `día`, `mes`) o un rango fecha/hora (usando total estimado por Go). Si el usuario elige ordenar por precio sin unidad, preseleccionar automáticamente `hora` con chip visible. |
| Diseño en pantallas | En escritorio hay lista a la izquierda y mapa a la derecha; en móvil falta compacidad y estados claros del panel inferior. | En escritorio: mapa fijo mientras se hace scroll en la lista. En móvil: cabecera compacta con barra adhesiva, botón visible para alternar **Mapa / Lista** y *bottom sheet* con 3 estados (resumido, intermedio, expandido) más botones explícitos de expandir/contraer. |
| Pines, colisiones y lista | Agrupación con conteo; al pulsar pin abre panel sin foto de portada; sincronización incompleta. | Implementar **cápsulas de precio (*Price Pills*)** con 4 estados accesibles (`default`, `hover/focus`, `selected`, `visited`), diferenciados por borde/escala además de color. Sincronización bidireccional tarjeta ↔ pin con scroll y teclado. En zoom máximo con múltiples espacios en el mismo punto: abrir lista compacta de esos N espacios (sin *spiderfy*). |
| Resultados vacíos y errores | Hay estados vacíos básicos y fallback a cuadrícula. | Si `total_count === 0`, mostrar estado vacío con botón explícito **"Ver cercanos fuera de esta zona"** (hasta 6 resultados más cercanos manteniendo categoría, precio, capacidad y horario) + banner para volver al área original. Diferenciar carga, error de red, GPS denegado y mapa no disponible. |
| Datos y rendimiento | El frontend hace hasta 10 llamadas secuenciales de 48 marcadores (480 en total) a `GET /public/spaces/map`. | **Eliminar paginación secuencial en mapa.** `GET /public/spaces/map` devuelve en **1 sola llamada hasta 250 marcadores ligeros** (`id, slug, title, lat, lng, price, unit, cover_url`) + `has_more` y `total_count` acotados a `bbox`/`radius`. Usar `AbortController` en todas las búsquedas. |
| Geografía, privacidad y PostGIS | Dirección pública al confirmar; Haversine + índice B-tree parcial (V123). | Mantener Haversine + V123 en Fases 1–3. Migrar a PostGIS en Fase 4 **solo si** con 10.000 espacios sintéticos el `p95` supera 50 ms. Privacidad aproximada (`location_visibility: 'exact' \| 'approximate'`) queda para Fase 3 con desplazamiento determinístico al guardar (~250 m en `public_lat/public_lng`). |
| Transporte y POIs | No hay capas de lugares cercanos ni tiempos a pie. | **Fase 3:** No mostrar POIs en el mapa general de exploración. Cargarlos **bajo demanda** al seleccionar un espacio o en la ficha (Transporte y Estacionamiento primero) con íconos secundarios claramente distintos a los espacios. |

**Evidencia acumulada.** Nueve escenarios de Chromium sobre cartografía y datos controlados en 390, 820 y 1440 px, 12 pruebas del adaptador/servicio, comprobaciones Go dirigidas y verificación local de persistencia de radio (5 km Cartagena) entre vistas. Referencia en la [guía de mapas](../../frontend/src/components/maps/README.md).

---

## 2. Contratos de estado y parámetros

### 2.1 Contrato URL vs. `sessionStorage`
- **URL (compartible y fuente de la consulta):**
  `q`, `category_code`, `areaMode` (`admin` | `radius` | `bbox`), `comuna_code`, `region_code`, `latitude`, `longitude`, `radius_km`, `bbox`, `min_unit_price_clp`, `max_unit_price_clp`, `rate_unit`, `start_at`, `end_at`, `min_capacity`, `sort`, `cursor`. El parser admite los alias breves del diseño original y serializa los nombres vigentes. `areaMode` se usa en la navegación. Go lo deduce de los campos geográficos y rechaza combinaciones entre modos.
- **`sessionStorage` (estado efímero de navegación, nunca en la URL):**
  Cada conjunto de filtros conserva el desplazamiento de lista y mapa, el espacio seleccionado, la cámara (`{ center: [lng, lat], zoom }`) y el estado del panel de resultados. El historial de fichas visitadas guarda hasta 50 IDs por pestaña.

### 2.2 Reglas de presentación de precio
- Formato compacto en *Price Pills* con `font-variant-numeric: tabular-nums`: ej. `$25.000/h`, `$60.000/día`, `$420.000/mes` (o `$25k/h` si hay colisión en móvil).
- Si el usuario definió rango fecha/hora y Go entrega precio total estimado, mostrar la misma etiqueta en pin, tarjeta y ficha. El total definitivo se revalida al cotizar.

---

## 3. Fases de ejecución

### Fase 0 · P0 (Bloqueante) · Sincronización Mapa → Formulario en Creación/Edición de Espacios
- **Objetivo:** Garantizar que cada espacio creado o editado tenga coordenadas, región y comuna 100% coherentes, gobernadas por el mapa.
- **Implementación:**
  - Conectar el componente de mapa en el formulario de creación/edición de espacio para que cualquier selección/arrastre del pin o búsqueda en mapa ejecute geocodificación inversa (con `AbortController`) y resuelva `region` y `comuna` contra el catálogo oficial chileno.
  - Hacer `region` y `comuna` estrictamente **solo lectura** en el formulario (rellenados únicamente desde el mapa) con texto de ayuda visible.
  - Mantener `direccion` editable por el usuario (pre-rellenada como sugerencia desde el mapa), sin alterar `lat`/`lng`/`region`/`comuna` al editar el texto.
  - Bloquear el guardado/avance si el pin no tiene `lat`, `lng`, `region` y `comuna` resueltos y válidos; reforzar validación en Go.
- **Aceptación:** Al mover el pin a distintas comunas/regiones, el formulario actualiza automáticamente región, comuna y dirección sugerida; el usuario puede ajustar la dirección pero no editar región ni comuna; al guardar, la ficha y Explorar muestran el espacio exactamente en esa ubicación y comuna.

### Fase 1 · P0 · Claridad de búsqueda, filtros y continuidad
- **Objetivo:** Búsqueda comprensible en móvil y escritorio, sin filtros contradictorios y con regreso intacto desde la ficha.
- **Implementación:**
  - Implementar `areaMode: 'admin' | 'radius' | 'bbox'` mutuamente excluyente; conservar precio, horario, capacidad y categoría al cambiar de ubicación.
  - Reemplazar inputs RFC 3339 por selector de fecha + hora inicio/fin (bloques de 30 o 60 min) con conversión explícita en `America/Santiago`.
  - Agregar soporte uniforme de `min_capacity` en Go (`GET /public/spaces` y `GET /public/spaces/map`), OpenAPI y Frontend.
  - Aplicar regla de ordenamiento por precio (exigir unidad o rango horario; preseleccionar `hora` si se ordena sin unidad).
  - Implementar barra compacta móvil, chips de filtros activos removibles individualmente, alternancia **Mapa / Lista** y persistencia de scroll + cámara + selección en `sessionStorage`.
- **Aceptación:** Una persona sin GPS busca por comuna, tipo, capacidad y horario en móvil, entra a una ficha y al volver conserva scroll, cámara, filtros y selección.

### Fase 2 · P0 · Mapa comparativo y rendimiento de marcadores
- **Objetivo:** Comparar opciones directamente desde el mapa de forma rápida y sin paginación secuencial.
- **Implementación:**
  - Refactorizar `GET /public/spaces/map` para retornar en **1 sola llamada hasta 250 marcadores ligeros** (`id, slug, title, lat, lng, price, unit, cover_url`) + `has_more` y `total_count`, eliminando las 10 llamadas secuenciales en Frontend. Añadir `AbortController` en el cliente.
  - Mostrar aviso explícito en el mapa cuando `has_more === true` (*"Acércate para ver más espacios en esta zona"*) y cuando la cámara cambió sin presionar **Buscar en esta zona**.
  - Implementar *Price Pills* con los 4 estados accesibles (`default`, `hover/focus`, `selected`, `visited`) y sincronización bidireccional tarjeta ↔ pin con scroll y teclado.
  - Vista previa (popover/panel) con foto de portada, título, comuna, precio con unidad y acción **Ver espacio**.
  - Resolver múltiples espacios en la misma coordenada (zoom máximo) abriendo una lista compacta de esos N espacios en el panel/popover.
  - Implementar estado vacío con botón **"Ver cercanos fuera de esta zona"** (hasta 6 espacios más cercanos conservando filtros) + botón para volver al área original.
- **Aceptación:** Se pueden comparar tres espacios sin abrir fichas, navegar por teclado entre lista y mapa, inspeccionar múltiples espacios en un mismo edificio y recuperarse de un área vacía sin perder filtros.

### Fase 3 · P1 · Destinos extendidos, privacidad opcional y contexto (POIs)
- **Destinos:** Priorizar catálogo oficial de 345 comunas y sumar sugerencias de barrios/calles/hitos vía Go (si se usa Amazon Location `Suggest`, aplicar debounce, `AbortController`, límites de tasa y sin persistir resultados de `Suggest`).
- **Privacidad aproximada:** Agregar `location_visibility: 'exact' | 'approximate'` por espacio. En modo `approximate`, calcular y persistir `public_lat/public_lng` desplazados de forma **determinística al guardar** (~250 m, nunca aleatorios por request) y renderizar círculo aproximado en el mapa público. La dirección exacta solo se expone al confirmar reserva/pago.
- **POIs bajo demanda:** Mostrar **Transporte** y **Estacionamiento** únicamente al seleccionar un espacio o en su ficha (botón *"Ver cercanía"*), con íconos secundarios. Calcular minutos a pie solo si hay ruta disponible; de lo contrario, etiquetar explícitamente distancia en línea recta.

### Fase 4 · P1 · Medición, PostGIS condicional y validación de salida
- Medir `p50`/`p95`, tamaño de respuesta y latencia en red móvil. Ejecutar `EXPLAIN (ANALYZE, BUFFERS)` con 10.000 espacios sintéticos combinando área, disponibilidad, capacidad y precio.
- **Regla de decisión PostGIS:** Migrar a `geography(Point, 4326)` + índice `GiST` (`ST_DWithin` / operador de caja) **solo si** `p95 > 50 ms` bajo carga sintética. Si cumple debajo de 50 ms, conservar Haversine + índice B-tree parcial (V123).
- Validación en dispositivos físicos iOS/Android, accesibilidad (teclado y lector de pantalla), enlaces compartidos, red lenta y recorrido completo hasta cotización. Telemetría sin coordenadas exactas ni texto libre del usuario.

---

## 4. Checklist técnico de ejecución (Fases 0, 1 y 2)

### Fase 0 · Creación/Edición de espacio (Mapa → Formulario)
- [x] **Frontend (Mapa + Formulario de creación/edición):**
  - Identificar el componente de mapa y el formulario de ubicación en creación/edición de espacio.
  - Conectar eventos de clic, arrastre de pin y selección en buscador de mapa para actualizar `lat`/`lng` y disparar geocodificación inversa con `AbortController`.
  - Mapear resultado a `region` y `comuna` del catálogo oficial (normalizando tildes/mayúsculas) y pre-rellenar `direccion` solo cuando el pin cambie de ubicación.
  - Marcar `region` y `comuna` como `readOnly` (bloqueados a edición manual) con mensaje explicativo; mantener `direccion` editable.
  - Bloquear envío del formulario si falta ubicación válida en mapa (`lat`, `lng`, `region`, `comuna`).
- [x] **Backend (Go):**
  - Verificar que el handler de creación/edición valide presencia y coherencia de `lat`, `lng`, `region` y `comuna`.
- [x] **Pruebas:**
  - Test E2E (Chromium): mover pin a dos comunas distintas, comprobar que `region` y `comuna` se actualizan solas y no se pueden editar manualmente, editar `direccion`, guardar y verificar que los datos persistidos coinciden con el mapa.

### Fase 1 · Explorar (Claridad, filtros y continuidad)
- [x] **Backend (Go) y OpenAPI:**
  - Añadir parámetro `min_capacity` en `GET /public/spaces` y `GET /public/spaces/map`, actualizando contrato OpenAPI y tests Go.
  - Confirmar soporte de `areaMode` / exclusión clara entre `bbox`, `radius` y `comuna`/`region`.
- [x] **Frontend (Next.js):**
  - Refactorizar estado de filtros URL vs. `sessionStorage` (`scrollY`, `selectedSpaceId`, `camera`, `visitedSpaceIds`).
  - Reemplazar inputs de fecha/hora RFC 3339 por selector legible en `America/Santiago`.
  - Implementar regla de ordenamiento por precio con unidad obligatoria o rango horario.
  - Ajustar barra compacta móvil, chips removibles y botón de alternancia Mapa/Lista.
- [x] **Pruebas:**
  - Validar en Chromium (390 px y 1440 px) ida y vuelta desde ficha conservando scroll, cámara y selección.

### Fase 2 · Mapa comparativo y rendimiento
- [x] **Backend (Go) y OpenAPI:**
  - Actualizar `GET /public/spaces/map` para devolver hasta 250 marcadores ligeros en 1 llamada + `has_more` y `total_count`.
  - Agregar endpoint `GET /public/spaces/nearby` para "cercanos fuera de esta zona" (`limit=6` ordenado por distancia manteniendo filtros no geográficos).
- [x] **Frontend (Next.js):**
  - Eliminar bucle de 10 páginas secuenciales en mapa y añadir `AbortController`.
  - Implementar *Price Pills* (4 estados), sincronización bidireccional tarjeta ↔ pin, vista previa con foto, popover/lista para múltiples espacios en mismo punto y flujo de estado vacío con recuperación.
- [x] **Pruebas:**
  - Verificar compilación Go/Next, TypeScript, ESLint y contrato OpenAPI.

### Fase 3 · Destinos extendidos, privacidad y POIs
- [x] **Destinos:** `GET /public/destinations/suggest` priorizando catálogo oficial de comunas y sugerencias de lugares con rate limit y debounce + `AbortController` en frontend.
- [x] **Privacidad aproximada:** Migración `V000136__public_location_visibility.sql`, proyección determinística (~250 m) en `internal/spaces`, enmascaramiento de dirección/coordenadas en `publiccatalog` y círculo aproximado en `LocationMap`.
- [x] **POIs bajo demanda:** `GET /public/spaces/{id}/nearby-pois` y componente `SpaceNearbyPois` en vista previa del mapa y ficha del espacio.

### Fase 4 · Medición y decisión PostGIS
- [x] **Benchmark 10.000 espacios:** Ejecutado con `scripts/benchmark-phase4-explore-postgres.sh` (`p95` bajo 50 ms: se conserva Haversine + V123).

---

## 5. Inspección y propuesta exacta para la Fase 0

### 5.1 Qué está causando el desfase

| Archivo | Comportamiento observado |
| --- | --- |
| [Mapa compartido](../../frontend/src/components/maps/location-map.tsx) | Clic y fin de arrastre ya notifican por `onMove`. Falta conectar una acción de GPS del anfitrión y hacer que el candidato del buscador siga el mismo camino. |
| [Campos del mapa](../../frontend/src/features/host-listings/ui/commercial-location-fields.tsx) | `movePin` consulta geocodificación inversa, pero seleccionar un candidato o escribir coordenadas llama directamente a `onChange`. El efecto depende de dirección, región y comuna, por lo que editar el texto puede cancelar una consulta del pin. Una respuesta vacía no limpia por sí misma los códigos anteriores. La búsqueda por texto hoy exige ambos códigos. |
| [Asistente de alta](../../frontend/src/features/host-listings/ui/listing-wizard.tsx) | Valida la dirección y los códigos en el paso 2, sin exigir un pin resuelto. Crea el borrador y configura la zona horaria antes de guardar la ubicación. Ante ciertos rechazos geométricos hace otro PUT con `public_confirmed: false`. |
| [Ubicación del editor](../../frontend/src/features/host-listings/ui/commercial-location-editor.tsx) | Bloquea los selectores solo cuando una respuesta devuelve ambos códigos. Ante un movimiento o error puede reabrir la edición manual y conservar los códigos del punto anterior. |
| [Editor principal](../../frontend/src/features/host-listings/ui/listing-editor.tsx) y [repositorio](../../internal/adapters/postgres/spaces/repository.go) | Guardar Detalles reenvía la dirección o los códigos por rutas genéricas. Aunque se actualizan tras guardar Ubicación, son estados separados y esas rutas todavía pueden modificar geografía sin validar el pin. |
| [Validación Go](../../internal/spaces/location.go) y [consulta inversa](../../internal/spaces/transport/http/reverse_geocode.go) | Go comprueba la geometría en PUT de ubicación únicamente si se confirma públicamente y el verificador existe. La consulta inversa ya obtiene el código CUT desde el polígono y lo cruza con el catálogo activo. Puede devolver `{}` sin coincidencia o una pareja válida sin dirección sugerida. |

### 5.2 Cambios propuestos por archivo

| Tarea | Archivos | Cambio y criterio de entrega |
| --- | --- | --- |
| **0.1 · Estado compartido** | Nuevo `ui/use-commercial-pin-resolution.ts`; `domain/commercial-location.ts`; `commercial-location-runtime.ts`; `infrastructure/commercial-geocoder.ts` | Definir `idle / resolving / resolved / invalid / unavailable`. Mantener un único objeto con pin, revisión de la selección, códigos, propuesta de dirección y estado de confirmación. Toda selección llama a `selectPin`. Primero guarda el nuevo punto, limpia los códigos y la confirmación, aborta el request anterior y después resuelve. Aceptar una respuesta solo si su secuencia corresponde al punto actual, aunque el transporte ignore el aborto. Cancelar al desmontar o cambiar de espacio/sesión. No usar la edición de dirección como disparador ni dependencia del lookup. |
| **0.2 · Eventos y campos privados** | `ui/commercial-location-fields.tsx`; `components/maps/location-map.tsx` solo si hace falta una extensión privada optativa | Conectar clic, `dragend`, GPS explícito y selección de candidato al mismo `selectPin`. Separar el texto del buscador del mapa de la dirección editable. Retirar la edición directa de coordenadas y los selectores administrativos en alta/edición. Mostrar región y comuna como valores de solo lectura con «Detectando comuna…», «Mueve el pin para cambiarla» o un error. El GPS fallido conserva el punto vigente y ofrece selección manual en el mapa. Mantener el teclado con búsqueda y selección de candidatos o un botón accesible para usar el centro movido con teclado. |
| **0.3 · Dirección editable y guardado** | `ui/listing-wizard.tsx`; `ui/commercial-location-editor.tsx`; `ui/listing-editor.tsx`; tipos y adaptador `host-listings-openapi-adapter.ts` | Vincular dirección y códigos al estado compartido. Al editar texto, conservar coordenadas, códigos y origen del pin, y pedir que se revise nuevamente el consentimiento de publicación si corresponde. Recordar el punto de la última edición manual. Una propuesta para un movimiento de hasta 30 m conserva ese texto. Si supera 30 m y llega una propuesta válida, aplicarla con aviso y opción de deshacer. Si se escribe durante la consulta, preservar la edición más reciente. Bloquear Siguiente, Confirmar alta y Guardar ubicación mientras falten pin y códigos resueltos para la misma revisión. Eliminar el reintento que convierte un rechazo geométrico en guardado sin confirmar. Desacoplar Detalles de las escrituras de Ubicación. |
| **0.4 · Buscador del mapa y contrato de resolución** | `internal/spaces/transport/http/{geocode,reverse_geocode}.go`; `cmd/api/location_geocode.go` si requiere adaptar el proveedor; `planning/openapi.yaml` | Permitir en el buscador privado una consulta textual de destino contextualizada en Chile sin exigir códigos previos. Conservar el modo con ambos códigos para los consumidores existentes. Los resultados son candidatos, y seleccionar uno vuelve a resolver las coordenadas contra Go. Los códigos CUT provienen del polígono y el catálogo activo. Devolver dos códigos válidos o un error distinguible de punto no resoluble o servicio no disponible. La calle sugerida es opcional si la resolución oficial sí terminó. Mantener autenticación y revisar el límite de solicitudes del flujo combinado. |
| **0.5 · Alta atómica y escritor único** | `internal/spaces/{domain,service,location}.go`; `internal/spaces/transport/http/{handler,location}.go`; `internal/adapters/postgres/spaces/{repository,location}.go`; `planning/openapi.yaml`; cliente y allowlist generados | Propuesta de contrato: `POST /spaces` usa un esquema de alta separado con `location` obligatorio que contiene dirección, latitud, longitud, región, comuna, origen, precisión si aplica y consentimiento público. Persistir borrador y ubicación válida en la misma transacción usando columnas V123/V135 y sus protecciones vigentes. Un error de ubicación no deja borrador parcial. Configurar zona horaria y horario después, usando el ETag del alta, y conservar recuperación si falla esa etapa. `PUT /spaces/{id}/location` conserva CAS y exige coordenadas y códigos canónicos incluso con consentimiento público apagado. Las rutas de contenido dejan de escribir región, comuna y dirección. Durante una transición, aceptar campos heredados solo si coinciden con los valores guardados, verificando bajo el bloqueo de fila. Inventariar y actualizar consumidores y fixtures de `POST` antes de exigir el nuevo contrato. |
| **0.6 · Autoridad geográfica y registros anteriores** | `internal/spaces/location.go`; `internal/geoboundary`; `cmd/api/space_location_boundaries.go`; [guía de mapas](../../frontend/src/components/maps/README.md) | Centralizar en Go la resolución usada por consulta inversa, alta y edición. Validar coordenadas, coincidencia exacta de ambos códigos con el CUT resuelto, catálogo activo y desempate estable en límites. Si falta geometría o catálogo, rechazar la mutación con error explícito. Confirmar carga del archivo del entorno y medir qué códigos del catálogo tienen polígono. El generador documenta que DPA 2023 excluye Antártica. No suponer cobertura de todos los códigos ni asignar una comuna aproximada en zonas sin polígono. Abrir espacios anteriores sin modificar sus datos al cargar. Un registro sin ubicación coherente debe resolver un punto antes de guardar Ubicación. Revisar el paso a publicación y mantener la reparación separada de cambios de título/fotos. No corregir en masa ni esconder publicaciones existentes por inferencia. |
| **0.7 · Comprobación bloqueante** | `frontend/tests/wizard-improvements.spec.ts`; pruebas del editor y catálogo; `internal/spaces/transport/http/{geocode,location}_test.go`; pruebas de Geoboundary y de repositorio | Sustituir expectativas que hoy permiten selectores manuales o el reintento sin confirmar. Verificar alta y edición, dos comunas de regiones distintas, clic, arrastre, GPS permitido/denegado, candidato, mar, dato ausente, error HTTP, respuestas invertidas, edición manual antes/durante la consulta, umbral de 30 m, portales responsive y datos antiguos. En API, rechazar coordenadas y códigos incompatibles aunque se llame directamente, incluyendo confirmación apagada y verificador ausente. Comprobar rollback, conflicto ETag y que Detalles no pisa la ubicación. Guardar, recargar, publicar y comparar fila, ficha y marcador de Explorar con datos locales. |

**Distinción para confirmar junto con el plan.** Si Go responde con una región y comuna oficiales válidas, pero el proveedor carece de calle, se permite completar la dirección manualmente. Si falla la consulta a Go, la geometría o el catálogo oficial, se limpian los códigos, se muestra el error y se bloquea el guardado. Así una calle sin cobertura no se confunde con un pin sin comuna.

**Puerta de salida de Fase 0.** Demo local de alta y edición con ubicación verificable en API, ficha y Explorar. Pruebas de error sin borrador incoherente ni sobrescritura. Evidencia para cada caso clave. La Fase 1 queda cerrada hasta acreditar esta puerta.

## 6. Tablero ejecutable después de la confirmación

| Fase | Orden y tareas | Verificación para cerrar |
| --- | --- | --- |
| **0 · Bloqueante** | **0.1** contrato compartido de estado y API. **0.2–0.3** integrar mapa y formularios. **0.4–0.6** buscador privado, alta/edición atómica, catálogo y rutas de contenido. UI y Go pueden avanzar en paralelo tras fijar el contrato. **0.7** pruebas conjuntas. | Puerta de salida de la sección 5. |
| **1 · Búsqueda y continuidad** | **1.1** contrato `areaMode`, parser/serializador, enlaces antiguos y traducción a parámetros públicos vigentes, con un solo modo a la vez. **1.2** `min_capacity` en Go y OpenAPI para lista/mapa. **1.3** selector fecha/hora de Santiago, conversión a RFC 3339 y pruebas de cambios DST. **1.4** orden por precio por unidad o total, con chip visible si se preselecciona hora. **1.5** cabecera y chips en móvil, alternancia y retorno con cámara, scroll, selección e historial FIFO de hasta 50. **1.6** pruebas de navegación y enlaces. | Intercambiar modos y vistas sin perder filtros no geográficos. URL compartible con filtros y estado local con cámara/selección. Los cursores vigentes pueden seguir siendo opacos aunque la UI muestre una página. |
| **2 · Comparación en el mapa** | **2.1** esquema de marcadores y consulta acotada a 250 con conteo para la misma búsqueda, más búsqueda alternativa de hasta 6 fuera del área. Usar el identificador/ruta canónica actual hasta verificar si existe slug público real. **2.2** eliminar el bucle e integrar cancelación de búsquedas obsoletas en el transporte efectivo Next/Go. **2.3** cápsulas accesibles, agrupación, selección bidireccional y tratamiento de coincidencias exactas. **2.4** panel con tres alturas, foto y precios con unidad o total estimado. **2.5** indicador de área pendiente, límite visible y recuperación del área vacía. **2.6** pruebas por tamaño, teclado y red lenta. | Una solicitud de marcadores por búsqueda aplicada. Conteo y aviso correctos. La alternativa muestra resultados fuera del área con banner y retorno. El movimiento del mapa solo consulta al pulsar la acción. |
| **3 · Destinos, privacidad y contexto** | **3.1** sugerencias por comuna, barrio, calle e hito mediante Go con cancelación y límites. **3.2** migración de precisión pública, valor heredado `exact`, proyección aproximada persistida y control de acceso exacto según el hito de reserva/pago autorizado en el plan de contratos. **3.3** revisar búsqueda, respuesta, precios/distancias, fotografías/metadatos y POIs para evitar inferir el punto exacto en modalidad aproximada. Usar una proyección pública coherente para filtrar, ordenar y dibujar. **3.4** transporte y estacionamiento bajo demanda, con rutas peatonales solo cuando exista cobertura. **3.5** verificación de privacidad y fallback. | Un espacio aproximado nunca entrega su dirección/point exactos en rutas públicas. Contexto de alrededores separado del catálogo y sin consultas automáticas masivas. |
| **4 · Medición y cierre** | **4.1** fixture de 10.000 espacios y matriz de consultas combinadas con `EXPLAIN (ANALYZE, BUFFERS)`. **4.2** medir p50/p95 y decidir PostGIS solo si el p95 de la consulta supera 50 ms bajo la carga acordada. **4.3** si se activa, comparar planes y resultados, preparar migración y probar radio y bbox con índices compatibles. **4.4** validar iOS/Android físicos, teclado y lector, red lenta, enlaces y recorrido hasta cotización. **4.5** registrar evidencias, latencias y cobertura sin telemetría con coordenadas exactas ni texto libre. | Suite local completada con evidencia. Decisión de consulta respaldada por medición. Preparar integración según las reglas del repositorio cuando el usuario la autorice. |

Las fases 2–4 quedan **pendientes de inicio**. La evidencia de las fases ejecutadas se registra al final del documento.


## 7. Evidencia de cierre de la Fase 0

- `POST /spaces` guarda borrador, punto y códigos juntos. `PUT /spaces/{id}/location` valida la misma resolución oficial con el ETag vigente. Las ediciones de contenido del anfitrión y del administrador no reescriben ubicación.
- [Ensayo en PostgreSQL aislado](../../scripts/test-phase0-location-postgres.sh) aprobado. Usó el esquema local, cinco catálogos de referencia, un anfitrión sintético y los polígonos DPA. Un alta incoherente dejó cero filas. Se guardó un cambio de Santiago a Valparaíso, se rechazaron un ETag vencido y un cambio directo de comuna desde Detalles, y la ficha, la cuadrícula y el mapa mostraron la ubicación esperada.
- Ocho escenarios dirigidos en Chromium de escritorio y dos en emulación móvil pasaron sobre la compilación local final. Incluyeron GPS permitido y denegado, clic, arrastre, selección por buscador, 422 geográfico, respuestas invertidas, corrección de dirección sin mover el pin, edición, reapertura con y sin resolución disponible y recuperación tras un fallo posterior de horarios.
- Pasaron la compilación local, TypeScript, ESLint de los archivos del corte, las suites Go dirigidas, la verificación de OpenAPI y 22 pruebas JavaScript de ubicación y guardado. Una prueba de comparación con una versión histórica del adaptador requiere un fuente de referencia optativo. La prueba PostgreSQL usa el esquema ya migrado. La Fase 0 no añadió migraciones.
- El servidor de desarrollo quedó reiniciado con los límites comunales. Una futura promoción debe preparar la geometría oficial y su configuración en el destino. La Fase 0 habilitó el inicio de la Fase 1.

## 8. Evidencia de la Fase 1

- El filtro `min_capacity` funciona en lista y mapa con la misma condición de PostgreSQL y tiene cobertura de validación, contrato OpenAPI, lista de parámetros del gateway y vinculación de cursores. La API devuelve 422 cuando se mezclan modos geográficos. El frontend conserva los filtros de categoría, capacidad, horario y precio al cambiar de modo.
- El selector usa hora local de Santiago en bloques de 30 minutos. Se probaron offsets de verano e invierno, horas inexistentes y horas repetidas. El orden usa tarifa comparable por unidad o total estimado cuando hay un intervalo. Elegir tarifa sin unidad ni intervalo aplica por hora y crea un chip removible.
- Pasaron 13 escenarios de Chromium con catálogo de prueba, incluidos móvil de 390 px, tablet de 820 px y escritorio de 1440 px. Se verificaron búsqueda sin GPS por comuna, horario y capacidad, transición a radio, chips, alternancia de vista, confirmación manual del área y regreso a Explorar con desplazamiento, cámara y espacio seleccionado.
- Pasaron la compilación de Next.js con TypeScript, ESLint dirigido, la verificación completa de OpenAPI y las suites Go de arranque de API, catálogo, transporte y repositorio. La suite JavaScript pasó 821 pruebas. Una comparación histórica quedó omitida por requerir `BASELINE_DRAFT_ADAPTER_SOURCE`. Se actualizó la prueba heredada del asistente para verificar los campos de región y comuna resueltos desde el mapa en la Fase 0.

## 9. Evidencia de las Fases 2, 3 y 4

- **Fase 2:** `GET /public/spaces/map` devuelve hasta 250 marcadores en una sola llamada con `has_more`, `total_count`, `comuna_name`, `estimated_total_clp` y `cover_url`. Se añadió `GET /public/spaces/nearby` (`limit=6` ordenado por distancia conservando filtros no geográficos). En frontend se eliminó el bucle secuencial de 10 páginas, se integró `AbortController`, *Price Pills* con 4 estados accesibles (`default`, `hover/focus`, `selected`, `visited`), sincronización bidireccional tarjeta ↔ pin con teclado y scroll, lista para múltiples espacios en el mismo punto, vista previa con foto, *bottom sheet* móvil de 3 alturas (`collapsed`/`half`/`expanded`) y recuperación de área vacía.
- **Fase 3:** Se aplicó la migración `V000136__public_location_visibility.sql` (`ubicacion_visibilidad`, `ubicacion_publica_latitud`, `ubicacion_publica_longitud`), cálculo determinístico (~250 m) en `internal/spaces`, proyección pública consistente y dirección enmascarada en `publiccatalog` + círculo aproximado en `LocationMap`. Se añadieron `GET /public/destinations/suggest` (con rate limiting, prioridad de comunas y debounce + `AbortController` en frontend) y `GET /public/spaces/{id}/nearby-pois` + componente `SpaceNearbyPois` bajo demanda en mapa y ficha.
- **Fase 4:** Benchmark `scripts/benchmark-phase4-explore-postgres.sh` con 10.000 espacios sintéticos en PostgreSQL 18 (`EXPLAIN (ANALYZE, BUFFERS)`, 20 iteraciones): `bbox_map_250` `p50=0.16 ms` / `p95=0.19 ms`; `haversine_10km` `p50=22.62 ms` / `p95=23.15 ms`. Al estar `p95 < 50 ms`, se conserva Haversine + índice B-tree parcial (V123) sin migrar a PostGIS.
