"use client";

import { useState } from "react";

export function PublicHostAvatar({
  name,
  imageUrl,
}: {
  name: string | null;
  imageUrl?: string | null;
}) {
  const [failed, setFailed] = useState(false);
  const initials = (name ?? "Anfitrión")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => Array.from(word)[0]?.toLocaleUpperCase("es") ?? "")
    .join("");

  return (
    <span
      aria-hidden="true"
      className="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted font-semibold"
    >
      {imageUrl && !failed ? (
        // The avatar endpoint is scoped to a currently published listing.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={imageUrl}
          alt=""
          className="size-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : (
        initials
      )}
    </span>
  );
}
