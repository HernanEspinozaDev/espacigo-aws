import { createRequire } from "node:module";
import process from "node:process";

const require = createRequire(`${process.cwd()}/frontend/package.json`);
const { chromium } = require("@playwright/test");
const target = process.argv[2];

if (!target) {
  throw new Error("Indica la URL local que se debe comprobar");
}

const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  const failures = [];
  let documentRequests = 0;
  let hmrConnections = 0;

  page.on("pageerror", (error) => failures.push(error.message));
  page.on("response", (response) => {
    if (response.url().includes("/_next/static/") && response.status() >= 400) {
      failures.push(`El recurso estático devolvió HTTP ${response.status()}: ${new URL(response.url()).pathname}`);
    }
  });
  page.on("request", (request) => {
    if (request.isNavigationRequest() && request.resourceType() === "document") documentRequests += 1;
  });
  page.on("websocket", (socket) => {
    if (socket.url().includes("/_next/") && /hmr|hot-update/i.test(socket.url())) {
      hmrConnections += 1;
    }
  });

  const response = await page.goto(target, { waitUntil: "domcontentloaded", timeout: 15_000 });
  if (!response || response.status() !== 200) {
    throw new Error(`La página devolvió HTTP ${response?.status() ?? "sin respuesta"}`);
  }
  await page.locator("body").waitFor({ state: "visible", timeout: 10_000 });
  const loginForm = page.locator("form").filter({ has: page.locator('input[type="email"]') });
  if ((await loginForm.count()) !== 1 || (await loginForm.locator('button[type="submit"], input[type="submit"]').count()) < 1) {
    failures.push("No se mostró el formulario de inicio de sesión con correo y botón de envío");
  }
  await page.waitForTimeout(8_000);
  if ((await page.locator("body").innerText()).includes("Comprobando sesión")) {
    failures.push("La pantalla quedó detenida en la comprobación de sesión");
  }

  if (documentRequests !== 1) {
    failures.push(`La página principal solicitó el documento ${documentRequests} veces`);
  }
  if (hmrConnections !== 0) {
    failures.push(`Se observaron ${hmrConnections} conexiones HMR`);
  }
  if (failures.length > 0) {
    throw new Error(failures.join("; "));
  }

  console.log(`Smoke de navegador correcto para ${new URL(target).pathname}.`);
} finally {
  await browser.close();
}
