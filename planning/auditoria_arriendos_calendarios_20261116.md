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
| Calendarios: configuración de anfitrión | **Parcial; regla base nativa verificada** | Escritura de regla, CAS obsoleto, recarga, horario semanal y publicación comprobados mediante BFF/Go/PostgreSQL; faltan mutación/reversión nativas de overrides y tarifa especial | Completar excepciones, precios y concurrencia entre dos escritores; la prueba de revisión antigua no equivale a una carrera concurrente |
| Ambos: aceptación integral de dos cuentas | **Ensayo horario nativo verificado hasta listo para check-in** | KYC fixture autorizado; cuatro cotizaciones/reservas, pagos, garantías, aprobación y firmas simuladas con rangos persistidos. No equivale a revisión documental ni dinero/firma real | Cancelación/liberación, operación física futura, precio variable y unidades no horarias siguen como gates independientes |

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

## Publicación y aceptación nativa del calendario — 2026-11-16

**Código publicado:** `00986814fc3fed5f34f4957fbd17e1782ebfe9b6`, `main` de `HernanEspinozaDev/espacigo-aws`; SHA remoto leído después del push y autor/committer confirmados como `HernanEspinozaDev`. Se conservaron los cambios previos de Gemini. Los 58 paths del checkpoint fueron contrastados con el índice congelado; `.local/` quedó excluida. La revisión frontend pasó; la observación CAS hipotética de backend fue adjudicada sin cambiar código: el catálogo real no contiene un trigger de actualización automática de revisión y la escritura permanece bajo el mismo `FOR UPDATE` hasta commit. El primer revisor ejecutó comandos de lectura pese a su restricción de herramientas; la adjudicación acotada sólo leyó archivos y declaró `executed_commands: []`.

### Lo que sí se ejecutó sobre PostgreSQL real

Objetivo único: API `127.0.0.1:8080` y DB Docker `espacigo_aws_local`, sin cuentas AWS reales. Se usó el build aislado `:4318` a través de una entrada HTTPS loopback `:4210`, para ejercer las cookies `__Host-` y el BFF reales. No hubo mocks del gateway ni respuestas API fabricadas.

- Registro normal de **tres cuentas sintéticas nuevas**: anfitrión, arrendatario A y arrendatario B; aceptación de términos vigentes, correo verificado por Mailpit, login y perfil confirmados mediante GET y estado `activo` leído en DB. Contraseñas/tokens sólo en el fixture privado ignorado, con archivo `0600` y directorio `0700`.
- Borrador horario nuevo creado por API, tarifa **12.000 CLP/hora**, sin modificar cuentas o espacios preexistentes. Identificador público de prueba: `7aec3e2d-8ac8-441c-a906-18020289b0d0`.
- En el navegador anfitrión se guardó la regla **lunes a domingo, 10:00–18:00**, precio de calendario 12.000, estadía 1–30. PUT con `If-Match` observado → 200 → GET independiente 200 → recarga de página conserva 10:00–18:00. Una escritura explícita con la revisión anterior respondió **412** y el GET posterior conservó la revisión y la regla nuevas. Esto prueba rechazo de revisión obsoleta, no una carrera simultánea.
- El horario semanal habitual se guardó desde su pestaña y luego se publicó el espacio desde la interfaz. Ambos PUT devolvieron 200; el PUT semanal utilizó el ETag devuelto por la configuración previa. GET posterior: siete días con tramo 10:00–18:00 y estado `activa`. Son operaciones distintas del guardado de la regla de inventario, no una escritura única ficticia.
- Postcheck SQL independiente: **1 regla base y 1 configuración semanal**, con valores 10:00–18:00 y precio 12.000. El GET semanal usa el envelope real `data.days[].periods[]`, no el modelo UI `weekly_slots`: se corrigió una aserción del harness después del primer guardado sin repetir esa mutación.
- Navegador público **390 px**: el 17 de noviembre de 2026 aparece `AVAILABLE`; mes e intervalos responden 200. API devuelve 13 alternativas de dos horas. Se seleccionó **10:00–12:00 America/Santiago**, conservando exactamente `2026-11-17T13:00:00Z`–`2026-11-17T15:00:00Z` en la CTA. Cero mutaciones, cero solicitudes externas y sin desborde horizontal. Capturas anfitrión/móvil inspeccionadas visualmente. Son alternativas disponibles, **no 13 reservas ni tickets vendidos**.

### Bloqueo de las reservas solicitadas

La primera cotización real de A para ese intervalo respondió **404 `not_found`**. La consulta trial exige elegibilidad KYC vigente del anfitrión y el commit de reserva revalida ambas partes. Las cuentas recién registradas no tienen esa elegibilidad: verificar correo no equivale a aprobar identidad.

Se preguntó por autorización específica para preparar KYC sintético exclusivamente en estas tres cuentas. La solicitud **quedó sin respuesta por timeout**; no se aprobó KYC, no se cambiaron roles/controles y no se usaron cuentas existentes como sustitución. El postcheck SQL del espacio confirma **0 cotizaciones, 0 reservas y 0 ocupaciones**. Por tanto, **no están probados** todavía dos arrendatarios en horas diferentes del mismo día ni en fechas distintas; no se declara ese flujo terminado.

El contrato actual admite `hora/dia/mes`, no una unidad `semana`. No se ejecutaron reservas diarias o semanales ni se afirma que un precio del calendario sea una cotización. Próximo gate, condicionado a decisión de KYC: preparar la elegibilidad sintética expresamente autorizada o completar la verificación normal; después ejecutar cuatro solicitudes no solapadas A/B en dos fechas, leer reservas y rangos de ocupación desde DB y probar rechazo de solapamiento. Mantener el namespace trial visible y separado del arriendo/pago comercial.

Receipts, scripts y capturas nativos permanecen en `.local/testing/ticket-native-20261116/`, ignorado; no versionar el fixture con credenciales. El gateway HTTPS temporal se retira al cerrar este corte; API, frontend y los registros nuevos se conservan para continuación.

### CI del SHA publicado

- Application quality: **success**, run `43931181525`.
- Infrastructure as Code Quality: **success**, run `43931181609`.
- Frontend architecture and quality: **failure**, run `43931181650`; `pnpm format:check` identifica nueve archivos de formato fuera del checkpoint de código. No es un fallo de los tres tests de calendario, repetidos 3/3 antes del push.
- Workflow DB: **failure**, run `43931178770`, sin jobs; causa no adjudicada en este corte. No afirmar CI completamente verde ni despliegue por el push.

## Continuación autorizada: cuatro reservas horarias persistidas — 2026-11-16

La instrucción humana posterior autorizó expresamente simular KYC y marcar verificadas **sólo las tres cuentas nuevas de prueba**. Esto sustituye el bloqueo por falta de autorización del apartado anterior; no representa una revisión real ni autoriza cambios globales de seguridad.

### Fixture de identidad y límites

Se insertaron tres casos `local-fixture-v1`, sus elegibilidades KYC y dos eventos de historial por caso. La aprobación del anfitrión además exige la aserción documental propia: se guardó una declaración sintética por el API cifrado, se leyó de nuevo y se sembró exclusivamente un caso aprobado por cada cuenta, vinculado a esa revisión y con vigencia de doce meses. Los IDs y la etiqueta **QA simulada, no revisión documental real** permanecen en el receipt privado. No se subieron documentos ficticios para aparentar una revisión, no se consultó proveedor externo y no se deshabilitó el verificador. Tampoco se concedieron roles administrativos ni se modificaron cuentas preexistentes.

El primer intento de aprobación respondió 409: el KYC legacy no sustituye la aserción documental y el pago no sustituye la autorización de garantía. Se completaron esos prerrequisitos de prueba sin crear otra reserva ni repetir pagos confirmados. Todas las operaciones de arriendo, pago, garantía, aprobación y firma se ejecutaron después mediante **navegador → BFF HTTPS loopback → Go → PostgreSQL**; sólo la precondición de identidad se preparó como fixture autorizado.

### Escenarios conservados en la base de datos

Zona: **America/Santiago**. Espacio: `7aec3e2d-8ac8-441c-a906-18020289b0d0`. Precio: 12.000 CLP/hora; cada reserva son dos horas y subtotal 24.000 CLP, excluida la garantía simulada.

| Escenario | Fecha local | Horario local | Reserva persistida | Estado final |
|---|---|---|---|---|
| Arrendatario A / día 1 | 2026-11-17 | 10:00–12:00 | `161cedd8-a512-4cfe-842f-395f084f4643` | `lista_para_checkin` |
| Arrendatario B / día 1 | 2026-11-17 | 14:00–16:00 | `3517b4c2-9dbc-4be0-99c7-e414d61d42f0` | `lista_para_checkin` |
| Arrendatario A / día 2 | 2026-11-18 | 10:00–12:00 | `be704a27-f0a8-4166-8f70-3f199b0c1c68` | `lista_para_checkin` |
| Arrendatario B / día 2 | 2026-11-18 | 14:00–16:00 | `4e7cb6ee-5283-4d50-85da-a11625ed13e0` | `lista_para_checkin` |

Postcheck SQL independiente, con aserciones programáticas sobre **los cuatro IDs exactos**, confirma cuatro snapshots de quote concordantes con sus reservas, cuatro rangos `[inicio,termino)` exactos en `ocupacion`, activos, tipo `reserva`, sin expiración; cuatro contratos `firmado`; cinco transiciones por reserva. Dos arrendatarios, dos fechas, **8 horas / 96.000 CLP de arriendo simulado**, cero pares activos solapados. Las firmas son simuladas y no tienen validez jurídica. No se adelantó el reloj para fingir check-in/check-out: la aceptación termina en listo para check-in, no arriendo físico finalizado.

Controles negativos reales mediante BFF: intentar cotizar B sobre el tramo 10:00–12:00 ocupado por A responde **409**; B no puede leer la reserva de A (**404**). La lectura pública de intervalos elimina los ocupados y conserva exactamente 12:00–14:00 y 16:00–18:00 en ambas fechas. Recarga de las cuatro pantallas de reserva mantiene contratos firmados y estado; no hay solicitudes de navegador fuera del origen local.

### Bug descubierto por el recorrido, no por mocks

Con reservas 10:00–12:00 y 14:00–16:00, la proyección mensual antigua marcaba `BOOKED` al detectar ocupación en ambos presets mañana/tarde. El navegador deshabilitaba el día aunque el API de intervalos devolvía dos huecos reales. Se conservó el RED de navegador y se corrigió sólo el estado mensual **por hora**: cobertura recortada/unida de ocupaciones respecto a 10:00–18:00; ningún solapamiento → `AVAILABLE`, cobertura parcial → `PARTIAL`, cobertura total → `BOOKED`. Rango operativo vacío/inverso → `BLOCKED`; `PAST`, bloqueo de override y reglas diarias/mensuales conservados.

Pruebas del helper: doce casos (huecos, adyacencia, unión desordenada/solapada, límites half-open, clipping, cierre 24:00 y rangos inválidos). El padre observó RED específico de rangos inválidos y GREEN posterior. Race de spaces PG/dominio/HTTP y cmd/api, vet y build API: exit 0. Suite Go global final: **4.719 pass, 217 skip, 0 fail**, con live S3 excluido. API reiniciada conservando la misma DB; GET mensual posterior devuelve `PARTIAL` para los días 17/18. El harness nativo final pasó **dos veces consecutivas**: recarga de las cuatro reservas, contratos, denegaciones, intervalos y día público móvil nuevamente seleccionable. No se repitieron las cuatro creaciones para fabricar una segunda corrida.

Evidencia privada: `.local/testing/ticket-native-20261116/{fixture.json,postgres-bookings-proof.json,book-native.mjs,sign-and-verify.mjs,verify_postgres.py}`, capturas `*-signed.png` y `public-occupied-mobile.png`. No publicar archivos de credenciales. El script inicialmente consultó contrato por GET en la ruta de creación (405); se corrigió al GET por ID del contrato ya persistido, sin recrearlo. La comprobación repetida también se corrigió para navegar a la reserva antes de recargar cuando las firmas ya existían.

**Cerrado para este ensayo:** anfitrión publica/configura, dos arrendatarios seleccionan distintos tramos en el mismo día y en otro día, cotizan, solicitan, simulan pago/garantía, reciben aprobación y firman; persistencia y ocupación exactas verificadas. **No cerrar los planes completos:** quedan tarifas especiales, overrides nativos, cancelación/liberación y operación física; no se probaron días/semanas/meses ni se añadió unidad `semana`. CI histórico de formato/workflow DB no queda corregido por estas pruebas locales.
