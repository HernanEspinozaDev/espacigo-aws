"use client";

import { useEffect, useId, useMemo, useState, type FormEvent } from "react";
import Link from "next/link";
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronRight,
  Clock,
  Copy,
  Info,
  Loader2,
  QrCode,
  ShieldCheck,
  Smartphone,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useQRVerification } from "@/features/account/ui/use-qr-verification";

function formatFullDate(isoDate?: string | null): string {
  if (!isoDate) return "Fecha no registrada";
  try {
    const date = new Date(isoDate);
    if (Number.isNaN(date.getTime())) return isoDate;
    return new Intl.DateTimeFormat("es-CL", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  } catch {
    return isoDate;
  }
}

// Generates a deterministic 25x25 QR matrix with valid finder, timing, and alignment patterns
function generateQRMatrix(payload: string): boolean[][] {
  const size = 25;
  const matrix: boolean[][] = Array.from({ length: size }, () =>
    Array(size).fill(false),
  );
  const isReserved: boolean[][] = Array.from({ length: size }, () =>
    Array(size).fill(false),
  );

  // Helper to draw a square pattern
  const setFinder = (startRow: number, startCol: number) => {
    for (let r = -1; r <= 7; r++) {
      for (let c = -1; c <= 7; c++) {
        const row = startRow + r;
        const col = startCol + c;
        if (row >= 0 && row < size && col >= 0 && col < size) {
          isReserved[row][col] = true;
          // Outer 7x7 border
          if (r >= 0 && r <= 6 && c >= 0 && c <= 6) {
            if (r === 0 || r === 6 || c === 0 || c === 6) {
              matrix[row][col] = true;
            } else if (r >= 2 && r <= 4 && c >= 2 && c <= 4) {
              matrix[row][col] = true; // Inner 3x3 center
            } else {
              matrix[row][col] = false; // 1-module white ring
            }
          } else {
            matrix[row][col] = false; // 1-module white separator
          }
        }
      }
    }
  };

  // 1. Finder patterns: top-left, top-right, bottom-left
  setFinder(0, 0);
  setFinder(0, size - 7);
  setFinder(size - 7, 0);

  // 2. Timing patterns on row 6 and col 6
  for (let i = 8; i < size - 8; i++) {
    isReserved[6][i] = true;
    matrix[6][i] = i % 2 === 0;

    isReserved[i][6] = true;
    matrix[i][6] = i % 2 === 0;
  }

  // 3. Alignment pattern at (16, 16)
  const alignRow = 16;
  const alignCol = 16;
  for (let r = -2; r <= 2; r++) {
    for (let c = -2; c <= 2; c++) {
      const row = alignRow + r;
      const col = alignCol + c;
      if (row >= 0 && row < size && col >= 0 && col < size) {
        isReserved[row][col] = true;
        if (Math.abs(r) === 2 || Math.abs(c) === 2 || (r === 0 && c === 0)) {
          matrix[row][col] = true;
        } else {
          matrix[row][col] = false;
        }
      }
    }
  }

  // 4. Fill remaining cells deterministically based on payload hash
  let hash = 0x811c9dc5;
  for (let i = 0; i < payload.length; i++) {
    hash ^= payload.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }

  let lcg = hash >>> 0;
  for (let r = 0; r < size; r++) {
    for (let c = 0; c < size; c++) {
      if (!isReserved[r][c]) {
        lcg = (Math.imul(1103515245, lcg) + 12345) & 0x7fffffff;
        matrix[r][c] = (lcg & 1) === 1;
      }
    }
  }

  return matrix;
}

function QRCodeSVG({ payload }: { payload: string }) {
  const matrix = useMemo(() => generateQRMatrix(payload), [payload]);
  const size = matrix.length;

  return (
    <svg
      viewBox={`0 0 ${size} ${size}`}
      className="size-56 sm:size-64 rounded-xl bg-white p-3 shadow-inner border border-zinc-200"
      shapeRendering="crispEdges"
      role="img"
      aria-label="Código QR de verificación"
    >
      <rect width={size} height={size} fill="#ffffff" />
      {matrix.map((row, r) =>
        row.map(
          (filled, c) =>
            filled && (
              <rect
                key={`${r}-${c}`}
                x={c}
                y={r}
                width={1}
                height={1}
                fill="#18181b"
              />
            ),
        ),
      )}
    </svg>
  );
}

export default function CodigoQRPage() {
  const {
    status,
    activeTicket,
    ticketStatus,
    isLoading,
    isGeneratingTicket,
    isApproving,
    isDisabling,
    error,
    notification,
    generateTicket,
    approveTicket,
    disableQR,
    cancelTicket,
    clearError,
    clearNotification,
  } = useQRVerification();

  const [copiedToken, setCopiedToken] = useState(false);
  const [manualTokenInput, setManualTokenInput] = useState("");
  const [manualDeviceLabel, setManualDeviceLabel] = useState("");
  const [showDisableConfirm, setShowDisableConfirm] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const manualTokenInputId = useId();
  const manualDeviceLabelId = useId();

  // Timer tick for active ticket
  useEffect(() => {
    if (!activeTicket || ticketStatus?.status !== "pending") {
      return;
    }

    const interval = setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => clearInterval(interval);
  }, [activeTicket, ticketStatus?.status]);

  const secondsRemaining = useMemo(() => {
    if (!activeTicket) return 120;
    const expiresAt = new Date(activeTicket.expires_at).getTime();
    return Math.max(0, Math.floor((expiresAt - now) / 1000));
  }, [activeTicket, now]);

  const handleCopyToken = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedToken(true);
      setTimeout(() => setCopiedToken(false), 2500);
    } catch {
      // ignore
    }
  };

  const handleGenerateTicket = async () => {
    clearError();
    clearNotification();
    const ticket = await generateTicket();
    if (ticket) {
      setManualTokenInput(ticket.qr_payload);
    }
  };

  const handleManualApprove = async (e: FormEvent) => {
    e.preventDefault();
    const token = manualTokenInput.trim();
    if (!token) return;
    await approveTicket(token, manualDeviceLabel.trim() || undefined);
  };

  const handleQuickApprove = async () => {
    if (!activeTicket) return;
    await approveTicket(
      activeTicket.qr_payload,
      "Dispositivo local de prueba (Navegador)",
    );
  };

  const handleConfirmDisable = async () => {
    await disableQR();
    setShowDisableConfirm(false);
  };

  const isExpired =
    ticketStatus?.status === "expired" ||
    (Boolean(activeTicket) && secondsRemaining === 0);

  const formatCountdown = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins.toString().padStart(2, "0")}:${secs.toString().padStart(2, "0")}`;
  };

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav
        aria-label="Ruta de navegación"
        className="flex items-center gap-2 text-xs text-muted-foreground flex-wrap"
      >
        <Link
          href="/mi-cuenta"
          className="hover:text-primary transition-colors font-medium"
        >
          Mi perfil
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad"
          className="hover:text-primary transition-colors font-medium"
        >
          Seguridad
        </Link>
        <ChevronRight className="size-3.5" />
        <Link
          href="/mi-cuenta/seguridad/metodos"
          className="hover:text-primary transition-colors font-medium"
        >
          Métodos de verificación
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="text-foreground font-semibold">Código QR</span>
      </nav>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-2 border-b border-border">
        <div className="space-y-1">
          <div className="flex items-center gap-3">
            <Link
              href="/mi-cuenta/seguridad/metodos"
              className="size-8 rounded-full flex items-center justify-center hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
              aria-label="Volver a métodos de verificación"
            >
              <ArrowLeft className="size-4" />
            </Link>
            <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground flex items-center gap-2.5">
              <span>Código QR</span>
              {status?.enabled && (
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                  <CheckCircle2 className="size-3.5" />
                  Activo
                </span>
              )}
            </h1>
          </div>
          <p className="text-sm text-muted-foreground max-w-2xl pl-11">
            Autoriza inicios de sesión y acciones sensibles escaneando un código
            QR seguro desde tu dispositivo móvil de confianza.
          </p>
        </div>
      </div>

      {/* Notification banner */}
      {notification && (
        <div className="bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-400 p-4 rounded-xl flex items-start justify-between gap-3 text-sm">
          <div className="flex items-center gap-2.5">
            <CheckCircle2 className="size-5 shrink-0 text-emerald-600" />
            <span>{notification}</span>
          </div>
          <button
            onClick={clearNotification}
            className="text-muted-foreground hover:text-foreground text-xs p-1"
            type="button"
          >
            ✕
          </button>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div className="bg-destructive/10 border border-destructive/20 text-destructive p-4 rounded-xl flex items-start justify-between gap-3 text-sm">
          <div className="flex items-center gap-2.5">
            <AlertCircle className="size-5 shrink-0 text-destructive" />
            <span>{error}</span>
          </div>
          <button
            onClick={clearError}
            className="text-muted-foreground hover:text-foreground text-xs p-1"
            type="button"
          >
            ✕
          </button>
        </div>
      )}

      {/* Loading state */}
      {isLoading && !status && (
        <div className="p-12 flex flex-col items-center justify-center gap-3 text-muted-foreground bg-card border border-border rounded-2xl">
          <Loader2 className="size-8 animate-spin text-primary" />
          <p className="text-sm">Cargando estado de verificación QR...</p>
        </div>
      )}

      {/* VISTA 1: DISPOSITIVO ACTIVO (enabled: true) */}
      {status?.enabled && (
        <div className="space-y-6">
          <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div className="flex items-start gap-4">
                <div className="size-12 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center shrink-0">
                  <ShieldCheck className="size-7" />
                </div>
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <h2 className="text-lg sm:text-xl font-bold text-foreground">
                      Dispositivo QR vinculado y activo
                    </h2>
                    <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                      <CheckCircle2 className="size-3.5" />
                      Activo
                    </span>
                  </div>
                  <p className="text-xs sm:text-sm text-muted-foreground">
                    Activado el {formatFullDate(status.enabled_at)}
                  </p>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
              <div className="bg-muted/40 border border-border/80 rounded-xl p-4 space-y-1">
                <span className="text-xs font-medium text-muted-foreground">
                  Dispositivo de confianza
                </span>
                <p className="text-base font-bold text-foreground flex items-center gap-2">
                  <Smartphone className="size-4 text-primary" />
                  <span>
                    {status.trusted_device_label ||
                      "Dispositivo móvil vinculado"}
                  </span>
                </p>
                <p className="text-xs text-muted-foreground">
                  Autorizado para escanear y confirmar tickets de acceso a tu
                  cuenta.
                </p>
              </div>

              <div className="bg-muted/40 border border-border/80 rounded-xl p-4 space-y-1">
                <span className="text-xs font-medium text-muted-foreground">
                  Método de protección
                </span>
                <p className="text-base font-semibold text-foreground">
                  Desafío QR Criptográfico (TTL 120s)
                </p>
                <p className="text-xs text-muted-foreground">
                  Generación de tokens efímeros con firma unidireccional y
                  aislamiento por cuenta.
                </p>
              </div>
            </div>
          </div>

          {/* Desactivar dispositivo QR */}
          <div className="bg-card border border-destructive/20 rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
            <div className="flex items-start gap-4">
              <div className="size-10 rounded-xl bg-destructive/10 text-destructive flex items-center justify-center shrink-0 mt-0.5">
                <Trash2 className="size-5" />
              </div>
              <div className="space-y-1">
                <h3 className="text-base sm:text-lg font-bold text-foreground">
                  Desvincular dispositivo QR
                </h3>
                <p className="text-xs sm:text-sm text-muted-foreground">
                  Si cambiaste de dispositivo móvil o no reconoces el equipo
                  vinculado, puedes revocar inmediatamente la confianza y
                  desactivar este método.
                </p>
              </div>
            </div>

            {!showDisableConfirm ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setShowDisableConfirm(true)}
              >
                Desvincular este dispositivo
              </Button>
            ) : (
              <div className="bg-destructive/5 border border-destructive/20 rounded-xl p-4 space-y-3 max-w-lg">
                <div className="flex items-start gap-2.5 text-xs sm:text-sm text-destructive">
                  <AlertTriangle className="size-4 shrink-0 mt-0.5" />
                  <p>
                    ¿Estás seguro de que deseas desvincular tu dispositivo?
                    Tendrás que generar un nuevo código QR para volver a
                    vincularlo.
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={handleConfirmDisable}
                    disabled={isDisabling}
                  >
                    {isDisabling ? (
                      <>
                        <Loader2 className="size-4 animate-spin mr-1.5" />
                        <span>Desvinculando...</span>
                      </>
                    ) : (
                      <span>Sí, desvincular</span>
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setShowDisableConfirm(false)}
                    disabled={isDisabling}
                    className="text-xs text-muted-foreground"
                  >
                    Cancelar
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* VISTA 2: NO ACTIVADO (enabled: false) */}
      {!status?.enabled && !isLoading && (
        <div className="space-y-6">
          {!activeTicket ? (
            /* Tarjeta inicial previa a generar ticket */
            <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-6 shadow-sm">
              <div className="flex flex-col sm:flex-row items-start gap-5">
                <div className="size-14 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
                  <QrCode className="size-7" />
                </div>
                <div className="space-y-2 flex-1">
                  <h2 className="text-xl font-bold text-foreground">
                    Vincula un dispositivo con Código QR
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Genera un código QR temporal en tu pantalla para escanearlo
                    con la cámara de tu teléfono y registrarlo como tu
                    dispositivo móvil de confianza.
                  </p>
                </div>
              </div>

              {/* Beneficios */}
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-2">
                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <Clock className="size-4 text-primary" />
                    <span>Rápido y efímero</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Cada código QR cuenta con una vigencia estricta de 120
                    segundos.
                  </p>
                </div>

                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <ShieldCheck className="size-4 text-emerald-600" />
                    <span>Dispositivo confiable</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Solo los dispositivos autenticados por ti pueden aprobar
                    operaciones críticas.
                  </p>
                </div>

                <div className="bg-muted/30 border border-border/60 rounded-xl p-4 space-y-1.5">
                  <div className="flex items-center gap-2 text-foreground font-semibold text-sm">
                    <Smartphone className="size-4 text-primary" />
                    <span>Sin contraseñas</span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Autoriza accesos futuros con un solo escaneo desde tu
                    celular.
                  </p>
                </div>
              </div>

              <div className="pt-4 border-t border-border flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                <div className="text-xs text-muted-foreground flex items-center gap-1.5">
                  <Info className="size-4 text-muted-foreground shrink-0" />
                  <span>
                    El proceso toma menos de 1 minuto y requiere la cámara de tu
                    dispositivo.
                  </span>
                </div>

                <Button
                  variant="default"
                  size="lg"
                  onClick={handleGenerateTicket}
                  disabled={isGeneratingTicket}
                  className="font-medium"
                >
                  {isGeneratingTicket ? (
                    <>
                      <Loader2 className="size-4 animate-spin mr-2" />
                      <span>Generando código QR...</span>
                    </>
                  ) : (
                    <>
                      <QrCode className="size-4 mr-2" />
                      <span>Generar código QR</span>
                    </>
                  )}
                </Button>
              </div>
            </div>
          ) : (
            /* Vista con Ticket Activo (QR, cuenta regresiva, aprobación) */
            <div className="bg-card border border-border rounded-2xl p-6 sm:p-8 space-y-8 shadow-sm">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-border">
                <div className="space-y-1">
                  <span className="text-xs font-semibold tracking-wider uppercase text-primary">
                    Paso de Vinculación
                  </span>
                  <h2 className="text-xl sm:text-2xl font-bold text-foreground">
                    Escanea el código QR desde tu celular
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    Apunta con la cámara de tu teléfono móvil o usa el panel de
                    aprobación manual a continuación.
                  </p>
                </div>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={cancelTicket}
                  className="self-start sm:self-auto text-xs"
                >
                  Cancelar y volver
                </Button>
              </div>

              {/* Grid: QR + Indicadores */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8 items-center">
                {/* Lado izquierdo: Render QR SVG */}
                <div className="flex flex-col items-center justify-center p-6 bg-muted/30 border border-border rounded-2xl space-y-4">
                  <div className="relative">
                    <QRCodeSVG payload={activeTicket.qr_payload} />
                    {isExpired && (
                      <div className="absolute inset-0 bg-background/85 backdrop-blur-xs rounded-xl flex flex-col items-center justify-center p-4 text-center gap-2">
                        <AlertCircle className="size-8 text-destructive" />
                        <p className="text-sm font-bold text-foreground">
                          Código QR expirado
                        </p>
                        <p className="text-xs text-muted-foreground">
                          El tiempo límite de 120 segundos ha transcurrido.
                        </p>
                        <Button
                          variant="default"
                          size="sm"
                          onClick={handleGenerateTicket}
                          className="mt-2"
                        >
                          Generar nuevo código
                        </Button>
                      </div>
                    )}
                  </div>

                  {/* Estado en vivo */}
                  <div className="flex items-center gap-2 text-xs font-medium">
                    {ticketStatus?.status === "pending" && !isExpired && (
                      <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-500/20">
                        <span className="size-2 rounded-full bg-amber-500 animate-pulse" />
                        Esperando escaneo o aprobación...
                      </span>
                    )}
                    {ticketStatus?.status === "approved" && (
                      <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20">
                        <CheckCircle2 className="size-3.5" />
                        ¡Aprobado con éxito!
                      </span>
                    )}
                    {isExpired && (
                      <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-destructive/10 text-destructive border border-destructive/20">
                        <AlertCircle className="size-3.5" />
                        Expirado
                      </span>
                    )}
                  </div>
                </div>

                {/* Lado derecho: Cronómetro y detalles del ticket */}
                <div className="space-y-5">
                  {/* Tarjeta del cronómetro */}
                  <div className="bg-muted/40 border border-border rounded-xl p-5 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
                        <Clock className="size-4 text-primary" />
                        Tiempo restante del ticket
                      </span>
                      <span
                        className={`font-mono text-lg font-bold ${
                          secondsRemaining < 30
                            ? "text-destructive"
                            : "text-foreground"
                        }`}
                      >
                        {formatCountdown(secondsRemaining)}
                      </span>
                    </div>

                    {/* Barra de progreso */}
                    <div className="w-full bg-muted rounded-full h-2 overflow-hidden border border-border/60">
                      <div
                        className={`h-full transition-all duration-1000 ${
                          secondsRemaining < 30
                            ? "bg-destructive"
                            : "bg-primary"
                        }`}
                        style={{
                          width: `${Math.min(100, Math.max(0, (secondsRemaining / 120) * 100))}%`,
                        }}
                      />
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Por seguridad, este ticket se invalida automáticamente al
                      expirar el contador.
                    </p>
                  </div>

                  {/* Token payload con opción de copiado */}
                  <div className="space-y-2">
                    <Label className="text-xs font-medium text-muted-foreground">
                      Token criptográfico del ticket
                    </Label>
                    <div className="flex items-center gap-2">
                      <div className="font-mono text-xs bg-muted/70 border border-border rounded-lg px-3 py-2 flex-1 truncate text-foreground select-all">
                        {activeTicket.qr_payload}
                      </div>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleCopyToken(activeTicket.qr_payload)}
                        className="shrink-0 text-xs flex items-center gap-1.5"
                      >
                        {copiedToken ? (
                          <>
                            <Check className="size-3.5 text-emerald-600" />
                            <span>¡Copiado!</span>
                          </>
                        ) : (
                          <>
                            <Copy className="size-3.5" />
                            <span>Copiar</span>
                          </>
                        )}
                      </Button>
                    </div>
                  </div>

                  {/* Botón de prueba rápida en entorno local */}
                  <div className="pt-2">
                    <Button
                      variant="secondary"
                      size="default"
                      onClick={handleQuickApprove}
                      disabled={isApproving || isExpired}
                      className="w-full font-medium"
                    >
                      {isApproving ? (
                        <>
                          <Loader2 className="size-4 animate-spin mr-2" />
                          <span>Aprobando ticket...</span>
                        </>
                      ) : (
                        <>
                          <CheckCircle2 className="size-4 mr-2 text-emerald-600" />
                          <span>
                            Aprobar ticket inmediatamente (Prueba local)
                          </span>
                        </>
                      )}
                    </Button>
                  </div>
                </div>
              </div>

              {/* Sección manual: Pegar y aprobar token desde otro dispositivo */}
              <div className="pt-6 border-t border-border space-y-4">
                <div className="space-y-1">
                  <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                    <Smartphone className="size-4 text-primary" />
                    <span>Aprobar manualmente con token</span>
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    Si estás probando desde otro navegador o equipo, pega el
                    token para registrar la vinculación.
                  </p>
                </div>

                <form
                  onSubmit={handleManualApprove}
                  className="grid grid-cols-1 sm:grid-cols-3 gap-3"
                >
                  <div className="sm:col-span-2 space-y-1.5">
                    <Label
                      htmlFor={manualTokenInputId}
                      className="text-xs font-medium text-muted-foreground"
                    >
                      Token del código QR
                    </Label>
                    <Input
                      id={manualTokenInputId}
                      type="text"
                      placeholder="v1.qr.token..."
                      value={manualTokenInput}
                      onChange={(e) => setManualTokenInput(e.target.value)}
                      disabled={isApproving || isExpired}
                      className="font-mono text-xs h-10"
                      required
                    />
                  </div>

                  <div className="space-y-1.5">
                    <Label
                      htmlFor={manualDeviceLabelId}
                      className="text-xs font-medium text-muted-foreground"
                    >
                      Etiqueta del dispositivo
                    </Label>
                    <Input
                      id={manualDeviceLabelId}
                      type="text"
                      placeholder="Mi Celular (Opcional)"
                      value={manualDeviceLabel}
                      onChange={(e) => setManualDeviceLabel(e.target.value)}
                      disabled={isApproving || isExpired}
                      className="text-xs h-10"
                    />
                  </div>

                  <div className="sm:col-span-3 flex justify-end gap-3 pt-1">
                    <Button
                      type="submit"
                      variant="default"
                      size="sm"
                      disabled={
                        isApproving || isExpired || !manualTokenInput.trim()
                      }
                      className="font-medium"
                    >
                      {isApproving ? (
                        <>
                          <Loader2 className="size-4 animate-spin mr-1.5" />
                          <span>Validando...</span>
                        </>
                      ) : (
                        <span>Aprobar y vincular dispositivo</span>
                      )}
                    </Button>
                  </div>
                </form>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
