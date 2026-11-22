import { defineConfig, devices } from "@playwright/test";

const appPort = 4210;

export default defineConfig({
  testDir: "./tests",
  testMatch: "wallet-purchase-choice.spec.ts",
  fullyParallel: false,
  reporter: "list",
  outputDir: "test-results/wallet-purchase",
  use: {
    baseURL: `http://127.0.0.1:${appPort}`,
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command:
      "NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED=1 NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED=1 NEXT_PUBLIC_MP_PUBLIC_KEY=TEST-wallet-playwright API_BACKEND_BASE_URL=http://127.0.0.1:1/api/v1 pnpm exec next dev --hostname 127.0.0.1 --port " +
      appPort,
    url: `http://127.0.0.1:${appPort}/`,
    reuseExistingServer: false,
    timeout: 60000,
  },
});
