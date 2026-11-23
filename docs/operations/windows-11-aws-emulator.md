# Guía: Montar el flujo completo de `aws-emulator` en Windows 11 (Rama `kyb`)

Esta guía describe paso a paso cómo levantar el entorno de desarrollo local de **EspaciGo AWS** en **Windows 11**, usando **WSL2 (Ubuntu)** + **Docker Desktop**, para trabajar de forma aislada sobre la rama `kyb`.

> **Permisos de trabajo en GitHub:** la cuenta `ShivaAkasha` está configurada con permisos de escritura **exclusivamente sobre la rama `kyb`**. Las demás ramas (`main`, `desarrollo-local`, `publicacion-aws`, etc.) y la creación/borrado de otras ramas están bloqueadas por reglas del repositorio.

---

## 1. Arquitectura en Windows 11

Todos los scripts del repositorio (`scripts/local-dev.sh`, `scripts/local-aws-bootstrap.sh`, etc.) están escritos en Bash para Linux, y `aws-emulator` se ejecuta como binario Linux en `https://127.0.0.1:9297`. Por eso, el flujo recomendado en Windows 11 es ejecutar todo dentro de **WSL2 (Ubuntu)** y acceder a la web desde el navegador de Windows 11 en `http://127.0.0.1:3000`.

| Servicio | Puerto local | Dónde corre | Acceso desde Windows 11 |
| --- | --- | --- | --- |
| **Frontend Web (Next.js)** | `3000` | WSL2 (`./scripts/local-dev.sh web`) | `http://127.0.0.1:3000` |
| **Backend API (Go)** | `8080` | WSL2 (`./scripts/local-dev.sh api`) | `http://127.0.0.1:8080` |
| **AWS Emulator (S3, SSM, Logs, IAM)** | `9297` | WSL2 (`aws-emulator`) | `https://127.0.0.1:9297` |
| **PostgreSQL 18** | `5432` | Docker Desktop (WSL2) | `127.0.0.1:5432` |
| **Mailpit (SMTP / Web UI)** | `1025` / `8025` | Docker Desktop (`espacigo-mailpit`) | `http://127.0.0.1:8025` |

---

## 2. Prerrequisitos en Windows 11

### 2.1 Instalar WSL2 y Ubuntu

Abre **PowerShell como Administrador** y ejecuta:

```powershell
wsl --install -d Ubuntu-24.04
```

Reinicia Windows si te lo solicita, abre **Ubuntu 24.04** desde el menú Inicio y crea tu usuario/contraseña de Linux.

En Windows, crea `%UserProfile%\.wslconfig` directamente desde **PowerShell** (evita usar el Bloc de notas para no guardar `.wslconfig.txt` por error):

```powershell
@"
[wsl2]
localhostForwarding=true
memory=8GB
"@ | Set-Content -Path "$env:USERPROFILE\.wslconfig" -Encoding utf8

wsl --shutdown
```

> **Cómo abrir WSL2 rápidamente sin buscar "Ubuntu" cada vez:**
> - Presiona **`Win + R`**, escribe **`wsl ~`** y presiona **Enter** (abre directo en tu `$HOME` de Linux).
> - O en **Windows Terminal** (`Win + X` -> Terminal), abre **Configuración (`Ctrl + ,`) -> Inicio -> Perfil predeterminado** y elige **Ubuntu-24.04**.

### 2.2 Instalar Docker Desktop para Windows

1. Descarga e instala **Docker Desktop for Windows**.
2. **Reinicia Windows 11** tras la instalación para que se activen el servicio y los permisos del backend WSL2.
3. Abre **Docker Desktop** y espera hasta que en la esquina inferior izquierda aparezca el estado en verde (**Engine running**).
4. En **Settings (engranaje) -> General**, verifica que esté marcado **Use the WSL 2 based engine**.
5. En **Settings -> Resources -> WSL Integration**, activa **Enable integration with my default WSL distro** y el interruptor de **Ubuntu-24.04**, luego haz clic en **Apply & Restart**.

Verifica dentro de **Ubuntu (WSL2)** que Docker responde usando el socket Linux (no `npipe://`):

```bash
docker version
docker compose version
```

> Si `docker version` muestra tanto **Client** como **Server**, la integración está lista. Si aparece `npipe:////./pipe/docker_engine`, verifica que Docker Desktop esté abierto (**Engine running**) y ejecuta `wsl --shutdown` en PowerShell antes de reabrir Ubuntu.

---

## 3. Clonar el repositorio en la rama `kyb` (dentro de WSL2)

> **Importante:** clona el repositorio dentro del sistema de archivos Linux de WSL2 (por ejemplo `~/proyectos/espacigo-aws`), **no** en `/mnt/c/...`, para evitar lentitud de E/S y problemas de permisos con `.local/secrets/`.

Dentro de la terminal de **Ubuntu (WSL2)**:

```bash
sudo apt-get update && sudo apt-get install -y git curl jq unzip ca-certificates build-essential python3 postgresql-client

# Evita conversiones CRLF de Windows en scripts .sh
git config --global core.autocrlf input

# Autentica GitHub CLI con tu cuenta (ShivaAkasha)
# Si aún no tienes gh instalado:
type -p curl >/dev/null || sudo apt install curl -y
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null
sudo apt-get update && sudo apt-get install -y gh

gh auth login
gh auth setup-git
```

Clona directamente la rama `kyb`:

```bash
mkdir -p ~/proyectos
cd ~/proyectos
git clone -b kyb https://github.com/HernanEspinozaDev/espacigo-aws.git
cd espacigo-aws
```

---

## 4. Instalar dependencias del stack (Go, Node.js, pnpm, AWS CLI v2)

### 4.1 Go (`>= 1.27.1`)

El archivo `go.mod` requiere `go 1.27.1`. Instálalo en `/usr/local/go`:

```bash
GO_VERSION="1.27.1"
curl -fsSLO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
rm "go${GO_VERSION}.linux-amd64.tar.gz"

echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"' >> ~/.bashrc
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
go version
```

### 4.2 Node.js (`>= 24.21.0 < 27`) y pnpm (`12.15.0`)

`frontend/package.json` fija `"node": ">=24.21.0 <27"` y `"packageManager": "pnpm@12.15.0"`. Instálalo con `nvm` o binario oficial:

```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash
source ~/.nvm/nvm.sh
nvm install 24
corepack enable
corepack prepare pnpm@12.15.0 --activate

node -v
pnpm -v
```

Instala las dependencias del frontend:

```bash
cd ~/proyectos/espacigo-aws/frontend
pnpm install
cd ~/proyectos/espacigo-aws
```

### 4.3 AWS CLI v2

```bash
curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "/tmp/awscliv2.zip"
unzip -q /tmp/awscliv2.zip -d /tmp
sudo /tmp/aws/install
rm -rf /tmp/aws /tmp/awscliv2.zip
aws --version
```

---

## 5. Instalar y configurar `aws-emulator`

1. Instala el paquete oficial `.deb` de `aws-emulator` para Ubuntu (`amd64`):

```bash
curl -fsSLo /tmp/aws-emulator.deb https://downloads.aws.com/aws-emulator/packages/aws-emulator/LTS/latest/aws-emulator_amd64.deb
sudo apt-get install -y /tmp/aws-emulator.deb
rm -f /tmp/aws-emulator.deb
aws-emulator -v
```

2. Inicia el emulador en el puerto `9297`:

```bash
mkdir -p ~/.aws/emulator
aws-emulator -b 127.0.0.1 -p 9297
```

Al iniciar por primera vez, `aws-emulator` genera su CA TLS y credenciales en `~/.aws/emulator/9297/` (`ca.pem` y `emulator_credentials`).

3. Crea el archivo `~/.aws/emulator/9297.env` (si el instalador del emulador no lo creó ya) para aislar completamente las llamadas locales de cualquier perfil real de AWS:

```bash
cat <<'EOF' > ~/.aws/emulator/9297.env
# Source in a subshell: ( . ~/.aws/emulator/9297.env && aws ... )
unset AWS_PROFILE AWS_DEFAULT_PROFILE
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_SECURITY_TOKEN
unset AWS_CREDENTIAL_PROCESS AWS_WEB_IDENTITY_TOKEN_FILE AWS_ROLE_ARN AWS_ROLE_SESSION_NAME
unset AWS_CONFIG_FILE AWS_SHARED_CREDENTIALS_FILE AWS_CA_BUNDLE
export AWS_ENDPOINT_URL="https://127.0.0.1:9297"
export AWS_SHARED_CREDENTIALS_FILE="$HOME/.aws/emulator/9297/emulator_credentials"
export AWS_CONFIG_FILE="$HOME/.aws/emulator/9297/config"
export AWS_CA_BUNDLE="$HOME/.aws/emulator/9297/ca.pem"
export AWS_EC2_METADATA_DISABLED="true"
export AWS_REGION="us-east-1"
export AWS_DEFAULT_REGION="us-east-1"
EOF
```

4. Verifica la salud del emulador en otra terminal:

```bash
(
  . ~/.aws/emulator/9297.env
  curl --cacert "$AWS_CA_BUNDLE" -fsS "$AWS_ENDPOINT_URL/__admin/health"
  aws sts get-caller-identity
)
```

---

## 6. Instalar/levantar PostgreSQL 18, generar secretos, Mailpit y aprovisionar el stack

No necesitas instalar PostgreSQL manualmente en Ubuntu: el repositorio incluye `compose.yaml` + `Dockerfile` (`target: database`) con **PostgreSQL 18 + PostGIS 3.6**.

1. **Genera los secretos locales y aprovisiona `aws-emulator`:**
   Con `aws-emulator` corriendo en otra terminal, ejecuta `scripts/local-aws-bootstrap.sh`. Este script crea automáticamente `.local/secrets/` (`db_admin_password`, `runtime_password` y todos los keyrings JSON):

```bash
cd ~/proyectos/espacigo-aws
bash scripts/local-aws-bootstrap.sh
```

2. **Construye y levanta el contenedor PostgreSQL 18 (`espacigo_localbackend_20261024-database-1`):**
   Guardamos la configuración en `.local/compose-db.env` (ignorado por Git) para pasarla con `--env-file` y evitar que se pierdan variables entre comandos:

```bash
mkdir -p .local/database-tls ~/.local/share/espacigo/m03-evidence ~/.local/state/espacigo/privacy-replay

cat <<EOF > .local/compose-db.env
LOCAL_UID=$(id -u)
LOCAL_GID=$(id -g)
LOCAL_SECRETS_DIR=$PWD/.local/secrets
LOCAL_DATABASE_TLS_DIR=$PWD/.local/database-tls
LOCAL_M03_EVIDENCE_DIR=$HOME/.local/share/espacigo/m03-evidence
LOCAL_PRIVACY_REPLAY_DIR=$HOME/.local/state/espacigo/privacy-replay
LOCAL_COMPOSE_PROJECT_NAME=espacigo_localbackend_20261024
LOCAL_IMAGE_PREFIX=espacigo_localbackend_20261024
LOCAL_PGDATA_VOLUME=espacigo_localbackend_20261024_pgdata
LOCAL_DATABASE_NAME=espacigo_aws_local
EOF

cat <<'EOF' > .local/compose-db-ports.yaml
services:
  database:
    ports:
      - "127.0.0.1:5432:5432"

networks:
  data:
    internal: false
EOF

docker compose --env-file .local/compose-db.env -f compose.yaml -f .local/compose-db-ports.yaml up -d --build database
```

   Verifica que el contenedor esté corriendo y saludable (`healthy`):

```bash
docker ps --filter "name=espacigo_localbackend_20261024-database-1"
```

3. **Inicializa los roles base en PostgreSQL (solo la primera vez en una BD vacía):**
   `compose.yaml` crea `espacigo_admin` y `espacigo_runtime`, pero `scripts/local-dev.sh` también requiere `espacigo_api_login`, `espacigo_schema_owner` y `espacigo_migrator_login`. Si partes desde una base vacía (sin restaurar un dump previo):

```bash
ADMIN_PASS="$(cat .local/secrets/db_admin_password)"
RUNTIME_PASS="$(cat .local/secrets/runtime_password)"
docker exec -i -e PGPASSWORD="$ADMIN_PASS" espacigo_localbackend_20261024-database-1 \
  psql -U espacigo_admin -d espacigo_aws_local <<SQL
CREATE ROLE espacigo_api_login LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '${RUNTIME_PASS}';
GRANT espacigo_runtime TO espacigo_api_login;
CREATE ROLE espacigo_schema_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
CREATE ROLE espacigo_migrator_login LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
GRANT espacigo_schema_owner TO espacigo_migrator_login WITH SET OPTION;
SQL
```

   > Si el equipo te comparte un dump base de `espacigo_aws_local`, restáuralo aquí con `psql` / `pg_restore` antes del siguiente paso.

4. **Ejecuta el bootstrap completo, migraciones y verificación:**
   En Docker Desktop sobre Windows 11, exporta `ESPACIGO_LOCAL_DB_HOST=127.0.0.1` y `ESPACIGO_LOCAL_DB_PORT=5432` (porque las IPs internas `172.x.x.x` de los contenedores no son accesibles desde WSL2). `bootstrap` crea los 5 buckets S3, parámetros SSM (`/espacigo/local/`), Log Groups, permisos de `espacigo_runtime`, `migrator-setup`, `frontend/.env.local` y el contenedor **Mailpit**:

```bash
export ESPACIGO_LOCAL_DB_HOST=127.0.0.1
export ESPACIGO_LOCAL_DB_PORT=5432
./scripts/local-dev.sh bootstrap
./scripts/local-dev.sh migrate-plan
./scripts/local-dev.sh migrate
./scripts/local-dev.sh status
```

---

## 7. Ejecutar la API, el Frontend y las pruebas

Abre terminales separadas en WSL2 (o usa VS Code / Cursor en Windows conectado con la extensión **WSL**):

- **Terminal 1 (Backend Go en `:8080`):**
  ```bash
  ./scripts/local-dev.sh api
  ```
- **Terminal 2 (Frontend Next.js en `:3000`):**
  ```bash
  ./scripts/local-dev.sh web
  ```
- **Terminal 3 (Pruebas automatizadas):**
  ```bash
  ./scripts/local-dev.sh test
  ```

Desde tu navegador en **Windows 11**:
- **Aplicación web:** `http://127.0.0.1:3000`
- **Bandeja de correos (Mailpit):** `http://127.0.0.1:8025`

---

## 8. Flujo de trabajo Git en la rama `kyb`

Trabaja siempre sobre la rama `kyb` y haz push únicamente a `origin kyb`:

```bash
git status
git add <archivos>
git commit -m "feat(kyb): descripción del cambio"
git push origin kyb
```

> Si intentas hacer `git push` a `main`, `desarrollo-local` o crear otra rama remota con la cuenta `ShivaAkasha`, GitHub rechazará la operación por la regla del repositorio.

---

## 9. Solución de problemas frecuentes en Windows 11

- **`open //./pipe/docker_engine: The system cannot find the file specified`:** Docker Desktop no está corriendo o falta habilitar la integración WSL2. Reinicia Windows si acabas de instalarlo, abre Docker Desktop hasta ver **Engine running** en verde, activa *Settings -> Resources -> WSL Integration -> Ubuntu-24.04*, ejecuta `wsl --shutdown` en PowerShell y vuelve a abrir Ubuntu.
- **`required variable LOCAL_UID is missing a value`:** ejecutaste `docker compose` sin `--env-file .local/compose-db.env` (o sin las variables exportadas en esa misma terminal). Usa siempre `docker compose --env-file .local/compose-db.env -f compose.yaml ...`.
- **`IllegalLocationConstraintException` al crear buckets S3 en `local-aws-bootstrap.sh`:** `~/.aws/emulator/9297.env` debe usar `AWS_REGION="us-east-1"` y `AWS_DEFAULT_REGION="us-east-1"` (no `us-east-2`).
- **`127.0.0.1:3000` no carga en el navegador de Windows:** verifica que `%UserProfile%\.wslconfig` tenga `localhostForwarding=true` y reinicia WSL con `wsl --shutdown`.
- **Errores `\r` o `$'\r': command not found` al correr `.sh`:** asegúrate de tener `git config --global core.autocrlf input` y de haber clonado dentro de WSL2 (`~/proyectos/...`), no en `/mnt/c/...`.
- **`ERROR: aws-emulator env not found`:** confirma que `aws-emulator` está corriendo en otra terminal (`aws-emulator -b 127.0.0.1 -p 9297`) y que existe `~/.aws/emulator/9297.env`.
