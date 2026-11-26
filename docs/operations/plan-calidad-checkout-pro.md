# Plan de calidad de Checkout Pro · 74 a 100

**Objetivo.** Alcanzar 100/100 en el panel de Mercado Pago para la aplicación productiva de EspaciGo. El resultado se comprueba con una medición nueva del Checkout Pro que crea preferencias. El informe del 25 de noviembre de 2026 aprobó la integración con 74/100. Mercado Pago define el puntaje y las acciones pendientes. [Medición de calidad de Checkout Pro](https://www.mercadopago.cl/developers/es/docs/checkout-pro-preferences/how-tos/integration-quality?scope=prod)

## Fase 1 · Establecer la línea base

- [x] Consultar en modo de solo lectura el pago productivo `190475305601`. Mercado Pago informó `approved/accredited`, captura confirmada, débito Visa, una cuota, $1.036 cobrados, $39 de comisión de procesamiento, $72 del split y $925 netos. La base fue $1.000. El split agrupa $36 del arrendador y $36 del arrendatario según los metadatos.
- [x] Comprobar la representación histórica del pago. Tiene referencia externa y un ítem. El correo del pagador y el ID, la categoría y la descripción del ítem están ausentes en esa respuesta. La orden comercial respondió `Caller id must be the same as collector id.`, así que la preferencia histórica quedó sin verificar con el perfil del CLI.
- [ ] Guardar una captura o exportación de la medición completa, expandir las cinco acciones pendientes de aprobación y transcribir su campo, estado y sección. El texto pegado omite algunos rótulos. Confirmar en el panel si descriptor y URLs de retorno aparecen pendientes.
- [x] Consolidar la evidencia del pago histórico y del código local para los criterios identificados. La evaluación del panel sigue pendiente para la siguiente preferencia productiva.

| Criterio del informe | Evidencia de la medición histórica | Evidencia del candidato local |
| --- | --- | --- |
| Correo del comprador | Ausente en la respuesta del pago. | El correo de la cuenta activa se incluye en la preferencia. PostgreSQL verifica el caso con perfil y sin perfil verificado. |
| Apellidos del comprador | El informe los señala como acción recomendada. | El payload usa `payer.surname`. Se omite si la identidad no está verificada. Las pruebas cubren tildes y la proyección de uno o dos apellidos. |
| ID, categoría y descripción del ítem | Ausentes en el ítem de la respuesta del pago. | ID estable, categoría `services` y descripción con el título del espacio. El título conserva caracteres completos al truncar. |
| Descriptor y URLs de retorno | Su resultado exacto requiere consultar el detalle del panel o la preferencia histórica. | Descriptor `ESPACIGO`. Los retornos de éxito, rechazo y pendiente usan el origen HTTPS canónico. |
| Webhook y referencia externa | El pago tiene referencia externa. Los registros confirman firma válida y consulta al proveedor. | Las pruebas verifican URL de notificación, firma, deduplicación y que solo se aplican datos del proveedor. |
| Vigencia, cantidad, moneda, importe y cuotas | El pago confirmó CLP, $1.036 cobrados y una cuota. | Una preferencia de $1.000 base conserva cantidad uno, CLP, $1.036 de cobro, $72 de split, vigencia y una cuota. |
| Buenas prácticas sin puntaje en el informe | La evaluación las identifica como opcionales. | Su seguimiento se mantiene fuera de la medición principal. La conciliación operativa tiene su propio plan. |

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
- [x] Probar el payload con transporte simulado. La prueba controla que del pagador salgan únicamente nombre, apellidos y correo. La proyección con PostgreSQL incluye solo el correo cuando falta una identidad verificada. Las preferencias de prueba omiten datos de la cuenta de EspaciGo.
- [x] Ejecutar las pruebas locales de importe, split, vigencia, reintento de creación, webhook y reserva. La prueba de conciliación con PostgreSQL reprodujo $1.036 cobrados, $72 de split, $39 de cargo y $925 netos. La preferencia productiva posterior al despliegue sigue pendiente.
- [ ] Mantener el SDK de Go como mejora separada si ofrece una ventaja comprobable. Conservar el cliente HTTP mientras cubra las operaciones actuales. Dejar respuesta binaria, efectivo, anuncios, formato modal, logos, envío y exclusiones adicionales fuera del camino crítico salvo que el panel identifique un requisito aplicable.

**Salida.** Una preferencia nueva cumple el contrato de Checkout Pro y queda cubierta por pruebas antes de hacer un cargo. [Referencia de creación de preferencias](https://www.mercadopago.cl/developers/es/reference/online-payments/checkout-pro-preferences/create-preference/post)

## Fase 3 · Medir de nuevo y cerrar los pendientes

- [ ] Integrar las mejoras en `desarrollo-local`, comprobar la CI y promover a `main` con autorización explícita.
- [ ] Con autorización para una compra controlada, crear una preferencia y un pago productivos nuevos con un arrendador vinculado. Guardar en la evidencia solo identificadores operativos, estados, presencia de campos e importes.
- [ ] Ejecutar **Calidad de la integración → Medir de nuevo** para la aplicación que procesó ese pago. Registrar fecha, nuevo Payment ID, puntaje y estado de cada acción. La medición oficial de este flujo se confirma en el panel productivo. Un pago histórico conserva el payload con el que se creó.
- [ ] Corregir los criterios pendientes de puntuación de la nueva medición. Si un campo figura enviado y el panel lo marca pendiente, contrastar la preferencia y la versión desplegada antes de aplicar otro cambio. Escalar al soporte de Mercado Pago con el ID y evidencia de presencia si el panel sigue sin reconocerlo.
- [ ] Cerrar al obtener **100/100** en el panel. Registrar aparte cancelaciones, devoluciones, disputas y reportes financieros para que su entrega pueda verificarse de forma independiente.

**Salida.** El panel registra 100/100 para un pago nuevo y la evidencia identifica la versión de EspaciGo que lo procesó.
