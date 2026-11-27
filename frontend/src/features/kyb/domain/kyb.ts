import type { components } from "@/lib/api/generated/openapi";

export type KYBVerification = components["schemas"]["KYBVerification"];
export type KYBDocument = components["schemas"]["KYBDocument"] & {
  review_status?: "PENDING" | "IN_REVIEW" | "APPROVED" | "REJECTED";
  rejection_reason?: string;
  reviewed_at?: string;
};
export type KYBStatus = KYBVerification["status"];
export interface KYBDocumentDecision {
  document_id: string;
  decision: "APPROVE" | "REJECT";
  rejection_reason?: string;
}

export interface KYBSubmissionInput {
  organizationId: string;
  legalName: string;
  companyRut: string;
  representativeName: string;
  representativeRut: string;
  siiGiroCode: string;
  erut: File;
  siiStartActivities: File;
  powerCertificate: File;
}

export function kybStatusLabel(status: KYBStatus): string {
  switch (status) {
    case "PENDING":
      return "Pendiente";
    case "IN_REVIEW":
      return "En revisión";
    case "APPROVED":
      return "Aprobado";
    case "REJECTED":
      return "Rechazado";
  }
}

export function kybStatusTone(
  status: KYBStatus,
): "pending" | "review" | "approved" | "rejected" {
  switch (status) {
    case "PENDING":
      return "pending";
    case "IN_REVIEW":
      return "review";
    case "APPROVED":
      return "approved";
    case "REJECTED":
      return "rejected";
  }
}
