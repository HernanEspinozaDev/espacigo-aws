# Despliegue continuo de EspaciGo en AWS

## Ramas y promoción

- **`desarrollo-local`** recibe el desarrollo y ejecuta las pruebas de Go, Next.js, migraciones e infraestructura. Usa AWS Emulator, PostgreSQL local y Mailpit con datos sintéticos. Sus workflows de calidad no obtienen credenciales de AWS.
- **`main`** representa producción. Recibe los cambios de aplicación y las migraciones específicamente aprobados para la entrega. Los scripts del emulador, los cambios exclusivos del stack local y los datos de demostración quedan en `desarrollo-local`. Los cambios de red, IAM, definiciones de tareas o scripts de despliegue se aprueban por separado. Sigue la [regla de promoción](../../AGENTS.md#alcance-de-una-promoción-a-main).
- [Producción AWS](../../.github/workflows/production.yml) repite los cuatro controles para el SHA integrado en `main`. Cuando `release-ready` aprueba las cuatro comprobaciones y el SHA sigue vigente en `main`, el workflow avanza `publicacion-aws` a ese mismo commit. AWS CodeConnections detecta el cambio y dispara CodePipeline automáticamente. Esa referencia la administra el workflow. Para una entrega habitual basta con integrar a `main`. La etapa OIDC de GitHub Actions queda desactivada mediante `AWS_DEPLOY_ENABLED=false`.

Antes de publicar, revisa `git diff --name-status origin/main HEAD` desde el candidato basado en `main` y valida su contenido. Usa fast-forward cuando incluya sólo los cambios autorizados. Cuando `desarrollo-local` tenga commits exclusivos del entorno local, promueve por separado los commits aprobados mediante `cherry-pick` y vuelve a validar el candidato. El PR sigue siendo opcional cuando se permite el pase directo.

## Red y permisos de producción

CodePipeline lee el SHA validado de `main` desde la referencia `publicacion-aws` mediante la conexión a GitHub y usa su rol de servicio para invocar las etapas de AWS. Al desplegar una versión, ECS inicia las tareas en las subredes y los security groups configurados en el servicio. El despliegue de una imagen reutiliza esa configuración. El script de despliegue previsto actualiza el servicio y especifica la red privada aparte para la tarea de verificación de la base. [Red de Fargate](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/fargate-task-networking.html)

El diseño de EspaciGo coloca Next.js y la API Go en la misma tarea privada. El balanceador HTTPS entrega tráfico únicamente a Next.js. Next.js llama a Go por `127.0.0.1:8080`. El security group de RDS permite PostgreSQL desde el grupo de las tareas. IAM controla las operaciones que la tarea puede hacer en servicios como S3, y la definición de tarea referencia los secretos de producción. [Seguridad de red de ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/security-network.html), [Parámetros de tareas Fargate](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html)

## Preflight de EspaciGo

El 17 de noviembre de 2026, AWS CLI verificó la identidad del proyecto `805068224315` mediante el perfil `espacigo-prod` en `us-east-2`. El plan gratuito figura activo. El inventario en esa Región devolvió una VPC predeterminada y cero clústeres ECS, repositorios ECR, instancias RDS y balanceadores. El rol `espacigo-github-oidc-deploy-role` aún no existe.

La SCP administrada denegó tanto el listado de proveedores OIDC como la consulta directa del proveedor de GitHub. AWS documenta la restricción sobre proveedores IAM también para el plan de pago de la experiencia administrada. El paso de federación de GitHub Actions queda bloqueado en este proyecto hasta confirmar un destino que permita administrar proveedores OIDC. Mantén `AWS_DEPLOY_ENABLED=false` mientras se resuelve esa condición. [Políticas administradas de AWS](https://docs.aws.amazon.com/accounts/latest/reference/scps-and-rcps-for-projects.html)

## Conexión para CodePipeline

AWS CLI confirmó que `espacigo-github` está `AVAILABLE` en `us-east-2` después de que el repositorio pasó a privado. La conexión usa `arn:aws:codeconnections:us-east-2:805068224315:connection/8f134a30-0c61-44ac-9711-2b2e877afb44`. El piloto `espacigo-main-pilot` leyó la rama privada `main` y recuperó el commit `a518f121353a9877243f1fdd661626c89329090d`.

CodeBuild terminó correctamente la validación del commit. Pasaron los cinco paquetes Go seleccionados y la comprobación de tipos de frontend. La suite de frontend registró 681 pruebas aprobadas, una omitida y cero fallos. La ejecución tiene el ID `a0c1d4ac-7156-448c-b811-0e1f3450a16b`.

El pipeline `espacigo-main-pilot` usa `Source → Validate → Publish → Migrate → Deploy`. La fuente usa `publicacion-aws` con `DetectChanges=true`. El [buildspec de validación](../../infra/codepipeline/validate-buildspec.yml) comprueba el SHA y ejecuta `go test ./...`, `go vet ./...`, `go build ./...`, typecheck y pruebas de frontend. El código fuente se guarda en S3 privado con cifrado. Los logs se guardan en `/aws/codebuild/espacigo-main-pilot` durante 14 días.

`Publish` usa el proyecto CodeBuild `espacigo-main-publish` y ejecuta el [buildspec de publicación](../../infra/codepipeline/publish-buildspec.yml) configurado en AWS. Construye las tres imágenes Linux AMD64 desde el mismo SHA validado, comprueba el inventario del migrador y publica etiquetas inmutables con el SHA y el número de build. El artefacto `ReleaseManifest` contiene `release.json` con los tres digests e `imagedefinitions.json` con las referencias de `api` y `web`. El rol de publicación accede a los tres repositorios, los artefactos del piloto y su grupo de logs. Los logs de publicación se conservan durante 14 días. [Publicación en ECR](https://docs.aws.amazon.com/AmazonECR/latest/userguide/image-push-iam.html)

La primera ejecución manual con las tres etapas terminó en `Succeeded`. Su ID es `0e9b4747-b93e-41db-899a-c23265f34fa4` y publicó la etiqueta `a518f121353a9877243f1fdd661626c89329090d-cb1` en los tres repositorios. Se descargó el manifiesto cifrado de S3 y se comprobó que sus tres digests coinciden con ECR.

Para recuperar una entrega ya aprobada, verifica primero que `publicacion-aws` indique el SHA esperado y que no haya una ejecución activa. Inicia de nuevo el pipeline con AWS CLI.

```bash
aws codepipeline start-pipeline-execution --profile espacigo-prod --region us-east-2 \
  --name espacigo-main-pilot
```

En una entrega habitual, consulta `release-ready` y el job **Entregar el commit aprobado a CodePipeline** en GitHub. En AWS verifica que la revisión de `Source` coincida con ese SHA, que `Migrate` termine antes de `Deploy`, y que ECS esté estable y `/api/ready` responda después de publicar. Si falla la CI, corrige el problema en `desarrollo-local` y promueve el arreglo. [Conexión de GitHub en CodePipeline](https://docs.aws.amazon.com/codepipeline/latest/userguide/action-reference-CodestarConnectionSource.html)

## Imágenes y clúster de producción

AWS confirmó el clúster `espacigo-production` activo con capacidad Fargate en `us-east-2`. Está vacío. Los tres repositorios ECR son privados, tienen etiquetas inmutables, cifrado AES-256 y escaneo básico al publicar.

La primera publicación usó el commit `a518f121353a9877243f1fdd661626c89329090d`, validado en el piloto. Las tres imágenes se construyeron para Linux AMD64 desde el árbol de ese commit.

| Repositorio | Digest verificado en ECR |
| --- | --- |
| `espacigo-api` | `sha256:6316f27f4853d2e563ba72c248dab697cd42091c86b7f99a65dfaf2dc46a6fb2` |
| `espacigo-web` | `sha256:a0eb25bb3f434a2d11259d335cd5f28da775d7d8920311fa20a319a7c970b9fb` |
| `espacigo-dbmigrate` | `sha256:88f6db1bda67f995c91301ae916bd4635f517b29c7c684d67d4a878f8d24de42` |

La imagen web anterior se conserva como evidencia de publicación. Para el primer despliegue, construye las imágenes del SHA que incluya Next.js 16.4.6 y el endpoint `/api/ready`. Ese endpoint devuelve 200 sólo si la API Go informa disponibilidad y devuelve 503 si la API falla o queda inaccesible.

## Primer arranque de la API

`APP_PROFILE=production` habilita el catálogo público, la disponibilidad de espacios y las rutas de salud. La demo simulada está apagada por defecto. Para la presentación, la tarea API de ECS puede usar `RENTAL_DEMO_ENABLED=1`, `RENTAL_DEMO_SPACE_ID`, `RENTAL_DEMO_HOST_EMAIL` y `RENTAL_DEMO_RENTER_EMAIL`. Usa dos cuentas de prueba activas y una publicación por hora del arrendador destinada a la demo. Mantén los valores fuera de Git y conserva los secretos existentes de la tarea. El API valida la propiedad, las cuentas y el esquema antes de abrir las rutas. La confirmación crea una ocupación en la publicación elegida y un invoice de prueba. Para desactivar la demo, retira las cuatro variables de la revisión ECS y actualiza el servicio.

| Configuración de arranque | Valor esperado |
| --- | --- |
| `HTTP_ADDR`, `FRONTEND_PUBLIC_ORIGIN` | API Go en `127.0.0.1:8080` para el BFF de la misma tarea y origen público HTTPS. |
| `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_NAME` | Nombre DNS de RDS, puerto PostgreSQL y base inicializada. |
| `DATABASE_USER`, `DATABASE_SET_ROLE`, `DATABASE_PASSWORD_FILE` | `espacigo_api_login`, `espacigo_runtime` y archivo de contraseña del login. |
| `DATABASE_SSLMODE`, `DATABASE_SSLROOTCERT` | `verify-full` y ruta al bundle CA de RDS. |
| `AWS_REGION`, `BLOB_STORE_BACKEND`, `S3_STORAGE_BUCKET`, `S3_MEDIA_BUCKET` | `us-east-2`, `s3`, bucket privado de evidencia y bucket privado de multimedia. El SDK obtiene credenciales del rol de la tarea. |
| `PUBLIC_CATALOG_CURSOR_KEY_FILE` | Archivo estable con permisos `0600`, ruta absoluta y 32 bytes codificados en hexadecimal. |

La tarea ECS debe suministrar el bundle CA y materializar los archivos de secretos antes del arranque. En una prueba local con PostgreSQL 18 y TLS, el arranque devolvió 200 en `/health/ready` y 503 al detener PostgreSQL. Las rutas de la demo devolvieron 404. La prueba con RDS, los buckets reales y el rol de la tarea queda pendiente.

## Red inicial de una zona

El despliegue inicial usa una sola zona para Fargate, la instancia RDS `multi_az=false` y los endpoints de interfaz que requiera el servicio. El plan de Ohio seleccionó `us-east-2a`. La segunda subred privada en `us-east-2b` existe exclusivamente para el requisito del grupo de subredes de RDS. Fargate usa únicamente la subred privada de la zona principal y el NLB usa únicamente la subred pública de esa misma zona. El ALB y la redundancia multizona quedan como arquitectura futura. [Requisito de subredes de RDS](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_CreateDBSubnetGroup.html), [NLB por zona](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/create-network-load-balancer.html)

Cuatro endpoints costarían aproximadamente USD 29,20 al mes si permanecieran activos 730 horas, antes de tráfico, créditos, RDS, Fargate y NLB. El umbral de alerta configurado es USD 28. Terraform validó 66 altas para la base privada y 70 al incluir el NLB. El NLB se incorpora al plan cuando se proporciona un certificado ACM emitido en `public_tls_certificate_arn`. [Configuración de endpoints](../../infra/terraform/environments/aws-main/private-endpoints.tf), [Catálogo de precios de Amazon VPC en Ohio](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonVPC/current/us-east-2/index.json)

El despliegue por etapas ya creó `espacigo-db-prod` en `us-east-2a` como PostgreSQL 18.6, `db.t4g.micro`, Single-AZ, privado y cifrado, con 20 GB gp3. El plan gratuito rechazó los siete días de respaldo y aceptó un día. AWS confirmó un respaldo automático cifrado disponible y un punto de restauración. La tarifa base de instancia y disco es aproximadamente USD 13,98 por 730 horas, antes de créditos, Secrets Manager y otros consumos. [Catálogo de precios de RDS en Ohio](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonRDS/current/us-east-2/index.json)

### Comprobar RDS con recursos temporales

Desde el repositorio, con AWS CLI v2, Python 3, `uv`, `curl` y `zip` instalados y una sesión vigente del perfil de producción:

```bash
python3 scripts/aws-rds-check.py --profile espacigo-prod
```

Durante la revisión del arranque en RDS, conserva el acceso privado hasta la instrucción explícita de cierre. Abre una vez con `--keep-open` y reutiliza el recibo que imprima el comando:

```bash
python3 scripts/aws-rds-check.py --profile espacigo-prod --keep-open
python3 scripts/aws-rds-check.py --profile espacigo-prod --reuse .local/aws-rds-check/<recibo>.json
```

`--keep-open` conserva recursos incluso si falla la comprobación. El recibo permite volver a invocar la Lambda por AWS CLI. Su consulta sólo lee el historial, los propietarios y las membresías de PostgreSQL. Usa `--cleanup` cuando el usuario autorice el cierre.

Usa un perfil de operador con acceso a los metadatos de RDS, al endpoint y al grupo de seguridad temporales, al paquete en S3 y a la Lambda y su rol, incluido `iam:PassRole` para ese rol. La Lambda obtiene el secreto con su propio rol.

El comando valida el proyecto y el destino privado Single-AZ. Crea un endpoint temporal de Secrets Manager en la zona de RDS, un rol IAM de Lambda que sólo puede leer el secreto gestionado de la instancia a través de ese endpoint, y una Lambda privada en la misma subred. Usa la URL específica del endpoint para que el DNS del resto de la VPC conserve su configuración. La Lambda lee el secreto dentro de AWS, verifica la CA y el hostname de RDS con `verify-full` y devuelve el estado de TLS, extensiones, roles y ledger desde una transacción de sólo lectura. La contraseña se queda en la Lambda. El resultado de la comprobación se guarda junto al recibo en `.local/aws-rds-check/`. [Endpoint de Secrets Manager](https://docs.aws.amazon.com/secretsmanager/latest/userguide/vpc-endpoint-overview.html) · [TLS en PostgreSQL de RDS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/PostgreSQL.Concepts.General.SSL.html)

En modo de comprobación puntual, el comando ejecuta la limpieza aunque falle. Retira el endpoint, la Lambda, el permiso de leer el secreto y todas las versiones de su paquete temporal en el bucket de artefactos. Mantiene los permisos de red del rol hasta que Lambda libera su ENI, lo que puede tardar unos 20 minutos. Luego elimina el grupo de seguridad temporal y el rol. Si se interrumpe el proceso o expira la sesión, renueva el perfil y retoma la limpieza con el recibo que imprimió:

```bash
python3 scripts/aws-rds-check.py --profile espacigo-prod --cleanup .local/aws-rds-check/<recibo>.json
```

Elimina sólo los recursos identificados por ese recibo. La instancia RDS, su endpoint DNS, el secreto gestionado y el bucket de artefactos forman parte del entorno. El endpoint temporal factura por hora mientras existe. En Terraform, `enable_persistent_secretsmanager_endpoint` queda en `false` durante el piloto de base. El endpoint permanente de la prueba anterior se retiró con un plan de Terraform que eliminó exactamente ese recurso. Activa la variable en la entrega de infraestructura que incorpore consumidores privados continuos de Secrets Manager. [Limpieza de ENI de Lambda](https://docs.aws.amazon.com/lambda/latest/dg/configuration-vpc.html) · [Precios de PrivateLink](https://aws.amazon.com/privatelink/pricing/)

## Inicializar RDS y aplicar migraciones en CodePipeline

Las entregas usan un único login operativo de PostgreSQL, `espacigo_migrator_login`. Al conectarse asume `espacigo_schema_owner` con `SET ROLE` para ejercer la autoridad sobre los objetos de esquema que corresponda a la migración. `espacigo_schema_owner` es un rol interno sin inicio de sesión ni credencial propia. El rol de funciones de verificación y los roles de la API conservan sus responsabilidades. Las migraciones históricas verifican esas identidades y los permisos de sus objetos.

En el arranque V80, `--configure-migrator <recibo>` prepara el login dedicado desde la Lambda abierta. Guarda su credencial en `espacigo/prod/database/migrator`, configura `SET ROLE` sin herencia automática ni delegación, y verifica una conexión separada por TLS. La Lambda vuelve al modo de diagnóstico al finalizar. El secreto dedicado pertenece al entorno de migraciones y seguirá disponible al cerrar el acceso de diagnóstico. Configura la tarea de migración para leer ese secreto cuando se conecte `Migrate`.

En `desarrollo-local`, `scripts/local-dev.sh bootstrap` también prepara el login. Para configurar sólo esa credencial, usa `scripts/local-dev.sh migrator-setup`. La contraseña se genera localmente en `.local/secrets/migrator_password` con permisos `0600`. Consulta el plan con `scripts/local-dev.sh migrate-plan` y aplica SQL con `scripts/local-dev.sh migrate`. Los dos comandos se conectan al PostgreSQL del contenedor local, con el mismo login y rol efectivo que AWS.

RDS confirmó la sesión como `espacigo_migrator_login` y el rol efectivo como `espacigo_schema_owner`. La comprobación de RDS permanece disponible con el recibo activo. La base local y RDS tienen el historial verificado hasta V100.

1. **Primer arranque.** Empieza con una base RDS nueva y ensaya la secuencia V1–V100 en PostgreSQL desechable. Para el primer tramo, con Go y Git instalados y `origin/main` actualizado, ejecuta el mismo comando con `--bootstrap-v80`. El paquete toma los 80 SQL canónicos de `origin/main` y el migrador valida la huella revisada antes de conectarse. Crea los roles PostgreSQL iniciales y completa el ledger exacto bajo lock, con lectura independiente después de la ejecución. Un reintento continúa desde un prefijo válido. Al llegar a V80, revisa la transición de propietarios y los permisos de V81 antes de continuar con las autoridades de `db/release-policy.json`. V99 conserva su procedimiento dedicado. Verifica los checksums con `dbmigrate -plan-only -require-current` antes de finalizar la carga.
2. **Datos de referencia.** Las migraciones existentes cargan nueve categorías, sus perfiles y filtros, 16 regiones y 345 comunas del catálogo versionado. Ejecuta [la finalización de bootstrap](../../db/bootstrap/aws-main/finalize_reference_only.sql) una sola vez antes de iniciar el servicio. El script rechaza datos inesperados, elimina sólo cinco términos sintéticos y de evaluación de la carga histórica y verifica el catálogo. La base queda sin usuarios, espacios, reservas, pagos, documentos ni datos de demo.
3. **Entregas posteriores.** Crea y prueba las migraciones en `desarrollo-local`. Promueve los archivos SQL y sus entradas revisadas en `db/release-policy.json` junto con la aplicación. Mantén inmutables las migraciones aplicadas. El [buildspec de Migrate](../../infra/codepipeline/migrate-buildspec.yml) usa `SourceArchive` como `PrimarySource` y `ReleaseManifest` como segundo artefacto. Pasa `EXPECTED_SOURCE_SHA` desde la conexión GitHub y configura el proyecto AWS, Región, clúster, revisión base del migrador y red con una subred privada.
4. **Ejecución.** La etapa inicia una tarea Fargate del digest de migrador publicado en esa entrega. La tarea corre `dbmigrate -aws-release` con el login dedicado, hostname exacto en `AWS_RDS_HOST`, PostgreSQL `verify-full` y la CA regional incluida en la imagen. ECS inyecta sólo el campo `password` del secreto dedicado en `DATABASE_ECS_PASSWORD`. Los comandos administrativos conservan la entrada por archivo privado. El comando toma el lock del ledger y comprueba toda la tanda antes de aplicar SQL. Ejecuta automáticamente sólo expansiones aprobadas con checksum exacto y verifica el ledger al terminar. Si aparece una migración pendiente de revisión, el pipeline detiene `Deploy` y se atiende por el procedimiento de esa migración. El rol de CodeBuild queda limitado a registrar, iniciar y consultar la familia de tarea de migración, con `iam:PassRole` sólo para sus roles ECS.

`TMPDIR=/tmp bash scripts/test-migration-runner-postgres.sh aws-bootstrap` ensaya los SQL canónicos desde cero en PostgreSQL 18. Para V99 usa TLS con un certificado de prueba y un relay local hacia PostgreSQL desechable. Comprueba el rechazo de un hostname de certificado incorrecto, propietarios incompatibles, un OID de base distinto y un segundo intento de aplicar la misma migración. Después ejecuta V100, valida el ledger completo y aplica el SQL de finalización. El rol de la API comprueba la lectura de las 9 categorías, 16 regiones y 345 comunas.

Para preparar un futuro traslado del esquema y los datos de producción, sigue el [procedimiento de Ohio a Chile](aws-rds-ohio-a-chile.md).

### Regla de propiedad desde V101

PostgreSQL asigna los objetos nuevos al rol efectivo que ejecuta el SQL. Las primeras migraciones se hicieron con un administrador para inicializar roles y extensiones; luego se incorporó el migrador dedicado y algunas funciones y tablas quedaron vinculadas a dueños específicos. Desde V101, el único login de migraciones es `espacigo_migrator_login` y el dueño de los objetos ordinarios que crea ese login es `espacigo_schema_owner`.

Ejecuta [la habilitación de DDL](../../db/bootstrap/migration-owner.sql) una vez después de validar V100 en cada entorno. El SQL comprueba la huella de V100, la membresía y la propiedad de `ocupacion` y del historial. Concede `CREATE` sólo al propietario efectivo. En local se ejecuta con `scripts/local-dev.sh migrator-enable-ddl`. Si una migración nueva necesita cambiar un objeto histórico, verifica su dueño en los dos entornos y prepara una transferencia puntual con revisión de ACL, dependencias y funciones con autoridad propia. Los SQL históricos conservan su checksum.

El ciclo normal empieza con `migrate-plan` y `migrate` en `desarrollo-local`. La promoción lleva a `main` el mismo SQL y su clasificación. En AWS, `Migrate` valida el historial y aplica los cambios compatibles antes de `Deploy`. Se reproduce el cambio de esquema, dejando los datos y credenciales en cada entorno.

### Ejecutar V99 en RDS

V99 mantiene su ejecución manual y el mismo SQL histórico. `e1migrate` admite un bloque `RDS` en el JSON de operación fijado por SHA-256. Incluye `CAFile` con la ruta absoluta al bundle CA regional, `CASHA256` con su huella revisada y `PasswordFile` con la ruta absoluta al archivo privado (`0400` o `0600`) del login de migración. El destino aprobado debe ser el endpoint real de la instancia en `us-east-2` por el puerto 5432. El cliente verifica CA, hostname y TLS 1.2 o superior. [TLS de RDS PostgreSQL](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/PostgreSQL.Concepts.General.SSL.html)

1. Desde un ejecutor con acceso privado a RDS, revisa el OID de `espacigo_prod`, el identificador del clúster PostgreSQL, la versión del motor y el estado exacto de V1 a V98. Verifica que `espacigo_migrator_login` pueda asumir `espacigo_schema_owner`, que este rol pueda consultar `pg_control_system()` y que ya sea propietario de `public.ocupacion` y `public.schema_migrations`. El ejecutor exige esa propiedad y compara los perfiles de autoridad antes y después.
2. Prepara el manifiesto del commit y un archivo de respaldo validable con `pg_restore`, asociados al destino y con hashes revisados. Guarda el JSON de la operación con permisos `0600`; contiene las rutas a los archivos. Ejecuta `inspect` y `preflight` con la huella del JSON antes de usar `apply` con confirmación exacta `database:system-identifier:99`.
3. Si la confirmación del commit queda incierta, usa `postcheck` para leer el ledger y el catálogo antes de cualquier otra acción. Tras confirmar V99, ejecuta la preparación de V100 y cierra el permiso temporal de creación que requiere V100 para reemplazar el trigger.

El ensayo local prepara esos propietarios en la copia desechable. La operación en RDS completó el respaldo, el preflight y la verificación independiente de V99 antes de aplicar V100. El administrador de RDS tiene permisos distintos a los de PostgreSQL local. [Rol de administración de RDS](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Appendix.PostgreSQL.CommonDBATasks.Roles.rds_superuser.html)

El 18 de noviembre de 2026 se aplicaron V1–V80 en `espacigo-db-prod` por TLS 1.3 usando el inventario de `a518f121353a9877243f1fdd661626c89329090d`. La comprobación previa encontró la base vacía. El migrador validó las huellas y la lectura posterior confirmó 80 filas en el ledger, con V80 como última versión. Al terminar se confirmó la retirada del endpoint, la Lambda, su ENI, el rol IAM, el grupo temporal y el paquete. RDS siguió disponible en su subred privada.

El 18 de noviembre de 2026 se completó el historial de RDS hasta V98 con el acceso privado que quedó abierto. El operador ejecutó los tramos `--advance-v83`, `--advance-v93` y `--advance-v98` sobre el mismo recibo. Cada tramo compara el historial y el inventario fijado de `origin/main` y aplica el SQL bajo el login dedicado.

- V81 usa el esquema privado propiedad de `espacigo_document_verification_fn`, la membresía `SET ROLE` hacia ese rol, acceso por columna a `usuario` y `espacio`, y `SELECT` e `INSERT` sobre el historial para el migrador. V82 y V83 requieren una ventana de `CREATE`, propiedad temporal del trigger y permiso delegado de actualización de la columna `id` de la tabla de revisores.
- V84–V98 usan permisos temporales de `CREATE` y `REFERENCES(id)` según la versión. El validador acepta `espacigo_admin` como propietario de la base, `usuario` y el historial sólo en `espacigo_prod`, conservando los demás controles. V94 asignó la tabla `outbox_evento_local` a `postgres`, creado en RDS como rol restringido sin inicio de sesión, exclusivamente para conservar el requisito histórico de propiedad. El rol efectivo del migrador quedó sin `CREATE` permanente en `public`.
- Antes de V99 se guardó un respaldo lógico de RDS V98 y se validó con `pg_restore`. Se preparó la propiedad de `usuario`, `espacio`, `ocupacion` y `schema_migrations` con [la operación acotada](../../db/bootstrap/aws-main/prepare_v99_ownership.sql). Esas cuatro tablas quedaron bajo `espacigo_schema_owner`, igual que en local. El ejecutor dedicado verificó los perfiles de RDS, aplicó V99 y confirmó el commit desde una conexión independiente. Verificó el inventario completo del commit y sólo ejecutó el prefijo autorizado hasta V99.

- Se ejecutó `--advance-v100` sobre el mismo recibo para aplicar V100, finalizar los datos iniciales y habilitar el DDL del dueño común. RDS confirmó 100 entradas de migración, 9 categorías, 16 regiones y 345 comunas. El respaldo y las evidencias de la operación se conservan en `.local/aws-rds-check/v99/`.

El piloto continúa hasta `Publish`, con cero servicios ECS en el clúster. La Lambda de comprobación, su endpoint y sus permisos quedan disponibles hasta la instrucción de cierre.

### Activar Migrate en AWS

V102 admite S3 en los registros de imágenes. Si `media_variant` y `media_variant_intent` todavía pertenecen al administrador histórico, ejecuta [la preparación de propiedad de V102](../../db/bootstrap/aws-main/prepare_v102_media_ownership.sql) desde la conexión privada de diagnóstico antes de publicar. La preparación transfiere únicamente esas tablas, compara sus permisos de tabla y columna y deja la aplicación de V102 al migrador del pipeline.

El binario y la CA extraídos de la imagen candidata verificaron el historial V100 en el RDS real desde la Lambda privada, usando la credencial dedicada con el mismo formato que inyectará ECS. La ejecución devolvió cero cambios pendientes. La fase probó el migrador y la base. La prueba de arranque completa en Fargate se hace después de publicar la imagen aprobada.

[La definición de tarea](../../infra/terraform/environments/aws-main/migrator.tf) y [el proyecto CodeBuild](../../infra/terraform/environments/aws-main/migrate-codebuild.tf) están preparados con aprovisionamiento opcional. El plan consultó el estado real en Ohio y sólo propuso altas para la tarea, CodeBuild, políticas, logs y red privada. Para comprobar los recursos, utilizó como referencia el digest disponible del piloto. Usa el digest de la nueva imagen aprobada para el plan que se aplicará.

1. Promueve a `main` el código de migración y publicación revisado y publica la imagen de ese SHA. El digest debe pertenecer al repositorio privado `espacigo-dbmigrate` y su manifiesto debe identificar el mismo SHA que devolvió la conexión de GitHub.
2. Genera y revisa el plan de infraestructura con `migrator_seed_image` apuntando a ese digest y `enable_persistent_secretsmanager_endpoint=true`. Incluye los cuatro endpoints de interfaz en la subred privada principal y el gateway de S3. La tarea usa el grupo de seguridad de ECS y arranca sin IP pública. Su rol de ejecución sólo recupera el secreto dedicado por el endpoint permanente. El endpoint de comprobación sigue separado. [Red privada de ECR](https://docs.aws.amazon.com/AmazonECR/latest/userguide/vpc-endpoints.html) · [Inyección del secreto en Fargate](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/secrets-envvar-secrets-manager.html)
3. Con el proyecto CodeBuild y la tarea aprovisionados, agrega [la etapa Migrate](../../infra/codepipeline/migrate-stage.json) después de `Publish` en la configuración actual del pipeline. Usa `SourceArchive` como fuente principal, `ReleaseManifest` para el digest y `#{SourceVariables.CommitId}` como SHA esperado. Ejecuta manualmente el primer ciclo y verifica que la tarea Fargate termine con código 0 y confirme el historial. [Acción CodeBuild en CodePipeline](https://docs.aws.amazon.com/codepipeline/latest/userguide/action-reference-CodeBuild.html)
4. Prepara la tarea y el servicio ECS de la aplicación, las credenciales de la API, el balanceador y un certificado ACM emitido para el dominio. La solicitud ACM pública no exportable de `espacigo.testingpage.store` quedó en `PENDING_VALIDATION` en Ohio. El DNS autoritativo está en Cloudflare. Agrega el CNAME indicado por `aws acm describe-certificate` en modo DNS only y confirma `ISSUED` antes de usar el ARN en `public_tls_certificate_arn`. [Validación DNS de ACM](https://docs.aws.amazon.com/acm/latest/userguide/dns-validation.html) Tras una comprobación de salud y un primer despliegue manual exitoso, conecta `Deploy` y activa el disparo por cambios en la referencia `publicacion-aws` que actualiza la CI de `main`.

## Preparar el destino inicial

El estado de `aws-main` usa el bucket privado `espacigo-terraform-state-805068224315-us-east-2`. El bucket ya existe en Ohio con versionado, cifrado SSE-S3, bloqueo de acceso público y HTTPS obligatorio. Terraform usa el archivo de bloqueo de S3 y valida el identificador del proyecto. Se comprobó la adquisición y liberación del bloqueo con un `terraform plan`. La CI sigue usando `terraform init -backend=false` para validar el código sin credenciales de producción. [Backend S3 de Terraform](https://developer.hashicorp.com/terraform/language/backend/s3)

1. En AWS Settings, confirma el proyecto, la Región seleccionada, el plan y los límites de gasto. Comprueba acceso con un perfil dedicado. Las instrucciones siguientes usan AWS CLI v2 y variables del proyecto confirmado. [Inicio de sesión con AWS CLI](https://docs.aws.amazon.com/signin/latest/userguide/command-line-sign-in.html)
2. Completa `infra/terraform/environments/aws-main` antes de aplicar. El Terraform prepara RDS PostgreSQL 18.6 con la contraseña de administración gestionada por Secrets Manager. Se confirmó la disponibilidad regional de `db.t4g.micro` con gp3 y cifrado. AWS lista PostGIS para RDS 18.6. Quedan pendientes el certificado TLS para el NLB, las definiciones de tarea, el servicio ECS y sus roles. Usa el clúster y los repositorios ECR ya provisionados. [Extensiones de RDS PostgreSQL](https://docs.aws.amazon.com/AmazonRDS/latest/PostgreSQLReleaseNotes/postgresql-extensions.html)
3. Prueba el catálogo de `APP_PROFILE=production` con RDS, S3 y los secretos reales desde una tarea privada. Prueba el BFF de Next.js con la API Go por loopback dentro de la misma tarea ECS. Expón únicamente Next.js al balanceador. Configura el target group y la comprobación HTTPS del despliegue para consultar `/api/ready`.
4. Prepara la base inicial siguiendo el procedimiento anterior. Conserva los roles de ejecución, consulta y migración separados. Ensaya el aprovisionamiento, backup y restauración en una base desechable.
5. Para un destino alternativo que permita OIDC, crea el proveedor de GitHub con audiencia `sts.amazonaws.com` y el rol de entrega con el sujeto exacto `repo:HernanEspinozaDev/espacigo-aws:environment:produccion`. Limita su acceso a los tres ECR, el servicio ECS y las familias de tareas previstas. Restringe `iam:PassRole` a los roles de tarea y ejecución de ECS. Las tareas referencian secretos de AWS y el rol de CI carece de lectura de secretos de aplicación. [GitHub OIDC en AWS](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws), [Roles de tareas ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html)
6. Crea el servicio ECS inicial con una tarea privada de dos contenedores llamados `api` y `web`, y una tarea independiente con un contenedor `dbmigrate`. Configura health checks del contenedor y del target group para un rollout rolling con circuit breaker. La tarea de migración usa la subred privada principal, sin IP pública. [Task definitions de ECS](https://docs.aws.amazon.com/cli/latest/reference/ecs/register-task-definition.html)

## Comprobaciones iniciales con AWS CLI

Usa una sesión separada de la del emulador y la Región confirmada en AWS Settings.

```bash
export AWS_REGION="<region-seleccionada>"
export AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true
unset AWS_ENDPOINT_URL AWS_ENDPOINT_URL_STS AWS_ENDPOINT_URL_ECS AWS_ENDPOINT_URL_ECR AWS_CA_BUNDLE
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_SECURITY_TOKEN

aws login --profile espacigo-prod --region "$AWS_REGION"
aws sts get-caller-identity --profile espacigo-prod --region "$AWS_REGION"
aws freetier get-account-plan-state --profile espacigo-prod --region "$AWS_REGION"
aws ecr describe-repositories --profile espacigo-prod --region "$AWS_REGION"
aws ecs describe-services --profile espacigo-prod --region "$AWS_REGION" \
  --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE"
```

Comprueba el identificador devuelto por STS y el servicio ECS antes de habilitar el workflow. Aplica cambios de infraestructura mediante el plan IaC revisado en la Región seleccionada. Configura las variables del entorno de GitHub `produccion` con los valores reales del destino.

| Variables | Uso |
| --- | --- |
| `AWS_REGION`, `AWS_ACCOUNT_ID`, `AWS_DEPLOY_ROLE_ARN` | Alcance regional e identidad OIDC de producción. |
| `ECS_CLUSTER`, `ECS_SERVICE`, `ECS_MIGRATOR_TASK` | Clúster, servicio de aplicación y revisión aprobada de la tarea independiente. |
| `ECS_MIGRATOR_NETWORK` | JSON de configuración `awsvpcConfiguration`, subredes privadas, security groups y `assignPublicIp=DISABLED`. |
| `ECR_API_REPOSITORY`, `ECR_WEB_REPOSITORY`, `ECR_MIGRATOR_REPOSITORY` | Nombres de los tres repositorios ECR inmutables. |
| `PRODUCTION_HEALTH_URL` | URL HTTPS que confirma disponibilidad de Next.js y Go. |

## Entrega y rollback mediante el workflow OIDC

1. Tras pasar la CI en `main`, GitHub Actions publica las imágenes de API, web y migrador en ECR con el SHA y el intento del workflow. Registra revisiones ECS con referencias `@sha256:…`. Comprueba la identidad con AWS STS y conserva los digests y ambas revisiones ECS en el log de la ejecución. [ECR en AWS CLI](https://docs.aws.amazon.com/cli/latest/reference/ecr/)
2. Ejecuta `aws ecs run-task` con la imagen migradora de ese mismo SHA y `-plan-only -require-current`. Verifica el código de salida tras `aws ecs wait tasks-stopped`. Si hay migraciones pendientes, revisa la tarea y el ledger, prepara un snapshot y ejecuta la migración autorizada con el rol dedicado en la red privada. Después vuelve a ejecutar el workflow de `main`. Usa migraciones compatibles con ambas versiones para el rollback de aplicación. [ECS run-task](https://docs.aws.amazon.com/cli/latest/reference/ecs/run-task.html), [ECS tasks-stopped](https://docs.aws.amazon.com/cli/latest/reference/ecs/wait/tasks-stopped.html)
3. Actualiza el servicio con `aws ecs update-service`, espera a que ECS reporte estabilidad y comprueba que la revisión activa es la nueva y que la URL HTTPS responde. Ante un fallo, el circuit breaker de ECS aplica rollback de rollout. Si falla la comprobación externa y el servicio sigue en la revisión nueva, el script restaura la anterior. Revisa eventos del servicio y logs. La base se recupera con el procedimiento de restauración o una migración correctiva si hiciera falta. [ECS update-service](https://docs.aws.amazon.com/cli/latest/reference/ecs/update-service.html)

En un destino compatible con OIDC, deja ese workflow de entrega deshabilitado hasta comprobar infraestructura, perfil de aplicación, ledger y acceso OIDC. Después activa `AWS_DEPLOY_ENABLED=true` en el repositorio y ejecuta manualmente **Producción AWS** sobre `main` para registrar la primera entrega validada. Las integraciones posteriores disparan el despliegue automáticamente.

## Transición planificada de correo (`espacigo.cl` con Amazon SES / pago por uso)

La salida inicial de autenticación utiliza Zoho SMTP (`contacto@hotusoft.com`) a través de una instancia NAT `t4g.micro` cubierta por el Free Tier. Cuando se incorpore el dominio definitivo `espacigo.cl` (`contacto@espacigo.cl`), la entrega de correos de verificación y recuperación pasará a un esquema **100% por uso** sin instancias EC2 encendidas 24/7:

1. Verificar el dominio `espacigo.cl` en **Amazon SES** (registros DKIM/SPF/DMARC en el DNS del dominio) y solicitar salida de sandbox de SES para el remitente `contacto@espacigo.cl`.
2. Reemplazar el envío SMTP directo desde Fargate por un adaptador o invocación asíncrona vía **AWS Lambda** / **Amazon SES** (cobrando únicamente por correo enviado e invocación ejecutada, sin costo fijo mensual).
3. Retirar la instancia NAT `t4g.micro` (`infra/terraform/environments/aws-main/nat.tf`) una vez que Fargate no requiera salida directa a `smtp.zoho.com`.
