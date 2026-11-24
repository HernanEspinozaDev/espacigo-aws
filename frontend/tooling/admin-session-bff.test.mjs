import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { pathToFileURL } from "node:url";

const frontendRoot = path.resolve(import.meta.dirname, "..");
const scratch = await mkdtemp(path.join(os.tmpdir(), "admin-bff-test-"));

async function compile(sourcePath, outputName, replacements = []) {
  let source = await readFile(path.join(frontendRoot, sourcePath), "utf8");
  for (const [before, after] of replacements) {
    source = source.replaceAll(before, after);
  }
  const output = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
    },
  }).outputText;
  const destination = path.join(scratch, outputName);
  await writeFile(destination, output, "utf8");
  return destination;
}

await writeFile(
  path.join(scratch, "private-blob-urls.mjs"),
  "export const privateBlobUrls = { clearCount: 0, clear() { this.clearCount++; } };",
  "utf8",
);

const serverSessionPath = await compile(
  "src/lib/api/server-session.ts",
  "server-session.mjs",
  [['import "server-only";', ""]],
);

await compile("src/features/admin/domain/admin-rules.ts", "admin-rules.mjs");

const facadePath = await compile(
  "src/features/admin/application/admin-facade.ts",
  "admin-facade.mjs",
  [
    ['import "client-only";', ""],
    ["@/lib/api/private-blob-urls", "./private-blob-urls.mjs"],
    ["../domain/admin-rules", "./admin-rules.mjs"],
  ],
);

test.after(async () => {
  await rm(scratch, { recursive: true, force: true });
});

test("espacigo_admin_session cookie is isolated from commercial session cookie", async () => {
  const {
    ADMIN_SESSION_COOKIE_NAME,
    SESSION_COOKIE_NAME,
    buildAdminSessionCookie,
    buildClearAdminSessionCookie,
    parseAdminSessionCookie,
    parseSessionCookies,
  } = await import(pathToFileURL(serverSessionPath).href);

  const token = "adm_tok_1234567890abcdef";
  const futureIso = new Date(Date.now() + 3600_000).toISOString();
  const cookie = buildAdminSessionCookie(token, futureIso);

  assert.ok(cookie.startsWith(`${ADMIN_SESSION_COOKIE_NAME}=${token};`));
  assert.ok(cookie.includes("HttpOnly"));
  assert.ok(cookie.includes("SameSite=Lax"));
  assert.ok(cookie.includes("Path=/"));

  assert.equal(
    parseAdminSessionCookie(`${ADMIN_SESSION_COOKIE_NAME}=${token}`),
    token,
  );
  assert.equal(
    parseAdminSessionCookie(`${SESSION_COOKIE_NAME}=${token}`),
    null,
    "commercial session cookie must never satisfy admin session",
  );
  assert.equal(
    parseSessionCookies(`${ADMIN_SESSION_COOKIE_NAME}=${token}`),
    null,
    "admin session cookie must never satisfy commercial session",
  );

  const cleared = buildClearAdminSessionCookie();
  assert.ok(cleared.startsWith(`${ADMIN_SESSION_COOKIE_NAME}=;`));
  assert.ok(cleared.includes("Max-Age=0"));
});

test("AdminFacade dedicated admin session handles MFA challenge and 428 step-up", async () => {
  const { AdminFacade } = await import(pathToFileURL(facadePath).href);

  let stepUpResolved = false;
  const mockPort = {
    async startAdminSession() {
      return {
        status: "mfa_required",
        challenge_id: "chg-123",
        methods: ["totp"],
        expires_at: "2026-11-24T12:00:00Z",
      };
    },
    async verifyAdminSessionMfa(input) {
      if (input.challenge_id === "chg-123") {
        return {
          account_id: "00000000-0000-4000-8000-000000000001",
          audience: "admin",
          roles: ["administrador_global"],
          capabilities: ["accounts.read", "accounts.email.reveal"],
          mfa_verified_at: "2026-11-24T09:00:00Z",
          recent_auth_expires_at: "2026-11-24T09:10:00Z",
          expires_at: "2026-11-24T17:00:00Z",
        };
      }
      stepUpResolved = true;
      return {
        account_id: "00000000-0000-4000-8000-000000000001",
        audience: "admin",
        roles: ["administrador_global"],
        capabilities: ["accounts.read", "accounts.email.reveal"],
        mfa_verified_at: "2026-11-24T09:05:00Z",
        recent_auth_expires_at: "2026-11-24T09:15:00Z",
        expires_at: "2026-11-24T17:00:00Z",
      };
    },
    async requestGlobalAccountSensitiveAccess(accountId, input) {
      if (!stepUpResolved) {
        const err = new Error("recent auth required");
        err.status = 428;
        err.code = "recent_authentication_required";
        throw err;
      }
      return {
        account_id: accountId,
        field: input.field,
        value: "user@example.com",
        masked_value: "u***@example.com",
        scope: input.scope ?? "account_email",
        reason: input.reason,
        expires_at: "2026-11-24T09:10:00Z",
      };
    },
    async deleteAdminSession() {},
  };

  const facade = new AdminFacade(mockPort);
  const startRes = await facade.startAdminSession(
    "staff@espacigo.cl",
    "secret",
  );
  assert.equal(startRes?.status, "mfa_required");
  assert.equal(facade.getSnapshot().adminSessionStatus, "mfa_required");

  const mfaOk = await facade.verifyAdminSessionMfa("654321");
  assert.equal(mfaOk, true);
  assert.equal(facade.getSnapshot().adminSessionStatus, "authenticated");

  await assert.rejects(
    () =>
      facade.requestGlobalAccountSensitiveAccess("acc-1", {
        field: "email",
        reason: "Verificación de ticket",
        scope: "account_email",
      }),
    (err) => err.status === 428,
  );
  assert.equal(facade.getSnapshot().mfaStepUpRequired, true);
  assert.equal(facade.getSnapshot().recentAuthRequired, true);

  const stepUpOk = await facade.verifyAdminSessionMfa("654321");
  assert.equal(stepUpOk, true);
  assert.equal(facade.getSnapshot().mfaStepUpRequired, false);

  const revealed = await facade.requestGlobalAccountSensitiveAccess("acc-1", {
    field: "email",
    reason: "Verificación de ticket",
    scope: "account_email",
  });
  assert.equal(revealed?.value, "user@example.com");

  await facade.logoutAdminSession();
  assert.equal(facade.getSnapshot().adminSessionStatus, "anonymous");
  assert.equal(facade.getSnapshot().adminSession, null);
});
