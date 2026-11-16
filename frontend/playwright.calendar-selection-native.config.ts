import { defineConfig } from "@playwright/test";
import monthly from "./playwright.calendar-month-native.config";

export default defineConfig({
  ...monthly,
  testMatch: "calendar-selection-native-readonly.spec.ts",
  outputDir: "./test-results/calendar-selection-native",
});
