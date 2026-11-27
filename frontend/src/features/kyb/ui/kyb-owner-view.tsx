"use client";

import { useEffect, useState } from "react";
import { Building2 } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import {
  CompanyKYBCard,
  type CompanyKYBOrganization,
} from "./company-kyb-card";
import { PersonalSpaceCompliance } from "./personal-space-compliance";

export function KYBOwnerView() {
  const { snapshot: auth, getAccessToken } = useAuth();
  const [organizations, setOrganizations] = useState<CompanyKYBOrganization[]>(
    [],
  );
  const [selectedOrganizationId, setSelectedOrganizationId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const token = auth.status === "authenticated" ? getAccessToken() : null;
  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void fetch("/api/backend/host/organizations", {
      credentials: "same-origin",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok)
          throw new Error("No fue posible cargar las empresas.");
        const payload = (await response.json()) as {
          items?: CompanyKYBOrganization[];
        };
        const items = payload.items ?? [];
        setOrganizations(items);
        setSelectedOrganizationId((current) =>
          current && items.some((item) => item.id === current)
            ? current
            : (items[0]?.id ?? ""),
        );
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setError("No fue posible consultar tus empresas.");
      });
    return () => controller.abort();
  }, [token]);

  const selectedOrganization = organizations.find(
    (organization) => organization.id === selectedOrganizationId,
  );

  if (auth.status !== "authenticated") {
    return (
      <Card>
        <CardContent className="py-10 text-sm text-muted-foreground">
          Inicia sesión para completar la verificación empresarial.
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <PersonalSpaceCompliance organizations={organizations} />
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <Building2 className="size-5 text-primary" aria-hidden="true" />
              <CardTitle>Verificación empresarial KYB</CardTitle>
            </div>
          </div>
          <CardDescription>
            Valida la empresa seleccionada con e-RUT, SII y Vigencia de Poderes.
            El respaldo de cada inmueble se envía por separado.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="kyb-organization">Empresa</Label>
            <select
              id="kyb-organization"
              className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
              value={selectedOrganizationId}
              onChange={(event) =>
                setSelectedOrganizationId(event.target.value)
              }
            >
              <option value="">Selecciona una empresa</option>
              {organizations.map((organization) => (
                <option key={organization.id} value={organization.id}>
                  {organization.trade_name ||
                    organization.legal_name ||
                    organization.razon_social}
                </option>
              ))}
            </select>
            {organizations.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Primero registra una empresa en{" "}
                <a
                  className="text-primary underline"
                  href="/mi-cuenta/empresas"
                >
                  Perfil empresas
                </a>
                .
              </p>
            ) : null}
          </div>
          {selectedOrganization ? (
            <CompanyKYBCard
              organization={selectedOrganization}
              variant="embedded"
            />
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
