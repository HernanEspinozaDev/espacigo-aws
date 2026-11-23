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

En Windows, crea o edita `%UserProfile%\.wslconfig` para asegurar reenvío de puertos a `127.0.0.1` y suficiente memoria:

```ini
[wsl2]
localhostForwarding=true
memory=8GB
```

Si editaste `.wslconfig`, reinicia WSL desde PowerShell con `wsl --shutdown` y vuelve a abrir Ubuntu.

### 2.2 Instalar Docker Desktop para Windows

1. Descarga e instala **Docker Desktop for Windows**.
2. En **Settings -> General**, verifica que esté marcado **Use the WSL 2 based engine**.
3. En **Settings -> Resources -> WSL Integration**, activa la integración para **Ubuntu-24.04** y haz clic en **Apply & Restart**.

Verifica dentro de Ubuntu (WSL2) que Docker responde:

```bash
docker version
docker compose version
```

---

## 3. Clonar el repositorio en la rama `kyb` (dentro de WSL2)

> **Importante:** clona el repositorio dentro del sistema de archivos Linux de WSL2 (por ejemplo `~/proyectos/espacigo-aws`), **no** en `/mnt/c/...`, para evitar lentitud de E/S y problemas de permisos con `.local/secrets/`.

Dentro de la terminal de **Ubuntu (WSL2)**:

```bash
sudo apt-get update && sudo apt-get install -y git curl jq unzip ca-certificates build-essential python3

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

1. Instala el binario `aws-emulator` (Linux x86_64) en `~/.local/opt/aws-emulator/bin/aws-emulator` y crea el enlace simbólico en `~/.local/bin/aws-emulator`:

```bash
mkdir -p ~/.local/bin ~/.local/opt/aws-emulator/bin ~/.aws/emulator
# Copia o extrae el binario aws-emulator en ~/.local/opt/aws-emulator/bin/aws-emulator
chmod +x ~/.local/opt/aws-emulator/bin/aws-emulator
ln -sf ~/.local/opt/aws-emulator/bin/aws-emulator ~/.local/bin/aws-emulator
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
export PATH="$HOME/.local/bin:$PATH"
```

2. Inicia el emulador en el puerto `9297`:

```bash
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
export AWS_REGION="us-east-2"
export AWS_DEFAULT_REGION="us-east-2"
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

## 6. Levantar PostgreSQL 18, Mailpit y aprovisionar el stack

1. Asegúrate de que el contenedor PostgreSQL 18 (`espacigo_localbackend_20261024-database-1`) esté arriba en Docker Desktop (o levántalo con `compose.yaml` según el perfil local) y de contar con los secretos locales en `.local/secrets/` (nunca se suben a Git):
   - `.local/secrets/db_admin_password`
   - `.local/secrets/runtime_password`
   - `.local/secrets/migrator_password`
   - Keyrings locales requeridos por la API (`document-verification-keyring.json`, `identity-declaration-keyring.json`, `identity-rut-hmac-keyring.json`, etc.).

2. Ejecuta el bootstrap local para crear los 5 buckets S3 (`espacigo-evidence-local`, `espacigo-media-local`, `espacigo-kyc-identity-local`, `espacigo-kyc-space-local`, `espacigo-privacy-replay-local`), los parámetros SSM en `/espacigo/local/`, los Log Groups, los permisos de PostgreSQL, el login de migraciones, `frontend/.env.local` y el contenedor **Mailpit**:

```bash
cd ~/proyectos/espacigo-aws
./scripts/local-dev.sh bootstrap
```

3. Revisa y aplica las migraciones locales pendientes:

```bash
./scripts/local-dev.sh migrate-plan
./scripts/local-dev.sh migrate
```

4. Comprueba que los 5 componentes están `ONLINE`:

```bash
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

- **`127.0.0.1:3000` no carga en el navegador de Windows:** verifica que `%UserProfile%\.wslconfig` tenga `localhostForwarding=true` y reinicia WSL con `wsl --shutdown`.
- **`docker: command not found` o permiso denegado en WSL2:** abre Docker Desktop en Windows, ve a *Settings -> Resources -> WSL Integration* y habilita `Ubuntu-24.04`.
- **Errores `\r` o `$'\r': command not found` al correr `.sh`:** asegúrate de tener `git config --global core.autocrlf input` y de haber clonado dentro de WSL2 (`~/proyectos/...`), no en `/mnt/c/...`.
- **`ERROR: aws-emulator env not found`:** confirma que `aws-emulator` está corriendo y que existe `~/.aws/emulator/9297.env`.
