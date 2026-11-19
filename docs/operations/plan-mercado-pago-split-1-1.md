# Plan de integración de Mercado Pago para EspaciGo

## Punto de partida

La aplicación `pago-espacigo` está creada y el SDK Go `v1.14.0` está en la rama de pagos. El Access Token de prueba y la Public Key ya fueron preparados por el usuario. La API local sigue usando `fakebooking`. El flujo comercial necesita vincular a los anfitriones por OAuth, crear el checkout de cada reserva y confirmar el cobro mediante una consulta a Mercado Pago. Las notificaciones a personas se incorporan después de validar ese ciclo.

## 1. Confirmar Marketplace y preparar la conexión

- Usar **Checkout Pro con API de Preferences** en `pago-espacigo`, según la selección confirmada por el usuario. Verificar que la aplicación esté configurada como Marketplace y probar en Chile el cobro con saldo y el abono de la comisión a EspaciGo. Revisar una migración cuando Mercado Pago publique una fecha de descontinuación o un cambio que afecte esta aplicación. [Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)
- Preparar una cuenta de prueba compradora con saldo ficticio y una vendedora distinta, ambas chilenas. Configurar el Client ID y Client Secret del integrador en el backend y registrar la URL exacta del retorno OAuth cuando esa ruta exista. El secreto va en la configuración privada local. [Configuración de Marketplace](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/create-configuration)
- Implementar **Conectar Mercado Pago** en el área de anfitrión. Vincular el consentimiento con un `state` de un solo uso, con expiración y asociado al anfitrión autenticado. Intercambiar el código en Go, persistir el ID del vendedor y los tokens cifrados y renovar ambos tokens de forma atómica. Si la conexión caduca o se revoca, pedir reconexión antes de iniciar otro cobro.

**Criterio de salida.** Un anfitrión de prueba se vincula y la aplicación puede operar con su credencial OAuth. Una conexión fallida o revocada bloquea el cobro de ese anfitrión.

## 2. Crear el checkout de cada reserva y conectarlo a la web

- Reutilizar el ciclo de reservas y su operación de pago persistente. Ampliar su contrato para representar el inicio de un checkout externo con resultado pendiente. El adaptador actual parte de un resultado simulado. El nuevo inicio lee del servidor la reserva, su cotización congelada y el anfitrión, y conserva el vínculo con el objeto de Mercado Pago.
- El arrendatario paga el total de la reserva en CLP. Sobre el importe del arriendo publicado que quedó en la cotización, EspaciGo cobra 3% más IVA sobre ese 3%. Con IVA general de 19%, la comisión bruta equivale a 3,57% antes de redondear. Por un arriendo de $100.000, el comprador paga $100.000, EspaciGo recibe $3.570 y el anfitrión recibe $96.430 menos el cargo de Mercado Pago. Guardar base, comisión, IVA y total de comisión. Aplicar aritmética de pesos enteros con una regla de redondeo definida y validada para la liquidación. [Tasa general del SII](https://www.sii.cl/preguntas_frecuentes/impuestos_mensuales/001_130_0572.htm) · [Descuentos en Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)
- Crear la preferencia en `POST /checkout/preferences` con el token OAuth del anfitrión, `marketplace_fee` como importe, referencia externa de la operación, vencimiento coherente con la reserva y URLs de regreso web. Persistir el ID y la URL devuelta. Reusar el checkout vigente ante doble clic o reintento. Si la creación queda incierta por un timeout, buscar la referencia antes de iniciar otro cobro.
- Conectar el botón de la reserva en Next.js al backend y redirigir a la URL devuelta por Mercado Pago. Mostrar el estado confirmado por EspaciGo al regresar. Configurar y comprobar que el comprador solo pueda pagar con **dinero en cuenta**. Validar también el medio devuelto por Mercado Pago antes de confirmar la reserva. La validación posterior es una defensa adicional. La prueba debe comprobar que las tarjetas y otros medios quedan fuera del checkout.

**Criterio de salida.** Una reserva autorizada genera un único checkout vigente, con monto y comisión correctos. El usuario llega al pago web y ve la reserva pendiente mientras EspaciGo verifica el resultado.

## 3. Recibir y conciliar los webhooks de pago

- Crear una ruta de recepción en Go y hacerla accesible por HTTPS. En local, usar un túnel temporal limitado a esa ruta. Configurar inicialmente en el panel la URL de **Prueba** y el evento **Pagos**. Guardar la clave de firma en la configuración privada del backend. [Notificaciones de Checkout Pro](https://www.mercadopago.cl/developers/es/docs/checkout-pro-preferences/payment-notifications)
- Validar `x-signature` con los valores canónicos documentados y la clave de firma del entorno. Acusar recibo después de persistir el evento válido de forma durable. Deduplicar reenvíos de la misma notificación y conservar actualizaciones distintas de un mismo pago. Reutilizar el procesamiento y la conciliación existentes donde sus contratos lo permitan.
- Consultar el objeto de pago en Mercado Pago con la credencial correspondiente al vendedor. Comparar referencia, vendedor, importe, moneda, entorno y medio de pago con la operación local antes de aplicar un cambio. La URL de retorno muestra información al usuario. La consulta autenticada al proveedor determina el estado del cobro.
- Definir qué ocurre con pagos tardíos tras vencer o cancelar una reserva, con dos cobros para una operación y con reembolsos o contracargos. Conservar el rastro de conciliación y definir la resolución operativa de excepciones que no pueda aplicar el flujo automáticamente.

**Criterio de salida.** Una firma inválida se rechaza. La simulación del panel llega a EspaciGo. Un pago de prueba actualiza la reserva una sola vez aunque se repita o llegue fuera de orden el webhook. Si falla una entrega, la conciliación recupera el estado. El importe del comprador, la comisión de EspaciGo y el neto del anfitrión cuadran con Mercado Pago.

## 4. Enviar notificaciones al arrendatario y al anfitrión

- Enviar avisos después de confirmar y guardar la transición de estado. Para un pago aprobado, avisar al arrendatario que el pago fue recibido y al anfitrión que hay una reserva pagada, respetando el estado de confirmación de la reserva. Si el pago se rechaza o vence, informar al arrendatario con el siguiente paso disponible. Mostrar el estado pendiente en la vista de reserva.
- Usar el mecanismo de correo existente y Mailpit para las pruebas locales. Encolar cada mensaje de forma durable, con una clave por reserva, transición, destinatario y canal. Reintentar envíos fallidos y evitar que los reenvíos del webhook creen avisos nuevos.

**Criterio de salida.** Mailpit muestra un aviso por destinatario y evento de negocio, con el estado y monto correctos. Un fallo temporal de correo se recupera sin cambiar el resultado del cobro.

## 5. Integrar y habilitar

Hacer cambios en `codex/marketplace/feature/pagos-1-modulo-pagos`, integrar a `desarrollo-local` y ejecutar allí las comprobaciones del repositorio y la prueba de punta a punta. Si el módulo requiere cambios de esquema, versionarlos con su clasificación en `db/release-policy.json` y validarlos primero en local. Mantener el flujo simulado disponible para la demo. Con autorización explícita, promover a `main` y configurar la URL HTTPS y los secretos de producción antes de habilitar cobros reales.
