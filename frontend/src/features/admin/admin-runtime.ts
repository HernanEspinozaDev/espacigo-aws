import "client-only";

import { privateBlobUrls } from "@/lib/api/private-blob-urls";
import { AdminFacade } from "./application/admin-facade";
import { OpenApiAdminAdapter } from "./infrastructure/admin-openapi-adapter";

export const adminAdapter = new OpenApiAdminAdapter();
export const adminFacade = new AdminFacade(adminAdapter, undefined, () =>
  privateBlobUrls.clear(),
);
