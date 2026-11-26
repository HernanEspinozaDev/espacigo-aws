# Auditoría del flujo de pagos con Mercado Pago

## Checkout Pro en producción — 26 de noviembre de 2026

El informe de Mercado Pago del 25 de noviembre de 2026 registra **74/100 y aprobación** para el pago productivo `190475305601`. La revisión de CloudWatch en `/aws/ecs/espacigo-app-prod`, desde las 00:00 UTC del 25 de noviembre hasta la consulta, encontró 19 eventos de notificación y verificación para ese ID. Incluyen `signature_valid=true`, `amount_clp=1036`, `status=approved` y `result=matched`. Son eventos de un solo pago. Tras reactivar el token, `mpcli payments get` confirmó `approved/accredited`, $1.036 CLP cobrados, $39 de comisión de Mercado Pago, $72 de split y $925 netos. El recurso mostró $0 reembolsados y `refunds=[]`. El listado independiente devolvió `ERROR_RESOURCE_NOT_FOUND: Payment not found` y la orden comercial respondió `Caller id must be the same as collector id.`. Falta contrastar el historial con la autorización del vendedor. La revisión consistió en consultas de solo lectura. [Plan de calidad](./plan-calidad-checkout-pro.md) · [Plan de cancelaciones y reembolsos](./plan-cancelaciones-y-reembolsos-mercado-pago.md)

### Calidad de la integración

| Criterio | Estado en la rama de contratos |
| --- | --- |
| Cantidad, precio unitario y título del ítem | Enviados en el único ítem de la preferencia. |
| Descriptor del extracto | Agregado como `ESPACIGO`. La visualización en el extracto depende del emisor. |
| URL de retorno, webhook y referencia externa | Enviados desde Go. La referencia corresponde al ID interno de la operación. |
| Nombre, apellido y correo | Se toma el correo de la cuenta activa. El nombre y los apellidos se obtienen de la identidad verificada vigente. La preferencia usa `payer.surname` conforme al contrato del endpoint. Las preferencias de prueba omiten datos de cuenta de EspaciGo. |
| Categoría, descripción e ID del ítem | Agregados con la categoría `services`, una descripción del arriendo y el ID estable del espacio. |
| Vigencia de la preferencia y máximo de cuotas | Implementados. Se envían el rango de vigencia y una cuota. |
| Excluir tipos o medios de pago | Exclusión por tipo aplicada en pruebas. El filtro de medios por ID y nuevas exclusiones de producción quedan pendientes de una decisión de producto. |
| Mensajes de respuesta | Implementados en la pantalla de estado. La confirmación procede de la consulta al proveedor. |
| Consulta del pago notificado | Implementada. Se valida la firma del webhook y se consulta el pago con la autorización del vendedor. El pago productivo aportado tiene evidencia de este flujo. |
| Reembolso total o parcial | Implementado cuando se solicita desde una reserva pagada. Go usa el token OAuth del vendedor, el ID de pago y una clave de idempotencia persistida. |
| Cancelación de un pago pendiente | Falta la acción saliente. El webhook ya reconoce la confirmación remota `cancelled`. |
| Contracargos | Hay registro de la incidencia para conciliación. Falta un flujo operativo para presentar evidencia por la API de disputas. |
| Reportes de liquidaciones y de todas las transacciones | El panel actual usa snapshots por pago. Falta importar los reportes oficiales de liquidaciones y movimientos de saldo. |
| Respuesta binaria, fecha para efectivo, anuncios, apertura modal y logo | Configuración opcional. El flujo usa una redirección y admite estados pendientes. Fecha de efectivo solo aplica si se habilita ese medio. |
| Envío y dirección | El arriendo de un espacio carece de envío. La dirección del comprador es opcional y queda fuera del payload. |
| Identificación y teléfono del comprador | Quedan fuera del payload. Se envían los datos de identidad necesarios para el criterio del informe. |
| SDK frontend y backend | MercadoPago.js V2 existe para el checkout de tarjeta. Checkout Pro crea la preferencia con un cliente HTTP de Go. El SDK backend es una práctica opcional para este flujo. |

El puntaje de 74 corresponde a la preferencia histórica, que estos cambios no modifican. La herramienta MCP de evaluación exige un pago reciente con credenciales de prueba. El ID productivo se contrastó con los registros de producción y el informe adjunto. [Referencia de preferencias de Checkout Pro](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/create-preference/post)

### Cancelación y reembolso

| Estado verificado del pago | Operación |
| --- | --- |
| Sin ID de pago | Cerrar la opción de pago local y dejar expirar la preferencia. Antes de liberar una reserva con resultado incierto, conciliar por referencia externa. |
| `pending` o `in_process` | Mercado Pago admite cancelar con `PUT /v1/payments/{id}` y `status: cancelled`. Para habilitar el botón en EspaciGo, consultar el estado justo antes y aplicar la transición de reserva al confirmar la respuesta o el webhook. |
| `approved` | Mostrar la política y el monto que corresponden a la reserva. Solicitar el reembolso por el pago original. Usar el cuerpo vacío para el total o enviar el monto para un parcial. |
| Reembolso enviado, respuesta pendiente o incierta | Mantener la solicitud pendiente. Conciliar por ID de reembolso o volver a usar la clave de idempotencia que ya se guardó. |
| Reembolso realizado desde el portal de Mercado Pago | Cuando el webhook informa un total sin solicitud local asociada, el código envía el caso a conciliación. Antes de cerrar la reserva, comprobar monto, estado, pago y autorización. Vincular los reembolsos parciales al mismo historial. |
| Reembolso rechazado | El flujo actual conserva la reserva activa. Si la causa confirmada es falta de saldo, el vendedor puede reponer su parte y volver a tramitarse el reembolso. |

En split 1:1, Mercado Pago carga a vendedor y marketplace la parte correspondiente. Si el vendedor no cuenta con fondos, el marketplace puede cubrir su parte pero el total no se procesa por el mismo canal. EspaciGo puede ofrecer la solicitud y el seguimiento desde la reserva con autorización del vendedor. Una devolución por otra vía requiere un comprobante y un registro operativo que evite duplicar el abono. [Split 1:1 en Chile](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace) · [Reembolsos y cancelaciones](https://www.mercadopago.cl/developers/es/docs/checkout-pro-preferences/additional-settings/refunds-and-cancellations)

Pruebas para una siguiente activación del flujo pendiente: consultar el pago antes de cancelar, simular aprobación concurrente, confirmar que la reserva se actualiza una vez, probar reembolso total, parcial y rechazado, y conciliar un reembolso hecho fuera de EspaciGo antes de permitir un nuevo intento.

---

## Auditoría histórica de Checkout API, 20 de noviembre de 2026

**Fecha de corte:** 20 de noviembre de 2026

**Alcance:** revisión de código local, registros locales disponibles y configuración de prueba descrita en la sesión. No se creó ningún pago ni se modificó configuración externa durante esta auditoría.

## Resultado ejecutivo

EspaciGo está intentando usar **Checkout API / Payments API** en el formulario de tarjeta. No es Checkout Pro. El navegador tokeniza la tarjeta con MercadoPago.js; Go crea el pago autenticándose con el token OAuth del anfitrión, que funciona como vendedor del marketplace.

El último intento comprobable llegó a `POST https://api.mercadopago.com/v1/payments` y Mercado Pago respondió `HTTP 400`, `bad_request`, causa `2198`. La respuesta no incluyó un `payment_id`. El código registra la causa como `unclassified`, por lo que **la evidencia disponible no determina qué dato exacto rechazó Mercado Pago**. El sistema local no marcó el pago como aprobado. Tampoco se consultó al proveedor para demostrar que no exista un registro remoto, así que no repitas este intento ni concluyas que no hubo operación remota basándote solo en la ausencia del ID local.

Hay un problema propio del flujo que debe corregirse antes de seguir probando: después del rechazo 400, la respuesta inmediata del endpoint indica `provider_error`, pero no persiste ese estado ni limpia el reclamo de envío. La fila queda `pending` hasta vencer la cotización; en la lectura posterior de la base local ya aparecía `expired`. Consultar estado o repetir el POST no vuelve a enviar el pago ni busca uno remoto por referencia.

**Conclusión:** OAuth del anfitrión, captura de tarjeta y envío a Payments API están conectados hasta la respuesta de error del proveedor. No hay evidencia de un pago aprobado ni de una transacción con ID verificable. La causa `2198` permanece sin explicación confirmada y la recuperación local ante error necesita reparación.

## Qué se revisó y límites

- Código fuente y configuración de la copia local en `desarrollo-local`. La rama tiene modificaciones locales sin confirmar. El código que se está inspeccionando no demuestra por sí solo qué SHA está sirviendo el túnel en cada momento.
- Logs locales en `.local/logs/api.log`, sin copiar ni documentar tokens, datos de tarjeta, contraseñas, códigos de verificación o secretos.
- Checklist oficial de calidad de Mercado Pago para la aplicación de Checkout API configurada en esta sesión. El MCP devolvió criterios redactados para **Preferencias**; se identifican como no aplicables o parciales donde la integración actual usa Payments API.
- Estado local durante la auditoría: `scripts/local-dev.sh status` disponible, Go `/health/ready` y Next `/acceso` respondieron HTTP 200, PostgreSQL local tenía 115 migraciones. Esto no verifica cuál compilación sirve el dominio público.
- No se ejecutó `quality_evaluation`: Mercado Pago no devolvió un `payment_id` para evaluarlo.
- No se consultaron cuentas reales, credenciales de producción ni registros de transacciones reales. No se hicieron cargos ni se cambió configuración de Mercado Pago o Cloudflare.

## Identidades y credenciales por función

| Función | Identidad de prueba | Uso dentro del flujo |
| --- | --- | --- |
| Anfitrión vendedor | `Seller Test User` | Autoriza EspaciGo por OAuth. Su token autoriza la llamada de pago y su cuenta es el destino vendedor previsto. No se confirmó ningún abono porque no hubo pago aprobado. |
| Cuenta marketplace integradora | `espacigo1` | Cuenta de prueba de la integración. No sustituye al comprador ni al vendedor anfitrión en el flujo del arriendo. |
| Arrendatario comprador | `Buyer Test User` | Inicia sesión como usuario de EspaciGo y proporciona los datos de pagador/tarjeta de prueba en el formulario seguro. |

Las contraseñas, códigos de verificación, números de tarjeta, CVV, credenciales de aplicación, claves API y tokens OAuth **no forman parte de este informe**. Los secretos locales pertenecen a `.local/secrets/`, fuera de Git. La configuración de OAuth/retorno y de webhook está descrita en [mercado-pago-chile.md](mercado-pago-chile.md). El navegador recibe solo la Public Key necesaria para MercadoPago.js; el Access Token del vendedor permanece en el backend.

## Recorrido del pago y puntos de control

```mermaid
flowchart TD
  A[Anfitrión inicia OAuth en EspaciGo] --> B[Mercado Pago autoriza al anfitrión vendedor]
  B --> C[Callback HTTPS público del BFF Next]
  C --> D[Go valida state y cambia código OAuth por token de prueba]
  D --> E[Token OAuth cifrado en PostgreSQL]
  E --> F[Arrendatario autenticado elige horario y recibe cotización CLP]
  F --> G[/reservas/checkout/tarjeta/{quoteID}]
  G --> H[MercadoPago.js v2 tokeniza la tarjeta con la Public Key configurada]
  H --> I[Next BFF reenvía POST checkout-api a Go]
  I --> J[Go valida cotización y obtiene token OAuth del anfitrión]
  J --> K[Go reclama operación y envía POST /v1/payments]
  K --> L{Respuesta Mercado Pago}
  L -->|Último intento: HTTP 400, causa 2198, sin payment_id| M[UI provider_error; fila pending hasta vencimiento; luego expired]
  L -->|2xx con pago validado| N[Go valida modo, monto, moneda, vendedor y referencias]
  N --> O[PostgreSQL aplica resultado verificado]
  P[Webhook HTTPS de prueba] --> Q[Next BFF valida y reenvía firma y notificación]
  Q --> R[Go valida firma HMAC y guarda recibo durable]
  R --> S[Worker consulta GET /v1/payments/{payment_id}]
  S --> N
```

El webhook es una ruta aparte de la creación del pago. Solo puede conciliar un evento real si Mercado Pago entrega un ID de pago que el backend pueda consultar. La respuesta `200` de la prueba manual de Webhooks con `data.id=123456` prueba que el callback firmado fue recibido y guardado; **no** demuestra que se haya creado, aprobado o liquidado un pago.

## Rutas y responsabilidades verificadas en el código

| Etapa | Ruta o endpoint | Responsable y comportamiento |
| --- | --- | --- |
| Consentimiento del anfitrión | `POST /api/v1/host/mercadopago/oauth/start` | Go crea `state` y PKCE y devuelve el inicio OAuth. URL por defecto para Chile. `internal/marketplaceoauth/http.go:33-58`, `internal/marketplaceoauth/service.go:142-170`. |
| Retorno OAuth | `https://prueba-mercadopago.testingpage.store/api/backend/host/mercadopago/oauth/callback` | Next BFF reenvía el callback al backend Go en loopback. OAuth intercambia el código, comprueba modo de prueba y cifra el token del anfitrión. El callback no es el webhook de pagos. |
| Cotización | `POST /api/v1/rental-quotes` | Go determina disponibilidad, importe CLP y expiración. El navegador no fija el monto final. `internal/rentalflow/transport/http/handler.go:94-101`. |
| Formulario | `/reservas/checkout/tarjeta/{quoteID}` | `frontend/src/features/rental-checkout/ui/checkout-api-card-form.tsx` carga `https://sdk.mercadopago.com/js/v2`, crea CardForm y envía token de tarjeta, método, emisor y datos del pagador. No debe enviar PAN/CVV al servidor. |
| Crear pago | `POST /api/v1/rental-quotes/{quoteID}/checkout-api` | Endpoint Go autenticado con el arrendatario. Requiere `Idempotency-Key`. El backend vuelve a cargar y valida la cotización. `internal/rentalflow/transport/http/handler.go:100-102`, `checkout_api.go`. |
| Llamada del proveedor | `POST https://api.mercadopago.com/v1/payments` | `internal/checkoutapi/provider.go:130-205` envía token de tarjeta, `payment_method_id`, `issuer_id` disponible, `payer`, monto, `external_reference` e `X-Idempotency-Key`; Authorization usa el token OAuth del vendedor. |
| Estado consultado por la UI | `GET /api/v1/rental-quotes/{quoteID}/checkout-api` | Lee el estado local. No realiza búsqueda en Mercado Pago ni consulta por `external_reference`. `internal/checkoutapi/service.go:95-142`. |
| Webhook público | `https://prueba-mercadopago.testingpage.store/api/v1/webhooks/mercadopago` | Next filtra y reenvía la firma y la notificación a Go. Go valida HMAC, guarda inbox y devuelve respuesta. `frontend/src/app/api/v1/webhooks/mercadopago/route.ts`, `internal/checkoutpro/webhook.go:124-159`. |
| Consulta de pago notificado | `GET /v1/payments/{payment_id}` | Worker consulta el pago con el token del vendedor y valida modo, importe, moneda, referencia y receptor antes de actualizar la reserva. `internal/checkoutpro/reconciliation.go`, `provider.go`. |

La ruta alternativa `/api/v1/rental-quotes/{quoteID}/checkout-wallet` y el botón de inicio de sesión de Mercado Pago pertenecen al flujo **Checkout Pro con Preferencias** (`POST /checkout/preferences`). No son el formulario Checkout API. Ambos caminos pueden coexistir en código y no se deben confundir al identificar cuál se probó.

## Registro del último intento

| Campo | Valor observado |
| --- | --- |
| Fecha/hora local | 20-11-2026 20:37:15, zona `America/Santiago` |
| Modo | Prueba (`test_only_config=true`) |
| Cotización | `38a4edef-6291-48db-b7ed-e0a24911b399` |
| Monto | 24.000 CLP |
| Operación interna | `713e10df-8244-4489-87c9-5a8ed4c9d788` |
| Endpoint del proveedor | `POST /v1/payments` |
| Respuesta | HTTP 400, `bad_request`, causa `2198`, categoría local `unclassified` |
| ID de solicitud del proveedor | `bb60e3dd-0e6c-46bf-9a71-c6a39aa81ecc` |
| `payment_id` en respuesta/registro local | Vacío / no recibido |
| Comisión EspaciGo enviada | No. `application_fee_set=false` |
| Contexto de vendedor y clave | El log marcó `oauth_token_available` y `seller_context_verified`. En el intento se verificó la clave pública configurada para ese vendedor en el chequeo local. Esto no demuestra que Mercado Pago haya aceptado el pago ni confirma abono. |
| Resultado HTTP inmediato | `provider_error`; no se aprobó |
| Estado persistido después | `pending` hasta vencer; una lectura posterior mostró `expired`. Sin ID de pago. |
| Certeza sobre registro remoto | Desconocida. No se encontró un ID para consultar ni se probó búsqueda remota por `external_reference`. |

El log muestra que el backend tenía token de vendedor, token de tarjeta, método, emisor, correo e identificación presentes. El rechazo anterior se mantuvo después de probar cambios de datos y desactivar la comisión, según los registros. Esto elimina algunas hipótesis simples, pero no identifica por sí solo el dato inválido. El proveedor devolvió una causa opaca y la aplicación no conserva el cuerpo sensible completo de error.

## Evaluación de estados y del webhook

| Evidencia | Qué demuestra | Qué no demuestra |
| --- | --- | --- |
| OAuth y chequeo de clave del anfitrión | Log del intento registró token OAuth disponible y `seller_context_verified`; el modo local de tokenización por Public Key es opt-in y estaba activo en ese intento. | Que Mercado Pago acepte la combinación del token de tarjeta y OAuth, o que se haya recibido dinero. |
| Mercado Pago devuelve 400 `2198` y un request ID | La solicitud alcanzó la API de Mercado Pago y fue rechazada. | La causa de negocio exacta ni que no exista una operación remota en estados intermedios. |
| No llegó `payment_id` al backend | EspaciGo no tiene ID para consultar o conciliar ese intento. | Que Mercado Pago no haya creado ningún registro interno. Debe confirmarse con soporte o mecanismo de búsqueda disponible en la cuenta. |
| Panel de Webhooks responde `200 - OK` a `data.id=123456` | El endpoint público pudo recibir una notificación sintética, validar firma y guardar el recibo. | Un pago real/sandbox, una notificación originada por un pago o un estado `approved`. El log posterior muestra que la búsqueda del ID sintético falló y se agotaron reintentos. |
| Contador de notificaciones del panel igual a cero | El panel no muestra notificaciones registradas en esa vista/entorno. | Que el callback manual no haya alcanzado el receptor local; son registros de sistemas distintos. |

## Hallazgo prioritario de recuperación

En `internal/checkoutapi/service.go`, el backend persiste y reclama la operación antes de llamar al proveedor. `ClaimPayment` es de un solo uso. Ante un error HTTP 4xx, el endpoint devuelve resultado `provider_error`, pero no guarda ese estado ni libera el reclamo. Luego `GET .../checkout-api` reconstruye el estado desde la fila local y puede devolver `pending`. El siguiente POST idempotente no vuelve a llamar al proveedor.

Esto deja tres riesgos concretos:

1. Pantalla de fallo y estado consultado pendiente se contradicen.
2. Una respuesta 400 conocida y definitiva queda igual que un envío con resultado desconocido.
3. Si se pierde una respuesta exitosa y no se conoce el `payment_id`, no existe recuperación automática desde la referencia externa local.

No se recomienda volver a pulsar pagar repetidamente ni borrar la cotización para eludir el reclamo. La corrección debe separar rechazo definitivo de timeout ambiguo, persistir ambos estados, y definir cómo conciliar o detener un intento ambiguo antes de permitir un nuevo cobro.

## Checklist de seguridad transversal

| Control | Estado | Evidencia y límite |
| --- | --- | --- |
| Token de API fuera del código | Cumple en el diseño revisado | Token del vendedor se obtiene desde OAuth cifrado en backend; la guía guarda secretos en `.local/secrets/`. No se imprimieron valores durante esta auditoría. |
| `.env` y secretos fuera de Git | Parcialmente verificado | `AGENTS.md` y `docs/operations/mercado-pago-chile.md` exigen mantener `.local/` y llaves fuera de Git. Hay cambios locales pendientes; no se hizo auditoría completa de todos los archivos históricos ni se inspeccionó el contenido de secretos. |
| Firma de webhook HMAC-SHA256 | Cumple en código | `internal/checkoutpro/webhook.go` valida `x-signature`; log de la notificación de prueba registró `signature_valid=true`. |
| HTTPS para callbacks y retornos | Cumple en configuración documentada | La URI pública registrada usa HTTPS y Cloudflare Tunnel. No implica que todos los entornos futuros mantengan esa propiedad. |
| Estado verificado en servidor | Cumple para notificación con ID | El worker consulta `GET /v1/payments/{id}` antes de actualizar. El GET de estado de Checkout API en la UI es solo local y no concilia por sí mismo. |
| Idempotencia al crear pago | Parcial | El endpoint requiere clave y Go envía `X-Idempotency-Key`. El reclamo de un solo uso queda atascado tras error; falta estrategia segura de recuperación. |
| Referencia externa | Cumple | Se envía UUID interno de operación como `external_reference`. No existe búsqueda remota implementada por ese valor para recuperación. |
| Usuarios y claves sandbox separados | Parcial | El entorno local restringe Payments API y Public Key del vendedor al modo de prueba. Debe confirmarse el SHA desplegado y separar toda configuración de producción antes de usarla. |
| Uso de `sandbox_init_point` | Cumple en flujo Checkout API revisado | No aplica a la llamada directa `/v1/payments`; Checkout Pro tiene su propio `init_point`. |

## Checklist oficial de calidad recibido por MCP

La herramienta `quality_checklist` respondió para la aplicación Checkout API en esta sesión. Sus recomendaciones mencionan Preferences en varios campos; se registra esa discrepancia y no se presenta como score de homologación. Estados basados en inspección local del código actual.

### Requisitos de implementación

| Campo | Estado | Evidencia |
| --- | --- | --- |
| `email` del pagador | Implementado | `internal/checkoutapi/provider.go` define y envía `payer.email`; la tarjeta entrega el correo en `checkout-api-card-form.tsx`. |
| Nombre | N/A | El criterio MCP lo devuelve como `N/A`; el request de Checkout API no envía `payer.first_name`. |
| Apellido | No implementado | `Payer` solo incluye correo e identificación. |
| Categoría, descripción, ID, cantidad, título y precio unitario del ítem | N/A para `/v1/payments` | El checklist MCP usa campos del flujo Preferences. La ruta activa crea un pago directo y envía `transaction_amount`; no debe copiarse el objeto `items` de Checkout Pro sin validar la especificación de Payments API. |
| `device_id` | Implementado por MercadoPago.js v2 CardForm | El flujo usa iframe/CardForm de MercadoPago.js v2, que gestiona la información del dispositivo. `checkout-api-card-form.tsx:20,1014-1045`. |
| `issuer_id` | Parcial | El formulario lo envía cuando MercadoPago.js lo entrega; no es obligatorio en todos los métodos. `checkout-api-card-form.tsx:861,889`, `provider.go:48`. |
| Webhook / `notification_url` | Implementado con caveat de producto | El Webhook de aplicación recibe, valida HMAC y concilia pagos. La checklist recomienda `notification_url` en una Preference, pero esta ruta usa Payments API. `internal/checkoutpro/webhook.go:124-159`. |
| `external_reference` | Implementado | UUID estable de operación enviado en el payload y también usado como clave de idempotencia. |
| SDK de backend | No implementado como SDK | Go usa HTTP directo con el endpoint REST oficial. La checklist lo recomienda; no explica el 2198 por sí solo. |
| SDK de frontend | Implementado | `checkout-api-card-form.tsx` carga MercadoPago.js v2 y usa `mp.cardForm`. |
| `statement_descriptor` | No implementado | No aparece en el payload inspeccionado. Confirmar soporte para Payments API Chile antes de añadirlo. |
| Certificado SSL | Parcial | URL de prueba opera por HTTPS Cloudflare. No se hizo una verificación independiente de vigencia/chain durante esta auditoría. |
| TLS 1.2 o superior | No verificado | La implementación no fija el protocolo; falta una prueba externa de TLS del host público. |
| Campos de tarjeta seguros | Implementado en código, requiere verificación visual | MercadoPago.js v2 CardForm usa campos de tarjeta seguros en iframe; el PAN/CVV no debe atravesar Go. Hubo reportes previos de campos que no cargaban, así que la ejecución de navegador sigue siendo un criterio de aceptación pendiente. |

### Buenas prácticas

| Práctica | Estado | Evidencia |
| --- | --- | --- |
| Dirección del pagador | No implementado | `Payer` no la modela; el checklist la presenta como recomendación opcional. |
| Identificación del pagador | N/A en el checklist, implementada en el request | El MCP marca el criterio como N/A, pero Checkout API sí envía tipo y número de identificación (`provider.go:38-40`; formulario `checkout-api-card-form.tsx:893-901`). |
| Teléfono del pagador | N/A / no se recopila | El producto actual no solicita teléfono al comprador. |
| Identificación del pagador para MercadoLibre | N/A | El criterio de identificación MLM del MCP no aplica al marketplace de Chile revisado. |
| Consultar pago luego de webhook | Implementado | Reconciliación consulta `GET /v1/payments/{id}` con el token del vendedor. No resuelve la operación actual sin ID. |
| API de contracargos | No implementado | No se identificó flujo de contracargos en los archivos revisados. |
| Cancelación | No implementado | No se identificó operación de cancelación del pago en Checkout API. |
| Devoluciones | No implementado | No se identificó endpoint/flujo de reembolso. |
| Reporte de liquidaciones | No implementado | No se identificó reporte de settlement. |
| Reporte de transacciones | No implementado | No se identificó reporte de release. |
| `binary_mode` configurable | Parcial | El request fija `binary_mode=false` y no lo expone como opción. `internal/checkoutapi/provider.go:145-159`. |
| Captura diferida (`capture=false`) | No implementado / no confirmado | No está en el payload actual de `/v1/payments`; no es requisito declarado del flujo de arriendo actual. |
| Logo de Mercado Pago | No implementado en el formulario revisado | Se muestran textos e iconos de seguridad, pero no se encontró el logo oficial en `checkout-method-choice.tsx` ni en el formulario. |
| Mensajes de resultado | Parcial | UI distingue fallo/estado, pero la causa 2198 queda genérica y la UI de error contradice el GET local pendiente. |
| `payer.id` / `customer_id` | N/A para esta compra con token nuevo | No se usa tarjeta guardada asociada a un Customer. |

## Estado de lo probado

| Comprobación | Estado al corte |
| --- | --- |
| OAuth vendedor sandbox | Conectado en una de las pruebas manuales; no se vuelve a autenticar ni se cambia durante esta auditoría. |
| CardForm Checkout API | Carga/interacción reportada en varias pruebas, pero antes hubo pantalla sin campos; requiere reproducir y registrar consola/red del navegador. |
| Crear pago Checkout API | Rechazado por Mercado Pago con 400 / 2198 en los últimos intentos conocidos. |
| Pago aprobado / recibido por anfitrión | No observado. |
| `payment_id` real de prueba | No disponible en evidencia local. |
| Webhook sintético | Firma verificada, recibo guardado, `200`; en la lectura DB posterior al simulador de las 20:59 la fila estaba en `retry` con `provider_lookup_failed`, sin `processed_at`. El número de intentos puede variar al avanzar los reintentos. |
| Prueba E2E aprobada con sandbox | No completada. |
| Calidad oficial por `payment_id` | No ejecutada, porque no existe ID. |
| Homologación/producción | Pendiente; no procede hasta aprobación de sandbox y revisión del flujo. |

## Plan recomendado para continuar

1. **Pausar reintentos de cobro sobre esta cotización.** Guardar el request ID de Mercado Pago y la hora del intento para que soporte busque el rechazo en su trazabilidad. No volver a enviar el mismo token de tarjeta ni asumir que la operación remota no existe.
2. **Corregir persistencia de estados antes de otro intento.** Guardar 4xx definitivo como rechazo/error terminal con su código sanitizado; distinguirlo de timeout/resultado ambiguo. El GET debe devolver un estado coherente con la pantalla.
3. **Diseñar reconciliación segura sin `payment_id`.** Evaluar con documentación/soporte oficial cómo consultar un pago por `external_reference` y token del vendedor. Si no hay búsqueda segura disponible, bloquear reintentos ambiguos y permitir conciliación operativa manual. No marcar aprobado desde redirect ni desde datos del cliente.
4. **Aislar el motivo de 2198 con una matriz mínima, una variable por intento.** Confirmar primero por soporte si request ID existe y qué regla negó. Después comparar token de tarjeta generado con Public Key del vendedor frente a token OAuth, tipo y valor de identificación aceptado en Chile, cuenta Marketplace/vendedor elegible, email del comprador sandbox y campos mínimos del payload. Registrar para cada intento la hora UTC, request ID, causa sanitizada y configuración booleana. No volver a cambiar varias variables a la vez.
5. **Auditar tokenización y dispositivo en navegador.** Con DevTools, confirmar que CardForm se montó, que MercadoPago.js v2 cargó sin CSP/adyacencias bloqueadas, que emite token y `issuer_id`, y que los datos PAN/CVV no aparecen en requests a EspaciGo. No guardar payloads ni capturas con datos privados.
6. **Después de la corrección, usar una cotización nueva de prueba y una sola transacción controlada.** Registrar ID de pago, estado y detalle; comprobar webhook, consulta server-side, reserva y conciliación entre saldo de comprador y vendedor. Ejecutar `quality_evaluation` únicamente con ese ID de prueba.
7. **Mantener los cambios en la rama de pagos y destino `desarrollo-local`.** No desplegar ni promover a `main` hasta que el flujo de sandbox esté aprobado y el usuario autorice expresamente la promoción.

### Información útil para soporte de Mercado Pago

Compartir por el canal privado de soporte, no en un repositorio público:

- ID de la aplicación Checkout API y país Chile.
- Producto API Payments / Marketplace, modo de prueba.
- Request ID `bb60e3dd-0e6c-46bf-9a71-c6a39aa81ecc`.
- Fecha y hora local del rechazo: 20-11-2026 20:37:15, zona `America/Santiago`.
- HTTP 400, `bad_request`, causa 2198; confirmar qué validación representa esta causa en Chile.
- Preguntar si existe pago asociado a la solicitud aunque no se devolviera `payment_id`, y cómo encontrarlo de forma segura.
- Preguntar qué condiciones deben cumplirse para el par vendedor OAuth + tokenización de tarjeta en Marketplace Checkout API y si hay restricciones para cuentas seller/buyer de prueba chilenas.

No incluir credenciales, token de tarjeta, PAN/CVV, contraseña ni código de verificación.

## Referencias

- [Documentación de Mercado Pago Chile para Payments API](https://www.mercadopago.cl/developers/es/docs/checkout-api/landing)
- [Notificaciones Webhooks de Mercado Pago](https://www.mercadopago.cl/developers/es/docs/your-integrations/notifications/webhooks)
- [OAuth de Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/security/oauth/creation)
- [Prerrequisitos de Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/prerequisites)
- Skill de auditoría local `mercadopago:mp-review` v1.0.0.
- [Guía de configuración Chile](mercado-pago-chile.md)

## Informe de implementación

### Verificado

- [x] La ruta de Checkout API usa `POST /v1/payments` y autentica la llamada con token OAuth del vendedor.
- [x] El browser obtiene un token con MercadoPago.js v2; el backend no debe recibir PAN/CVV.
- [x] El request de pago incluye idempotencia, referencia externa y monto derivado de cotización.
- [x] La recepción Webhook valida firma y persiste recibos.
- [x] El callback sintético `123456` fue recibido con firma válida, y la conciliación no encontró un pago real con ese ID.
- [x] El último rechazo conocido es HTTP 400 / código 2198, sin `payment_id`, sin aprobación local.

### Requiere atención

- [ ] Investigar con Mercado Pago el significado del 2198 para ese request ID.
- [ ] Persistir errores definitivos y diferenciar los resultados ambiguos.
- [ ] Diseñar conciliación segura de intentos sin payment ID o bloquear su reintento.
- [ ] Confirmar compatibilidad de Public Key de vendedor con token OAuth del vendedor para Payments API Chile.
- [ ] Verificar carga CardForm, `device_id`, `issuer_id` y los campos mínimos en navegador.
- [ ] Revisar los criterios Preferences devueltos por la checklist oficial antes de tratarlos como requisitos de Payments API.
- [ ] Repetir E2E en `desarrollo-local`, usando una nueva cotización de prueba, y obtener un payment ID aprobado y su webhook.

### Bloqueos antes de producción

- [ ] No hay pago de sandbox aprobado ni evaluación por payment ID.
- [ ] El 2198 sigue sin causa confirmada.
- [ ] Ante un fallo, la operación puede quedar `pending` hasta vencer, sin mecanismo completo de recuperación.
- [ ] No iniciar producción ni promover a `main` hasta resolver esos puntos y tener autorización expresa del usuario.

### Siguiente paso

1. Enviar el request ID y hora a soporte de Mercado Pago para obtener significado específico del 2198 y confirmar si existe operación asociada.
2. Corregir la máquina de estados/recuperación local antes de autorizar otro intento.
3. Repetir una transacción controlada y completar la conciliación solo cuando se obtenga un payment ID consultable.

**Quality evaluation:** omitida — no se recibió ningún `payment_id`.

**Homologación:** no solicitada ni enviada.

**Veredicto:** **Bloqueado para completar pruebas; no listo para producción.**
