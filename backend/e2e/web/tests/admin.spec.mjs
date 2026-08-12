// Browser-E2E des Web-Admin: echter Chromium, echtes Zitadel, echtes Backend.
// Es wird nichts gemockt — der Login läuft über die Zitadel-Login-Oberfläche.
import { test, expect, state, anmelden } from '../fixtures.mjs';

test('Startseite antwortet und verlinkt Verwaltung und App', async ({ page }) => {
  const antwort = await page.request.get('/');
  expect(antwort.status()).toBe(200);
  const html = await antwort.text();
  expect(html).toContain('/admin/');
  expect(html).toContain('releases/latest');

  await page.goto('/');
  await expect(page.getByRole('link', { name: /Verwaltung/ })).toBeVisible();
});

test('Admin: Login, Karte, Orte, Aufgabe, Erledigung, Hitzefaktor', async ({ page }) => {
  const { admin } = state();
  const ortsname = `E2E-Kasten ${Date.now()}`;

  await test.step('Echter Zitadel-Login führt in die Admin-Oberfläche', async () => {
    await anmelden(page, admin.userName, admin.password);
    // Der Kern des Bugs: der Login-Overlay muss wirklich verschwinden.
    await expect(page.locator('#app-main')).toBeVisible();
    await expect(page.locator('#app-header')).toBeVisible();
    await expect(page.locator('#login-overlay')).toBeHidden();
    await expect(page.locator('#login-error')).toHaveText('');
  });

  await test.step('Karte lädt vollständig', async () => {
    await expect(page.locator('body')).toHaveAttribute('data-map-state', 'ready', { timeout: 60_000 });
    await expect(page.locator('#map-error')).toBeHidden();
    await expect(page.locator('#map canvas')).toBeVisible();
    expect(await page.evaluate(() => !!map.getLayer('places-layer'))).toBe(true);
  });

  let orteVorher = 0;
  await test.step('Seed-Orte werden gerendert', async () => {
    await expect(page.locator('#places .card').first()).toBeVisible();
    await expect(page.getByText('Unter den Eichen — Kasten 1')).toBeVisible();
    orteVorher = await page.locator('#places .card').count();
    expect(orteVorher).toBeGreaterThanOrEqual(2);
    // Jeder Ort der Liste liegt auch als Feature auf der Karte.
    await expect(page.locator('#map')).toHaveAttribute('data-markers', String(orteVorher));
  });

  await test.step('Ort über den Dialog anlegen', async () => {
    await page.locator('#new-place').click();
    await expect(page.locator('#place-dialog')).toBeVisible();
    await page.locator('#p-name').fill(ortsname);
    await page.locator('#p-kind').selectOption('beet');
    await page.locator('#p-desc').fill('vom Browser-E2E angelegt');
    await page.locator('#p-lat').fill('52.2115');
    await page.locator('#p-lon').fill('9.8710');
    await page.locator('#p-save').click();
    await expect(page.locator('#place-dialog')).toBeHidden();

    // In der Liste …
    const karte = page.locator('#places .card', { hasText: ortsname });
    await expect(karte).toBeVisible();
    // … und als echtes Feature auf der Karte.
    await expect(page.locator('#map')).toHaveAttribute('data-markers', String(orteVorher + 1));
    const koordinaten = await page.evaluate(async () => {
      const quelle = map.getSource('places');
      const daten = typeof quelle.getData === 'function' ? await quelle.getData() : quelle.serialize().data;
      return daten.features.map((f) => f.geometry.coordinates);
    });
    expect(koordinaten).toContainEqual([9.871, 52.2115]);
  });

  await test.step('Aufgabe anlegen und Erledigung melden', async () => {
    const karte = page.locator('#places .card', { hasText: ortsname });
    await karte.locator('.add-task').click();
    await expect(page.locator('#task-dialog')).toBeVisible();
    await page.locator('#t-kind').selectOption('giessen');
    await page.locator('#t-liters').fill('10');
    await page.locator('#t-interval').fill('7');
    await page.locator('#t-red').fill('14');
    await page.locator('#t-save').click();
    await expect(page.locator('#task-dialog')).toBeHidden();

    const aufgabe = karte.locator('.task').first();
    await expect(aufgabe).toBeVisible();
    await expect(aufgabe).toContainText('noch nie erledigt');

    await aufgabe.locator('.mark-done').click();
    await expect(karte.locator('.task').first()).toHaveAttribute('data-status', 'green');
    await expect(karte.locator('[data-place-status]')).toHaveAttribute('data-place-status', 'green');
    await expect(karte.locator('.task').first()).toContainText('zuletzt');
  });

  await test.step('Hitzefaktor setzen und nach Neuladen prüfen', async () => {
    await page.locator('#factor').fill('0.5');
    await page.locator('#save-factor').click();
    await expect(page.locator('#factor')).toHaveValue('0.5');

    await page.reload();
    await expect(page.locator('#app-main')).toBeVisible();
    await expect(page.locator('#factor')).toHaveValue('0.5');
    await expect(page.locator('#places .card', { hasText: ortsname })).toBeVisible();
  });
});

test('Mitglied ohne Admin-Rolle bekommt keine Verwaltung', async ({ page }) => {
  const { member } = state();
  await anmelden(page, member.userName, member.password);

  await expect(page.locator('#login-error')).toContainText('keine Admin-Rechte');
  await expect(page.locator('#login-overlay')).toBeVisible();
  await expect(page.locator('#app-main')).toBeHidden();
  await expect(page.locator('#app-header')).toBeHidden();
  expect(await page.evaluate(() => sessionStorage.getItem('token'))).toBeNull();
});
