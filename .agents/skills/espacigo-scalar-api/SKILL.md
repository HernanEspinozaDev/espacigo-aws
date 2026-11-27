---
name: espacigo-scalar-api
description: Mantener OpenAPI 3.1 y Scalar al crear, cambiar o retirar operaciones HTTP o contratos de cualquier módulo de EspaciGo. Usar también al incorporar integraciones que cambien la API de la aplicación.
---

# API y Scalar de EspaciGo

Trabaja en el worktree de la tarea. Usa `planning/openapi.yaml` como contrato fuente de la API de negocio. Scalar lee este contrato y la vista completa incluye todos sus módulos.

1. Contrasta el contrato con el registro de rutas y el handler Go. Comprueba método, ruta, perfil de ejecución, flags, permisos efectivos y comportamiento del BFF si lo consume Next.js. Para una integración externa, documenta la operación de EspaciGo y enlaza la documentación del proveedor cuando aporte contexto.
2. Documenta en el mismo cambio los métodos y rutas, `operationId` estables y únicos, categoría, propósito, entradas, respuestas y errores, autenticación y permisos. Incluye condiciones de entorno y, cuando apliquen, paginación, idempotencia y concurrencia. Usa datos sintéticos en ejemplos. Conserva compatibilidad con los consumidores existentes y explica retiros o cambios incompatibles.
3. Ejecuta `pnpm api:generate` y `pnpm api:check` desde `frontend/`. Revisa los tipos generados, la lista de operaciones del BFF y la matriz `planning/frontend_next/evidence/matriz-operaciones-api-fase3.md`. Un registro de prefijo por sí solo no prueba que la operación esté montada.
4. Verifica las rutas afectadas con las pruebas pertinentes. En desarrollo, comprueba en `http://127.0.0.1:8080/docs` o `http://127.0.0.1:8080/openapi.json?scope=all` que Scalar recibe el contrato esperado, con el alcance global. Para operaciones de escritura, usa pruebas aisladas o fixtures. Si el entorno impide la comprobación en vivo, indica qué validaste y qué falta.

Mantén el visor y el contrato interno fuera del acceso público. Excluye secretos, credenciales, datos personales y respuestas reales de proveedores. Conserva los permisos de negocio en Go. Un cambio exclusivamente visual o interno, sin cambios al contrato, no requiere regeneración.
