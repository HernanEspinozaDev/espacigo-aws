"use client";

import { Loader2 } from "lucide-react";

interface LoadingSpinnerProps {
  readonly label?: string;
  readonly inline?: boolean;
  readonly className?: string;
}

export function LoadingSpinner({
  label = "Cargando…",
  inline = false,
  className = "",
}: LoadingSpinnerProps) {
  if (inline) {
    return (
      <span
        role="status"
        aria-live="polite"
        className={`inline-flex items-center justify-center gap-2 ${className}`.trim()}
      >
        <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden="true" />
        {label ? <span>{label}</span> : null}
      </span>
    );
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className={`flex min-h-[140px] flex-col items-center justify-center gap-3 p-6 text-center text-sm text-gray-600 ${className}`.trim()}
    >
      <Loader2
        className="size-8 shrink-0 animate-spin text-orange-700"
        aria-hidden="true"
      />
      {label ? <p className="font-medium text-gray-700">{label}</p> : null}
    </div>
  );
}
