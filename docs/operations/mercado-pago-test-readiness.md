# Readiness para probar Mercado Pago con HTTPS

**Estado del código:** fase 1–4 integrada en `desarrollo-local` hasta `cc6d25bb7df723a4252f688bb3bbbb1f0b44763b`; el arreglo del receiver Go está en el commit local `4aafc8cbd0570d833c32b9a544b9e5f949767c3c`; el BFF dedicado y contrato OpenAPI están en `714efd1`.
**Estado de pruebas externas:** sin configuración de credenciales/cuentas ni prueba manual a través de un túnel público. No se inició OAuth ni se creó una preferencia real durante esta auditoría.
**Alcance:** preparación de un test con credenciales y cuentas de prueba. No habilita producción, configura AWS, cambia cuentas ni publica código.

## Estado verificable

- El callback OAuth público pasa por Next en `/api/backend/host/mercadopago/oauth/callback`. Su BFF limita los parámetros de retorno, no reenvía cookies al backend y filtra el redirect final a la página de Mercado Pago del anfitrión.
- El webhook público ahora tiene ruta Next dedicada en `/api/v1/webhooks/mercadopago`, alineada con `servers: /api/v1` en OpenAPI. El BFF conserva query y bytes del body, permite solo `data.id` y `type`, reenvía `x-signature`, `x-request-id` y `content-type`, y no reenvía cookies, autorización u origen.
- El receiver Go toma `data.id` de la query como valor canónico firmado; rechaza query ausente, duplicada, malformada o ambigua. Si el body incluye `data.id`, exige igualdad con la query. El test integrado HTTP usa la URL realista `?data.id=…&type=payment`.
- La prueba focalizada del BFF, `api:check`, typecheck, lint y suite frontend fueron reportadas por el agente frontend como exitosas. En Go pasaron `go test -count=1 ./internal/checkoutpro`, compilación de `cmd/api` y `git diff --check` para el arreglo local. La prueba PostgreSQL del E2E webhook/preferences/outbox pasó anteriormente en un clon desechable V109→V113; no se ha repetido con el nuevo arreglo de query y no se usó la base principal ni la preview V106.
- El recorrido OAuth + webhook por túnel HTTPS de Next sigue pendiente. Las pruebas de UI/BFF y fixture con proveedor fake no comprueban el túnel, el panel de Mercado Pago, una notificación entregada por Mercado Pago ni una preferencia de prueba real.

La aplicación crea Preferences sin el campo `notification_url`. La configuración prevista para este corte es Webhooks → Prueba → Pagos a nivel de aplicación Mercado Pago, apuntando a la URL pública indicada aquí; no se ha demostrado aún una entrega Marketplace 1:1 con una cuenta de prueba. La prueba manual debe comprobar que el panel entrega eventos para la preferencia creada y que el receiver acepta la firma. No inferir que falta entrega solo por no enviar `notification_url`; si el panel no notifica al vendedor/test, detenerse y revisar el alcance documentado por Mercado Pago antes de cambiar la preferencia.

## Prueba local por un único túnel HTTPS a Next

Usa un solo hostname público HTTPS, reenviado al puerto local de Next (`127.0.0.1:3000`). No abras túneles directos a Go ni PostgreSQL. Las dos URL públicas viven en el mismo origin:

| Función | URL pública del túnel |
| --- | --- |
| OAuth callback que se registra en la aplicación Mercado Pago | `https://<origin>/api/backend/host/mercadopago/oauth/callback` |
| Webhook de pagos que se registra en Webhooks de prueba | `https://<origin>/api/v1/webhooks/mercadopago` |
| Retorno Checkout Pro | `https://<origin>` (la aplicación agrega la ruta de retorno) |

Next reenvía internamente a Go mediante `API_BACKEND_BASE_URL=http://127.0.0.1:8080/api/v1`. Go escucha en loopback y PostgreSQL sigue local. El `x-signature`, `x-request-id`, query y body deben llegar al receiver sin transformación; el test BFF cubre el proxy local, pero no verifica todavía el proveedor ni el túnel externo.

Antes de arrancar:

1. Aplica V110–V113 mediante el flujo local normal y verifica plan/checksums. La DB aislada usada en las pruebas anteriores ya fue retirada; no reutilices el ledger histórico de pagos V106. Mantén separadas las bases de desarrollo y cualquier DB de producción.
2. Prepara dos cuentas de prueba chilenas distintas: anfitrión/vendedor y arrendatario/comprador. Usa una base local con datos sintéticos. `MERCADOPAGO_TEST_MODE=true` comprueba que el OAuth recibido sea de prueba, pero el backend no impone una allowlist fija de anfitrión, arrendatario y espacio. No expongas una sesión autenticada de prueba a personas ajenas al test.
3. Guarda `MERCADOPAGO_CLIENT_ID`, `MERCADOPAGO_CLIENT_SECRET`, `MERCADOPAGO_OAUTH_REDIRECT_URI`, `MERCADOPAGO_TOKEN_ENCRYPTION_KEY`, `MERCADOPAGO_TEST_MODE=true` y `MERCADOPAGO_WEBHOOK_SECRET_FILE` en `.local/secrets/mercadopago-test.env`, propiedad del usuario y modo `0600`. El archivo de firma separado también debe ser regular, privado y modo `0600`. No uses credenciales `live`, no las pongas en argumentos de shell, `NEXT_PUBLIC_*`, logs ni Git.
4. Configura en el panel de la aplicación de prueba el callback OAuth exacto de la tabla y, en Webhooks → Prueba → Pagos, registra `https://<origin>/api/v1/webhooks/mercadopago`. Esa es una configuración de aplicación; el código no pasa `notification_url` en cada Preference. Comprueba durante el test que la notificación de esa Preference efectivamente llega.
5. Arranca el API local con `COMMERCIAL_RENTAL_FLOW_ENABLED=1`, `MERCADOPAGO_CHECKOUT_ENABLED=1` y los origins públicos configurados para el origin del túnel. Mantén `LOCAL_AUTH_PROTOTYPE=1`, `APP_PROFILE` vacío, runtime estándar, `HTTP_ADDR=127.0.0.1:8080`, conexión DB `direct` a loopback y reloj normal. El script `scripts/local-dev.sh api` selecciona `LOCAL_BOOKING_TRIAL=0` cuando el modo comercial está activo. Levanta Next con el mismo `FRONTEND_PUBLIC_ORIGIN` y el backend loopback, y recién entonces inicia el túnel hacia Next.
6. Usa el mismo `https://<origin>` sin path para `FRONTEND_PUBLIC_ORIGIN`, `CORS_ALLOWED_ORIGINS` y `MERCADOPAGO_RETURN_ORIGIN`; define `MERCADOPAGO_OAUTH_REDIRECT_URI` como origin más la ruta callback. Nunca navegues primero por localhost y luego regreses al hostname del túnel: la sesión del anfitrión está ligada al origin del navegador.
7. Vincula primero el anfitrión de prueba, inicia una cotización comercial con la cuenta arrendataria y paga solo con saldo/medios de prueba. El redirect no es confirmación: la página consulta el estado al backend, y la transición requiere notificación firmada más lookup autenticado del pago.
8. Confirma aprobación/rechazo, idempotencia, outbox en Mailpit y que una query o body manipulados no cambien el estado. Si falla cualquiera de esos gates, detén el test; no intentes con una cuenta real para diagnosticar.

El modo comercial local requiere autenticación prototipo. Aunque la guard limita API y DB a loopback, el túnel expone Next al Internet durante la sesión. Úsalo solo por la ventana necesaria, con cuentas y datos sintéticos; cierra el túnel al terminar. No conviertas esta configuración local en una tarea ECS ni actives flags de producción.

## Corte hospedado de prueba: no disponible

No hay runtime ECS/infra aislado preparado para alojar el checkout de prueba. En `APP_PROFILE=production`, `validateProductionRuntime` rechaza `COMMERCIAL_RENTAL_FLOW_ENABLED`; el guard comercial exige perfil vacío, auth prototipo y API/DB loopback. Que `MERCADOPAGO_CHECKOUT_ENABLED` exista en configuración no elimina esas guards. La tarea productiva no monta las rutas comerciales de cotización/checkout ni debe usarse para probarlas.

Para proponer un target hospedado haría falta un corte de infraestructura separado, con aprobación y revisión propias:

- un servicio/entorno aislado y una base de prueba separada de producción, con `APP_PROFILE` y guard dedicados; no reutilizar la auth prototipo ni retirar guards productivos;
- scope server-side de prueba para anfitrión, arrendatario y espacio; `test_mode` por sí solo valida modo de token, no limita quién puede usar el endpoint comercial;
- secretos OAuth, clave de cifrado y firma de webhook en AWS Secrets Manager, con permiso de lectura mínimo para el execution role y sin variables `NEXT_PUBLIC_*`;
- resolver `MERCADOPAGO_WEBHOOK_SECRET_FILE`: hoy Go exige un archivo regular privado y el contenedor tiene filesystem raíz de solo lectura sin volumen/materializador para ese secreto. Diseñar su montaje/materialización segura o cambiar el puerto de configuración en un cambio revisado separado;
- origins HTTPS, DNS/certificado ACM y ruta pública que pase por Next/NLB, conservando el webhook query/body/header sin transformación; nunca exponer el listener Go;
- migraciones aprobadas con la clasificación/checksum exactos, plan del ledger del target y prueba normal en base aislada. La aplicación tiene que desplegar un SHA compatible; no aplicar migraciones manuales ni reusar el ledger de la fixture descartada;
- gates de seguridad y release: revisión del diff exacto, pruebas frontend/Go, `api:check`, CI del SHA, evidencia del schema y autorización explícita de promoción. Un cambio Terraform, permisos IAM, tarea ECS, secrets o DNS es entrega de infraestructura independiente.

No hay configuración AWS, secretos, cuentas de Mercado Pago ni despliegue autorizado en este readiness. No añadir valores de prueba a `infra/terraform/environments/aws-main` ni promover el checkout a `main` como parte del test local.

## Separación de gates

| Gate | Evidencia | Límite |
| --- | --- | --- |
| Código y contrato | Tests Go, prueba focalizada Next, `api:check`, OpenAPI y BFF alineados | No prueba Internet, panel ni entrega del proveedor |
| Esquema | V110–V113 aplicadas con ledger/checksums exactos en DB local/aislada autorizada | No configura OAuth ni Webhooks |
| Ingress de túnel | Un origin HTTPS hacia Next; callback OAuth y webhook BFF comprobados en ese hostname | No es un target hospedado ni demuestra pagos |
| Proveedor test | Cuentas de prueba distintas, anfitrión vinculado como test, Preference/lookup y HMAC recibido | No demuestra modo live, aceptación legal o go-live |
| Hosted readiness | Infra, aislamiento, secretos, allowlist y gates enumerados y revisados | Actualmente no implementado; requiere nuevo alcance |
| Promoción | Candidato exacto desde `desarrollo-local`, CI/gates aprobados y autorización expresa | No equivale a habilitar pagos ni a configurar cuentas |

Mantén CI/CD de producción fuera de este recorrido. La publicación en `main` puede activar los workflows de release; no publicar ni promover desde esta preparación.
