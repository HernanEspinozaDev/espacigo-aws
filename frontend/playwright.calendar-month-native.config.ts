import { defineConfig } from "@playwright/test";

const baseURL =
  process.env.CALENDAR_NATIVE_FRONTEND_ORIGIN ?? "http://127.0.0.1:4316";
const parsed = new URL(baseURL);
if (parsed.protocol !== "http:" || parsed.hostname !== "127.0.0.1") {
  throw new Error(
    "Calendar native read-only checks require an explicit loopback frontend.",
  );
}

export default defineConfig({
  testDir: "./tests",
  testMatch: "calendar-month-native-readonly.spec.ts",
  outputDir: "./test-results/calendar-month-native",
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  use: { baseURL, browserName: "chromium", trace: "retain-on-failure" },
});
