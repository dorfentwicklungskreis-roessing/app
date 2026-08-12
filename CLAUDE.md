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
- **Android**: Version Catalog (`gradle/libs.versions.toml`) pflegen, keine
  neuen DI-Frameworks — manuelle DI über `AppContainer`. UI-Strings nach
  `res/values/strings.xml` (deutsch). Unit-Tests für ViewModels Pflicht.
- **Docker-Images**: Immer native ARM-Runner (`ubuntu-24.04-arm`), niemals
  QEMU — siehe Workflow-Muster in `.github/workflows/backend.yml`.
- **SQLite**: eine Schreibverbindung (`SetMaxOpenConns(1)`), WAL-Modus wird
  in `internal/db` gesetzt. Deployment-Strategie bleibt `Recreate`.
- **Keine Secrets committen.** Der MCP-Endpoint nutzt OAuth (Rössing-ID,
  admin-Rolle) — es gibt bewusst kein statisches Token.

## Identität (Rössing-ID / Zitadel)

- Issuer: `https://id.xn--rssing-wxa.de`, Projekt `dorf-app`
- Rollen: `admin` (verwalten), `member` (melden). Rollen kommen als
  Claim `urn:zitadel:iam:org:project:roles` im JWT-Access-Token.
- Client-IDs: Android-App `385941807986376899` (nativ, PKCE),
  Web-Admin `385942875872952515` (User-Agent, PKCE),
  Claude-MCP-Connector `385946294599876803` (Web, PKCE, kein Secret).
