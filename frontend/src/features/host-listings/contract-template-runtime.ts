import "client-only";
import { ContractTemplateService } from "./application/contract-template.service";
import { OpenApiContractTemplateAdapter } from "./infrastructure/contract-template-openapi-adapter";
export const contractTemplateService = new ContractTemplateService(
  new OpenApiContractTemplateAdapter(),
);
