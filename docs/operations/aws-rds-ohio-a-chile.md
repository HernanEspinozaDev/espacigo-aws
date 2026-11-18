# Trasladar EspaciGo de Ohio a Chile

## Punto de partida

El último estado verificado fue PostgreSQL V100 tanto en desarrollo local como en RDS de producción en Ohio (`us-east-2`). La equivalencia corresponde al esquema y al historial de migraciones. Cada entorno mantiene sus propios datos y credenciales. La unificación define `espacigo_migrator_login` como el login operativo y `espacigo_schema_owner` como el rol efectivo para el DDL ordinario. [Ciclo de entrega y propiedad](aws-production-deployment.md#regla-de-propiedad-desde-v101)

AWS aún anunciaba Chile como futura Región en la revisión del 18 de noviembre de 2026. Antes de programar un traslado, confirma que Chile esté disponible, que RDS ofrezca allí la versión de PostgreSQL y las extensiones requeridas, y que el destino tenga acceso a los demás servicios de EspaciGo. El proyecto actual tiene Ohio como Región seleccionada. Cambiar la dirección de contacto conserva esa Región. Revisa en AWS Settings si el destino puede ser un nuevo proyecto con Chile asignado o si la operación requiere la experiencia avanzada. [Infraestructura global de AWS](https://aws.amazon.com/about-aws/global-infrastructure/regions_az/) · [Región de los proyectos](https://docs.aws.amazon.com/accounts/latest/reference/project-regions.html) · [Endpoints de RDS](https://docs.aws.amazon.com/general/latest/gr/rds-service.html)

## Del cambio local a RDS

1. Si una nueva opción de pagos necesita tablas, columnas o restricciones, agrega el siguiente archivo a `db/migrations/` en `desarrollo-local`. Clasifica el cambio y registra su huella en `db/release-policy.json`. Aplica con `./scripts/local-dev.sh migrate` bajo el login dedicado y comprueba el historial con `./scripts/local-dev.sh migrate-plan`.
2. Promueve el mismo SQL y su clasificación a `main`. Los objetos ordinarios nuevos quedan bajo `espacigo_schema_owner`. Para modificar un objeto histórico con otro dueño, prepara una transferencia puntual que conserve sus permisos. El rol que usa la API sigue atendiendo las peticiones de la aplicación.
3. Cuando CodePipeline tenga conectadas `Migrate` y `Deploy`, la primera aplicará y verificará las migraciones compatibles del commit antes de actualizar la aplicación. El estado actual del piloto llega hasta `Publish`. [Despliegue en AWS](aws-production-deployment.md#inicializar-rds-y-aplicar-migraciones-en-codepipeline)

## Traslado de una base con datos

Para una base que pueda detener las escrituras durante una ventana medida en ensayo, usa una copia lógica completa con `pg_dump` y `pg_restore`. Esto conserva el esquema, las filas, las secuencias y el historial `schema_migrations`. Provisiona los roles de aplicación en el destino y conserva los propietarios y permisos verificados en el ensayo. Las contraseñas de conexión se preparan en el proyecto de destino. Si el volumen hace inviable la ventana, evalúa AWS DMS con carga inicial y replicación continua, incluyendo verificación de cambios finales y ajuste de secuencias antes de habilitar escrituras. [Importación en RDS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/PostgreSQL.Procedural.Importing.html) · [Migración continua de PostgreSQL con DMS](https://docs.aws.amazon.com/dms/latest/userguide/CHAP_Source.PostgreSQL.html) · [Roles de PostgreSQL y archivos de respaldo](https://www.postgresql.org/docs/18/app-pg-dumpall.html)

1. **Preparar Chile.** Crea RDS privado y cifrado con una versión compatible de PostgreSQL, las extensiones requeridas y un plan de respaldo. Separa red, secretos y permisos de Ohio y Chile. Prepara `espacigo_migrator_login → espacigo_schema_owner`, el login y rol de la API y los roles auxiliares que figuren en el inventario. Prueba la conectividad privada y TLS con el hostname y la CA de cada instancia. Mantén cerrados al tráfico el servicio y los procesos con efectos externos durante el ensayo. [TLS en RDS PostgreSQL](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/PostgreSQL.Concepts.General.SSL.html)
2. **Ensayar.** Fija el commit de la aplicación y el último registro de `schema_migrations` en Ohio. Restaura una copia de ensayo en una base vacía en Chile. Valida las huellas del historial con `dbmigrate -plan-only -require-current` usando el inventario de ese commit. Compara conteos y totales de usuarios, espacios, reservas, pagos y comprobantes, además de permisos, claves foráneas y secuencias. Mide el tiempo completo de exportación, transferencia y restauración. Restringe el acceso al respaldo y a la base de ensayo.
3. **Preparar archivos y dependencias.** Inventaría los objetos S3 referenciados por las filas, como documentos y evidencias. Cópialos al destino, conserva las claves de objeto cuando corresponda y verifica que la API pueda resolverlas. Prepara en Chile las versiones vigentes de las claves de cifrado y HMAC necesarias para leer o comprobar datos existentes. Configura las referencias regionales de la API y del migrador, y revisa los callbacks de identidad y pagos que estén habilitados. Evita que ambas instalaciones procesen el mismo webhook o ejecuten los mismos workers al mismo tiempo.
4. **Hacer el corte.** Pausa los despliegues y las escrituras de Ohio, incluidas las de workers y webhooks. Registra cómo retener o reconciliar los eventos externos que lleguen durante la ventana. Toma la exportación definitiva, verifica su huella y restaura en una base de destino vacía. Repite las validaciones del ensayo y comprueba en Chile el inicio de sesión y la lectura de un arriendo y su comprobante, usando datos de prueba autorizados. Cambia el destino de la API, `Migrate`, los workers y los callbacks. Habilita un solo escritor y comprueba que los eventos retenidos se procesen una vez.
5. **Cerrar y poder volver.** Conserva Ohio disponible para consulta durante el periodo de observación acordado. Mientras Chile siga sin escrituras, puedes volver a dirigir la aplicación a Ohio. Si Chile ya recibió escrituras, pausa ambos lados y reconcilia los datos nuevos y eventos externos antes de cualquier vuelta. Retira Ohio y sus respaldos conforme al periodo de conservación acordado, tras confirmar la operación en Chile.

### Comandos de ensayo para la copia lógica

Desde un ejecutor autorizado con conectividad privada a ambos destinos, prepara dos servicios libpq, `espacigo-ohio` y `espacigo-chile`, con host, base, login, `sslmode=verify-full` y CA correspondiente. Proporciona la contraseña vía un archivo de permisos `0600`. Usa un directorio de transferencia cifrado y privado que ya exista, con una base de destino previamente identificada como vacía. [Archivos de servicio libpq](https://www.postgresql.org/docs/18/libpq-pgservice.html) · [Restauración con pg_restore](https://www.postgresql.org/docs/18/app-pgrestore.html)

```bash
set -euo pipefail
umask 077
: "${PGTRANSFER_DIR:?Indica el directorio privado para este ensayo}"
transfer="$(mktemp -d "${PGTRANSFER_DIR}/espacigo.XXXXXX")"
archive="$transfer/espacigo.dump"

pg_dump --dbname='service=espacigo-ohio' --format=custom --no-password --file="$archive"
sha256sum "$archive" > "$archive.sha256"
sha256sum --check "$archive.sha256"
pg_restore --list "$archive" > "$transfer/contents.txt"
pg_restore --dbname='service=espacigo-chile' --no-password \
  --exit-on-error --single-transaction "$archive"
```

Si el administrador del destino carece de autoridad para restaurar alguna propiedad o ACL, ajusta y prueba ese caso con roles específicos durante el ensayo. La recuperación se considera lista cuando los dueños, permisos y datos esperados se verifican en el destino.
