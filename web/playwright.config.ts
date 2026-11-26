import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  workers: process.env["CI"] ? 2 : 4,
  forbidOnly: !!process.env["CI"],
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  globalSetup: "./e2e/preparar.ts",
  globalTeardown: "./e2e/cobertura.ts",
  reporter: [["list"], ["html", { open: "never", outputFolder: "relatorio-playwright" }]],
  use: {
    locale: "pt-BR",
    colorScheme: "dark",
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 1,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1536, height: 864 } } }],
});
