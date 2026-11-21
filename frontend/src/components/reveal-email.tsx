"use client";

import { useEffect, useRef, useState } from "react";

export function maskEmail(email: string): string {
  const characters = Array.from(email);
  if (characters.length < 3 || !email.includes("@")) return "••••";
  return `${characters[0]}••••${characters.at(-1)}`;
}

export function maskPhone(phone?: string | null): string {
  if (!phone) return "••••";
  const trimmed = phone.trim();
  if (!trimmed) return "••••";
  const digits = trimmed.replace(/\D/g, "");
  if (digits.length < 4) return "••••";
  return `+56 •••• ${digits.slice(-4)}`;
}

export function maskRUT(rut: string): string {
  const trimmed = rut.trim();
  if (!trimmed) return "••••";
  if (trimmed.startsWith("••••")) {
    const characters = Array.from(trimmed);
    if (characters.length === 8) {
      const suffix = characters.slice(4);
      if (
        suffix.every((c) => (c >= "0" && c <= "9") || c === "K" || c === "k")
      ) {
        return `••••${suffix.join("").toUpperCase()}`;
      }
    }
    return "••••";
  }
  const characters = Array.from(trimmed);
  const significant: string[] = [];
  for (const c of characters) {
    if (c === "." || c === "-" || c === "–" || c === " ") continue;
    if ((c >= "0" && c <= "9") || c === "k" || c === "K") {
      significant.push(c.toUpperCase());
    } else {
      return "••••";
    }
  }
  if (significant.length < 4 || significant.length > 10) return "••••";
  for (let i = 0; i < significant.length - 1; i++) {
    if (significant[i] === "K") return "••••";
  }
  return `••••${significant.slice(-4).join("")}`;
}

export function RevealEmail({
  masked,
  reveal,
}: {
  masked: string;
  reveal: (signal: AbortSignal) => string | Promise<string>;
}) {
  const [email, setEmail] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => () => controller.current?.abort(), []);

  useEffect(() => {
    if (email === null) return;
    const hide = () => setEmail(null);
    const timeout = window.setTimeout(hide, 30_000);
    window.addEventListener("pagehide", hide);
    return () => {
      window.clearTimeout(timeout);
      window.removeEventListener("pagehide", hide);
    };
  }, [email]);

  async function toggle() {
    if (email !== null) {
      setEmail(null);
      return;
    }
    const attempt = new AbortController();
    controller.current = attempt;
    setFailed(false);
    setPending(true);
    try {
      const value = await reveal(attempt.signal);
      if (!attempt.signal.aborted) setEmail(value);
    } catch {
      if (!attempt.signal.aborted) setFailed(true);
    } finally {
      if (!attempt.signal.aborted) setPending(false);
    }
  }

  return (
    <span>
      <span aria-live="polite">{email ?? masked}</span>{" "}
      <button
        type="button"
        className="underline underline-offset-2"
        onClick={() => void toggle()}
        disabled={pending}
        aria-expanded={email !== null}
      >
        {email !== null ? "Ocultar correo" : "Mostrar correo"}
      </button>
      {failed ? (
        <span role="status"> No pudimos mostrar el correo.</span>
      ) : null}
    </span>
  );
}
