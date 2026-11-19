# Mercado Pago Chile en EspaciGo

El módulo usará Checkout Pro con Split de Pagos 1:1. Cada reserva vincula a un anfitrión vendedor y a EspaciGo como marketplace. El arrendatario paga en la página de Mercado Pago y el proveedor distribuye el pago. Mercado Pago descuenta su comisión de los fondos del vendedor antes de descontar la comisión de EspaciGo. Su guía de Split 1:1 en Chile documenta el saldo en cuenta y excluye las transferencias desde instituciones financieras externas. La guía general de Checkout Pro describe otros medios de pago. Confirma los medios disponibles para tu aplicación en el flujo 1:1 antes de ofrecerlos. [Integración oficial de Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)

La rama parte de `origin/desarrollo-local`. Integra los cambios del módulo en `desarrollo-local` y prueba allí. La promoción posterior a `main` requiere autorización explícita.

## 1. Identificar tu aplicación en Chile

1. Inicia sesión con tu cuenta de desarrollador en [Integraciones de Mercado Pago Chile](https://www.mercadopago.cl/developers/panel/app).
2. Abre `pago-espacigo` y verifica que corresponda a Chile (`MLC`) y al modelo **Marketplace**. La primera implementación usa **Checkout Pro con API de Preferences** y envía la comisión en `marketplace_fee`. [Checkout Pro con Preferencias](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/overview)
3. En **Credenciales → Prueba**, ubica el **Access Token de prueba** y la **Public Key de prueba**. Usa el selector de entorno para identificarlos. Un token de prueba de Checkout Pro también puede empezar por `APP_USR-`. El **Client ID** y el **Client Secret** del integrador se consultan en la sección de credenciales de la aplicación y se usan para el intercambio OAuth. Al vincular un vendedor de prueba solicita `test_token: true` y comprueba `live_mode=false`; las preferencias usan el endpoint normal y redirigen por `init_point`. [Credenciales](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/your-integrations/credentials) · [OAuth](https://www.mercadopago.cl/developers/es/docs/security/oauth/creation)
4. En **Cuentas de prueba**, prepara un vendedor y un comprador distintos, ambos de Chile. Usa una cuenta integradora de prueba si la configuración de marketplace lo requiere. Usa saldo ficticio para verificar el reparto. La guía de Split 1:1 pide KYC nivel 6 para la cuenta vendedora real. [Cuentas de prueba](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/your-integrations/test/accounts) · [Requisitos de Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/prerequisites)

## 2. Guardar credenciales en el checkout local

Desde la raíz del checkout donde correrá el backend, crea un archivo privado bajo `.local/secrets/`. Git ignora esa carpeta. El comando abre un editor para que pegues los valores ahí y queden fuera del historial de la terminal.

```bash
mkdir -p .local/secrets
(
  umask 077
  touch .local/secrets/mercadopago-test.env
  chmod 600 .local/secrets/mercadopago-test.env
  "${EDITOR:-nano}" .local/secrets/mercadopago-test.env
)
git check-ignore .local/secrets/mercadopago-test.env
```

Guarda el secreto de firma del webhook en un archivo separado, con permisos privados; la configuración solo conserva la ruta:

```bash
(
  umask 077
  touch .local/secrets/mercadopago-webhook-secret
  chmod 600 .local/secrets/mercadopago-webhook-secret
  "${EDITOR:-nano}" .local/secrets/mercadopago-webhook-secret
)
```

Usa esta plantilla para el adaptador Go. Copia los valores directamente desde el panel. Mantén los secretos fuera del chat, los logs, Git y las variables `NEXT_PUBLIC_*`.

```dotenv
MERCADOPAGO_TEST_ACCESS_TOKEN=
MERCADOPAGO_CLIENT_ID=
MERCADOPAGO_CLIENT_SECRET=
MERCADOPAGO_OAUTH_REDIRECT_URI=https://tu-dominio/api/backend/host/mercadopago/oauth/callback
MERCADOPAGO_TOKEN_ENCRYPTION_KEY=
MERCADOPAGO_TEST_MODE=true
MERCADOPAGO_WEBHOOK_SECRET_FILE=.local/secrets/mercadopago-webhook-secret
```

El archivo anterior contiene solo claves que `scripts/local-dev.sh` acepta. Antes de iniciar Go con un túnel de prueba, exporta los parámetros no secretos en la misma terminal:

```bash
export FRONTEND_PUBLIC_ORIGIN="https://tu-dominio"
export CORS_ALLOWED_ORIGINS="$FRONTEND_PUBLIC_ORIGIN"
export MERCADOPAGO_RETURN_ORIGIN="$FRONTEND_PUBLIC_ORIGIN"
export MERCADOPAGO_CHECKOUT_ENABLED=0
export MERCADOPAGO_WEBHOOK_SECRET_FILE="$PWD/.local/secrets/mercadopago-webhook-secret"
```

El valor predeterminado es `MERCADOPAGO_CHECKOUT_ENABLED=0`. Cámbialo a `1` solo después de aplicar y probar las migraciones, vincular un vendedor de prueba y montar el receiver/worker de pagos. Usa el túnel para navegar la web completa durante OAuth; Go y `API_BACKEND_BASE_URL` siguen en loopback.

| Valor | Uso |
| --- | --- |
| `MERCADOPAGO_TEST_ACCESS_TOKEN` | Credencial del panel para comprobar acceso API manualmente; no se usa para vincular vendedores ni para cobrar. |
| `MERCADOPAGO_CLIENT_ID` | Identifica la aplicación de EspaciGo en el consentimiento OAuth. |
| `MERCADOPAGO_CLIENT_SECRET` | Permite a Go intercambiar y renovar credenciales OAuth en el servidor. |
| `MERCADOPAGO_OAUTH_REDIRECT_URI` | URL pública exacta registrada en `pago-espacigo`; el BFF reenvía el callback a Go. |
| `MERCADOPAGO_TOKEN_ENCRYPTION_KEY` | Clave AES-256 codificada en Base64 para cifrar credenciales OAuth. Guárdala solo en el archivo privado local o en el gestor de secretos productivo. |
| `MERCADOPAGO_TEST_MODE` | `true` solicita credenciales OAuth de prueba con `test_token=true` y verifica que el proveedor las marque como no productivas. Cambia a `false` solo al configurar deliberadamente producción. |
| `MERCADOPAGO_WEBHOOK_SECRET_FILE` | Ruta privada al archivo con la clave de firma de Webhooks. Mantén el archivo con permisos `0600`; Go rechaza enlaces simbólicos, archivos amplios o permisos de grupo/otros. |
| `MERCADOPAGO_CHECKOUT_ENABLED` | Parámetro de entorno no secreto; no se agrega al archivo privado. Mantén `0` hasta que el flujo de prueba completo esté listo. `1` requiere OAuth más una URL de retorno HTTPS. |
| `MERCADOPAGO_RETURN_ORIGIN` | Parámetro no secreto; no se agrega al archivo privado. Usa el origin HTTPS que Mercado Pago admite para `back_urls`, sin path, igual a `FRONTEND_PUBLIC_ORIGIN` durante la prueba con túnel. |
| `FRONTEND_PUBLIC_ORIGIN` | Origin público donde abres Next durante OAuth de prueba. Usa el mismo HTTPS del túnel para la página, callback y retorno; el callback BFF valida que el backend redirija al mismo origin. `API_BACKEND_BASE_URL` se mantiene en loopback hacia Go. |
| Public Key de prueba | Clave publicable. Hace falta si la interfaz inicializa el SDK de Mercado Pago. Una redirección directa usa la URL que devuelve la preferencia. |

El SDK oficial de Go está agregado al módulo como `github.com/mercadopago/sdk-go v1.14.0`. La importación inicial que muestra Mercado Pago (`github.com/mercadopago/sdk-go/pkg/config`) configura el cliente del servidor con un Access Token. Para Split 1:1, la preferencia real de cada anfitrión debe autenticarse con el token OAuth de ese vendedor.

El callback OAuth debe usar la URL HTTPS pública exacta del BFF (`/api/backend/host/mercadopago/oauth/callback`) registrada en la aplicación. Para desarrollo local, abre Next a través del mismo túnel HTTPS y configura con ese origin `MERCADOPAGO_OAUTH_REDIRECT_URI`, `FRONTEND_PUBLIC_ORIGIN`, `CORS_ALLOWED_ORIGINS` y `MERCADOPAGO_RETURN_ORIGIN`. Mantén `API_BACKEND_BASE_URL` dirigido al Go local por loopback. No inicies OAuth desde `http://127.0.0.1:3000` y regreses por otro host: la cookie de sesión y el redirect de retorno pertenecen al origin público del navegador. Cada intento guarda `state` de un solo uso, PKCE y expiración en PostgreSQL; el backend persiste los tokens OAuth cifrados con AES-GCM, ID de vendedor, scopes y vencimiento. Los tokens nunca se envían al navegador. Si OAuth no está configurado, el estado de conexión muestra `authorization_available=false` y no puede iniciar consentimiento.

## 3. Confirmar acceso por terminal o por MCP

Puedes confirmar acceso a la API sin crear pagos. Desde la raíz del checkout, este comando usa el token de prueba y muestra solo el código HTTP. Un `200` demuestra que la API respondió a la consulta de medios de pago. La disponibilidad del reparto se comprueba después con un vendedor vinculado.

```bash
(
  set -euo pipefail
  source .local/secrets/mercadopago-test.env
  : "${MERCADOPAGO_TEST_ACCESS_TOKEN:?Falta el Access Token de prueba}"
  printf 'Authorization: Bearer %s\n' "$MERCADOPAGO_TEST_ACCESS_TOKEN" |
    curl --silent --show-error --fail --max-time 15 \
      --output /dev/null --write-out 'HTTP %{http_code}\n' \
      --header @- https://api.mercadopago.com/v1/payment_methods
)
```

Mercado Pago documenta su plugin para Codex y el comando `/mp-connect` para autorizar al agente mediante el navegador. En esta sesión el conector MCP pudo listar una aplicación de prueba. Compara el nombre y el ID que devuelve con los de tu panel para confirmar que es la aplicación de EspaciGo. Si falta, autoriza la cuenta correcta en Chile. Usa el panel para copiar los secretos directamente al archivo local. El acceso MCP sirve para tareas de desarrollo y administración. El backend llama a la API de Mercado Pago desde Go mediante HTTP o el SDK oficial. Para una consulta de terminal basta `curl`. [Plugin y autorización para Codex](https://www.mercadopago.cl/developers/es/docs/checkout-pro-orders/create-application?scope=prod) · [Referencia de Preferencias](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/overview)

## 4. Implementar y probar el flujo del módulo

1. **Vincular al anfitrión.** Desde EspaciGo, el anfitrión inicia el consentimiento OAuth. Registra en la aplicación una URL de retorno de prueba y usa exactamente la misma URI en la autorización y el intercambio. Cuando existan las rutas, usa una dirección HTTPS de prueba para la integración local. Go verifica un `state` de un solo uso ligado a la sesión del anfitrión, usa PKCE y cambia el código en `POST /oauth/token`. Guarda cifrados el token y su renovación, junto con el ID del vendedor y el vencimiento. Solicita y verifica el modo de prueba antes de continuar. [Flujo OAuth](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/security/oauth/creation)
2. **Pagar la reserva.** Go toma monto, moneda `CLP` y comisión desde la reserva validada. Crea la preferencia en `POST /checkout/preferences` autenticándose con el token OAuth del anfitrión e incluye `marketplace_fee` como monto, `external_reference` como identificador interno y `back_urls` para la vuelta del comprador. Guarda la relación entre reserva y preferencia y reutilízala ante reintentos de inicio. Next.js redirige a la URL de Checkout Pro devuelta para el entorno de prueba. [Crear una preferencia](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/overview) · [Comisión de marketplace](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)
3. **Confirmar el pago.** Configura **Webhooks → Prueba → Pagos** con una URL HTTPS pública que apunte a `/api/v1/webhooks/mercadopago`. El backend valida `x-signature`, guarda un recibo durable y responde antes de que el worker consulte `GET /v1/payments/{id}` con la credencial OAuth del vendedor asociado. Verifica entorno, vendedor, moneda, monto, estado y referencia antes de aplicar de forma idempotente el cambio en la reserva. Las URLs de retorno sirven para mostrar el estado al comprador, incluida la espera mientras el backend confirma el pago. [Webhooks de Mercado Pago](https://www.mercadopago.cl/developers/es/docs/split-payments/additional-content/your-integrations/notifications/webhooks)
4. **Verificar en `desarrollo-local`.** La operación comercial del hito actual parte de una cotización persistida (`POST /rental-quotes/{quoteID}/checkout`); no reutiliza los IDs de `booking-trial`. Prueba aprobación, rechazo, timeout/recuperación de Preference, retención exclusiva del intervalo, reintentos idempotentes y pago tardío. El redirect nunca confirma el pago; el estado debe proceder de conciliación del backend. Comprueba en las cuentas de prueba el importe del comprador, la comisión de EspaciGo y el saldo neto del anfitrión. Ejecuta `./scripts/local-dev.sh test` una vez integrado el receptor de notificaciones.

### Guía manual del Checkout Pro comercial

El endpoint `POST /api/v1/webhooks/mercadopago`, su BFF Next, inbox durable y worker están implementados. El BFF público preserva query/body y reenvía los headers de firma necesarios; el receiver Go firma el `data.id` de query y valida su igualdad con el body si el body lo incluye. Las pruebas focalizadas del BFF y del receiver pasan. La prueba integrada de fixture con webhook firmado, proveedor fake, reserva V113 y outbox pasó antes del último ajuste de URL/query; ese E2E no se ha repetido con el nuevo request fixture. La prueba manual de OAuth/webhook/Checkout Pro desde túnel con cuentas de prueba sigue pendiente. Para ese recorrido, aplica V110–V113, configura OAuth y Webhooks → Prueba → Pagos a nivel de aplicación, inicia el API con `MERCADOPAGO_CHECKOUT_ENABLED=1` y registra la URL HTTPS del túnel más `/api/v1/webhooks/mercadopago`. La Preference no envía `notification_url` individual; confirma en la prueba que la notificación configurada a nivel de aplicación llegue para una operación Marketplace 1:1. No uses cuentas reales, credenciales live ni una bandeja SMTP externa. Las pruebas Playwright de inicio/retorno usan respuestas sintéticas; no crean preferencias ni cobran. Consulta [readiness de prueba HTTPS](mercado-pago-test-readiness.md) para el único túnel a Next, guard local y gates del posible runtime hospedado.

1. Levanta la web por un túnel HTTPS dirigido a Next local y configura el API con `FRONTEND_PUBLIC_ORIGIN`, `CORS_ALLOWED_ORIGINS`, `MERCADOPAGO_OAUTH_REDIRECT_URI` y `MERCADOPAGO_RETURN_ORIGIN` coherentes con ese mismo origin público. Mantén `API_BACKEND_BASE_URL` y Go en loopback; configura `LOCAL_SMTP_ADDR=127.0.0.1:1025` para Mailpit. El webhook de prueba apunta al túnel más `/api/v1/webhooks/mercadopago`.
   Para una sesión manual del flujo comercial, exporta `COMMERCIAL_RENTAL_FLOW_ENABLED=1` y `MERCADOPAGO_CHECKOUT_ENABLED=1` antes de `./scripts/local-dev.sh api`; el runner apaga automáticamente `LOCAL_BOOKING_TRIAL` en ese modo. Déjalos en `0` para la demo habitual. No inicies el modo comercial hasta tener la fixture V113, OAuth de prueba y archivo de firma configurados.
2. Con dos cuentas de prueba chilenas, vincula primero la cuenta anfitriona desde **Anfitrión → Mercado Pago** (`/anfitrion/mercado-pago`). Confirma que la UI muestra la conexión y que el backend informa disponibilidad de autorización. Si no se configuraron OAuth y la clave de cifrado, detén el recorrido aquí.
3. Con una cuenta arrendataria de prueba, abre una publicación comercial, elige un horario y calcula la cotización oficial. Verifica que el monto y vencimiento vienen de la respuesta de Go. Pulsa **Pagar con Mercado Pago** y comprueba que el navegador navega al `init_point` HTTPS del entorno de prueba devuelto por el backend.
4. Completa el checkout únicamente con el comprador de prueba y saldo ficticio. Al regresar a `/reservas/checkout/{quoteID}`, comprueba que el estado se carga desde `GET /rental-quotes/{quoteID}/checkout`. Cambiar o añadir `status=approved` o `collection_status=approved` a la URL de retorno no debe cambiar el estado mostrado.
5. Espera la notificación de prueba y confirma que solo la consulta autenticada al proveedor produce la transición final. Un pago aprobado debe conservar la reserva comercial pagada; rechazo o vencimiento libera el horario según el estado de la operación.
6. Revisa Mailpit en `http://127.0.0.1:8025`: la aprobación genera un aviso al arrendatario y otro al anfitrión; rechazo o vencimiento genera uno al arrendatario. Reenvía la misma notificación y confirma que no aparece otro aviso. Para revisar reintentos, interrumpe temporalmente Mailpit antes de procesar una transición sintética y restáuralo; el worker debe entregar el aviso pendiente sin cambiar el pago.

El flujo `/demo/arriendo` y `Pagar (simulado)` siguen siendo una demo independiente. Esa ruta nunca inicia un Checkout Pro comercial ni genera cargos reales.
