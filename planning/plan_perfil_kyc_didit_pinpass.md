# Plan de perfil, KYC con Didit y Pinpass

## Alcance

Implementar el flujo en la rama `codex/marketplace/feature/contratos-1-modulo-contratos` y en su worktree de contratos. El desarrollo de KYC parte del código disponible en esta rama, sin integrar el trabajo de la rama KYC de Eric. La primera entrega cubre a personas naturales con cédula chilena. La cuenta puede crearse por correo y conserva su UUID durante todo el recorrido.

El recorrido de Mi perfil será **completar y guardar todos los datos obligatorios → verificar la identidad con Didit → habilitar las operaciones que exigen identidad vigente**. El navbar y las publicaciones mostrarán el nombre de usuario elegido. Los contratos autorizados usarán los datos legales comprobados y EspaciGo Pinpass permitirá consultar los datos sensibles propios durante 15 minutos.

## Fase 1 — Formulario de perfil e identidad de la cuenta

1. Integrar **Completa tus datos** y **Verifica tu identidad** en `/mi-cuenta/perfil`. Todos los campos del formulario de configuración son obligatorios, incluido el nombre de usuario. Permitir guardar un borrador parcial con un estado explícito de perfil incompleto. Para finalizar la configuración y habilitar KYC, Go debe confirmar que todos los campos requeridos son válidos.
2. Pedir un **nombre de usuario público** de libre elección dentro de las reglas de formato y unicidad. Mostrarlo en el navbar, en lugar del UID que se usa como respaldo, y en el perfil público, el catálogo y las publicaciones. Antes de elegirlo, mostrar **Completa tu perfil** en el navbar y una etiqueta neutra en publicaciones existentes. Evitar valores automáticos derivados del nombre legal, correo o RUN. Pedir a los perfiles existentes que lo elijan antes de su próxima publicación. Refrescar el navbar al guardar y al cambiar de sesión. El reemplazo del UID es visual y las relaciones de la cuenta siguen usando el UUID interno.
3. Validar cada campo al salir de él y al guardar. Next.js orientará a la persona y Go aplicará las mismas reglas al recibir la solicitud.

   | Dato obligatorio | Validación y presentación |
   | --- | --- |
   | Nombre de usuario | Mostrar las reglas de longitud y caracteres antes de escribir. Comprobar disponibilidad sin distinguir mayúsculas y minúsculas y confirmar unicidad al guardar. Diferenciar formato inválido de nombre ocupado. |
   | Nombres legales y apellidos | Exigir nombres y primer apellido. Admitir letras con tildes, espacios, guiones y apóstrofos. Para el segundo apellido, exigir el valor tal como figura en el documento o una selección explícita de **No tengo segundo apellido**. |
   | RUN | Aplicar formato chileno progresivo con puntos y guion, por ejemplo `12.345.678-5`. Admitir escritura y pegado de valores con o sin separadores. Normalizar la `k` a `K`, comprobar la estructura y el dígito verificador mediante módulo 11 y guardar el valor canónico. Conservar la posición del cursor al editar o borrar y evitar descartar caracteres inválidos en silencio. Los separadores se completan automáticamente. El dígito verificador lo ingresa la persona. |
   | Fecha de nacimiento | Exigir una fecha real y no futura. Mostrar el formato esperado y aplicar la regla de edad que defina el producto. |
   | Tipo de documento | Exigir elegir un tipo admitido por el flujo. En la primera entrega, validar que corresponda a la cédula chilena. |
   | Teléfono | Exigir un número válido con código de país y guardar en formato E.164. Si se requiere confirmar su titularidad, usar la verificación de teléfono existente. |

   Mostrar el correo real de la cuenta como dato enmascarado de solo lectura dentro de la configuración. Si la plantilla exige domicilio contractual, solicitarlo como requisito de esa etapa y registrar que fue declarado por la persona.

   Marcar **Obligatorio** en los campos y mostrar ejemplos junto a la etiqueta. Distinguir campo pendiente, error de formato y valor válido con texto e iconos, además del color. Mientras se escribe un dato incompleto, mostrar ayuda de formato. Al intentar finalizar, mantener los valores ingresados, resumir qué hay que corregir y llevar el foco al primer campo con error. Asociar cada mensaje al campo y comprobar la interacción en móvil, con teclado y con lector de pantalla. Reservar **Identidad verificada** para el resultado confirmado del KYC.
4. Guardar los datos de identidad en revisiones cifradas asociadas al UUID. Cada intento de Didit referencia la revisión que se envió. Mostrar los estados **Perfil incompleto**, **Listo para verificar**, **En curso**, **En revisión**, **Requiere corrección**, **Verificado**, **Vencido** y **Revocado**.
5. Tratar el RUN verificado como **identificador legal prioritario y único**, ligado de forma permanente al UUID. Las relaciones técnicas de espacios, reservas y contratos siguen usando el UUID. Antes de la primera verificación, la persona puede corregir el RUN declarado. Tras verificarlo, el formulario y la API impiden reemplazarlo. Las renovaciones verifican nuevamente el mismo RUN. Un cambio de otros datos legales abre una nueva revisión, mientras que el nombre de usuario y la foto se editan de forma independiente.
6. Representar el vínculo verificado con el RUN cifrado y una huella con clave del RUN normalizado para búsquedas de unicidad. Aplicar restricciones de unicidad por RUN y por UUID en PostgreSQL. Registrar la asociación y la aprobación en la misma transacción para que dos aprobaciones simultáneas del mismo RUN no puedan prosperar. Si existe una asociación con otro UUID, impedir la aprobación y ofrecer recuperación de la cuenta sin revelar datos de su titular. La expiración o revocación de KYC no libera el RUN para otra cuenta. Definir antes de producción cómo se aplica esta regla ante supresión de datos y qué tratamiento operativo reciben las incidencias de identidad.

**Criterio de cierre.** Desde una cuenta creada por correo, comprobar que omitir cualquier dato obligatorio impide finalizar el perfil y activar KYC, incluso por API directa. Verificar que el navbar y las publicaciones muestren el nombre de usuario elegido y que el navbar lo actualice al guardar. Probar cada error de campo y su corrección. Para RUN, cubrir escritura progresiva, pegado, edición intermedia, borrado, cuerpo de distinta longitud, dígito `K` o `0`, dígito incorrecto y caracteres inválidos. Probar que el RUN puede corregirse antes de verificar, queda bloqueado después y solo una cuenta puede reclamarlo, incluso en intentos simultáneos o cuando la verificación anterior esté vencida o revocada.

## Fase 2 — Activación y sincronización con Didit

1. Configurar un workflow propio en **Hotusoft Sandbox** para documento, prueba de vida y cotejo facial. En la revisión inicial, **Basic KYC** figuraba como borrador. Publicar el workflow de prueba y confirmar la cobertura de la cédula chilena, incluidos los campos donde Didit devuelve el RUN y el número de serie del documento.
2. Mantener el formulario en EspaciGo y usar la página alojada por Didit para la captura. Go creará la sesión con `expected_details` para enviar únicamente los datos necesarios para el cotejo y una referencia opaca que vincule la sesión con el intento local. El retorno del navegador mostrará el avance. La aprobación dependerá del resultado validado en Go. [API de creación de sesiones](https://docs.didit.me/sessions-api/create-session)
3. Recibir webhooks firmados y validar aplicación, entorno, workflow, sesión e intento. Deduplicar eventos, conciliar atrasos mediante la API de decisiones e impedir que un evento antiguo revierta una decisión posterior. [Webhooks de Didit](https://docs.didit.me/integration/webhooks)
4. Comparar el resultado de Didit con la revisión exacta del perfil. Si hay discrepancias relevantes o datos faltantes, pedir corrección o revisión. Si corresponde aprobar, comprobar el vínculo único con el RUN y guardar la vigencia y la evidencia mínima. La captura de fotos y biometría queda en Didit. [Discrepancias de identidad](https://docs.didit.me/core-technology/id-verification/warnings-id-verification#expected-details-mismatches)

**Criterio de cierre.** Demostrar aprobación, discrepancia, rechazo, expiración, reintento y webhook duplicado en Sandbox. Una respuesta tardía para una revisión anterior no verifica el perfil vigente. [Pruebas Sandbox](https://docs.didit.me/integration/sandbox-testing)

## Fase 3 — Guía y requisito de publicación

1. Agregar a la guía **Completa tu perfil** y **Verifica tu identidad** como pasos consecutivos. Abrir la sección correspondiente de Mi perfil y devolver a la persona al mismo borrador.
2. Usar el estado de identidad de Go en Mi perfil, en la guía y en las validaciones del servidor. Exigir perfil completo e identidad vigente vinculada a la revisión actual para publicar. Distinguir perfil incompleto, revisión pendiente y verificación vencida. El nombre público de la publicación viene del nombre de usuario.
3. Conectar la confirmación de reservas y los contratos a la misma fuente de identidad, con verificación de ambas partes donde hoy se exige. Retirar la casilla de autoaprobación como vía para obtener esos permisos y tratar las aprobaciones sintéticas existentes como datos de prueba. Aplicar también los requisitos propios de Mercado Pago y de la plantilla contractual.

**Criterio de cierre.** Bloquear una publicación por interfaz y por API directa cuando falle un requisito de identidad. Al completar el perfil y aprobar KYC, la guía actualiza el estado del mismo borrador.

## Fase 4 — Datos del contrato

1. Revisar la plantilla y definir el origen de cada campo. El nombre legal y el RUN vendrán de la revisión validada de cada parte. El domicilio contractual, cuando se exija, vendrá de lo declarado por la parte correspondiente. Espacio, fechas, montos y condiciones vendrán de la reserva y de la plantilla. Incorporar correo, teléfono o fecha de nacimiento al documento cuando cumplan una finalidad definida en la plantilla.
2. Aplicar la misma matriz en vista previa, propuesta, PDF y firma. Si falta un dato obligatorio, indicar qué debe completar la parte correspondiente antes de emitir la versión definitiva.
3. Fijar los datos utilizados al emitir la versión que se envía a firma. Las ediciones del perfil y las renovaciones de KYC se reflejarán en contratos futuros. Cada versión emitida conserva los datos con que se generó.
4. Mantener el enmascarado de la vista previa y los permisos de cada participante según el estado del contrato. El acceso temporal de Mi perfil se limita a datos propios.

**Criterio de cierre.** Generar un contrato con dos cuentas sintéticas, comprobar el origen de los campos y lo que puede ver cada parte, y confirmar que una edición posterior de perfil conserva el PDF emitido.

## Fase 5 — EspaciGo Pinpass y acceso por 15 minutos

1. Ofrecer la creación de **EspaciGo Pinpass** al guardar el perfil completo y desde Seguridad. Exigir autenticación reciente para crear, cambiar o recuperar la clave. Mostrar un control accesible de seis dígitos, con seis casillas visuales, confirmación y soporte de teclado y pegado.
2. Guardar un verificador con Argon2id, sal individual y secreto del servidor separado de la base. Limitar los intentos por cuenta y sesión, aplicar pausas progresivas y recuperar el PIN con un factor de la cuenta ya verificado.
3. Mostrar un ojito por cada dato sensible del titular, incluidos correo y RUN. El primer acceso pide el PIN. Go concede 15 minutos desde la validación, ligados a la sesión y a los datos propios. Dentro de esa ventana, la persona puede mostrar y volver a ocultar cada dato. Abrir otro campo no prolonga el plazo.
4. Entregar máscaras antes de autorizar y valores completos bajo demanda, con respuestas sin caché. Retirar los valores del cliente al vencer el plazo, cerrar sesión o cambiar de cuenta. Revocar el permiso al restablecer el PIN y auditar el acceso sin registrar la clave ni los valores revelados.

**Criterio de cierre.** Mostrar el correo real y el RUN del titular después de validar el PIN. Probar el vencimiento con la pestaña abierta, el cierre de sesión, el intento de leer datos de otro titular y el restablecimiento de la clave.

## Fase 6 — Pruebas, integración y activación

1. Probar con dos cuentas sintéticas en navegador el recorrido de completar y guardar el formulario obligatorio, ver el nombre de usuario en el navbar, verificar con Didit Sandbox, publicar con ese nombre, reservar, generar contrato y consultar datos propios. Incluir validación y corrección de campos, cambios de identidad, RUN duplicado, KYC vencido y fallos de PIN.
2. Versionar las migraciones y su clasificación, actualizar OpenAPI y Scalar, y ejecutar las comprobaciones de Go, Next.js y PostgreSQL. Integrar la entrega probada en `desarrollo-local` y ejecutar allí las comprobaciones de integración.
3. Validar workflow, credenciales y webhook del entorno `live`. Revisar los campos de la plantilla definitiva y el tratamiento y la retención de datos con Didit. Promover a `main` únicamente con autorización explícita y comprobaciones aprobadas.

## Estado de la implementación en la rama de contratos

- El perfil guarda una revisión cifrada por UUID, mantiene un borrador recuperable con la contraseña actual y permite cambiar el nombre público por separado. Go reserva el RUN verificado de forma transaccional. El correo y el RUN se consultan desde la interfaz privada de Pinpass.
- Publicación y reservas usan la revisión de identidad vigente. Los contratos nuevos toman el nombre legal y el RUN de esa revisión y conservan la instantánea que se usa para emitir la propuesta y el PDF. La plantilla actual fija el domicilio contractual en la comuna del inmueble. Para recoger un domicilio personal, primero debe definirse ese requisito en la plantilla.
- Pinpass solicita la contraseña actual para crearlo o restablecerlo. El permiso dura 15 minutos con un vencimiento preciso fijado por el servidor y queda ligado a la sesión. Se exige el perfil completo antes de crear el PIN.
- Las migraciones V000007 y V000008 incluyen permisos acotados. El comando `bash scripts/test-profile-identity-postgres.sh` prueba el guardado, el KYC sintético, la unicidad concurrente, la lectura de identidad y el vencimiento y la revocación de Pinpass en un PostgreSQL desechable.

### Activación de Didit

El borrador **EspaciGo — Identidad Chile (Contratos)** se creó y validó dentro de **Hotusoft (Sandbox)** con cédula chilena, prueba de vida y cotejo facial. Su identificador es `300ba68a-fec4-4df2-9649-a9616ad9cf33`. La revisión automática rechazó publicar porque la respuesta del workflow incluía todas las salidas, con imágenes y video. El MCP disponible permite editar el grafo y leer los campos de respuesta. La configuración de **Returned data** debe hacerse en la consola de Didit.

Antes de publicar, limitar la respuesta a los resultados necesarios para comprobar la decisión, el nombre legal, la fecha de nacimiento, el RUN, el país y tipo de documento, la vigencia, la prueba de vida y el cotejo facial. Excluir imágenes, videos, enlaces y bytes de evidencia biométrica. La aplicación contrasta el RUN con `personal_number` y acepta `identification_number` como campo alternativo del proveedor. [API de decisiones](https://docs.didit.me/sessions-api/retrieve-session) · [Webhooks de Didit](https://docs.didit.me/integration/webhooks)

Para una prueba integrada en Sandbox, configurar fuera de Git la clave de servicio, el identificador de la aplicación y del workflow, el entorno `sandbox` y el secreto del webhook mediante `PROFILE_KYC_DIDIT_API_KEY`, `PROFILE_KYC_DIDIT_APPLICATION_ID`, `PROFILE_KYC_DIDIT_WORKFLOW_ID`, `PROFILE_KYC_DIDIT_ENVIRONMENT` y `PROFILE_KYC_DIDIT_WEBHOOK_SECRET`. La URL del webhook es `/api/v1/webhooks/profile-identity` sobre el origen HTTPS del entorno. Configurar el keyring de declaraciones y el keyring HMAC de RUN. Configurar `PROFILE_PINPASS_PEPPER_HEX` con 32 bytes de entropía codificados en hexadecimal. Conservar versiones de las claves HMAC que todavía aparezcan en las reservas de RUN hasta completar su rotación.

La prueba con Hotusoft Sandbox, la integración a `desarrollo-local` y cualquier activación Live quedan pendientes hasta resolver la publicación del workflow y verificar el recorrido con la configuración real de Sandbox.
