import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "public-rental-commercial-journey.spec.ts",
  fullyParallel: false,
  reporter: "list",
  outputDir: "test-results/host-checkout-api",
  use: {
    baseURL: "http://127.0.0.1:3000",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
