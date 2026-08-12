# Dorf-App Rössing 🌻

Die App fürs Dorf: Login mit der **Rössing-ID**, Karte der Blumenkästen und
Beete mit Ampel-Status (grün/gelb/rot), Gieß- und Jätpläne, Erledigungen
melden. Langfristig: ERNA-Mitgliederverwaltung u.v.m.

## Aufbau (Monorepo)

| Verzeichnis | Inhalt |
|---|---|
| `android/` | Native Android-App (Kotlin, Jetpack Compose, Material 3, MapLibre) |
| `backend/` | Go-Backend: REST-API, MCP-Server, Web-Admin. SQLite (WAL) |
| `deploy/`  | Kustomize-Overlay für den K3S-Cluster (Flux deployt) |
| `.github/workflows/` | CI: Tests, E2E auf Emulatoren, Multi-Arch-Images, Releases |

## Architektur

- **Identität**: Zitadel auf `id.xn--rssing-wxa.de` („Rössing-ID").
  - App-Login: OIDC Authorization Code + PKCE im System-Browser (AppAuth), mit Consent-Screen.
  - Projekt `dorf-app` mit Rollen `admin` und `member`. Jeder eingeloggte
    Dorfbewohner darf Erledigungen melden; nur `admin` darf verwalten.
- **Backend** (`app.xn--rssing-wxa.de`):
  - `GET/POST/PUT/DELETE /api/v1/…` — REST-API (JWT-geprüft via JWKS)
  - `/mcp` — MCP-Server (Streamable HTTP) für Admin aus Claude heraus.
    Auth: OAuth gegen die Rössing-ID (RFC 9728 Protected Resource),
    admin-Rolle erforderlich — kein statisches Token
  - `/admin` — Web-Admin (OIDC-PKCE-Login, Karte, CRUD)
  - SQLite im WAL-Modus auf einem PVC (`/data/dorfapp.sqlite`)
- **Domänenmodell**: Orte (`blumenkasten`, `beet`, `sonstiges`) haben
  Pflegeaufgaben (`giessen` mit Litern, `jaeten`, `sonstiges`), je mit
  Intervall (→ gelb) und Rot-Schwelle. Globaler **Hitzefaktor** (z.B. 0.5)
  beschleunigt nur Gieß-Aufgaben.

## Entwicklung

```sh
# Backend lokal (ohne echte Auth, mit Beispieldaten)
cd backend
DB_PATH=/tmp/dorf.sqlite AUTH_MODE=insecure-dev SEED=1 \
  ADMIN_CLIENT_ID=385942875872952515 go run ./cmd/server
# → http://localhost:8080/admin/ (Web-Admin); MCP lokal: Bearer "sub:Name:admin"

# Android (Dev-Login + lokales Backend)
cd android
./gradlew assembleDebug -PdevAuth=true -PapiBaseUrl=http://10.0.2.2:8080
./gradlew testDebugUnitTest            # Unit-Tests
./gradlew connectedDebugAndroidTest    # UI-Tests (Emulator nötig)
```

Der „Entwickler-Login" erscheint nur in Debug-Builds mit `-PdevAuth=true`
und funktioniert nur gegen ein Backend mit `AUTH_MODE=insecure-dev`.

## MCP für Admins

In claude.ai einbinden (Einstellungen → Connectors → Custom Connector):

```
URL:             https://app.xn--rssing-wxa.de/mcp
OAuth-Client-ID: 385946294599876803   (kein Secret, PKCE)
```

Beim Verbinden loggt man sich mit der Rössing-ID ein; nur Nutzer mit der
Projektrolle `admin` kommen durch.

Tools: `orte_liste`, `ort_anlegen/aendern/loeschen`,
`aufgabe_anlegen/aendern/loeschen`, `erledigung_melden`, `hitzefaktor_setzen`.

## Deployment

Push auf `main` mit Backend-Änderungen → GitHub Actions baut ein
Multi-Arch-Image (amd64 + arm64, native Runner), bumpt den Tag in
`deploy/overlays/production/kustomization.yaml`, Flux rollt aus
(GitRepository/Kustomization im `server-config`-Repo).

## Releases (Android)

Git-Tag `v*` → signiertes APK/AAB als GitHub-Release. Play-Store-Upload und
Firebase App Distribution aktivieren sich, sobald die Secrets existieren
(siehe `.github/workflows/release.yml`, Kopfkommentar).
