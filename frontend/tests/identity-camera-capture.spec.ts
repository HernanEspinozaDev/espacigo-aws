import {
  expect,
  test,
  type Page,
  type Request,
  type Route,
} from "@playwright/test";
import { createBffFixtureState } from "./helpers/bff-fixture";

const accountId = "550e8400-e29b-41d4-a716-446655440041";
const caseId = "770e8400-e29b-41d4-a716-446655440041";
const expiresAt = new Date(Date.now() + 3_600_000).toISOString();

type Reply = {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
};

async function installIdentityFixture(
  page: Page,
  respond: (path: string, method: string, request: Request) => Reply,
) {
  const fixture = createBffFixtureState();
  await page.route("**/api/backend/**", async (route: Route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let reply: Reply;
    if (path === "/api/backend/auth/login" && request.method() === "POST") {
      reply = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          expires_at: expiresAt,
          restricted_mode: false,
        },
      };
    } else if (
      path === "/api/backend/auth/session" &&
      request.method() === "GET"
    ) {
      reply = {
        status: 200,
        body: {
          account_id: accountId,
          roles: ["arrendatario"],
          capabilities: [],
          expires_at: expiresAt,
          restricted_mode: false,
        },
      };
    } else {
      reply = respond(path, request.method(), request);
    }
    const adapted = fixture.adapt(request, reply);
    await route.fulfill({
      status: adapted?.status ?? 500,
      headers: {
        "content-type": "application/json; charset=utf-8",
        "cache-control": "no-store",
        ...adapted?.headers,
      },
      body: JSON.stringify(adapted?.body ?? {}),
    });
  });
}

async function loginOnVerificationPage(page: Page) {
  await page.goto(
    `/acceso?return_to=${encodeURIComponent("/mi-cuenta/verificaciones")}`,
  );
  await page.getByLabel("Correo electrónico").fill("camera@example.invalid");
  await page
    .getByLabel("Contraseña", { exact: true })
    .fill("SyntheticOnly123!");
  await page.getByRole("button", { name: "Iniciar sesión" }).click();
  await page.waitForURL((url) => url.pathname === "/mi-cuenta/verificaciones");
  await expect(
    page.getByRole("heading", { level: 1, name: "Verificación de identidad" }),
  ).toBeVisible();
}

function identityCase(files: unknown[] = []) {
  return {
    id: caseId,
    purpose: "identity",
    state: "borrador",
    revision: 1,
    document_format_declared: "unsure",
    files,
  };
}

async function installSyntheticCamera(
  page: Page,
  permission: "granted" | "denied" | "missing" | "late" = "granted",
) {
  await page.addInitScript((cameraPermission) => {
    const state = {
      facingModes: [] as string[],
      stoppedTracks: 0,
      releaseLateStream: null as null | (() => void),
    };
    Object.defineProperty(window, "__identityCameraTestState", {
      configurable: false,
      value: state,
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async (constraints: MediaStreamConstraints) => {
        if (cameraPermission === "denied") {
          throw new DOMException(
            "Synthetic camera permission denied",
            "NotAllowedError",
          );
        }
        if (cameraPermission === "missing") {
          throw new DOMException(
            "Synthetic camera is unavailable",
            "NotFoundError",
          );
        }
        const facingMode =
          constraints.video && typeof constraints.video === "object"
            ? (constraints.video as MediaTrackConstraints).facingMode
            : undefined;
        const constrainedMode =
          typeof facingMode === "object" &&
          facingMode !== null &&
          !Array.isArray(facingMode)
            ? (facingMode as ConstrainDOMStringParameters)
            : undefined;
        state.facingModes.push(
          String(
            constrainedMode?.ideal ??
              constrainedMode?.exact ??
              facingMode ??
              "",
          ),
        );
        const source = document.createElement("canvas");
        source.width = 640;
        source.height = 480;
        const context = source.getContext("2d");
        if (context) {
          context.fillStyle = "#22c55e";
          context.fillRect(0, 0, source.width, source.height);
          context.fillStyle = "#0f172a";
          context.font = "bold 36px sans-serif";
          context.fillText("SYNTHETIC CAMERA", 24, 80);
        }
        const stream = source.captureStream(5);
        for (const track of stream.getTracks()) {
          const originalStop = track.stop.bind(track);
          Object.defineProperty(track, "stop", {
            configurable: true,
            value: () => {
              state.stoppedTracks += 1;
              originalStop();
            },
          });
        }
        if (cameraPermission === "late") {
          return await new Promise<MediaStream>((resolve) => {
            state.releaseLateStream = () => resolve(stream);
          });
        }
        return stream;
      },
    });
  }, permission);
}

test("synthetic identity camera switches facing mode, captures through the existing upload, and stops tracks", async ({
  page,
}) => {
  await installSyntheticCamera(page);
  let uploadedRoles: string[] = [];
  let currentFiles: unknown[] = [];
  let currentEtag = '"1"';

  await installIdentityFixture(page, (path, method, request) => {
    if (
      path === "/api/backend/document-verification/cases" &&
      method === "GET"
    ) {
      return { status: 200, body: [identityCase(currentFiles)] };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}` &&
      method === "GET"
    ) {
      return {
        status: 200,
        body: identityCase(currentFiles),
        headers: { etag: currentEtag },
      };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}/files` &&
      method === "POST"
    ) {
      const body = request.postData() ?? "";
      const role =
        body.match(/name="evidence_role"\r?\n\r?\n([^\r\n]+)/)?.[1] ?? "";
      const filename = body.match(/filename="([^"]+)"/)?.[1] ?? "";
      expect(body).toContain("Content-Type: image/jpeg");
      expect(filename).toMatch(/^identity_(front|back)\.jpg$/);
      expect(["identity_front", "identity_back"]).toContain(role);
      uploadedRoles = [...uploadedRoles, role];
      const file = {
        id: `synthetic-${role}`,
        evidence_role: role,
        state: "pendiente",
        mime: "image/jpeg",
        size: 1024,
        pages: 0,
      };
      currentFiles = [
        ...currentFiles.filter(
          (item) => (item as { evidence_role?: string }).evidence_role !== role,
        ),
        file,
      ];
      currentEtag = '"2"';
      return {
        status: 201,
        body: { revision: 2, file },
        headers: { etag: currentEtag },
      };
    }
    return { status: 404, body: { error: { code: "not_found" } } };
  });

  await loginOnVerificationPage(page);
  await expect(
    page.getByRole("button", { name: "Abrir cámara para anverso" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Abrir cámara para selfie" }),
  ).toHaveCount(0);
  await expect(
    page.getByLabel("Imagen JPEG o PNG · máximo 10 MiB"),
  ).toBeVisible();

  await page.getByRole("button", { name: "Abrir cámara para anverso" }).click();
  await expect(page.getByRole("button", { name: "Tomar foto" })).toBeVisible();
  await expect(page.locator("video")).toBeVisible();
  await page.screenshot({
    path: "/tmp/espacigo-identity-camera-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: "/tmp/espacigo-identity-camera-mobile.png",
    fullPage: true,
  });

  await page.getByRole("button", { name: "Cambiar cámara" }).click();
  await expect
    .poll(() =>
      page.evaluate(() =>
        (
          window as Window & {
            __identityCameraTestState?: { facingModes: string[] };
          }
        ).__identityCameraTestState?.facingModes.at(-1),
      ),
    )
    .toBe("user");
  expect(
    await page.evaluate(
      () =>
        (
          window as Window & {
            __identityCameraTestState?: { facingModes: string[] };
          }
        ).__identityCameraTestState?.facingModes[0],
    ),
  ).toBe("environment");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as Window & {
              __identityCameraTestState?: { stoppedTracks: number };
            }
          ).__identityCameraTestState?.stoppedTracks,
      ),
    )
    .toBeGreaterThanOrEqual(1);

  await page
    .getByRole("button", { name: "Tomar foto" })
    .evaluate((button: HTMLButtonElement) => {
      button.click();
      button.click();
    });
  await expect(page.getByText("Foto capturada · Anverso")).toBeVisible();
  await expect(page.getByRole("button", { name: "Cerrar cámara" })).toHaveCount(
    0,
  );
  expect(uploadedRoles).toEqual([]);
  await page.getByRole("button", { name: "Repetir foto" }).click();
  await expect(
    page.getByRole("region", { name: "Cámara para capturar el anverso" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Cerrar cámara" }).click();
  await expect(page.getByText("Foto capturada · Anverso")).toBeVisible();
  expect(uploadedRoles).toEqual([]);
  await page.getByRole("button", { name: "Repetir foto" }).click();
  await expect(
    page.getByRole("region", { name: "Cámara para capturar el anverso" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Tomar foto" }).click();
  await expect(page.getByText("Foto capturada · Anverso")).toBeVisible();
  expect(uploadedRoles).toEqual([]);
  await page.getByRole("button", { name: "Cargar evidencia" }).click();
  await expect(
    page.getByText("Evidencia recibida y pendiente de inspección."),
  ).toBeVisible();
  expect(uploadedRoles).toEqual(["identity_front"]);
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as Window & {
              __identityCameraTestState?: { stoppedTracks: number };
            }
          ).__identityCameraTestState?.stoppedTracks,
      ),
    )
    .toBeGreaterThanOrEqual(2);

  await page.getByLabel("Tipo de evidencia").selectOption("identity_back");
  await page.getByRole("button", { name: "Abrir cámara para reverso" }).click();
  await expect(page.getByRole("button", { name: "Tomar foto" })).toBeVisible();
  await page.getByRole("button", { name: "Tomar foto" }).click();
  await page.getByRole("button", { name: "Cargar evidencia" }).click();
  await expect(
    page.getByText("Evidencia recibida y pendiente de inspección."),
  ).toBeVisible();
  expect(uploadedRoles).toEqual(["identity_front", "identity_back"]);
});

test("camera permission denial preserves the existing file-upload fallback", async ({
  page,
}) => {
  await installSyntheticCamera(page, "denied");
  let uploadCount = 0;
  await installIdentityFixture(page, (path, method, request) => {
    if (
      path === "/api/backend/document-verification/cases" &&
      method === "GET"
    ) {
      return { status: 200, body: [identityCase()] };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}` &&
      method === "GET"
    ) {
      return { status: 200, body: identityCase(), headers: { etag: '"1"' } };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}/files` &&
      method === "POST"
    ) {
      uploadCount += 1;
      expect(request.postData()).toContain('name="evidence_role"');
      return {
        status: 201,
        body: {
          revision: 2,
          file: {
            id: "synthetic-upload-fallback",
            evidence_role: "identity_front",
            state: "pendiente",
            mime: "image/png",
            size: 128,
            pages: 0,
          },
        },
        headers: { etag: '"2"' },
      };
    }
    return { status: 404, body: { error: { code: "not_found" } } };
  });

  await loginOnVerificationPage(page);
  await page.getByRole("button", { name: "Abrir cámara para anverso" }).click();
  await expect(
    page
      .getByRole("region", { name: "Cámara para capturar el anverso" })
      .getByRole("alert"),
  ).toContainText(/cámara|permiso/i);
  await expect(
    page.getByLabel("Imagen JPEG o PNG · máximo 10 MiB"),
  ).toBeVisible();
  await page.getByLabel("Imagen JPEG o PNG · máximo 10 MiB").setInputFiles({
    name: "synthetic-fallback.png",
    mimeType: "image/png",
    buffer: Buffer.from("synthetic image bytes"),
  });
  await page.getByRole("button", { name: "Cargar evidencia" }).click();
  await expect(
    page.getByText("Evidencia recibida y pendiente de inspección."),
  ).toBeVisible();
  expect(uploadCount).toBe(1);
});

test("unavailable camera explains the fallback without creating an upload", async ({
  page,
}) => {
  await installSyntheticCamera(page, "missing");
  let uploadCount = 0;
  await installIdentityFixture(page, (path, method) => {
    if (
      path === "/api/backend/document-verification/cases" &&
      method === "GET"
    ) {
      return { status: 200, body: [identityCase()] };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}` &&
      method === "GET"
    ) {
      return { status: 200, body: identityCase(), headers: { etag: '"1"' } };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}/files` &&
      method === "POST"
    ) {
      uploadCount += 1;
      return { status: 500, body: { error: { code: "unexpected_upload" } } };
    }
    return { status: 404, body: { error: { code: "not_found" } } };
  });

  await loginOnVerificationPage(page);
  await page.getByRole("button", { name: "Abrir cámara para anverso" }).click();
  await expect(
    page
      .getByRole("region", { name: "Cámara para capturar el anverso" })
      .getByRole("alert"),
  ).toContainText(/no se pudo acceder a la cámara/i);
  await expect(
    page.getByLabel("Imagen JPEG o PNG · máximo 10 MiB"),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Cerrar cámara" }),
  ).toBeVisible();
  expect(uploadCount).toBe(0);
});

test("closing while camera startup is pending stops a late stream and cancels preview", async ({
  page,
}) => {
  await installSyntheticCamera(page, "late");
  await installIdentityFixture(page, (path, method) => {
    if (
      path === "/api/backend/document-verification/cases" &&
      method === "GET"
    ) {
      return { status: 200, body: [identityCase()] };
    }
    if (
      path === `/api/backend/document-verification/cases/${caseId}` &&
      method === "GET"
    ) {
      return { status: 200, body: identityCase(), headers: { etag: '"1"' } };
    }
    return { status: 404, body: { error: { code: "not_found" } } };
  });

  await loginOnVerificationPage(page);
  await page.getByRole("button", { name: "Abrir cámara para anverso" }).click();
  await expect(page.getByText("Abriendo cámara…")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as Window & {
              __identityCameraTestState?: {
                releaseLateStream: null | (() => void);
              };
            }
          ).__identityCameraTestState?.releaseLateStream !== null,
      ),
    )
    .toBe(true);

  await page.getByRole("button", { name: "Cerrar cámara" }).click();
  await expect(
    page.getByRole("region", { name: "Cámara para capturar el anverso" }),
  ).toHaveCount(0);
  await page.evaluate(() =>
    (
      window as Window & {
        __identityCameraTestState?: {
          releaseLateStream: null | (() => void);
        };
      }
    ).__identityCameraTestState?.releaseLateStream?.(),
  );
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as Window & {
              __identityCameraTestState?: { stoppedTracks: number };
            }
          ).__identityCameraTestState?.stoppedTracks,
      ),
    )
    .toBeGreaterThanOrEqual(1);
  await expect(page.locator("video")).toHaveCount(0);
});
