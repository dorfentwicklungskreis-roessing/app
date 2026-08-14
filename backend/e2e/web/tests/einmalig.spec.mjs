// Browser-E2E der einmaligen Aufgaben (#6): echter Chromium, echtes Zitadel,
// echtes Backend. Geprüft wird der Weg, um den es geht — anlegen, richtige
// Ampel, erledigen, verschwinden — und dass das Formular ohne JavaScript
// bedienbar bleibt.
import { test, expect, state, anmelden } from '../fixtures.mjs';
import { BASE_URL } from '../config.mjs';

/** Datum im Format des Eingabefelds, um `tage` verschoben. */
const tag = (tage) => {
  const d = new Date();
  d.setDate(d.getDate() + tage);
  return d.toISOString().slice(0, 10);
};

/** Legt einen eigenen Ort an und liefert dessen URL. */
async function eigenerOrt(page, name) {
  await page.goto('/admin/mithelfen/orte/neu');
  await page.locator('#feld-name').fill(name);
  await page.locator('#feld-art').selectOption('sonstiges');
  await page.locator('#feld-lat').fill('52.2108');
  await page.locator('#feld-lon').fill('9.8692');
  await page.locator('#ort-speichern').click();
  await expect(page).toHaveURL(/\/admin\/mithelfen\/orte\/\d+$/);
  return page.url();
}

/** Legt eine einmalige Aufgabe an dem Ort an. */
async function einmaligeAufgabe(page, ortURL, { titel, termin, entfernen = false }) {
  await page.goto(ortURL);
  await page.locator('#neue-aufgabe').click();
  await expect(page).toHaveURL(/\/aufgaben\/neu$/);
  await page.locator('#feld-art').selectOption('sonstiges');
  await page.locator('#feld-titel').fill(titel);
  await page.locator('#feld-wiederholung-einmalig').check();
  await page.locator('#feld-termin').fill(termin);
  if (entfernen) await page.locator('#feld-entfernen').check();
  await page.locator('#aufgabe-speichern').click();
  await expect(page).toHaveURL(ortURL);
}

test('Einmalige Aufgabe: Termin macht die Ampel, Erledigen räumt sie ab', async ({ page }) => {
  const { admin } = state();
  await anmelden(page, admin.userName, admin.password);
  const ortURL = await eigenerOrt(page, `E2E-Bahnhof ${Date.now()}`);

  await test.step('Anlegen mit Termin statt Intervall', async () => {
    await einmaligeAufgabe(page, ortURL, {
      titel: 'Zum Bahnhof fahren', termin: tag(10), entfernen: true,
    });
    const aufgabe = page.locator('[data-aufgabe-id]').first();
    await expect(aufgabe.locator('[data-einmalig]')).toBeVisible();
    await expect(aufgabe.locator('[data-entfernen]')).toBeVisible();
    await expect(aufgabe.locator('[data-plan]')).toContainText('einmalig, fällig am');
    // Zehn Tage hin: grün.
    await expect(aufgabe).toHaveAttribute('data-status', 'green');
  });

  await test.step('Ohne Termin wird abgewiesen — auf der Seite, mit Erklärung', async () => {
    await page.goto(ortURL);
    await page.locator('#neue-aufgabe').click();
    await page.locator('#feld-wiederholung-einmalig').check();
    await page.locator('#feld-termin').fill('');
    await page.locator('#aufgabe-speichern').click();
    await expect(page.locator('#formularfehler')).toContainText('dueDate');
    // Die getippte Auswahl steht noch da.
    await expect(page.locator('#feld-wiederholung-einmalig')).toBeChecked();
  });

  await test.step('Ein verstrichener Termin ist rot, ein naher gelb', async () => {
    await einmaligeAufgabe(page, ortURL, { titel: 'Längst fällig', termin: tag(-2) });
    await einmaligeAufgabe(page, ortURL, { titel: 'Übermorgen', termin: tag(1) });
    await page.goto(ortURL);
    const zeile = (titel) => page.locator('[data-aufgabe-id]').filter({ hasText: titel });
    await expect(zeile('Längst fällig')).toHaveAttribute('data-status', 'red');
    await expect(zeile('Übermorgen')).toHaveAttribute('data-status', 'yellow');
    // Der Ort trägt den schlechtesten Stand seiner Aufgaben.
    await expect(page.locator('#ort-status')).toHaveAttribute('data-status', 'red');
  });

  await test.step('Erledigen nimmt sie von der Seite — mit Schalter', async () => {
    await page.goto(ortURL);
    const vorher = await page.locator('[data-aufgabe-id]').count();
    await page.locator('[data-aufgabe-id]').filter({ hasText: 'Zum Bahnhof fahren' })
      .locator('.erledigt-melden').click();
    await expect(page).toHaveURL(/\/erledigt$/);
    await page.locator('#erledigt-bestaetigen').click();
    await expect(page).toHaveURL(ortURL);
    await expect(page.locator('#meldung')).toHaveAttribute('data-art', 'success');
    await expect(page.locator('[data-aufgabe-id]')).toHaveCount(vorher - 1);
    await expect(page.locator('#aufgaben')).not.toContainText('Zum Bahnhof fahren');
  });

  await test.step('Ohne Schalter bleibt sie stehen — grün und nicht wieder fällig', async () => {
    await page.goto(ortURL);
    await page.locator('[data-aufgabe-id]').filter({ hasText: 'Längst fällig' })
      .locator('.erledigt-melden').click();
    await page.locator('#erledigt-bestaetigen').click();
    await expect(page).toHaveURL(ortURL);
    const zeile = page.locator('[data-aufgabe-id]').filter({ hasText: 'Längst fällig' });
    await expect(zeile).toHaveAttribute('data-status', 'green');
  });
});

// Die Verwaltung muss ohne JavaScript bedienbar bleiben — das ist die Regel
// des Bereichs, und ein Radioknopf mit zwei Feldgruppen erfüllt sie.
test.describe('ohne JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  test('Einmalige Aufgabe lässt sich ohne JavaScript anlegen', async ({ page }) => {
    const { admin } = state();
    await anmelden(page, admin.userName, admin.password);
    await expect(page).toHaveURL(`${BASE_URL}/admin/`);

    const ortURL = await eigenerOrt(page, `E2E-ohne-JS ${Date.now()}`);
    await einmaligeAufgabe(page, ortURL, { titel: 'Ohne JS', termin: tag(-1) });

    const aufgabe = page.locator('[data-aufgabe-id]').first();
    await expect(aufgabe.locator('[data-einmalig]')).toBeVisible();
    await expect(aufgabe).toHaveAttribute('data-status', 'red');
  });
});
