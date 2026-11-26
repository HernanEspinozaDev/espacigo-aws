# Plan de calidad de Checkout Pro · 74 a 100

**Objetivo.** Alcanzar 100/100 en el panel de Mercado Pago para la aplicación productiva de EspaciGo. El resultado se comprueba con una medición nueva del Checkout Pro que crea preferencias. El informe del 25 de noviembre de 2026 aprobó la integración con 74/100. Mercado Pago define el puntaje y las acciones pendientes. [Medición de calidad de Checkout Pro](https://www.mercadopago.cl/developers/es/docs/checkout-pro-preferences/how-tos/integration-quality?scope=prod)

## Fase 1 · Establecer la línea base

- [x] Consultar en modo de solo lectura el pago productivo `190475305601`. Mercado Pago informó `approved/accredited`, captura confirmada, débito Visa, una cuota, $1.036 cobrados, $39 de comisión de procesamiento, $72 del split y $925 netos. La base fue $1.000. El split agrupa $36 del arrendador y $36 del arrendatario según los metadatos.
- [x] Comprobar la representación histórica del pago. Tiene referencia externa y un ítem. El correo del pagador y el ID, la categoría y la descripción del ítem están ausentes en esa respuesta. La orden comercial respondió `Caller id must be the same as collector id.`, así que la preferencia histórica quedó sin verificar con el perfil del CLI.
- [ ] Guardar una captura o exportación de la medición completa, expandir las cinco acciones pendientes de aprobación y transcribir su campo, estado y sección. El texto pegado omite algunos rótulos. Confirmar en el panel si descriptor y URLs de retorno aparecen pendientes.
- [ ] Consolidar una matriz con la evidencia del payload, el resultado del proveedor y el estado de cada criterio. Distinguir acciones obligatorias y recomendadas de las buenas prácticas señaladas como sin puntaje.

**Salida.** Matriz trazable al Payment ID histórico y a la aplicación productiva que se medirá.

## Fase 2 · Validar los campos que afectan la evaluación

| Campo | Trabajo y criterio de aceptación |
| --- | --- |
| `payer.email` | Validar que Go envía el correo de la cuenta activa en la preferencia productiva y que la evaluación lo reconoce. Cubrir los casos con perfil pendiente y verificado. |
| Nombre y apellido | Validar el uso exclusivo de la identidad vigente, con apellidos en `payer.surname`, la clave del endpoint de preferencias. Cubrir una persona con un apellido, dos apellidos y caracteres con tilde. |
| `items.id`, `items.category_id`, `items.description` | Confirmar ID estable del espacio, categoría `services` y descripción del arriendo en la preferencia. Mantener moneda CLP, cantidad uno e importe de la cotización congelada. |
| `statement_descriptor` | Verificar `ESPACIGO` en la preferencia nueva. La presentación en el estado de cuenta depende del emisor. |
| `back_urls`, `notification_url`, `external_reference` | Confirmar las tres rutas HTTPS de retorno, el webhook productivo y la correspondencia entre la referencia y la operación interna. Demostrar firma válida y consulta server-side por ID tras el aviso. |
| Vigencia y cuotas | Confirmar `expires`, rango de fechas y una cuota en una preferencia nueva. Mantener pruebas de aprobación, pendiente y rechazo al volver a EspaciGo. |

- [x] El commit local `ead12de` incorpora los campos faltantes y pruebas de serialización con compradores sintéticos. Falta validar la preferencia devuelta por Mercado Pago, con la identidad autorizada del vendedor, para una transacción posterior al despliegue.
- [ ] Probar el payload con transporte simulado y comprobar que no incluye RUN, fecha de nacimiento, teléfono ni dirección. Las pruebas del proveedor usan compradores de prueba y las preferencias de test de EspaciGo omiten datos reales.
- [ ] Ejecutar pruebas de regresión de importe, split, vencimiento, reintento de creación, webhook y reserva. Comprobar la igualdad entre resultado de Mercado Pago y registro contable del anfitrión.
- [ ] Mantener el SDK de Go como mejora separada si ofrece una ventaja comprobable. Conservar el cliente HTTP mientras cubra las operaciones actuales. Dejar respuesta binaria, efectivo, anuncios, formato modal, logos, envío y exclusiones adicionales fuera del camino crítico salvo que el panel identifique un requisito aplicable.

**Salida.** Una preferencia nueva cumple el contrato de Checkout Pro y queda cubierta por pruebas antes de hacer un cargo. [Referencia de creación de preferencias](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/create-preference/post)

## Fase 3 · Medir de nuevo y cerrar los pendientes

- [ ] Integrar las mejoras en `desarrollo-local`, comprobar la CI y promover a `main` con autorización explícita.
- [ ] Con autorización para una compra controlada, crear una preferencia y un pago productivos nuevos con un arrendador vinculado. Guardar en la evidencia solo identificadores operativos, estados, presencia de campos e importes.
- [ ] Ejecutar **Calidad de la integración → Medir de nuevo** para la aplicación que procesó ese pago. Registrar fecha, nuevo Payment ID, puntaje y estado de cada acción. La medición oficial de este flujo se confirma en el panel productivo. Un pago histórico conserva el payload con el que se creó.
- [ ] Corregir los criterios pendientes de puntuación de la nueva medición. Si un campo figura enviado y el panel lo marca pendiente, contrastar la preferencia y la versión desplegada antes de aplicar otro cambio. Escalar al soporte de Mercado Pago con el ID y evidencia de presencia si el panel sigue sin reconocerlo.
- [ ] Cerrar al obtener **100/100** en el panel. Registrar aparte cancelaciones, devoluciones, disputas y reportes financieros para que su entrega pueda verificarse de forma independiente. [Plan operativo de cancelaciones y reembolsos](./plan-cancelaciones-y-reembolsos-mercado-pago.md)

**Salida.** El panel registra 100/100 para un pago nuevo y la evidencia identifica la versión de EspaciGo que lo procesó.
