# Plan de cancelaciones y reembolsos · Checkout Pro Split 1:1

**Objetivo.** Coordinar el estado de la reserva, el pago y la devolución dentro de EspaciGo. El arrendatario puede iniciar una solicitud según su política y el arrendador conserva la autorización sobre el pago en Mercado Pago. Hotosoft participa por la parte del marketplace que corresponda al split. [Split 1:1 en Chile](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)

## Fase 1 · Confirmar datos y permisos de cada operación

- [x] Verificar con `mpcli payments get 190475305601` un pago productivo aprobado, capturado y con fondos liberados. El recurso informó `transaction_amount_refunded=0` y `refunds=[]`.
- [x] Consultar el historial separado con `mpcli refunds list 190475305601`. El perfil actual recibió `ERROR_RESOURCE_NOT_FOUND: Payment not found`. La consulta de la orden comercial asociada devolvió `ERROR_UNAUTHORIZED: Caller id must be the same as collector id.` El dato de cero del objeto del pago es evidencia disponible. El historial independiente queda pendiente.
- [ ] Repetir las consultas de lectura con OAuth del arrendador que cobró el pago, desde un proceso autorizado y sin exponer el token. Comprobar ID, referencia externa, vendedor, CLP, entorno, importe, saldo reembolsable e historial. Guardar solo la respuesta normalizada. Un error de acceso o búsqueda deja el historial como desconocido.
- [ ] Confirmar en pruebas qué ocurre con el split, las comisiones y la liberación de fondos en devoluciones totales y parciales. Tratar el neto de $925 como el neto de esa operación. El saldo disponible y el importe que Mercado Pago exige a cada parte se verifican por separado.

**Salida.** Permisos y evidencia de lectura comprobados para los tres roles. El pago productivo queda como referencia de solo lectura mientras se usa un pago sintético separado para las pruebas de escritura. [API de consulta de reembolsos](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/get-refunds/get)

## Fase 2 · Cancelar un pago pendiente

- [x] Separar en Go el estado de la reserva del estado de Mercado Pago. Antes de ofrecer **Cancelar pago**, verificar que quien actúa pertenece a la reserva y consultar el pago con el OAuth del vendedor.
- [x] Si aún no existe ID de pago, cerrar el checkout local conforme a su vencimiento. Consultar por referencia externa antes de liberar la disponibilidad cuando el resultado remoto sea incierto.
- [x] Para `pending` o `in_process`, persistir la intención y enviar `PUT /v1/payments/{id}` con `status=cancelled`. Reutilizar la transición existente al confirmar la cancelación por respuesta o webhook. Si el pago se aprueba durante la operación, conservar la confirmación y presentar la vista previa de reembolso.
- [x] Mantener la reserva en espera ante timeout o respuesta ambigua. Recuperar el estado consultando a Mercado Pago. Emitir un solo aviso y liberar disponibilidad una vez que corresponda.

**Salida.** Pruebas locales de cancelación aceptada, repetida, tardía, sin ID, con timeout y con aprobación concurrente. [Cancelaciones y reembolsos en Checkout Pro](https://www.mercadopago.cl/developers/es/docs/checkout-pro-preferences/additional-settings/refunds-and-cancellations)

## Fase 3 · Completar los reembolsos de pagos aprobados

- [x] Reutilizar la vista previa y el endpoint de reserva existentes. Calcular el importe en Go con la política congelada y los reembolsos aprobados. Mostrar total cobrado, importe solicitado y saldo reembolsable. Volver a comprobar autorización y estado justo antes de ejecutar.
- [x] Persistir el intento, el monto y la clave de idempotencia antes de `POST /v1/payments/{id}/refunds`. Para el total usar cuerpo vacío si no existe una devolución previa. Para un parcial o una devolución del remanente usar el monto exacto comprobado. Prevenir reembolsos duplicados o superiores al total mediante bloqueo y conciliación por ID.
- [x] Distinguir resultado pendiente, aprobado, rechazado y acceso al proveedor no disponible. Ante resultado incierto, recuperar el mismo intento y su clave. Ante rechazo definitivo, conservar la reserva activa según el comportamiento actual y mostrar un próximo paso. Mostrar falta de saldo únicamente si el proveedor la confirma.
- [x] Si existe un reembolso hecho en el portal, vincularlo por ID después de verificar pago, vendedor, monto, moneda y estado. Si no hay coincidencia suficiente, ponerlo en revisión operativa antes de permitir un nuevo POST.
- [ ] Para falta de saldo confirmada, permitir que el arrendador reponga su parte y reintente desde la reserva. Definir el procedimiento de soporte para devoluciones externas excepcionales con comprobante y control de doble pago. Hotosoft puede tramitar su parte conforme a las capacidades del split. La diferencia pertenece al flujo de cobro del vendedor.

**Avance local.** En la rama `codex/marketplace/feature/contratos-1-modulo-contratos` quedaron implementadas y probadas localmente con Go, PostgreSQL y Playwright: (1) cancelación de pago pendiente con búsqueda por `external_reference`, cierre local al vencer, arriendo de lease de 30s, recuperación tras timeout/aprobación concurrente y aviso único `pago_pendiente_cancelado`; (2) cálculo y exposición de `refunded_amount_clp` y `refundable_balance_clp`, cuerpo vacío solo en reembolso total sin devoluciones previas, monto explícito para parcial/remanente, detección normalizada de falta de saldo confirmada (`409 refund_insufficient_funds`) conservando la reserva reintentable tras reponer fondos, bloqueo por `refund_requires_reconciliation`, vinculación de reembolso externo (`reconcile-refund`) y bloqueo ante contracargos (`charged_back`); (3) notificaciones durables deduplicadas (`pago_pendiente_cancelado`, `reembolso_fallido`, `reserva_cancelada`, `contracargo_reserva`), OpenAPI/clientes generados y UI de reserva/Finanzas sin exponer netos sin conciliar tras reversas. Siguen pendientes: repetir lecturas productivas con OAuth real del vendedor, validar comportamiento real del split ante devoluciones en MP, definir el procedimiento operativo de soporte para devoluciones externas excepcionales, integrar el reporte de ventas Split 1:1, ampliar la vista de soporte e integrar/promover tras probar en `desarrollo-local`.

**Salida.** Pruebas de total, parcial, remanente, falta de saldo, OAuth revocado, webhook repetido, disputa, devolución externa y ausencia de doble abono. Validar que el estado de reserva y la ocupación solo se cierran una vez conforme al resultado confirmado. [API de creación de reembolso](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-api-payments/create-refund/post)

## Fase 4 · Conciliar Finanzas, reportes y operación

- [ ] Mostrar la misma secuencia de estados en la reserva, en Finanzas del arrendador y en soporte. Separar el cobro original, el cargo real del procesador, el split, las devoluciones confirmadas y el neto posterior. Evitar presentar como recibido un importe aún no conciliado.
- [ ] Integrar el reporte oficial de ventas Split 1:1 y evaluar los reportes de liquidaciones y movimientos de saldo. Cruzar por pago, vendedor y moneda. Conservar íntegro el snapshot original y registrar reversas y diferencias por separado. [Reporte de ventas con split](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/reports/sales-report/introduction)
- [ ] Añadir avisos durables a arrendador y arrendatario y una vista de soporte con fecha, monto, transición y motivo técnico acotado. Mantener el contracargo como incidencia propia y definir cómo presentar evidencia al proveedor.
- [ ] Actualizar OpenAPI y Scalar al agregar o cambiar rutas. Ejecutar la matriz con PostgreSQL y navegadores en `desarrollo-local`. Promover a producción con autorización explícita, y ejecutar cualquier devolución real solo cuando se autoricen el pago y el importe concretos.

**Salida.** Una persona puede consultar qué ocurrió, cuánto se devolvió, cómo se confirmaron los importes y qué acción queda disponible. Finanzas coincide con Mercado Pago para los casos de prueba conciliados.
