# Comprobante del anfitrión y conciliación con Mercado Pago

El comprobante del anfitrión debe mostrar el neto informado por Mercado Pago para cada pago comercial, explicar el origen de cada cargo y distinguir el estado del pago de la disponibilidad de los fondos. La calculadora de comisiones vivirá en **Anfitrión → Mis reservas → Finanzas** y permitirá estimar el neto antes de cobrar.

## Diagnóstico y pago de referencia

La consulta comercial actual calcula `net_host_amount_clp` como base del arriendo menos comisión e IVA de EspaciGo del anfitrión. La función de desglose reutiliza ese campo en el comprobante, los KPIs de Finanzas, el detalle y otras vistas. El modal indica «antes de tarifa MP» en algunos puntos, mientras una salida imprimible etiqueta el mismo importe como «Ingreso Neto Anfitrión». El proveedor Checkout Pro ya consulta pagos por ID con OAuth del vendedor para verificar el pago. El decodificador actual no proyecta el neto ni los cargos de procesamiento de esa respuesta. [Consulta SQL](../internal/adapters/postgres/rentalflow/commercial_reservations.go#L195) · [Desglose compartido](../frontend/src/features/bookings/domain/financial-breakdown.ts#L21) · [Salida imprimible](../frontend/src/features/bookings/ui/reservation-invoice-modal.tsx#L106) · [Decodificador de Checkout Pro](../internal/checkoutpro/provider.go#L159)

El pago de producción de Checkout Pro **`190475305601`** se verificó con MPCLI y el perfil autorizado `anfitrion-prod` (Chile). El pago con débito Visa figura `approved` / `accredited`, con fondos `released` y sin reembolso al momento de la consulta. Mercado Pago informó la misma hora de aprobación y liberación, el 25 de noviembre de 2026 a las 12:37:30 de Santiago. En esta consulta el perfil coincidió con el titular del marketplace y la respuesta de MPCLI no incluyó `collector_id` en el objeto de pago. Durante la implementación, comprobar si el endpoint directo con OAuth del vendedor lo devuelve y conservar la validación de titularidad antes de persistir la conciliación. [Identificador en el comprobante](../frontend/src/features/bookings/ui/reservation-invoice-modal.tsx#L95) · [API Obtener pago](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-api-payments/get-payment/get)

| Concepto | CLP | Origen |
| --- | ---: | --- |
| Valor del arriendo | 1.000 | Metadatos del pago |
| Comisión EspaciGo del arrendatario, $30 más $6 de IVA | +36 | Metadatos del pago y regla del checkout |
| Total cobrado al arrendatario | 1.036 | `transaction_details.total_paid_amount` |
| Comisión EspaciGo del anfitrión, $30 más $6 de IVA | −36 | Metadatos del pago |
| Comisión total de EspaciGo incluida en el split | 72 | `fee_details.application_fee` y cargo `third_payment` |
| Importe del anfitrión antes de la tarifa de Mercado Pago | 964 | Cálculo actual |
| Neto informado por Mercado Pago | 925 | `transaction_details.net_received_amount` |
| Comisión de procesamiento de Mercado Pago | 39 | `fee_details.mercadopago_fee` y cargo `mercadopago_fee` |

Los cargos informados cierran el pago en **$1.036 − $72 − $39 = $925**. La comisión de Mercado Pago equivale al **3,76 %** del total pagado de $1.036 en esta operación. La comisión del arrendatario ya está incluida en el total cobrado y en la comisión conjunta del split. El desglose identifica el mismo cargo en `fee_details` y `charges_details`, por lo que se cuenta una sola vez. [Cálculo y redondeo del checkout](../internal/checkoutpro/service.go#L142) · [Distribución de comisiones en Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/integration-configuration/integrate-marketplace)

## Fase 1 — Confirmar el pago y normalizar el desglose

- [x] Consultar el pago de referencia con un perfil autorizado y registrar moneda, medio, montos, cargos y fechas de aprobación y liberación sin datos personales.
- [ ] Verificar el identificador del vendedor y la referencia comercial en la lectura que usará el backend.
- [ ] Confirmar qué devuelve Checkout Pro para esta cuenta en `transaction_details.net_received_amount`, `fee_details`, `charges_details`, `money_release_date` y `money_release_status`. Comprobar qué cargos corresponden al procesamiento, al marketplace y a otros conceptos. Verificar su relación con el monto que vio el anfitrión. [API Obtener pago](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-api-payments/get-payment/get)
- [ ] Si el pago omite campos o el neto diverge del movimiento, contrastar por ID con el reporte de ventas Split 1:1. Consultar el reporte de liberaciones cuando haya que verificar el ingreso al saldo disponible. Identificar los registros de pago, reembolso, contracargo y otros ajustes antes de sumarlos. [Reporte de ventas Split 1:1](https://www.mercadopago.cl/developers/es/docs/split-payments/split-1-1/additional-content/reports/sales-report/download-report) · [Reporte de liberaciones](https://www.mercadopago.cl/developers/es/docs/reports/released-money/report-fields)
- [x] Explicar la diferencia entre $964 y $925. El proveedor identifica los $39 completos como comisión de procesamiento.

## Fase 2 — Conciliar y sincronizar por ID de pago

- [ ] Reutilizar la relación entre reserva y `provider_payment_id`, el cliente Go y el acceso OAuth del anfitrión. Ampliar la lectura para proyectar únicamente los datos financieros necesarios. Validar pago, vendedor, moneda, referencia, monto y modo real o de prueba antes de vincularlos con una reserva.
- [ ] Persistir una conciliación por pago con procedencia, fecha de consulta, importes en CLP exactos, comisión de procesamiento, otros cargos identificados, neto del vendedor y estado de liberación. Conservar los valores de comisión e IVA cobrados por EspaciGo al crear el pago. Registrar ausencia de datos como campos sin valor y conservar el último resultado confirmado si falla una actualización.
- [ ] Procesar la conciliación tras la verificación del pago y refrescarla cuando cambie el proveedor o cuando falten datos de liberación. Usar reintentos idempotentes y controlar el orden de las actualizaciones. Completar el historial de pagos aprobados por lotes acotados con acceso al vendedor correspondiente. Registrar reembolsos o contracargos como movimientos vinculados al pago original.
- [ ] Aceptación. Volver a procesar el mismo pago conserva un único resultado por versión. Los estados de conciliación distinguen **Pendiente**, **Conciliado** y **Con diferencias**. El pago aprobado y el dinero liberado se representan por separado, con la fecha informada por el proveedor cuando exista.

## Fase 3 — Actualizar el comprobante y Finanzas

- [ ] Cambiar el comprobante comercial del anfitrión, su descarga y el detalle de reserva para mostrar el ID de pago, valor del arriendo, comisión EspaciGo del arrendatario, total pagado, comisión e IVA de EspaciGo del anfitrión, comisión de procesamiento de Mercado Pago, otros ajustes identificados y neto. Mostrar fuente, fecha de conciliación y disponibilidad de los fondos cuando el proveedor las informe.
- [ ] Consumir el mismo desglose en el modal, la salida imprimible, la lista y los KPIs de Finanzas. Para pagos pendientes de conciliar, mostrar `Importe antes de tarifa Mercado Pago` e indicar que el neto verificado está pendiente. En los totales, distinguir montos conciliados de montos pendientes de confirmar.
- [ ] Aceptación. Para el ejemplo verificado, Finanzas y el comprobante muestran $39 de procesamiento y $925 de neto. Un pago aprobado con fondos aún pendientes se muestra como aprobado con disponibilidad pendiente.

## Fase 4 — Calculadora de comisiones

- [ ] Agregar una acción **Calculadora de comisiones** en Finanzas, junto al resumen y al listado de transacciones. Las filas ya permiten buscar por ID de pago y pueden llevar al comprobante conciliado. Mantener la calculadora accesible desde el mismo lugar para todos los espacios del anfitrión. [Navegación de reservas](../frontend/src/features/bookings/ui/reservations-list.tsx#L955)
- [ ] Permitir ingresar el valor del arriendo y la comisión de Mercado Pago como monto fijo o porcentaje estimado. Para cualquier porcentaje, indicar la base elegida y si incluye IVA. Calcular las dos comisiones de EspaciGo con la política y el redondeo del checkout. Mostrar total del arrendatario, cargos por separado y neto estimado del anfitrión. Usar sus propios importes conciliados como referencia cuando existan, indicando fecha y método de pago.
- [ ] Fórmula de la vista estimada. **Total arrendatario = arriendo + comisión EspaciGo arrendatario. Neto anfitrión = arriendo − comisión EspaciGo anfitrión − costo Mercado Pago − otros cargos aplicables.** Mostrar el efecto de cambiar la tarifa sin actualizar el comprobante histórico ni el monto de checkout.
- [ ] Aceptación. Con $1.000 de arriendo y $39 de costo de procesamiento informado manualmente, el arrendatario ve un total estimado de $1.036 y el anfitrión un neto estimado de $925.

## Verificación y orden de trabajo

- [ ] Probar precisión y redondeo CLP, datos faltantes, costo cero confirmado, OAuth vencido, cuenta o pago ajeno, discrepancias, reintentos, devoluciones y cambios de liberación. Usar respuestas sintéticas normalizadas para Go y PostgreSQL.
- [ ] Verificar en Chromium el comprobante descargable, la consistencia de cifras en Finanzas y la calculadora. Usar el caso reportado de $1.000 como fixture separado de cualquier pago real.
- [ ] Entregar primero la conciliación y la corrección de etiquetas. Agregar después el historial y la calculadora sobre el mismo desglose. La revisión de producción se puede hacer de solo lectura con el pago existente, con acceso al vendedor autorizado para esa operación.


## Implementación en contratos

- V000006 agrega la conciliación financiera a la operación comercial. La verificación de Checkout Pro guarda el desglose por ID de pago y un proceso periódico completa pagos aprobados y revisa liberaciones pendientes con OAuth del anfitrión.
- El comprobante del anfitrión, el texto descargable y la salida imprimible muestran los dos cargos de EspaciGo (3 % más IVA por cada parte), el total pagado por el arrendatario, la tarifa de Mercado Pago y el neto informado. Finanzas deja fuera del neto informado los pagos comerciales cuyo neto está pendiente.
- La calculadora de Finanzas permite simular el costo en pesos o con una tasa, con base e IVA explícitos. Los resultados estimados se mantienen en la calculadora.
- La implementación conserva la validación del vendedor ya existente. Queda por validar la carga automática del pago de referencia en la aplicación desplegada con la credencial del vendedor asociado.
