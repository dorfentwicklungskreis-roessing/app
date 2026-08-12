import { defineConfig, devices } from '@playwright/test';
import { BASE_URL } from './config.mjs';

export default defineConfig({
  testDir: './tests',
  // Der Admin-Flow baut aufeinander auf (Ort → Aufgabe → Erledigung) und
  // schreibt in eine gemeinsame SQLite — daher strikt seriell.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  globalSetup: './global-setup.mjs',
  globalTeardown: './global-teardown.mjs',
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: BASE_URL,
    locale: 'de-DE',
    timezoneId: 'Europe/Berlin',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    trace: 'retain-on-failure',
    actionTimeout: 20_000,
    navigationTimeout: 45_000,
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // Ohne GPU braucht MapLibre die Software-Implementierung von WebGL;
        // neuere Chromium-Versionen verlangen dafür dieses Flag.
        launchOptions: { args: ['--enable-unsafe-swiftshader'] },
      },
    },
  ],
});
