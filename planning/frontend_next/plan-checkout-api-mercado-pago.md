# Checkout API de Mercado Pago en desarrollo local

## 1. Estabilizar la cotización

- Conservar la selección de fecha, horario y modalidad y el resumen lateral que ya existen.
- Dejar un botón visible **Cotizar arriendo** y mostrar **Pagar con Mercado Pago** cuando el servidor confirme el importe. Mantener contraste, foco, estado de carga y ancho adecuado en móvil.
- Comprobar que el HTML del servidor y los recursos del cliente pertenecen al mismo build. Revisar los cambios de estilo y el primer render sin ocultar los avisos de hidratación.

## 2. Preparar la pantalla de pago

- Abrir un formulario dentro de EspaciGo que muestre el total oficial en CLP, la información de la reserva y que Mercado Pago procesa el pago.
- Usar la identidad visual de EspaciGo y campos seguros de MercadoPago.js. Mostrar **Pagar con tarjeta** al confirmar el formulario. Presentar cada medio adicional solo si existe un flujo soportado para la cuenta y el Split 1:1.
- Mostrar por separado los estados de carga, error de tokenización, pago pendiente, rechazo, aprobación y conciliación.

## 3. Conectar y conciliar el pago de prueba

- Usar la Public Key de prueba en el navegador y el token OAuth del vendedor en Go. Enviar únicamente el token de un solo uso y los campos necesarios a API Pagos.
- Calcular importe y comisión en el servidor. Persistir retención e idempotencia antes del POST e impedir un segundo envío si la respuesta queda incierta.
- Verificar modo de prueba, vendedor, importe, moneda, comisión y referencia antes de confirmar la reserva. Procesar los cambios posteriores mediante el webhook firmado.
- Registrar etapa, estado HTTP, código permitido del proveedor e identificador de solicitud cuando exista. Mantener fuera de logs tarjetas, credenciales y datos del comprador.

## 4. Validar en el túnel de pruebas

- Ejecutar los checks de Go, Next y del contrato OpenAPI. Probar con respuestas simuladas una aprobación, un rechazo y un resultado incierto.
- Aplicar únicamente las migraciones pendientes de este flujo a la base local, habilitar Checkout API de prueba y verificar el túnel.
- Recorrer **seleccionar → cotizar → Pagar con Mercado Pago → Pagar con tarjeta** en móvil y escritorio. Verificar el formulario y la ausencia de avisos de hidratación.
- Reservar para la prueba manual con Mercado Pago el pago aprobado, la comisión y la confirmación de la reserva. Documentar cualquier restricción de tarjeta o saldo del proveedor que aparezca.

## Verificación local

- Go, TypeScript, OpenAPI y las pruebas del gateway pasaron. La migración V114 ya estaba aplicada y el plan local no tenía migraciones pendientes.
- Las pruebas focales de navegador comprobaron el contraste del botón de pago, la hidratación y un envío aprobado con MercadoPago.js y el backend simulados. El recorrido sintético que comienza en la publicación obtuvo un 404 para su espacio de prueba. La publicación local usada para Mercado Pago respondió HTTP 200 por el túnel.
- El túnel sirve la pantalla de Checkout API y la API de pago exige una sesión. La validación del pago y del webhook con Mercado Pago queda para una compra manual de prueba.
