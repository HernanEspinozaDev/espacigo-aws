#!/usr/bin/env bash
# ==============================================================================
# seed-rental-lifecycle.sh
# 
# Inicializador Soberano y Reproducible para el Entorno Local de EspaciGo AWS.
# Garantiza que las cuentas de ensayo (arrendatario y anfitrión) cuenten con:
#  1. Contraseña activa conocida ('TestPassword123!').
#  2. Roles asignados (arrendador, arrendatario, administrador).
#  3. Verificación KYC Soberana aprobada (< 24 meses) requerida por el backend Go.
#  4. Espacio 'casa cartagena' publicado y disponible en el catálogo comercial.
# ==============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB_CONTAINER="espacigo_localbackend_20261024-database-1"
DB_NAME="espacigo_aws_local"

RENTER_ID="7864e777-dbdc-4ae8-8497-9dbf083a3366"
RENTER_EMAIL="hernan.test@espacigo.invalid"

HOST_ID="fc06f3b4-fe96-4cb9-a8f9-833fc6c28d74"
HOST_EMAIL="hernan_espinoza@outlook.com"

SPACE_ID="9417a1ee-4e87-4a4a-9bab-3b2a11b036d9"
PASS_HASH="\$2a\$12\$eM6Lg/A1JKFzSyx6cpGKWeTLd1y2IM6TC7sDJAxEYb1enNWZMIwGa"

admin_pass=$(cat "${REPO_ROOT}/.local/secrets/db_admin_password" 2>/dev/null || echo "")
if [[ -z "$admin_pass" ]]; then
  echo "[-] ERROR: .local/secrets/db_admin_password no encontrado." >&2
  exit 1
fi

psql_cmd() {
  docker exec -i -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" -v ON_ERROR_STOP=1 "$@"
}

echo "=== [EspaciGo AWS] Bootstrap de Cuentas y Entorno de Arriendo ==="

# 1. Asegurar usuarios y contraseñas
echo "[1/4] Configurando cuentas base y credenciales..."
psql_cmd <<EOF
-- Arrendatario
INSERT INTO public.usuario (id, correo_original, correo_normalizado, hash_clave, estado, creado_en, actualizado_en)
VALUES ('$RENTER_ID', '$RENTER_EMAIL', '$RENTER_EMAIL', '$PASS_HASH', 'activo', now(), now())
ON CONFLICT (id) DO UPDATE SET hash_clave = EXCLUDED.hash_clave, estado = 'activo', intentos_fallidos_consecutivos = 0, bloqueado_hasta = NULL;

-- Anfitrión
INSERT INTO public.usuario (id, correo_original, correo_normalizado, hash_clave, estado, creado_en, actualizado_en)
VALUES ('$HOST_ID', '$HOST_EMAIL', '$HOST_EMAIL', '$PASS_HASH', 'activo', now(), now())
ON CONFLICT (id) DO UPDATE SET hash_clave = EXCLUDED.hash_clave, estado = 'activo', intentos_fallidos_consecutivos = 0, bloqueado_hasta = NULL;

-- Perfiles
INSERT INTO public.perfil_usuario (usuario_id, nombre_visible, actualizado_en)
VALUES ('$RENTER_ID', 'Hernán Test', now())
ON CONFLICT (usuario_id) DO NOTHING;

INSERT INTO public.perfil_usuario (usuario_id, nombre_visible, actualizado_en)
VALUES ('$HOST_ID', 'Hernán Espinoza', now())
ON CONFLICT (usuario_id) DO NOTHING;

-- Roles
INSERT INTO public.rol_usuario (usuario_id, rol) VALUES ('$RENTER_ID', 'arrendatario') ON CONFLICT DO NOTHING;
INSERT INTO public.rol_usuario (usuario_id, rol) VALUES ('$HOST_ID', 'arrendador') ON CONFLICT DO NOTHING;
INSERT INTO public.rol_usuario (usuario_id, rol) VALUES ('$HOST_ID', 'administrador') ON CONFLICT DO NOTHING;
EOF

# 2. Asegurar KYC Soberano (< 24 meses)
echo "[2/4] Asegurando elegibilidad KYC para ambas partes..."
psql_cmd <<EOF
-- Asegurar casos documentales con validez estricta (< 24 meses)
UPDATE public.document_verification_case
SET verified_until = verified_at + interval '12 months'
WHERE subject_id IN ('$RENTER_ID', '$HOST_ID') AND (verified_until IS NULL OR verified_until > verified_at + interval '24 months');

-- Asegurar estado elegible activo
UPDATE public.elegibilidad_verificacion_local
SET estado = 'elegible', revocada_en = NULL, revocada_por = NULL, motivo_revocacion_codigo = NULL
WHERE usuario_id IN ('$RENTER_ID', '$HOST_ID') AND tipo = 'kyc';
EOF

# 3. Asegurar publicación comercial del espacio
echo "[3/4] Verificando publicación de espacio ($SPACE_ID)..."
psql_cmd <<EOF
UPDATE public.espacio
SET propietario_id = '$HOST_ID', estado = 'activa', capacidad_maxima = 5, superficie_m2 = 25
WHERE id = '$SPACE_ID';
EOF

# 4. Resumen
echo "[4/4] Bootstrap completado con éxito."
echo ""
echo "----------------------------------------------------------------------"
echo "  Arrendatario : $RENTER_EMAIL  | Clave: TestPassword123!"
echo "  Anfitrión    : $HOST_EMAIL | Clave: TestPassword123!"
echo "  Espacio      : http://127.0.0.1:3000/espacios/$SPACE_ID"
echo "  Mailpit      : http://127.0.0.1:8025"
echo "----------------------------------------------------------------------"
