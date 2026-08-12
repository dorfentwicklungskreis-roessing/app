// Browser-E2E der Verwaltung: echter Chromium, echtes Zitadel, echtes Backend.
// Es wird nichts gemockt — der Login läuft über die Zitadel-Login-Oberfläche,
// der Code-Tausch im Backend.
import { test, expect, state, anmelden, ueberwachen } from '../fixtures.mjs';
import { BASE_URL } from '../config.mjs';

test('Startseite antwortet und verlinkt Verwaltung und App', async ({ page }) => {
  const antwort = await page.request.get('/');
  expect(antwort.status()).toBe(200);
  const html = await antwort.text();
  expect(html).toContain('/admin/');
  expect(html).toContain('releases/latest');
  // Das ausgelieferte CSS wird aus dem Repo geladen, nicht von einem CDN.
  expect(html).toContain('/admin/static/app.css');
  expect(html).not.toContain('cdn.');

  await page.goto('/');
  await expect(page.getByRole('link', { name: /Verwaltung/ })).toBeVisible();
  // Die Dorf-App ist mehr als Dorfpflege — das muss die Startseite sagen.
  await expect(page.locator('body')).toContainText('Dorf-App Rössing');
  await expect(page.locator('body')).toContainText('Dorfladen ERNA');

  const css = await page.request.get('/admin/static/app.css');
  expect(css.status()).toBe(200);
  const cssText = await css.text();
  // Beweis, dass wirklich Tailwind + DaisyUI ausgeliefert werden.
  expect(cssText).toContain('tailwindcss');
  expect(cssText).toContain('drawer-open');
  expect(cssText).toContain('badge-warning');
});

test('Verwaltung: Login, Bereiche, Karte, Ort, Aufgabe, Erledigung, Hitzefaktor, Löschen', async ({ page }) => {
  const { admin } = state();
  const ortsname = `E2E-Kasten ${Date.now()}`;
  let ortURL = '';

  await test.step('Echter Zitadel-Login endet in der Verwaltung', async () => {
    await anmelden(page, admin.userName, admin.password);
    await expect(page).toHaveURL(`${BASE_URL}/admin/`);
    await expect(page.locator('#seitentitel')).toHaveText('Verwaltung');
    await expect(page.locator('#bereich-dorfpflege')).toBeVisible();
    await expect(page.locator('#angemeldet-als')).toContainText(admin.userName);
    // Kein Token im Browser — die Sitzung hängt an einem HttpOnly-Cookie.
    expect(await page.evaluate(() => sessionStorage.length + localStorage.length)).toBe(0);
    const cookies = await page.context().cookies();
    const sitzung = cookies.find((c) => c.name === 'dorf_admin_session');
    expect(sitzung, 'Session-Cookie fehlt').toBeTruthy();
    expect(sitzung.httpOnly).toBe(true);
    expect(sitzung.sameSite).toBe('Lax');
  });

  await test.step('Echter Seitenwechsel in den Bereich Dorfpflege', async () => {
    await page.locator('#bereich-dorfpflege').getByRole('link', { name: 'Öffnen' }).click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/`);
    await expect(page.locator('#seitentitel')).toHaveText('Dorfpflege');
    await expect(page.locator('#orte-tabelle')).toBeVisible();
    await expect(page.getByText('Unter den Eichen — Kasten 1')).toBeVisible();
  });

  let orteVorher = 0;
  await test.step('Karte lädt und zeigt jeden Ort der Liste', async () => {
    orteVorher = await page.locator('#orte-tabelle tbody tr').count();
    expect(orteVorher).toBeGreaterThanOrEqual(2);
    await expect(page.locator('body')).toHaveAttribute('data-map-state', 'ready', { timeout: 60_000 });
    await expect(page.locator('#karte canvas')).toBeVisible();
    await expect(page.locator('#karte')).toHaveAttribute('data-markers', String(orteVorher));
  });

  await test.step('Ort über eine eigene Seite anlegen', async () => {
    await page.locator('#neuer-ort').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/orte/neu`);
    await page.locator('#feld-name').fill(ortsname);
    await page.locator('#feld-art').selectOption('beet');
    await page.locator('#feld-beschreibung').fill('vom Browser-E2E angelegt');
    await page.locator('#feld-lat').fill('52.2115');
    await page.locator('#feld-lon').fill('9.8710');
    await page.locator('#ort-speichern').click();

    // Post/Redirect/Get: eigene Detailseite mit eigener URL.
    await expect(page).toHaveURL(/\/admin\/dorfpflege\/orte\/\d+$/);
    ortURL = page.url();
    await expect(page.locator('#meldung')).toHaveAttribute('data-art', 'success');
    await expect(page.locator('#seitentitel')).toHaveText(ortsname);
    await expect(page.locator('#feld-lat')).toHaveValue('52.2115');
  });

  await test.step('Der neue Ort steht in Liste und Karte', async () => {
    await page.goto('/admin/dorfpflege/');
    await expect(page.locator('#orte-tabelle tbody tr')).toHaveCount(orteVorher + 1);
    await expect(page.locator('#orte-tabelle').getByText(ortsname)).toBeVisible();
    await expect(page.locator('#karte')).toHaveAttribute('data-markers', String(orteVorher + 1));
    await expect(page.locator('body')).toHaveAttribute('data-map-state', 'ready', { timeout: 60_000 });
    const koordinaten = await page.evaluate(() =>
      JSON.parse(document.getElementById('karte').dataset.karte).features.map((f) => f.geometry.coordinates));
    expect(koordinaten).toContainEqual([9.871, 52.2115]);
  });

  await test.step('Aufgabe über eine eigene Seite anlegen', async () => {
    await page.goto(ortURL);
    await expect(page.locator('#keine-aufgaben')).toBeVisible();
    await page.locator('#neue-aufgabe').click();
    await expect(page).toHaveURL(/\/admin\/dorfpflege\/orte\/\d+\/aufgaben\/neu$/);
    await page.locator('#feld-art').selectOption('giessen');
    await page.locator('#feld-liter').fill('10');
    await page.locator('#feld-intervall').fill('7');
    await page.locator('#feld-rot').fill('14');
    await page.locator('#aufgabe-speichern').click();

    await expect(page).toHaveURL(ortURL);
    await expect(page.locator('#aufgaben')).toHaveAttribute('data-anzahl', '1');
    await expect(page.locator('[data-aufgabe-id]').first()).toContainText('noch nie erledigt');
  });

  await test.step('Fehlerhafte Aufgabe wird auf der Seite abgewiesen', async () => {
    await page.locator('.aufgabe-bearbeiten').first().click();
    await expect(page).toHaveURL(/\/admin\/dorfpflege\/aufgaben\/\d+$/);
    await page.locator('#feld-rot').fill('3'); // kleiner als das Intervall
    await page.locator('#aufgabe-speichern').click();
    await expect(page.locator('#formularfehler')).toContainText('redAfterDays');
    await page.locator('#feld-rot').fill('14');
    await page.locator('#aufgabe-speichern').click();
    await expect(page).toHaveURL(ortURL);
  });

  await test.step('Erledigung melden macht den Ort grün', async () => {
    const aufgabe = page.locator('[data-aufgabe-id]').first();
    await aufgabe.locator('.erledigt-melden').click();
    await expect(page).toHaveURL(ortURL);
    await expect(page.locator('#meldung')).toHaveAttribute('data-art', 'success');
    await expect(page.locator('[data-aufgabe-id]').first()).toHaveAttribute('data-status', 'green');
    await expect(page.locator('#ort-status')).toHaveAttribute('data-status', 'green');
    await expect(page.locator('[data-aufgabe-id]').first()).toContainText('zuletzt');
    // Historie steht auf derselben Seite.
    await expect(page.locator('#historie tbody tr').first()).toContainText(state().admin.userName);
  });

  await test.step('Hitzefaktor auf eigener Seite setzen', async () => {
    await page.goto('/admin/dorfpflege/');
    await page.locator('#zu-einstellungen').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/einstellungen`);
    await page.locator('#feld-hitzefaktor').fill('0.5');
    await page.locator('#hitzefaktor-speichern').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/einstellungen`);
    await expect(page.locator('#feld-hitzefaktor')).toHaveValue('0.5');

    await page.goto('/admin/dorfpflege/');
    await expect(page.locator('#anzeige-hitzefaktor')).toHaveText('0.5');
  });

  await test.step('Löschen läuft über eine Bestätigungsseite, kein Popup', async () => {
    await page.goto(ortURL);
    await page.locator('#ort-loeschen').click();
    await expect(page).toHaveURL(/\/admin\/dorfpflege\/orte\/\d+\/loeschen$/);
    await expect(page.locator('#bestaetigen-text')).toContainText(ortsname);

    // Abbrechen führt zurück, ohne zu löschen.
    await page.locator('#loeschen-abbrechen').click();
    await expect(page).toHaveURL(ortURL);

    await page.locator('#ort-loeschen').click();
    await page.locator('#loeschen-bestaetigen').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/`);
    await expect(page.locator('#orte-tabelle').getByText(ortsname)).toHaveCount(0);
    await expect(page.locator('#orte-tabelle tbody tr')).toHaveCount(orteVorher);
  });

  await test.step('Abmelden beendet die Sitzung', async () => {
    await page.locator('#abmelden').click();
    await page.waitForURL(/\/admin\/?$|\/ui\//, { timeout: 45_000 });
    await page.goto('/admin/dorfpflege/');
    await expect(page).toHaveURL(`${BASE_URL}/admin/`);
    await expect(page.locator('#anmelden')).toBeVisible();
  });
});

test('Mitglied ohne Admin-Rolle bekommt keine Verwaltung', async ({ page }) => {
  const { member } = state();
  await anmelden(page, member.userName, member.password);

  await expect(page).toHaveURL(`${BASE_URL}/admin/`);
  await expect(page.locator('#meldung')).toContainText('keine Admin-Rechte');
  await expect(page.locator('#anmelden')).toBeVisible();
  expect(await page.context().cookies().then((cs) => cs.some((c) => c.name === 'dorf_admin_session'))).toBe(false);

  // Auch direkt angesteuerte Seiten bleiben verschlossen.
  await page.goto('/admin/dorfpflege/orte/neu');
  await expect(page).toHaveURL(`${BASE_URL}/admin/`);
  await expect(page.locator('#anmelden')).toBeVisible();
});

test('Verwaltung funktioniert vollständig ohne JavaScript', async ({ browser }) => {
  const { admin } = state();
  const kontext = await browser.newContext({
    javaScriptEnabled: false,
    baseURL: BASE_URL,
    locale: 'de-DE',
    timezoneId: 'Europe/Berlin',
  });
  const page = await kontext.newPage();
  const { fehler } = ueberwachen(page);
  const ortsname = `Ohne-JS ${Date.now()}`;

  try {
    // Anmeldung: der Code-Tausch passiert im Backend, der Browser muss nichts können.
    await anmelden(page, admin.userName, admin.password);
    await expect(page).toHaveURL(`${BASE_URL}/admin/`);
    await expect(page.locator('#bereich-dorfpflege')).toBeVisible();

    // Navigation über echte Links.
    await page.locator('#bereich-dorfpflege').getByRole('link', { name: 'Öffnen' }).click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/`);
    await expect(page.locator('#orte-tabelle')).toBeVisible();
    // Ohne JavaScript bleibt die Karte leer, die Verwaltung aber bedienbar.
    await expect(page.locator('#karte canvas')).toHaveCount(0);

    // Formularaktion: Ort anlegen.
    await page.locator('#neuer-ort').click();
    await page.locator('#feld-name').fill(ortsname);
    await page.locator('#feld-lat').fill('52.2100');
    await page.locator('#feld-lon').fill('9.8690');
    await page.locator('#ort-speichern').click();
    await expect(page).toHaveURL(/\/admin\/dorfpflege\/orte\/\d+$/);
    await expect(page.locator('#seitentitel')).toHaveText(ortsname);
    const ortURL = page.url();

    // Zweite Formularaktion: Aufgabe anlegen und erledigen.
    await page.locator('#neue-aufgabe').click();
    await page.locator('#feld-intervall').fill('7');
    await page.locator('#feld-rot').fill('14');
    await page.locator('#aufgabe-speichern').click();
    await expect(page).toHaveURL(ortURL);
    await page.locator('.erledigt-melden').first().click();
    await expect(page.locator('#ort-status')).toHaveAttribute('data-status', 'green');

    // Löschen über die Bestätigungsseite — ohne confirm() geht das auch ohne JS.
    await page.locator('#ort-loeschen').click();
    await page.locator('#loeschen-bestaetigen').click();
    await expect(page).toHaveURL(`${BASE_URL}/admin/dorfpflege/`);
    await expect(page.locator('#orte-tabelle').getByText(ortsname)).toHaveCount(0);
  } finally {
    await kontext.close();
  }
  expect(fehler, `Browser-Fehler ohne JavaScript:\n${fehler.join('\n')}`).toEqual([]);
});
