import { Check } from "lucide-react";

export function IdentityVerifiedBadge() {
  return (
    <span className="inline-flex w-fit shrink-0 items-center gap-1.5 rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-300">
      <Check className="size-3.5" aria-hidden="true" />
      Verificado
    </span>
  );
}
