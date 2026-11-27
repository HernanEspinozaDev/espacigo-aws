# Matriz de evidencia y cumplimiento técnico de privacidad (Fases 8–9)

Este documento consolida la trazabilidad técnica de privacidad de EspaciGo (migraciones, privilegios de base de datos, contratos HTTP, capacidades granulares, MFA reciente `428`, simulacro de fiscalización y gobierno de entregas), complementando el [procedimiento operativo de privacidad](./privacy-operations.md) y el [plan del administrador global](./plan-administrador-global-espacigo.md).

## 1. Marco regulatorio y vigencia

- **Régimen vigente hoy:** **Ley 19.628** sobre protección de la vida privada.
- **Régimen futuro modificado:** **Ley 21.719**, modificada por la **Ley 21.855** (publicada el 23 de noviembre de 2026), cuya entrada en vigor vigente es el **15 de octubre de 2027**.
- **Alcance operativo y técnico:** Las capacidades implementadas en base de datos, API Go y consola Next.js preparan la operación de derechos ARCO+ (`acceso`, `rectificacion`, `supresion`, `oposicion`, `portabilidad` y `bloqueo` temporal), portabilidad interoperable, restricciones de tratamiento, gestión de incidentes y retenciones legales. Esta implementación técnica **no afirma exigibilidad normativa anticipada** antes del 15 de octubre de 2027 y mantiene sujetas a aprobación jurídica las reglas de plazos, excepciones, verificación de identidad y notificaciones a la autoridad.

---

## 2. Trazabilidad de migraciones `V000022`..`V000028`

Todas las migraciones de privacidad están clasificadas en [`db/release-policy.json`](../../db/release-policy.json) con estrategia `expand` (aditiva) y diseñadas para ejecutarse con la identidad única de migración `espacigo_migrator_login` en sesión asumiendo el rol efectivo y propietario `espacigo_schema_owner` (`SET ROLE espacigo_schema_owner`). En `V000026`, `V000027` y `V000028` existe además una guarda explícita que exige `session_user = 'espacigo_migrator_login'`, `current_user = 'espacigo_schema_owner'`, `current_schema() = 'public'` y privilegio `CREATE` en `public` (permitiendo superusuario únicamente para instancias desechables de prueba).

| Migración | Checksum SHA-256 (`db/release-policy.json`) | Estrategia y `requires_schema_owner` | Propósito técnico | Ownership y privilegios mínimos (`espacigo_runtime`) |
| --- | --- | --- | --- | --- |
| [`V000022__privacy_rights_case_workflow.sql`](../../db/migrations/V000022__privacy_rights_case_workflow.sql) | `f73df5baeb38534182ba9ee6724f43bc20fc5c7facabaa2629faa60c825a1516` | `expand`<br>`public.solicitud_titular` | Amplía `public.solicitud_titular` a los 6 tipos ARCO+ (`acceso`, `rectificacion`, `supresion`, `oposicion`, `portabilidad`, `bloqueo`), agrega `solicitado_por`, `motivo_administrativo`, `caso_ref_administrativo` y `respuesta_titular` con restricción de consistencia. | Objeto propiedad de `espacigo_schema_owner`. Concede únicamente `UPDATE (respuesta_titular)` sobre `public.solicitud_titular` a `espacigo_runtime`. |
| [`V000023__privacy_rights_intake.sql`](../../db/migrations/V000023__privacy_rights_intake.sql) | `0d22b4572626dd775ad3c88ea4758b5f4b946f8018be578e910e23fd85551418` | `expand`<br>`public.solicitud_titular` | Agrega `detalle_titular` a `public.solicitud_titular` y valida longitud mínima (`>= 10` caracteres para `rectificacion` y `oposicion`, `1..1000` en general). | Modifica `public.solicitud_titular` bajo `espacigo_schema_owner` sin ampliar permisos de escritura de `espacigo_runtime`. |
| [`V000024__privacy_response_dispatch_receipt.sql`](../../db/migrations/V000024__privacy_response_dispatch_receipt.sql) | `3dad61951952b956ea03fb1813a93799b71a8052553f59ccfddab2fbb97f8244` | `expand`<br>— | Crea la tabla `public.privacy_response_dispatch_receipt` para registrar la remisión externa de respuestas de acceso (`email` o `postal`, `destination`, `sent_at`, `evidence_ref`). | Tabla propiedad de `espacigo_schema_owner`; `REVOKE ALL` de `PUBLIC` y `espacigo_runtime`; `GRANT SELECT, INSERT` a `espacigo_runtime` (registro inmutable sin `UPDATE`/`DELETE`). |
| [`V000025__privacy_case_lifecycle_deadlines_and_events.sql`](../../db/migrations/V000025__privacy_case_lifecycle_deadlines_and_events.sql) | `e510f03b75195b4a1dc4128fce43d7c0daa7d28cf26f48070e47b45054322d72` | `expand`<br>`public.solicitud_titular` | Agrega `asignado_a_id`, `vence_en` (2 días para `bloqueo`, 30 días para el resto), `prorrogada_hasta` (máx. +31 días), `prorroga_motivo`, concurrencia optimista `version` e índices; crea la bitácora append-only `public.solicitud_titular_evento`. | `GRANT UPDATE (asignado_a_id, vence_en, prorrogada_hasta, prorroga_motivo, version)` sobre `public.solicitud_titular` a `espacigo_runtime`. En `public.solicitud_titular_evento`: `REVOKE ALL` de `PUBLIC`/`espacigo_runtime` y `GRANT SELECT, INSERT` a `espacigo_runtime`. |
| [`V000026__privacy_public_intake_and_outbox.sql`](../../db/migrations/V000026__privacy_public_intake_and_outbox.sql) | `8e1769401b3f376ba187b7cbadcb79840ebd306559c6b3dcc01e6bed07ba9464` | `expand`<br>`public.solicitud_titular` | Habilita solicitudes sin cuenta (`usuario_id` nullable, `folio_publico` único `PRIV-...`, `solicitante_tipo`, datos de representante, `consulta_token_hash` de 32 bytes y canales `publico`/`correo`); crea la cola durable `public.privacidad_notificacion_outbox`. | `GRANT UPDATE (usuario_id, folio_publico)` sobre `public.solicitud_titular` a `espacigo_runtime`. En `public.privacidad_notificacion_outbox`: `REVOKE ALL` de `PUBLIC`/`espacigo_runtime` y `GRANT SELECT, INSERT, UPDATE (estado, intentos, disponible_en, reclamado_en, entregado_en, terminal_en, ultimo_error_codigo)` a `espacigo_runtime`. |
| [`V000027__privacy_rights_execution_and_restrictions.sql`](../../db/migrations/V000027__privacy_rights_execution_and_restrictions.sql) | `571771857a57fe9886fd8441646867dbb15890456acb4dd851e9fbecba8e8142` | `expand`<br>Tabla creada en `V000025` durante la misma tanda | Amplía `tipo_evento` con `ejecucion_derecho` y crea `public.privacidad_preferencia_finalidad`, `public.privacidad_bloqueo_tratamiento` (índice único parcial para bloqueos `activo`) y `public.privacidad_ejecucion_derecho`. | `REVOKE ALL` de `PUBLIC`/`espacigo_runtime` en las tres tablas. Concede a `espacigo_runtime`: `SELECT, INSERT, UPDATE` en `privacidad_preferencia_finalidad`; `SELECT, INSERT, UPDATE (estado, revisar_en, levantado_por, levantado_en, motivo_levantamiento)` en `privacidad_bloqueo_tratamiento`; y `SELECT, INSERT` (append-only) en `privacidad_ejecucion_derecho`. |
| [`V000028__privacy_incidents_and_legal_holds.sql`](../../db/migrations/V000028__privacy_incidents_and_legal_holds.sql) | `d21142c19be3e0cf0f32169cd5f7b384f18130c4151a65cc216138080569be59` | `expand`<br>— | Crea `public.privacidad_incidente_datos`, la línea temporal append-only `public.privacidad_incidente_evento` y `public.privacidad_retencion_legal` (retenciones legales por cuenta, solicitud o incidente). | `REVOKE ALL` de `PUBLIC`/`espacigo_runtime` en las tres tablas. Concede a `espacigo_runtime`: `SELECT, INSERT, UPDATE` en `privacidad_incidente_datos`; `SELECT, INSERT` (append-only) en `privacidad_incidente_evento`; y `SELECT, INSERT, UPDATE (estado, revisar_en, liberada_por, liberada_en, motivo_liberacion)` en `privacidad_retencion_legal`. |

> **Nota operativa:** En producción y local, la API conecta como `espacigo_api_login` con `SET ROLE espacigo_runtime`, por lo que nunca ejecuta DDL ni dispone de `DELETE` sobre bitácoras, recibos de despacho o ejecuciones de derechos.

---

## 3. Matriz de endpoints, capacidades granulares y MFA `428`

Autoridades de definición:
- Capacidades del dominio de identidad: [`internal/identity/domain.go`](../../internal/identity/domain.go) (`privacy.requests.read`, `privacy.requests.manage`, `privacy.export.approve`, `privacy.incidents.read`, `privacy.incidents.manage`).
- Contrato HTTP: [`planning/openapi.yaml`](../../planning/openapi.yaml) y [`planning/frontend_next/evidence/matriz-operaciones-api-fase3.md`](../../planning/frontend_next/evidence/matriz-operaciones-api-fase3.md) (`425` operaciones OpenAPI inventariadas).
- Handlers autoritativos en Go: [`internal/identity/transport/http/handler.go`](../../internal/identity/transport/http/handler.go) y [`internal/adminglobal/transport/http/specialized_ops.go`](../../internal/adminglobal/transport/http/specialized_ops.go). En mutaciones administrativas protegidas, `recentlyAuthenticated` ([`internal/adminglobal/transport/http/handler.go`](../../internal/adminglobal/transport/http/handler.go)) exige autenticación MFA reciente (`recentAuthenticationTTL = 10m`) y responde `HTTP 428 Precondition Required` (`recent_authentication_required`) cuando falta o expiró.

### 3.1 Rutas públicas anónimas (sin cuenta ni sesión)

| Método y ruta API | `operationId` OpenAPI | Autenticación / Capacidad | MFA `428` | Propósito y controles |
| --- | --- | --- | --- | --- |
| `POST /api/v1/privacy/public-requests` | `submitPublicPrivacyRequest` | Anónimo (sin Bearer) | No | Ingreso público ARCO+ para titular o representante (`mandatario`, `representante_legal`, `tutor_nna`). Emite folio `PRIV-...` y código de seguimiento (almacenado solo como hash SHA-256). |
| `POST /api/v1/privacy/public-requests/lookup` | `lookupPublicPrivacyRequest` | Anónimo (folio + código de seguimiento) | No | Consulta de estado, respuesta publicada y eventos con visibilidad `titular`; oculta notas y eventos `interna`. |

### 3.2 Rutas autenticadas del titular

| Método y ruta API | `operationId` OpenAPI | Autenticación / Capacidad | MFA `428` | Propósito y controles |
| --- | --- | --- | --- | --- |
| `GET /api/v1/rights-requests` | `listOwnRightsRequests` | Bearer titular (`UserOperation`) | No | Lista los folios propios del titular (`Cache-Control: no-store`). |
| `POST /api/v1/rights-requests` | `requestDataRight` | Bearer titular (`UserOperation`) | No | Registra solicitud de derecho ARCO+ con detalle opcional/obligatorio (`HTTP 202`). No ejecuta automáticamente la acción solicitada. |
| `GET /api/v1/privacy/preferences` | `listOwnPrivacyPreferences` | Bearer titular (`UserOperation`) | No | Consulta preferencias de finalidades opcionales (`comunicaciones_comerciales`, `analitica_producto`, `personalizacion_recomendaciones`, `comparticion_encargados_opcionales`). |
| `PUT /api/v1/privacy/preferences` | `updateOwnPrivacyPreference` | Bearer titular (`UserOperation`) | No | Actualiza estado (`permitido`, `opuesto`, `retirado`) y causal de una finalidad opcional. |
| `GET /api/v1/privacy/restrictions` | `listOwnPrivacyRestrictions` | Bearer titular (`UserOperation`) | No | Lista bloqueos/restricciones de tratamiento vigentes o históricos de la cuenta (`Cache-Control: no-store`). |
| `GET /api/v1/privacy/portability` | `exportOwnPrivacyPortability` | Bearer titular (`UserOperation`) | No | Descarga paquete JSON estructurado e interoperable (`espacigo-portabilidad-v1.json`) con cabecera `X-Content-SHA256` y `Cache-Control: no-store`. |
| `GET /api/v1/privacy/export` | `exportOwnIdentityData` | Bearer titular (`UserOperation`) | No | Exportación JSON de datos propios del titular. |
| `GET /api/v1/privacy/export/archive` | `downloadCompleteOwnDataArchive` | Bearer titular (`UserOperation`) | No | Descarga archivo ZIP completo de datos propios (`espacigo-datos-propios.zip`, `Cache-Control: no-store`). |

> **Nota sobre rutas administrativas heredadas de supresión y purga local:** En [`internal/identity/transport/http/handler.go`](../../internal/identity/transport/http/handler.go), las rutas `/api/v1/privacy/suppression-requests`, `/api/v1/privacy/suppression-requests/{request_id}/review`, `/api/v1/privacy/suppression-requests/{request_id}/execute` y `/api/v1/privacy/retention/purge` requieren rol `administrador` en su superficie histórica/local.

### 3.3 Rutas administrativas de privacidad, incidentes y retenciones legales

Todas se montan en `registerProductionAdminAccountRoutes` ([`cmd/api/main.go`](../../cmd/api/main.go)) bajo sesión administrativa global (`administrador_global`) y auditoría correlacionada.

| Método y ruta API | `operationId` OpenAPI | Capacidad granular requerida (Go) | MFA `428` | Control adicional |
| --- | --- | --- | --- | --- |
| `GET /api/v1/admin/global/privacy-requests` | `listGlobalAdminPrivacyRequests` | `privacy.requests.read` o `privacy.requests.manage` | No | Paginación y filtros (`status`, `right_type`, `account_id`, `assigned_to_id`, `channel`, `overdue`). |
| `POST /api/v1/admin/global/privacy-requests` | `createGlobalAdminPrivacyRequest` | `privacy.requests.manage` | No | Crea folio ARCO+ administrativo con identidad pendiente de comprobación. |
| `GET /api/v1/admin/global/privacy-requests/{requestID}` | `getGlobalAdminPrivacyRequest` | `privacy.requests.read` o `privacy.requests.manage` | No | Detalle del expediente, eventos (`titular` e `interna`), outbox y ejecuciones. |
| `POST /api/v1/admin/global/privacy-requests/{requestID}/workflow` | `transitionGlobalAdminPrivacyRequest` | `privacy.requests.manage` | **Sí (`428`)** | Transición de estado, asignación, verificación de identidad, notas internas o prórroga con `expected_version`. |
| `POST /api/v1/admin/global/privacy-requests/{requestID}/resolution` | `resolveGlobalAdminPrivacyRequest` | `privacy.requests.manage` | **Sí (`428`)** | Publica resolución al titular (`resuelta` o `denegada_fundada`) y encola aviso en outbox. |
| `POST /api/v1/admin/global/privacy-requests/{requestID}/dispatch-receipt` | `recordGlobalAdminPrivacyDispatchReceipt` | `privacy.requests.manage` | **Sí (`428`)** | Registra comprobante de remisión externa (`email` o `postal`) de respuesta ya publicada. |
| `POST /api/v1/admin/global/privacy-requests/{requestID}/notifications/retry` | `retryGlobalPrivacyNotifications` | `privacy.requests.manage` | **Sí (`428`)** | Reintenta notificaciones pendientes o terminales en `privacidad_notificacion_outbox`. |
| `POST /api/v1/admin/global/privacy-requests/{requestID}/executions` | `executeGlobalPrivacyRight` | `privacy.requests.manage` | **Sí (`428`)** | Ejecuta y deja constancia en `privacidad_ejecucion_derecho` (rectificación, oposición, bloqueo, levantamiento, portabilidad o propagación a encargado) con `expected_version`. |
| `GET /api/v1/admin/global/privacy-incidents` | `listGlobalPrivacyIncidents` | `privacy.incidents.read` o `privacy.incidents.manage` | No | Lista incidentes de datos personales filtrados por `status` y `severity`. |
| `POST /api/v1/admin/global/privacy-incidents` | `createGlobalPrivacyIncident` | `privacy.incidents.manage` | **Sí (`428`)** | Abre incidente (`INC-...`), asigna líder/suplente y registra evento inicial `apertura`. |
| `GET /api/v1/admin/global/privacy-incidents/{incidentID}` | `getGlobalPrivacyIncident` | `privacy.incidents.read` o `privacy.incidents.manage` | No | Devuelve ficha de incidente y cronología append-only `privacidad_incidente_evento`. |
| `POST /api/v1/admin/global/privacy-incidents/{incidentID}/transitions` | `transitionGlobalPrivacyIncident` | `privacy.incidents.manage` | **Sí (`428`)** | Avanza estado (`contencion`, `evaluacion`, `notificacion_agencia`, `notificacion_titulares`, `nota`, `cierre`) con `expected_version`. |
| `GET /api/v1/admin/global/privacy-legal-holds` | `listGlobalPrivacyLegalHolds` | `privacy.incidents.read`, `privacy.incidents.manage`, `privacy.requests.read` o `privacy.requests.manage` | No | Lista retenciones legales filtradas por `status`, `scope` y `account_id`. |
| `POST /api/v1/admin/global/privacy-legal-holds` | `createGlobalPrivacyLegalHold` | `privacy.incidents.manage` | **Sí (`428`)** | Aplica retención legal (`HOLD-...`) bloqueando purga/baja sobre el ámbito indicado. |
| `POST /api/v1/admin/global/privacy-legal-holds/{holdID}/release` | `releaseGlobalPrivacyLegalHold` | `privacy.incidents.manage` | **Sí (`428`)** | Libera retención legal exigiendo `release_reason` documentado. |
| `POST /api/v1/admin/global/approvals` y `.../decisions` (`action_type: privacy_export`) | `createGlobalAdminApproval` / `decideGlobalAdminApproval` | `approvals.request` / `approvals.decide` (+ capacidad `privacy.export.approve` en catálogo de dominio) | **Sí (`428`)** | Doble control (`solicitante != aprobador`) para aprobaciones de exportación de privacidad (`privacy_export`). |

---

## 4. Guía reproducible de simulacro de fiscalización y validación técnica (Fases 8–9)

Ejecuta todos los comandos desde la raíz del repositorio (o worktree correspondiente) para reproducir la validación completa antes de promover cambios.

### 4.1 Verificación del plan de migraciones y checksums

```bash
# Verificar estado de migraciones aplicadas y pendientes contra db/release-policy.json
./scripts/local-dev.sh migrate-plan

# Aplicar migraciones pendientes en el PostgreSQL local usando espacigo_migrator_login -> espacigo_schema_owner
./scripts/local-dev.sh migrate

# Comprobar checksums SHA-256 locales de V000022..V000028
sha256sum db/migrations/V00002{2,3,4,5,6,7,8}*.sql
```

### 4.2 Pruebas unitarias y de contrato HTTP/OpenAPI en Go

```bash
go test ./internal/privacy/... ./internal/adminglobal/... ./internal/identity/... ./internal/openapicontract/... ./cmd/api/...
```

### 4.3 Simulacro de fiscalización y pruebas de integración en PostgreSQL 18 desechable

Las pruebas de integración crean una base temporal aislada, ejecutan `migrator.Run` sobre `db/migrations` y validan comportamiento y restricciones de privilegios reales en PostgreSQL 18 cuando `TEST_DATABASE_URL` apunta a una instancia administrativa de pruebas:

```bash
# Obtener la credencial administrativa local (si usas el contenedor local)
ADMIN_PASS="$(cat .local/secrets/db_admin_password)"
export TEST_DATABASE_URL="postgres://espacigo_admin:${ADMIN_PASS}@127.0.0.1:5432/espacigo_aws_local?sslmode=disable"

# 1) Simulacro reproducible de fiscalización Fase 9 (reconstrucción de expediente, ARCO+, bloqueo, incidente y retención legal)
go test -v ./internal/adminglobal -run TestPrivacyPhase9FiscalizacionDrill

# 2) Suite completa de integración PG 18 para privacidad, administración global y migrador
go test ./internal/adminglobal ./internal/adapters/postgres/identity ./internal/migrator
```

### 4.4 Validación de contrato OpenAPI, inventario y Scalar local

Referencia de contrato e inventario: [`planning/openapi.yaml`](../../planning/openapi.yaml) y [`planning/frontend_next/evidence/matriz-operaciones-api-fase3.md`](../../planning/frontend_next/evidence/matriz-operaciones-api-fase3.md) (`425 operations` inventariadas).

```bash
# Verificar sincronización entre planning/openapi.yaml, tipos generados e inventario (desde frontend/)
cd frontend
pnpm api:check

# Si modificaste planning/openapi.yaml, regenera primero:
# pnpm api:generate && pnpm api:check
```

Para inspeccionar el visor **Scalar** local (solo disponible cuando `ENVIRONMENT=development` y `LOCAL_AUTH_PROTOTYPE=1` en `./scripts/local-dev.sh api`):

1. Inicia la API local con `./scripts/local-dev.sh api`.
2. Abre en navegador o consulta por HTTP:
   - Vista completa: `http://127.0.0.1:8080/docs?scope=all` (especificación JSON en `http://127.0.0.1:8080/openapi.json?scope=all`).
   - Vista administrativa: `http://127.0.0.1:8080/docs?scope=admin`.
3. Confirma que figuran las operaciones de `/privacy/*`, `/rights-requests` y `/admin/global/privacy-*` junto con sus respuestas `428`.

### 4.5 Build de frontend y pruebas Playwright (desktop/móvil y rutas públicas/admin)

```bash
cd frontend

# Compilación de producción Next.js + validación de rutas
pnpm build

# Pruebas Playwright de privacidad pública, cuenta titular y consola administrativa
pnpm exec playwright test tests/privacy-public-intake.spec.ts
pnpm exec playwright test tests/phase6-account.spec.ts tests/phase9-admin.spec.ts
```

---

## 5. Guía de separación de commits y aprobaciones humanas pendientes

### 5.1 Separación recomendada de commits (Conventional Commits)

De acuerdo con [`AGENTS.md`](../../AGENTS.md), separa la entrega en commits atómicos y excluye cualquier cambio exclusivo del entorno local al promover a `main`:

1. **Commit 1 — Migraciones SQL y política de liberación (`feat(db): ...`)**
   - Incluye únicamente `db/migrations/V000022__*.sql`..`V000028__*.sql` y `db/release-policy.json`.
   - Verifica previamente que todas las migraciones sean aditivas (`expand`), respeten `espacigo_schema_owner` / `espacigo_runtime` y coincidan sus checksums SHA-256.
2. **Commit 2 — Código compartido de aplicación, OpenAPI y UI (`feat(privacy): ...`)**
   - Incluye `internal/identity/`, `internal/privacy/`, `internal/adminglobal/`, `internal/adapters/postgres/identity/`, `cmd/api/`, `planning/openapi.yaml` y `frontend/src/` (junto con los archivos generados por `pnpm api:generate`).
3. **Commit 3 — Evidencia operativa y pruebas de simulacro (`docs(privacy): ...` / `test(privacy): ...`)**
   - Incluye `docs/operations/privacy-operations.md`, `docs/operations/privacy-compliance-evidence-matrix.md`, `docs/operations/plan-administrador-global-espacigo.md`, `planning/frontend_next/evidence/matriz-operaciones-api-fase3.md`, `internal/adminglobal/privacy_fiscalizacion_test.go` y `frontend/tests/`.
4. **Exclusiones permanentes de promoción a `main`:**
   - Cualquier ajuste en `scripts/local-dev.sh`, `scripts/local-aws-bootstrap.sh`, `scripts/iniciar-local.sh`, configuración del emulador AWS, Mailpit o fixtures sintéticos permanece únicamente en `desarrollo-local`.

### 5.2 Cierre técnico, revisión jurídica y salida a producción

La integración técnica y la preparación de los documentos cuentan con autorización para `desarrollo-local`. El plan conserva estos hitos para el seguimiento de cada entrega.

1. **Revisión jurídica y de gobierno (Fase 2 / [`privacy-operations.md` §9](./privacy-operations.md)):**
   - Hotusoft es el operador y responsable del tratamiento. La dirección identificó a Hernán Maximiliano Espinoza Castillo como responsable y a Anita Belen Marchant Pacheco como suplente operativa. El paquete jurídico v1.2 queda disponible para presentación. La firma del acta, la aceptación firmada de la suplencia y la conformidad del abogado de cabecera de Hotusoft quedan para una etapa posterior.
   - Aprobación jurídica de plazos legales, reglas de suspensión/prórroga, método proporcional de verificación de identidad sin cuenta, matriz de rechazo/excepciones, tabla de retención y eliminación por clase de expediente, y protocolo de notificación a la Agencia y titulares (art. 14 sexies de la Ley 21.719 modificada por la Ley 21.855) antes del **15 de octubre de 2027**.
   - El contrato OpenAPI y Go aplican `privacy.incidents.manage` para crear y liberar retenciones. La consulta administrativa admite el permiso de lectura correspondiente.
2. **Aprobación de promoción de ramas:**
   - Aprobación e integración en `desarrollo-local` tras pasar pruebas Go, PG 18, `pnpm api:check`, `pnpm build` y Playwright.
   - Autorización explícita del usuario para promover los commits compartidos aprobados a `main`, verificación de `release-ready` en GitHub Actions y avance de `publicacion-aws`.
3. **Comprobación en infraestructura AWS y borde (RDS / ECS / Cloudflare Access):**
   - **RDS PostgreSQL 18:** ejecutar la comprobación y aplicación de `V000022`..`V000028` mediante la Lambda privada usando `espacigo_migrator_login` -> `espacigo_schema_owner` antes de actualizar la aplicación (y retirar recursos temporales solo cuando el usuario lo indique).
   - **ECS Fargate (`APP_PROFILE=production`):** validar arranque de tarea privada (`api` + `web` por loopback `127.0.0.1:8080`), conexión `espacigo_api_login` -> `espacigo_runtime` con `DATABASE_SSLMODE=verify-full`, entrega de correos del outbox de privacidad vía Zoho SMTP real y ausencia de flags locales (`LOCAL_*`).
   - **Cloudflare Access / borde administrativo:** si se habilita host administrativo separado o acceso protegido por Cloudflare Access, validar políticas de acceso, recuperación y bypass controlado, manteniendo en Go la verificación autoritativa de sesión administrativa, capacidad granular y MFA reciente `428`.
