// Richtet im LOKALEN Zitadel (backend/e2e/docker-compose.yml) alles ein, was
// der Android-E2E für einen echten Rössing-ID-Login braucht:
//
//   * Projekt „dorf-app-android-e2e" mit den Rollen admin und member
//   * eine NATIVE OIDC-Anwendung mit PKCE und der Redirect-URI der App
//     (de.roessing.app:/oauth2redirect) — genau die Bauart der ausgelieferten
//     App, kein Sonderweg für den Test
//   * ein menschlicher Testnutzer mit Passwort und der Rolle admin
//
// Damit ersetzt der Lauf das, was bisher in der PRODUKTIONS-Zitadel stand
// (Testkonto, Projekt, Client-ID) — reproduzierbar, ohne Handarbeit und ohne
// GitHub-Secrets. Jeder CI-Lauf bekommt seinen eigenen Nutzer (Zeitstempel im
// Namen), deshalb behindern sich gleichzeitige Läufe nicht mehr.
//
// Aufruf:
//   node android/e2e/zitadel-bootstrap.mjs \
//     --issuer http://10.0.2.2:8123 \
//     --key backend/e2e/machinekey/zitadel-admin-sa.json \
//     --out /tmp/android-e2e-zitadel.env
//
// Ausgabe ist eine Datei mit KEY=WERT-Zeilen (für $GITHUB_ENV geeignet):
//   E2E_OIDC_ISSUER, E2E_OIDC_CLIENT_ID, E2E_OIDC_PROJECT_ID,
//   E2E_LOGIN_USER, E2E_LOGIN_PASSWORD
import fs from 'node:fs';
import { iamToken, zapi, loginPolicyFuerTests, menschAnlegen } from '../../backend/e2e/web/zitadel.mjs';

/** Die Redirect-URI der App — muss zu OIDC_REDIRECT_URI in build.gradle.kts passen. */
const REDIRECT_URI = 'de.roessing.app:/oauth2redirect';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`--${name} fehlt`);
}

async function warteAuf(url, timeoutMs) {
  const ende = Date.now() + timeoutMs;
  for (;;) {
    try {
      const r = await fetch(url);
      if (r.ok) return await r.json();
    } catch { /* noch nicht da */ }
    if (Date.now() > ende) throw new Error(`${url} wurde nicht bereit`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

const issuer = arg('issuer');
const keyPath = arg('key');
const out = arg('out');

// Der Aussteller im Discovery-Dokument MUSS wörtlich dem entsprechen, was die
// App und das Backend konfiguriert bekommen — sonst weist AppAuth die Antwort
// ab. Lieber hier laut scheitern als später im Emulator raten.
const disco = await warteAuf(`${issuer}/.well-known/openid-configuration`, 180_000);
if (disco.issuer !== issuer) {
  throw new Error(
    `Zitadel meldet den Aussteller „${disco.issuer}", erwartet war „${issuer}". ` +
    'ZITADEL_EXTERNALDOMAIN/-PORT im docker compose passen nicht zur Adresse, ' +
    'unter der Emulator und Backend die Instanz erreichen.',
  );
}

const token = await iamToken(issuer, keyPath);
const stempel = Date.now();

await loginPolicyFuerTests(issuer, token);

const projekt = await zapi(issuer, token, 'POST', '/management/v1/projects', {
  name: `dorf-app-android-e2e-${stempel}`,
  // Ohne Rollen-Zusicherung legt Zitadel gar keinen Rollen-Claim ins Token —
  // dann wäre in der App niemand Verwaltung. Genau das prüft der Login-Test.
  projectRoleAssertion: true,
  projectRoleCheck: false,
});
const projectId = projekt.id;

for (const [roleKey, displayName] of [['admin', 'Admin'], ['member', 'Mitglied']]) {
  await zapi(issuer, token, 'POST', `/management/v1/projects/${projectId}/roles`, { roleKey, displayName });
}

// Native App mit PKCE ohne Secret — dieselbe Bauart wie die Android-App in der
// Produktion (Client-ID 385941807986376899). devMode erlaubt den Klartext-
// Aussteller http://10.0.2.2:8123; produktiv läuft alles über https.
const app = await zapi(issuer, token, 'POST', `/management/v1/projects/${projectId}/apps/oidc`, {
  name: 'Dorf-App Android E2E',
  redirectUris: [REDIRECT_URI],
  responseTypes: ['OIDC_RESPONSE_TYPE_CODE'],
  grantTypes: ['OIDC_GRANT_TYPE_AUTHORIZATION_CODE', 'OIDC_GRANT_TYPE_REFRESH_TOKEN'],
  appType: 'OIDC_APP_TYPE_NATIVE',
  authMethodType: 'OIDC_AUTH_METHOD_TYPE_NONE',
  postLogoutRedirectUris: [REDIRECT_URI],
  devMode: true,
  // Das Backend prüft ein JWT gegen den Aussteller; ein opakes Token käme dort
  // nicht durch. Die Rollen müssen im Access-Token stehen, dort liest sie
  // backend/internal/auth.
  accessTokenType: 'OIDC_TOKEN_TYPE_JWT',
  accessTokenRoleAssertion: true,
  idTokenRoleAssertion: true,
});

const nutzer = await menschAnlegen({
  issuer,
  token,
  projectId,
  // Der Zeitstempel im Namen ist Absicht: Zwei gleichzeitige CI-Läufe teilten
  // sich bisher EIN Produktionskonto und fielen sich gegenseitig aus der
  // Sitzung. Jetzt hat jeder Lauf seinen eigenen Nutzer.
  userName: `test-android-${stempel}`,
  password: 'Test-Dorf-2026!',
  roleKeys: ['admin'],
});

const zeilen = [
  `E2E_OIDC_ISSUER=${issuer}`,
  `E2E_OIDC_CLIENT_ID=${app.clientId}`,
  `E2E_OIDC_PROJECT_ID=${projectId}`,
  `E2E_LOGIN_USER=${nutzer.userName}`,
  `E2E_LOGIN_PASSWORD=${nutzer.password}`,
];
fs.writeFileSync(out, `${zeilen.join('\n')}\n`);
console.log(`Zitadel eingerichtet: Projekt ${projectId}, Client ${app.clientId}, Nutzer ${nutzer.userName}`);
console.log(`Werte geschrieben nach ${out}`);
