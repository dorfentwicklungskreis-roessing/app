// Baut die komplette Testumgebung auf: echtes Zitadel (docker compose läuft
// bereits), echtes Backend-Binary, echte OIDC-App, echte Nutzer mit Passwort.
import { spawn, spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { bootstrap } from './zitadel.mjs';
import { ISSUER, BASE_URL, PORT, STATE_FILE } from './config.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const backendDir = path.resolve(here, '../..');
const keyPath = path.resolve(here, '../machinekey/zitadel-admin-sa.json');

async function waitFor(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const r = await fetch(url);
      if (r.ok) return;
    } catch { /* noch nicht da */ }
    if (Date.now() > deadline) throw new Error(`${url} wurde nicht bereit`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

export default async function globalSetup() {
  if (process.env.E2E_SKIP_BOOTSTRAP === '1') {
    console.log('Bootstrap übersprungen (E2E_SKIP_BOOTSTRAP=1)');
    return;
  }
  if (!fs.existsSync(keyPath)) {
    throw new Error(`Zitadel-Machine-Key fehlt: ${keyPath}\n` +
      'Zuerst: docker compose -f backend/e2e/docker-compose.yml up -d --wait');
  }

  console.log('→ Warte auf Zitadel …');
  await waitFor(`${ISSUER}/.well-known/openid-configuration`, 120_000);

  console.log('→ Bootstrap in Zitadel (Projekt, Rollen, PKCE-Apps, Nutzer) …');
  const boot = await bootstrap({ issuer: ISSUER, keyPath, redirectUri: `${BASE_URL}/admin/` });
  console.log(`   Client-ID ${boot.clientId}, MCP-Client-ID ${boot.mcpClientId}, ` +
    `Admin ${boot.admin.userName}, Mitglied ${boot.member.userName}`);

  console.log('→ Backend-Binary bauen …');
  const outDir = fs.mkdtempSync(path.join(process.env.RUNNER_TEMP || '/tmp', 'dorf-e2e-'));
  const bin = path.join(outDir, 'server');
  const build = spawnSync('go', ['build', '-o', bin, './cmd/server'], { cwd: backendDir, stdio: 'inherit' });
  if (build.status !== 0) throw new Error('go build fehlgeschlagen');

  console.log('→ Backend starten …');
  const logFile = path.join(here, 'backend.log');
  const log = fs.openSync(logFile, 'w');
  const srv = spawn(bin, [], {
    env: {
      ...process.env,
      LISTEN_ADDR: `:${PORT}`,
      DB_PATH: path.join(outDir, 'e2e.sqlite'),
      AUTH_ISSUER: ISSUER,
      // Empfängerprüfung wie in Produktion: Der Web-Admin tauscht den Code
      // gegen ein Access-Token und lässt es vom selben Verifier prüfen. Zitadel
      // trägt die Client-ID der anfragenden Anwendung als Empfänger ein — genau
      // die steht hier. Dieser Test ist damit der Nachweis, dass die
      // Produktionskonfiguration (Client-IDs in AUTH_AUDIENCE) trägt: Fiele die
      // Client-ID nicht in den aud-Claim, käme hier keine Anmeldung mehr durch.
      // Der Connector kommt mit einer eigenen Anwendung; ohne deren Client-ID
      // in der Liste wiese das Backend jedes Token von claude.ai ab.
      AUTH_AUDIENCE: `${boot.clientId},${boot.mcpClientId}`,
      PUBLIC_URL: BASE_URL,
      ADMIN_CLIENT_ID: boot.clientId,
      // Diese ID gibt der Registrierungs-Endpunkt (/oauth/register) heraus.
      MCP_CLIENT_ID: boot.mcpClientId,
      SEED: '1',
      // Ideen-Eingang: erlaubtes Weiterleitungsziel wie in Produktion. Die
      // Zugriffsgrenze wird hochgesetzt, weil der Browser-Test in Folge
      // einreicht — sie hat einen eigenen Test in internal/api.
      IDEEN_ZIELE: 'https://xn--rssing-wxa.de',
      IDEEN_BURST: '100',
      IDEEN_PRO_STUNDE: '100',
      // Träger-Mitgliedschaften kommen wie in Produktion aus der Rössing-ID:
      // Das Backend fragt sie mit dem Dienst-Nutzer über die Management-API
      // ab. Im E2E ist das derselbe Machine-Key wie beim Bootstrap.
      ZITADEL_SERVICE_USER_KEY_FILE: path.resolve(backendDir, 'e2e/machinekey/zitadel-admin-sa.json'),
      ZITADEL_ROLLEN_TTL: '1s',
    },
    stdio: ['ignore', log, log],
    detached: true,
  });
  srv.unref();
  await waitFor(`${BASE_URL}/healthz`, 60_000);

  fs.writeFileSync(STATE_FILE, JSON.stringify({ ...boot, pid: srv.pid, outDir, logFile }, null, 2));
  console.log(`→ Bereit unter ${BASE_URL} (PID ${srv.pid})`);
}
