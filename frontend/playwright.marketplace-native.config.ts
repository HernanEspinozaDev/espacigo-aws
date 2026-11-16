import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "marketplace-local-readonly.spec.ts",
  workers: 1,
  reporter: "list",
  outputDir: "test-results/marketplace-native-readonly",
  use: {
    baseURL: "http://127.0.0.1:4313",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command:
      "API_BACKEND_BASE_URL=http://127.0.0.1:18122/api/v1 HOSTNAME=127.0.0.1 PORT=4313 pnpm start:local",
    url: "http://127.0.0.1:4313/api/health",
    reuseExistingServer: false,
    timeout: 60000,
  },
});
