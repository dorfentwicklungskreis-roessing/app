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
        launchOptions: {
          args: [
            // Ohne GPU braucht MapLibre die Software-Implementierung von
            // WebGL; neuere Chromium-Versionen verlangen dafür dieses Flag.
            '--enable-unsafe-swiftshader',
            // claude.ai gibt es für diesen Browser nicht.
            //
            // Der MCP-Anmeldeweg endet bei der Rücksprung-Adresse des
            // Connectors — genau die, die der Registrierungs-Endpunkt
            // herausgibt. Sie im Test abzufangen reicht nicht: Playwright
            // ruft seinen Route-Handler beim LETZTEN Sprung einer fremden
            // Weiterleitungskette nicht auf, und Chromium holte die Seite
            // dann wirklich (nachgewiesen im ersten CI-Lauf: der Rücksprung
            // landete bei claude.ai, das prompt weiterleitete und seine
            // ganze Oberfläche nachlud).
            //
            // Hier wird der Name deshalb eine Ebene tiefer unauflösbar
            // gemacht — vor Weiterleitung, Route-Handler und Cache. Was der
            // Test braucht, ist die Adresse mit dem Code, und die steht schon
            // in der Anfrage; die Antwort braucht er nie.
            '--host-resolver-rules=MAP claude.ai ~NOTFOUND',
          ],
        },
      },
    },
  ],
});
