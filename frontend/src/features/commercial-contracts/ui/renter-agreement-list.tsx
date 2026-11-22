"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { useAuth } from "@/features/auth/ui/use-auth";
import { commercialAgreements } from "../infrastructure/agreements-runtime";
import {
  agreementStateLabel,
  type CommercialAgreement,
} from "../domain/agreements";
import { agreementErrorMessage } from "./renter-agreement-panel";

export function RenterAgreementList() {
  const { accessToken, snapshot, getAccessToken } = useAuth();
  if (!accessToken || snapshot.restrictedMode)
    return <p>Inicia sesión para consultar tus solicitudes de contrato.</p>;
  return (
    <AgreementList
      key={`${accessToken}:${snapshot.accountId}`}
      isCurrent={() => getAccessToken() === accessToken}
    />
  );
}
function AgreementList({ isCurrent }: { isCurrent(): boolean }) {
  const [items, setItems] = useState<CommercialAgreement[] | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    commercialAgreements
      .list("renter")
      .then((loaded) => {
        if (active && isCurrent()) {
          setItems(loaded);
          setError("");
        }
      })
      .catch((e: unknown) => {
        if (active && isCurrent()) setError(agreementErrorMessage(e));
      });
    return () => {
      active = false;
    };
  }, [attempt, isCurrent]);
  return (
    <section className="space-y-4" aria-label="Tus solicitudes de arriendo">
      {error && (
        <p role="alert" className="text-sm text-red-800">
          {error}
        </p>
      )}
      <button
        type="button"
        className="min-h-11 text-sm underline underline-offset-4"
        onClick={() => setAttempt((v) => v + 1)}
      >
        Actualizar solicitudes
      </button>
      {!items && !error && <p role="status">Cargando solicitudes…</p>}
      {items?.length === 0 && (
        <p>
          Aún no enviaste solicitudes de contrato. Elige un espacio y revisa sus
          condiciones antes de firmar.
        </p>
      )}
      <ul className="grid gap-4 sm:grid-cols-2">
        {items?.map((a) => (
          <li
            key={a.id}
            className="space-y-3 rounded-xl border border-border bg-card p-5"
          >
            <h2 className="font-semibold">{agreementStateLabel(a.state)}</h2>
            <p className="text-sm text-muted-foreground">
              Enviada el {new Date(a.created_at).toLocaleDateString("es-CL")}
            </p>
            <Link
              className="inline-flex min-h-11 items-center font-medium underline underline-offset-4"
              href={`/reservas/contratos/${encodeURIComponent(a.id)}`}
            >
              Ver contrato y estado
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
