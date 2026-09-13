import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  timeout: 90000,
  use: {
    baseURL: process.env.TEST_WEB_ORIGIN || "http://localhost:5173",
    channel: process.env.PW_CHANNEL || "chrome",
    headless: true,
    viewport: { width: 1440, height: 1000 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: process.env.RECORD_DEMO ? "on" : "off",
  },
  reporter: "list",
});
