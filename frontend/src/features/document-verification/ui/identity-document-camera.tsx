"use client";

import { useEffect, useRef, useState } from "react";
import { Camera, Loader2, RefreshCw, X } from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import type { IdentityEvidenceRole } from "../domain/document-verification";

type FacingMode = "environment" | "user";

const roleLabel: Record<"identity_front" | "identity_back", string> = {
  identity_front: "anverso",
  identity_back: "reverso",
};

export function IdentityDocumentCamera({
  role,
  onCapture,
  onClose,
}: {
  role: Extract<IdentityEvidenceRole, "identity_front" | "identity_back">;
  onCapture: (file: File) => void;
  onClose: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const generationRef = useRef(0);
  const mountedRef = useRef(false);
  const capturePendingRef = useRef(false);
  const [facingMode, setFacingMode] = useState<FacingMode>("environment");
  const [status, setStatus] = useState<"opening" | "ready" | "error">(
    "opening",
  );
  const [capturePending, setCapturePending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function stopStream() {
    const stream = streamRef.current;
    streamRef.current = null;
    stream?.getTracks().forEach((track) => track.stop());
    if (videoRef.current) videoRef.current.srcObject = null;
  }

  async function startStream(mode: FacingMode, showOpeningState = true) {
    const requestGeneration = ++generationRef.current;
    stopStream();
    if (showOpeningState) {
      setFacingMode(mode);
      setStatus("opening");
      setError(null);
    }

    try {
      const mediaDevices = navigator.mediaDevices;
      if (!mediaDevices?.getUserMedia) {
        throw new Error("camera_unavailable");
      }
      const stream = await mediaDevices.getUserMedia({
        video: { facingMode: { ideal: mode } },
        audio: false,
      });
      if (!mountedRef.current || requestGeneration !== generationRef.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play().catch(() => undefined);
      }
      if (requestGeneration === generationRef.current) setStatus("ready");
    } catch {
      if (!mountedRef.current || requestGeneration !== generationRef.current) {
        return;
      }
      stopStream();
      setStatus("error");
      setError(
        "No se pudo acceder a la cámara. Puedes continuar seleccionando una imagen JPEG o PNG.",
      );
    }
  }

  useEffect(() => {
    mountedRef.current = true;
    const startTimer = window.setTimeout(
      () => void startStream("environment", false),
      0,
    );
    return () => {
      window.clearTimeout(startTimer);
      mountedRef.current = false;
      generationRef.current += 1;
      stopStream();
    };
    // startStream closes over stable refs and state setters; start once per panel mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function captureFrame() {
    if (capturePendingRef.current) return;
    const video = videoRef.current;
    if (!video || video.videoWidth < 1 || video.videoHeight < 1) {
      setStatus("error");
      setError(
        "La cámara todavía no está lista. Espera un momento o selecciona un archivo.",
      );
      return;
    }
    const canvas = document.createElement("canvas");
    const maxDimension = 1920;
    const scale = Math.min(
      1,
      maxDimension / Math.max(video.videoWidth, video.videoHeight),
    );
    canvas.width = Math.max(1, Math.round(video.videoWidth * scale));
    canvas.height = Math.max(1, Math.round(video.videoHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) {
      setStatus("error");
      setError(
        "No se pudo preparar la imagen. Selecciona un archivo JPEG o PNG.",
      );
      return;
    }
    capturePendingRef.current = true;
    setCapturePending(true);
    const captureGeneration = generationRef.current;
    context.drawImage(video, 0, 0, canvas.width, canvas.height);
    canvas.toBlob(
      (blob) => {
        if (
          !blob ||
          !mountedRef.current ||
          captureGeneration !== generationRef.current
        ) {
          capturePendingRef.current = false;
          if (mountedRef.current) {
            setCapturePending(false);
            if (!blob) {
              setStatus("error");
              setError(
                "No se pudo capturar la imagen. Selecciona un archivo JPEG o PNG.",
              );
            }
          }
          return;
        }
        const file = new File([blob], `${role}.jpg`, {
          type: "image/jpeg",
          lastModified: Date.now(),
        });
        generationRef.current += 1;
        capturePendingRef.current = false;
        setCapturePending(false);
        stopStream();
        onCapture(file);
      },
      "image/jpeg",
      0.88,
    );
  }

  return (
    <section
      aria-label={`Cámara para capturar el ${roleLabel[role]}`}
      className="space-y-3 rounded-md border bg-muted/20 p-3 sm:p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium">Captura del {roleLabel[role]}</p>
        <Button type="button" variant="ghost" size="sm" onClick={onClose}>
          <X className="mr-2 size-4" aria-hidden="true" />
          Cerrar cámara
        </Button>
      </div>
      <div className="overflow-hidden rounded-md bg-black">
        <video
          ref={videoRef}
          aria-label={`Vista previa de cámara para el ${roleLabel[role]}`}
          autoPlay
          muted
          playsInline
          className="mx-auto aspect-[4/3] max-h-[60vh] w-full object-contain"
        />
      </div>
      {status === "opening" && (
        <p role="status" className="flex items-center gap-2 text-sm">
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
          Abriendo cámara…
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          onClick={() => void captureFrame()}
          disabled={status !== "ready" || capturePending}
        >
          <Camera className="mr-2 size-4" aria-hidden="true" />
          Tomar foto
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            void startStream(
              facingMode === "environment" ? "user" : "environment",
            )
          }
          disabled={status === "opening" || capturePending}
        >
          <RefreshCw className="mr-2 size-4" aria-hidden="true" />
          Cambiar cámara
        </Button>
      </div>
    </section>
  );
}
