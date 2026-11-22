# Mejora de la creación y publicación de espacios

EspaciGo tendrá un asistente de seis pasos, un editor privado por pestañas y una ficha pública de una sola página. El objetivo es que el anfitrión entienda cómo completar y publicar un espacio, y que el visitante pueda revisar lo esencial y reservar.

## Fase 1. Reorganizar el asistente

- [x] **Tipo y resumen.** Mantener categoría, tipo de publicación y título. La descripción será opcional, sin mínimo. Alinear el asistente, el editor, Go, OpenAPI y PostgreSQL para permitir crear, editar y publicar con una descripción vacía o breve.
- [x] **Ubicación.** Separar dirección, región, comuna y mapa. Permitir buscar la dirección, ajustar el pin y confirmar qué se hará público. Conservar la ubicación al cambiar de paso y guardarla con el borrador.
- [x] **Atributos del espacio.** Dar a capacidad, superficie y los campos de la categoría un paso propio. Identificar los campos requeridos y conservar las respuestas al volver.
- [ ] Refinar la agrupación de atributos en equipamiento y servicios cuando el perfil de cada categoría los distinga.
- [x] **Tarifas y reglas.** Mantener la edición de precios y mostrar la unidad de cobro junto al monto. Presentar la política de cancelación aparte de las reglas.
- [x] **Disponibilidad y horarios.** Mostrar los controles según la modalidad de arriendo y ofrecer una vista previa del horario.
- [x] **Confirmación.** Mostrar un resumen con una acción **Editar** por bloque y permitir regresar a la confirmación al terminar el ajuste.

## Fase 2. Horarios y política de cancelación

- [x] Explicar cómo activar días y configurar tramos. Ofrecer estados **Cerrado**, **Agregar tramo** y **Todo el día** cuando correspondan. Permitir seleccionar horas en el panel o introducir **Desde** y **Hasta**.
- [x] Mostrar un resumen legible como **Lunes a viernes · 09:00–18:00**. Distinguir el horario habitual de los bloqueos y fechas especiales. Adaptar los controles al arriendo por hora, día o mes.
- [x] Guardar una política versionada por espacio y usarla en el editor, la ficha, la cotización y el contrato. Los cambios aplican a cotizaciones nuevas. Las cotizaciones existentes y los contratos conservan sus condiciones. Revisar la transición de publicaciones que tienen la política mezclada en el texto de reglas y mantener intactos los documentos históricos.

## Fase 3. Editor privado y fotografías

- [x] Organizar el editor en **Detalles**, **Ubicación**, **Tarifas y reglas**, **Calendario y horarios**, **Fotografías** y **Contrato**.
- [x] Reunir información general y atributos en Detalles. Colocar dirección y mapa en Ubicación. Al cambiar la dirección, pedir que se revise el pin para volver a confirmarlo.
- [x] Reunir plantilla, vista previa y versión publicada bajo Contrato. Incorporar estado de guardado y proteger los cambios pendientes al navegar entre pestañas.
- [x] Diferenciar agregar fotos de ordenarlas. Incorporar zona para soltar archivos, selector, asa de arrastre, posición de destino y botones **Mover antes** y **Mover después**. Identificar la portada con texto e icono. Conservar orden y portada al recargar y dar feedback de guardado o reintento.

## Fase 4. Ficha pública y tarjeta del Marketplace

- [x] Dar prioridad a fotos, título, precio por unidad y acción de reserva en una sola página pública. Usar enlaces internos a **Detalles y atributos**, **Disponibilidad**, **Ubicación** y **Reglas y cancelación**. Colocar el mapa en Ubicación y mantener la revisión del contrato durante la reserva.
- [x] Dar prioridad a portada, título, comuna y precio por unidad en la tarjeta. Mostrar hasta dos atributos útiles según la categoría y mantener la composición con datos o fotos faltantes.
- [x] Aplicar el acento de EspaciGo a las acciones principales y usar fondos suaves para agrupar secciones. Identificar estados con texto e iconos, mantener contraste y foco visible y usar controles cómodos al tacto.

## Fase 5. Integración y pruebas

- [x] Coordinar los contratos de datos de descripción, ubicación y cancelación antes de integrar las vistas. El asistente, el editor y la ficha pública se pueden trabajar en paralelo con GPT‑6 Luna en razonamiento medio. El orquestador integra migraciones, API y verificaciones compartidas.
- [ ] Probar en celular, tablet y escritorio el recorrido **crear con descripción vacía → confirmar pin → configurar horarios → ordenar fotos → publicar → encontrar la tarjeta en Explorar**.
- [ ] Comprobar control con teclado, feedback de errores, guardado, orden y portada después de recargar, cambios de dirección y coincidencia de la política entre ficha, cotización y contrato.
- [ ] Registrar las pruebas realizadas y los casos pendientes dentro de la rama de Contratos. Aplicar el flujo de integración y promoción indicado para el repositorio.


## Evidencia de desarrollo

- V131 permite descripciones breves o vacías y conserva las existentes. V132 añade la versión de cancelación al espacio y el texto congelado a la cotización. Ambas migraciones se probaron con el rol de esquema en PostgreSQL aislado.
- Una prueba de integración con PostgreSQL confirmó que la propuesta lee la política guardada en la cotización y conserva su contenido. Las publicaciones antiguas pueden elegir una política desde Tarifas y reglas. Las cotizaciones anteriores sin versión conservan el tratamiento de compatibilidad.
- Pasaron 809 pruebas automatizadas de frontend, TypeScript, la comprobación de OpenAPI y ESLint sin errores. En Go se identificó un fallo preexistente en el cálculo de liquidaciones de la rama base. La suite pasó al excluir ese caso y los paquetes modificados quedaron cubiertos.
- En Chromium con fixtures locales pasaron siete casos del asistente, uno del editor privado, seis de galería pública y tarjetas, cinco de mapa y once de contratos y reservas. El caso diario envía descripción vacía. La galería verifica contraste, estrategias de recuperación cuando una imagen falla y navegación a la fotografía siguiente. El editor verifica cambios sin guardar al alternar pestañas, ubicación, arrastre, portada y recuperación ante conflictos de versión.
- La verificación de navegador usa fixtures locales. Queda pendiente aplicar V131 y V132 en el entorno montado y completar allí el recorrido integrado con cuentas sintéticas.
