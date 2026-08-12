// Zitadel-Bootstrap für den Browser-E2E — spricht die ECHTE Management-API
// der Compose-Instanz an. Keine Mocks, keine Fixtures.
import crypto from 'node:crypto';
import fs from 'node:fs';

/** Signiert die JWT-Bearer-Assertion für einen Zitadel-Machine-Key. */
function signAssertion(key, issuer) {
  const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url');
  const now = Math.floor(Date.now() / 1000);
  const head = b64({ alg: 'RS256', typ: 'JWT', kid: key.keyId });
  const body = b64({ iss: key.userId, sub: key.userId, aud: issuer, iat: now, exp: now + 3600 });
  const sig = crypto.sign('RSA-SHA256', Buffer.from(`${head}.${body}`), key.key).toString('base64url');
  return `${head}.${body}.${sig}`;
}

/** Tauscht den Machine-Key gegen ein Access-Token für die Zitadel-API. */
export async function iamToken(issuer, keyPath) {
  const key = JSON.parse(fs.readFileSync(keyPath, 'utf8'));
  const r = await fetch(`${issuer}/oauth/v2/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'urn:ietf:params:oauth:grant-type:jwt-bearer',
      assertion: signAssertion(key, issuer),
      scope: 'openid urn:zitadel:iam:org:project:id:zitadel:aud',
    }),
  });
  const t = await r.json();
  if (!t.access_token) throw new Error(`Kein IAM-Token: ${JSON.stringify(t)}`);
  return t.access_token;
}

/** Ruft die Zitadel-API auf und wirft bei HTTP-Fehlern. */
export async function zapi(issuer, token, method, path, body) {
  const r = await fetch(issuer + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await r.text();
  if (!r.ok) throw new Error(`${method} ${path}: HTTP ${r.status}: ${text}`);
  return text ? JSON.parse(text) : {};
}

/** Wie zapi, ignoriert aber Fehler (für optionale Aufräum-/Policy-Schritte). */
async function zapiSoft(issuer, token, method, path, body) {
  try {
    return await zapi(issuer, token, method, path, body);
  } catch (e) {
    console.warn(`  (ignoriert) ${e.message.slice(0, 160)}`);
    return null;
  }
}

/**
 * Legt Projekt, Rollen, eine User-Agent-App mit PKCE und zwei menschliche
 * Nutzer (mit Passwort) an. Gibt Client-ID und Zugangsdaten zurück.
 */
export async function bootstrap({ issuer, keyPath, redirectUri }) {
  const token = await iamToken(issuer, keyPath);
  const stamp = Date.now();

  // Zweitfaktor-Prompt abschalten: sonst schiebt die Login-UI nach dem
  // Passwort eine „2FA einrichten"-Seite dazwischen und der Flow wird flaky.
  for (const f of ['SECOND_FACTOR_TYPE_OTP', 'SECOND_FACTOR_TYPE_U2F', 'SECOND_FACTOR_TYPE_OTP_EMAIL', 'SECOND_FACTOR_TYPE_OTP_SMS']) {
    await zapiSoft(issuer, token, 'DELETE', `/admin/v1/policies/login/second_factors/${f}`);
  }
  await zapiSoft(issuer, token, 'PUT', '/admin/v1/policies/login', {
    allowUsernamePassword: true, allowRegister: false, allowExternalIdp: false,
    forceMfa: false, forceMfaLocalOnly: false, passwordlessType: 'PASSWORDLESS_TYPE_NOT_ALLOWED',
    hidePasswordReset: true, ignoreUnknownUsernames: false, disableLoginWithEmail: false,
    disableLoginWithPhone: true,
  });

  const project = await zapi(issuer, token, 'POST', '/management/v1/projects', {
    name: `dorf-app-web-e2e-${stamp}`,
    projectRoleAssertion: true,
    projectRoleCheck: false,
  });
  const projectId = project.id;

  for (const [roleKey, displayName] of [['admin', 'Admin'], ['member', 'Mitglied']]) {
    await zapi(issuer, token, 'POST', `/management/v1/projects/${projectId}/roles`, { roleKey, displayName });
  }

  // User-Agent-App (SPA) mit PKCE — genau die Konfiguration des Web-Admin.
  const app = await zapi(issuer, token, 'POST', `/management/v1/projects/${projectId}/apps/oidc`, {
    name: 'Web-Admin E2E',
    redirectUris: [redirectUri],
    responseTypes: ['OIDC_RESPONSE_TYPE_CODE'],
    grantTypes: ['OIDC_GRANT_TYPE_AUTHORIZATION_CODE'],
    appType: 'OIDC_APP_TYPE_USER_AGENT',
    authMethodType: 'OIDC_AUTH_METHOD_TYPE_NONE',
    postLogoutRedirectUris: [redirectUri],
    devMode: true, // erlaubt http-Redirects auf localhost
    accessTokenType: 'OIDC_TOKEN_TYPE_JWT',
    accessTokenRoleAssertion: true,
    idTokenRoleAssertion: true,
  });

  const newHuman = async (userName, password, roleKeys) => {
    const u = await zapi(issuer, token, 'POST', '/management/v1/users/human/_import', {
      userName,
      profile: { firstName: 'E2E', lastName: userName, displayName: userName, preferredLanguage: 'de' },
      email: { email: `${userName}@e2e.invalid`, isEmailVerified: true },
      password,
      passwordChangeRequired: false,
    });
    if (roleKeys.length) {
      await zapi(issuer, token, 'POST', `/management/v1/users/${u.userId}/grants`, { projectId, roleKeys });
    }
    return { userName, password, userId: u.userId };
  };

  // Der Nutzername „test-dorf" spiegelt den Test-Account der Produktion.
  const admin = await newHuman(`test-dorf-${stamp}`, 'Test-Dorf-2026!', ['admin']);
  const member = await newHuman(`test-mitglied-${stamp}`, 'Test-Dorf-2026!', ['member']);

  return { projectId, clientId: app.clientId, admin, member };
}
