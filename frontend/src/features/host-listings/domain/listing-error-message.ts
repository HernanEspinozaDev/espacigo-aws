import { ApiError } from "@/lib/api/errors";

const GENERIC_ERROR =
  "No fue posible completar la operación. Intenta nuevamente.";

export function listingErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return GENERIC_ERROR;

  if (error.code === "location_confirmation_required")
    return "Confirma la dirección y el pin en Dirección y mapa del espacio. Después vuelve a Publicar sin salir del editor.";
  if (error.code === "identity_verification_required")
    return "Debes confirmar tu identidad (KYC) antes de publicar un espacio.";
  if (error.code === "mercadopago_connection_required")
    return "Debes vincular tu cuenta de Mercado Pago antes de publicar un espacio.";
  if (error.code === "contract_template_required")
    return "Debes configurar y publicar la plantilla de contrato del espacio antes de publicarlo.";
  if (error.code === "account_disabled_missing_documentation")
    return "No puedes publicar o administrar espacios hasta completar la documentación pendiente. Puedes seguir usando tu cuenta como arrendatario y subir documentos desde Verificación y documentación.";

  if (error.code === "host_policy_acceptance_required")
    return "Acepta los términos vigentes de EspaciGo para publicar o administrar espacios.";

  switch (error.status) {
    case 401:
      return "Tu sesión venció. Inicia sesión nuevamente para continuar.";
    case 403:
      return "No tienes permiso para realizar esta operación.";
    case 412:
      return "Este espacio cambió. Actualiza la información antes de volver a guardar.";
    case 422:
      return "Revisa la información ingresada e intenta nuevamente.";
    case 428:
      return "Vuelve a verificar tu identidad para continuar.";
    default:
      return GENERIC_ERROR;
  }
}
