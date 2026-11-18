# Despliegue continuo de EspaciGo en AWS

## Ramas y promoción

- **`desarrollo-local`** recibe el desarrollo y ejecuta las pruebas de Go, Next.js, migraciones e infraestructura. Usa AWS Emulator, PostgreSQL local y Mailpit con datos sintéticos. Sus workflows de calidad no obtienen credenciales de AWS.
- **`main`** representa producción. Los PR deben venir de `desarrollo-local` del mismo repositorio y pasar `release-ready`. Usa merge commit y, después de integrar, actualiza `desarrollo-local` con el último `main`. Conserva ambas ramas.
- [Producción AWS](../../.github/workflows/production.yml) repite los cuatro controles para el SHA integrado en `main`. Cuando se habilite `AWS_DEPLOY_ENABLED=true` en las variables del repositorio, cada SHA aprobado puede desplegar. El entorno GitHub `produccion` restringe el despliegue a `main`. Sólo ese job solicita un token OIDC.

## Preparar el destino inicial

1. En AWS Settings, confirma el proyecto, la Región seleccionada, el plan y los límites de gasto. Comprueba acceso con un perfil dedicado. Las instrucciones siguientes usan AWS CLI v2 y variables del proyecto confirmado. [Inicio de sesión con AWS CLI](https://docs.aws.amazon.com/signin/latest/userguide/command-line-sign-in.html)
2. Completa `infra/terraform/environments/aws-main` antes de aplicar. La base RDS todavía tiene un valor temporal de contraseña y el entorno carece de los recursos ECS y ECR requeridos por el pipeline. Prepara estado remoto, red privada, RDS con respaldo y TLS, tres repositorios ECR con etiquetas inmutables, ECS Fargate, ALB HTTPS, cobertura de logs y roles separados. Valida disponibilidad de PostGIS para la versión elegida de RDS y disponibilidad de servicios en la Región del proyecto.
3. Implementa y prueba `APP_PROFILE=production`. El código actual reconoce el perfil cloud de pruebas y la demo de arriendo vive en el perfil local. El perfil nuevo debe validar autenticación, correo, S3, secretos y health checks con servicios reales. Prueba el BFF de Next.js con la API Go por loopback dentro de la misma tarea ECS. Expón únicamente Next.js al balanceador. El sitio público HTTPS debe ofrecer un endpoint de salud que verifique también el acceso a la API.
4. Prepara la base inicial y revisa la política de cada migración. Conserva los roles de ejecución, consulta y migración separados. Ensaya el aprovisionamiento, backup y restauración en una base desechable. Usa la autoridad operativa correspondiente a cada migración marcada para revisión en `db/release-policy.json`, incluida V99. La tarea automática de CI/CD ejecuta un preflight de sólo lectura y se detiene si el ledger tiene cambios pendientes o no coincide con el código.
5. Crea el proveedor de GitHub OIDC con audiencia `sts.amazonaws.com` y el rol de entrega con el sujeto exacto `repo:HernanEspinozaDev/espacigo-aws:environment:produccion`. Limita su acceso a los tres ECR, el servicio ECS y las familias de tareas previstas. Restringe `iam:PassRole` a los roles de tarea y ejecución de ECS. Las tareas referencian secretos de AWS y el rol de CI carece de lectura de secretos de aplicación. [GitHub OIDC en AWS](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws), [Roles de tareas ECS](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html)
6. Crea el servicio ECS inicial con una tarea privada de dos contenedores llamados `api` y `web`, y una tarea independiente de verificación con un contenedor `dbmigrate`. Configura health checks del contenedor y del target group para un rollout rolling con circuit breaker. La tarea de verificación necesita acceso de lectura al ledger y una red Fargate sin IP pública. [Task definitions de ECS](https://docs.aws.amazon.com/cli/latest/reference/ecs/register-task-definition.html)

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

## Entrega y rollback

1. Tras pasar la CI en `main`, GitHub Actions publica las imágenes de API, web y migrador en ECR con el SHA y el intento del workflow. Registra revisiones ECS con referencias `@sha256:…`. Comprueba la identidad con AWS STS y conserva los digests y ambas revisiones ECS en el log de la ejecución. [ECR en AWS CLI](https://docs.aws.amazon.com/cli/latest/reference/ecr/)
2. Ejecuta `aws ecs run-task` con la imagen migradora de ese mismo SHA y `-plan-only -require-current`. Verifica el código de salida tras `aws ecs wait tasks-stopped`. Si hay migraciones pendientes, revisa la tarea y el ledger, prepara un snapshot y ejecuta la migración autorizada con el rol dedicado en la red privada. Después vuelve a ejecutar el workflow de `main`. Usa migraciones compatibles con ambas versiones para el rollback de aplicación. [ECS run-task](https://docs.aws.amazon.com/cli/latest/reference/ecs/run-task.html), [ECS tasks-stopped](https://docs.aws.amazon.com/cli/latest/reference/ecs/wait/tasks-stopped.html)
3. Actualiza el servicio con `aws ecs update-service`, espera a que ECS reporte estabilidad y comprueba que la revisión activa es la nueva y que la URL HTTPS responde. Ante un fallo, el circuit breaker de ECS aplica rollback de rollout. Si falla la comprobación externa y el servicio sigue en la revisión nueva, el script restaura la anterior. Revisa eventos del servicio y logs. La base se recupera con el procedimiento de restauración o una migración correctiva si hiciera falta. [ECS update-service](https://docs.aws.amazon.com/cli/latest/reference/ecs/update-service.html)

Deja la entrega deshabilitada hasta comprobar infraestructura, perfil de aplicación, ledger y acceso OIDC. Después activa `AWS_DEPLOY_ENABLED=true` en el repositorio y ejecuta manualmente **Producción AWS** sobre `main` para registrar la primera entrega validada. Las integraciones posteriores disparan el despliegue automáticamente.
