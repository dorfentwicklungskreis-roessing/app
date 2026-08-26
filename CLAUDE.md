# Claude-Konfiguration für die Dorf-App

**Immer auf Deutsch kommunizieren** (Antworten, PRs, Issues, Review-Notizen).
GitHub-Magic-Keywords (`fixes`, `closes` …) bleiben Englisch. Für den
Quelltext gilt etwas anderes — siehe „Sprache im Quelltext".

## Projekt

Monorepo: `android/` (Kotlin/Compose), `ios/` (Swift 6/SwiftUI),
`backend/` (Go, SQLite WAL, REST + MCP + Web-Admin), `deploy/` (Kustomize,
Flux deployt in den K3S-Cluster). Details: siehe `README.md`.

## Regeln

- **Sprache im Quelltext** (ganzes Repo, alle Plattformen): **Bezeichner und
  Kommentare englisch, sichtbare Texte deutsch.** Englisch sind also Typen,
  Eigenschaften, Funktionen, Parameter, Dateinamen, Testnamen, Test-Kennungen
  (`accessibilityIdentifier`, `testTag`, `data-testid`) und jeder Kommentar.
  Deutsch ist allein, was jemand in der App, im Browser oder in einer
  Meldung liest — samt der Ressourcendateien, in denen diese Texte stehen.
  Der begründende Ton der Kommentare bleibt; es ändert sich die Sprache,
  nicht die Haltung.
  **Der Bestand ist noch nicht umgestellt**: Große Teile von `ios/`,
  `android/` und `backend/` tragen weiter deutsche Bezeichner (`AppUmgebung`,
  `OrteModell`, `DorfApi.hole` …). Das flächendeckend zu ändern ist ein
  eigener Zug — nicht nebenbei in einer fachlichen Änderung, sonst wird jeder
  Zweig unlesbar groß und kollidiert mit den anderen. Neuer Quelltext hält
  sich an die Regel, angefasster Bestand bleibt, wie er heißt.
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
- **iOS**: Das Xcode-Projekt wird aus `ios/project.yml` **erzeugt**
  (`make projekt`, XcodeGen) und nie committet — eine `.pbxproj` ist nicht
  lesbar zu prüfen und kollidiert bei jedem zweiten Merge. Zum Backend führt
  **genau ein Weg**: `DorfApi`; neue Endpunkte kommen als
  `DorfApi+<Thema>.swift` dazu und benutzen dessen Transport-Helfer, das DTO
  gehört zu den übrigen in `Modelle.swift`. Ein zweiter Transport lässt
  Fristen, Kopfzeilen und Fehlerübersetzung auseinanderlaufen — genau das war
  schon einmal der Fall. Jeder Bereich wohnt unter
  `ios/Dorf/Bereiche/<Bereich>/` und fasst keinen fremden an.
  **Keine Fremdbibliothek außer MapLibre** — Netz, JSON, OIDC und Ablage
  macht die Standardbibliothek. Adressen und Kennungen stehen ausschließlich
  in den Build-Einstellungen (`project.yml` → `Info.plist` →
  `Konfiguration.swift`), nie im Quelltext, damit CI und Tests sie
  übersteuern können.
- **Tests laufen ausschließlich lokal.** Kein Test — auch kein E2E — darf
  einen entfernten Server anfassen, erst recht nicht die Produktion. Zitadel
  gehört zur CI-Umgebung (`backend/e2e/docker-compose.yml`), Terminfeed und
  Kartenstil kommen aus `android/e2e/fixtures/`. Wer eine neue Adresse
  braucht: den Dienst in der CI mitstarten, nicht nach draußen zeigen.
  `.github/workflows/lokale-tests.yml` prüft das bei jeder Änderung. Ausgenommen
  ist allein die **Auslieferung** (Play-Upload, Firebase-Verteilung,
  TestFlight-Upload, GHCR-Push) — eine ausgelieferte App muss auf die
  Produktion zeigen. E2E ohne Mocks bleibt Vorgabe — lokal heißt nicht gemockt.
  Für iOS-Tests (`ios/DorfTests/`) prüft die Wache zusätzlich **strukturell**,
  nicht nur nach Adressen: `Konfiguration.*`, ein `DorfApi(` ohne `basis:`
  sowie `URLSession.shared`/`URLSession.dorfSitzung` sind dort verboten — über
  sie greift die Produktions-Vorbelegung aus `ios/project.yml`, ohne dass eine
  Adresse in der Datei stünde. Ein Test setzt seine Basis selbst und fängt
  seine Sitzung über `protocolClasses` ab.
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
  iOS-App `387943892076527811` („Dorf-App iOS", nativ, PKCE, Rücksprung
  `de.roessing.app:/oauth2redirect`) — **muss in `AUTH_AUDIENCE` stehen**
  (`deploy/overlays/production/deployment.yaml`), sonst weist das Backend
  jedes Token dieser App ab,
  Web-Verwaltung `385942875872952515` (User-Agent, PKCE ohne Secret; der
  Code-Tausch passiert serverseitig, Redirect-URI ist `{PUBLIC_URL}/admin/`),
  Claude-MCP-Connector `385946294599876803` (Web, PKCE, kein Secret; wird via DCR-Endpoint /oauth/register an claude.ai ausgegeben).

## Entwicklungsumgebung (iOS)

Die iOS-App wird auf einer **headless macOS-VM** gebaut — dort hängt **kein
Bildschirm**. Wer hier arbeitet, sieht ausschließlich Terminalausgabe.

- **Keine grafischen Anwendungen starten.** `open -a Simulator`, `open
  Dorf.xcodeproj` oder `xed` bewirken nichts Sichtbares; das Fenster geht auf,
  aber niemand sieht es. Wer jemandem etwas zeigen will, muss es in Worte
  fassen.
- **Der Simulator wird ausschließlich über `xcrun simctl` bedient**: `boot`,
  `bootstatus <id> -b`, `install`, `launch`, `io <id> screenshot <datei>`.
  Screenshots sind zum Nachsehen für den, der sie erzeugt — sie ersetzen keine
  Beschreibung.
- **Der Simulator muss vor `xcodebuild test` wirklich gebootet sein.** Steht er
  auf `Shutdown`, stirbt der Testträger reproduzierbar mit „Early unexpected
  exit / signal kill", ohne dass am Code etwas falsch wäre. `xcrun simctl
  bootstatus <id> -b` vorweg räumt das aus.
- **Ohne Signatur startet im Simulator nichts**: `CODE_SIGNING_ALLOWED=NO`
  lässt dyld das eingebettete `MapLibre.framework` abweisen. Es genügt eine
  Ad-hoc-Signatur — in `ios/project.yml` als `CODE_SIGN_IDENTITY: "-"`
  hinterlegt, in der CI und im `Makefile` ebenso.
- **Ein echtes iPhone gibt es nicht.** Zum Bauen, Prüfen und für
  Store-Screenshots reicht der Simulator; TestFlight und der App-Review
  brauchen dagegen Hardware bei jemand anderem. Vor einer Einreichung gehört
  ein Durchlauf auf einem echten Gerät dazu.
