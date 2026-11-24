# Métodos de seguridad de la cuenta

Trabajo sobre `fa5b16f` en `codex/marketplace/feature/contratos-1-modulo-contratos`.

| Sección | Implementado en este corte | Pendiente de validación o desarrollo |
| --- | --- | --- |
| Panel y contraseña | Resumen de configuración desde Go, recuperación ante errores y acceso directo al formulario de cambio de contraseña. | Validación con cuentas existentes en el entorno de prueba. |
| Correo electrónico | Estado y dirección enmascarada del correo principal real. | Cambio de correo y confirmación de la dirección nueva. |
| Teléfono | Envío por Twilio Programmable Messaging, selección de canal, reenvío al destino guardado, vencimiento y consumo único. | Verificar entregas en Chile, fijar límites de gasto y completar el cambio de un teléfono ya verificado mediante verificación adicional. |
| Llaves de acceso | Pantalla unificada y registro WebAuthn con el dominio del navegador. | Verificar el registro en los dominios previstos e implementar acceso y confirmación de operaciones con la llave. |
| Authenticator | Configuración mediante QR local o clave manual. Acceso con contraseña más TOTP o código de respaldo, activable por entorno. | Prueba completa del acceso con una cuenta enrolada y una app de autenticación antes de habilitarlo de forma general. |
| Código QR | QR escaneable que abre la pantalla de vinculación de otro dispositivo con sesión activa. | Probar con dos dispositivos físicos y conectar aprobaciones a operaciones específicas. |
| Reconocimiento facial | Figura como no disponible y se excluye del progreso. | Fuera del alcance de este plan. |

## Configuración para un piloto de teléfono

- Por defecto, la API de desarrollo entrega el código en Mailpit y el resto de los entornos deja el envío inactivo. Las credenciales se suministran al proceso de Go fuera de Git.
- Para probar un envío real, configurar `PHONE_OTP_PROVIDER=twilio`, `PHONE_OTP_HMAC_KEY` (32 bytes aleatorios o más, en Base64), `TWILIO_ACCOUNT_SID`, `TWILIO_API_KEY_SID`, `TWILIO_API_KEY_SECRET` y `TWILIO_MESSAGING_SERVICE_SID`.
- Para ofrecer WhatsApp, configurar también `TWILIO_WHATSAPP_FROM` en E.164 y `TWILIO_WHATSAPP_CONTENT_SID` con la plantilla de autenticación aprobada que recibe el código en la variable `1`. El servidor anuncia únicamente los canales configurados.
- El proveedor devuelve la aceptación del envío. La prueba de aceptación operativa requiere recibir el mensaje y verificarlo desde la misma cuenta de prueba.
- `AUTH_TOTP_LOGIN_ENABLED=true` exige el código al entrar a una cuenta con Authenticator enrolado. Verificar el llavero TOTP y probar con códigos de respaldo antes de habilitar el interruptor.

## Comprobaciones de este corte

- Go pasó las pruebas de identidad y transporte HTTP, del adaptador Twilio, y de `cmd/api`. El proveedor se probó con transporte HTTP de prueba.
- Next pasó la compilación de producción, la comprobación de tipos y la generación de OpenAPI. La batería de frontend pasó 810 pruebas con una omitida por la suite.
- Chromium local pasó en 360 y 1280 px con respuestas BFF de prueba para recuperación de error, selección del canal y correo enmascarado. Esta comprobación cubre la interfaz.
- El esquema de PostgreSQL puede conservarse. Los códigos telefónicos emitidos antes de este corte deben solicitarse de nuevo. Los teléfonos ya verificados se conservan.
