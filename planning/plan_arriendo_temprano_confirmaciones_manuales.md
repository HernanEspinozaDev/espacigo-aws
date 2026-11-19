# Demo local de arriendo con pago e invoice

**Objetivo.** Completar un arriendo desde una pantalla sencilla, con respaldo en la API Go y PostgreSQL local. Usar un espacio publicado, una hora disponible y dos cuentas de prueba existentes. Una persona puede mostrar ambos roles con dos sesiones.

**Estado al 17 de noviembre de 2026.** Demo implementada en Go, con V100 aplicada en PostgreSQL local. Las confirmaciones, la aprobación del procesador y la ocupación se verificaron en la base local. La invoice se leyó desde ambas cuentas y después de recargar en Chromium.

## Abrir la demo integrada

1. Inicia la API con `LOCAL_EARLY_RENTAL_FLOW_ENABLED=1 ./scripts/local-dev.sh api`.
2. En otra terminal ejecuta `./scripts/local-dev.sh web` y abre [EspaciGo local](http://127.0.0.1:3000).
3. Usa dos sesiones con cuentas de prueba distintas. El arrendatario selecciona un horario en la publicación y el arrendador sigue el aviso del menú de cuenta o entra en **Mis reservas**.

La pantalla anterior sigue disponible en [Arriendo de prueba](http://127.0.0.1:8080/demo/arriendo/). La demo integrada admite publicaciones por hora con duraciones de 1, 2 o 4 horas y las reglas vigentes de disponibilidad y tarifa. La retención dura hasta 30 minutos o hasta que comience el horario, lo que ocurra primero.

## Recorrido

| Paso | Qué se muestra | Qué guarda la API |
| --- | --- | --- |
| 1. Enviar la solicitud | El arrendatario elige la hora, envía la solicitud y confirma su identidad de prueba. El arrendador recibe un aviso dentro de la aplicación. | Los datos del arriendo, el monto en CLP, la retención y la confirmación del arrendatario. |
| 2. Responder | El arrendador aprueba o rechaza la solicitud. | La aprobación habilita el contrato. El rechazo libera la retención y se registra en el historial. |
| 3. Firmar | Cada participante marca **Apruebo y firmo el contrato de prueba** y guarda su firma. | Una confirmación contractual por participante, con actor, fecha y versión. |
| 4. Pagar | Cuando ambas firmas están guardadas, el arrendatario previsualiza el invoice y pulsa **Aprobar pago simulado**. | Go registra el resultado del procesador local. PostgreSQL confirma el arriendo y convierte la retención en reserva. |
| 5. Mostrar el invoice | Ambas cuentas ven el pago aprobado y el invoice en **Mis reservas**. | La API devuelve el comprobante con el mismo número, monto y fecha al volver a abrir la reserva. |

## Acceso por API y comprobación

La API usa el prefijo `/api/v1/local/early-rentals` y Bearer de sesión local. El contrato de operaciones está en [OpenAPI](openapi.yaml). El arrendatario llama a `POST /{id}/pay` con el `ETag` del detalle en `If-Match` y un cuerpo `{}`. El resultado devuelve `data.invoice`, que también aparece en el GET del detalle. Reintentar el pago devuelve el mismo comprobante.

Para contrastar el resultado, consulta el mismo ID en `arriendo_temprano`, `arriendo_temprano_confirmacion`, `arriendo_temprano_pago` y `ocupacion`. Debe haber dos confirmaciones de identidad, dos confirmaciones del mismo contrato, un solo pago recibido y una reserva definitiva. La referencia de la aprobación queda en `arriendo_temprano_evento.procesador_referencia`. La invoice es un comprobante de prueba y el procesador no mueve dinero.
