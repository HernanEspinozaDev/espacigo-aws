# Ejecución: marketplace local — Fase 1, cortes 1.1–1.4

## Autoridad y alcance

- Repositorio: `espacigo-aws`; rama autorizada: `main`.
- Base de inicio: `761fd7e3db6e53bd4eec444738033a5e859979c3`.
- Instrucción humana: ejecutar los cuatro cortes 1.1–1.4 del plan de arriendo y modernización del marketplace; iniciar 1.1 y 1.2 con propiedad de archivos separada.
- Plan canónico: `planning/plan_flujo_arrendamiento_local_aws.md`. Este documento registra ejecución/evidencia, no lo reemplaza.
- Entorno exclusivo: AWS Emulator `https://127.0.0.1:9297`, cuenta `000000000000`; PostgreSQL `espacigo_aws_local`; Mailpit local. Sin acceso a nube real ni solicitud de credenciales IAM.
- No modificar ni versionar `.local/`, ni introducir migraciones o alterar datos persistentes como efecto lateral del rediseño.
- Delegación comprobada: proveedor `openai-codex`, modelo `gpt-6-luna`, razonamiento `medium` (perfil solicitado `gpt-6-luna-medio`). El padre revisa, ejecuta gates finales y realiza los commits; los ejecutores no publican ni commitean.

## Preflight realizado

`bash ./scripts/local-dev.sh status` terminó con código 0:

- Emulador saludable y cuenta local correcta.
- Cinco buckets y trece parámetros.
- PostgreSQL disponible con 97 migraciones aplicadas.
- Mailpit disponible en SMTP `1025` y web `8025`.

Se leyeron `AGENTS.md`, la guía de arquitectura local, el plan canónico, las reglas frontend y los puntos de entrada de catálogo/DTO/calendario/reserva antes de delegar.

## Baseline anterior a los cambios

Se extrajo el HEAD inicial a un directorio aislado bajo el scratch de Hermes, sin copiar `.local/` ni configuración privada. En ese árbol:

- `go test ./internal/publiccatalog/... ./internal/adapters/postgres/publiccatalog/...`: código 0.
- `pnpm typecheck`: código 0.
- `pnpm build`: código 0. La primera preparación con `node_modules` enlazado fuera del workspace fue rechazada por Turbopack; copiar las dependencias dentro del árbol aislado resolvió únicamente ese problema del harness, sin cambios de producto.
- Capturas BEFORE desde standalone aislado, contra catálogo Go local existente: desktop 1440×1100 y móvil 390×844. Ambas mostraron desborde horizontal por una descripción continua sin espacios. Se preservan fuera de Git en el scratch como `marketplace-before-desktop.png` y `marketplace-before-mobile.png`.
- No se reinició la API ni el frontend de desarrollo para estas capturas.

## Matriz de aceptación

| Corte | Entrega requerida | Evidencia requerida | Estado |
|---|---|---|---|
| 1.1 | Detalle público con aforo, atributos tipados completos, anfitrión público mínimo, cancelación y garantía desde fuentes reales | RED/GREEN Go, contrato OpenAPI, ausencia de PII, consulta/scan compatibles | En ejecución |
| 1.2 | Tres columnas superiores, resumen expandible, calendario existente, mosaico/miniaturas/lightbox y protección anti-desborde | RED/GREEN de componentes y posterior navegador desktop/móvil | En ejecución |
| 1.3 | Detalles inferiores, anfitrión, reseñas o vacío honesto y barra móvil | DTO mapeado, comportamiento renderizado y accesibilidad | Pendiente |
| 1.4 | Una selección autoritativa compartida por calendario/tarjeta/barra móvil, CLP exacto y navegación con parámetros conservados | Pruebas de cambios de selección, navegación, errores y sesión; typecheck/test/build; navegador | Pendiente |

## Límites funcionales que deben conservarse

- Atributos públicos: proyectar únicamente campos definidos por el perfil de categoría; no enviar JSON privado arbitrario.
- Anfitrión: no divulgar correo, teléfono, RUT, identificador privado, dirección exacta ni documentos. El sello deriva de la verificación vigente, no de una aprobación histórica.
- Cancelación y garantía: no derivar la política del anuncio desde reservas históricas de terceros. Cuando no exista una fuente pública vigente para el espacio, informar ausencia/null y no inventar montos o promesas.
- Reseñas: no inventar calificaciones, comentarios ni recuentos; usar un estado vacío cuando no exista una fuente pública autorizada.
- El calendario y las estimaciones no crean ocupación, reserva ni pago. Preservar la autoridad de Go y las reglas de sesión/CAS existentes.
- La ruta actual `/reservas/nueva` lee `space_id`; la instrucción solicita `spaceId`. Reconciliar ambos contratos explícitamente al implementar el handoff, preservando fechas y modalidad; no producir un enlace que descarte la selección.
- El lanzador actual activa `LOCAL_BOOKING_TRIAL=1`; el flujo comercial tiene una activación distinta e incompatible. No convertir el ensayo en una confirmación comercial por un cambio visual ni activar pagos/firma por inferencia.
- Los tipos OpenAPI y la allowlist deben regenerarse después de fijar el contrato backend, antes de implementar sus consumidores.
- Un fallo preexistente de `api:check` corresponde a una matriz Markdown de inventario ausente; registrar separadamente cualquier reparación necesaria del tooling, sin atribuirlo a credenciales o nube.

## Evidencia de cierre

Pendiente de las entregas, revisión independiente y ejecución del padre. No se declara ningún corte completo por la sola asignación de un subagente.

## Comprobaciones intermedias del padre

- El contrato OpenAPI y sus tipos ya incluyen `capacity_max`, `host`, `attributes`, `cancellation_policy` y `guarantee` en el detalle, no en la lista. Cancelación y garantía se documentan como `null`: no existe una política pública por anuncio que permita informar condiciones o un monto sin inventarlos.
- Se reparó el inventario Markdown generado ausente, creando su directorio y ejecutando el generador existente. `node tooling/api-check.mjs` terminó con código 0 y verificó 215 operaciones. Es una reparación del artefacto de inventario, no un cambio de autorización del gateway.
- Se compiló un harness temporal con el repositorio y los handlers Go reales en una extracción aislada del HEAD más los seis archivos de producción backend modificados. No usa `cmd/api`, no inicia workers y solo admite GET/HEAD públicos o GET de disponibilidad. Cada conexión verifica `espacigo_aws_local`, rol `espacigo_runtime` y `default_transaction_read_only=on`; el destino Docker se confirmó como `172.24.0.3`.
- En ese harness, `/health/ready`, lista y detalle respondieron HTTP 200. Se recorrió el inventario público completo disponible: **una publicación**. Aforo positivo, cuatro claves públicas exactas del anfitrión, sello coincidente con la verificación vigente, cancelación/garantía nulas y ausencia de los nuevos campos en el listado. La publicación disponible no tiene atributos expuestos: este recorrido demuestra ejecución real de la consulta, pero no sustituye los tests tipados positivos. No se alteraron datos, esquema, permisos, el servidor `:8080` ni archivos privados.
- Una build aislada de la UI superior compiló; el navegador detectó un fallo real no cubierto por los guards de fuente: Next Image rechazaba las rutas BFF optimizadas y el fallback absoluto colapsaba la caja del lightbox, bloqueando el clic de navegación. Se preservó el RED y se añadieron casos para fotos rotas y fallback de variante a original. El GREEN de navegador sigue pendiente; el PASS de typecheck/tooling de un ejecutor no se acepta como cierre de la galería.
- Las entregas parciales backend se conservaron y acotaron a reparaciones pequeñas. La suite completa del padre aún tiene una regresión del fake HTTP: no reconoce la nueva consulta de atributos. No se presenta ese fallo de fixture como un problema de PostgreSQL ni se declara 1.1 completo antes de repetir la suite combinada.
