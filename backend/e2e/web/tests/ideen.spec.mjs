// Browser-E2E des Bereichs „Ideen“: echter Chromium, echtes Zitadel, echtes
// Backend. Eingereicht wird so, wie es die Website tut — als klassisches
// Formular ohne JavaScript.
import { test, expect, state, anmelden } from '../fixtures.mjs';
import { BASE_URL } from '../config.mjs';

const marke = Date.now();
const wunsch = `E2E-Wunsch ${marke}: ein Mitfahrbrett für Fahrten nach Hildesheim.`;

test('Ideen: öffentlich einreichen, in der Verwaltung einordnen und löschen', async ({ page }) => {
  const { admin } = state();

  await test.step('Der öffentliche Eingang nimmt ohne Anmeldung an', async () => {
    const antwort = await page.request.post('/api/v1/ideen', {
      form: { name: 'Erna E2E', email: 'erna@example.org', wunsch },
    });
    expect(antwort.status()).toBe(201);
  });

  await test.step('Ein Browser landet danach auf der Dankeseite der Website', async () => {
    const antwort = await page.request.post('/api/v1/ideen', {
      form: { wunsch: `E2E-Danke ${marke}: bitte weiterleiten.`, redirect: 'https://xn--rssing-wxa.de/app/danke' },
      maxRedirects: 0,
    });
    expect(antwort.status()).toBe(303);
    expect(antwort.headers()['location']).toBe('https://xn--rssing-wxa.de/app/danke');
  });

  await test.step('Auf fremde Ziele wird nie weitergeleitet', async () => {
    const antwort = await page.request.post('/api/v1/ideen', {
      form: { wunsch: `E2E-Umleitung ${marke}: darf nicht ankommen.`, redirect: 'https://boese.example/' },
      maxRedirects: 0,
    });
    expect(antwort.status()).toBe(400);
  });

  await test.step('Der Honigtopf verwirft still, die Antwort bleibt freundlich', async () => {
    const antwort = await page.request.post('/api/v1/ideen', {
      form: { wunsch: `E2E-Honigtopf ${marke}: darf nicht ankommen.`, webseite: 'http://spam.example' },
    });
    expect(antwort.status()).toBe(201);
  });

  await test.step('Die Fehlerseite behält den getippten Text', async () => {
    const antwort = await page.request.post('/api/v1/ideen', {
      form: { name: 'Erna E2E', email: 'erna@example.org', wunsch: 'zu' },
      headers: { Accept: 'text/html' },
    });
    expect(antwort.status()).toBe(400);
    const html = await antwort.text();
    expect(html).toContain('erna@example.org');
    expect(html).toContain('Erna E2E');
    expect(html).toContain('>zu</textarea>');
    // Auch diese Seite lädt ihr CSS aus dem Repo, nicht von einem CDN.
    expect(html).toContain('/admin/static/app.css');
    expect(html).not.toContain('cdn.');
  });

  await test.step('Ohne Anmeldung führt die Verwaltung zur Anmeldeseite', async () => {
    const antwort = await page.request.get('/admin/ideen/', { maxRedirects: 0 });
    expect(antwort.status()).toBe(303);
    expect(antwort.headers()['location']).toBe('/admin/');
  });

  await anmelden(page, admin.userName, admin.password);

  let ideeURL = '';
  await test.step('Die Übersicht zählt die neuen Ideen und führt in den Bereich', async () => {
    await expect(page.locator('#bereich-ideen')).toBeVisible();
    await expect(page.locator('#ideen-neu')).not.toHaveText('0');
    await page.locator('#bereich-ideen').getByRole('link', { name: 'Öffnen' }).click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/ideen/`);
    await expect(page.locator('#seitentitel')).toHaveText('Ideen aus dem Dorf');
  });

  await test.step('Die Liste zeigt Datum, Name, E-Mail, Wunsch und Stand', async () => {
    const zeile = page.getByRole('row', { hasText: `E2E-Wunsch ${marke}` });
    await expect(zeile).toContainText('Erna E2E');
    await expect(zeile).toContainText('erna@example.org');
    await expect(zeile).toContainText('neu');
    // Was der Missbrauchsschutz verworfen hat, darf nirgends auftauchen.
    await expect(page.locator('body')).not.toContainText(`E2E-Honigtopf ${marke}`);
    await expect(page.locator('body')).not.toContainText(`E2E-Umleitung ${marke}`);
    await zeile.getByRole('link', { name: new RegExp(`E2E-Wunsch ${marke}`) }).click();
    await expect(page.locator('#idee-wunsch')).toContainText(wunsch);
    ideeURL = page.url();
  });

  await test.step('Stand und interne Notiz lassen sich speichern', async () => {
    await page.locator('#feld-status').selectOption('umgesetzt');
    await page.locator('#feld-notiz').fill('E2E: kommt mit der nächsten Version.');
    await page.locator('#idee-speichern').click();
    await expect(page.locator('#meldung')).toHaveAttribute('data-art', 'success');
    await expect(page.locator('#feld-notiz')).toHaveValue('E2E: kommt mit der nächsten Version.');
    await expect(page.locator('#feld-status')).toHaveValue('umgesetzt');
  });

  await test.step('Der Statusfilter ist echte Seitennavigation', async () => {
    await page.goto('/admin/ideen/?status=neu');
    await expect(page.locator('body')).not.toContainText(`E2E-Wunsch ${marke}`);
    await page.goto('/admin/ideen/?status=umgesetzt');
    await expect(page.locator('body')).toContainText(`E2E-Wunsch ${marke}`);
  });

  await test.step('Gelöscht wird über eine eigene Bestätigungsseite', async () => {
    await page.goto(ideeURL);
    await page.locator('#idee-loeschen').click();
    await expect(page.locator('#loeschen-bestaetigen')).toBeVisible();
    await page.locator('#loeschen-bestaetigen').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/ideen/`);
    await expect(page.locator('body')).not.toContainText(`E2E-Wunsch ${marke}`);
  });

  await test.step('Die Danke-Idee wieder aufräumen', async () => {
    await page.goto('/admin/ideen/');
    const zeile = page.getByRole('row', { hasText: `E2E-Danke ${marke}` });
    if (await zeile.count()) {
      await zeile.getByRole('link', { name: new RegExp(`E2E-Danke ${marke}`) }).click();
      await page.locator('#idee-loeschen').click();
      await page.locator('#loeschen-bestaetigen').click();
    }
  });
});
