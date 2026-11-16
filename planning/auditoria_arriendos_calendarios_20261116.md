# Auditoría trazable de estado real: arriendos y calendarios

**Fecha de corte:** 2026-11-16
**Lectura vigente:** el addendum «Verificación del padre: selección y handoff» al final de este documento sustituye los bloqueos históricos que allí se identifican como resueltos. Las tablas iniciales se conservan como baseline, no como estado actual.
**Repositorio:** `/home/nandev/Documentos/projects/espacigo-aws`
**Rama/HEAD observados antes de editar:** `main` / `3dc1c18fd7e8ff7cd3d310b16615c1c8c97ec2c3`.
**Alcance:** los dos planes enlazados y las fuentes de implementación indicadas; no se modificó producto ni infraestructura. Los hechos de DB/servicios y respuestas HTTP siguientes son auditoría previa del padre comunicada para esta revisión, no se reejecutaron aquí.

## Evidencia de estado observado

- El padre verificó en lectura solamente **98/98 migraciones** en la DB Docker local: nombres y checksums coinciden, sin pendientes. Esto no certifica comportamiento funcional de cada flujo. No se ejecutaron migraciones ni SQL de escritura.
- El repositorio estaba limpio en `3dc1c18`; el historial de trabajo reportado por el padre indica avance desde `761fd7e`. En esta sesión `git status` mostró además `M scripts/local-dev.sh` y `?? planning/plan_calendario_reactivo_motor_inventario.md`; ambos son propiedad ajena y se preservaron sin cambios.
- Front `:3000` y API Go `:8080` fueron reportados persistentes y saludables por el padre. PIDs no se guardan: son volátiles.
- Pruebas HTTP comunicadas por padre: Go responde 200 al endpoint heredado mensual `/spaces/{id}/availability?month=...`; BFF responde 400 `query_parameter_not_allowed`; la UI muestra «Sin información». También se reportó que la UI fabrica intervalos con offset `-03:00` cuando la API devuelve vacío y que hay respuestas sin fence.
- El endpoint mensual legado de Go sólo filtra espacios eliminados y silencia errores; no comparte completamente reglas/precios con el cotizador. Nuevos escritores están atendiendo fail-closed, GET público canónico y seguridad de selección: **estado pendiente de verificación del padre**, no se atribuye corrección en este documento.
- Fuente de código confirmada en este checkout: `internal/rentalflow/availability.go` implementa disponibilidad comercial por fecha/duración con validaciones del listing y normalización UTC; `cmd/api/commercial_rental_flow.go` monta `/api/v1/spaces/{spaceID}/rental-availability` y cotizaciones, y exige `COMMERCIAL_RENTAL_FLOW_ENABLED=1`, runtime local explícito; rechaza coexistencia con `LOCAL_BOOKING_TRIAL=1`. `frontend/src/lib/api/generated/gateway-operations.ts` contiene además `/spaces/{spaceId}/availability` de perfil anfitrión, que no es el contrato de grilla pública mensual.
- Los dos planes contienen diseño y claims históricos que no bastan para acreditar capas actuales; matriz y gates por bloque están en sus addenda. Las `V000085`/`V000086` de plan original son propuesta histórica; no se cambia SQL/ledger aplicado.

## Matriz cruzada de ambos planes

| Dominio | Plan de arriendo | Plan de calendario | Estado de evidencia al corte | Gate 100% común (Go + PostgreSQL + Next) |
|---|---|---|---|---|
| Contrato/calendario público | Ficha y mes público se describen como integrados; no equivalente al endpoint mensual real | GET mensual `?month=` se propone como contrato | **Con incongruencias/bugs**: legacy Go 200 vs BFF 400; UI con estado vacío incorrecto | OpenAPI→Go→BFF exacto; schema, status/error envelope, mes/días y privacidad; requests reales cubiertos en navegador; fail-closed verificado tras escritor |
| Motor/fuente inventario | Depende de horario, bloqueos y `ocupacion` | Reglas base + override propuestos | **Parcial**: la disponibilidad comercial actual es intervalar por fecha/duración; mensual legacy no equivale a reglas completas/quote | Mismo schedule, zona, tarifa/versión, override y ocupación deben alimentar GET y quote; integración en DB local |
| Edición de anfitrión | Se enumera como etapa, pero claim histórico no prueba owner CRUD actual | Regla rápida, multiselección y PUT están especificados como construcción | **Parcial/falta construir** según capacidad concreta; no inferir regla nueva solo del plan | Propiedad/autorización, ETag/CAS, persistencia en esquema vigente, readback, error y reflejo público/browser |
| Quote y precio | Quote comercial con snapshot descrito; trial quote también existe | Precio por día y precio de override propuestos | **Parcial**: código comercial tiene validaciones/snapshot, pero la disponibilidad mensual no queda alineada por eso | Quote revalida disponibilidad y precio; snapshot DB inmutable idéntico al desglose UI; drift/conflicto probado |
| Reserva/ocupación | Trial local tiene etapas posteriores; «completo» histórico no acredita comercial | El plan imagina bloqueo tras generar reserva | **Parcial (trial), comercial no demostrado por simulación** | Confirmar namespace/flag, persistencia real, atomicidad, exclusión/idempotencia y readback; pago/firma separados |
| UI y navegador | Diseños/CTA y checks históricos no constituyen navegador actual | Ensayo de dos cuentas está propuesto | **Falta construir** el cierre cruzado y probarlo después de cambios pendientes | Build final y Playwright de host→público→selección→quote→reserva con DB real local; diferenciar mocks y trial |

### Separación no negociable de modos

`LOCAL_BOOKING_TRIAL` (namespace `/local/booking-trial/*`, adaptadores de prueba/fake) es un ensayo local persistente según su backend, no arriendo ni pago comercial. `COMMERCIAL_RENTAL_FLOW_ENABLED=1` habilita rutas comerciales separadas bajo comprobaciones explícitas de loopback/runtime y es incompatible con el trial. El hecho de que ambos compartan una base PostgreSQL local no convierte el trial, el fake payment, el contrato sintético, Mailpit ni fixtures/mock en operación comercial real. El standalone `internal/rentaldemo` en memoria es otra modalidad y tampoco prueba persistencia PostgreSQL.

## Verificación ejecutada en esta sesión

Fecha de corrida: **2026-11-16**, desde `frontend/`. Cada salida completa queda en `/home/nandev/.hermes/cache/scratch/qa-gpt6/`. No se ejecutó `pnpm build`, Playwright ni toda la suite Go; no se activaron nativos ni liveS3.

| Comando | Exit real | Resultado observado | Log |
|---|---:|---|---|
| `pnpm typecheck` | 0 | PASS | `frontend-pnpm_typecheck.log` |
| `pnpm test` | 0 | PASS | `frontend-pnpm_test.log` |
| `pnpm api:check` | 0 | PASS | `frontend-pnpm_api_check.log` |
| `pnpm lint` | 1 | FAIL: 4 errores: 1 `react-hooks/set-state-in-effect` en `access-screen.tsx`, 2 en `booking-calendar.tsx` y 1 `no-explicit-any` en `public-space-marketplace-layout.tsx, 28 warnings. Hay escritor concurrente sobre componentes; esto es baseline fechado y no consolidado final. | `frontend-pnpm_lint.log` |

La afirmación previa de QA Go (`exit 0, 4671 pass / 217 skip`) se comprobó parseando `qa-gpt6/go-test.jsonl`: acciones test `pass=4671`, `skip=217`, sin acciones `fail`; esto confirma el log existente, no es una ejecución Go nueva ni valida servicios live/DB/browser. Archivo fuente: `/home/nandev/.hermes/cache/scratch/qa-gpt6/go-test.jsonl`.

## Secuencia de tracer bullets y bloqueo de cierre

1. **Contrato público:** congelar forma mensual canónica; corregir OpenAPI y query allowlist BFF; verificar vacío vs error; fail-closed sin fugas ni fallback inventado.
2. **Edición host:** autenticar propietario, versión/CAS y zona; escribir y leer schedule/overrides en esquema actual, sin ejecutar migración por inferencia.
3. **Disponibilidad→quote:** una única regla de horario, excepción/ocupación y tarifa efectiva; snapshot inmutable con comparación en DB.
4. **Reserva:** revalidar quote, persistir estado e intervalo atómicamente/idempotente; prueba de solapamiento y lectura pública; separar trial del modo comercial.
5. **Navegador:** journey host y arrendatario sobre servicios locales reales, con reload/readback y errores; no usar mocks como evidencia de DB.

| Dominio | Estado confirmado del corte | Estado por plan | Criterio verificable para 100% |
|---|---|---|---|
| Calendario mensual: transporte/render del mes y selección de día nativo | **CERRADO — sólo este corte**: OpenAPI/BFF mensual canónico y lectura/render sin slots ficticios; flujo público real 1440/390 | Arriendo: **parcial** global. Calendario: **parcial** global; edición de anfitrión, rango/quote, escritura/ocupación no cubiertos | Mantener DTO y error/vacío fieles; GET de intervalos y quote autoritativos, luego selección rango y persistencia/readback real |
| Selección pública con mes vacío / error | **Parcial**: pruebas preparadas; interceptan ruta mensual real con respuesta vacía tipada o 500, no alteran GET de catálogo ni crean fixture; 2 fallan por CTA habilitada | Ambos planes: gate de selección/quote **pendiente** | Vacío/error conserva cero tickets y la CTA debe estar deshabilitada hasta una selección real válida |
| Edición de calendario anfitrión | **Parcial / pendiente** | Arriendo: **parcial**; calendario: **falta construir** | Dueño, CAS/ETag, horario/zona y persistencia/readback PostgreSQL coherentes con GET/quote |
| Quote, reserva, ocupación, pagos y estados posteriores | **Fuera de este corte; no acreditados como comercial** | Arriendo: **parcial (trial distinto)**; calendario: **parcial** | Quote y snapshot autoritativos; ocupación transaccional/readback; pago comercial sólo con gate/proveedor explícito |

### QA nativo del corte mensual

Corrida final de esta sesión contra el artefacto preconstruido `127.0.0.1:4316` → API `127.0.0.1:8080` → catálogo publicado de DB Docker; no se reconstruyó ni escribió al servicio. Salida aislada (no pisa `native-green`): `frontend/test-results/calendar-month-native-qa-20261116/`. Comando: `CALENDAR_NATIVE_READONLY=1 node_modules/.bin/playwright test -c playwright.calendar-month-native.config.ts --output=test-results/calendar-month-native-qa-20261116`; exit **1**, 4 pruebas: **2 pass** (lectura/render BFF y día nativo a 1440 y 390 px), **2 fail** (empty tipado y failure mocked: la CTA «Selecciona fechas en el calendario» permanece enabled aunque no hay selección, ambas reproducciones). Es una falla real observada en UI del artefacto; el alcance de propiedad prohíbe cambiar producción aquí. Las pruebas usan el GET real del catálogo para elegir un `space_id` publicado y el mes local; interceptan una vez la ruta del BFF mensual con DTO tipado (respuesta `days: []` conforme al esquema, o 500 etiquetado como falla simulada). No alteran el catálogo, DB, ni crean cuentas/reservas. Axe default y overflow mobile se ejecutaron tras el assertion soft; no reportaron violations ni overflow. CTA pendiente bloquea aceptación de selección.

### Límite de cierre y claims

**Cierre mensual de transporte/render no equivale al cierre del plan.** No se probó selección positiva real de intervalos (`GET .../rental-availability`), quote, ocupación ni pago. El gate de intervalos/quote, ahora asignado a escritores paralelos, permanece **pendiente de resultado/revisión del padre**, no resuelto ni listado como hallazgo arreglado. No afirmar disponibilidad/ocupación global, reserva confirmada o pago comercial a partir de esta corrida; catálogo, calendario y hasta cualquier «slot» no son prueba de ocupación. El antecedente Gemini y sus estados históricos permanecen intactos; esta adenda clasifica evidencia por capa.

**Pendiente exacto:** corregir/decidir CTA que permite continuar sin selección y repetir estos dos controles sobre el mismo artefacto; integrar escritor de intervalos/quote y sólo entonces preparar selección positiva sin escrituras según contrato. No esperar ni atribuir pass al trabajo paralelo.

## Verificación del padre: selección y handoff — 2026-11-16

Esta actualización sustituye los bloqueos anteriores de transporte mensual, montaje de intervalos y CTA sin selección. No cierra los planes completos ni certifica envío/persistencia de una solicitud.

### Cambios comprobados

- El runtime local con `LOCAL_BOOKING_TRIAL=1` expone lectura pública de intervalos sin activar `COMMERCIAL_RENTAL_FLOW_ENABLED` ni cotización comercial. La API fue recargada después de ejecutar tests. Lecturas reales de los tres espacios publicados devolvieron 200 e intervalos, sin mutaciones.
- `PublicIntervals` e `IntervalAvailable` aplican restricciones V98 de días/horas, estadía diaria y overrides bloqueados/mantenimiento, además de las comprobaciones existentes. Se observaron cuatro regresiones RED antes de integrar el filtro; la suite focal con `-race` pasó después. Esto no prueba que el quote trial comparta ya esa misma autoridad transaccional.
- La CTA queda deshabilitada sin selección. Se retiró la garantía fija de $50.000, carente de fuente contractual. El subtotal sigue siendo una estimación de tarifa base, no una cotización de tarifas variables.
- El handoff pasa inicio, fin y zona exactos a `BookingQuoteCard`; los controles de modalidad/duración/inicio se bloquean y el rango se muestra en la zona del espacio. El handler usa el selector de payload antes de llamar a `requestQuote`. Unit tests cubren DST y aniversario mensual recibido. El flujo legacy continúa editable; la tarjeta conserva la etiqueta **Ensayo**.

### Evidencia ejecutada por el padre

Artefacto aislado: `/home/nandev/.hermes/cache/scratch/espacigo-resume-candidate-wy8riz7k/frontend`; manifest de importación con SHA256 en el directorio padre. No se construyó sobre `.next` del dev `:3000`. Servidor de aceptación `127.0.0.1:4317`, Go `127.0.0.1:8080`; sin cuentas ni reservas creadas.

| Gate | Resultado y alcance |
|---|---|
| Go global, excluyendo `TestLiveAWSEmulatorStoreMediaAndReplayRegistry` | Exit 0; 4.696 acciones test pass, 217 skip, 0 fail. Skips no equivalen a integración DB. Log `espacigo-resume-gates/go.jsonl` en scratch. |
| Race focal | Exit 0 en rentalflow PG/dominio/HTTP, cmd/api y spaces PG/dominio/HTTP. |
| Frontend final | `typecheck`, `test`, `lint`, `api:check`: exit 0. Unit: 669 pass, 1 skip; lint: 0 errores, 20 warnings. OpenAPI/inventario: 216 operaciones. Se regeneró inventario después de desplazar líneas de mounts Go. |
| Build producción | Exit 0 en snapshot aislado. |
| Playwright mensual | 4/4 pass y repetición 4/4 pass: dos lecturas/renders nativos 1440/390 y dos fallos/vacíos simulados etiquetados, con Axe default/overflow móvil. |
| Playwright selección + handoff | 2/2 pass: API real → una consulta de intervalos → CTA con timestamps exactos → clic real → preparación con controles bloqueados y zona correcta, en ambos viewports. Sin POST. |
| Formato global | Exit 1: 10 archivos fuera del diff del corte no cumplen Prettier. Se conserva su contenido para evitar reformat ajeno. Archivos frontend modificados/nuevos del corte fueron formateados. |

Logs finales: `/home/nandev/.hermes/cache/scratch/espacigo-resume-gates/`; `browser-month-repeat.log`, `browser-selection-handoff.log`, `types-final.log`, `unit-final.log`, `lint-final.log`, `api-final.log`, `format-final.log`, `build.log`. Capturas en `selection-handoff/` y `month-repeat/`. Los cambios de pruebas posteriores al snapshot no afectan el código de producción construido.

### Matriz unificada vigente

| Fase / Bloque | Estado actual en código | Qué hay que corregir o alinear | Qué falta exactamente para dar el plan por 100% cerrado |
|---|---|---|---|
| Arriendos: lectura pública → selección → preparación | **Ya finalizado** para este corte de lectura | Mantener el intervalo de Go; no convertir estimación o ensayo en cotización comercial | Ninguno para el handoff de lectura probado; no incluye enviar solicitud |
| Arriendos: precio efectivo y cotización | **Con incongruencias/bugs** | El mensual admite precio custom por día; CTA/quote de base no acreditan ese precio. Alinear snapshot, constraints SQL y suma por unidad | Cotización real del mismo rango con tarifa especial, snapshot persistido y rechazo/reconciliación del cambio de tarifa |
| Arriendos: solicitud, ocupación y cancelación | **Parcial** | Trial persistente y flujo comercial son distintos; garantizar autoridad V98 también en el commit trial y no sólo en lecturas | Navegador autenticado → solicitud persistida → ocupación → rechazo de solapamiento → cancelación/liberación y readback |
| Arriendos: pagos/contratos/comprobantes | **Parcial** | Conservar etiquetas sintéticas y contratos actuales; no equiparar simulación con operación comercial | Aceptación local persistida de los estados y documentos exigidos por el plan, separada de proveedores reales |
| Calendarios: contrato mensual y estados error/vacío | **Ya finalizado** para transporte/render | Mantener endpoint canónico, generación OpenAPI y fail-closed | Ninguno para esta aceptación; no implica autoridad global de inventario/precio |
| Calendarios: intervalos y restricciones V98 | **Parcial** | Lecturas y `IntervalAvailable` ya filtran V98; mensual, quote trial y commit requieren equivalencia completa | Horarios, DST, overrides y concurrencia comprobados en una única traza persistida |
| Calendarios: configuración de anfitrión | **Parcial** | Cerrar contrato privado de lectura/escritura, CAS, semántica batch y carga tras recarga | Anfitrión edita regla/override/precio, readback Go/Postgres y reflejo público comprobados en navegador |
| Ambos: aceptación integral de dos cuentas | **Falta construir** como evidencia actual | No sustituir DB por mocks ni reserva por href | Journey anfitrión → público → quote → solicitud → ocupación/cancelación sobre el artefacto final |

**Siguiente trabajo asignado:** contraste de precio variable con constraints efectivos y contrato privado del calendario anfitrión, en dos auditorías focales sin escrituras. La incidencia recurrente del worker `private media cleanup pass failed` permanece abierta; health 200 no la resuelve. No se alteraron sus permisos ni roles.

## Corte posterior: configuración privada y CAS — 2026-11-16

Las auditorías focales confirmaron dos defectos: el gestor anfitrión enviaba únicamente `time_zone` sin revisión ni reglas/excepciones; y la rama de inventario del backend ignoraba el `If-Match` ya parseado. La proyección mensual pública tampoco sirve como lectura de configuración editable: no devuelve regla y excepciones privadas completas.

Implementado y verificado por el padre, todavía sin aceptación UI/persistencia nativa del gestor:

- Nuevo GET/PUT privado `/spaces/{spaceId}/availability/configuration`, sin cambiar el GET privado legado de intervalos ni el PUT legado de zona. El GET devuelve `space_id`, `time_zone`, `rule`, `overrides`, `updated_at` y ETag del espacio. Lectura de propietario dentro de transacción con bloqueo; sin IDs internos de excepciones. PUT es merge explícito por fecha y su respuesta es un acuse, no readback.
- CAS de inventario compara la revisión bajo `FOR UPDATE`. Una revisión vacía/obsoleta se rechaza antes de mutaciones; el espacio renueva su revisión también en cambios sólo de excepciones. Se propagaron fallos de lectura final y renovación de revisión. Tres pruebas RED observadas antes del fix pasaron después. Cambiar la zona existente de una publicación no borrador queda rechazado.
- Pruebas HTTP ejercitan auth, propietario ajeno, query no admitida, lectura, 428, 412, reenvío de revisión y readback. Race de spaces PG/dominio/HTTP y cmd/api pasó. Go global posterior: **4.706 pass, 217 skip, 0 fail**, live S3 excluido; log `espacigo-host-config-gates/go.jsonl` en scratch.
- Contrato OpenAPI/tipos/política/inventario: **218 operaciones**; API check pasó. API recargada, health 200; GET anónimo del nuevo recurso responde 401 directamente y mediante BFF. Esto no acredita una lectura autenticada de DB ni una escritura nativa.
- Adaptador frontend: 6/6 tests; validación de DTO/ETag, cierre `24:00`, rechazo de updates/días vacíos y omisión de opcionales `undefined`. Controlador: 5/5 tests; un PUT con revisión, GET de confirmación, incertidumbre sin replay, recuperación explícita, descarte de respuestas antiguas y bloqueo de lectura sin sesión. Typecheck y lint focal pasaron después de revisar ambos.

**Estado aún parcial:** integración del componente anfitrión y harness Playwright mock están asignados por separado; no se atribuye un browser GREEN antes de construir y ejercer el artefacto. El caso de precio variable continúa pendiente de alinear la fuente V98, el quote trial y las restricciones del snapshot; no se modificaron constraints ni se aplicaron migraciones en este corte.

## Integración visible del calendario anfitrión — 2026-11-16

La UI y el harness del apartado anterior ya se integraron y ejercitaron. **El bloque anfitrión sigue Parcial por falta de aceptación nativa de escritura**, no por ausencia del guardado UI:

- El gestor carga la configuración privada antes de editar. Hidrata días, horario, check-in/out, precio base, estadías y excepciones; conserva motivos, estados y tarifas de excepciones no modificadas. El guardado manda el lote completo por el adaptador/controlador, con CAS y GET independiente posterior.
- Check-in/out de tarifa diaria no se convierte en horario semanal. El callback semanal sólo se emite para modalidad hora. En el asistente sin ID el botón de guardar está deshabilitado y se explica que esas excepciones no se persisten antes de crear el espacio; la integración de inventario en el alta sigue pendiente.
- Se revisó el hook: snapshots vacíos estables, identidad de sesión vigente, limpieza segura en Strict Mode y descarte de operaciones antiguas. El estado incierto retira el editor y ofrece una actualización explícita, sin repetir PUT. Tras confirmación se muestra «Configuración guardada y verificada».

### Aceptación ejecutada y límites

Artefacto construido desde el árbol combinado en `/home/nandev/.hermes/cache/scratch/espacigo-host-candidate-204eyh11/frontend`, servido en `127.0.0.1:4318`. Build exit 0. Hashes de `frontend/src/` sin divergencias posteriores frente al manifest del snapshot al verificar las corridas. No se reconstruyó el dev `:3000`.

La entrega inicial del test QA usó un DTO que **no correspondía al contrato** (`days/open_time` en vez de `rule/overrides`); ese fallo no se aceptó como evidencia de producto. El padre reemplazó el fixture por `components.schemas.OwnCalendarConfiguration` y agregó interacciones reales antes del guardado. La regresión corregida falla en baseline `:4317` al no encontrar el control etiquetado con el horario cargado; pasa en el nuevo artefacto.

| Prueba sobre `:4318` | Resultado |
|---|---|
| Anfitrión con gateway simulado | 3/3 pass, dos corridas consecutivas: 1440 y 390 px cargan regla no-default; cambian día/hora y apertura de una excepción; verifican payload exacto + If-Match, un PUT, GET de confirmación y conservación tras reload. Caso 412 sin replay y recuperación explícita. |
| Mensual público | 4/4 pass: dos lecturas nativas y dos vacíos/errores controlados. |
| Selección/handoff público | 2/2 pass con API/DB reales de lectura, una consulta de intervalos y navegación hasta preparación. |
| Unit frontend final | 681 pass, 1 skip, 0 fail; typecheck exit 0; lint exit 0 con 18 warnings. API check de 218 operaciones exit 0. |

Evidencia: `/home/nandev/.hermes/cache/scratch/espacigo-host-config-gates/`: `host-baseline-red.log`, `host-repeat.log`, `public-month.log`, `public-selection.log`, `unit-final.log`, `lint-final.log`, `build.log`; capturas en `host-repeat/`. La suite del anfitrión **simula el gateway y la persistencia del fixture**: no acredita cookies BFF reales, autenticación Go, SQL de escritura, CAS concurrente PostgreSQL ni reflejo público de una mutación real. No se crearon cuentas ni reservas ni se alteraron publicaciones persistentes durante estas pruebas.

### Pendientes que no cambian por este GREEN

1. Prueba anfitrión autenticado → configuración persistida → calendario público sobre API/DB reales, con CAS concurrente y lectura posterior.
2. Incorporar base/overrides del asistente al alta sin declarar éxito local ficticio.
3. Precio diario personalizado coherente con el quote y sus restricciones/snapshot inmutable, incluida la semántica de hora/mes.
4. Solicitud real de ensayo → ocupación → rechazo de solapamiento → cancelación/liberación; pagos/documentos sintéticos con sus gates propios.
5. Incidencia del cleanup worker y formato preexistente fuera de este corte.
