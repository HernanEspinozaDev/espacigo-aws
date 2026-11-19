import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import ts from "typescript";
import { pathToFileURL } from "node:url";

const root = path.resolve(import.meta.dirname, "..");
const scratch = await mkdtemp(
  path.join(process.env.TMPDIR ?? os.tmpdir(), "espacigo-config-health-"),
);

const serverConfigSource = (
  await readFile(path.join(root, "src/lib/api/server-config.ts"), "utf8")
).replace('import "server-only";', "");

const compiledConfig = ts.transpileModule(serverConfigSource, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;

const serverConfigPath = path.join(scratch, "server-config.mjs");
await writeFile(serverConfigPath, compiledConfig);

const healthRouteSource = (
  await readFile(path.join(root, "src/app/api/health/route.ts"), "utf8")
).replace(
  'import { validateServerConfig } from "@/lib/api/server-config";',
  'import { validateServerConfig } from "./server-config.mjs";',
);

const compiledHealth = ts.transpileModule(healthRouteSource, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ESNext,
  },
}).outputText;

const healthRoutePath = path.join(scratch, "health-route.mjs");
await writeFile(healthRoutePath, compiledHealth);

const {
  getEnvironment,
  isProductionEnvironment,
  getApiBackendBaseUrl,
  validateServerConfig,
} = await import(pathToFileURL(serverConfigPath));

const { GET, HEAD } = await import(pathToFileURL(healthRoutePath));

const originalEnv = { ...process.env };

test.afterEach(() => {
  for (const key of Object.keys(process.env)) {
    if (!(key in originalEnv)) {
      delete process.env[key];
    }
  }
  Object.assign(process.env, originalEnv);
});

test.after(async () => {
  await rm(scratch, { recursive: true, force: true });
});

test("getEnvironment defaults to development when unset", () => {
  delete process.env.ENVIRONMENT;
  process.env.NODE_ENV = "test";
  assert.equal(getEnvironment(), "development");
  assert.equal(isProductionEnvironment(), false);
});

test("getEnvironment accepts exact 'development' and 'production'", () => {
  process.env.ENVIRONMENT = "development";
  assert.equal(getEnvironment(), "development");
  assert.equal(isProductionEnvironment(), false);

  process.env.ENVIRONMENT = "production";
  assert.equal(getEnvironment(), "production");
  assert.equal(isProductionEnvironment(), true);
});

test("getEnvironment rejects invalid environments", () => {
  process.env.ENVIRONMENT = "staging";
  assert.throws(
    () => getEnvironment(),
    /ENVIRONMENT must be 'development' or 'production'/,
  );

  process.env.ENVIRONMENT = "qa";
  assert.throws(
    () => getEnvironment(),
    /ENVIRONMENT must be 'development' or 'production'/,
  );
});

test("getApiBackendBaseUrl returns default loopback in development when unset", () => {
  process.env.ENVIRONMENT = "development";
  delete process.env.API_BACKEND_BASE_URL;
  delete process.env.API_INTERNAL_BASE_URL;

  const url = getApiBackendBaseUrl();
  assert.equal(url.origin, "http://127.0.0.1:8080");
  assert.equal(url.pathname, "/api/v1");
});

test("getApiBackendBaseUrl requires API_BACKEND_BASE_URL in production", () => {
  process.env.ENVIRONMENT = "production";
  delete process.env.API_BACKEND_BASE_URL;
  delete process.env.API_INTERNAL_BASE_URL;

  assert.throws(
    () => getApiBackendBaseUrl(),
    /API_BACKEND_BASE_URL is not set/,
  );
});

test("validateServerConfig confirms valid development configuration", () => {
  process.env.ENVIRONMENT = "development";
  process.env.API_BACKEND_BASE_URL = "http://127.0.0.1:8080/api/v1";

  const result = validateServerConfig();
  assert.equal(result.valid, true);
  assert.equal(result.environment, "development");
  assert.equal(result.apiBackendBaseUrl, "http://127.0.0.1:8080/api/v1");
});

test("validateServerConfig rejects remote unencrypted backend in production", () => {
  process.env.ENVIRONMENT = "production";
  process.env.API_BACKEND_BASE_URL = "http://api.remote.invalid/api/v1";

  const result = validateServerConfig();
  assert.equal(result.valid, false);
  assert.equal(result.environment, "production");
  assert.match(result.error ?? "", /must use https:/);
});

test("health GET and HEAD return 200 with environment in valid configuration", async () => {
  process.env.ENVIRONMENT = "development";
  process.env.API_BACKEND_BASE_URL = "http://127.0.0.1:8080/api/v1";

  const res = GET();
  assert.equal(res.status, 200);
  assert.equal(
    res.headers.get("cache-control"),
    "no-store, no-cache, must-revalidate",
  );
  const data = await res.json();
  assert.equal(data.status, "ok");
  assert.equal(data.environment, "development");
  assert.ok(typeof data.timestamp === "string");
  assert.ok(typeof data.uptime === "number");

  const headRes = HEAD();
  assert.equal(headRes.status, 200);
});

test("health GET and HEAD return 503 when configuration is invalid", async () => {
  process.env.ENVIRONMENT = "invalid_env";

  const res = GET();
  assert.equal(res.status, 503);
  const data = await res.json();
  assert.equal(data.status, "error");
  assert.match(data.error, /ENVIRONMENT must be/);

  const headRes = HEAD();
  assert.equal(headRes.status, 503);
});
