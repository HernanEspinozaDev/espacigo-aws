import { defineConfig } from "@playwright/test";
const baseURL =
  process.env.HOST_CALENDAR_FRONTEND_ORIGIN ?? "http://127.0.0.1:4317";
const parsed = new URL(baseURL);
if (parsed.protocol !== "http:" || parsed.hostname !== "127.0.0.1")
  throw new Error("Host calendar browser checks require loopback 127.0.0.1");
export default defineConfig({
  testDir: "./tests",
  testMatch: "host-calendar-configuration.spec.ts",
  outputDir:
    process.env.HOST_CALENDAR_PLAYWRIGHT_OUTPUT ??
    "./test-results/host-calendar-configuration",
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  use: { baseURL, browserName: "chromium", trace: "retain-on-failure" },
});
