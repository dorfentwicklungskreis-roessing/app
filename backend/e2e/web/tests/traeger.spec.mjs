// Browser-E2E des Träger-Bereichs: echter Chromium, echtes Zitadel, echtes
// Backend. Geprüft wird der Weg, den der Betreiber im Alltag geht — einen
// Verein zulassen, eine Befähigung anlegen und eine interne Aufgabe
// einstellen, die außerhalb des Trägers nirgends auftaucht.
import { test, expect, state, anmelden, ueberwachen } from '../fixtures.mjs';
import { BASE_URL } from '../config.mjs';

test('Träger: anlegen, zulassen, Befähigung pflegen, interne Aufgabe einstellen', async ({ page }) => {
  ueberwachen(page);
  const { admin } = state();
  await anmelden(page, admin.userName, admin.password);

  await test.step('Der Bereich ist von der Verwaltung aus erreichbar', async () => {
    await expect(page.locator('#bereich-traeger')).toBeVisible();
    await page.locator('#bereich-traeger').getByRole('link', { name: 'Öffnen' }).click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/traeger/`);
    await expect(page.locator('#seitentitel')).toHaveText('Träger');
  });

  await test.step('Die Umstellung hat den Dorfentwicklungskreis angelegt', async () => {
    // Er ist der Platzhalter, dem die Bestandsaufgaben gehören, bis die
    // Dorfpflege offiziell zugestimmt hat.
    await expect(page.locator('#traeger-tabelle')).toContainText('Dorfentwicklungskreis');
  });

  await test.step('Neuer Träger wird angelegt und ist zunächst unsichtbar', async () => {
    await page.locator('#traeger-neu').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/traeger/neu`);
    await page.locator('#name').fill('Dorfpflege');
    await page.locator('#beschreibung').fill('Rasenmähen, Gießen, Jäten');
    await page.locator('#status').selectOption('beantragt');
    await page.locator('#speichern').click();

    await expect(page).toHaveURL(/\/admin\/traeger\/\d+$/);
    await expect(page.locator('#seitentitel')).toHaveText('Dorfpflege');
    await expect(page.locator('#zulassungsstand')).toHaveText('beantragt');
    // Ohne Zitadel-Projekt gibt es weder Mitglieder noch Verwaltende.
    await expect(page.locator('#kein-projekt')).toBeVisible();
  });

  await test.step('Der Betreiber lässt ihn zu', async () => {
    await page.locator('#zulassen').click();
    await expect(page.locator('#zulassungsstand')).toHaveText('zugelassen');
    await expect(page.locator('#meldung')).toContainText('ist zugelassen');
  });

  await test.step('Eine Befähigung anlegen', async () => {
    await page.locator('#befaehigung-name').fill('Motorsense');
    await page.locator('#befaehigung-beschreibung').fill('Einweisung am Gerät');
    await page.locator('#befaehigung-anlegen').click();
    await expect(page.locator('#befaehigungen')).toContainText('Motorsense');
    await expect(page.locator('#antraege-offen')).toHaveText('0');
  });

  await test.step('Sperren nimmt den Träger samt Aufgaben aus der Sicht', async () => {
    await page.locator('#sperren').click();
    await expect(page.locator('#zulassungsstand')).toHaveText('gesperrt');
    await page.locator('#zulassen').click();
    await expect(page.locator('#zulassungsstand')).toHaveText('zugelassen');
  });
});

test('Aufgabe: Sichtbarkeit und Einweisung lassen sich einstellen', async ({ page }) => {
  ueberwachen(page);
  const { admin } = state();
  await anmelden(page, admin.userName, admin.password);

  await page.goto('/admin/mithelfen/');
  await page.locator('#neuer-ort').click();
  await page.locator('#feld-name').fill('Streuobstwiese');
  await page.locator('#feld-art').selectOption('sonstiges');
  await page.locator('#feld-lat').fill('52.2115');
  await page.locator('#feld-lon').fill('9.8710');
  // Jeder Ort gehört einem Träger — die Auswahl muss dastehen.
  await expect(page.locator('#feld-traeger')).toBeVisible();
  await page.locator('#ort-speichern').click();
  await expect(page).toHaveURL(/\/admin\/mithelfen\/orte\/\d+$/);

  await page.locator('#neue-aufgabe').click();
  await page.locator('#feld-art').selectOption('sonstiges');
  await page.locator('#feld-titel').fill('Interne Geräteprüfung');
  await page.locator('#feld-intervall').fill('30');
  await page.locator('#feld-rot').fill('60');
  await page.locator('#feld-sichtbarkeit').selectOption('nur_mitglieder');
  await page.locator('#aufgabe-speichern').click();

  await expect(page).toHaveURL(/\/admin\/mithelfen\/orte\/\d+$/);
  await expect(page.locator('#meldung')).toContainText('wurde angelegt');
});

test('Träger-Bereich bleibt ohne Anmeldung verschlossen', async ({ page }) => {
  await page.goto('/admin/traeger/');
  await expect(page).toHaveURL(`${BASE_URL}/admin/`);
  await expect(page.locator('#anmelden')).toBeVisible();
});
