import { defineConfig, devices } from "@playwright/test";

const appPort = 4312;
const fixturePort = 18121;
export default defineConfig({
  testDir: "./tests",
  testMatch: "marketplace-phase1.spec.ts",
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  outputDir: "test-results/marketplace-phase1",
  use: { baseURL: `http://127.0.0.1:${appPort}`, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: `MARKETPLACE_FIXTURE_PORT=${fixturePort} node tooling/marketplace-phase1-fixture.mjs`,
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
