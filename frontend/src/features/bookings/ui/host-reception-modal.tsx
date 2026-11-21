"use client";

import { useEffect, useRef, useState } from "react";
import {
  Camera,
  CheckCircle2,
  Loader2,
  Play,
  QrCode,
  Search,
  ShieldCheck,
  X,
} from "lucide-react";
import type { BookingInterval, BookingReservation } from "../domain/types";
import {
  formatTimeRange24h,
  usageStatusLabel,
} from "../domain/reservation-intervals";
import { Button } from "@/components/workspace/ui/button";

interface HostReceptionModalProps {
  open: boolean;
  spaceId?: string;
  onClose: () => void;
  onUpdated?: () => void;
}

interface VerifiedPayload {
  reservation: BookingReservation;
  interval: BookingInterval;
}

export function HostReceptionModal({
  open,
  spaceId,
  onClose,
  onUpdated,
}: HostReceptionModalProps) {
  const [codeInput, setCodeInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [verified, setVerified] = useState<VerifiedPayload | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);

  const stopCamera = () => {
    if (streamRef.current) {
      for (const track of streamRef.current.getTracks()) {
        track.stop();
      }
      streamRef.current = null;
    }
    setScanning(false);
  };

  useEffect(() => {
    return () => {
      if (streamRef.current) {
        for (const track of streamRef.current.getTracks()) {
          track.stop();
        }
        streamRef.current = null;
      }
    };
  }, []);

  const handleClose = () => {
    stopCamera();
    setVerified(null);
    setError(null);
    setActionMessage(null);
    setCodeInput("");
    onClose();
  };

  if (!open) return null;

  const callVerify = async (
    codeOrQr: string,
    action: "preview" | "check_in" | "check_out",
  ) => {
    const trimmed = codeOrQr.trim();
    if (!trimmed) {
      setError("Ingresa un código de pase (ej. ESP-4A9F12) o escanea un QR.");
      return;
    }
    setLoading(true);
    setError(null);
    setActionMessage(null);
    try {
      const res = await fetch(
        "/api/backend/commercial-reservations/verify-pass",
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/json",
          },
          body: JSON.stringify({
            code_or_qr: trimmed,
            ...(spaceId ? { space_id: spaceId } : {}),
            action,
          }),
        },
      );
      const body = (await res.json().catch(() => null)) as {
        data?: VerifiedPayload;
        error?: { code?: string };
      } | null;

      if (!res.ok || !body?.data) {
        if (res.status === 404) {
          setError(
            "No encontramos un pase pagado vigente con ese código para tus espacios.",
          );
        } else if (res.status === 409) {
          setError(
            "Este pase ya fue marcado como utilizado anteriormente.",
          );
        } else {
          setError("No se pudo validar el pase. Verifica el código e intenta otra vez.");
        }
        if (action === "preview") setVerified(null);
        return;
      }

      setVerified(body.data);
      if (action === "check_in") {
        setActionMessage("Llegada confirmada: el pase pasó a estado En uso.");
        onUpdated?.();
      } else if (action === "check_out") {
        setActionMessage("Sesión finalizada: el pase quedó marcado como Usada.");
        onUpdated?.();
      }
    } catch {
      setError("Error de red al validar el pase.");
    } finally {
      setLoading(false);
    }
  };

  const startQrScan = async () => {
    setError(null);
    const BarcodeDetectorCtor = (
      window as unknown as {
        BarcodeDetector?: new (opts: { formats: string[] }) => {
          detect: (
            source: HTMLVideoElement,
          ) => Promise<Array<{ rawValue?: string }>>;
        };
      }
    ).BarcodeDetector;

    if (!BarcodeDetectorCtor || !navigator.mediaDevices?.getUserMedia) {
      setError(
        "Tu navegador actual no soporta escaneo directo de cámara. Ingresa el código corto de 6 caracteres que te indique el cliente.",
      );
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: "environment" },
      });
      streamRef.current = stream;
      setScanning(true);
      requestAnimationFrame(() => {
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          void videoRef.current.play();
        }
      });
      const detector = new BarcodeDetectorCtor({ formats: ["qr_code"] });
      const tick = async () => {
        if (!streamRef.current || !videoRef.current) return;
        try {
          const codes = await detector.detect(videoRef.current);
          const found = codes.find((c) => c.rawValue)?.rawValue;
          if (found) {
            stopCamera();
            setCodeInput(found);
            void callVerify(found, "preview");
            return;
          }
        } catch {
          // Ignore single-frame detection errors
        }
        if (streamRef.current) {
          setTimeout(() => {
            void tick();
          }, 300);
        }
      };
      setTimeout(() => {
        void tick();
      }, 400);
    } catch {
      stopCamera();
      setError(
        "No fue posible acceder a la cámara. Puedes ingresar el código corto manualmente.",
      );
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="espacigo-reception-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-3 backdrop-blur-xs sm:p-4"
    >
      <div className="relative max-h-[92vh] w-full max-w-lg overflow-y-auto rounded-2xl border border-border bg-card text-foreground shadow-2xl">
        <div className="h-1.5 w-full bg-gradient-to-r from-[#EE8953] via-[#D46236] to-[#B94628]" />
        <div className="p-5 sm:p-6 space-y-5">
          <div className="flex items-start justify-between gap-4 border-b border-border pb-4">
            <div className="flex items-center gap-3">
              <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#D46236]/10 text-[#D46236]">
                <ShieldCheck className="h-6 w-6" />
              </div>
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[#D46236]">
                  Recepción Anfitrión
                </p>
                <h2 id="espacigo-reception-title" className="text-lg font-bold">
                  Validar Ingreso (QR o Código)
                </h2>
              </div>
            </div>
            <button
              type="button"
              onClick={handleClose}
              aria-label="Cerrar recepción"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              void callVerify(codeInput, "preview");
            }}
            className="space-y-3"
          >
            <label
              htmlFor="reception-code-input"
              className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground"
            >
              Código del pase (ej. ESP-4A9F12) o lectura QR
            </label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <input
                id="reception-code-input"
                type="text"
                value={codeInput}
                onChange={(e) => setCodeInput(e.target.value)}
                placeholder="ESP-XXXXXX"
                className="flex-1 rounded-xl border border-border bg-background px-3.5 py-2.5 font-mono text-sm uppercase focus:border-[#D46236] focus:outline-none"
              />
              <Button
                type="submit"
                disabled={loading}
                className="gap-1.5 bg-[#D46236] text-white hover:bg-[#B94628]"
              >
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Search className="h-4 w-4" />
                )}
                Validar
              </Button>
            </div>
            <div className="flex items-center justify-between">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={scanning ? stopCamera : () => void startQrScan()}
                className="gap-1.5 text-xs"
              >
                <Camera className="h-3.5 w-3.5" />
                {scanning ? "Detener cámara" : "Escanear QR con cámara"}
              </Button>
            </div>
          </form>

          {scanning && (
            <div className="overflow-hidden rounded-xl border border-border bg-black">
              <video
                ref={videoRef}
                playsInline
                muted
                className="h-56 w-full object-cover"
              />
            </div>
          )}

          {error && (
            <p className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-xs font-medium text-destructive">
              {error}
            </p>
          )}

          {actionMessage && (
            <p className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs font-semibold text-emerald-800 dark:text-emerald-300">
              {actionMessage}
            </p>
          )}

          {verified && (
            <div className="space-y-3 rounded-xl border border-border bg-muted/25 p-4 text-xs">
              <div className="flex items-center justify-between gap-2">
                <span className="inline-flex items-center gap-1.5 font-bold uppercase tracking-wider text-foreground">
                  <QrCode className="h-4 w-4 text-[#D46236]" />
                  Pase Verificado
                </span>
                <span className="rounded-full border border-border bg-background px-2.5 py-0.5 font-semibold">
                  {usageStatusLabel(
                    verified.interval.usage_status ?? "pendiente",
                  )}
                </span>
              </div>
              <p className="text-sm font-bold text-foreground">
                {verified.reservation.space_title || "Espacio EspaciGo"}
              </p>
              <p className="font-mono text-foreground">
                {new Intl.DateTimeFormat("es-CL", {
                  dateStyle: "medium",
                  timeZone: verified.reservation.time_zone || "America/Santiago",
                }).format(new Date(verified.interval.start_at))}{" "}
                ·{" "}
                {formatTimeRange24h(
                  verified.interval.start_at,
                  verified.interval.end_at,
                  verified.reservation.time_zone || "America/Santiago",
                )}{" "}
                ({verified.interval.units}{" "}
                {verified.reservation.rate_unit === "hora"
                  ? verified.interval.units === 1
                    ? "hora"
                    : "horas"
                  : verified.reservation.rate_unit}
                )
              </p>

              <div className="flex flex-wrap gap-2 pt-2">
                {(verified.interval.usage_status ?? "pendiente") ===
                  "pendiente" && (
                  <Button
                    type="button"
                    size="sm"
                    disabled={loading}
                    onClick={() => void callVerify(codeInput, "check_in")}
                    className="gap-1.5 bg-blue-600 text-white hover:bg-blue-700"
                  >
                    <Play className="h-3.5 w-3.5" />
                    Confirmar Llegada (En uso)
                  </Button>
                )}
                {((verified.interval.usage_status ?? "pendiente") ===
                  "pendiente" ||
                  verified.interval.usage_status === "en_uso") && (
                  <Button
                    type="button"
                    size="sm"
                    disabled={loading}
                    onClick={() => void callVerify(codeInput, "check_out")}
                    className="gap-1.5 bg-emerald-600 text-white hover:bg-emerald-700"
                  >
                    <CheckCircle2 className="h-3.5 w-3.5" />
                    Marcar Sesión Usada
                  </Button>
                )}
              </div>
            </div>
          )}

          <div className="flex justify-end border-t border-border pt-3">
            <Button type="button" variant="outline" size="sm" onClick={handleClose}>
              Cerrar
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
