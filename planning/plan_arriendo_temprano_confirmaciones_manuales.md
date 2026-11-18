# Demo local de arriendo con pago e invoice

**Objetivo.** Completar un arriendo desde una pantalla sencilla, con respaldo en la API Go y PostgreSQL local. Usar un espacio publicado, una hora disponible y dos cuentas de prueba existentes. Una persona puede mostrar ambos roles con dos sesiones.

**Estado al 17 de noviembre de 2026.** Demo implementada en Go, con V100 aplicada en PostgreSQL local. Las confirmaciones, la aprobación del procesador y la ocupación se verificaron en la base local. La invoice se leyó desde ambas cuentas y después de recargar en Chromium.

## Abrir la demo

- Entra a [Arriendo de prueba](http://127.0.0.1:8080/demo/arriendo/) desde el equipo local.
- Usa las dos cuentas de prueba existentes. El arrendatario crea el arriendo y abre el enlace que aparece en la pantalla en una ventana privada para iniciar sesión como arrendador. Entre cambios de sesión, pulsa **Actualizar estado** para leer las confirmaciones guardadas por la otra cuenta. Para el ejemplo verificado se usaron `host` y `renter_a` de la fixture privada `.local/testing/ticket-native-20261116/fixture.json`.
- Si el servidor local está detenido, ejecuta `LOCAL_EARLY_RENTAL_FLOW_ENABLED=1 ./scripts/local-dev.sh api` desde la raíz del repositorio.

## Recorrido

| Paso | Qué se muestra | Qué guarda la API |
| --- | --- | --- |
| 1. Crear el arriendo | El arrendatario selecciona el espacio y una hora. | El arriendo, el monto en CLP y la retención de esa hora. |
| 2. Identidad, contrato y firma | Cada participante marca **Verificar identidad**, **Aprobar contrato** y **Firmar contrato**. Son casillas de prueba. | Una confirmación de identidad y una confirmación contractual por participante. La API guarda la confirmación contractual cuando recibe aprobación y firma juntas, con actor, fecha y versión del contrato. |
| 3. Pagar | El arrendatario ve el monto y pulsa **Pagar** cuando ambas partes completan las casillas. | Go inicia el pago y procesa el resultado del procesador local. Cuando éste aprueba, PostgreSQL registra el pago aprobado, confirma el arriendo y convierte la retención en reserva. La aprobación proviene del procesador y el arrendador no tiene que intervenir. |
| 4. Mostrar la invoice | La misma pantalla muestra **Pago aprobado** y la **Invoice de prueba**. | El detalle de la API devuelve un número estable de comprobante, la referencia del arriendo y del pago, el monto y la fecha de aprobación. Se obtiene del pago persistido y sigue visible después de recargar. |

## Acceso por API y comprobación

La API usa el prefijo `/api/v1/local/early-rentals` y Bearer de sesión local. El contrato de operaciones está en [OpenAPI](openapi.yaml). El arrendatario llama a `POST /{id}/pay` con el `ETag` del detalle en `If-Match` y un cuerpo `{}`. El resultado devuelve `data.invoice`, que también aparece en el GET del detalle. Reintentar el pago devuelve el mismo comprobante.

Para contrastar el resultado, consulta el mismo ID en `arriendo_temprano`, `arriendo_temprano_confirmacion`, `arriendo_temprano_pago` y `ocupacion`. Debe haber dos confirmaciones de identidad, dos confirmaciones del mismo contrato, un solo pago recibido y una reserva definitiva. La referencia de la aprobación queda en `arriendo_temprano_evento.procesador_referencia`. La invoice es un comprobante de prueba y el procesador no mueve dinero.
