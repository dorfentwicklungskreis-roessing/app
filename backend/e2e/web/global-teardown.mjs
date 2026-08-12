// Stoppt das im Setup gestartete Backend-Binary.
import fs from 'node:fs';
import { STATE_FILE } from './config.mjs';

export default async function globalTeardown() {
  if (!fs.existsSync(STATE_FILE)) return;
  const state = JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'));
  if (state.pid) {
    try {
      process.kill(state.pid, 'SIGTERM');
    } catch { /* schon beendet */ }
  }
}
