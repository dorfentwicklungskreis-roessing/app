// Gemeinsame Konstanten für Setup, Teardown und Tests.
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));

// Muss zum Compose-Setup passen (Zitadel EXTERNALPORT 8123).
export const ISSUER = process.env.E2E_ISSUER || 'http://localhost:8123';
export const PORT = Number(process.env.E2E_PORT || 8124);
export const BASE_URL = process.env.E2E_BASE_URL || `http://localhost:${PORT}`;
export const STATE_FILE = path.join(here, '.e2e-state.json');
