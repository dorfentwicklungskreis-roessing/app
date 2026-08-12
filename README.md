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
  - `/admin` — Web-Verwaltung: server-gerendertes Multi-Page-Interface
    (Go `html/template`, Tailwind v4 + DaisyUI v5). Echte Seitennavigation,
    Formulare per Post/Redirect/Get, keine Modals, ohne JavaScript bedienbar
    (JS nur für die Karte). Anmeldung: OIDC Authorization Code + PKCE
    **serverseitig** — im Browser liegt nur ein signiertes, HttpOnly-Cookie.
    Aufbau nach Bereichen: `/admin/` listet die Bereiche der Dorf-App,
    die Dorfpflege liegt unter `/admin/dorfpflege/…`
  - SQLite im WAL-Modus auf einem PVC (`/data/dorfapp.sqlite`)
- **Domänenmodell**: Orte (`blumenkasten`, `beet`, `sonstiges`) haben
  Pflegeaufgaben (`giessen` mit Litern, `jaeten`, `sonstiges`), je mit
  Intervall (→ gelb) und Rot-Schwelle. Globaler **Hitzefaktor** (z.B. 0.5)
  beschleunigt nur Gieß-Aufgaben.
- **Rangliste**: `GET /api/v1/stats/leaderboard?period=woche|monat|saison|jahr|gesamt`
  (Standard `saison` = 1. März bis 31. Oktober, Grenzen in Ortszeit) zeigt je
  Person Anzahl und Liter, die Gesamtsummen des Dorfes, den eigenen Rang und
  schlichte Auszeichnungen (Gießkanne des Monats, Frühaufsteher, Retter,
  Ausdauer). Eine irrtümliche Meldung nimmt
  `DELETE /api/v1/completions/{id}` zurück — erlaubt dem Melder und Admins.
  In der App gibt es dazu den Tab „Rangliste"; gemeldet wird erst nach einer
  Rückfrage (Ort und Menge), damit ein Fehlklick nichts einträgt. In der
  Verwaltung liegt beides unter `/admin/dorfpflege/rangliste` bzw. als
  eigene Bestätigungsseite in der Historie eines Ortes.
- **Spielschutz**: Nach einer Erledigung bleibt dieselbe Aufgabe gesperrt —
  50 % des Soll-Intervalls (beim Gießen mit dem Hitzefaktor skaliert, bei
  Jäten & Co. nicht), mindestens 12 Stunden, höchstens das volle Intervall.
  Ein Verstoß ergibt **HTTP 409** mit `{"error":…,"retryAfter":…}` (RFC3339);
  `GET /api/v1/places` liefert je Aufgabe `lockedUntil` (fehlt, wenn nicht
  gesperrt), damit die App den Knopf gar nicht erst anbietet. Prüfen und
  Eintragen laufen in einer Transaktion, damit ein Doppeltipp nicht zwei
  Meldungen erzeugt. Admins dürfen mit `force: true` übergehen (wird als
  `forced` vermerkt) und mit `doneAt` bis zu 14 Tage zurückdatieren;
  Zeitpunkte in der Zukunft werden abgelehnt. Die Sperre gilt je Aufgabe,
  nicht je Person — sonst könnten mehrere Leute denselben Kasten nacheinander
  „gießen". Dieselbe Prüfung gilt für REST, MCP und die Web-Verwaltung; dort
  zeigt die Ortsseite „Bereits erledigt — wieder ab …" und die
  Bestätigungsseite bietet das Übergehen als Haken an (kein Popup).
- **Wertung der Rangliste**: Gezählt wird nur, was eine echte Erledigung sein
  kann — eine Meldung auf eine Aufgabe, die zu diesem Zeitpunkt nicht frisch
  erledigt war (Sperrfrist abgelaufen, Ampel also gelb oder rot). Alles
  andere bleibt in der Historie sichtbar, zählt aber weder für Rang, Liter,
  Gesamtsummen noch für Auszeichnungen; das betrifft vor allem den
  Altbestand aus der Zeit vor dem Spielschutz. Erzwungene Nachträge eines
  Admins (`forced`) zählen der genannten Person normal und sind in der
  Historie als „nachgetragen" gekennzeichnet. Die Regel steht als SQL in
  `backend/internal/db/stats.go` (`gewertetSQL`) und rechnet mit dem aktuell
  eingestellten Hitzefaktor — er ist eine tagesaktuelle Einstellung und wird
  nicht historisiert.

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

### CSS der Verwaltung

Das ausgelieferte CSS wird gebaut und **committet** (`go:embed`), damit zur
Laufzeit nichts von einem CDN kommt:

```sh
cd backend/internal/admin
npm ci && npm run build:css   # schreibt static/app.css
```

Die CI baut das CSS neu und schlägt fehl, wenn es vom committeten abweicht.

### End-to-End-Tests (echtes Zitadel, keine Mocks)

Beide E2E-Suiten brauchen die Compose-Umgebung mit echtem Zitadel:

```sh
cd backend
mkdir -p e2e/machinekey && chmod 777 e2e/machinekey
docker compose -f e2e/docker-compose.yml up -d --wait

# API + MCP gegen echte Tokens
go test -tags e2e -v ./e2e/

# Web-Admin im echten Browser (Login über die Zitadel-Oberfläche)
cd e2e/web && npm ci && npx playwright install --with-deps chromium && npx playwright test
```

Der Browser-E2E bootstrappt Projekt, Rollen, eine PKCE-App und zwei Nutzer
mit Passwort in Zitadel, startet das echte Backend-Binary und lässt den Test
scheitern, sobald im Browser eine Konsolen- oder Skriptfehlermeldung auftritt.
Geprüft werden echte Seitenwechsel (URL-Vergleich), das Anlegen von Ort und
Aufgabe, Erledigungen, Hitzefaktor, Löschen über die Bestätigungsseite, das
Rollen-Gating — und ein kompletter Durchlauf **mit abgeschaltetem JavaScript**.

### Konfiguration des Backends (Env)

| Variable | Bedeutung |
|---|---|
| `LISTEN_ADDR` | Standard `:8080` |
| `DB_PATH` | Standard `/data/dorfapp.sqlite` |
| `AUTH_ISSUER` | Rössing-ID, Standard `https://id.xn--rssing-wxa.de` |
| `AUTH_MODE` | `oidc` (Standard) oder `insecure-dev` (nur lokal/E2E) |
| `PUBLIC_URL` | öffentliche Basis-URL; daraus entsteht die OIDC-Redirect-URI `{PUBLIC_URL}/admin/` |
| `ADMIN_CLIENT_ID` | Client-ID der Verwaltung (leer = nur Startseite) |
| `SESSION_KEY` | Schlüssel für die signierten Session-Cookies; leer = zufällig beim Start (Sessions überleben dann keinen Neustart) |
| `MCP_CLIENT_ID` | PKCE-Client für die MCP-Anbindung |
| `SEED` | `1` → Beispieldaten anlegen, falls die DB leer ist |
### Dev-Container

`.devcontainer/` beschreibt eine fertige Umgebung (JDK 21, Android-SDK 35,
Go 1.23, Node, Claude-CLI) für VS Code, Codespaces oder die `devcontainer`-CLI:

```sh
devcontainer up --workspace-folder .
```

Der Emulator ist mit drin; das System-Image lädt man einmalig nach und der
Container braucht `/dev/kvm`. Die genauen Befehle stehen als Kommentar in
`.devcontainer/devcontainer.json`.

## MCP für Admins

In claude.ai einbinden (Einstellungen → Connectors → Custom Connector):

```
URL: https://app.xn--rssing-wxa.de/mcp
```

Mehr nicht — claude.ai registriert sich per Dynamic Client Registration
(das Backend spiegelt dafür die AS-Metadata von Zitadel und beantwortet
DCR mit der festen PKCE-Client-ID). Beim Verbinden loggt man sich mit der
Rössing-ID ein; nur Nutzer mit der Projektrolle `admin` kommen durch.

Tools: `orte_liste`, `ort_anlegen/aendern/loeschen`,
`aufgabe_anlegen/aendern/loeschen`, `erledigung_melden`,
`erledigung_zuruecknehmen`, `rangliste`, `hitzefaktor_setzen`.

## Deployment

Push auf `main` mit Backend-Änderungen → GitHub Actions baut ein
Multi-Arch-Image (amd64 + arm64, native Runner), bumpt den Tag in
`deploy/overlays/production/kustomization.yaml`, Flux rollt aus
(GitRepository/Kustomization im `server-config`-Repo).

## Releases (Android)

Git-Tag `v*` → signiertes APK/AAB als GitHub-Release. Play-Store-Upload und
Firebase App Distribution aktivieren sich, sobald die Secrets existieren
(siehe `.github/workflows/release.yml`, Kopfkommentar).

## Veröffentlichung im Play Store

Alles, was ohne Play-Console-Konto vorbereitbar ist, liegt in **`store/`**:

| Datei | Inhalt |
|---|---|
| `store/metadata/android/{de-DE,en-US}/` | Store-Texte im Fastlane-Format (Titel, Kurz-/Langbeschreibung, Änderungshinweise je `versionCode`) |
| `store/metadata/android/de-DE/images/` | Icon 512×512 und Feature-Grafik 1024×500; Screenshots fehlen noch (README im Unterverzeichnis erklärt, wie sie entstehen) |
| `store/assets/` | SVG-Quellen der Grafiken + `render.sh` |
| `store/veroeffentlichung.md` | Schritt-für-Schritt in der Play Console: Kontotyp, Play App Signing, Service-Account, Secret |
| `store/data-safety.md` | Antworten für das Formular „Datensicherheit", belegt am Code |
| `store/content-rating.md` | Antworten für den IARC-Fragebogen zur Altersfreigabe |
| `store/datenschutz.md` | Kurzfassung; die verbindliche Erklärung kommt öffentlich auf roessing.de |

```sh
python3 store/check_metadata.py   # Zeichenlimits, Vollständigkeit, Bildmaße
bash store/assets/render.sh       # Grafiken neu erzeugen (ImageMagick)
```

`.github/workflows/store.yml` prüft die Metadaten bei jeder Änderung an
`store/` oder am `versionCode`; der Release-Workflow prüft sie erneut, bevor er
das AAB auf den Play-Track **`internal`** lädt. Fehlt das Secret
`PLAY_SERVICE_ACCOUNT_JSON`, wird der Upload übersprungen.

**Offen:** Der `versionCode` steht fest in `android/app/build.gradle.kts`; Play
verlangt bei jedem Upload einen höheren. Vorschlag zur Automatisierung in
`store/README.md`.
