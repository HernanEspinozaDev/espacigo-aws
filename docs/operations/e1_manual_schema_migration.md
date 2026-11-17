# Operación local E1 — contrato de arriendo manual

## Alcance

V99 (`V000099__early_rental_manual_contract.sql`) es una migración manual y
`review_required`, nunca una publicación automática. Agrega cinco tablas, siete
funciones de trigger y el vínculo de ocupación; conserva las migraciones V1–V98,
los owners y las autoridades preexistentes. No habilita E2 ni los endpoints del
flujo: `FlowReady=false` sigue siendo obligatorio hasta completar sus gates.

El comando específico es `cmd/e1migrate`. El runner genérico rechaza V99 pendiente
antes de inicializar ledger o aplicar un prefijo; un V99 ya aplicado sólo admite
el no-op con historia coincidente. No usar SQL ad hoc como sustituto del comando.

## Destino y autorización

Para este corte el usuario autorizó expresamente inspección READ ONLY, backup y
aplicación condicionada a todos los gates sobre `espacigo_aws_local`, contenedor
`espacigo_localbackend_20261024-database-1`, ID completo
`76b02541fc7d934d014ea0faa2d5e8b0ce9d600c2c319e729a6e56b5bbfe24ec`.
El contenedor conserva etiquetas de otro worktree: la autorización es específica
y no convierte otros targets en equivalentes.

Identidad inspeccionada: PostgreSQL 18.6 (`180006`), OID de DB `23865`, clúster
`7700108872039940141`. Principal: `espacigo_migrator_login`, NOINHERIT y no elevado;
rol efectivo existente: `espacigo_schema_owner`, owner de ocupación y ledger.
No cambiar credenciales, roles, grants de autoridad ni ownership para pasar gates.
Los grants/revokes del delta se limitan a sus objetos nuevos y preservan defaults.

El transporte puede ser socket Unix o loopback explícito. Si se usa
`scripts/local-postgres-socket-relay.py`, comprobar el ID completo antes de abrir
un listener sólo en `127.0.0.1`, y conservarlo exclusivamente durante la operación.
El relay no selecciona DB, lee secretos ni ejecuta SQL. La identidad SQL se
comprueba nuevamente en cada transacción; un puerto saludable no prueba target.
Cerrar el proceso propio al terminar, sin detener contenedores o servicios ajenos.

## Artefactos necesarios

1. **Fuente publicada y revisada:** SHA Git completo, inventario V1–V99 con nombres
   y SHA-256 exactos y fingerprint de inventario. `SourceManifest` verifica Git,
   checkout y manifest; staging escribe los bytes capturados, no una relectura.
2. **Perfil independiente:** `planning/evidence/e1_authority_profile_20261117.json`.
   Conserva roles/membresías/ACL/defaults legacy; agrega sólo los permisos mínimos
   del SQL nuevo. Compara 14 columnas de catálogo ACL: seis clases leídas en scope
   y ocho baselines explícitos preservados. Los hashes de exclusión son específicos
   del destino, no fingerprints semánticos portables entre clusters restaurados.
3. **Backup privado actual:** custom `PGDMP`, fuera de Git y de tmpfs, owner actual,
   modo 0600, SHA-256/tamaño/pins de DB/clúster/motor/fuente. Deben pasar listado y
   parse completo con `pg_restore`, no sólo el header o `--list`. El comando pasa
   el descriptor abierto al parser y revalida el archivo posteriormente.
4. **Operación JSON privada 0600**, sin credenciales, que contiene `Source`,
   `Migration`, `Before`, `After`, `Backup` y `RestoreBinary`. Su SHA-256 debe fijarse
   de forma independiente mediante `--operation-sha256`; nunca aprender un perfil
   observado como un contrato aceptado desde el modo de aplicación.
5. **Recibos privados** de preflight, ejecución y postcheck. No pegar dumps ni
   configuración privada en el chat o repositorio.

## Secuencia del comando

Compilar desde el SHA revisado, sin arrancar el API ni workers. Los paths de
operación/staging/recibos los fija el operador; staging debe ser un directorio
nuevo absoluto. El comando rechaza variables `PG*` de override; no hace login con
un administrador ni lee contraseñas de archivos para este transporte local.

- `--mode inspect`: sólo observación READ ONLY no aprobada; sin staging ni DDL.
- `--mode preflight`: valida manifest/staging/backup y compara predecessor y perfil
  BEFORE en transacción READ ONLY estable. No aplica SQL.
- `--mode apply`: exige confirmación literal
  `espacigo_aws_local:7700108872039940141:99`, además del pin de operación. Repite
  identidad, historia y autoridad bajo el lock de migraciones y transacción
  SERIALIZABLE. Rechaza cualquier discrepancia sin grants/reparaciones automáticas.
- `--mode postcheck`: relectura independiente del límite AFTER; no stages ni DDL.

Todos requieren `--operation`, `--operation-sha256` y `--repo` absolutos; preflight y
apply requieren `--stage`. Sólo apply admite `--confirm`. Consultar `--help` para
los flags efectivos; no combinar con las rutas históricas de `cmd/dbmigrate`.

La comparación BEFORE se repite antes de DDL. AFTER se comprueba antes del commit,
en la postlectura del writer y mediante otra conexión. La exclusión GiST,
relaciones/constraints/triggers y checksum de ledger también deben verificarse por
SQL independiente antes de cerrar E1. El ensayo de DB no sustituye la futura UI.

## Resultado y recuperación

`CommitOutcome` distingue:

- `not_started`: esta invocación no inició la aplicación.
- `not_committed`: el writer rechazó/abortó antes de intentar commit.
- `unknown`: commit ambiguo o reconciliación no concluyente; **no repetir apply**.
- `committed`: commit confirmado, aunque un postcheck pueda fallar.

`Verified=true` requiere postcheck completo. Un `Committed=false` aislado no prueba
rollback si `CommitOutcome=unknown`. El CLI conserva el error de reconciliación en
stderr incluso si falla la escritura JSON de stdout.

Ante resultado ambiguo o fallo posterior al commit: preservar staging, backup y
recibos; ejecutar únicamente postcheck readback-first. No borrar la fila del
ledger, reescribir SQL aplicado ni restaurar el dump automáticamente. Una deriva
real exige un diagnóstico y delta de reparación revisado y autorizado, no replay.

## Pruebas y límites

El harness `scripts/test-migration-runner-postgres.sh full` usa contenedor propio
sin red/puertos/volúmenes persistentes, requiere cada caso nominal y rechaza skips.
Mantiene fixtures históricos explícitos V1–V98 y prueba V99 canónica aparte sobre
fresh/upgrade. Incluye bilateralidad, motivo terminal cerrado, expiración no
prematura, ACL nueva mínima frente a defaults amplios, rol efectivo existente,
rollback y no-op/replay. Los tests de CLI verifican pin, permisos, JSON y fallos de
salida. El backup real se valida con un opt-in explícito y no se imprime.

Cerrar E1 sólo después de aplicar el SHA exacto y releer V1–V99, catálogo, owners,
ACL y preservación legacy en el destino autorizado. El estado/evidencia de cierre
se registra en el plan canónico, sin declarar E2–E7 completas.
