"use client";

import { useEffect, useRef, useState } from "react";
import { Clock3 } from "lucide-react";

interface ReservationCountdownProps {
  expiresAt?: string | null;
  onExpire?: () => void;
  className?: string;
}

function formatRemaining(ms: number): string {
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function ReservationCountdown({
  expiresAt,
  onExpire,
  className = "",
}: ReservationCountdownProps) {
  const targetMs = expiresAt ? Date.parse(expiresAt) : Number.NaN;
  const [nowMs, setNowMs] = useState<number>(() => Date.now());
  const expiredFiredRef = useRef(false);
  const onExpireRef = useRef(onExpire);

  useEffect(() => {
    onExpireRef.current = onExpire;
  }, [onExpire]);

  useEffect(() => {
    expiredFiredRef.current = false;
  }, [expiresAt]);

  useEffect(() => {
    if (!Number.isFinite(targetMs)) return;
    const tick = () => {
      const current = Date.now();
      setNowMs(current);
      if (targetMs - current <= 0 && !expiredFiredRef.current) {
        expiredFiredRef.current = true;
        onExpireRef.current?.();
      }
    };
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [targetMs]);

  if (!Number.isFinite(targetMs)) return null;

  const remainingMs = targetMs - nowMs;
  const expired = remainingMs <= 0;
  const urgent = !expired && remainingMs <= 60_000;

  return (
    <span
      role="timer"
      aria-live="polite"
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-semibold tabular-nums ${
        expired
          ? "border-red-300 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950/30 dark:text-red-300"
          : urgent
            ? "border-amber-400 bg-amber-100 text-amber-900 dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-200"
            : "border-[#e8c5b5] bg-[#f5e3db] text-[#702a1b] dark:border-amber-800/60 dark:bg-amber-950/20 dark:text-amber-200"
      } ${className}`.trim()}
    >
      <Clock3 className="size-3.5 shrink-0" aria-hidden="true" />
      {expired
        ? "Tiempo agotado (00:00)"
        : `Tiempo restante para esta reserva: ${formatRemaining(remainingMs)}`}
    </span>
  );
}
