# Bitácora — estado del flujo de arriendo y gates de main

**Fecha:** 2026-11-15  
**Alcance:** worktree DEMO-05 mensual; no equivale al cierre del flujo comercial A10.

## Punto de control

- Worktree: `/home/nandev/Documentos/projects/espaciGo-rent-published-space-monthly-calendar`.
- Rama: `codex/marketplace/feature/rent-published-space-monthly-calendar`.
- HEAD base: `7b01a47370d119a0433190c25a9fad7f55dc3dc6`; `origin/main` y la rama local `main` apuntaban al mismo SHA tras `git fetch origin --prune`.
- El worktree de integración local `/home/nandev/Documentos/projects/espaciGo-main-integration` está limpio en `main` al mismo SHA. El checkout habitual `/home/nandev/Documentos/projects/espaciGo` pertenece a otra rama y está dirty; se conserva intacto.
- Aún no se hizo commit, merge ni push de los cambios DEMO-05.

## Avance verificado en el candidato

- DEMO-00–04 y el recorrido local de calendarios/confirmación simulada ya están en este candidato; DEMO-05 añade aislamiento de Playwright, accesibilidad y regresiones de concurrencia.
- Se corrigió la carrera que permitía que un login pendiente restaurara sesión después de iniciar logout. `sessionBusy` mantiene mutuamente excluidos login/logout aunque cambie el espacio.
- La prueba visible difiere PUT de configuración, cambia de espacio y comprueba que no se publique un readback obsoleto. Antes de volver al espacio editado, arma una espera de request; luego valida el GET exacto y su respuesta `unit_price_clp=150001`. Es estado compartido en memoria del demo, no persistencia durable.
- El test visible recorre anfitrión desktop y arrendatario móvil, con foco/teclado, navegación, Axe, logout/cookie, mutaciones y readbacks. La interceptación limita requests cross-origin del navegador; no demuestra aislamiento de egreso server-side de Next.

## Verificación local registrada

- Playwright dedicado, configuración aislada Go en memoria + Next/BFF loopback: dos ejecuciones consecutivas tras reforzar el test, ambas `2/2 PASS`; salida en `/home/nandev/.hermes/cache/scratch/rental-demo-config-readback-causal` y `/home/nandev/.hermes/cache/scratch/rental-demo-config-readback-causal-second`.
- `pnpm api:check`: PASS, inventario vigente de 216 operaciones.
- `pnpm test`: 550; 549 pass, 0 fail, 1 skipped.
- `pnpm typecheck`: PASS.
- `pnpm lint`: PASS; 0 errores, 12 advertencias preexistentes/mixtas.
- `pnpm format:check`: PASS.
- `pnpm build` con entorno aislado: PASS después del último cambio de UI; las ediciones posteriores fueron solo prueba y documentación.
- `go test -race -count=1 ./cmd/rentaldemo ./internal/rentaldemo ./internal/calendar ./internal/pricing`: PASS, sin cambios Go posteriores.
- `git diff --check`: PASS tras el refuerzo del test y las ediciones documentales; volver a verificar antes de cualquier integración.
- La revisión read-only `deleg_725cbebd` pasó sin bloqueadores para el estado anterior. `deleg_11d5fbee` señaló que el readback podía consumir una respuesta GET no causal; el test ahora captura la solicitud armada inmediatamente antes de seleccionar el espacio y comprueba su respuesta 200. La prueba visible pasó dos ejecuciones consecutivas tras el cambio; re-revisión independiente del diff reforzado pendiente.

## Archivos incluidos en el candidato dirty

Modificados: `frontend/package.json`, `frontend/src/app/demo/arriendo/page.tsx`, `frontend/src/features/rental-demo/infrastructure/demo-openapi-adapter.ts`, `frontend/src/features/rental-demo/ui/rental-demo-view.tsx`, `frontend/tooling/rental-demo-e2e.test.mjs`, `planning/arrendamiento_espacio_publicado/evidence/demo-02-05-calendarios-arriendo-completo.md`, `planning/arrendamiento_espacio_publicado/plan_arrendamiento_espacio_publicado.md`.

Nuevos: `frontend/playwright.rental-demo.config.ts`, `frontend/tests/rental-demo-visible.spec.ts`, `planning/arrendamiento_espacio_publicado/evidence/demo-05-visible-e2e-20261115.md`, `planning/arrendamiento_espacio_publicado/evidence/estado-arriendo-y-gates-main-20261115.md` (este log).

## Gate pendiente para publicar a `main`

El trigger regional `espacigo-main-rollout` está activo (`548b8eff-e489-4307-a77f-455b56aa4e57`), conectado a `cloudbuild.yaml` y filtrado por `^main$`. El push a `main` no es solo Git: inicia Cloud Build. El último build del HEAD base `7b01a47370d119a0433190c25a9fad7f55dc3dc6` falló en `verify-source-quality`; en GitHub, `Frontend architecture and quality` falló `pnpm test`, mientras `Application quality` y `Database migration quality` pasaron. El último build trigger exitoso observado fue para `f3671834727417733a11132aef783fa9941ba012`.

`release_main.py` clasifica contra un watermark privado: el modo `web` solo despliega las webs, pero el modo `full` construye API/migrador, crea backup y ejecuta plan/apply de migraciones antes del rollout. La diferencia de fuente entre `f367183…` y el HEAD base `7b01a…` contiene 125 paths no-frontend (excluyendo docs); por tanto no es seguro inferir que el próximo push se limitaría a web. No se leyó el watermark privado en esta fase ni se ejecutó SQL, backup, despliegue o remediación del build.

**No se publicó a `main`**: antes de un push se requiere resolver el `pnpm test` fallido para el SHA exacto que vaya a publicarse y completar el preflight de release que identifique el watermark/alcance, evidencia de esquema/backup/autoridad requerida si resulta `full`, y la autorización operativa para cualquier aplicación Cloud SQL. No omitir ni desactivar gates para satisfacer el push. La identidad GitHub/Git verificada en el worktree DEMO-05 es `HernanEspinozaDev`; hooks están configurados.

## Pendientes del flujo comercial (separados de DEMO-05)

- A10 sigue parcial: no hay recorrido integrado UI→BFF→Go→PostgreSQL ni cotización comercial persistida demostrada.
- A05/A06 requieren preflight de autoridad/ledger Cloud SQL ligado al SHA, conforme al runbook incremental; no se ejecutó aquí.
- A09 conserva pendientes de perfil API productivo, identidad/autenticación, clasificación y almacenamiento durable; las pruebas de storage disponibles son sintéticas/fakes.
- Sin aceptación comercial online, reservas/pagos/firma reales ni QA del usuario inferidos por el demo.
