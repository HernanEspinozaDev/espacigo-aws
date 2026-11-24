import "client-only";
import { authFacade } from "@/features/auth/auth-runtime";
import { AgreementsService } from "../application/agreements.service";
import { OpenApiAgreementsAdapter } from "./agreements-openapi-adapter";
export const commercialAgreements = new AgreementsService(
  new OpenApiAgreementsAdapter(),
  (operation) =>
    authFacade.runAuthenticatedOperation((_token, signal) => operation(signal)),
);
