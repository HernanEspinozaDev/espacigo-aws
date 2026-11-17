# Plan de fase temprana: arriendo por confirmaciones manuales

**Estado de corte — 17 de noviembre de 2026:** E0 completada; E1 parcial (contrato, candidatos de esquema y 82 casos obligatorios PostgreSQL desechables verificados); E2 con dominio/helpers/resolver, sin repositorio de escritura ni servicio; E3–E7 pendientes. Los candidatos no pertenecen al inventario aplicable V1–V98. No hay flujo temprano funcional de usuario ni aplicación persistente acreditada. Ver checklist y asignaciones en §10.1–10.3.
**Fecha:** 16 de noviembre de 2026.
**Base del plan:** `espacigo-aws`, rama `main`, HEAD `d4bfac43260dfc53f5298117ae33fff14e0f0d67` al redactarse; el checkout y el working tree se vuelven a comprobar antes de cada corte.
**Ámbito:** Next.js/BFF → monolito modular Go → PostgreSQL local `espacigo_aws_local`, con datos de prueba. Sin AWS real ni GCP.
**Autorización vigente:** el usuario aprobó comenzar el plan local. No habilita KYC documental, cobro real, cloud, publicación ni el uso de una base de datos de otro worktree sin reconciliar primero su procedencia.

## 1. Objetivo y decisión de producto

Permitir que una persona pruebe manualmente el recorrido completo con dos sesiones reales, reemplazando módulos todavía incompletos por casillas de confirmación. **Se simplifican los pasos externos, no la persistencia, la autorización ni la exclusión de calendario.**

Orden obligatorio del nuevo recorrido:

1. Seleccionar espacio y ticket/rango disponible; crear una oferta de arriendo de prueba.
2. Cada participante confirma su propia identidad mediante una casilla rápida.
3. Ambos leen y aceptan **la misma versión de contrato/oferta** mediante su propia casilla.
4. Sólo después de ambas aceptaciones se habilita el pago.
5. El arrendatario marca **«Pago enviado»**.
6. El arrendador marca **«Pago recibido»**.
7. La operación queda **«Arriendo confirmado — ensayo»**, con ocupación persistida y el mismo movimiento visible en el histórico de ambos paneles.

No requiere documentos, RUT, biometría, un proveedor KYC, firma electrónica, pasarela, tarjeta, garantía, liquidación o PDF para avanzar. No se deben sembrar aprobaciones documentales reales ni fabricar eventos de un proveedor para conseguirlo.

**Leyenda permanente:** «Fase temprana de prueba: identidad, aceptación y pago son confirmaciones manuales simuladas. No hay verificación real de identidad, firma electrónica ni movimiento de dinero».

Esta propuesta cambia deliberadamente el orden del ensayo anterior. **No es sólo cambiar etiquetas del frontend:** el backend debe imponer contrato bilateral antes de pago y PostgreSQL debe conservar ese orden.

## 2. Relación con el estado actual

### 2.1. Hechos comprobados en el código base

| Fuente existente | Lo que demuestra | Consecuencia para esta fase |
|---|---|---|
| `internal/adapters/postgres/booking/repository.go`, métodos `Quote`, `Create`, `Decide`, `Get`, `List` | El ensayo actual persiste cotizaciones, reservas, ocupación e historial; `Create` empieza en `pendiente_de_pago` | Reutilizar conceptos y helpers puros; no declarar que soporta el orden nuevo sin cambios |
| `internal/booking/service.go:1272` y `internal/identityverification/service.go:57` | La aprobación actual exige verificación documental de ambas partes | La casilla temprana NO debe escribir una aprobación documental ni desactivar este guard global |
| `frontend/src/features/bookings/ui/reservation-lifecycle-panel.tsx:293`, `:386`, `:581` | UI actual: pago, decisión del anfitrión y después contrato/firmas | No reutilizar esas acciones como si fueran contrato antes de pago |
| `frontend/src/features/bookings/ui/reservations-list.tsx` | Existe un listado de reservas con modalidad de arrendatario/anfitrión, filtros y actualización | Mantener la composición visual; añadir presentación del nuevo flujo sin mezclar estados |
| `frontend/src/app/(workspace)/reservas/page.tsx:5`, `frontend/src/app/(workspace)/anfitrion/reservas/page.tsx:5` | Existen las dos bandejas reales `/reservas` y `/anfitrion/reservas` | Integrar en ambas; no dejar la oferta temprana visible sólo para quien la creó |
| `frontend/src/app/(workspace)/mi-cuenta/account-navigation.tsx:16` y `frontend/src/features/auth/domain/return-target.ts:35` | La navegación actual no contiene Transacciones de prueba; los destinos tras login están allowlistados | Añadir enlace y destinos internos validados al implementar; una nueva página sin navegación/return seguro no cierra la integración |
| `frontend/src/features/bookings/ui/reservation-lifecycle-panel.tsx:263` | Existe historial de transiciones dentro del detalle | No confundirlo con un histórico financiero bilateral de «enviado/recibido» |
| `db/migrations/V000009__m06_draft_manual_occupancy.sql:12` | `ocupacion` tiene rangos `[inicio,termino)` y exclusión GiST global por espacio mientras están activos | Debe seguir siendo la única autoridad de inventario, también para este flujo |
| `db/migrations/V000011__m06_local_booking_trial.sql:56` | `ocupacion.reserva_id` referencia `reserva_ensayo_local`; los CHECK enlazan tipo y reserva | Un agregado nuevo NO puede escribir allí su ID como si fuera una reserva antigua; hace falta una migración aditiva explícita |
| `internal/adapters/postgres/booking/expiry/expire.go:17` | La expiración antigua actúa sobre reservas y sus referencias concretas | No encargarle la expiración temprana por accidente; inventariar todos los escritores antes de habilitarla |
| `internal/rentaldemo/store.go` | La demo usa un Store con mutex y estado en memoria | Puede servir de referencia de interacción, nunca de prueba de PostgreSQL ni almacén del nuevo flujo |
| `cmd/api/commercial_rental_flow.go:27` | El modo comercial tiene condiciones propias y es incompatible con `LOCAL_BOOKING_TRIAL=1` | No activar ambos flags ni el modo comercial para sortear los requisitos del ensayo |

Las cuatro reservas históricas del ensayo anterior siguen siendo válidas como evidencia de aquel recorrido. **No se reescribirán, migrarán de significado ni marcarán retroactivamente como contrato previo al pago.**

### 2.2. Alcance exacto

Incluido: oferta/ticket con snapshot, dos participantes, identidad rápida simulada, aceptación bilateral, envío/recepción manual de pago, registro transaccional, ocupación compartida, caducidad, rechazo/cancelación antes de confirmación, panel de ambos y lectura tras reiniciar.

Excluido: identidad oficial, fotos/documentos, garantías, firma legal, PDF, cobros/reembolsos reales, conciliación bancaria, saldo disponible real, comisiones, liquidación al dueño, reclamos, check-in/check-out físicos y ampliaciones automáticas de unidades de arriendo.

Se conservan las unidades existentes `hora/dia/mes`; no se inventa `semana`. La aceptación inicial obligatoria se hace por horas. Antes de habilitar día/mes en este nuevo recorrido deberán pasar sus casos reales de precio, zona y disponibilidad; si no pasan, la UI informa explícitamente que esa modalidad aún no está habilitada en fase temprana. Nunca convertir una selección válida en otra duración silenciosamente.

## 3. Experiencia propuesta y responsabilidades

### 3.1. Un solo detalle de operación, dos perspectivas

El mismo arriendo tiene un ID estable. El arrendatario y el arrendador ven el mismo espacio, rango, precio, versión de contrato, estado y línea de tiempo. Cambian las acciones permitidas y la perspectiva del movimiento, no los datos fuente.

La oferta aparece en ambas bandejas desde su creación, aunque todavía falten identidades. No esperar al pago para que el arrendador descubra la oferta.

| Sección | Arrendatario | Arrendador |
|---|---|---|
| Identidad | `[ ] Confirmar mi identidad para este ensayo` | `[ ] Confirmar mi identidad para este ensayo` |
| Resultado propio | «Identidad confirmada — simulación» | «Identidad confirmada — simulación» |
| Identidad de contraparte | Estado de sólo lectura | Estado de sólo lectura |
| Contrato | `[ ] Acepto este contrato de prueba` | `[ ] Acepto este contrato de prueba` |
| Antes de la segunda aceptación | «Esperando aceptación de la contraparte»; pago bloqueado | Mismo estado, pago bloqueado |
| Pago | `[ ] Pago enviado — simulación` | «Esperando envío del arrendatario» |
| Tras envío | «Pago enviado; esperando confirmación de recepción» | `[ ] Pago recibido — simulación` |
| Tras recepción | «Arriendo confirmado — ensayo» | Mismo resultado y mismo ID de transacción |

La identidad rápida se registra **por participante y por operación temprana**, no como badge KYC general de la cuenta. Esto evita atribuir a todo el producto una aprobación de identidad que sólo era una casilla de ensayo. No reutilizar el badge documental público.

Cada casilla es una intención explícita enviada a Go. No aparece preseleccionada ni confirma al otro usuario. Una confirmación persistida pasa a sólo lectura; desmarcarla no deshace un contrato o pago. Antes de guardar, el estado local puede cambiar; después, una corrección necesita una transición explícita, no borrar el registro.

### 3.2. Confirmación visible y fallos

- Al marcar: «Guardando…», control deshabilitado y sin avance optimista de fase.
- Sólo después de commit y GET independiente: casilla confirmada, hora del servidor y nuevo estado.
- Un error antes de dispatch deja el control sin confirmar.
- Si la respuesta se pierde después de enviar: «Resultado pendiente de verificar»; bloquear nuevas acciones y ofrecer **«Actualizar estado»**, sin repetir automáticamente el POST.
- Recarga, nueva pestaña y nuevo login deben reconstruir el estado desde Go/DB, nunca desde localStorage o una bandera React.
- En la sesión de la contraparte: actualizar al volver al foco, botón manual y polling acotado mientras la operación esté pendiente y la pestaña visible; propuesta inicial cada cinco segundos. No hace falta WebSocket para este MVP.
- No modificar el diseño global ni crear otro dashboard de muestra. Conservar componentes/tokens del panel actual, labels accesibles, teclado, estados pendientes, contraste y `break-words`/contención móvil.

### 3.3. Entrada pública, rutas y bandejas

**Rutas siguientes propuestas, no existentes:** `/reservas/tempranas/nueva` para preparar la oferta con la selección autoritativa; `/reservas/tempranas/{id}` para el detalle común autorizado a ambas partes. Se conserva el ID/rol en el DTO del servidor, no se deduce autorización del nombre de la URL.

La CTA pública sólo dirige al nuevo recorrido cuando Go confirma que el modo temprano está habilitado para esa sesión/entorno y unidad. Sin sesión, primero se ofrece acceso conservando la selección; tras autenticar se consulta capabilities antes de decidir el recorrido. En caso contrario conserva el destino actual y sus límites, sin forzar un bypass con una query del navegador. Conservar exactamente los timestamps/zona del handoff, validar UUID y parámetros y añadir los nuevos destinos a `return-target.ts`; después del login se retorna a la oferta correcta, sin open redirects.

`/reservas` y `/anfitrion/reservas` incorporan una pestaña/filtro «Fase temprana» con las ofertas enviadas/recibidas, además del acceso al histórico. El detalle temprano **no** se envía a `ReservationLifecyclePanel` ni al adaptador de booking-trial: esos componentes también solicitan garantías, liquidaciones, firmas y reloj del flujo anterior. Reutilizar composición visual, no sus efectos/acciones. Usar una unión discriminada por tipo de flujo o viewmodels separados; no forzar nuevos estados mediante casts sobre `BookingState` ni mostrarlos con badges legacy incorrectos.

## 4. Máquina de estados autoritativa

El modelo de `internal/earlyrental/state.go` cubre identidad, aceptación bilateral, envío/recepción simulados, rechazo, cancelación y la transición pura de vencimiento idempotente; también rechaza snapshots cuyo estado, confirmaciones y timestamps describen un historial imposible. El worker, la liberación transaccional de ocupación y la persistencia siguen pendientes. `GET /api/v1/local/early-rentals/capabilities` ya está montado sólo junto al runtime local autenticado: apagado responde `enabled=false`; solicitar activación antes de que el flujo esté listo devuelve `503`, sin anunciarlo habilitado. No hay handlers de mutación ni persistencia. El resto del contrato fuente en `planning/openapi.yaml` sigue siendo propuesto; los artefactos frontend están sincronizados en `frontend-next-hermes` para 230 operaciones.

| Estado | Entrada permitida | Condición para avanzar | Siguiente estado |
|---|---|---|---|
| `pendiente_identidad` | Oferta creada con snapshot y retención temporal | Ambos participantes confirman su propia identidad simulada | `pendiente_contrato` |
| `pendiente_contrato` | Identidades completas | Ambos aceptan el mismo `contract_version` y digest | `pendiente_envio_pago` |
| `pendiente_envio_pago` | Contrato bilateral aceptado | Sólo el arrendatario confirma envío | `pendiente_recepcion_pago` |
| `pendiente_recepcion_pago` | Envío persistido | Sólo el arrendador confirma recepción, antes del vencimiento | `confirmado` |
| `confirmado` | Recepción y ocupación definitiva confirmadas en una transacción | Fin de este MVP | Sin transición física automática |
| `rechazado` | Rechazo explícito de oferta/contrato pendiente | Terminal; liberar retención y conservar historial | — |
| `cancelado` | Cancelación por participante antes de `confirmado`, con motivo | Terminal; liberar retención; pago enviado, si lo había, queda anulado sólo en el ensayo | — |
| `vencido` | Deadline superado antes de recepción | Terminal; liberar retención y conservar envío sin recepción | — |

Las confirmaciones individuales de identidad/contrato generan eventos aunque no cambie la fase global. Así ambos paneles pueden mostrar quién falta sin inventar una nueva operación.

Reglas de orden:

1. Un usuario no puede ser simultáneamente ambas partes de la misma operación.
2. No aceptar contrato si falta cualquiera de las identidades.
3. No enviar pago con una sola aceptación, una versión distinta o una oferta terminal.
4. No recibir pago antes de envío ni por el arrendatario.
5. «Enviado» no implica «recibido»; ninguna marca inicia un pago de proveedor.
6. No reabrir una operación vencida/cancelada mediante una respuesta tardía.
7. La oferta es inmutable en esta primera fase. Cambiar fechas, monto, partes o condiciones exige cancelar y crear otra oferta; no conservar aceptaciones sobre otros términos. Versionar el contrato igualmente para rechazar payloads obsoletos y permitir evolución posterior.
8. Las cuentas deben seguir activas y autorizadas en cada mutación; la casilla no permite eludir bloqueo de cuenta, propiedad o acceso al recurso.

## 5. Integración elegida: flujo aditivo, no adulterar el ensayo antiguo

**Decisión aprobada:** agregado `earlyrental` aditivo dentro del monolito, con PostgreSQL y el inventario existente. El paquete de dominio ya comenzó; los nombres de tablas y rutas aún no son contratos implementados.

Por qué no sólo reutilizar las acciones antiguas:

- El ensayo actual empieza por pago y después aprobación/contrato; sus CHECK, TTL, garantías, verificador documental y workers siguen ese orden.
- Forzar estados tempranos en registros antiguos o fabricar un `exito_simulado` al marcar «enviado» falsearía recepción, auditoría y obligaciones financieras.
- Un Store separado en memoria incumple el objetivo de persistir y sincronizar ambos paneles.

La alternativa de añadir `flow_mode` a todas las tablas/consumidores antiguos sólo se reconsiderará en el gate técnico inicial si una revisión demuestra una delta menor **sin** excepciones dispersas, estados ambiguos o afectación del histórico. El plan no deja ambas implementaciones abiertas en paralelo: la base de trabajo propuesta es el agregado aditivo.

**Activación aprobada:** `LOCAL_EARLY_RENTAL_FLOW_ENABLED=1`, desactivada por defecto, sólo en el runtime local autenticado estándar (`APP_PROFILE` vacío, `LOCAL_AUTH_PROTOTYPE=1`); se rechaza en perfiles cloud y runtimes dedicados. Hoy la ruta de capacidades devuelve `503` si se solicita activar el flujo, porque persistencia, transacciones y handlers aún no están listos. No se cambia el valor ni la incompatibilidad de los flags comerciales existentes. La convivencia con el ensayo histórico debe probar aislamiento de IDs, workers y acciones; si no puede demostrarse, el modo temprano no se habilita hasta resolverlo. No usar un perfil que exija DB loopback por nombre cuando el target autorizado es el contenedor local: validar el target real y su política, no sustituir la DB.

El frontend obtiene de Go las capacidades/métodos disponibles. Ocultar una casilla no constituye la protección. Al apagar la bandera: ninguna nueva mutación temprana; las operaciones históricas siguen consultables para sus participantes, claramente etiquetadas. Reactivar no extiende deadlines ni reproduce comandos.

## 6. Persistencia propuesta y sincronización con calendario

### 6.1. Modelo mínimo aditivo

| Objeto propuesto | Contenido y restricciones principales |
|---|---|
| `arriendo_temprano` | UUID, `flow_version=manual_early_v1`, espacio, arrendador, arrendatario distintos; snapshot de título/condiciones/tarifa/versionado, modalidad, unidades, CLP exactos, zona, inicio/fin; estado; revisión monotónica; creación/actualización/deadline; referencia de ocupación |
| `arriendo_temprano_confirmacion` | Operación, participante, clase identidad/contrato, versión/digest aplicables, timestamp del servidor y clave de operación; unicidad de la confirmación por actor/clase/versión; no PII/documentos de identidad |
| `arriendo_temprano_pago` | Un único movimiento por arriendo: ID estable, importe/moneda tomados del snapshot, estado pendiente/enviado/recibido/anulado/vencido, emisor/receptor derivados de las partes, timestamps de envío/recepción; no datos bancarios |
| `arriendo_temprano_evento` | Eventos append-only, secuencia por operación, actor o sistema, acción, estado anterior/nuevo, versión de contrato, importe cuando corresponda, momento, correlación y referencia de idempotencia |
| Registro de idempotencia del agregado | Actor + operación/comando + clave + huella de payload, resultado/evento confirmado; puede residir en el evento si resuelve también comandos cuyo replay no debe agregar otro evento |

Referencias de cuentas hacia `public.usuario(id)`, nunca otra tabla de usuarios. Monto/unidades/tiempo no se aceptan del browser como autoridad; se calculan/validan en Go contra las fuentes vigentes. El cliente no asigna actor, rol, estado, timestamp ni éxito financiero.

En una misma transacción se guardan: cambio de agregado, confirmación/pago, eventos, revisión e impacto de ocupación. Un fallo provoca rollback completo. El histórico se consulta desde esos registros, no desde copias divergentes para cada panel.

### 6.2. Única autoridad de ocupación

Reutilizar **`public.ocupacion` y su exclusión GiST**, no una segunda tabla de reservas libres que el calendario actual ignore.

Delta a diseñar/revisar en una migración nueva:

- Agregar un vínculo nullable propuesto `arriendo_temprano_id`, con FK hacia el agregado temprano y unicidad por operación.
- Añadir tipos distinguibles propuestos `retencion_temprana` y `reserva_temprana`.
- Conservar intacta la semántica legacy: bloqueo manual sin reserva; `retencion/reserva` con `reserva_id` viejo; tipos tempranos con **sólo** `arriendo_temprano_id`, sin `reserva_id` viejo. Los CHECK deben rechazar todos los vínculos ausentes, dobles o incompatibles, incluidos NULL parciales.
- Mantener el mismo rango `[inicio,termino)`, exclusión para **todo** registro activo sin filtrar por origen, marca de desactivación y referencias coherentes de espacio/intervalo.
- Decidir FK circular diferida o inserción ordenada del vínculo sin ventana de commit incompleta; no deshabilitar integridad para conseguirlo.
- Auditar writers/lectores, expiración, eliminación de espacio, privacidad y cualquier trigger que asuma `reserva_id` o sólo tres tipos; no suponer que agregar un CHECK basta. Revisar todas las versiones posteriores de funciones, no sólo V9/V11.
- Lectores mensual/de intervalos y calendario anfitrión deben descontar las ocupaciones tempranas activas igual que las demás. No mostrar actores/pagos en DTO público.

#### Inventario E1 de ocupación revisado

- V9 crea `ocupacion_tipo_reserva_ck`, `ocupacion_reserva_id_uk` y la exclusión GiST por `(espacio_id, intervalo)` para todas las filas activas. V11 añade la FK diferida `ocupacion_reserva_ensayo_fk`; la búsqueda de migraciones no encontró otra alteración de `ocupacion` entre V12–V98.
- Escritores de producción observados: `occupancy.Repository` inserta y desactiva sólo `bloqueo_manual`; `booking.Repository` crea la retención legacy enlazada a `reserva_id`; `booking/payment_inbox.go`, `booking/expiry/expire.go`, `booking/guarantee.go` y `contracts.Repository` convierten/liberan ocupación por `reserva_id`. Esos recorridos no deben recibir IDs tempranos ni reutilizarse para vencimiento temprano.
- Lectura pública: `rentalflow.Repository.PublicIntervals` y `spaces/availability.go` filtran toda ocupación activa por espacio/rango, por lo que una fila temprana puede descontarse sin revelar su vínculo. La configuración privada de bloqueos sólo proyecta `tipo='bloqueo_manual'`. En cambio, `operations/returns.go` y el guard de publicación de `spaces.Repository` son explícitamente legacy (`reserva_id`, `tipo='reserva'`); la integración deberá confirmar que la despublicación detiene ofertas nuevas sin borrar ni romper ofertas ya creadas.
- `runtime_permissions.go` concede actualmente a `espacigo_runtime` SELECT/INSERT/UPDATE sobre `ocupacion`, sin DELETE. La migración y su verificador ACL deben añadir sólo accesos mínimos a las tablas nuevas y comprobar el contrato efectivo; no ampliar a `ALL`.
- Propuesta de integridad para el próximo esquema: almacenar el `intervalo tstzrange` canónico en el agregado; FK compuestas diferidas en ambos sentidos que vinculen agregado, ocupación, espacio e intervalo exactos; `UNIQUE` para la relación 1:1; ampliar el CHECK para dos tipos tempranos con sólo `arriendo_temprano_id`. Mantener la exclusión GiST original y no actualizar filas históricas. Esta propuesta requiere prueba PostgreSQL de inserción atómica, FK mismatch, rollback y conflicto de solape antes de cerrar E1.
- El recorrido inicial sigue limitado a `hora`. Hasta cerrar la semántica de precio efectivo de V98, creación temprana debe rechazar intervalos cubiertos por un override con `tarifa_personalizada_clp`, nunca guardar `Listing.UnitPriceCLP` como fallback silencioso. La cotización legacy no se modifica en esta fase.
- El agregado exige `CalendarRateOverrideAbsent` en `OfferSnapshotInput`; `Unresolved` y `Present` se rechazan. `rentalflow.Repository.ResolveEarlyRentalOfferSnapshot` ya resuelve los hechos desde PostgreSQL dentro de una transacción del llamador: bloquea cuentas en orden estable y el espacio con `FOR SHARE`, revalida publicación/listado, estado de ambas cuentas, precio base CLP por hora, intervalo público exacto y ocupación; consulta overrides personalizados por todas las fechas locales realmente cubiertas por el intervalo half-open. Solo devuelve el estado ausente resuelto; un override personalizado devuelve conflicto y errores de lectura abortan. El llamador debe mantener la misma transacción hasta persistir oferta y ocupación; aún no hay creador ni handler que lo invoque.
- Verificación de este corte: `go test -race` y `go vet` de `rentalflow`/`earlyrental` pasaron; `scripts/test-migration-runner-postgres.sh full` pasó en contenedor PostgreSQL desechable sin red/puertos/volúmenes y aplicó el harness V1–V98 más V98 al fixture rentalflow V1–V61. La integración confirmó rechazo de tarifa personalizada en la fecha tocada y aceptación del intervalo que termina a medianoche local cuando el override solo existe en la fecha siguiente; el contenedor y scratch propios se eliminaron y verificaron. No se conectó a ninguna base persistente.
- Primer ensayo del delta en PostgreSQL descartable (16 de noviembre de 2026): el runner llegó a V99 y `RunWithRole(..., "espacigo_schema_owner")` falló antes de ejecutar DDL con `42501: must be owner of table ocupacion`. El fixture combinado aplica V1–V80 como `postgres` antes del cambio de rol; por tanto, no es válido “arreglar” la prueba transfiriendo ownership a `espacigo_schema_owner` ni ejecutar V99 con admin para declarar compatibilidad. El camino histórico pre-V81 también es administrativo; la propiedad del `ocupacion` real sigue sin leerse, y la base persistente asociada a otro worktree permanece fuera de alcance. Se retiró el candidato V99 y sus cambios de conteo/harness; no hubo aplicación de DDL ni quedó un ledger parcial. Antes de asignar versión nueva hay que diseñar y revisar un camino de autoridad que preserve este límite y pruebe tanto fresh como upgrade, o cambiar el delta sin romper la exclusión compartida. La ejecución completa con el candidato falló por ese gate; después de retirarlo, el runner V1–V98 volvió a pasar, lo que no valida el delta temprano. El contenedor/scratch descartable se verificó eliminado.

- Prueba de factibilidad de autoridad (17 de noviembre de 2026): en el harness PostgreSQL 18.7, una migración sintética V99 ejecutada por el runner dentro de una sola transacción pudo `SET ROLE espacigo_schema_owner` para crear su tabla, `RESET ROLE` para alterar una tabla `ocupacion` propiedad de `postgres` y registrar el ledger como `postgres`; el postcheck conservó ambos propietarios. Un fallo deliberado después de ambas alteraciones revirtió tabla, columna y ledger. El scope `authority-window` pasó y verificó eliminación exacta del contenedor y scratch. **Esto prueba la mecánica en un fixture desechable, no implementa ni autoriza un ejecutor operativo, no valida la base persistente ni es una migración real V99.** No asignar versión ni reintroducir candidato hasta revisar un executor exacto y el contrato completo.

#### Avance del ejecutor acotado — 17 de noviembre de 2026

`internal/migrator/single_owner_migration.go` incorpora `RunSingleOwnerMigration` como biblioteca no conectada al CLI ni al runtime. Recibe una conexión dedicada y un contrato previamente revisado; fija host/puerto, nombre y OID de base, identificador del clúster, versión exacta de PostgreSQL 18, usuario de sesión, propietarios de `ocupacion`/ledger, inventario completo y una única versión/nombre/checksum pendiente. Comprueba el prefijo aplicado bajo advisory lock y locks de tablas; no inicializa ledger, concede privilegios ni transfiere propietarios.

La migración debe ser un único bloque `DO $migration$`, ejecutado mediante `PgConn.ExecParams` para que PostgreSQL rechace múltiples sentencias top-level. El bloque puede alternar roles para DDL revisado, pero debe restaurar el rol inicial y conservar los propietarios legacy. DDL y ledger comparten transacción; después del commit se releen identidad/propietarios y ledger en otra transacción read-only. El resultado distingue commit confirmado de postcheck verificado; un error de commit exige reconciliar, no repetir automáticamente.

Pruebas del nuevo corte: unitarios con rechazo de contratos incompletos y SQL alterado; harness `full` con commit y lectura desde otra sesión, rollback por fallo DDL o ledger, rechazo de clúster/OID/motor/owner/prefijo distintos, replay, múltiples sentencias, escape transaccional y rol no restaurado. El ensayo detectó que `pgx.Exec` sin parámetros fuerza simple protocol incluso con `QueryExecModeExec`; se corrigió usando `PgConn.ExecParams` y el harness completo volvió a pasar. Contenedor y scratch propios eliminados con verificación. No se accedió a la base persistente ni se creó una migración V99.

**E1 no está cerrada:** falta revisar la integración del ejecutor con el delta real y sus ACL/invariantes fresh/upgrade. El contrato de destino de prueba no autoriza un destino persistente; siguen pendientes reconciliación de procedencia, backup/preflight y una ruta operativa explícita. La restricción sobre esa base no impide seguir desarrollando código y pruebas desechables.

La revisión independiente detectó que un campo `SourceSHA` suministrado por el llamador sólo validaba formato y no probaba procedencia Git. Se retiró del contrato de esta primitiva: fija los bytes del inventario/SQL, **no** certifica un commit. El futuro comando operativo debe verificar por separado el vínculo entre commit autorizado y manifest revisado, además de backup/ACL/procedencia del destino; no basta pasarle un SHA arbitrario.

- Requisitos restantes de integración operativa del ejecutor: preflight READ ONLY externo, backup y manifest de procedencia; revisión de owner/ACL del ledger y membresías/atributos requeridos para el delta real. La biblioteca implementa los pins y la transacción descritos arriba, pero no abre ventanas de grants ni verifica el contrato ACL completo de una migración todavía inexistente. Los valores exactos del destino y la ruta CLI siguen pendientes; no existe un comando aprobado aplicable a persistencia.

#### Candidato de agregado y enlace de inventario — 17 de noviembre de 2026

Sobre HEAD `00c3e21723cd44a3dc7dbdbb74f2d5d9f1b866ab`, se preparó `internal/migrator/testdata/early_rental_schema_candidate.sql`, **fuera de `db/migrations` y sin asignar una versión operativa**. El test construye una V99 únicamente en `t.TempDir` para ejercer `RunSingleOwnerMigration`; esto no reserva ese número ni autoriza aplicación persistente.

Este corte cubre sólo `arriendo_temprano` y su enlace bidireccional a `ocupacion`: FK compuestas diferidas atan IDs, espacio e intervalo exactos; CHECKs distinguen tipos tempranos y legacy y conservan la exclusión GiST universal. El candidato añade restricciones estructurales de snapshot horario/CLP, digest SHA-256 del contrato y deadline no posterior al inicio ni a treinta minutos de creación. Conserva owners legacy, crea el agregado como `espacigo_schema_owner` y concede al runtime SELECT/INSERT/UPDATE, no DELETE/TRUNCATE. La fixture abre y cierra sus prerrequisitos DDL temporales y verifica su revocación.

**Prueba PostgreSQL ejecutada por el padre:** `scripts/test-migration-runner-postgres.sh full` pasó en contenedor desechable aislado. El checker exige **26 casos ejecutados, ninguno omitido**, en baselines vacía y V75 actualizadas por la secuencia real V1–V98: par oferta/ocupación confirmado y leído desde otra sesión; rollback por vínculo ausente en ambas direcciones, espacio/rango/ID distintos, segundo vínculo inverso inactivo, NULL parcial, tipo incorrecto, motivo no permitido, retención sin vencimiento y solape; inserción legacy manual y exclusión entre legacy/temprano. Los negativos verifican SQLSTATE/constraint y conteos anteriores/posteriores desde otra sesión. También pasaron `go test -race` y `go vet` de migrator/earlyrental y `git diff --check`. El contenedor y scratch propios se eliminaron y su ausencia se verificó.

Las primeras entregas delegadas dejaron skips y transacciones incompletas; se rechazaron como evidencia, se completaron en el padre y se ejecutó el harness completo. El primer intento nativo identificó que algunas FK diferidas y CHECKs fallaban por una restricción distinta a la anticipada; se corrigieron las expectativas, no se debilitaron las restricciones. Las hipótesis de que se aceptaría motivo no nulo o expiración NULL se contrastaron con negativos PostgreSQL que las rechazan.

La revisión independiente de SQL y la revisión de tests/checker terminaron favorables, sin bloqueadores concretos. Se usaron subagentes `gpt-6-luna` con razonamiento `medium`, comprobado en la configuración de delegación. El checker también se ejerció con evidencia completa, ausente y omitida: sólo aceptó la completa. La aprobación de este corte candidato no cierra E1 ni habilita aplicación persistente.

**Límites del primer corte:** sus pruebas no acreditaban todavía evidencia auxiliar ni coherencia de fase/inventario. El corte siguiente añade esas estructuras y pruebas, sin convertirlas en handlers/worker ni en una migración habilitada. La convivencia legacy probada incluye bloques manuales y sus solapes; no equivale a repetir todo el ciclo de reservas antiguas.

#### Solicitud de cierre completo de E1 — 17 de noviembre de 2026

El usuario pidió terminar E1 completa. Se solicitó explícitamente sustituir la restricción sobre `espacigo_aws_local` del contenedor asociado al otro worktree por inspección, backup y aplicación revisada; el formulario terminó **sin respuesta (`timed_out`)**. No se interpretó el silencio como autorización, no se repitió la solicitud y no se conectó al destino. Esto no impidió continuar el trabajo técnico independiente.

Se amplió el candidato base con un trigger de inmutabilidad de snapshot/partes/intervalo/deadline y triggers diferidos en ambos lados del vínculo: ofertas pendientes conservan retención activa y el mismo vencimiento; confirmadas exigen reserva activa sin expiry; terminales exigen retención inactiva. Las FK siguen acreditando la asociación exacta y no se altera el comportamiento legacy. El fichero auxiliar `internal/migrator/testdata/early_rental_evidence_candidate.sql` añade confirmaciones, movimiento único, eventos append-only e idempotencia ligada al mismo actor/comando/evento; los permisos del runtime se limitan a sus accesos previstos, sin DELETE. La identidad simulada no almacena datos contractuales; la aceptación contractual exige versión/digest exactos. Los SQL siguen fuera del inventario aplicable; el test usa versiones 99/100 sólo en directorios temporales.

Verificación del árbol combinado: el harness `full` pasó y exigió **60 casos nominales fresh/upgrade ejecutados, ninguno omitido**, incluyendo los 26 iniciales, inmutabilidad/coherencia y cuatro grupos de evidencia auxiliar por baseline. Los negativos nuevos exigen SQLSTATE específico y conteos independientes; la recepción de pago se lee después de commit y el evento no admite UPDATE/DELETE. Se verificaron también el checker completo/ausente/skip, `go test -race` y `go vet` de migrator, dbmigrate, earlyrental, rentalflow y cmd/api, build de API, `api:check`, typecheck y diff-check. La revisión independiente del árbol ampliado sigue como gate separado; la ejecución desechable no prueba el destino persistente.

La revisión ampliada encontró una brecha real no cubierta por los primeros 60 casos: eventos sin referencia de evidencia y huecos de secuencia. La entrega delegada correctiva no la implementó; el padre completó la corrección y repitió el harness completo. El guard de eventos ahora bloquea la fila del agregado antes de exigir la siguiente secuencia y continuidad de estado; identidad/contrato requieren una confirmación de la misma oferta/actor/clase y envío/recepción requieren el mismo movimiento. Triggers diferidos verifican al commit que cantidad/última secuencia, revisión y estado final coincidan con el agregado. El evento inicial se inserta junto a oferta/ocupación en la fixture, no después de un commit incompleto.

La segunda verificación pasó con **80 casos obligatorios fresh/upgrade, ninguno omitido**. Los veinte casos adicionales cubren, en ambas baselines, huecos, referencias ausentes o de otro actor/clase, falta de pago, estado previo/siguiente incorrecto, evento sin actualización de revisión, revisión sin evento y oferta sin evento inicial; verifican rollback y relectura independiente de revisión/estado. Esta evidencia sustituye el alcance incompleto de los 60 casos, no lo presenta como un cierre previo válido.

La revisión correctiva `deleg_8b49759c` devolvió `passed=false`, pero mezcló observaciones con su propia retractación y una afirmación incorrecta sobre la ausencia de control de revisión en el agregado: el control existente está en `internal/migrator/testdata/early_rental_schema_candidate.sql`, función de inmutabilidad, y exige revisión anterior +1 cuando cambia estado. No se toma ese resultado como aprobación ni se descarta automáticamente. Su contraejemplo sí identificaba un avance prematuro de fase con una sola identidad confirmada. Se reforzó el guard para derivar el siguiente estado de las confirmaciones ya registradas de ambos participantes, tanto para identidad como para contrato, e impedir eventos repetidos del mismo tipo/actor.

La ejecución combinada posterior pasó con **82 casos obligatorios fresh/upgrade, ninguno omitido**, incluyendo `schema-bilateral-lifecycle` en ambas baselines. Este caso rechaza con SQLSTATE `23514` los avances anticipados de identidad y contrato aunque el escritor actualice coordinadamente estado/revisión/evidencia/evento; comprueba rollback con lector independiente y después ejecuta el recorrido SQL válido de ambas identidades, ambas aceptaciones contractuales, envío y recepción del mismo pago y conversión de la ocupación en reserva activa. Es evidencia del esquema desechable, no de handlers, UI o E2 implementados. El informe de revisión no queda convertido en favorable por estos cambios: el gate independiente permanece separado, al igual que los pendientes operativos de E1.

| Criterio E1 | Estado observado |
|---|---|
| OpenAPI y artefactos frontend sincronizados | Verificado: 230 operaciones, sin afirmar que las futuras rutas estén montadas |
| Precio personalizado tratado sin fallback silencioso | Verificado por resolver y pruebas PostgreSQL previas |
| Delta de esquema y asociación con inventario | Candidatos implementados y probados en PostgreSQL desechable, todavía fuera de `db/migrations` |
| Integridad, rollback, solape y ACL de las nuevas estructuras | Harness combinado verificado; revisión de publicación favorable tras reevaluación `deleg_3f46fe45`. Contrato completo de autoridad operativa todavía pendiente |
| Ejecutar una sola migración con target/owner/checksum/ledger exactos | Primitiva probada; falta el comando operativo con procedencia/backup y contrato de autoridad del destino real |
| Reconciliar procedencia, ledger y owner/engine del destino persistente | Bloqueado por restricción específica vigente, no por falta de autorización general de desarrollo |
| Backup y asignación de versión aplicable contra ese ledger | Pendiente del punto anterior; no inventar una versión libre ni afirmar backup efectuado |

**E1 permanece parcial, no cerrada.** El bloqueo operativo y los faltantes del comando/migración se conservan explícitos; no se trasladan a E2 ni se redefinen los criterios para simular cierre. Retención/borrado del ciclo FK, atomicidad de la máquina completa entre agregado/evidencias/eventos e idempotencia de servicio necesitan su integración posterior; estas pruebas son de esquema, no del recorrido UI→BFF→Go→DB.

Crear oferta toma el bloqueo compartido de espacio/calendario usado por escritores competidores, revalida publicación, horario, overrides, tarifa/versionado y ausencia de ocupación, e inserta la retención en la misma transacción. La exclusión de PostgreSQL sigue siendo la última defensa frente a dos usuarios simultáneos.

Al confirmar recepción, convertir **la misma ocupación** a definitiva, sin insertar otra ni abrir un hueco; mantener activo el intervalo. Cancelación/rechazo/expiración desactiva la retención correspondiente en el mismo commit que su evento. No afectar ocupaciones ajenas ni las cuatro reservas históricas ya verificadas.

### 6.3. Caducidad y precios

Regla aprobada: retención máxima de treinta minutos desde creación, limitada siempre por el inicio del intervalo, sin renovación automática por polling/clic. `earlyrental.NewOffer` calcula esa expiración de dominio, devuelve el evento inicial `oferta_creada` en secuencia/revisión 1 y `capabilities` publica el mismo `HoldTTL`; `Apply` y `Expire` emiten tipos de evento alineados con el contrato OpenAPI y secuencia igual a la revisión. Esto aún no persiste eventos, una retención ni reserva ocupación. El servidor devolverá `expires_at` y la UI mostrará contador informativo. Caducar oferta y liberar ocupación es trabajo transaccional de un worker temprano acotado; cada comando también verifica el deadline bajo bloqueo. El contador del browser no libera nada por sí mismo. Con el worker detenido no se aceptan comandos vencidos; las lecturas no deben prometer libre un rango todavía retenido en DB.

Un pago enviado sin recepción al vencer se conserva como **«Envío simulado no confirmado; operación vencida»**. No contabilizarlo como ingreso recibido ni crear un reembolso bancario ficticio. Una confirmación tardía de recepción responde conflicto; requiere una nueva oferta con disponibilidad revalidada.

La tarifa debe conservar el snapshot vigente y el monto exacto. La migración V98 añade `espacio_calendario_override.tarifa_personalizada_clp`, pero el quote comercial actual toma `Listing.UnitPriceCLP` y la lectura de restricciones consulta sólo el estado del override; por tanto, la tarifa personalizada por fecha no queda reflejada en ese camino. E1 debe resolverlo antes de crear ofertas tempranas: usar una tarifa efectiva verificada transaccionalmente o rechazar intervalos con precio personalizado. No cobrar silenciosamente la tarifa base ni recalcular históricos.

## 7. Contrato API — capabilities implementado; operaciones de arriendo aún propuestas

Prefijo Go propuesto `/api/v1/local/early-rentals`; BFF same-origin `/api/backend/local/early-rentals`. No usar las rutas de KYC real ni los callbacks de pago para estas casillas.

| Método / ruta propuesta | Actor | Efecto |
|---|---|---|
| `GET /local/early-rentals/capabilities` | Sesión autenticada | Informar modo habilitado, unidades admitidas y política de plazo; no crea ofertas ni aprueba identidades |
| `POST /local/early-rentals` | Arrendatario autenticado | Crear oferta + snapshot + retención + evento; 201 |
| `GET /local/early-rentals?role=…&cursor=…&limit=…` | Participante | Bandeja propia enviada/recibida; 200 |
| `GET /local/early-rentals/{id}` | Participante | Estado autoritativo, confirmaciones, contrato, pago, deadline, acciones y revisión; 200 + ETag |
| `POST /local/early-rentals/{id}/identity-confirmations` | Cada parte para sí | Confirmación simulada propia; 200 |
| `POST /local/early-rentals/{id}/contract-acceptances` | Cada parte para sí | Aceptación de versión y digest exactos; 200 |
| `POST /local/early-rentals/{id}/payment-sent` | Sólo arrendatario | Registrar envío simulado; 200 |
| `POST /local/early-rentals/{id}/payment-received` | Sólo arrendador | Registrar recepción + confirmar arriendo/ocupación; 200 |
| `POST /local/early-rentals/{id}/reject` | Participante de oferta pendiente | Rechazar, liberar y auditar; 200 |
| `POST /local/early-rentals/{id}/cancel` | Participante antes de confirmar | Cancelar con motivo, liberar y conservar historial; 200 |
| `GET /local/early-rentals/{id}/events?after=…` | Participante | Línea de tiempo con secuencia estable; 200 |
| `GET /local/early-rental-transactions?role=…&state=…&cursor=…&limit=…` | Cuenta autenticada | Movimientos propios con perspectiva ingreso/egreso; 200 |

Reglas de las mutaciones propuestas: `Idempotency-Key` obligatorio y `If-Match` fuerte para acciones sobre recurso existente. Creación: idempotencia sin inventar un ETag previo. La capa de dominio compara `ExpectedRevision` con la revisión cargada; un desajuste no produce evento y se traduce a 412. El adaptador PostgreSQL debe repetir la comparación bajo bloqueo/CAS. Casilla de identidad envía sólo intención de confirmación; contrato añade versión/digest; pago no acepta importe editable ni identidad de la contraparte. JSON exacto y DTOs tipados desde OpenAPI.

#### Reglas del contrato fuente para las operaciones aún sin handler

El contrato OpenAPI expresa estas reglas para las operaciones de oferta y sus comandos; no declara que estén montadas ni habilita tráfico antes de existir handler, persistencia y bandera local. La única excepción actual es `GET .../capabilities`, que consulta autenticación y estado de activación, pero nunca crea ofertas ni muta datos.

- `CreateEarlyRentalRequest` exige exactamente `space_id` (UUID), `start_at` y `end_at` (RFC3339 con offset). No acepta actor/rol, unidades, tarifa, monto, moneda, estado, revisión, vencimiento, confirmaciones ni datos de identidad. Go deriva participante, unidad, snapshot y monto desde publicación/tarifa/calendario vigentes. Sólo se admite `hora` en esta primera versión.
- Los comandos de identidad, envío y recepción aceptan `{}`. La futura capa de aplicación genera el UUID canónico del movimiento al enviar y reutiliza ese mismo ID al registrar recepción; `payment_id` nunca se acepta desde JSON. La aceptación contractual exige exactamente `contract_version` (entero positivo) y `contract_digest` (SHA-256 hexadecimal en minúsculas, 64 caracteres); `NewOffer` recibe sólo texto/versionado contractual del servidor, almacena el texto exacto (hasta 20.000 puntos de código) y deriva el digest; la futura capa debe seleccionar la versión inmutable y persistir texto/digest juntos. Rechazo/cancelación exigen `reason_code` de catálogo cerrado y no texto libre. Ningún comando acepta `actor_id`, timestamps, importe o identidad de contraparte.
- `EarlyRentalDetail` contiene `id`, `flow_type="manual_early_v1"`, `actor_role` (`arrendatario|arrendador`), snapshot inmutable (`space_id`, título, `rate_unit`, `unit_price_clp`, `currency="CLP"`, `units`, `subtotal_clp`, `start_at`, `end_at`, `time_zone`), `state`, `revision`, `last_event_sequence`, `created_at`, `updated_at`, `expires_at`, confirmaciones separadas por rol, contrato (`version`, `digest`, texto de ensayo y aceptaciones por rol), pago único (ID, importe/moneda, estado y timestamps enviados/recibidos) y `allowed_actions`. Las confirmaciones pendientes se representan con `null`; no se exponen RUT, documentos ni decisiones KYC.
- `EarlyRentalListEnvelope` y `EarlyRentalTransactionListEnvelope` contienen `items` y `next_cursor` nullable. Cada item conserva `id`/flujo y una sola fuente de monto; el tipo de perspectiva (`egreso|ingreso`) deriva del rol autenticado, nunca de un parámetro que permita consultar a otra cuenta. El cursor es opaco y está ligado por Go al actor, filtros y orden estable. En ambos listados, `role` es opcional: si se omite se devuelven todas las operaciones/movimientos propios de ambas perspectivas; si se especifica filtra por el rol del actor en cada oferta. `cursor` admite 1–2048 puntos de código y `limit` usa default 25/rango 1–100; transacciones además acepta `state` (`enviado|recibido|anulado|vencido`).
- `EarlyRentalEventsEnvelope` contiene eventos ordenados por secuencia estrictamente creciente; cada elemento expone secuencia, tipo allowlisted, actor como rol (`arrendatario|arrendador|sistema`), timestamp UTC, revisión y sólo los datos de contrato/pago necesarios. `after` es opcional, no negativo e int64; sin límite/`next_cursor` en el contrato, cada lectura devuelve el sufijo completo `after+1..revision` y la ausencia de eventos sólo es válida si `after >= revision`. No duplica PII ni payloads arbitrarios.
- Implementado como helper no montado: `internal/earlyrental/transport/http/events.go` produce el envelope exacto, convierte IDs de cuenta a roles, filtra campos privados y rechaza secuencias/actores/tipos incompatibles, huecos respecto de `after` o un sufijo que no alcance la revisión leída. `events_query.go` acepta sólo el parámetro opcional `after` int64 no negativo (ausente equivale a cero). `Apply` conserva el UUID canónico compartido en oferta y eventos de envío/recepción; el repositorio transaccional aún debe persistir ese mismo movimiento junto al evento antes de proyectarlos. El helper no sustituye autorización por participante ni existe todavía un handler de historial.
- Implementado como helper no montado: `commands.go` decodifica sólo los DTOs definidos, rechaza JSON duplicado, campos desconocidos/case-variant y cuerpos mayores de 16 KiB, y mantiene actor, revisión, timestamps y `payment_id` fuera del body. Los valores contractuales y motivos se validan con el contrato OpenAPI/catálogo del dominio; aún no hay handler que traduzca esos errores a HTTP.
- Implementado como helper no montado: `create_request.go` admite exactamente `space_id`, `start_at` y `end_at`, normaliza UUID/instantes RFC3339 a tipos y UTC, y rechaza intervalos vacíos o invertidos. No valida publicación, horario, ocupación, tarifa ni disponibilidad; esas decisiones requieren la lectura autoritativa y revalidación transaccional del futuro servicio.
- Implementados como helpers no montados: `list_query.go` valida `role`, `cursor` y `limit`; `summary.go` proyecta el resumen allowlisted por rol con fechas de fuente y cursor opaco recibido de aplicación. `transaction_query.go` valida `role`, `state`, `cursor` y `limit`; `transactions.go` proyecta un movimiento canónico por oferta pagada, deriva egreso/ingreso desde el actor autenticado y comparte la clasificación de estado con el detalle; rechaza ID de movimiento duplicado y no fabrica recepción. Ambos DTOs exponen `CursorBinding(actorID)` para construir el binding con filtros normalizados; `list_cursor.go` cifra/autentica scope, actor, filtros, límite y posición `(created_at,id)` con AES-GCM y clave de 32 bytes inyectada. La clave estable de configuración local aún no está cableada y ningún handler/repositorio consume el binding ni aplica el mismo orden en consultas. Ningún helper de listado autoriza, consulta o persiste; el significado de `role` omitido sigue pendiente de la capa de aplicación.
- Implementado como helper no montado: `detail.go` valida el agregado reconstruido, confirma que el solicitante sea un participante y emite las 23 propiedades del detalle OpenAPI; convierte confirmaciones a roles, usa fechas de creación/actualización de la misma lectura, proyecta pago enviado/recibido/anulado/vencido sin inventar recepción y entrega `allowed_actions` sólo como pista. No autoriza ni sustituye un handler/repositorio.
- GET autenticados devuelven `Cache-Control: no-store`; detalle y mutaciones deberán devolver ETag fuerte canónico, codificado como Base64URL sin padding del UUID de oferta, byte NUL y revisión decimal, entre comillas. El helper no montado `etag.go` formatea y analiza exactamente una etiqueta, valida UUID/revisión canónicos, rechaza tags débiles, wildcard y listas, y distingue recurso ajeno; el handler aún debe comparar la revisión bajo CAS y traducir 428/400/412. Creación responde 201 con `Location`, detalle y ETag. Una mutación responde 200 con el detalle autoritativo y ETag nuevos; la UI sólo lo considera confirmado tras GET independiente.
- Estados HTTP para las operaciones de oferta: 400 para JSON malformado/campos desconocidos; 401 sin sesión; 404 para inexistente/no participante; 409 para transición, disponibilidad o vigencia incompatible; 412 para `If-Match` obsoleto; 422 para valores semánticamente inválidos; 428 si falta precondición; 503 si la bandera está apagada o la dependencia no está disponible. `capabilities` responde 200 con `enabled=false` cuando el flag está apagado y 503 si se solicita activación antes de que el flujo esté listo. El error body usa el envelope común, código estable allowlisted y sin mensajes SQL.

La creación debe rechazar con conflicto/no disponibilidad cualquier intervalo horario que se cruce, en la zona IANA del espacio, con una fecha que tenga `tarifa_personalizada_clp IS NOT NULL`; el final es exclusivo (`[inicio,termino)`), por lo que un intervalo que termina exactamente a medianoche local no incluye el día siguiente. Es una política conservadora de esta fase, no una interpretación de la tarifa especial ni un cambio al quote legacy. Se revalida dentro de la transacción de creación para evitar carreras con la configuración del calendario.

La ruta literal `capabilities` debe resolverse antes que `{id}` en mux/BFF y generación de políticas; no ampliar métodos, headers o queries de rutas legacy para acomodarla. El DTO temprano no expone nombre legal, RUT ni documentos: la contraparte se presenta con nombre visible minimizado y texto seguro, nunca HTML arbitrario del usuario.

Detalle mínimo propuesto: ID/tipo de flujo, rol del actor, snapshot del espacio/rango/precio, `state`, `revision`, `last_event_sequence`, `updated_at`, `expires_at`, identidad propia y contraparte, contrato/version/digest/texto/aceptaciones, pago/ID/estado/timestamps y `allowed_actions`. `allowed_actions` es presentación, no sustituye la autorización de cada handler.

Respuestas cerradas: 401 sin sesión; 404 recurso ajeno/inexistente; 409 transición, rango o plazo incompatible; 412 revisión obsoleta; 428 falta revisión; 422 contenido/confirmación inválida; 503 fase no habilitada. No incluir payloads de identidad, claves de almacenamiento, secretos o mensajes SQL.

Un replay autorizado con misma clave/huella reutiliza el resultado/evento sin repetir movimiento ni ocupación. Una clave reutilizada con otro payload da conflicto. Resolver idempotencia antes de rechazar una revisión antigua de una operación ya confirmada; después, leer la revisión vigente. Un ACK no se muestra como éxito definitivo hasta el GET independiente.

## 8. Histórico de transacciones en ambos paneles

### 8.1. Qué se registra y dónde se ve

Añadir al panel canónico de Mi cuenta una sección propuesta **«Transacciones de prueba»** (`/mi-cuenta/transacciones`, ruta nueva a aprobar) y acceso desde las bandejas de reservas enviadas/recibidas. No usar `/dashboard/activity` ni datos del starter como histórico real.

La tabla debe mostrar: ID de operación/ID único del movimiento, espacio, rol propio, contraparte mínima, fecha/hora/zona del arriendo, monto/CLP, estado de contrato, pago enviado/recibido, última actualización y enlace a detalle/línea de tiempo. Filtros por rol, estado y fecha; paginación estable y estados vacío/error/cargando. Una cuenta que arrienda y publica dispone de ambas perspectivas sin mezclar permisos.

- Desde creación: la oferta aparece en el histórico operativo de ambas partes, también si queda pendiente, rechazada, cancelada o vencida.
- Al enviar pago: se crea **un solo movimiento** compartido. A ve «Egreso declarado — pendiente de recepción»; H ve «Ingreso pendiente de confirmar».
- Al recibir: se actualiza ese mismo ID a recibido. A ve egreso confirmado de ensayo; H ingreso confirmado de ensayo. No insertar dos pagos ni duplicar importe por tener dos pantallas.
- La línea de tiempo conserva creación, dos identidades, dos aceptaciones, envío, recepción y eventos terminales según proceda, con actor/rol y hora del servidor.
- Los totales de movimientos recibidos/confirmados excluyen los enviados sin recepción, cancelados y vencidos; mostrar pendientes por separado. Nada se llama saldo bancario disponible o liquidación real.
- El histórico anterior permanece consultable y etiquetado como flujo anterior; no convertir sus pagos/firmas en confirmaciones tempranas ni duplicarlo en la nueva tabla financiera. Si se integra una vista unificada, discriminar fuente e ID en cada fila.

### 8.2. Coherencia técnica entre pantallas

Una única consulta autorizada por participante/rol deriva ambas vistas de las mismas tablas. Ni dos estados locales independientes ni dos inserciones de «histórico del usuario». Orden estable por creación + ID; eventos por secuencia por operación; cursor opaco validado por Go y acotado a la cuenta/filtros.

Después de cada mutación: confirmar detalle, invalidar/refrescar bandeja e histórico propios y los calendarios afectados. La otra sesión observa la nueva revisión mediante refresh/foco/polling, sin cambiar de cuenta. Capturar identidad/sesión y generación de recurso para descartar respuestas tardías (incluido A→B→A). Ante 401/403, purgar datos privados y detener polling. Logout no deja contratos/movimientos del usuario anterior visibles.

GETs coherentes de snapshot/revisión evitan construir una pantalla con aceptación vieja y pago nuevo. No ejecutar escrituras durante render ni convertir un GET en creador de contrato/movimiento.

## 9. Cambios por capa y orden de trabajo

Todos los paths nuevos aquí son **propuestos**; resolver nombres exactos en la fase de contrato antes de asignar escritores.

| Capa | Reutilizar | Delta propuesta |
|---|---|---|
| Dominio Go | Calendario civil, cálculo de unidades/precio, guards de cuenta/propiedad | `internal/earlyrental/`: estados, comandos, snapshot, confirmaciones y capacidades del flujo temprano |
| Persistencia Go | Pool/tx, locks ordenados, rangos/ocupación, autorización | `internal/adapters/postgres/earlyrental/`: agregado, idempotencia, eventos, pago, proyección bilateral y expiración |
| HTTP/composición | Auth, errores, configuración local y mux del monolito | Handler temprano y montaje con bandera; sin sustituir `identityverification` o `booking.Service.Decide` |
| OpenAPI/BFF | Generación de tipos, allowlist, sesión/cookies existentes | Operaciones nuevas exactas y políticas por especificidad de ruta; ninguna llamada SQL desde Next |
| Aplicación frontend | Patrón puerto→adaptador→fachada y fencing de sesión | Feature temprana tipada; comandos explícitos, GET de confirmación e invalidaciones coordinadas |
| UI | Shell de Mi cuenta, listados de reservas, calendario/ficha actuales | Modo temprano, detalle por pasos/casillas, bandeja de oferta para ambas partes e histórico bilateral |
| DB | Cuenta/espacio/tarifa y autoridad `ocupacion` | Migración nueva y aditiva de tablas, vínculo/tipos, CHECK/FK/índices/ACL mínimos; nunca reescribir V9/V11/V98 |

No fijar un número de migración por memoria ni reutilizar V85/V86 del plan histórico. El inventario fuente llega a V98. Se sincronizaron los inventarios y fixtures fresh/upgrade, y el runner full confirmó V1–V98, upgrades V75/V96→V97/V97→V98 y disponibilidad sobre PostgreSQL desechable. Esta prueba no actualiza la base compartida. Antes de crear V99, volver a revisar todos los worktrees/inventarios y cerrar contrato de `ocupacion`, ACL y writers; no elevar roles ni ejecutar bootstrap global para salvar un error.

La instancia Docker documentada como `espacigo_aws_local` responde, pero sus labels de Compose la sitúan bajo otro worktree (`/home/nandev/.codex/worktrees/img01-ledger-resumed/espaciGo`). No se ejecutará DDL ni se escribirán datos allí hasta reconciliar explícitamente que esa instancia es el destino autorizado para este checkout. Esta comprobación de procedencia es distinta de la aprobación funcional del plan.

Revalidación de procedencia en solo lectura (17 de noviembre de 2026): `espacigo_localbackend_20261024-database-1` está activo, sin puerto PostgreSQL publicado al host, y sus labels mantienen como `working_dir` y archivo Compose `/home/nandev/.codex/worktrees/img01-ledger-resumed/espaciGo`. No se conectó ni se escribió en esa instancia. El `cmd/dbmigrate -release` existente no resuelve E1: su validador fija la base de staging `espacigo_dms_stage_20261020`, y `internal/migrator.RunRelease` aplica el sufijo pendiente completo, no una sola migración local con autoridad/owners de `ocupacion` fijados. No reutilizarlo ni sustituir el target; el executor exacto sigue pendiente.

El alcance de navegación incluye explícitamente `mi-cuenta/account-navigation.tsx`, las dos páginas de bandeja, el parser `return-target.ts` y los nuevos route groups propuestos. No basta con crear un componente sin enlace visible para ambos roles. La inspección de frontend/backend confirma la estrategia aditiva y la ausencia, en las superficies revisadas, de un histórico bilateral con semántica enviado/recibido; esto es inventario de fuente, no una prueba runtime ni una migración ejecutada.

## 10. Fases de implementación y puertas de salida

**E0 aprobada por el usuario al ordenar “comienza con el plan” y reiterar “Revisado y conforme. Continua”.** E1 no está detenida por falta de permiso general: sus candidatos y pruebas ya existen; faltan implementación operativa, revisión y gates específicos del destino. El error histórico de ownership fue de un fixture, no de la base persistente no inspeccionada. OpenAPI y artefactos frontend están integrados en `main`; sólo `GET /local/early-rentals/capabilities` tiene handler, con `FlowReady=false`. Las otras 11 operaciones, persistencia de aplicación, worker y recorrido visible siguen pendientes. Las marcas siguientes distinguen componentes terminados de fases terminadas; publicar este checkpoint no cierra E1 ni E7.

| Fase | Entregable acotado | Criterio para pasar |
|---|---|---|
| E0 — Aprobación | **Completada:** orden, alcance simulado, UX, estrategia aditiva y retención del plan aprobados al iniciar | La aprobación no habilita KYC documental ni pagos reales |
| E1 — Contrato y delta de esquema | **Parcial:** contrato, resolver, primitiva target-pinned y candidatos de agregado/ocupación/confirmaciones/pago/eventos/idempotencia implementados; 82 casos obligatorios fresh/upgrade pasaron. SQL sólo en `testdata`, no migraciones aplicables. Pendientes revisión final, comando operativo, procedencia, contrato completo de autoridad, reconciliación del destino, backup y aplicación/readback autorizados. | Cerrar E1-R y E1-O1–O6 de §10.2; sin ownership transfer, versiones inventadas ni sustitución de destino |
| E2 — Persistencia y dominio | **Parcial, sin vertical persistente:** dominio puro `NewOffer`/`Apply`/`Expire`/`AllowedActions`, snapshot horario CLP, contrato UTF-8 con digest, revisión y TTL implementados y probados. Resolver PostgreSQL y helpers de transporte existentes; no hay repositorio de escritura, servicio ni handlers de las operaciones tempranas. Un valor conservado en el agregado Go no es persistencia PostgreSQL; decodificar idempotencia no es replay durable. Capabilities sigue con `FlowReady=false`. | Cerrar slices E2-P1–P6 de §10.3: transacciones, lecturas autorizadas, CAS, pago/ocupación, idempotencia y composición; pruebas PostgreSQL independientes. Operación persistente sólo después de cerrar E1; worker y carreras se verifican en E5 |
| E3 — Vertical identidad/contrato | Crear oferta, bandeja recibida, dos identidades y dos aceptaciones reales | Dos sesiones browser leen la misma versión; pago sigue bloqueado con una sola aceptación; reload conserva marcas |
| E4 — Vertical pago/histórico | Envío por A, recepción por H, un movimiento, dos perspectivas | Evento/movimiento/estado/ocupación en mismo commit; ambos paneles muestran mismo ID, importe y estado; sin pagos duplicados |
| E5 — Concurrencia y recuperaciones | Carreras, cancelación/vencimiento, respuesta perdida, cuenta equivocada y worker | Sin overbooking ni avances fuera de orden; relectura explícita sin replay automático; no alteración de fixtures ajenos |
| E6 — Aceptación manual/nativa | Checklist de tres cuentas, capturas desktop/móvil y consultas SQL posteriores | Todos los criterios de la sección 11 ejecutados sobre el mismo build/API/DB, sin mocks como prueba de durabilidad |
| E7 — Cierre y entrega | Revisión independiente, evidencia, docs, estado de CI y procedimiento de encendido/apagado | Publicación sólo con autorización correspondiente; no declarar completados módulos reales excluidos |

Separar propietarios de archivos por dominio/DB, contrato/adapter, UI e histórico/QA. Un solo responsable del build combinado; el padre revisa diffs y repite las pruebas. No reconstruir el `.next` del dev compartido durante aceptación.

### 10.1. Checklist contrastado con fuente y evidencia

`[x]` significa componente implementado y con la evidencia indicada; no implica que la fase completa ni la base persistente estén listas. `[ ]` permanece abierto. No se asignan porcentajes a criterios de distinto peso.

- [x] **E0:** alcance/orden/estrategia aditiva/TTL aprobados; identidad y pago siguen simulados.
- [x] **E1-C1 — Contrato:** 12 operaciones tempranas definidas en `planning/openapi.yaml`; artefactos frontend integrados. Sólo capabilities montado, no 12 handlers.
- [x] **E1-C2 — Inventario:** auditoría de escritores/lectores de `ocupacion` documentada en §6.2; inventario fuente V1–V98. No es lectura del ledger persistente.
- [x] **E1-C3 — Snapshot/precio:** `ResolveEarlyRentalOfferSnapshot`, transacción del caller y rechazo de overrides; pruebas de fecha local y medianoche half-open.
- [x] **E1-C4 — Primitiva:** `RunSingleOwnerMigration`, identidad/owner/checksum/ledger fijados y rollback probado; sin CLI operativo.
- [x] **E1-C5 — Candidato base:** agregado, FK bidireccionales, snapshot inmutable, coherencia diferida estado/ocupación, exclusión legacy preservada; `testdata/early_rental_schema_candidate.sql`.
- [x] **E1-C6 — Evidencia:** confirmaciones, movimiento único, eventos con referencias/roles/secuencia y fases bilaterales, idempotencia ligada a actor/comando/evento; `testdata/early_rental_evidence_candidate.sql`.
- [x] **E1-C7 — Harness:** 82 casos obligatorios fresh/upgrade pasaron sin omisiones, incluido recorrido SQL bilateral hasta recepción/reserva; negativos con SQLSTATE/rollback/readback y cleanup verificado. Race/vet y checker completo/ausente/skip pasaron. No prueba UI, servicio ni concurrencia de eventos que no se haya ejercido explícitamente.
- [x] **E1-R — Revisión de publicación del checkpoint:** reevaluación `deleg_3f46fe45` favorable, sin defectos concretos de fuente; no sustituye la futura revisión del comando/migración operativos.
- [x] **E1-O1a — Biblioteca de procedencia offline:** commit, manifest, inventario y SQL capturado verificados; revisión independiente `deleg_80595e0a` favorable. Falta conexión al comando operativo.
- [x] **E1-O2a — Modelo/comparador de autoridad:** snapshots explícitos, duplicados y deriva comprobados; misma revisión favorable. No es lector ni prueba de cobertura de catálogos PostgreSQL.
- [x] **E1-O1b — Staging offline:** materializa SQL capturado/verificado en un directorio privado nuevo; readback de disco, cancelación y cleanup acotado probados. Revisión correctiva `deleg_3d8628d6` favorable; aún sin CLI.
- [x] **E1-O2b1 — Lector parcial de identidades:** roles, membresías en ambas direcciones y owners canónicos en transacción read-only estable; seis casos PostgreSQL obligatorios pasaron sin skips. No incluye ACL/defaults/probes.
- [ ] **E1-O1–O6:** integración y cierre operativo descritos abajo. **E1 completa permanece abierta.**
- [x] **E2-D1 — Dominio puro:** `NewOffer`, `Apply`, `Expire`, revisión, contrato/digest, roles y TTL en `internal/earlyrental`; no escribe PostgreSQL.
- [x] **E2-D2 — Helpers HTTP:** decoders, proyecciones, ETag/idempotency header y cursor AES-GCM en `internal/earlyrental/transport/http`; no equivalen a endpoints ni replay durable.
- [x] **E2-D3 — Capabilities fail-closed:** montaje local autenticado en `cmd/api/early_rental_capabilities.go`, `FlowReady=false`.
- [ ] **E2:** repositorio/servicio transaccional, lecturas autorizadas, idempotencia durable, configuración estable de cursor y composición completa.
- [ ] **E3:** vertical browser de creación, bandejas, identidad y contrato bilateral con relectura.
- [ ] **E4:** vertical browser de envío/recepción e histórico compartido con un solo movimiento.
- [ ] **E5:** worker y matriz real de carreras, pérdida de respuesta, cancelación/vencimiento y aislamiento entre cuentas.
- [ ] **E6:** checklist completo §11 con H/A/B, reinicio, SQL independiente, desktop/móvil y accesibilidad.
- [ ] **E7:** revisión final del MVP, evidencia/CI y encendido/apagado seguro. Un push de avance no satisface esta fase.

### 10.2. E1 restante dividida en tareas pequeñas

Las asignaciones son responsabilidades de trabajo, no afirmaciones de que un subagente siga ejecutándose. El padre integra y verifica; trabajadores técnicos usan la configuración comprobada `gpt-6-luna` + razonamiento `medium`, no un identificador ficticio `gpt-6-luna-medio`.

| ID / estado | Responsable y propiedad exclusiva propuesta | Dependencia | Salida comprobable |
|---|---|---|---|
| **E1-R — completada para publicar el checkpoint** | Revisor independiente; sólo lectura de SQL/tests/checker congelados | C5–C7 | `deleg_3f46fe45`: `passed=true`, sin bloqueadores; retiró explícitamente sus tres hallazgos al contrastar las fuentes exactas. Aprobación de publicación WIP, no cierre operativo de E1 |
| **E1-O1 — O1a/O1b completadas y revisadas; CLI pendiente** | Backend/migrator: manifest de procedencia y sus tests, sin editar SQL candidato | C4 + revisión de contrato | `VerifyMigrationSource` exige commit e inventario completos, coteja blobs/bytes y las tres membresías del inventario (Git, checkout, manifest), admite mapping de candidatos y retorna SQL capturado. Pendiente CLI que encadene manifest revisado y staging antes de conexión en O4; no hay CLI ni prueba de aprobación humana |
| **E1-O2 — O2a/O2b1 completadas; O2b2 pendiente** | Seguridad DB: contrato/verificador de owners, ACL directas/efectivas, membresías/atributos y tests | C5/C6, coordinado con O1 | `CompareSingleOwnerAuthorityContract` compara snapshots explícitos y rechaza duplicados/deriva de flags/owners/ACL/defaults/probes sin mutar inputs. O2b2 todavía debe capturar catálogos completos y fijar el perfil operativo con negativos PostgreSQL; sin grants automáticos ni cambios de owners |
| **E1-O3 — pendiente, desarrollable ahora** | Herramientas locales: preflight READ ONLY y recibo de backup, archivos propios | O1/O2 | Identidad/OID/clúster/motor/ledger/owners/ACL fijados; validar dump privado, parse completo, permisos y checksum sin exponer secretos. Probar contra descartable; no tocar destino restringido |
| **E1-O4 — pendiente, desarrollable ahora** | Backend CLI: entrada operativa y pruebas de proceso, sin modificar primitiva salvo necesidad revisada | O1–O3 | Conectar preflight/manifest/backup a `RunSingleOwnerMigration`; defaults fail-closed, una versión exacta, commit ambiguo sin replay, postcheck y contrato probado en descartable |
| **E1-O5 — pendiente de autorización específica y reconciliación** | Padre/operador; ningún subagente conecta por inferencia | O1–O4 y alcance específico resuelto | Reconciliar `espacigo_aws_local` / contenedor de otro worktree; inspección READ ONLY autorizada, ledger/owners/engine reales y versión globalmente libre. Sin respuesta no hay permiso |
| **E1-O6 — pendiente, operación final** | Padre con revisor final; migración aplicable/release-policy sólo tras reconciliar | R + O1–O5 + backup válido | Asignar versión/checksum revisados, ensayo exacto fresh/upgrade y autoridad, aplicación expresamente autorizada y readback independiente de ledger/catálogo/ACL/owners; evidencia retenida |

**Orden siguiente:** O1a/O2a completadas y revisadas; publicar este corte y continuar con O2b (captura de catálogo y perfil de autoridad en descartable), integración de procedencia/staging, O3, O4 y finalmente O5/O6 cuando su alcance esté resuelto. La restricción de O5 no bloquea desarrollar O1–O4. No iniciar una vertical E3 ni activar el flag para demostrar progreso mientras E1 siga abierta.

### 10.3. Desglose de las fases posteriores (asignación planificada, no iniciada)

| Fase / tareas pequeñas pendientes | Responsable | Dependencias y aceptación de salida |
|---|---|---|
| **E2-P1:** crear oferta con snapshot/hold/evento en una tx; **P2:** reconstruir detalle/lista/eventos autorizados; **P3:** confirmar identidades/contrato con CAS; **P4:** movimiento envío/recepción y ocupación atómicos; **P5:** idempotencia/replay durable; **P6:** cursor estable, handlers/composición y mapeo HTTP | Backend persistencia → aplicación → HTTP, archivos separados | E1 cerrada para operación persistente; cada slice con commit real, readback independiente y rollback de errores. Helpers actuales se reutilizan, no se cuentan como servicio terminado |
| **E3-U1:** adaptador/fachada tipados; **U2:** entrada pública/return seguro y crear oferta; **U3:** ambas bandejas/detalle; **U4:** casillas identidad/contrato y GET tras comando | Frontend feature; navegación en slice separado | E2 de las operaciones utilizadas. Dos sesiones, mismo contrato; una aceptación no habilita pago; recarga mantiene hechos; `break-words`, sin efectos legacy |
| **E4-H1:** comandos de pago visibles por rol; **H2:** consulta/histórico bilateral; **H3:** navegación/filtros/cursor e invalidaciones | Backend de lecturas y frontend histórico | E2-P4/P5 + E3. Mismo movimiento/importe/estado en ambas cuentas tras reload; no contabilizar enviado como recibido ni duplicar pagos |
| **E5-Q1:** worker acotado de vencimiento; **Q2:** cancel/reject y liberación; **Q3:** carreras oferta/recepción/vencimiento; **Q4:** respuesta perdida y replay; **Q5:** fencing A→B→A y compatibilidad legacy/retención | Backend worker/concurrencia y QA frontend, sin writers compartidos | E2–E4. Locks observados y estado final por SQL; sin overbooking, resurrección terminal ni datos cruzados; no usar mocks como evidencia de durabilidad |
| **E6-A1:** preparar H/A/B sintéticos sin KYC real; **A2:** ejecutar §11.1 y negativos; **A3:** reinicio y postcheck SQL; **A4:** capturas 1440/390 px, calendario y Axe | QA nativo; padre controla servicios/fixtures | E3–E5 y build/API/DB exactos. Checklist completo con resultados y límites, sólo recursos propios; persistencia no se infiere del build |
| **E7-C1:** revisión independiente final; **C2:** reporte reproducible y CI exacto; **C3:** runbook activar/desactivar con histórico legible; **C4:** entrega/publicación final autorizada | Padre + revisor + documentación | E1–E6 cerradas y evidencia real de aceptación. Separar publicación, migración aplicada y MVP aceptado; no prometer identidad/pagos reales |

**Asignaciones de contraste completadas:** revisión de publicación E1-R y auditoría E1–E7, ambas de sólo lectura. El padre conserva plan, revisión combinada y commit/push. En el corte siguiente se asignaron `deleg_30b2fb34` **O1a** (manifest/commit/bytes, sólo `source_manifest.go` y su test) y **O2a** (modelo/comparación pura de autoridad, sólo `single_owner_authority_contract.go` y su test), con propiedad exclusiva y TDD; no equivalen todavía a O1/O2 completos. O3–O6 y las fases posteriores no están iniciados.

#### Validación combinada O1a/O2a — 17 de noviembre de 2026

El padre corrigió los faltantes de la primera entrega O1a y verificó RED/GREEN reales para la omisión de una migración comprometida, el alias symlink de raíz, un SQL aplicable extra cuando sólo hay candidatos comprometidos y el modo symlink de un blob Git. Se añadieron negativos de pins/bytes/paths/entorno/cancelación y captura estable de SQL. O2a tiene regresiones de duplicados en ambos lados, atributos/opciones individuales, ACL/defaults/probes, strings inválidos y no-mutación; sigue siendo un comparador puro, no una captura de PostgreSQL.

Desde `git archive HEAD` más los seis paths exactos del corte, sin configuración privada, pasaron los nuevos unitarios y subtests sin skips, race de migrator/dbmigrate/earlyrental/rentalflow/API, vet, build de API y `actionlint` del workflow corregido. El harness `full` volvió a exigir y pasar los **82 casos PostgreSQL obligatorios fresh/upgrade**, con eliminación verificada del contenedor/scratch propios. La revisión independiente `deleg_80595e0a` terminó con `passed=true`, sin defectos concretos de fuente, y aprobó publicar estas bibliotecas como WIP; no se marcan O1/O2 completos ni aplicación persistente. Las verificaciones de fuente y autoridad no están conectadas a un CLI de aplicación.

#### Corte O1b/O2b1 asignado — 17 de noviembre de 2026

Con HEAD `9b8691b` limpio se asignaron dos tareas pequeñas en `deleg_62f7f789`: **O1b**, staging offline desde los bytes verificados hacia un directorio nuevo sin overwrite; **O2b1**, lector de roles, membresías entrantes/salientes y propietarios de objetos explícitos, dentro de una transacción read-only estable del caller. Los trabajadores sólo poseen sus archivos nuevos de biblioteca/unitarios y prueban en copias aisladas.

El padre añadió y ejecutó un test PostgreSQL separado con destino estrictamente desechable: lectura canónica con funciones sobrecargadas, rechazo de transacción read-write, rechazo de read-committed y ausencia de resultados parciales ante rol/objeto faltante o una base distinta a `current_database()`. El checker exige estos **seis casos nativos**, además de los 82 del esquema; las verificaciones con fixtures completas/ausentes/skip del checker se conservan separadas de la ejecución real.

La primera ejecución real pasó cinco casos. Al ampliar la frontera de destino se reprodujo un RED nativo que aceptaba la identidad de otra base; se añadió el filtro de base actual y el harness completo pasó los **82 + 6 casos obligatorios, ninguno omitido**, desde una copia de fuente aislada. También se verificó RED/GREEN unitario de cancelación antes de consultar y después de la última lectura, y se añadieron errores de iteración/consulta sin resultados parciales. El staging O1b quedó implementado, probado y revisado favorablemente para este checkpoint: usa una raíz abierta para anclar escrituras, conserva identidades de archivos creados, elimina sólo esos archivos y el directorio vacío, nunca hace cleanup recursivo de entradas desconocidas. Las pruebas releen SQL/checksum desde disco y preservan sentinels tanto en un destino reemplazante como dentro del staging original.

La verificación combinada final se repitió desde fuente aislada con los siete paths del corte, sin archivos privados: unitarios/race, vet, build de API y harness nativo pasaron. El checker rechazó cada caso de identidad ausente o skipped en trece escenarios sintéticos separados de la evidencia de DB. La revisión inicial `deleg_8a8e57db` señaló un supuesto acceso a root nil; se contrastó el callsite con lista de archivos nil y se añadieron tres caracterizaciones (directorio propio vacío, archivo desconocido y destino reemplazado). La reevaluación acotada `deleg_3d8628d6` retiró el hallazgo y devolvió `passed=true`, sin bloqueadores; producción/harness permanecieron idénticos. Esto aprueba publicar O1b/O2b1, no cerrar E1 operativa.

O2b1 devuelve un tipo parcial explícito, nunca un snapshot de autoridad completo con ACL/probes falsamente vacíos. **O2b2** deberá añadir ACL/defaults/privilegios efectivos, comprobar cobertura de catálogos y fijar el perfil operativo revisado. O1b tampoco equivale al CLI O4. No se ha conectado a la base restringida ni se asignó versión real de migración.

La CI del checkpoint `9b8691b` confirmó éxito de Database migration quality, Application quality e Infrastructure as Code Quality; frontend sigue fallando por formato en la capa no modificada. La corrección del workflow de base ya tiene evidencia remota de éxito, separada de los nuevos cambios aún locales.

#### Seguimiento de CI del checkpoint `2e5f4f0`

Application quality e Infrastructure as Code Quality terminaron con éxito. Frontend architecture and quality falló en `format:check` sobre nueve archivos que no se modificaron en este checkpoint; la comparación con el padre no contiene cambios frontend. No se maquilla ese resultado ni se reformatea otra capa dentro de E1.

El workflow de base fallaba antes de crear jobs. Se reprodujo localmente con `actionlint` v1.7.12: `runner.temp` no es un contexto permitido en `jobs.<job>.env`. Se movió exclusivamente al `env` del step del harness que necesita `TMPDIR`, conservando triggers, permisos y pruebas obligatorias. La misma validación pasó después y las siete pruebas Python de evidencia pasaron; esto es corrección/validación local, no una ejecución remota verde del workflow corregido. No cambia ni autoriza el destino persistente.

## 11. Plan de pruebas y aceptación manual

### 11.1. Checklist obligatorio del usuario — sin SQL de preparación KYC

Usar un anfitrión H y dos arrendatarios A/B de prueba, con sesiones separadas. Para esta fase la identidad se confirma **por la nueva casilla**, no mediante los scripts de aprobación usados en el ensayo anterior.

1. H publica un espacio horario; Go guarda horario y precio. A selecciona un intervalo real del calendario y crea la oferta.
2. H ve la oferta en su bandeja; ambos ven el mismo rango/monto y fase pendiente.
3. A confirma su identidad: sólo A queda marcado. Recargar ambos paneles conserva A y no inventa H.
4. H confirma su identidad; se habilita contrato para ambos, todavía no pago.
5. A acepta versión/digest del contrato; pago bloqueado en ambas sesiones.
6. H acepta la misma versión; ambos ven «Contrato aceptado por ambas partes» y sólo A puede enviar pago.
7. A marca enviado; una fila de movimiento aparece en ambos históricos, recibida aún no. Reload mantiene esa distinción.
8. H marca recibido; ambos ven confirmado, el mismo ID de transacción y monto; ocupación definitiva bloquea exactamente ese intervalo.
9. Repetir A/B en horas distintas del mismo día y luego en otra fecha, conservando las cuatro operaciones y sin reusar el ID de otro arriendo.
10. Reiniciar el API y recargar ambos paneles: reconstruir detalles, contratos, confirmaciones, movimientos y calendarios desde PostgreSQL.
11. Verificar por SQL independiente los IDs exactos: participantes, snapshot, dos identidades, dos aceptaciones de la misma versión, envío previo a recepción, un movimiento, eventos secuenciados y ocupación exacta; mismo número de filas que el guion.
12. Comprobar que el calendario público y anfitrión descuenta lo ocupado y mantiene seleccionables los huecos; capturas 1440/390 px y auditoría Axe completa sin desborde.

### 11.2. Negativos y concurrencia obligatorios

- Sin sesión, cuenta bloqueada, ID ajeno o actor pretendiendo otro rol: denegar sin mutación ni filtración.
- A no marca identidad/contrato por H; H no puede marcar pago enviado por A; A no puede confirmar recepción.
- Enviar con identidad incompleta, una aceptación, digest/versión distintos o importe añadido al payload: rechazo sin movimiento.
- Recibir sin envío o después de vencer/cancelar: rechazo sin reactivar ocupación.
- Doble clic, reintento de red y replay tras reiniciar: misma confirmación/evento/movimiento, sin duplicados.
- Misma clave con payload distinto: conflicto; versión obsoleta: 412 y refresh, sin sobrescribir.
- Dos aceptaciones simultáneas: conservar la primera y completar la segunda tras reconciliar revisión, sin borrar la aceptación opuesta ni habilitar pago prematuramente.
- Dos solicitudes por el mismo ticket y dos confirmaciones de recepción concurrentes: un ocupante/una transición; exclusión real de PostgreSQL y locks observados, no sólo mocks.
- Cancelar/vencer frente a recibir: una sola transición terminal coherente; rango/estado/histórico coinciden.
- Respuesta perdida tras commit: relectura detecta el evento sin volver a cobrar/marcar ni crear otra oferta.
- Cambiar cuenta A→B→A con respuestas tardías: ningún dato ni checkbox privado de otra sesión permanece.
- Preservar el ensayo viejo, sus cuatro reservas y sus workers; no filtrar temprano hacia KYC documental, firma/pago/garantía legacy o endpoints comerciales.

### 11.3. Evidencia por capa

Unitarios y fakes acreditan reglas de dominio/serialización; tests de repositorio con PostgreSQL acreditan constraints/locks/rollback; browser con gateway mock acredita UI; **sólo browser con BFF/Go/DB reales + postcheck independiente acredita el recorrido pedido**. Separar pass, skip y límites. Build, types, lint, OpenAPI, race/vet y migraciones se ejecutan sólo al implementar; este encargo documental no los presenta como nuevos resultados.

## 12. Definición de terminado y reversibilidad

El MVP está listo únicamente si una persona completa los pasos por casillas, ambos participantes observan el mismo estado/histórico tras recarga/reinicio, PostgreSQL conserva los hechos y el calendario impide solapamientos sin ocultar huecos. Una marca React, un toast o una fila mock no lo satisfacen.

Para desactivar: impedir nuevas mutaciones tempranas con su bandera, dejar lectura de histórico y resolver/expirar retenciones según política; no borrar cuentas, contratos ni transacciones para volver a encender. Mantener la compatibilidad de los lectores antiguos. Cualquier cleanup de fixtures es posterior, acotado y autorizado, no un reset global de la DB.

La futura integración de identidad real, firma y pago sustituirá puertos/orquestación en un corte nuevo y con una nueva versión de flujo. No convertir retrospectivamente una casilla en evidencia documental o bancaria, ni mezclar en estadísticas dinero real con movimientos de ensayo.

## 13. Decisiones aprobadas y límites vigentes

- Nuevo recorrido local y persistente, aditivo; no desactivar controles del flujo anterior.
- Identidad propia por cada operación temprana; no aprobación global KYC.
- Contrato/oferta inmutable y aceptación de ambos antes de habilitar pago.
- Enviado sólo por arrendatario; recibido sólo por arrendador; no garantías obligatorias en este modo.
- Un movimiento compartido, dos perspectivas en panel y línea de tiempo auditada.
- Retención inicial de treinta minutos, acotada por inicio; cancelación/rechazo antes de confirmar y vencimiento sin recepción.
- Fin de esta fase en arriendo confirmado y ocupación persistida, no ejecución física del arriendo ni liquidación bancaria.
- Aceptación inicial horaria; habilitar otras unidades sólo cuando sus pruebas específicas pasen.

**Ejecución iniciada con aprobación del usuario.** `internal/earlyrental` aplica las transiciones y control de revisión en memoria de dominio; no demuestra persistencia ni sincronización. No declarar el recorrido completo hasta integrar PostgreSQL, HTTP/BFF y ambas interfaces, reconciliar el destino de DDL y pasar las puertas E1–E7.
