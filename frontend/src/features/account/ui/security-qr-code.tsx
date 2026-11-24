"use client";

import { useMemo } from "react";
import QRCode from "qrcode";

export function SecurityQrCode({
  payload,
  label,
}: {
  payload: string;
  label: string;
}) {
  const image = useMemo(() => {
    try {
      const { modules } = QRCode.create(payload, { errorCorrectionLevel: "M" });
      const margin = 4;
      const path: string[] = [];
      for (let row = 0; row < modules.size; row++) {
        for (let col = 0; col < modules.size; col++) {
          if (modules.get(row, col)) {
            path.push(`M${col + margin} ${row + margin}h1v1h-1z`);
          }
        }
      }
      return { size: modules.size + margin * 2, path: path.join("") };
    } catch {
      return null;
    }
  }, [payload]);

  if (!image) {
    return (
      <p role="alert" className="text-sm">
        No pudimos generar el código QR.
      </p>
    );
  }

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${image.size} ${image.size}`}
      className="size-52 max-w-full rounded-lg border bg-white"
      shapeRendering="crispEdges"
    >
      <path d={image.path} fill="black" />
    </svg>
  );
}
