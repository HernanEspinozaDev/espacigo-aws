import { defineConfig, devices } from "@playwright/test";
const appPort = Number(process.env.CATALOG_MAP_PORT ?? 4345);
const fixturePort = Number(process.env.CATALOG_MAP_FIXTURE_PORT ?? 18445);
export default defineConfig({
  testDir: "./tests",
  testMatch: "public-catalog-map.spec.ts",
  workers: 1,
  reporter: "list",
  outputDir: "test-results/catalog-map",
  use: { baseURL: `http://127.0.0.1:${appPort}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: `CATALOG_MAP_FIXTURE_PORT=${fixturePort} node tooling/public-catalog-map-fixture.mjs`,
      url: `http://127.0.0.1:${fixturePort}/healthz`,
      reuseExistingServer: false,
      timeout: 10000,
    },
    {
      command: `API_BACKEND_BASE_URL=http://127.0.0.1:${fixturePort}/api/v1 HOSTNAME=127.0.0.1 PORT=${appPort} pnpm start:local`,
      url: `http://127.0.0.1:${appPort}/api/health`,
      reuseExistingServer: false,
      timeout: 60000,
    },
  ],
});
