# Claude-Konfiguration für die Dorf-App

**Immer auf Deutsch kommunizieren** (Antworten, Kommentare, PRs, Issues).
GitHub-Magic-Keywords (`fixes`, `closes` …) bleiben Englisch.

## Projekt

Monorepo: `android/` (Kotlin/Compose), `backend/` (Go, SQLite WAL, REST + MCP
+ Web-Admin), `deploy/` (Kustomize, Flux deployt in den K3S-Cluster).
Details: siehe `README.md`.

## Regeln

- **Backend**: Nur Standard-Library-HTTP (`net/http`, Go 1.22-Routing),
  `modernc.org/sqlite` (CGO-frei!), keine schweren Frameworks. Vor jedem
  Commit: `gofmt -w . && go vet ./... && go test ./...`.
- **Web-Verwaltung** (`backend/internal/admin`): server-gerenderte Seiten mit
  `html/template`, echte Navigation, Post/Redirect/Get. **Keine Modals, keine
  Overlays, kein clientseitiger Zustand.** Styling ausschließlich mit Tailwind
  v4 + DaisyUI v5 — kein handgeschriebenes CSS. Das CSS wird mit
  `npm run build:css` erzeugt und **committet** (`static/app.css`, `go:embed`);
  die CI prüft auf Drift. JavaScript nur für die Karte: alles muss ohne JS
  bedienbar bleiben. Bereiche liegen unter eigenen Präfixen
  (`/admin/mithelfen/…`), damit weitere Bereiche danebenpassen. Wird ein
  Bereich umbenannt, bleibt der alte Pfad als 308-Weiterleitung bestehen.
- **Android**: Version Catalog (`gradle/libs.versions.toml`) pflegen, keine
  neuen DI-Frameworks — manuelle DI über `AppContainer`. UI-Strings nach
  `res/values/strings.xml` (deutsch). Unit-Tests für ViewModels Pflicht.
- **Docker-Images**: Immer native ARM-Runner (`ubuntu-24.04-arm`), niemals
  QEMU — siehe Workflow-Muster in `.github/workflows/backend.yml`.
- **SQLite**: eine Schreibverbindung (`SetMaxOpenConns(1)`), WAL-Modus wird
  in `internal/db` gesetzt. Deployment-Strategie bleibt `Recreate`.
- **Keine Secrets committen.** Der MCP-Endpoint nutzt OAuth (Rössing-ID,
  admin-Rolle) — es gibt bewusst kein statisches Token.

## Träger (Vereine und Gruppen)

Die Dorf-App verwaltet die **Allmende**, sie vermittelt nicht zwischen
Privatleuten. Jeder Ort und jede Aufgabe gehört einem **Träger**; Aufgaben
werden von Vereinen und Gruppen **kuratiert** eingestellt. **Keine
Parallelstrukturen zu bestehenden Vereinen aufbauen.**

- **Ein Träger = ein Zitadel-Projekt** mit genau zwei Rollen: `admin` und
  `mitglied`. **Keine Rollennamen mit Vereinspräfix.**
- Mitgliedschaften kommen **nicht aus dem Token**, sondern über einen
  **Dienst-Nutzer** aus der Zitadel-Management-API (`internal/mitglied`),
  kurz gepuffert. Eine neue Mitgliedschaft wirkt damit sofort.
- Bei Zitadel-Ausfall gilt der letzte bekannte Stand: **lesen ja, schreiben
  nein** (`model.Zugriff.DarfVerwalten`).
- Alle Sichtbarkeits- und Rechtefragen werden an **genau einer Stelle**
  entschieden: `model.Zugriff` in `internal/model/traeger.go`. Keine zweite
  Prüfung danebenbauen — Handler fragen ihn.
- `nur_mitglieder` heißt wirklich nirgends: Listen, Karte, Historie,
  Rangliste (SQL-Filter!) und Push. Neue Ausgabewege müssen mitgefiltert
  werden.

## Identität (Rössing-ID / Zitadel)

- Issuer: `https://id.xn--rssing-wxa.de`, Projekt `dorf-app`
- Rollen: `admin` (Betreiber der Plattform: Träger zulassen, alles sehen),
  `member` (melden). Rollen kommen als Claim
  `urn:zitadel:iam:org:project:roles` im JWT-Access-Token. Träger-Rollen
  stehen dort bewusst NICHT — siehe oben.
- Client-IDs: Android-App `385941807986376899` (nativ, PKCE),
  Web-Verwaltung `385942875872952515` (User-Agent, PKCE ohne Secret; der
  Code-Tausch passiert serverseitig, Redirect-URI ist `{PUBLIC_URL}/admin/`),
  Claude-MCP-Connector `385946294599876803` (Web, PKCE, kein Secret; wird via DCR-Endpoint /oauth/register an claude.ai ausgegeben).
