export const cancellationPolicies = [
  {
    version: "flexible_v1",
    label: "Flexible",
    summary: "Reembolso total hasta 24 horas antes del inicio del arriendo.",
  },
  {
    version: "moderada_v1",
    label: "Moderada",
    summary: "Reembolso del 50 % hasta 48 horas antes del inicio del arriendo.",
  },
  {
    version: "estricta_v1",
    label: "Estricta",
    summary: "Sin reembolso después de confirmar la reserva.",
  },
] as const;

export type CancellationPolicyVersion =
  (typeof cancellationPolicies)[number]["version"];

export function cancellationPolicyVersion(
  value: string,
): CancellationPolicyVersion | undefined {
  return cancellationPolicies.find((option) => option.version === value)
    ?.version;
}
