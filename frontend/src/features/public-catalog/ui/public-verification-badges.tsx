import { BadgeCheck } from "lucide-react";
import type { PublicVerification } from "../domain/verification";

export function PublicVerificationBadges({
  verification,
}: {
  verification: PublicVerification;
  variant?: "card" | "detail";
}) {
  const showSpace =
    verification.identityVerified && verification.spaceVerified;

  if (!showSpace) return null;

  return (
    <ul
      aria-label="Distintivos informativos de verificación"
      className="flex flex-wrap gap-2"
    >
      <li title="La revisión no acredita legalidad o titularidad ni constituye garantía o firma.">
        <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-200">
          <BadgeCheck
            className="size-3.5 shrink-0 text-[var(--brand-success)]"
            aria-hidden="true"
          />
          Espacio verificado
        </span>
        <span className="sr-only">
          La revisión no acredita legalidad o titularidad ni constituye
          garantía o firma.
        </span>
      </li>
    </ul>
  );
}
