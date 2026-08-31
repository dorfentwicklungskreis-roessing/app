// Der Anmeldeweg des MCP-Endpunkts — einmal ganz durch, so wie ihn claude.ai
// geht: unauthentifizierte Anfrage → Hinweis auf die Metadata →
// Protected-Resource-Metadata → gespiegelte AS-Metadata → Dynamic Client
// Registration → Authorize mit PKCE → echter Login an der Rössing-ID →
// Code-Tausch → Werkzeugaufruf. Zum Schluss die Gegenprobe: ein falsches
// Token kommt nicht durch, und wer keine admin-Rolle hat, auch nicht.
//
// Ein Test, der nur nachsieht, ob es einen Registrierungs-Endpunkt gibt,
// beweist nichts über die Kette. Genau deshalb wird hier nichts gemockt:
// Zitadel läuft im Compose, das Backend ist das echte Binary, die Anmeldung
// geht durch die echte Login-Oberfläche.
//
// Die Rücksprung-Adresse ist die von claude.ai — sie ist die einzige, die der
// Registrierungs-Endpunkt herausgibt, und darum die einzige, die etwas
// beweist. Angefasst wird claude.ai dabei nicht: Die Route wird im Browser
// abgefangen und lokal beantwortet; den Code lesen wir aus der abgefangenen
// Adresse.
import crypto from 'node:crypto';
import { test, expect, state, anmeldemaskeDurchlaufen } from '../fixtures.mjs';
import { BASE_URL, ISSUER } from '../config.mjs';

const RUECKSPRUNG = 'https://claude.ai/api/mcp/auth_callback';

/** Erzeugt Verifier und Challenge für PKCE (S256). */
function pkce() {
  const verifier = crypto.randomBytes(48).toString('base64url');
  const challenge = crypto.createHash('sha256').update(verifier).digest('base64url');
  return { verifier, challenge };
}

/** Holt ein JSON-Dokument und gibt Status, Kopfzeilen und Inhalt zurück. */
async function holen(url, init) {
  const antwort = await fetch(url, init);
  const text = await antwort.text();
  let inhalt = null;
  try {
    inhalt = text ? JSON.parse(text) : null;
  } catch {
    inhalt = null;
  }
  return { status: antwort.status, headers: antwort.headers, inhalt, text };
}

/** Ruft den MCP-Endpunkt per JSON-RPC auf. */
async function mcp(token, method, params) {
  const antwort = await fetch(`${BASE_URL}/mcp`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
  });
  const text = await antwort.text();
  let inhalt = null;
  try {
    inhalt = text ? JSON.parse(text) : null;
  } catch {
    inhalt = null;
  }
  return { status: antwort.status, headers: antwort.headers, inhalt, text };
}

/**
 * Geht den Weg, den ein Client nimmt, der nur die Adresse des Endpunkts kennt:
 * abgewiesen werden, dem Hinweis folgen, die Metadata lesen.
 */
async function anmeldewegEntdecken() {
  const abgewiesen = await mcp(null, 'tools/list', null);
  expect(abgewiesen.status, 'ohne Token muss der Endpunkt abweisen').toBe(401);

  const hinweis = abgewiesen.headers.get('www-authenticate') || '';
  const metadataUrl = /resource_metadata="([^"]+)"/.exec(hinweis)?.[1];
  expect(metadataUrl, `WWW-Authenticate ohne Metadata-Adresse: ${hinweis}`).toBeTruthy();

  const prm = await holen(metadataUrl);
  expect(prm.status).toBe(200);
  // RFC 9728 §3.3: Die Kennung muss zu dem Dokument passen, unter dem sie
  // gefunden wurde. Wer hier etwas anderes findet, verwirft das Dokument.
  expect(prm.inhalt.resource).toBe(`${BASE_URL}/mcp`);
  expect(prm.inhalt.authorization_servers?.[0]).toBe(BASE_URL);

  const asUrl = `${prm.inhalt.authorization_servers[0]}/.well-known/oauth-authorization-server`;
  const as = await holen(asUrl);
  expect(as.status).toBe(200);
  expect(as.inhalt.authorization_endpoint).toContain(ISSUER);
  expect(as.inhalt.token_endpoint).toContain(ISSUER);
  expect(as.inhalt.registration_endpoint).toBe(`${BASE_URL}/oauth/register`);
  expect(as.inhalt.code_challenge_methods_supported).toContain('S256');

  return { prm: prm.inhalt, as: as.inhalt };
}

/** Registriert sich per Dynamic Client Registration und liefert die Client-ID. */
async function registrieren() {
  const antwort = await holen(`${BASE_URL}/oauth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ redirect_uris: [RUECKSPRUNG], client_name: 'Claude' }),
  });
  expect(antwort.status, `Registrierung fehlgeschlagen: ${antwort.text}`).toBe(201);
  expect(antwort.inhalt.token_endpoint_auth_method).toBe('none');
  return antwort.inhalt.client_id;
}

/**
 * Fährt Authorize + Login im Browser durch und liefert den Code aus der
 * abgefangenen Rücksprung-Adresse.
 */
async function codeHolen(page, { as, prm, clientId, benutzer, passwort }) {
  const { verifier, challenge } = pkce();
  const zustand = crypto.randomBytes(16).toString('base64url');

  // Genau die Scopes, die der Server anbietet — mehr weiß ein Client nicht.
  // Fehlt darunter der Rollen-Scope, kommt gleich ein Token ohne Rollen
  // zurück und der Werkzeugaufruf endet in 403.
  const scopes = prm.scopes_supported.join(' ');

  const ziel = new URL(as.authorization_endpoint);
  ziel.search = new URLSearchParams({
    client_id: clientId,
    redirect_uri: RUECKSPRUNG,
    response_type: 'code',
    scope: scopes,
    state: zustand,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  }).toString();

  let rueckkehr = null;
  await page.route('https://claude.ai/**', async (route) => {
    rueckkehr = route.request().url();
    await route.fulfill({
      status: 200,
      contentType: 'text/html; charset=utf-8',
      body: '<!doctype html><title>Rücksprung</title><p>abgefangen</p>',
    });
  });

  await page.goto(ziel.toString());
  await anmeldemaskeDurchlaufen(page, benutzer, passwort, () => rueckkehr !== null);

  const zurueck = new URL(rueckkehr);
  expect(zurueck.searchParams.get('error'), `Rücksprung mit Fehler: ${rueckkehr}`).toBeNull();
  expect(zurueck.searchParams.get('state')).toBe(zustand);
  const code = zurueck.searchParams.get('code');
  expect(code, `kein Code im Rücksprung: ${rueckkehr}`).toBeTruthy();

  return { code, verifier, scopes };
}

/** Tauscht den Code am Token-Endpunkt der Rössing-ID gegen ein Access-Token. */
async function tokenTauschen(as, { clientId, code, verifier }) {
  const antwort = await holen(as.token_endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: RUECKSPRUNG,
      client_id: clientId,
      code_verifier: verifier,
    }).toString(),
  });
  expect(antwort.status, `Code-Tausch fehlgeschlagen: ${antwort.text}`).toBe(200);
  expect(antwort.inhalt.access_token, 'Antwort ohne access_token').toBeTruthy();
  return antwort.inhalt.access_token;
}

test('MCP: Ohne Token wird abgewiesen — samt Wegbeschreibung zur Anmeldung', async () => {
  const { prm, as } = await anmeldewegEntdecken();

  // Der Rollen-Scope ist der Unterschied zwischen „angemeldet" und
  // „angemeldet und darf etwas": Zitadel legt die Projektrollen nur ins
  // Access-Token, wenn der Client sie anfragt. Anfragen kann er sie nur,
  // wenn sie hier stehen.
  const rollenScope = 'urn:zitadel:iam:org:projects:roles';
  expect(prm.scopes_supported).toContain(rollenScope);
  expect(as.scopes_supported).toContain(rollenScope);
});

test('MCP: Die Registrierung ist aus dem Browser heraus benutzbar', async ({ page }) => {
  // Die Einrichtung eines Connectors läuft in claude.ai zu einem Teil im
  // Browser. Ohne CORS bricht der Browser schon vor der eigentlichen Anfrage
  // ab — der Client sieht dann nie, dass es eine Registrierung gibt, und
  // kann nur noch nach einer Client-ID fragen.
  //
  // Geprüft wird das aus einer fremden Herkunft heraus, mit einem echten
  // Browser: Die Seite liegt auf dem Zitadel-Port, der Endpunkt auf dem des
  // Backends. Node selbst würde CORS gar nicht durchsetzen und damit nichts
  // beweisen.
  await page.goto(`${ISSUER}/.well-known/openid-configuration`);

  const ergebnis = await page.evaluate(async ({ url, ziel }) => {
    try {
      const antwort = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ redirect_uris: [ziel], client_name: 'Claude' }),
      });
      return { status: antwort.status, inhalt: await antwort.json() };
    } catch (e) {
      return { fehler: String(e) };
    }
  }, { url: `${BASE_URL}/oauth/register`, ziel: RUECKSPRUNG });

  expect(ergebnis.fehler, `Registrierung im Browser blockiert: ${ergebnis.fehler}`).toBeUndefined();
  expect(ergebnis.status).toBe(201);
  expect(ergebnis.inhalt.client_id).toBe(state().mcpClientId);
});

test('MCP: Fremde Rücksprung-Adressen bekommen keine Client-ID', async () => {
  const antwort = await holen(`${BASE_URL}/oauth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ redirect_uris: ['https://boese.example/callback'] }),
  });
  expect(antwort.status).toBe(400);
  expect(antwort.text).not.toContain(state().mcpClientId);
});

test('MCP: Der ganze Weg trägt — von der Abweisung bis zum Werkzeugaufruf', async ({ page }) => {
  const { prm, as } = await anmeldewegEntdecken();
  const clientId = await registrieren();
  expect(clientId).toBe(state().mcpClientId);

  const { admin } = state();
  const { code, verifier } = await codeHolen(page, {
    as, prm, clientId, benutzer: admin.userName, passwort: admin.password,
  });
  const token = await tokenTauschen(as, { clientId, code, verifier });

  // Ab hier redet claude.ai mit dem Server: Handschlag, Werkzeugliste,
  // Werkzeugaufruf.
  const handschlag = await mcp(token, 'initialize', { protocolVersion: '2025-06-18' });
  expect(handschlag.status, `initialize: ${handschlag.text}`).toBe(200);
  expect(handschlag.inhalt.result.serverInfo.name).toBe('dorf-app');

  const werkzeuge = await mcp(token, 'tools/list', null);
  expect(werkzeuge.status, `tools/list: ${werkzeuge.text}`).toBe(200);
  const namen = werkzeuge.inhalt.result.tools.map((w) => w.name);
  expect(namen).toContain('orte_liste');

  const aufruf = await mcp(token, 'tools/call', { name: 'orte_liste', arguments: {} });
  expect(aufruf.status, `tools/call: ${aufruf.text}`).toBe(200);
  expect(aufruf.inhalt.result.isError, `orte_liste meldete einen Fehler: ${aufruf.text}`).toBe(false);

  // Gegenprobe: ein erfundenes Token kommt nicht durch, und der Server sagt
  // auch dann noch, wo der Anmeldeweg steht.
  const falsch = await mcp('kein-echtes-token', 'tools/list', null);
  expect(falsch.status).toBe(401);
  expect(falsch.headers.get('www-authenticate') || '').toContain('oauth-protected-resource');
});

test('MCP: Wer nur Mitglied ist, kommt durch die Anmeldung und trotzdem nicht rein', async ({ page }) => {
  const { prm, as } = await anmeldewegEntdecken();
  const clientId = await registrieren();

  const { member } = state();
  const { code, verifier } = await codeHolen(page, {
    as, prm, clientId, benutzer: member.userName, passwort: member.password,
  });
  const token = await tokenTauschen(as, { clientId, code, verifier });

  // Das Token ist echt und gültig — es fehlt allein die Rolle. Genau das
  // muss der Endpunkt unterscheiden: 403, nicht 401.
  const werkzeuge = await mcp(token, 'tools/list', null);
  expect(werkzeuge.status, `Mitglied bekam HTTP ${werkzeuge.status}: ${werkzeuge.text}`).toBe(403);
});
