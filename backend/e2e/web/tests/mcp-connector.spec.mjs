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
import { test, expect, state, anmelden, anmeldemaskeDurchlaufen } from '../fixtures.mjs';
import { BASE_URL, ISSUER } from '../config.mjs';

const REDIRECT_URI = 'https://claude.ai/api/mcp/auth_callback';

// Der Rollen-Scope ist der Unterschied zwischen „angemeldet" und „angemeldet
// und darf etwas": Zitadel legt die Projektrollen nur ins Access-Token, wenn
// der Client sie anfragt — und anfragen kann er nur, was der Server nennt.
const ROLES_SCOPE = 'urn:zitadel:iam:org:projects:roles';

/** Erzeugt Verifier und Challenge für PKCE (S256). */
function pkce() {
  const verifier = crypto.randomBytes(48).toString('base64url');
  const challenge = crypto.createHash('sha256').update(verifier).digest('base64url');
  return { verifier, challenge };
}

/** Holt ein JSON-Dokument und gibt Status, Kopfzeilen und Inhalt zurück. */
async function fetchJson(url, init) {
  const response = await fetch(url, init);
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = null;
  }
  return { status: response.status, headers: response.headers, body, text };
}

/** Ruft den MCP-Endpunkt per JSON-RPC auf. */
async function callMcp(token, method, params) {
  return fetchJson(`${BASE_URL}/mcp`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
  });
}

/**
 * Geht den Weg, den ein Client nimmt, der nur die Adresse des Endpunkts kennt:
 * abgewiesen werden, dem Hinweis folgen, die Metadata lesen.
 */
async function discoverAuthFlow() {
  const rejected = await callMcp(null, 'tools/list', null);
  expect(rejected.status, 'ohne Token muss der Endpunkt abweisen').toBe(401);

  const challenge = rejected.headers.get('www-authenticate') || '';
  const metadataUrl = /resource_metadata="([^"]+)"/.exec(challenge)?.[1];
  expect(metadataUrl, `WWW-Authenticate ohne Metadata-Adresse: ${challenge}`).toBeTruthy();
  // Ein Browser-Client darf die Kopfzeile nur lesen, wenn sie freigegeben ist.
  // Ohne diese Freigabe steht der ganze Weg da, aber niemand kommt an ihn heran.
  expect(rejected.headers.get('access-control-expose-headers') || '')
    .toContain('WWW-Authenticate');

  const resource = await fetchJson(metadataUrl);
  expect(resource.status).toBe(200);
  // RFC 9728 §3.3: Die Kennung muss zu dem Dokument passen, unter dem sie
  // gefunden wurde. Wer hier etwas anderes findet, verwirft das Dokument.
  expect(resource.body.resource).toBe(`${BASE_URL}/mcp`);
  expect(resource.body.authorization_servers?.[0]).toBe(BASE_URL);

  const serverUrl = `${resource.body.authorization_servers[0]}/.well-known/oauth-authorization-server`;
  const server = await fetchJson(serverUrl);
  expect(server.status).toBe(200);
  expect(server.body.authorization_endpoint).toContain(ISSUER);
  expect(server.body.token_endpoint).toContain(ISSUER);
  expect(server.body.registration_endpoint).toBe(`${BASE_URL}/oauth/register`);
  expect(server.body.code_challenge_methods_supported).toContain('S256');
  // Ein öffentlicher Client ohne Secret — sonst suchte claude.ai eines.
  expect(server.body.token_endpoint_auth_methods_supported).toContain('none');

  return { resource: resource.body, server: server.body };
}

/** Registriert sich per Dynamic Client Registration und liefert die Client-ID. */
async function registerClient() {
  const response = await fetchJson(`${BASE_URL}/oauth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ redirect_uris: [REDIRECT_URI], client_name: 'Claude' }),
  });
  expect(response.status, `Registrierung fehlgeschlagen: ${response.text}`).toBe(201);
  expect(response.body.token_endpoint_auth_method).toBe('none');
  return response.body.client_id;
}

/**
 * Fährt Authorize + Login im Browser durch und liefert den Code aus der
 * abgefangenen Rücksprung-Adresse.
 */
async function fetchAuthCode(page, { server, resource, clientId, user, password }) {
  const { verifier, challenge } = pkce();
  const stateParam = crypto.randomBytes(16).toString('base64url');

  // Genau die Scopes, die der Server anbietet — mehr weiß ein Client nicht.
  // Fehlt darunter der Rollen-Scope, kommt gleich ein Token ohne Rollen
  // zurück und der Werkzeugaufruf endet in 403.
  const scopes = resource.scopes_supported.join(' ');

  const target = new URL(server.authorization_endpoint);
  target.search = new URLSearchParams({
    client_id: clientId,
    redirect_uri: REDIRECT_URI,
    response_type: 'code',
    scope: scopes,
    state: stateParam,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  }).toString();

  let returned = null;
  await page.route('https://claude.ai/**', async (route) => {
    returned = route.request().url();
    await route.fulfill({
      status: 200,
      contentType: 'text/html; charset=utf-8',
      body: '<!doctype html><title>Rücksprung</title><p>abgefangen</p>',
    });
  });

  await page.goto(target.toString());
  await anmeldemaskeDurchlaufen(page, user, password, () => returned !== null);

  const back = new URL(returned);
  expect(back.searchParams.get('error'), `Rücksprung mit Fehler: ${returned}`).toBeNull();
  expect(back.searchParams.get('state')).toBe(stateParam);
  const code = back.searchParams.get('code');
  expect(code, `kein Code im Rücksprung: ${returned}`).toBeTruthy();

  return { code, verifier, scopes };
}

/** Tauscht den Code am Token-Endpunkt der Rössing-ID gegen ein Access-Token. */
async function exchangeCode(server, { clientId, code, verifier }) {
  const response = await fetchJson(server.token_endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: REDIRECT_URI,
      client_id: clientId,
      code_verifier: verifier,
    }).toString(),
  });
  expect(response.status, `Code-Tausch fehlgeschlagen: ${response.text}`).toBe(200);
  expect(response.body.access_token, 'Antwort ohne access_token').toBeTruthy();
  return response.body.access_token;
}

test('Verwaltung: Die Adresse des Connectors steht in der Verwaltung', async ({ page }) => {
  // Wer den Connector einrichtet, sitzt am Rechner in der Web-Verwaltung.
  // Findet er die Adresse dort nicht, bleibt nur: im Quelltext nachsehen oder
  // jemanden fragen. Deshalb wird sie hier so gesucht, wie er sie sucht —
  // von der Übersicht aus, ohne die Adresse vorher zu kennen.
  const { admin } = state();
  await anmelden(page, admin.userName, admin.password);

  await page.locator('#bereich-connector a').click();
  await page.waitForURL(/\/admin\/connector\//);

  await expect(page.locator('#mcp-address')).toHaveText(`${BASE_URL}/mcp`);
  // Die Frage, an der der Betreiber tatsächlich hängenblieb.
  await expect(page.locator('#step-client')).toContainText('automatisch erzeugten');

  // Und aus dem Menü heraus ebenso — eine Seite, die man nur über die
  // Übersicht findet, ist auf jeder anderen Seite verschwunden.
  await page.goto('/admin/ideen/');
  await page.getByRole('link', { name: 'Mit Claude' }).click();
  await page.waitForURL(/\/admin\/connector\//);
});

test('MCP: Ohne Token wird abgewiesen — samt Wegbeschreibung zur Anmeldung', async () => {
  const { resource, server } = await discoverAuthFlow();
  expect(resource.scopes_supported).toContain(ROLES_SCOPE);
  expect(server.scopes_supported).toContain(ROLES_SCOPE);
});

test('MCP: Die Registrierung ist aus dem Browser heraus benutzbar', async ({ page }) => {
  // Die Einrichtung eines Connectors kann zu einem Teil im Browser laufen.
  // Ohne CORS bricht der Browser schon vor der eigentlichen Anfrage ab — der
  // Client sieht dann nie, dass es eine Registrierung gibt, und kann nur noch
  // nach einer Client-ID fragen.
  //
  // Geprüft wird das aus einer fremden Herkunft heraus, mit einem echten
  // Browser: Die Seite liegt auf dem Zitadel-Port, der Endpunkt auf dem des
  // Backends. Node selbst würde CORS gar nicht durchsetzen und damit nichts
  // beweisen.
  await page.goto(`${ISSUER}/.well-known/openid-configuration`);

  const result = await page.evaluate(async ({ url, redirect }) => {
    try {
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ redirect_uris: [redirect], client_name: 'Claude' }),
      });
      return { status: response.status, body: await response.json() };
    } catch (e) {
      return { error: String(e) };
    }
  }, { url: `${BASE_URL}/oauth/register`, redirect: REDIRECT_URI });

  expect(result.error, `Registrierung im Browser blockiert: ${result.error}`).toBeUndefined();
  expect(result.status).toBe(201);
  expect(result.body.client_id).toBe(state().mcpClientId);
});

test('MCP: Fremde Rücksprung-Adressen bekommen keine Client-ID', async () => {
  const response = await fetchJson(`${BASE_URL}/oauth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ redirect_uris: ['https://boese.example/callback'] }),
  });
  expect(response.status).toBe(400);
  expect(response.text).not.toContain(state().mcpClientId);
});

test('MCP: Der ganze Weg trägt — von der Abweisung bis zum Werkzeugaufruf', async ({ page }) => {
  const { resource, server } = await discoverAuthFlow();
  const clientId = await registerClient();
  expect(clientId).toBe(state().mcpClientId);

  const { admin } = state();
  const { code, verifier } = await fetchAuthCode(page, {
    server, resource, clientId, user: admin.userName, password: admin.password,
  });
  const token = await exchangeCode(server, { clientId, code, verifier });

  // Ab hier redet claude.ai mit dem Server: Handschlag, Werkzeugliste,
  // Werkzeugaufruf.
  const handshake = await callMcp(token, 'initialize', { protocolVersion: '2025-06-18' });
  expect(handshake.status, `initialize: ${handshake.text}`).toBe(200);
  expect(handshake.body.result.serverInfo.name).toBe('dorf-app');

  const tools = await callMcp(token, 'tools/list', null);
  expect(tools.status, `tools/list: ${tools.text}`).toBe(200);
  expect(tools.body.result.tools.map((w) => w.name)).toContain('orte_liste');

  const call = await callMcp(token, 'tools/call', { name: 'orte_liste', arguments: {} });
  expect(call.status, `tools/call: ${call.text}`).toBe(200);
  expect(call.body.result.isError, `orte_liste meldete einen Fehler: ${call.text}`).toBe(false);

  // Gegenprobe: ein erfundenes Token kommt nicht durch, und der Server sagt
  // auch dann noch, wo der Anmeldeweg steht.
  const forged = await callMcp('kein-echtes-token', 'tools/list', null);
  expect(forged.status).toBe(401);
  expect(forged.headers.get('www-authenticate') || '').toContain('oauth-protected-resource');
});

test('MCP: Wer nur Mitglied ist, kommt durch die Anmeldung und trotzdem nicht rein', async ({ page }) => {
  const { resource, server } = await discoverAuthFlow();
  const clientId = await registerClient();

  const { member } = state();
  const { code, verifier } = await fetchAuthCode(page, {
    server, resource, clientId, user: member.userName, password: member.password,
  });
  const token = await exchangeCode(server, { clientId, code, verifier });

  // Das Token ist echt und gültig — es fehlt allein die Rolle. Genau das
  // muss der Endpunkt unterscheiden: 403, nicht 401.
  const tools = await callMcp(token, 'tools/list', null);
  expect(tools.status, `Mitglied bekam HTTP ${tools.status}: ${tools.text}`).toBe(403);
});
