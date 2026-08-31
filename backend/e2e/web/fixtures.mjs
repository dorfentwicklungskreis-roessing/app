// Test-Fixture: sammelt ALLE Browser-Meldungen und lässt den Test scheitern,
// sobald eine echte Konsolen-Fehlermeldung oder eine unbehandelte Exception
// auftritt.
import { test as base, expect } from '@playwright/test';
import fs from 'node:fs';
import { BASE_URL, STATE_FILE } from './config.mjs';

// Bekanntes, unvermeidbares Rauschen aus dem Headless-Chromium bzw. den
// Vektorkacheln — alles andere gilt als Fehler.
const HARMLOS = [
  /GL Driver Message/i,
  /Automatic fallback to software WebGL/i,
  /SwiftShader/i,
  /WebGL: CONTEXT_LOST_WEBGL/i,
  // Abgewiesene Eingaben liefern bewusst die Seite samt Erklärung: HTTP 400
  // bei ungültigen Formularwerten, HTTP 409, wenn der Spielschutz die Aufgabe
  // noch sperrt. Chromium protokolliert jede Navigation mit Nicht-2xx als
  // Konsolenfehler — das ist erwartetes Verhalten, kein Skriptfehler.
  // Alles andere (403, 404, 5xx) bleibt ein Fehler.
  /Failed to load resource: the server responded with a status of (400 \(Bad Request\)|409 \(Conflict\))/i,
];

// Nur Meldungen aus UNSERER Seite zählen. Die Login-Oberfläche von Zitadel
// wirft eigenes Rauschen (blockierte RSC-Prefetches wegen ihrer CSP), das wir
// weder verursachen noch beheben können.
const eigeneSeite = (url) => !url || url.startsWith(BASE_URL);

export const state = () => JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'));

/** Hängt den Fehler-Wächter an eine Seite und liefert Prüf-/Protokollhilfen. */
export function ueberwachen(page) {
  const alle = [];
  const fehler = [];
  page.on('console', (m) => {
    const quelle = m.location()?.url || page.url();
    const zeile = `[${m.type()}] ${m.text()}   (${quelle})`;
    alle.push(zeile);
    if (m.type() === 'error' && eigeneSeite(quelle) && !HARMLOS.some((r) => r.test(m.text()))) {
      fehler.push(zeile);
    }
  });
  page.on('pageerror', (e) => {
    const quelle = page.url();
    const zeile = `[pageerror] ${e.message}   (${quelle})\n${e.stack || ''}`;
    alle.push(zeile);
    if (eigeneSeite(quelle)) fehler.push(zeile);
  });
  return { alle, fehler };
}

export const test = base.extend({
  page: async ({ page }, use, testInfo) => {
    const { alle, fehler } = ueberwachen(page);
    await use(page);
    await testInfo.attach('browser-console.log', { body: alle.join('\n'), contentType: 'text/plain' });
    expect(fehler, `Browser-Fehler während „${testInfo.title}":\n${fehler.join('\n')}`).toEqual([]);
  },
});

export { expect };

/**
 * Fährt die Anmeldemaske der Rössing-ID durch — Benutzername, Passwort und
 * die Zwischenseiten, die Zitadel je nach Version einschiebt.
 *
 * `fertig()` sagt, woran das Ende zu erkennen ist. Das ist nicht immer die
 * Verwaltung: Der MCP-Connector landet am Ende bei seiner eigenen
 * Rücksprung-Adresse, geht aber durch dieselbe Maske.
 */
export async function anmeldemaskeDurchlaufen(page, user, passwort, fertig) {
  await page.waitForURL(/\/ui\//, { timeout: 60_000 });

  const benutzerfeld = page
    .locator('input[name="loginName"], input#loginName, input[name="username"], input[type="email"]')
    .first();
  await benutzerfeld.waitFor({ state: 'visible', timeout: 60_000 });
  await benutzerfeld.fill(user);
  await absenden(page);

  const passwortfeld = page.locator('input[type="password"]').first();
  await passwortfeld.waitFor({ state: 'visible', timeout: 60_000 });
  await passwortfeld.fill(passwort);
  await absenden(page);

  // Zitadel schiebt je nach Version „Zwei-Faktor einrichten" oder
  // „Passkey einrichten" dazwischen — solche Seiten überspringen wir.
  const frist = Date.now() + 45_000;
  while (!fertig() && Date.now() < frist) {
    if (!(await ueberspringenFallsNoetig(page))) await page.waitForTimeout(500);
  }
  if (!fertig()) {
    const text = (await page.locator('body').innerText().catch(() => '')).replace(/\s+/g, ' ').slice(0, 400);
    throw new Error(`Login blieb bei der Rössing-ID hängen.\nURL: ${page.url()}\nSeite: ${text}`);
  }
}

/**
 * Fährt den ECHTEN Zitadel-Login durch. Der Code-Tausch passiert danach im
 * Backend; im Browser landet nur ein HttpOnly-Session-Cookie.
 * Funktioniert auch in Kontexten ohne JavaScript.
 */
export async function anmelden(page, user, passwort) {
  await page.goto('/admin/');
  await page.locator('#anmelden').click();
  await anmeldemaskeDurchlaufen(page, user, passwort, () => /\/admin\//.test(page.url()));
  await page.waitForURL(/\/admin\//, { timeout: 30_000 });
}

// Zitadel Login v1 hat auf der Benutzernamen-Seite mehrere Submit-Buttons
// („Registrieren" steht vor „Weiter"), deshalb erst die eindeutigen Kandidaten.
async function absenden(page) {
  const kandidaten = [
    page.locator('#submit-button'),
    page.getByRole('button', { name: /^(weiter|continue|next|anmelden|sign in|log in|absenden)$/i }),
    page.locator('button[type="submit"], input[type="submit"]'),
  ];
  for (const kandidat of kandidaten) {
    const knopf = kandidat.first();
    if (await knopf.count() && await knopf.isVisible()) {
      await knopf.click();
      return;
    }
  }
  throw new Error('Kein Submit-Button auf der Login-Seite gefunden');
}

/** Klickt einen „Überspringen"-Knopf, falls einer da ist. */
async function ueberspringenFallsNoetig(page) {
  const kandidaten = [
    page.locator('#skip-button'),
    page.getByRole('button', { name: /überspringen|skip|später|nicht jetzt|not now/i }),
    page.getByRole('link', { name: /überspringen|skip|später|nicht jetzt|not now/i }),
  ];
  for (const kandidat of kandidaten) {
    const knopf = kandidat.first();
    try {
      if (await knopf.count() && await knopf.isVisible()) {
        await knopf.click();
        return true;
      }
    } catch {
      // Seite hat mitten im Wechsel navigiert — nächster Versuch.
    }
  }
  return false;
}
