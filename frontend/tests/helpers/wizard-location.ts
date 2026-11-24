import { expect, type Page } from "@playwright/test";
import centers from "../../src/features/public-catalog/domain/commune-centers.json";

type CommuneCode = keyof typeof centers;

export async function chooseWizardLocation(
  page: Page,
  communeCode: CommuneCode,
  controlled = false,
) {
  const [latitude, longitude] = centers[communeCode];
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude))
    throw new Error("Fixture commune requires a valid reference point");
  if (controlled) {
    await page.route(
      "**/api/backend/spaces/location/reverse-geocode",
      (route) =>
        route.fulfill({
          json: {
            region_code: communeCode.slice(0, 2),
            comuna_code: communeCode,
          },
        }),
    );
  }
  await page.context().grantPermissions(["geolocation"], {
    origin: new URL(page.url()).origin,
  });
  await page.context().setGeolocation({ latitude, longitude, accuracy: 20 });
  await page.getByRole("button", { name: "Usar mi ubicación" }).click();
  await expect(page.locator("#wizard-comuna")).not.toHaveText(
    /Detectando|Pendiente/,
  );
}
