# Mercado Pago Chile en EspaciGo

El módulo usará Checkout Pro con Split de Pagos 1:1. Cada reserva vincula a un anfitrión vendedor y a EspaciGo como marketplace. El arrendatario paga en la página de Mercado Pago y el proveedor distribuye el pago. Mercado Pago descuenta su comisión de los fondos del vendedor antes de descontar la comisión de EspaciGo. Su guía de Split 1:1 en Chile documenta el saldo en cuenta y excluye las transferencias desde instituciones financieras externas. La guía general de Checkout Pro describe otros medios de pago. Confirma los medios disponibles para tu aplicación en el flujo 1:1 antes de ofrecerlos. [Integración oficial de Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)

La rama parte de `origin/desarrollo-local`. Integra los cambios del módulo en `desarrollo-local` y prueba allí. La promoción posterior a `main` requiere autorización explícita.

## 1. Identificar tu aplicación en Chile

1. Inicia sesión con tu cuenta de desarrollador en [Integraciones de Mercado Pago Chile](https://www.mercadopago.cl/developers/panel/app).
2. Abre la aplicación que usarás para EspaciGo. Verifica que corresponda a Chile (`MLC`) y a **Checkout Pro**. Para Split 1:1, la documentación específica usa **Checkout Pro con Preferencias** y el campo `marketplace_fee`. La documentación general recomienda Orders para integraciones nuevas. Si el panel solo ofrece Orders, confirma con Mercado Pago que tu aplicación admite Split 1:1 antes de elegir esa variante. [Checkout Pro con Preferencias](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/overview) · [Referencia de Checkout Pro con Orders](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-orders/overview)
3. En **Credenciales → Prueba**, ubica el **Access Token de prueba** y la **Public Key de prueba**. Usa el selector de entorno para identificarlos. Un token de prueba de Checkout Pro también puede empezar por `APP_USR-`. El **Client ID** y el **Client Secret** del integrador se consultan en la sección de credenciales de producción del panel y se usan para el intercambio de OAuth de la aplicación. Para pedir credenciales OAuth de sandbox, solicita `test_token: true` y comprueba el entorno antes de hacer pagos. [Credenciales](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/your-integrations/credentials) · [OAuth](https://www.mercadopago.cl/developers/es/docs/security/oauth/creation)
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

Usa esta plantilla con los nombres propuestos para el futuro adaptador Go. Copia los valores directamente desde el panel. Mantén los secretos fuera del chat, los logs, Git y las variables `NEXT_PUBLIC_*`.

```dotenv
MERCADOPAGO_TEST_ACCESS_TOKEN=
MERCADOPAGO_CLIENT_ID=
MERCADOPAGO_CLIENT_SECRET=
MERCADOPAGO_WEBHOOK_SECRET=
```

| Valor | Uso |
| --- | --- |
| `MERCADOPAGO_TEST_ACCESS_TOKEN` | Credencial del panel para comprobar acceso a la API con el entorno de prueba. |
| `MERCADOPAGO_CLIENT_ID` | Identifica la aplicación de EspaciGo en el consentimiento OAuth. |
| `MERCADOPAGO_CLIENT_SECRET` | Permite a Go intercambiar y renovar credenciales OAuth en el servidor. |
| `MERCADOPAGO_WEBHOOK_SECRET` | Clave de firma generada al configurar Webhooks para el entorno correspondiente. Completa este valor cuando esté disponible la ruta de prueba. |
| Public Key de prueba | Clave publicable. Hace falta si la interfaz inicializa el SDK de Mercado Pago. Una redirección directa usa la URL que devuelve la preferencia. |

El SDK oficial de Go está agregado al módulo como `github.com/mercadopago/sdk-go v1.14.0`. La importación inicial que muestra Mercado Pago (`github.com/mercadopago/sdk-go/pkg/config`) configura el cliente del servidor con un Access Token. Para Split 1:1, la preferencia real de cada anfitrión debe autenticarse con el token OAuth de ese vendedor.

El backend actual usa el adaptador simulado `fakebooking`. `scripts/local-dev.sh api` todavía carece de un cargador para este archivo y de un adaptador de Mercado Pago. Los tokens OAuth de los anfitriones se obtendrán después de su consentimiento y se guardarán cifrados por anfitrión en el backend. El token de prueba del panel sirve para la comprobación inicial de acceso. La preferencia con reparto usa el token OAuth del anfitrión.

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
3. **Confirmar el pago.** Configura **Webhooks → Prueba → Pagos** con una URL HTTPS pública que exponga solo la ruta de notificaciones necesaria. Valida `x-signature` con la clave del panel y consulta `GET /v1/payments/{id}` con la credencial del vendedor asociado. Verifica entorno, vendedor, moneda, monto, estado y referencia antes de aplicar de forma idempotente el cambio en la reserva. Las URLs de retorno sirven para mostrar el estado al comprador, incluida la espera mientras el backend confirma el pago. [Webhooks de Mercado Pago](https://www.mercadopago.cl/developers/es/docs/split-payments/additional-content/your-integrations/notifications/webhooks)
4. **Verificar en `desarrollo-local`.** Prueba pago aprobado, pendiente y rechazado, duplicados del webhook, firma inválida y reconexión del anfitrión. Comprueba en las cuentas de prueba el importe del comprador, la comisión de EspaciGo y el saldo neto del anfitrión. Ejecuta `./scripts/local-dev.sh test` una vez implementado e integrado el código.
