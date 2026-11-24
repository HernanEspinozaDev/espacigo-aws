import { expect, test } from "@playwright/test";

test("anonymous mobile menu exposes Ingresar and closes when opening access", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.getByRole("button", { name: "Abrir menú" }).click();

  const drawer = page.getByRole("dialog");
  const loginLink = drawer.getByRole("link", {
    name: "Iniciar sesión",
  });
  await expect(loginLink).toBeVisible();
  await loginLink.click();

  await expect(page).toHaveURL(/\/acceso$/);
  await expect(
    page.getByRole("heading", { name: "Acceso a tu cuenta" }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Correo electrónico" }),
  ).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Contraseña" })).toBeVisible();
  await expect(drawer).toBeHidden();
});
