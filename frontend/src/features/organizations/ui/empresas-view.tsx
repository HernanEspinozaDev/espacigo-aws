"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  AlertCircle,
  Building2,
  Clipboard,
  Loader2,
  Plus,
  RefreshCw,
  Send,
  Users,
} from "lucide-react";
import { Button } from "@/components/workspace/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/workspace/ui/card";
import { Input } from "@/components/workspace/ui/input";
import { Label } from "@/components/workspace/ui/label";
import { useAuth } from "@/features/auth/ui/use-auth";
import { isValidRut, normalizeRut } from "@/features/verifications/rut-utils";

type Member = {
  id?: string;
  member_id?: string;
  email?: string;
  display_name?: string;
  role?: string;
  status?: string;
  rol?: string;
  estado?: string;
};
type Organization = {
  id: string;
  legal_name?: string;
  razon_social?: string;
  trade_name?: string;
  nombre_comercial?: string;
  public_summary?: string;
  public_description?: string;
  description?: string;
  rut?: string;
  company_rut?: string;
  role?: string;
  my_role?: string;
  members?: Member[];
};
type OrganizationJoinRequest = {
  id: string;
  usuario_id?: string;
  rol_propuesto: string;
  estado: string;
  created_at?: string;
  expires_at?: string;
};

const api = "/api/backend/host";
async function request<T>(
  token: string,
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  if (!token) throw new Error("Inicia sesión para continuar.");
  const response = await fetch(`${api}${path}`, {
    method,
    credentials: "same-origin",
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(
      typeof payload.message === "string"
        ? payload.message
        : "No fue posible completar la operación.",
    ) as Error & { status: number; code?: string };
    error.status = response.status;
    error.code = payload.error?.code;
    throw error;
  }
  return payload as T;
}
function errorMessage(cause: unknown) {
  const error = cause as Error & { status?: number; code?: string };
  if (error.status === 409 || error.code?.includes("already_exists"))
    return "Esta empresa ya está registrada. Solicita una invitación a tu representante legal o contacta a soporte.";
  return error.message || "No fue posible completar la operación.";
}
const orgName = (org: Organization) =>
  org.trade_name ||
  org.nombre_comercial ||
  org.legal_name ||
  org.razon_social ||
  "Empresa";
const orgId = (org: Organization) => org.id;

export function EmpresasView() {
  const { snapshot, getAccessToken } = useAuth();
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [inviteLink, setInviteLink] = useState("");
  const [incomingInvite, setIncomingInvite] = useState<{
    organizationId: string;
    invitationId: string;
    token: string;
  } | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [members, setMembers] = useState<Member[]>([]);
  const [legalName, setLegalName] = useState("");
  const [tradeName, setTradeName] = useState("");
  const [newLegalName, setNewLegalName] = useState("");
  const [newTradeName, setNewTradeName] = useState("");
  const [newCompanyRut, setNewCompanyRut] = useState("");
  const [showJoinRequest, setShowJoinRequest] = useState(false);
  const [joinRole, setJoinRole] = useState("representante_legal");
  const [joinRequest, setJoinRequest] = useState<{
    id: string;
    estado: string;
    expires_at?: string;
  } | null>(null);
  const [organizationJoinRequests, setOrganizationJoinRequests] = useState<
    OrganizationJoinRequest[]
  >([]);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("operador");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const token = snapshot.status === "authenticated" ? getAccessToken() : null;
  useEffect(() => {
    void Promise.resolve().then(() => {
      const params = new URLSearchParams(window.location.hash.slice(1));
      const organizationId = params.get("organizationId");
      const invitationId = params.get("invitationId");
      const invitationToken = params.get("token");
      if (organizationId && invitationId && invitationToken)
        setIncomingInvite({
          organizationId,
          invitationId,
          token: invitationToken,
        });
    });
  }, []);
  const load = useCallback(async () => {
    if (!token) return;
    setBusy(true);
    try {
      const orgResult = await request<
        { items?: Organization[] } | Organization[]
      >(token, "/organizations");
      const orgs = Array.isArray(orgResult)
        ? orgResult
        : (orgResult.items ?? []);
      setOrganizations(orgs);
      const activeId =
        selectedId && orgs.some((org) => orgId(org) === selectedId)
          ? selectedId
          : orgs[0]
            ? orgId(orgs[0])
            : "";
      setSelectedId(activeId);
      if (activeId) {
        const [detail, memberResult] = await Promise.all([
          request<Organization>(
            token,
            `/organizations/${encodeURIComponent(activeId)}`,
          ),
          request<{ items?: Member[] } | Member[]>(
            token,
            `/organizations/${encodeURIComponent(activeId)}/members`,
          ),
        ]);
        const organization = ((
          detail as Organization & { organization?: Organization }
        ).organization ?? detail) as Organization;
        setMembers(
          (Array.isArray(memberResult)
            ? memberResult
            : (memberResult.items ?? organization.members ?? [])
          ).map((member) => ({
            ...member,
            role: member.role || member.rol,
            status: member.status || member.estado,
          })),
        );
        setLegalName(
          organization.legal_name || organization.razon_social || "",
        );
        setTradeName(
          organization.trade_name || organization.nombre_comercial || "",
        );
        if (
          (organization.my_role ||
            orgs.find((org) => orgId(org) === activeId)?.my_role) ===
          "representante_legal"
        ) {
          const result = await request<{ items?: OrganizationJoinRequest[] }>(
            token,
            `/organizations/${encodeURIComponent(activeId)}/join-requests`,
          );
          setOrganizationJoinRequests(result.items ?? []);
        } else {
          setOrganizationJoinRequests([]);
        }
      } else {
        setMembers([]);
        setOrganizationJoinRequests([]);
      }
      setError("");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }, [token, selectedId]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  if (snapshot.status !== "authenticated")
    return (
      <Card>
        <CardContent className="py-10 text-sm text-muted-foreground">
          Inicia sesión para administrar tus empresas.
        </CardContent>
      </Card>
    );

  async function createOrganization(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setNotice("");
    if (!isValidRut(newCompanyRut)) {
      setError("Ingresa un RUT empresarial válido.");
      return;
    }
    if (!token) return;
    setBusy(true);
    try {
      const createdResult = await request<{ organization: Organization }>(
        token,
        "/organizations",
        "POST",
        {
          legal_name: newLegalName.trim(),
          trade_name: newTradeName.trim(),
          company_rut: normalizeRut(newCompanyRut),
        },
      );
      const created = createdResult.organization;
      setNotice("Empresa creada. Completa su carpeta en Verificación KYB.");
      setNewLegalName("");
      setNewTradeName("");
      setNewCompanyRut("");
      setSelectedId(created.id);
      await load();
    } catch (cause) {
      const apiError = cause as Error & { status?: number; code?: string };
      if (apiError.status === 409) {
        setShowJoinRequest(true);
        setError(
          "Esta empresa ya se encuentra registrada. Puedes solicitar acceso con el mismo RUT; no mostraremos información de sus integrantes.",
        );
      } else setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  async function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !selectedId) return;
    setError("");
    setNotice("");
    setBusy(true);
    try {
      await request(
        token,
        `/organizations/${encodeURIComponent(selectedId)}`,
        "PATCH",
        { legal_name: legalName.trim(), trade_name: tradeName.trim() },
      );
      setNotice("Perfil comercial actualizado.");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !selectedId) return;
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const result = await request<{
        invitation: { id: string };
        invitation_token: string;
      }>(
        token,
        `/organizations/${encodeURIComponent(selectedId)}/invitations`,
        "POST",
        { email: inviteEmail.trim(), rol: inviteRole },
      );
      const link = new URL("/anfitrion/empresas", window.location.origin);
      link.hash = new URLSearchParams({
        organizationId: selectedId,
        invitationId: result.invitation.id,
        token: result.invitation_token,
      }).toString();
      setInviteLink(link.toString());
      setNotice(
        `Invitación creada para ${inviteEmail.trim()}. Copia y comparte el enlace de un solo uso.`,
      );
      setInviteEmail("");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  async function acceptInvitation() {
    if (!token || !incomingInvite) return;
    setBusy(true);
    setError("");
    try {
      await request(
        token,
        `/organizations/${encodeURIComponent(incomingInvite.organizationId)}/invitations/${encodeURIComponent(incomingInvite.invitationId)}/accept`,
        "POST",
        { token: incomingInvite.token },
      );
      setNotice(
        "Invitación aceptada. Ya puedes operar en nombre de la empresa.",
      );
      setIncomingInvite(null);
      window.history.replaceState({}, "", "/anfitrion/empresas");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  async function submitJoinRequest(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token) return;
    setError("");
    setNotice("");
    if (!isValidRut(newCompanyRut)) {
      setError("Ingresa el RUT válido de la empresa que ya está registrada.");
      return;
    }
    setBusy(true);
    try {
      const result = await request<{
        request: { id: string; estado: string; expires_at?: string };
      }>(token, "/organization-join-requests", "POST", {
        company_rut: normalizeRut(newCompanyRut),
        rol_propuesto: joinRole,
      });
      setJoinRequest(result.request);
      setShowJoinRequest(false);
      setNotice(
        "Solicitud de adhesión enviada. Los representantes legales podrán revisarla sin que se revelen datos de la empresa ni de sus integrantes.",
      );
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  async function refreshJoinRequest() {
    if (!token || !joinRequest) return;
    setBusy(true);
    try {
      const result = await request<{
        request: { id: string; estado: string; expires_at?: string };
      }>(
        token,
        `/organization-join-requests/${encodeURIComponent(joinRequest.id)}`,
      );
      setJoinRequest(result.request);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  async function decideJoinRequest(
    requestId: string,
    decision: "approve" | "reject",
  ) {
    if (!token || !selectedId) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      if (decision === "approve") {
        const result = await request<{
          invitation: { id: string };
          invitation_token: string;
        }>(
          token,
          `/organizations/${encodeURIComponent(selectedId)}/join-requests/${encodeURIComponent(requestId)}/approve`,
          "POST",
          {},
        );
        const link = new URL("/anfitrion/empresas", window.location.origin);
        link.hash = new URLSearchParams({
          organizationId: selectedId,
          invitationId: result.invitation.id,
          token: result.invitation_token,
        }).toString();
        setInviteLink(link.toString());
        setNotice(
          "Solicitud aprobada. Comparte el enlace para que la persona invitada acepte y se integre a la empresa.",
        );
      } else {
        await request(
          token,
          `/organizations/${encodeURIComponent(selectedId)}/join-requests/${encodeURIComponent(requestId)}/reject`,
          "POST",
          {},
        );
        setNotice("Solicitud rechazada.");
      }
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  }

  async function changeMember(memberId: string, role?: string) {
    if (!token || !selectedId) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await request(
        token,
        `/organizations/${selectedId}/members/${memberId}`,
        role ? "PATCH" : "DELETE",
        role ? { rol: role } : undefined,
      );
      await load();
      setNotice(role ? "Rol actualizado." : "Acceso retirado.");
    } catch (cause) {
      const error = cause as Error & { status?: number };
      setError(
        error.status === 409
          ? "La empresa debe conservar al menos un representante legal activo."
          : error.message,
      );
    } finally {
      setBusy(false);
    }
  }

  const selected = organizations.find((org) => orgId(org) === selectedId);
  const canManageTeam = selected?.my_role === "representante_legal";
  return (
    <div className="space-y-6">
      {incomingInvite ? (
        <Card>
          <CardHeader>
            <CardTitle>Invitación a una empresa</CardTitle>
            <CardDescription>
              Aceptar vincula tu cuenta personal como integrante de la empresa.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              type="button"
              disabled={busy}
              onClick={() => void acceptInvitation()}
            >
              Aceptar invitación
            </Button>
          </CardContent>
        </Card>
      ) : null}
      {error ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900"
        >
          <AlertCircle className="mt-0.5 size-4 shrink-0" />
          {error}
        </div>
      ) : null}
      {notice ? (
        <div
          role="status"
          className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900"
        >
          {notice}
        </div>
      ) : null}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Building2 className="size-5" />
                Perfiles de empresa
              </CardTitle>
              <CardDescription>
                El RUT identifica una empresa única. Cada integrante accede con
                su propia cuenta personal.
              </CardDescription>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void load()}
              disabled={busy}
            >
              <RefreshCw
                className={`mr-2 size-4 ${busy ? "animate-spin" : ""}`}
              />
              Actualizar
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {organizations.length ? (
            <div
              className="mb-5 flex flex-wrap gap-2"
              aria-label="Seleccionar empresa"
            >
              {organizations.map((org) => (
                <Button
                  key={org.id}
                  type="button"
                  size="sm"
                  variant={selectedId === org.id ? "default" : "outline"}
                  onClick={() => setSelectedId(org.id)}
                >
                  {orgName(org)}
                </Button>
              ))}
            </div>
          ) : (
            <p className="mb-5 text-sm text-muted-foreground">
              Aún no tienes empresas vinculadas. Puedes registrar una empresa o
              aceptar una invitación.
            </p>
          )}
          {selected ? (
            <form
              onSubmit={saveProfile}
              className="grid gap-4 rounded-lg border p-4"
            >
              <h3 className="font-medium">Perfil comercial público</h3>
              <p className="text-sm text-muted-foreground">
                El nombre comercial aparece en las publicaciones de esta
                empresa.
              </p>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="org-legal">Razón social</Label>
                  <Input
                    id="org-legal"
                    value={legalName}
                    readOnly
                    required
                    maxLength={200}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="org-trade">Nombre comercial</Label>
                  <Input
                    id="org-trade"
                    value={tradeName}
                    onChange={(e) => setTradeName(e.target.value)}
                    readOnly={!canManageTeam}
                    maxLength={200}
                  />
                </div>
              </div>
              <div>
                <Button type="submit" disabled={busy || !canManageTeam}>
                  {busy ? (
                    <Loader2 className="mr-2 size-4 animate-spin" />
                  ) : null}
                  Guardar perfil
                </Button>
              </div>
            </form>
          ) : null}
        </CardContent>
      </Card>
      {selected ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="size-5" />
              Equipo y representantes
            </CardTitle>
            <CardDescription>
              Los representantes legales firman contratos en nombre de la
              empresa. Mantén al menos uno activo.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {canManageTeam && organizationJoinRequests.length > 0 ? (
              <section className="space-y-3 rounded-lg border p-4">
                <div>
                  <h3 className="font-medium">Solicitudes de adhesión</h3>
                  <p className="text-sm text-muted-foreground">
                    Revisa las solicitudes de personas que afirmaron representar
                    a esta empresa. Aprobar crea una invitación; no añade
                    integrantes hasta que acepten el enlace.
                  </p>
                </div>
                {organizationJoinRequests.map((join) => (
                  <div
                    key={join.id}
                    className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3 text-sm"
                  >
                    <div>
                      <p className="font-medium">
                        {roleLabel(join.rol_propuesto)} ·{" "}
                        {joinStatusLabel(join.estado)}
                      </p>
                      {join.created_at ? (
                        <p className="text-muted-foreground">
                          Recibida:{" "}
                          {new Date(join.created_at).toLocaleDateString(
                            "es-CL",
                          )}
                        </p>
                      ) : null}
                    </div>
                    {join.estado === "pendiente" ? (
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          size="sm"
                          disabled={busy}
                          onClick={() =>
                            void decideJoinRequest(join.id, "approve")
                          }
                        >
                          Aprobar y preparar invitación
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() =>
                            void decideJoinRequest(join.id, "reject")
                          }
                        >
                          Rechazar
                        </Button>
                      </div>
                    ) : null}
                  </div>
                ))}
              </section>
            ) : null}
            <ul className="divide-y rounded-lg border">
              {members.map((member, index) => (
                <li
                  key={member.id || member.member_id || index}
                  className="flex flex-wrap items-center justify-between gap-2 p-3 text-sm"
                >
                  <span>
                    {member.display_name ||
                      member.email ||
                      `Integrante ${index + 1}`}
                  </span>
                  <span className="text-muted-foreground">
                    {roleLabel(member.role)}
                    {member.status ? ` · ${member.status}` : ""}
                  </span>
                  {canManageTeam && member.id && member.status === "activo" ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <select
                        aria-label={`Rol de integrante ${index + 1}`}
                        className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                        value={member.role}
                        disabled={busy}
                        onChange={(event) =>
                          void changeMember(member.id!, event.target.value)
                        }
                      >
                        <option value="representante_legal">
                          Representante legal
                        </option>
                        <option value="administrador">Administrador</option>
                        <option value="operador">Operador</option>
                      </select>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={busy}
                        onClick={() => void changeMember(member.id!)}
                      >
                        Retirar acceso
                      </Button>
                    </div>
                  ) : null}
                </li>
              ))}
              {members.length === 0 ? (
                <li className="p-3 text-sm text-muted-foreground">
                  No hay integrantes para mostrar.
                </li>
              ) : null}
            </ul>
            {canManageTeam ? (
              <form
                onSubmit={invite}
                className="grid gap-3 sm:grid-cols-[1fr_200px_auto]"
              >
                <div className="space-y-2">
                  <Label htmlFor="invite-email">Correo de la persona</Label>
                  <Input
                    id="invite-email"
                    type="email"
                    value={inviteEmail}
                    onChange={(e) => setInviteEmail(e.target.value)}
                    placeholder="persona@empresa.cl"
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="invite-role">Rol</Label>
                  <select
                    id="invite-role"
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                    value={inviteRole}
                    onChange={(e) => setInviteRole(e.target.value)}
                  >
                    <option value="representante_legal">
                      Representante legal
                    </option>
                    <option value="administrador">Administrador</option>
                    <option value="operador">Operador</option>
                  </select>
                </div>
                <div className="flex items-end">
                  <Button type="submit" disabled={busy}>
                    <Send className="mr-2 size-4" />
                    Invitar
                  </Button>
                </div>
              </form>
            ) : null}
            {inviteLink ? (
              <div className="space-y-2 rounded-lg border bg-muted/30 p-3">
                <Label htmlFor="invite-link">
                  Enlace de invitación de un solo uso
                </Label>
                <div className="flex gap-2">
                  <Input id="invite-link" readOnly value={inviteLink} />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      void navigator.clipboard.writeText(inviteLink)
                    }
                  >
                    <Clipboard className="mr-2 size-4" />
                    Copiar
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Comparte el enlace únicamente con la persona invitada. El
                  token permite aceptar la invitación.
                </p>
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Plus className="size-5" />
            Registrar empresa
          </CardTitle>
          <CardDescription>
            Si el RUT ya está registrado, EspaciGo bloqueará el duplicado y
            protegerá la información de sus miembros.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={createOrganization} className="grid gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="new-legal">Razón social</Label>
                <Input
                  id="new-legal"
                  value={newLegalName}
                  onChange={(e) => setNewLegalName(e.target.value)}
                  required
                  maxLength={200}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-trade">Nombre comercial</Label>
                <Input
                  id="new-trade"
                  value={newTradeName}
                  onChange={(e) => setNewTradeName(e.target.value)}
                  maxLength={200}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-rut">RUT de la empresa</Label>
                <Input
                  id="new-rut"
                  value={newCompanyRut}
                  onChange={(e) => setNewCompanyRut(e.target.value)}
                  placeholder="76.123.456-0"
                  required
                />
              </div>
            </div>
            <div>
              <Button type="submit" disabled={busy}>
                {busy ? (
                  <Loader2 className="mr-2 size-4 animate-spin" />
                ) : (
                  <Plus className="mr-2 size-4" />
                )}
                Crear empresa
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
      {showJoinRequest ? (
        <Card>
          <CardHeader>
            <CardTitle>Solicitar adhesión a una empresa existente</CardTitle>
            <CardDescription>
              Se enviará una solicitud privada a sus representantes legales. No
              recibirás el nombre ni los datos de contacto de sus integrantes.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={submitJoinRequest}
              className="grid gap-4 sm:grid-cols-[1fr_240px_auto]"
            >
              <div className="space-y-2">
                <Label htmlFor="join-rut">RUT de la empresa</Label>
                <Input
                  id="join-rut"
                  value={newCompanyRut}
                  onChange={(event) => setNewCompanyRut(event.target.value)}
                  placeholder="76.123.456-0"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="join-role">Rol solicitado</Label>
                <select
                  id="join-role"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm"
                  value={joinRole}
                  onChange={(event) => setJoinRole(event.target.value)}
                >
                  <option value="representante_legal">
                    Representante legal
                  </option>
                  <option value="administrador">Administrador</option>
                  <option value="operador">Operador</option>
                </select>
              </div>
              <div className="flex items-end">
                <Button type="submit" disabled={busy}>
                  Enviar solicitud
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}
      {joinRequest ? (
        <Card>
          <CardHeader>
            <CardTitle>Estado de solicitud de adhesión</CardTitle>
            <CardDescription>
              {joinStatusLabel(joinRequest.estado)}
              {joinRequest.expires_at
                ? ` · vence ${new Date(joinRequest.expires_at).toLocaleDateString("es-CL")}`
                : ""}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void refreshJoinRequest()}
            >
              <RefreshCw className="mr-2 size-4" />
              Actualizar estado
            </Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
function roleLabel(role?: string) {
  return (
    (
      {
        representante_legal: "Representante legal",
        administrador: "Administrador",
        operador: "Operador",
      } as Record<string, string>
    )[role || ""] ||
    role ||
    "Miembro"
  );
}
function joinStatusLabel(status: string) {
  return (
    (
      {
        pendiente: "Pendiente de revisión",
        aceptada: "Aprobada; invitación pendiente de aceptación",
        rechazada: "Rechazada",
        expirada: "Vencida",
      } as Record<string, string>
    )[status] || status
  );
}
