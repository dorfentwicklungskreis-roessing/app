# Dorf-App Rössing 🌻

Die App fürs Dorf: Login mit der **Rössing-ID**, danach eine Startseite mit
Bereichen. Der erste Bereich heißt **„Mithelfen"** — was gerade im Dorf
ansteht: Karte der Blumenkästen und Beete mit Ampel-Status (grün/gelb/rot),
Gieß- und Jätpläne, Erledigungen melden, Rangliste. Dazu **Mein Profil** und
**Dorfbewohner**. Weitere Bereiche kommen; das Gießen ist ausdrücklich nur der
Anfang.

## Aufbau (Monorepo)

| Verzeichnis | Inhalt |
|---|---|
| `android/` | Native Android-App (Kotlin, Jetpack Compose, Material 3, MapLibre) |
| `ios/` | Native iOS-App (Swift 6, SwiftUI, MapLibre). Xcode-Projekt aus `project.yml` |
| `backend/` | Go-Backend: REST-API, MCP-Server, Web-Admin. SQLite (WAL) |
| `deploy/`  | Kustomize-Overlay für den K3S-Cluster (Flux deployt) |
| `.github/workflows/` | CI: Tests, E2E auf Emulatoren, Multi-Arch-Images, Releases |

## Architektur

- **Identität**: Zitadel auf `id.xn--rssing-wxa.de` („Rössing-ID").
  - App-Login: OIDC Authorization Code + PKCE im System-Browser (AppAuth), mit Consent-Screen.
  - **Rollen müssen angefordert werden.** App und Web-Verwaltung fragen beim
    Login zusätzlich den Scope `urn:zitadel:iam:org:projects:roles` an
    („projects", Plural); Zitadel legt die Rollen daraufhin unter dem Claim
    `urn:zitadel:iam:org:project:roles` („project", Singular) ins Token, aus
    dem `internal/auth` sie liest. Fehlt der Scope, stellt Zitadel ein Token
    **ganz ohne Rollen** aus — dann ist niemand `admin`, und alles, was die
    Rolle verlangt (Orte und Aufgaben pflegen), antwortet mit 403. Genau das
    war in der App der Fall, bis `LOGIN_SCOPES` den Scope bekam.
    Achtung beim Ausrollen: Ein bereits angemeldetes Gerät behält seinen
    Token-Satz, auch über die Aktualisierung hinweg — **einmal abmelden und
    neu anmelden**, sonst fehlen die Rollen weiterhin.
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
    „Mithelfen" liegt unter `/admin/mithelfen/…` (der frühere Pfad
    `/admin/dorfpflege/…` leitet dauerhaft per 308 dorthin um)
  - SQLite im WAL-Modus auf einem PVC (`/data/dorfapp.sqlite`)
- **Träger**: Alles, was in der App steht, gehört einem **Verein oder einer
  Gruppe**. Die Dorf-App vermittelt ausdrücklich **nicht** zwischen
  Privatleuten, sondern verwaltet die **Allmende** — das, was dem Dorf
  gemeinsam gehört. Aufgaben entstehen deshalb nicht spontan von Einzelnen,
  sondern werden von Trägern **kuratiert** eingestellt; neben den bestehenden
  Vereinen entsteht keine Parallelstruktur.
  **Ein Träger = ein Zitadel-Projekt** mit genau zwei Rollen, `admin` und
  `mitglied` (ohne Vereinspräfix — sie sind im Projekt eindeutig).
  Ein Träger hat einen **Zulassungsstand** (`beantragt`, `zugelassen`,
  `gesperrt`; zulassen darf **nur der Plattform-Betreiber**, sonst ist er
  unsichtbar) und eine **Sichtbarkeit** (`offen` = steht im Verzeichnis,
  `geschlossen` = nur Mitglieder finden ihn). Erster Träger ist der
  **Dorfentwicklungskreis** als Platzhalter; ihm gehören die bestehenden
  Aufgaben („Blumengießen Unter den Eichen"), bis die **Dorfpflege** offiziell
  zugestimmt hat und ihre Orte übernimmt.
- **Mitgliedschaften kommen nicht aus dem Token.** Zitadel legt Rollen eines
  Projekts nur dann ins Token, wenn die App genau dieses Projekt als Empfänger
  anfordert (`urn:zitadel:iam:org:project:id:<id>:aud`) — für jeden neuen
  Verein hieße das App-Update und erneute Anmeldung auf jedem Gerät.
  Stattdessen fragt das Backend die Rollenzuweisungen mit einem eigenen
  **Dienst-Nutzer** (Machine User) über die Management-API ab
  (`POST /management/v1/users/grants/_search`, gefiltert nach `userId`) und
  puffert die Auskunft kurz (`ZITADEL_ROLLEN_TTL`, Vorgabe 45 s). Der gewollte
  Vorteil: Eine neue Mitgliedschaft wirkt **sofort**, ohne Ab- und Anmelden.
  Code: `backend/internal/mitglied`.
  **Fällt Zitadel aus**, gilt der letzte bekannte Stand aus dem
  Zwischenspeicher — als „veraltet" markiert. Damit wird weiter **gelesen**,
  aber nicht mehr **geschrieben**: Ein zu lange gültiger Lesezugriff ist
  heilbar, eine Änderung nach dem Austritt nicht. Ist gar nichts bekannt, gibt
  es keine Mitgliedschaften (man sieht nur Öffentliches). Die globale
  Betreiber-Rolle steckt im Token und bleibt davon unberührt — der Betreiber
  bleibt handlungsfähig. Kurz: Ein Ausfall macht die App vorsichtiger, nie
  großzügiger.
- **Sichtbarkeit einer Aufgabe**: `oeffentlich` oder `nur_mitglieder`. Eine
  **geschlossene Gruppe kann sehr wohl öffentlich ausschreiben** — die
  Sichtbarkeit des Trägers betrifft nur das Verzeichnis. Umgekehrt gilt für
  `nur_mitglieder` die schärfste Regel des Systems: Die Aufgabe erscheint
  außerhalb des Trägers **auf keinem Weg** — nicht in Listen, nicht auf der
  Karte, nicht in Historie oder Rangliste (dort wird schon in SQL gefiltert,
  damit nicht die Gesamtsumme verrät, was die Zeilen verschweigen) und nicht
  als Anfrage oder Push. Ein Ort, dessen sämtliche Aufgaben intern sind,
  verschwindet gleich mit: Eine leere Nadel auf der Karte wäre schon ein
  Hinweis. Entschieden wird das an genau einer Stelle,
  `model.Zugriff` (`backend/internal/model/traeger.go`).
- **Befähigungen** („Einweisung nötig"): Ein Träger definiert Befähigungen
  (z.B. „Motorsense", „Schlüssel Gerätehaus"); eine Aufgabe kann eine
  voraussetzen. Wer sie nicht hat, **kann nicht zusagen** — serverseitig
  durchgesetzt, nicht bloß in der Oberfläche ausgeblendet — und wird von der
  Vergabe erst gar nicht gefragt. Beantragt wird sie von der Person,
  freigegeben oder abgelehnt vom Träger-`admin`. Bewusst als Befähigung der
  **Person** modelliert und nicht je Aufgabe: Wer einmal eingewiesen ist, ist
  es überall — sonst müsste jede einzelne Wiese neu freigegeben werden.
- **Wer darf was (Träger)**: Orte, Aufgaben und Befähigungen eines Trägers
  pflegt ausschließlich dessen `admin`. Die bestehende globale `admin`-Rolle
  bleibt die **Betreiberrolle**: Träger zulassen und sperren, alles sehen.
- **Domänenmodell**: Orte (`blumenkasten`, `beet`, `sonstiges`) haben
  Pflegeaufgaben (`giessen` mit Litern, `jaeten`, `sonstiges`). Eine Aufgabe
  ist entweder **regelmäßig** — Intervall (→ gelb) und Rot-Schwelle — oder
  **einmalig** (`oneOff`) mit einem **Fälligkeitsdatum** (`dueDate`) statt
  eines Intervalls: „einmal zum Bahnhof fahren". Deren Ampel richtet sich
  nach dem Termin: gelb ab drei Tagen davor (`model.OneOffLeadTime`), rot,
  sobald er verstrichen ist; erledigt bleibt sie grün und wird nicht wieder
  fällig. Der globale **Hitzefaktor** (z.B. 0.5) beschleunigt nur
  Gieß-Aufgaben — auf einen Termin wirkt er nicht.
  Mit `removeWhenDone` verschwindet eine einmalige Aufgabe nach der Meldung
  von Karte und Liste. Gelöscht wird sie dabei **nicht**, sondern *abgeräumt*
  (`removed_at`): An ihr hängen die Erledigungen, und die zählen weiter für
  die Rangliste. Eine erledigte einmalige Aufgabe nimmt keine zweite Meldung
  mehr an (409).
- **Wer darf was**: Orte und Aufgaben anlegen, ändern, pausieren und löschen
  darf ausschließlich die Verwaltung (Projektrolle `admin`). Durchgesetzt
  wird das serverseitig aus dem Token — in REST (`adminOnly`), in der
  Web-Verwaltung (`requireAdmin`) und am MCP-Endpoint gleichermaßen; die App
  blendet den Bereich zusätzlich nur für Verwaltende ein. Alle anderen
  Angemeldeten melden Erledigungen, tragen sich als Helfer:innen ein, sagen
  zu und stehen in der Rangliste — daran ändert sich nichts.
  **In der App** gibt es dafür den Bereich **„Verwaltung"**: Orte und
  Aufgaben pflegen, während man davor steht. Der Standort kommt aus dem
  eigenen Gerät („Meinen Standort übernehmen") oder aus einem Tipp auf die
  Karte.
- **Pausieren und Löschen mit Ansage**: Wer eine Aufgabe gerade zugesagt hat,
  bekommt den Hinweis „nicht mehr nötig", sobald sie pausiert oder gelöscht
  wird — für einzelne Aufgaben wie für ganze Orte, in REST, Web-Verwaltung
  und MCP. Damit der Hinweis das Löschen überlebt, hängt
  `care_notifications` nicht mehr per Fremdschlüssel am Vergabe-Vorgang und
  führt Ort und Aufgabe zusätzlich im Klartext mit.
- **Rangliste**: `GET /api/v1/stats/leaderboard?period=woche|monat|saison|jahr|gesamt`
  (Standard `saison` = 1. März bis 31. Oktober, Grenzen in Ortszeit) zeigt je
  Person Anzahl und Liter, die Gesamtsummen des Dorfes, den eigenen Rang und
  schlichte Auszeichnungen (Gießkanne des Monats, Frühaufsteher, Retter,
  Ausdauer). Eine irrtümliche Meldung nimmt
  `DELETE /api/v1/completions/{id}` zurück — erlaubt dem Melder und Admins.
  In der App gibt es dazu den Reiter „Rangliste" im Bereich „Mithelfen"; gemeldet wird erst nach einer
  Rückfrage (Ort und Menge), damit ein Fehlklick nichts einträgt. In der
  Verwaltung liegt beides unter `/admin/mithelfen/rangliste` bzw. als
  eigene Bestätigungsseite in der Historie eines Ortes.
- **Profile**: Jede Person pflegt in der App unter „Mein Profil" ihren
  Anzeigenamen, einen Nickname für die Rangliste, Telefon, E-Mail und eine
  kurze Notiz — **je Feld mit eigenem Sichtbarkeits-Schalter** (`dorf` = alle
  Angemeldeten, `verwaltung` = nur Verwaltende). Vorbelegt sind Anzeigename
  und Nickname sichtbar, **Telefon, E-Mail und Notiz nicht**: Kontaktdaten
  werden nie still veröffentlicht. Anzeigename und E-Mail kommen aus der
  Rössing-ID und sind überschreibbar.
  `GET /api/v1/me` liefert das eigene Profil mit, `PUT /api/v1/me/profile`
  ändert es (eine fremde Kennung im Rumpf ergibt **403**, auch für Admins),
  `GET /api/v1/members` listet die Dorfbewohner mit **genau den freigegebenen
  Feldern** — Gesperrtes verlässt den Server nicht. Verwaltende sehen alles,
  erkennbar an `adminView: true` und `restricted: [...]` je Eintrag. Wer
  weder Anzeigenamen noch Nickname freigibt, taucht für Mitglieder gar nicht
  auf. In der App gibt es dazu „Dorfbewohner" mit antippbarer Rufnummer und
  E-Mail, in der Verwaltung den Bereich `/admin/dorfbewohner/`.
  **Namen in Rangliste und Historie** kommen jetzt aus dem Profil statt aus
  dem, was bei der Meldung eingefroren wurde: Gibt es kein Profil, gilt der
  gespeicherte Name (Bestandsdaten laufen unverändert weiter); gehört der
  gespeicherte Name zur Person (Name aus der Rössing-ID, Anzeigename oder
  Nickname), gilt der Profilname; sonst bleibt er stehen — so behält ein
  Nachtrag, den die Verwaltung unter fremdem Namen eingetragen hat, die
  genannte Person. Die SQL-Gruppierung bleibt unangetastet, ersetzt wird erst
  für die Anzeige. Datenschutz-Folgen: siehe `backend/SICHERHEIT.md`.
- **Push-Benachrichtigungen**: Neben der Abrufliste (`GET
  /api/v1/me/notifications`) verschickt der Server Anfragen und Hinweise über
  **Firebase Cloud Messaging** (`internal/push`, HTTP v1, Zugriffstoken aus
  dem Dienstkonto selbst erzeugt — keine Fremdbibliothek). Die App meldet ihre
  Gerätekennung mit `POST /api/v1/me/devices` an und beim Abmelden mit
  `DELETE /api/v1/me/devices` wieder ab; mehrere Geräte je Person sind
  vorgesehen. Die Kennung wird nie ausgeliefert, und was Google als ungültig
  meldet, wirft der Server weg. Ohne `FCM_CREDENTIALS_FILE` (Dienstkonto-
  Schlüssel, im Cluster als SealedSecret) wird nicht gepusht — der Betrieb
  läuft dann unverändert über die Abrufliste. In der App gibt es dafür zwei
  Kanäle („Anfragen", „Hinweise"), die Frage nach der Erlaubnis stellt sich
  erst, wenn jemand sich als Helfer:in eingetragen hat, und ein Tipp auf die
  Meldung führt direkt zur Aufgabe. Datenschutz-Folgen: siehe
  `backend/SICHERHEIT.md`.
- **Ideen-Sammlung** („Sag uns, was die App können soll"): Wünsche aus dem
  Dorf mit Name, E-Mail (beides freiwillig) und dem Wunsch selbst.
  `POST /api/v1/ideen` ist als einziger Endpunkt **ohne Anmeldung**
  erreichbar — die Website ist öffentlich, und wer noch keine App hat, soll
  trotzdem etwas sagen können. Er versteht JSON **und** klassische
  HTML-Formulare, damit das Formular auf der Website ohne JavaScript
  funktioniert; Browser werden danach auf eine Dankeseite weitergeleitet
  (`redirect`, nur auf **freigegebene Ursprünge** — `IDEEN_ZIELE`, Vorgabe
  `https://xn--rssing-wxa.de`), alle anderen bekommen **201** mit JSON.
  Eine abgewiesene Eingabe ergibt für Browser eine eigene Fehlerseite, auf
  der der getippte Text vollständig zurückkommt. Kommt ein gültiges Token
  mit (App), wird die Idee dem Konto zugeordnet (`quelle: "app"`,
  `userSub`), sonst gilt `quelle: "website"`.
  **Missbrauchsschutz ohne Captcha und ohne Fremddienst**: eine eigene,
  strenge Zugriffsgrenze je IP (`IDEEN_BURST`/`IDEEN_PRO_STUNDE`, Vorgabe
  5 am Stück und 5 pro Stunde), ein verstecktes Feld (`webseite`), das nur
  Skripte ausfüllen, und eine Mindestzeit von 3 Sekunden zwischen
  Formularaufruf und Absenden (`gestartet`, Unix-Millisekunden; fehlt das
  Feld, wird nicht geprüft). Was daran hängen bleibt, bekommt eine
  freundliche 201 und wird verworfen.
  Lesen und Ändern darf nur die Verwaltung: `GET /api/v1/ideen?status=…`,
  `PATCH /api/v1/ideen/{id}` (Stand `neu`/`gelesen`/`umgesetzt`/`abgelehnt`
  und interne Notiz), `DELETE /api/v1/ideen/{id}`. In der Verwaltung liegt
  der Bereich unter `/admin/ideen/` (Zähler „neu" auf der Bereichsübersicht,
  Statusfilter als echte Links, Löschen über eine eigene Bestätigungsseite)
  samt Export unter `/admin/ideen/export.csv` — Semikolon und BOM, damit
  deutsche Tabellenprogramme die Datei ohne Import-Dialog öffnen, und Zellen,
  die mit `=`, `+`, `-` oder `@` beginnen, werden entschärft. Aus Claude
  heraus gibt es `ideen_liste` (mit Überblick über den ganzen Bestand: gesamt,
  offen, je Stand, je Weg, neueste und älteste Einreichung) und
  `idee_status_setzen`.
  **In der App** führt eine eigene Kachel auf der Bereichs-Startseite
  („Idee vorschlagen — was soll die App noch können?") zum Formular: ein
  mehrzeiliges Wunschfeld, darunter Name und E-Mail, **aus dem Profil
  vorbelegt** und änderbar. Eine abgelehnte Einreichung zeigt die Begründung
  des Backends im Wortlaut und lässt den getippten Text stehen; nach dem
  Abschicken bleibt nur das Wunschfeld leer, damit die nächste Idee ohne
  Tipparbeit hineinpasst.
- **Veranstaltungen** („Was ist los in Rössing"): Die Termine kommen von der
  **Website** (`https://xn--rssing-wxa.de/events.json`) und werden **dort**
  gepflegt (`src/content/events/` im Repo `roessing.de`) — keine zweite
  Pflegestelle, kein eigener Bestand im Dorf-Backend, das nichts davon weiß.
  Die App holt die Datei ohne Anmeldung und **ohne Zugangstoken** (eigener
  HTTP-Client, die Website ist öffentlich), zeigt kommende Termine zuerst und
  siebt Vergangenes selbst noch einmal aus — die Datei entsteht beim Bauen der
  Website und altert zwischen zwei Bauläufen. Ganztägige Termine kommen ohne
  erfundene Uhrzeit aus; Zeitpunkte tragen ihren Offset (`+01:00`/`+02:00`)
  und werden in Ortszeit angezeigt. Verweist ein Termin per `url` auf eine
  **externe Primärquelle**, führt der Tipp dorthin statt auf rössing.de —
  dieselbe Regel wie auf der Website, damit Inhalte nicht doppelt erzählt
  werden. Lässt sich nichts laden, steht ein Hinweis über der (womöglich
  älteren) Liste statt einer leeren Seite. **Kein Push für Termine**: Eine
  Erinnerung wäre ein eigenes Thema mit eigener Einwilligung.
  Termine mit Koordinaten sind für die Dorfkarte vorbereitet; Koordinaten
  pflegt die Website an den Orten (`geo` in `src/data/locations/*.yaml`).
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

### iOS

```sh
cd ios
make projekt        # erzeugt Dorf.xcodeproj aus project.yml (XcodeGen)
make bauen          # Simulator-Build
make testen         # Unit-Tests (mit lokalen Adressen, nie gegen die Produktion)
make pruefen        # die Wache „Tests nur lokal" über den iOS-Teil
```

`Dorf.xcodeproj` wird **erzeugt und nicht committet** (`.gitignore`): Eine
`.pbxproj` ist nicht lesbar zu prüfen und erzeugt bei jedem zweiten Merge einen
Konflikt. Wer eine Datei hinzufügt, ruft danach `make projekt` auf. Gebraucht
werden XcodeGen (`brew install xcodegen`) und Xcode 26 (Swift 6 mit
MainActor-Vorbelegung).

Alle Adressen und Kennungen stehen als Build-Einstellungen in `ios/project.yml`
und sind über `xcodebuild API_BASE_URL=… ` übersteuerbar — dasselbe Verfahren
wie `-PapiBaseUrl` auf Android, damit kein Test gegen die Produktion läuft.

Die iOS-App hat einen **eigenen nativen PKCE-Client** in der Rössing-ID:
`387943892076527811` („Dorf-App iOS", Rücksprung
`de.roessing.app:/oauth2redirect`). Diese Client-ID muss in `AUTH_AUDIENCE` des
Backends stehen (`deploy/overlays/production/deployment.yaml`) — sonst weist das
Backend jedes Token der iOS-App ab.

Gebaut und getestet wird in der CI von `.github/workflows/ios.yml` auf einem
macOS-Runner, ohne Signierung (`CODE_SIGNING_ALLOWED=NO`): ein
Apple-Zertifikat gibt es dort noch nicht.

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

### Alle Tests laufen ausschließlich lokal

**Kein Test greift auf einen entfernten Server zu — erst recht nicht auf die
Produktion.** Das gilt ausnahmslos: Zitadel, Dorf-API, Terminfeed der Website,
Kartenkacheln und Firebase stehen im Testlauf allesamt auf dem Rechner, der
den Test ausführt.

Der Anlass ist nicht theoretisch: Der Android-Login-Test meldete sich früher an
der echten Rössing-ID an. Fiel der Server aus, wurde die CI rot, obwohl am Code
nichts falsch war; zwei gleichzeitige Läufe warfen sich gegenseitig aus der
Sitzung desselben Testkontos; und ein Test mit gültiger Anmeldung kann in der
Produktion Daten verändern.

Was die CI dafür selbst hochfährt (`.github/workflows/android.yml`):

| Dienst | Adresse im Test | Woher |
|---|---|---|
| Zitadel | `http://10.0.2.2:8123` | `backend/e2e/docker-compose.yml` mit `ZITADEL_EXTERNALDOMAIN=10.0.2.2` |
| Backend (Dev-Login) | `http://10.0.2.2:8099` | `go run ./cmd/server`, `AUTH_MODE=insecure-dev` |
| Backend (echtes OIDC) | `http://10.0.2.2:8098` | `go run ./cmd/server` gegen obiges Zitadel |
| Terminfeed, Kartenstil | `http://10.0.2.2:8097` | `android/e2e/fixtures/` über `python3 -m http.server` |

`10.0.2.2` ist die Adresse, unter der der Emulator den Host erreicht. Weil OIDC
verlangt, dass **alle** Beteiligten wörtlich denselben Aussteller sehen, legt
der Runner sich diese Adresse zusätzlich als Loopback an
(`ip addr add 10.0.2.2/32 dev lo`) — danach meint „10.0.2.2" im Emulator und
auf dem Host dieselbe Zitadel-Instanz, und das Backend prüft Tokens gegen genau
den Aussteller, den die App angefragt hat.

Projekt, Rollen, die native PKCE-App und das Testkonto legt
`android/e2e/zitadel-bootstrap.mjs` bei jedem Lauf neu an — reproduzierbar,
ohne Handarbeit und ohne GitHub-Secrets. Jeder Lauf bekommt sein eigenes Konto,
gleichzeitige Läufe stören sich also nicht mehr.

Für iOS gilt dieselbe Regel (`.github/workflows/ios.yml`): Jeder
`xcodebuild`-Aufruf übersteuert **alle** Adressen der App auf den Runner selbst.
Dort horcht in diesem Job niemand — ein Zugriff nach draußen fällt damit auf,
statt still die Produktion zu treffen:

| Einstellung | Adresse im Test |
|---|---|
| `API_BASE_URL` | `http://127.0.0.1:8099` |
| `WEBSITE_BASE_URL` | `http://127.0.0.1:8097` |
| `OIDC_ISSUER` | `http://127.0.0.1:8123` |
| `MAP_STYLE_URL` | `http://127.0.0.1:8097/map-style.json` |

Fehlt eine davon, greift die Produktions-Vorbelegung aus `ios/project.yml` —
genau so ist der Fehler auf Android ursprünglich entstanden. Die Prüfung unten
achtet deshalb auf jede einzelne.

Damit das so bleibt, prüft `.github/workflows/lokale-tests.yml` bei **jeder**
Änderung, ob in Testquellen oder Test-CI wieder eine entfernte Adresse steht:

```sh
python3 .github/scripts/pruefe_lokale_tests.py            # Prüfung
python3 .github/scripts/pruefe_lokale_tests.py --selbsttest  # prüft die Prüfung
```

Die Auslieferungs-Workflows sind davon ausgenommen — Play-Upload, Firebase App
Distribution und der GHCR-Push müssen selbstverständlich nach außen.

### Konfiguration des Backends (Env)

| Variable | Bedeutung |
|---|---|
| `LISTEN_ADDR` | Standard `:8080` |
| `DB_PATH` | Standard `/data/dorfapp.sqlite` |
| `AUTH_ISSUER` | Rössing-ID, Standard `https://id.xn--rssing-wxa.de` |
| `AUTH_MODE` | `oidc` (Standard) oder `insecure-dev` (nur lokal/E2E) |
| `PUBLIC_URL` | öffentliche Basis-URL; daraus entsteht die OIDC-Redirect-URI `{PUBLIC_URL}/admin/`. Ohne Angabe die Produktions-URL — mit `AUTH_MODE=insecure-dev` stattdessen `http://localhost:<Port aus LISTEN_ADDR>` |
| `ADMIN_CLIENT_ID` | Client-ID der Verwaltung (leer = nur Startseite) |
| `SESSION_KEY` | Schlüssel für die signierten Session-Cookies; leer = zufällig beim Start (Sessions überleben dann keinen Neustart) |
| `MCP_CLIENT_ID` | PKCE-Client für die MCP-Anbindung |
| `SEED` | `1` → Beispieldaten anlegen, falls die DB leer ist |
| `AUTH_AUDIENCE` | kommaseparierte Liste erlaubter Token-Empfänger — die Client-IDs der Anwendungen, die dieses Backend nutzen dürfen. **Im OIDC-Modus Pflicht:** ohne sie prüft das Backend nur Aussteller und Signatur, und ein Token für ein anderes Projekt derselben Rössing-ID käme durch. Der Server verweigert deshalb ohne diesen Wert den Start |
| `ZITADEL_SERVICE_USER_KEY_FILE` | JSON-Schlüssel des Dienst-Nutzers, mit dem die Träger-Mitgliedschaften über die Management-API abgefragt werden. **Fehlt er, gibt es keine Träger-Rollen**: Dann verwaltet nur der Betreiber, und alle anderen sehen die öffentlichen Aufgaben. Der Betrieb läuft dabei unverändert weiter |
| `ZITADEL_ROLLEN_TTL` | Wie lange eine Mitgliedschafts-Auskunft als frisch gilt (Vorgabe `45s`) |
| `RATE_LIMIT` | `off` schaltet die Zugriffsbegrenzung ab |
| `RATE_LIMIT_BURST` / `RATE_LIMIT_PER_MINUTE` | Eimergröße (60) und Nachfüllrate pro Minute (120) |
| `MAX_BODY_BYTES` | Obergrenze je Anfrage, Standard 1 MiB |
| `BACKUP` | `off` schaltet die Sicherung ab |
| `BACKUP_DIR` | Zielverzeichnis, Standard `<Verzeichnis von DB_PATH>/backups` |
| `BACKUP_KEEP` / `BACKUP_INTERVAL` | Anzahl Kopien (14) und Abstand (`24h`) |
| `LOG_FORMAT` | `json` (Standard) oder `text` |

### Sicherung der Datenbank

Der Server sichert die SQLite-Datei selbst: einmal täglich per
`VACUUM INTO` nach `/data/backups/dorfapp-<Zeitstempel>.sqlite`, 14 Kopien
werden aufbewahrt, ältere gelöscht. Geprüft wird alle 15 Minuten, gesichert
nur, wenn die jüngste Kopie älter als das Intervall ist — dadurch übersteht
der Plan Neustarts und Rollouts, ohne bei jedem Start eine Kopie zu schreiben.

**Warum im Server und nicht als Kubernetes-`CronJob`:** Die Datenbank liegt
auf einem ReadWriteOnce-PVC, das Deployment fährt mit `Recreate` und genau
einem Pod, der die einzige Schreibverbindung hält. Ein CronJob-Pod müsste
dasselbe RWO-Volume einhängen — das gelingt nur auf demselben Knoten und
ließe einen zweiten Prozess in eine WAL-Datenbank greifen, die einem anderen
gehört. Der eingebaute Zeitplan nutzt dieselbe Verbindung; SQLite garantiert
damit eine in sich stimmige Kopie ohne Betriebsunterbrechung.

Eine Sicherung zurückspielen (Pod ist gestoppt, `Recreate` sorgt dafür beim
nächsten Rollout ohnehin):

```sh
kubectl -n dorf-app scale deploy/dorf-app-backend --replicas=0
# im Pod bzw. auf dem Volume:
cp /data/backups/dorfapp-<Zeitstempel>.sqlite /data/dorfapp.sqlite
rm -f /data/dorfapp.sqlite-wal /data/dorfapp.sqlite-shm
kubectl -n dorf-app scale deploy/dorf-app-backend --replicas=1
```

Sicherheitsreview, Härtung und die Gründe dahinter: `backend/SICHERHEIT.md`.

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
`aufgabe_anlegen/aendern/loeschen` (regelmäßig mit `intervalDays`, einmalig
mit `oneOff` + `dueDate`, dazu `removeWhenDone`), `erledigung_melden`,
`erledigung_zuruecknehmen`, `rangliste`, `hitzefaktor_setzen`.

## Deployment

Push auf `main` mit Backend-Änderungen → GitHub Actions baut ein
Multi-Arch-Image (amd64 + arm64, native Runner), bumpt den Tag in
`deploy/overlays/production/kustomization.yaml`, Flux rollt aus
(GitRepository/Kustomization im `server-config`-Repo).

## Releases (Android)

**Getaggt und veröffentlicht wird von Hand.** Eine Automatik dafür gibt es
bewusst nicht: Ein Tag-Push aus einem Workflow löst wegen der
GitHub-Token-Sperre keine weiteren Ereignisse aus — sauber ginge das nur mit
einer eigenen GitHub-App. Solange die fehlt, wäre die Automatik Overkill.

### Der Weg

1. **Stand prüfen** — Android- und Backend-Workflow müssen für den Commit grün
   sein, der veröffentlicht wird:
   ```sh
   gh run list --limit 5
   ```
2. **Version hochzählen** in `android/app/build.gradle.kts`: `versionCode` um
   eins erhöhen, `versionName` auf die neue Version setzen.
3. **Änderungshinweis anlegen**, benannt nach dem neuen `versionCode`, in
   **beiden** Sprachen, höchstens 500 Zeichen:
   `store/metadata/android/{de-DE,en-US}/changelogs/<versionCode>.txt`
4. **Prüfen und committen**:
   ```sh
   python3 store/check_metadata.py
   ```
5. **Taggen und den Release-Workflow starten**:
   ```sh
   git tag v0.1.4 && git push origin v0.1.4
   # Der Tag-Push startet den Release-Workflow normalerweise selbst.
   # Passiert nach ~1 Minute nichts (kommt bei Pushes aus Workflows vor):
   gh workflow run release.yml --ref v0.1.4
   gh run watch "$(gh run list --workflow=release.yml --limit 1 \
     --json databaseId --jq '.[0].databaseId')"
   ```

Der Release-Workflow signiert APK und AAB, legt das GitHub-Release an, verteilt
über Firebase App Distribution an die Gruppe `tester` und lädt (nur mit
hinterlegtem `PLAY_SERVICE_ACCOUNT_JSON`) in den Play-Track „internal".

### Zwei Stolpersteine

* **Der Tag-Push startet den Workflow meistens selbst** — bei Tags, die von
  Hand gepusht werden, verlässlich. Nur wird der Tag aus einem
  Workflow heraus mit dem `GITHUB_TOKEN` gepusht, greift `push: tags` nicht —
  GitHub unterbindet so Endlosketten. Deshalb immer
  `gh workflow run release.yml --ref <tag>` hinterherschicken; von Hand mit
  eigenem Token gepusht startet der Workflow dagegen von selbst.
* **`versionCode` und Änderungshinweis müssen zusammenpassen.**
  `store/check_metadata.py` verlangt zu dem `versionCode` aus
  `build.gradle.kts` eine Datei `changelogs/<versionCode>.txt` **in jeder**
  Sprache — sonst wird der Store-Workflow rot und der Release-Lauf bricht vor
  dem Play-Upload ab. Play nimmt außerdem jeden `versionCode` nur ein einziges
  Mal an; er muss bei jedem Upload steigen.

> **Merke für das nächste Release:** `v0.1.3` wurde einmalig mit
> `versionCode 1000103` gebaut und über Firebase an die Tester verteilt.
> Android installiert keinen Build mit kleinerem `versionCode` über einen
> größeren — wer `0.1.3` auf dem Telefon hat, müsste vorher deinstallieren.
> Beim nächsten Release also entweder einen `versionCode` über `1000103`
> wählen oder die Tester einmal deinstallieren lassen.

Play-Store-Upload und Firebase-Verteilung aktivieren sich, sobald die Secrets
existieren (siehe `.github/workflows/release.yml`, Kopfkommentar).

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

Den Änderungshinweis zum jeweiligen `versionCode` schreibt man von Hand — siehe
„Releases (Android)" oben.
