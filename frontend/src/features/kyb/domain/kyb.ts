import type { components } from "@/lib/api/generated/openapi";

export type KYBVerification = components["schemas"]["KYBVerification"];
export type KYBDocument = components["schemas"]["KYBDocument"];
export type KYBStatus = KYBVerification["status"];

export interface KYBSubmissionInput {
  legalName: string;
  companyRut: string;
  representativeName: string;
  representativeRut: string;
  siiGiroCode: string;
  erut: File;
  powerCertificate: File;
  propertyOrSubleaseAuthorization: File;
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

export function kybStatusTone(status: KYBStatus): "pending" | "review" | "approved" | "rejected" {
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
