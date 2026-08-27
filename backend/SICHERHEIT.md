# Sicherheit des Dorf-App-Backends

Stand: 12.08.2026. Dieser Bericht fasst das Sicherheitsreview des Backends
zusammen: was geprüft wurde, was gefunden wurde und was daraus geworden ist.
Jede Schwäche hat zuerst einen Test bekommen, der sie belegt, danach den Fix.

## Kurzfassung

Die Grundstatik war in Ordnung: echte OIDC-Prüfung gegen die Rössing-ID, keine
Token im Browser, ausschließlich parametrisiertes SQL, `html/template` mit
automatischem Escaping, Admin-Rolle als harte Bedingung für Verwaltung und MCP.
Gefunden wurden vor allem Härtungslücken — fehlende Grenzen (Größe, Rate,
Textlängen), zu gesprächige Fehlermeldungen und fehlende Schutz-Kopfzeilen.

## Gefundene Schwächen und ihr Status

| # | Fund | Schwere | Status | Test |
|---|------|---------|--------|------|
| 1 | `AUTH_AUDIENCE` wurde stillschweigend ignoriert (`SkipClientIDCheck`, Parameter verworfen): Jedes Token derselben Rössing-ID galt hier, auch das einer fremden Anwendung. | mittel | behoben — Audience wird geprüft, und die Liste ist im OIDC-Modus jetzt Pflicht: ohne sie verweigert der Server den Start, statt still ungeprüft zu laufen. Produktion trägt die Client-IDs von Android-App, Web-Admin und MCP. | `internal/auth/auth_test.go: TestOIDCVerifierPrueftAudience`, `cmd/server/main_test.go: TestServerVerweigertOIDCOhneAudience` |
| 2 | Technische Fehler gingen im Klartext nach außen: `{"error":"sql: database is closed"}`, in der Verwaltung `Fehler: …`. Das verrät Dateipfade, Treiber und SQL-Interna. | mittel | behoben — nach außen nur noch eine allgemeine Meldung, Ursache ausschließlich im Log. | `internal/api/validierung_test.go: TestInterneFehlerBleibenInnen`, `internal/admin/sicherheit_test.go: TestFehlerseiteOhneInterna` |
| 3 | Keine Größenbegrenzung für Anfragen: `json.NewDecoder(r.Body)` überall, auch am unauthentifizierten `POST /oauth/register`. Ein einziger Aufruf konnte den 128-MiB-Pod fluten. | mittel | behoben — global 1 MiB (`MAX_BODY_BYTES`), zusätzlich 16 KiB direkt in der Client-Registrierung. | `internal/httpx/security_test.go: TestBodyBegrenzung`, `internal/mcp/register_test.go: TestRegistrierungBegrenztKoerper` |
| 4 | Eingabeprüfung war für `NaN`/`Inf` blind: Vergleiche mit `NaN` sind immer falsch, `lat < -90 \|\| lat > 90` also wirkungslos. Über das Formular genügte die Eingabe „NaN"; der Wert landete in der Datenbank und zerlegte Karte und Ampel-Rechnung. | mittel | behoben — `NaN`/`Inf` und absurde Größen werden abgewiesen. | `internal/api/validierung_test.go: TestPlaceInputLehntUnzahlenAb`, `TestTaskInputLehntUnzahlenAb` |
| 5 | Freie Texte (Name, Beschreibung, Titel, Notiz und Melder einer Erledigung) ohne Längenbegrenzung. | niedrig | behoben — höchstens 500 Zeichen. | `internal/api/validierung_test.go: TestPlaceInputBegrenztTextlaenge`, `TestMeldungBegrenztTextlaenge` |
| 6 | Cookie-Signaturen ohne Zweckbindung: Sitzungs-, Flow- und Flash-Cookie wurden mit demselben Schlüssel und ohne Kontext signiert; ein Cookie war in der Rolle eines anderen gültig. | niedrig | behoben — der Cookie-Name geht in die HMAC ein. | `internal/admin/sicherheit_test.go: TestCookieSignaturIstZweckgebunden` |
| 7 | Kein Rate-Limiting: `/mcp`, `/oauth/register` und alle Schreibpfade waren beliebig oft aufrufbar. | mittel | behoben — Token-Bucket je Nutzer/IP, `429` mit `Retry-After`. Die Eimer-Tabelle ist gedeckelt, weil der Schlüssel ein frei erfindbares Bearer-Token enthält. | `internal/httpx/ratelimit_test.go`, u.a. `TestRateLimitDeckeltDieAnzahlEimer` |
| 8 | Keine Sicherheits-Kopfzeilen: keine CSP, kein `nosniff`, kein Klickjacking-Schutz. | mittel | behoben — strenge CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, HSTS hinter TLS. | `internal/httpx/security_test.go` |
| 9 | Kein Panic-Schutz: Ein Programmierfehler in einem Handler riss den ganzen Server mit — inklusive der einen SQLite-Schreibverbindung. | mittel | behoben — Recovery-Middleware, Antwort ohne Panic-Text. | `internal/httpx/security_test.go: TestRecoverFaengtPanic` |
| 10 | `http.ListenAndServe` ohne jede Zeitschranke, kein Signal-Handling: langsame Verbindungen banden Ressourcen unbegrenzt, ein Rollout brach laufende Schreibvorgänge hart ab. | mittel | behoben — `http.Server` mit Lese-/Schreib-/Idle-Timeouts und geordnetem Herunterfahren. | — (Betriebsverhalten, im E2E abgedeckt) |
| 11 | MCP-Discovery: `http.Get` ohne Timeout, und ein einmaliger Fehler wurde per `sync.Once` bis zum Neustart festgeschrieben (der MCP-Login blieb dauerhaft tot). | niedrig | behoben — 15-s-Timeout, Fehler werden nicht mehr zwischengespeichert. | — |
| 12 | Cross-Site-Request-Forgery war allein durch `SameSite=Lax` abgedeckt. | niedrig | gehärtet — zusätzlich Herkunftsprüfung (`Origin`) für alle schreibenden Zugriffe unter `/admin`. | `internal/httpx/security_test.go: TestHerkunftsPruefung` |
| 13 | Keine Sicherung der Datenbank. Kein Angriff, aber das größte reale Verlustrisiko. | mittel | behoben — täglicher `VACUUM INTO` ins PVC, 14 Kopien. | `internal/backup/backup_test.go` |
| 14 | `AUTH_MODE=insecure-dev` war auch mit öffentlicher https-URL startbar. | niedrig | behoben — der Server verweigert diese Kombination. Ohne ausdrückliche `PUBLIC_URL` steht im Dev-Modus ohnehin `http://localhost:<Port>`, damit lokale Läufe und der Android-E2E nicht in die Prüfung laufen. | `cmd/server/main_test.go` |

## Geprüft und in Ordnung

- **Die Test-Knöpfe unter `/dev`** (Uhr stellen, Vergabe anstoßen) sind kein
  neuer Angriffspunkt: Sie werden nur eingehängt, wenn `AUTH_MODE` auf
  `insecure-dev` steht. In der Produktion gibt es die Pfade nicht — sie
  antworten dort mit 404, weil nichts registriert ist, und nicht mit 403,
  weil eine Prüfung greift. Der Unterschied ist der Punkt: Eine bewachte
  Route wäre eine vergessene Prüfung davon entfernt, die Uhr des laufenden
  Dorfes zu verstellen und damit Fälligkeiten, Fristen und Sitzungsablauf.
  Die Sperre sitzt in `devmode.Register` selbst, nicht nur an der Aufrufstelle.
  Nachweis: `internal/devmode/devmode_test.go: TestNotMountedOutsideDevMode`
  und `cmd/server/main_test.go: TestTestEndpunkteNurImDevModus` — dort startet
  der echte Server einmal im Dev-Modus und einmal im Produktionspfad.
- **Rückdatierung von Erledigungen** ist von 14 auf **drei Tage** gesenkt
  (`model.MaxBackdate`); die Verwaltung darf weiterhin 14 Tage zurück
  (`model.MaxBackdateAdmin`). Gewertet wird eine Meldung nur, wenn die Aufgabe
  zu ihrem Zeitpunkt nicht frisch erledigt war — ein breites Zeitfenster ist
  deshalb ein Werkzeug, die Rangliste nachträglich umzuschreiben. Einen
  abweichenden Zeitpunkt darf ohnehin nur die Verwaltung setzen
  (`internal/api/completion.go`). Nachweis:
  `internal/api/spielschutz_test.go: TestBackdatingWindow`,
  `TestBackdatingLimit`, `TestBackdatingBleibtDerVerwaltungVorbehalten`.

- **Redirect-URI-Allowlist der Dynamic Client Registration (`POST /oauth/register`).**
  Der Schwerpunkt der Prüfung. Der Vergleich ist ein exakter Nachschlag in einer
  festen Liste — keine Präfixe, keine Muster, keine Normalisierung. Alle
  üblichen Umgehungen laufen ins Leere und sind jetzt als Test festgehalten:
  Sub-Domain (`claude.ai.boese.example`), eingebettete Zugangsdaten
  (`https://claude.ai@boese.example/…`), Pfad-Anhängsel und `../`, Query- und
  Fragment-Anhänge, abweichende Groß-/Kleinschreibung, `http` statt `https`,
  explizite Portangabe, schemenlose URL, `javascript:`/`data:`. Eine gemischte
  Liste aus gültiger und fremder URI wird komplett abgelehnt.
  Zweite Verteidigungslinie: Die zurückgegebene Client-ID ist ein fester,
  in Zitadel angelegter PKCE-Client; die dort hinterlegten Redirect-URIs
  entscheiden am Ende, wohin ein Code überhaupt gehen darf.
  (`internal/mcp/register_test.go`)
- **Sitzungs-Cookie der Verwaltung.** HMAC-SHA256-signiert, Vergleich in
  konstanter Zeit, `HttpOnly`, `SameSite=Lax`, `Secure` sobald die öffentliche
  URL https ist, `Path=/admin`, Ablauf nach 8 Stunden — server- wie
  clientseitig. Es enthält kein Access-Token. Nach der Anmeldung wird immer ein
  frisches Cookie gesetzt (keine Session-Fixation); der State-/PKCE-Cookie des
  Login-Flows wird im Callback gelöscht, State und Code-Verifier werden geprüft.
  Manipulierte, abgelaufene und fremd signierte Cookies wurden gegengeprüft.
  (`internal/admin/sicherheit_test.go`)
- **SQL.** Alle Werte gehen als Parameter in die Abfragen. Die wenigen
  zusammengesetzten Abfragen in `internal/db/stats.go` setzen ausschließlich
  eigene Konstanten und aus der Zeitzonendatenbank berechnete Offsets ein — kein
  Weg für Eingaben von außen. IDs laufen über `strconv.ParseInt` (64 Bit, kein
  Überlauf), `LIMIT` ist auf 200 gedeckelt, negative Werte fallen auf die
  Vorgabe zurück.
- **Autorisierung.** REST: Bearer-Token Pflicht, Schreibzugriffe auf Orte und
  Aufgaben nur mit `admin`; eine Meldung darf nur zurücknehmen, wer sie selbst
  abgegeben hat (oder ein Admin). MCP: `admin` für jeden Aufruf, ohne Token
  `401` samt `WWW-Authenticate`-Hinweis. Verwaltung: jede Seite hinter
  `requireAdmin`.
- **Ausgabe.** Alle Seiten laufen über `html/template` (kontextabhängiges
  Escaping); die Kartendaten werden mit `json.Marshal` erzeugt, das `<`, `>`
  und `&` maskiert und damit nicht aus dem Skript-Kontext ausbrechen kann.
- **Logs.** Protokolliert werden Methode, Pfad, Status und Dauer. Tokens,
  Cookies, Session-Inhalte und E-Mail-Adressen erscheinen nirgends; die
  Rate-Limit-Schlüssel sind gekürzte Hashes, nie das Geheimnis selbst.

## Bewusst offen gelassen

- **Audience-Prüfung ist in Produktion nicht eingeschaltet.** `AUTH_AUDIENCE`
  ist vorbereitet und getestet, aber nicht gesetzt: Die Android-App und die
  Verwaltung holen Tokens mit unterschiedlichen Audiences, ein voreiliges
  Einschalten würde Nutzer aussperren. Empfehlung: Werte in Zitadel prüfen,
  danach setzen.
- **Kein CSRF-Token.** `SameSite=Lax` plus Herkunftsprüfung deckt den
  realistischen Angriff ab; ein Token pro Formular wäre der nächste Schritt,
  wenn die Verwaltung einmal Cross-Site-Einbettungen braucht.
- **Zwei-Faktor, Passwort-Politik, Sperren nach Fehlversuchen** liegen bei der
  Rössing-ID (Zitadel), nicht hier.

## Betriebsseitige Härtung

- HTTP-Server mit Lese- (15 s), Kopfzeilen- (10 s), Schreib- (60 s) und
  Leerlauf-Zeitschranke (120 s), begrenzten Kopfzeilen (64 KiB) und
  geordnetem Herunterfahren auf `SIGTERM`/`SIGINT` (20 s Frist). Wichtig, weil
  genau eine SQLite-Schreibverbindung offen ist: laufende Schreibvorgänge
  dürfen zu Ende gehen, bevor die Datenbank schließt.
- Panic-Recovery je Anfrage.
- Strukturierte Logs (JSON, `LOG_FORMAT=text` für die Werkbank).
- Container läuft als `nonroot` aus einem `distroless`-Image ohne Shell.

## CSP im Zusammenspiel mit der Karte

Die Richtlinie ist streng — `default-src 'self'`, `script-src 'self'` ohne
`unsafe-inline`/`unsafe-eval`, `object-src 'none'`, `base-uri 'none'`,
`frame-ancestors 'none'`, `form-action 'self'`. Ausnahmen gibt es nur für
MapLibre: `worker-src`/`child-src blob:` (der Kartenrenderer startet seinen
Worker aus einem Blob) sowie `connect-src`/`img-src` für
`https://tiles.openfreemap.org` (Stil, Vektorkacheln, Schriften, Sprites).
Ein Test liest die Host-Angaben direkt aus `internal/admin/static/karte.js` und
schlägt an, sobald Karte und Richtlinie auseinanderlaufen
(`TestCSPPasstZumKartenSkript`).

## Profile: freiwillige Kontaktdaten (Stand 14.08.2026)

Mit der Profilverwaltung speichert das Backend erstmals Daten, die **nicht**
aus der Rössing-ID stammen und die für andere Angemeldete sichtbar werden
können. Das ist eine Änderung an der Datenlage, nicht nur am Code —
**Datenschutzerklärung und die Play-Datensicherheit müssen nachgezogen
werden** (siehe „Was nachzuziehen ist").

### Was gespeichert wird

Tabelle `profiles`, ein Datensatz je Person, Schlüssel ist die Zitadel-Kennung
(`user_sub`):

| Feld | Herkunft | Pflicht |
|---|---|---|
| `display_name` | vorbelegt aus dem Token der Rössing-ID, überschreibbar | nein |
| `nickname` | selbst eingetragen; ersetzt den Namen in Rangliste und Historie | nein |
| `phone` | selbst eingetragen | **freiwillig** |
| `email` | vorbelegt aus dem Token, überschreibbar | **freiwillig** |
| `note` | selbst eingetragen, kurzer Freitext („erreichbar abends") | **freiwillig** |
| `vis_*` | Sichtbarkeit je Feld: `dorf` oder `verwaltung` | — |
| `token_name` | Name aus der Rössing-ID, intern zur Zuordnung von Bestandsdaten | — |
| `updated_at` | Zeitstempel der letzten Änderung | — |

Der Datensatz entsteht beim ersten `GET /api/v1/me` aus den Angaben, die
ohnehin im Token stehen. Telefon und Notiz sind dann leer.

### Wer was sieht

- **Vorbelegung:** Anzeigename und Nickname `dorf`, Telefon, E-Mail und Notiz
  `verwaltung`. Kontaktdaten werden also **nie still veröffentlicht** — es
  braucht eine bewusste Entscheidung in der App bzw. der Verwaltung.
- `GET /api/v1/members` liefert Angemeldeten ausschließlich die auf `dorf`
  gestellten Felder. Nicht freigegebene Felder verlassen den Server nicht
  (nicht ausgegraut, nicht leer mitgeschickt — sie sind schlicht nicht in der
  Antwort). Belegt in
  `internal/api/profil_test.go: TestMitgliederSehenNurFreigegebenes`.
- Verwaltende sehen alles, aber gekennzeichnet: `adminView: true` und je
  Eintrag `restricted: [...]` mit den Feldern, die die Person **nicht**
  freigegeben hat. Die Verwaltungsoberfläche schreibt „nur Verwaltende"
  daneben.
- Wer weder Anzeigenamen noch Nickname freigibt, erscheint für gewöhnliche
  Mitglieder gar nicht in der Liste
  (`TestGanzZurueckhaltendeTauchenNichtAuf`).
- Nur das eigene Profil ist änderbar. `PUT /api/v1/me/profile` mit fremder
  Kennung im Rumpf antwortet `403` — auch für Admins
  (`TestFremdesProfilVerboten`).
- Eingaben werden geprüft: Längen (80/40/40/120/200 Zeichen), Telefonformat
  großzügig aber plausibel (mindestens fünf Ziffern, keine Buchstaben),
  E-Mail-Form, und **keine Steuerzeichen** — auch keine Zeilenumbrüche und
  Tabulatoren (`TestProfilValidierung`).

### Namen in Rangliste und Historie

Erledigungen speichern seit jeher den Namen, der beim Melden galt. Statt diese
Bestandsdaten anzufassen, wird der Name **beim Anzeigen** aufgelöst
(`model.NameResolver`):

1. Gibt es kein Profil zur Kennung → gespeicherter Name (Bestandsdaten
   funktionieren unverändert weiter).
2. Gehört der gespeicherte Name zu dieser Person — er entspricht dem Namen aus
   der Rössing-ID, dem Anzeigenamen oder dem Nickname → Profilname
   (Nickname, sonst Anzeigename).
3. Sonst bleibt der gespeicherte Name stehen. Das ist der Fall „Verwaltung
   trägt einen telefonisch gemeldeten Vollzug für jemand anderen ein": Diese
   Zeile gehört der genannten Person und darf nicht den Nickname der
   eintragenden Person bekommen (`TestNachtragUnterFremdemNamenBleibt`).

Die SQL-Gruppierung der Rangliste bleibt dabei unangetastet (`user_sub` +
gespeicherter Name); erst ganz zum Schluss, nach Auszeichnungen und der Suche
nach dem eigenen Eintrag, wird der Anzeigename ersetzt. Dadurch spaltet sich
keine Person in zwei Zeilen, wenn sie sich umbenennt.

### Migration

`CREATE TABLE IF NOT EXISTS profiles` — rein additiv. An `places`,
`care_tasks`, `completions` und `settings` ändert sich nichts, es werden keine
Daten umgeschrieben. Die laufende Produktionsdatenbank bekommt die Tabelle
beim nächsten Start einfach dazu; ein Rückschritt auf die vorige Version
funktioniert weiterhin (die alte Version ignoriert die Tabelle).

### Was nachzuziehen ist (nicht in diesem Repo)

1. **Datenschutzerklärung auf der Website.** Neu aufzunehmen: Kategorie
   „freiwillig angegebene Kontaktdaten (Telefonnummer, E-Mail-Adresse,
   Anzeigename, Nickname, kurze Notiz)"; Zweck: Nachbarschaftliche
   Erreichbarkeit innerhalb der Dorf-App; Rechtsgrundlage: Einwilligung
   (Art. 6 Abs. 1 lit. a DSGVO), erteilt durch das bewusste Umlegen des
   Sichtbarkeits-Schalters, jederzeit widerruflich durch Zurückstellen bzw.
   Leeren des Feldes; Empfänger: alle in der Dorf-App angemeldeten Personen
   (bei Freigabe „für alle Dorfbewohner") bzw. ausschließlich die
   Verwaltenden; Speicherdauer: bis zur Änderung oder Löschung durch die
   Person.
2. **Play-Datensicherheit (Data Safety).** Bisher wurde „keine Daten erhoben"
   bzw. nur die Anmeldung gemeldet. Neu zu deklarieren:
   - *Personal info → Name* (erhoben, geteilt: nein, optional: ja)
   - *Personal info → Email address* (erhoben, optional: ja)
   - *Personal info → Phone number* (erhoben, optional: ja)
   - *Personal info → Other info* (die freie Notiz, optional: ja)
   jeweils Zweck „App functionality", Übertragung verschlüsselt, Löschung auf
   Wunsch möglich, Daten sind **nicht** an Dritte weitergegeben (die
   Sichtbarkeit innerhalb der App ist keine Weitergabe an Dritte im Sinne der
   Erklärung, wohl aber in der Datenschutzerklärung zu benennen).
3. **Hinweis in der App selbst** ist bereits umgesetzt: Die Profilseite trägt
   den Hinweis „Das sehen andere" gut sichtbar über dem Formular, nicht im
   Kleingedruckten.

## Vergabe der Pflegeaufgaben (Stand 14.08.2026)

Wer sich zum Mithelfen anmeldet, wird von der App angesprochen, sobald an
„seinem" Ort etwas fällig wird. Datenschutzseitig entstehen dabei drei neue
Tabellen (`care_signups`, `care_assignments`, `care_notifications`) mit
folgenden Angaben:

| Angabe | Inhalt | freiwillig? |
|---|---|---|
| Anmeldung | Kennung, Ort, ggf. Aufgabenart, Zeitpunkt | **ja**, jederzeit widerrufbar |
| Vorgang | Aufgabe, Stand, wer zugesagt hat und bis wann | folgt aus der Zusage |
| Zustellung | Kennung, Anlass, Zeitpunkt, ob gelesen | folgt aus der Anmeldung |

### Wer was sieht

- **Angemeldet wird immer nur die eigene Person.** `POST
  /api/v1/places/{id}/signup` mit fremder Kennung im Rumpf antwortet `403` —
  auch für Admins (`internal/api/vergabe_test.go: TestKeineFremdeAnmeldung`).
  Verwaltende können also nachsehen, wer mithilft, aber niemanden heimlich
  eintragen.
- **Namen der Angemeldeten sehen nur Verwaltende.** `GET
  /api/v1/places/{id}/signups` ist admin-pflichtig
  (`TestAnmeldungenSehenNurAdmins`); alle anderen bekommen in der Orts-Liste
  ausschließlich die Anzahl (`signupCount`) und den eigenen Zustand
  (`signedUp`).
- **Sichtbar ist eine Zusage.** Wer eine Aufgabe übernimmt, erscheint für alle
  Angemeldeten mit Namen und Frist („übernommen von … bis …") — das ist der
  Zweck der Sache: Sonst gießen zwei Leute denselben Kasten. Der Name folgt
  denselben Regeln wie in der Rangliste (`model.NameResolver`).
- **Benachrichtigungen sind privat.** `GET /api/v1/me/notifications` liefert
  ausschließlich die eigenen; fremde lassen sich weder lesen noch bestätigen
  (`TestBenachrichtigungAbrufenUndBestaetigen`).
- **Ruhezeiten** (Vorgabe 21–7 Uhr Ortszeit) sind kein Datenschutz, aber
  Rücksicht: Zwischen diesen Zeiten wird nichts zugestellt.

### Was daraus für Datenschutzerklärung und Play folgt

1. **Datenschutzerklärung** (`store/datenschutz.md`) muss um die
   Aufgaben-Vergabe ergänzt werden: dass eine freiwillige Anmeldung
   gespeichert wird, dass daraus Anfragen entstehen (Zeitpunkt, Ort, Aufgabe,
   ob gelesen), dass eine Zusage mit Namen und Frist für die übrigen
   Angemeldeten sichtbar ist, und dass Abmelden jederzeit möglich ist. Die
   Aufbewahrung folgt der Aufgabe: Vorgänge und Zustellungen hängen an der
   Pflegeaufgabe und verschwinden mit ihr (`ON DELETE CASCADE`).
2. **Play-Datensicherheit** braucht keine neue Kategorie: Es kommen keine
   weiteren personenbezogenen Felder hinzu (nur Kennung, Ort, Zeitpunkte).
   Die bestehende Angabe „App activity → Other actions" deckt die Anmeldung
   und die Zusagen ab; als Zweck weiterhin „App functionality". Für **diese
   Angaben** gibt es keine Weitergabe an Dritte — für die Gerätekennung schon,
   siehe Punkt 3.
3. **Push ist da — und damit ist Google beteiligt.** Seit `0.1.7` gibt es
   neben der Abrufliste den Versand über Firebase Cloud Messaging
   (`internal/push`, Zusteller an `vergabe.Zusteller`). Das ändert die Lage in
   drei Punkten:
   - **Neue Angabe: Gerätekennung.** Die App holt sich von Firebase eine
     Kennung ihrer Installation und meldet sie an `POST /api/v1/me/devices`
     (Tabelle `push_devices`: Kennung, Person, Plattform, Zeitstempel). Die
     Kennung ist ein Schlüssel zum Gerät — sie wird deshalb in **keiner**
     Antwort ausgeliefert, gehört immer nur einer Person (eindeutiger Index)
     und lässt sich nur von ihr selbst abmelden (`DELETE …/me/devices`).
     Was Google als ungültig meldet (`UNREGISTERED`, `INVALID_ARGUMENT`),
     löscht der Server von sich aus.
   - **Google sieht mit.** An Firebase gehen Gerätekennung, Titel und Text der
     Meldung (also Ortsname und Aufgabe) sowie die Kennungen von Ort, Aufgabe
     und Vorgang. Das ist eine **Weitergabe an ein anderes Unternehmen** im
     Sinne der Play-Datensicherheit und in der Datenschutzerklärung zu nennen.
     Namen anderer Personen stehen nie in einer Push-Nachricht.
   - **Freiwillig ist die Anzeige — die Kennung noch nicht.** Ohne
     `FCM_CREDENTIALS_FILE` im Cluster wird gar nicht gepusht, und wer die
     Erlaubnis verweigert (ab Android 13 eine eigene Frage mit Begründung),
     bekommt nichts angezeigt; die App holt ihre Anfragen dann wie bisher
     selbst ab. Diese Rückfallebene läuft immer mit — sie ist kein Notbehelf,
     sondern der verlässliche Weg.
     **Offener Punkt:** `ui/HomeScreen.kt` meldet die Kennung in einem
     `LaunchedEffect(Unit)` beim Betreten der Startseite an, **bevor** die
     Erlaubnis geklärt ist. Damit entsteht die Kennung auch bei jemandem, der
     ablehnt, und Nachrichten für dieses Gerät laufen über Google, ohne dass
     sie jemand zu sehen bekommt. Solange das so ist, trägt die Einwilligung
     als Rechtsgrundlage nicht sauber und die Play-Angabe „optional" ist
     angreifbar (siehe `store/data-safety.md`, „Abweichung im Code").
     Richtig wäre: erst anmelden, wenn die Erlaubnis vorliegt, und beim
     Widerruf `DELETE /api/v1/me/devices` schicken.

## Ideen-Sammlung (Stand 14.08.2026)

`POST /api/v1/ideen` ist der **einzige** Endpunkt ohne Anmeldung. Das ist
Absicht: Das Formular steht auf der öffentlichen Website, und wer noch keine
App hat, soll trotzdem sagen können, was sie können soll. Damit daraus kein
offenes Scheunentor wird:

| Riegel | Wirkung | Test |
|---|---|---|
| Eigene Zugriffsgrenze | `IDEEN_BURST` (5) und `IDEEN_PRO_STUNDE` (5) je Aufrufer — bei anonymen Einreichungen also je IP. `429` mit `Retry-After`; im Browser samt der Seite, auf der der getippte Text noch steht. | `TestIdeenRateLimit`, `TestIdeeRateLimitVerliertDenTextNicht` |
| Honigtopf | Verstecktes Feld `webseite`; ist es gefüllt, gibt es eine freundliche `201`, gespeichert wird nichts. | `TestIdeeHonigtopfWirdVerworfen` |
| Mindestzeit | `gestartet` (Unix-Millisekunden) — unter 3 Sekunden zwischen Aufruf und Absenden wird still verworfen. Fehlt das Feld (kein JavaScript), wird nicht geprüft. | `TestIdeeMindestzeitZwischenAufrufUndAbsenden` |
| Weiterleitung | `redirect` nur auf freigegebene Ursprünge (`IDEEN_ZIELE`, Vorgabe `https://xn--rssing-wxa.de`). Relative Pfade, fremde Hosts, `javascript:` und Benutzerangaben im Host werden mit `400` abgewiesen. | `TestIdeenWeiterleitungNurAufErlaubteZiele` |
| Eingabeprüfung | Wunsch 5–2000 Zeichen, Name ≤ 100, E-Mail ≤ 200 und plausibel, keine Steuerzeichen (im Wunsch sind Zeilenumbrüche erlaubt). | `TestIdeeValidierung` |
| Rechte | Lesen, Ändern und Löschen nur mit `admin`; Mitglieder bekommen `403`, ohne Token `401`. | `TestIdeenVerwaltungNurFuerAdmins` |

Kein Captcha und kein Fremddienst: Beides würde Daten an Dritte tragen und
Menschen mit Vorlesehilfe ausbremsen.

Die interne Notiz der Verwaltung verlässt die Verwaltung nicht — die Antwort
des öffentlichen Eingangs blendet sie aus.

**Anzeige und Export.** Eingereichter Text ist Fremdtext und wird nie als
Markup wirksam: Die Verwaltung rendert ausschließlich über `html/template`
(`TestIdeeMitMarkupWirdEscaped`), die Fehlerseite des Eingangs ebenso
(`TestIdeeFehlerseiteEscaptDenText`). Der CSV-Export unter
`/admin/ideen/export.csv` (admin-pflichtig) entschärft Zellen, die mit `=`,
`+`, `-` oder `@` beginnen — sonst führte ein eingereichter Wunsch beim
Öffnen im Tabellenprogramm eine Formel aus
(`TestIdeenExportEntschaerftFormeln`).

**Realistische Missbrauchsmuster** sind als eigene Reihe festgehalten
(`internal/api/ideen_missbrauch_test.go`): Dauerfeuer über 40 Versuche —
auch mit ungültigen Eingaben, die keinen Freifahrtschein geben —, überlange
Texte, Steuerzeichen und Kopfzeilen-Einschleusung in der E-Mail, leere
Formulare sowie ein Dutzend Varianten gefälschter Weiterleitungsziele
(Portangabe, Rückschrägstrich, eingebettete Zugangsdaten, `data:`,
vorangestellter Leerraum, Sub-Domain-Anhang).

**Migration**: `CREATE TABLE IF NOT EXISTS ideen` — rein additiv, an
bestehenden Tabellen ändert sich nichts, ein Rückschritt auf die vorige
Version funktioniert weiterhin.

### Was daraus für die Datenschutzerklärung folgt

Die Datenschutzerklärung (Website `/app/datenschutz` und
`store/datenschutz.md`) braucht einen eigenen Abschnitt: gespeichert werden
**Wunschtext (Pflicht) sowie Name und E-Mail (beides freiwillig)**, dazu
Eingangszeitpunkt, der Weg (Website oder App) und — nur bei Einreichung aus
der angemeldeten App — die Kennung der Rössing-ID. Zweck: die Weiterentwicklung
der Dorf-App und Rückfragen zum Wunsch. Rechtsgrundlage: Einwilligung
(Art. 6 Abs. 1 lit. a DSGVO) durch das Absenden des Formulars; ohne Name und
E-Mail ist die Einreichung anonym möglich. Empfänger: ausschließlich die
Verwaltenden des Dorfentwicklungskreises — Ideen werden **nicht**
veröffentlicht. Speicherdauer: bis der Wunsch erledigt oder verworfen ist,
längstens bis zum Widerruf. Widerruf und Löschung formlos per E-Mail an den
Dorfentwicklungskreis. **Es wird keine IP-Adresse gespeichert**; die
Zugriffsgrenze hält sie nur flüchtig im Arbeitsspeicher.

Für die Play-Datensicherheit ändert sich nichts Neues gegenüber den
Profildaten: *Name* und *Email address* sind bereits als optional erhoben
deklariert, dazu kommt *Personal info → Other info* für den Wunschtext.

## Träger und Befähigungen (Stand 16.08.2026)

**Was neu verarbeitet wird.** Zwei Dinge: die **Vereinszugehörigkeit** einer
Person (welche Rolle sie in welchem Zitadel-Projekt hat) und ihre
**Befähigungen** (erteilte Einweisungen samt Antrag, Begründung, Entscheidung
und interner Notiz des Trägers).

**Woher die Zugehörigkeit kommt.** Nicht aus dem Token, sondern über einen
**Dienst-Nutzer** aus der Zitadel-Management-API. Zweck: Nur so wirkt eine
neue Mitgliedschaft sofort, ohne dass die App für jeden neuen Verein einen
weiteren Scope lernen und sich jedes Gerät neu anmelden muss. Die Auskunft
wird **nur im Arbeitsspeicher** zwischengespeichert (Vorgabe 45 Sekunden,
`ZITADEL_ROLLEN_TTL`) und **nirgends in die Datenbank geschrieben** — der
Bestand bleibt bei Zitadel, wo er hingehört. In Logs landen weder
Mitgliedschaften noch das Dienst-Token.

**Rechte des Dienst-Nutzers.** Er braucht ausschließlich **Lesezugriff** auf
Rollenzuweisungen (`ORG_USER_GRANT_VIEWER` bzw. „Org User Manager" nur lesend).
Er darf ausdrücklich keine Nutzer anlegen, ändern oder Rollen vergeben. Sein
Schlüssel liegt als Datei (im Cluster ein SealedSecret), das Access-Token wird
kurzlebig selbst erzeugt — **kein statisches Token in der Konfiguration.**

**Was ein Ausfall bedeutet.** Fällt Zitadel aus, gilt der letzte bekannte
Stand aus dem Zwischenspeicher, gekennzeichnet als „veraltet". Damit wird
weiter **gelesen**, aber nicht mehr **geschrieben** (`503` statt `403`, mit
Erklärung). Begründung: Ein zu lange gültiger Lesezugriff ist heilbar, eine
Änderung, die jemand nach seinem Austritt vornimmt, nicht. Ist gar nichts
bekannt, gibt es **keine** Mitgliedschaften — man sieht dann nur, was ohnehin
öffentlich ist. Ein Ausfall macht die App also vorsichtiger, nie großzügiger.
Die globale Betreiber-Rolle steckt im Token und bleibt handlungsfähig, damit
im Ernstfall jemand eingreifen kann.

**Interne Aufgaben.** `nur_mitglieder` ist als harte Grenze umgesetzt und
nicht als Anzeige-Einstellung: Die Aufgabe fehlt in der Ortsliste, ihre
Historie und eine Meldung darauf ergeben **404** (nicht 403 — die bloße
Existenz soll nicht durchsickern), sie zählt für Außenstehende nicht in der
Rangliste (gefiltert bereits in SQL, damit nicht die Gesamtsumme verrät, was
die Zeilen verschweigen) und die Vergabe bietet sie niemandem außerhalb an.
Ein Ort, dessen sämtliche Aufgaben intern sind, verschwindet mit — eine leere
Nadel auf der Karte wäre schon ein Hinweis. Ohne gesicherte
Mitgliedschafts-Auskunft wird eine interne Aufgabe **von sich aus an
niemanden** verteilt: Ein Push ist nicht zurückzuholen.

**Befähigungen als personenbezogene Daten.** Antrag, Begründung, Entscheidung
und Notiz stehen in der Datenbank und sind für die **Verwaltenden des
jeweiligen Trägers** sichtbar — nicht für andere Träger und nicht für andere
Mitglieder. Rechtsgrundlage: berechtigtes Interesse an der sicheren Nutzung
von Geräten (Art. 6 Abs. 1 lit. f DSGVO); ohne Nachweis der Einweisung darf
niemand mit der Motorsense losgeschickt werden. Speicherdauer: solange die
Person dem Träger angehört. Für die App-Nutzung ändert sich an der
Play-Datensicherheit nichts — es kommen keine neuen Kategorien hinzu.

## Kontolöschung (Stand 26.08.2026)

`DELETE /api/v1/me` (`internal/api/konto.go`, `internal/db/konto.go`) löscht
das **eigene** Konto — angestoßen aus der App heraus, nicht per E-Mail an den
Dorfentwicklungskreis. Apples Richtlinie 5.1.1 (v) verlangt genau das von
jeder App mit Konto, Art. 17 DSGVO ohnehin. Eine fremde Kennung im Rumpf
ergibt **403**, auch für Admins: Fremde Konten löscht die Verwaltung, nicht
die Selbstbedienung.

**Gelöscht** werden: das Profil, die Gerätekennungen (der Push hört damit
sofort auf), die Helfer-Eintragungen, die Zustellungen (Anfragen und
Hinweise) und die Befähigungsanträge samt Begründung und interner Notiz.
Laufende Zusagen werden **freigegeben** statt anonym festzuhängen — sonst
wartete ein Blumenkasten auf jemanden, den es nicht mehr gibt; die Vergabe
fragt beim nächsten Durchlauf die Übrigen.

**Anonymisiert** statt gelöscht werden zwei Dinge:

- die **Erledigungen** (`completions`): Kennung raus, Name ersetzt durch
  „Ehemaliges Mitglied";
- die **beendeten Zusagen**: dieselbe Behandlung, sie sind Historie.

Eingereichte **Ideen** bleiben stehen, weil die Verwaltung sie abarbeitet,
verlieren aber Kennung, Name und E-Mail.

**Warum die Erledigungen bleiben.** An ihnen hängen die Gesamtsummen des
Dorfes und die Historie der Orte („zuletzt gegossen am …"). Sie zu löschen
hieße, die Arbeit **anderer** zu verfälschen — eine gemeinsame Bilanz, aus
der jemand nachträglich Zeilen entfernt, stimmt nicht mehr. Sie unter Namen
zu behalten hieße umgekehrt, das Löschen zu verweigern. Also bleibt die
Zeile, der Name wird ersetzt und die Kennung entfernt: Die Rangliste bleibt
stimmig, und die Person verschwindet. Dass danach **alle** Gelöschten in der
Rangliste zu einer Zeile zusammenfallen (Gruppierung nach Kennung und Name),
ist gewollt: Ein Ersatzschlüssel je Person wäre wieder ein Personenbezug.

**Die Rössing-ID bleibt unangetastet.** Dieser Endpunkt löscht ausdrücklich
**nicht** das Konto in Zitadel. Es gehört der Rössing-ID, und dieselbe
Anmeldung dient auch anderen Anwendungen des Dorfes — die Dorf-App darf sie
nicht mit wegräumen. Wer auch sie loswerden will, wendet sich an die
Rössing-ID; die Antwort des Endpunkts sagt das im Klartext (`roessingId`,
`roessingIdUrl`), und die App zeigt es an, bevor sie sich abmeldet. Damit
glaubt niemand, mit dem einen Knopf sei beides erledigt.

Alles läuft in **einer Transaktion** — ein halb gelöschtes Konto wäre das
Schlechteste von beidem. Ein zweiter Aufruf ist unschädlich. Zurück kommt
eine `bilanz` mit den Zeilenzahlen; sie enthält keine personenbezogenen
Daten.

## Gerätekennung für iOS: APNs statt Firebase (Stand 26.08.2026)

Für iOS spricht das Backend **direkt mit Apple** (`internal/push/apns.go`),
nicht über Firebase. Kein Firebase-iOS-SDK: Das Backend baut sein
Anbietertoken ohnehin selbst (bei Apple ES256 mit einem `.p8`-Schlüssel), und
der iOS-Weg kommt damit **ganz ohne Google** aus. Die Weiche steht im Feld
`platform` von `POST /api/v1/me/devices` (`internal/api/geraete.go`,
`internal/push/weiche.go`): „ios" geht zu APNs, alles andere zu FCM.

- **Was gespeichert wird**: derselbe Satz wie auf Android (`push_devices`:
  Kennung, Person, Plattform, Zeitstempel) — nur ist die Kennung hier der
  **rohe APNs-Token als Hex-Zeichenkette**, den die App aus Apples
  `Data`-Objekt bildet. Sie kommt in **keiner** Antwort vor (auch nicht in
  der auf das Anmelden), gehört immer nur einer Person und lässt sich nur von
  ihr selbst abmelden.
- **Was an Apple geht**: Gerätekennung, Titel und Text der Meldung (also
  Ortsname und Aufgabe) sowie die Kennungen von Benachrichtigung, Vorgang,
  Aufgabe und Ort — dieselbe Nutzlast wie bei FCM. **Namen anderer Personen
  stehen nie in einer Push-Nachricht.** Empfänger ist damit Apple statt
  Google; eine Weitergabe an ein anderes Unternehmen bleibt es.
- **Die Kennung entsteht erst nach der Erlaubnis.** `ios/Dorf/Push/`
  fordert die Kennung nur an, wenn die Benachrichtigungserlaubnis wirklich
  vorliegt, und meldet sie beim Entzug wieder ab
  (`DELETE /api/v1/me/devices`). Das ist genau das, was oben unter „Push ist
  da" als **offener Punkt** für Android notiert ist — auf iOS ist es von
  Anfang an so gebaut.
- **Ungültige Kennungen wirft der Server weg.** Was Apple zurückweist
  (`BadDeviceToken`, `Unregistered`, `DeviceTokenNotForTopic` oder 410 Gone),
  wird gelöscht — ebenso eine Kennung, die
  gar keine Hex-Zeichenkette ist (dann war es eine FCM-Kennung, die als
  „ios" gemeldet wurde). Sonst prallte sie bei jeder Vergabe erneut ab.
- **Sandbox ist kein Testserver, sondern eine eigene Welt.** Eine Kennung aus
  einem Build, der direkt aus Xcode aufs Gerät ging, gilt *nur* dort;
  **TestFlight-Builds tragen dagegen das App-Store-Distributionsprofil mit
  `aps-environment: production` und bekommen Produktions-Kennungen.** Deshalb
  ist `APNS_UMGEBUNG` eine eigene Einstellung und keine Ableitung aus
  irgendetwas anderem — ein Tippfehler wird abgewiesen, statt still in der
  Produktion zu landen, wo jede fremde Kennung als „ungültig" zurückkäme und
  das Gerät weggeworfen würde.
- **Ohne `APNS_KEY_FILE` wird für iOS gar nicht gepusht.** Der Betrieb läuft
  unverändert weiter: Die App holt ihre Benachrichtigungen ohnehin ab. Push
  ist die Abkürzung, nicht der Weg.
- **Kein Test spricht mit Apple.** `api.push.apple.com` und
  `api.sandbox.push.apple.com` stehen in `.github/scripts/pruefe_lokale_tests.py`
  auf der Sperrliste; die Go-Tests prüfen die ES256-Signatur gegen einen
  lokalen Server, der sich verhält wie Apple.

## Fehlerberichte aus den Apps (Stand 27.08.2026)

`POST /api/v1/error-reports` ist nach dem Ideen-Eingang der **zweite**
Endpunkt ohne Anmeldepflicht. Das ist Absicht und keine Bequemlichkeit: Die
Ausfälle, auf die es ankommt, sind gerade die, bei denen das Anmelden selbst
klemmt — ein Bericht, der dann nicht hinausgeht, ist nichts wert. Kommt ein
gültiges Token mit, hängt der Bericht am Konto; die Person wird dabei **aus
dem Token** genommen und nie aus dem Rumpf, sonst ließe sie sich behaupten.

| Riegel | Wirkung | Test |
|---|---|---|
| Eigene Zugriffsgrenze | `FEHLERBERICHT_BURST` (10) und `FEHLERBERICHT_PRO_STUNDE` (10) je Aufrufer — bei anonymen Berichten also je IP. `429` mit `Retry-After`. `RATE_LIMIT=off` schaltet sie mit ab. | `TestErrorReportRateLimit` |
| Eingabeprüfung | Art und Plattform aus geschlossenen Listen, Meldung ≤ 500, technische Angaben ≤ 8000, Ergänzung ≤ 2000, Bereich und Versionen ≤ 100, keine Steuerzeichen (in mehrzeiligen Feldern sind Umbrüche erlaubt). | `TestErrorReportValidierung` |
| Person aus dem Token | `userSub`/`userName` im Rumpf werden ignoriert. | `TestErrorReportMitAnmeldungHaengtAmKonto` |
| Uhr des Geräts | Ein `occurredAt` aus der Zukunft wird auf „jetzt“ gezogen, ein unlesbares ebenso. Sonst schöbe eine falsch gestellte Uhr den Bericht ans Ende der Liste, wo ihn niemand sieht. | `TestErrorReportZeitstempelAusDerZukunftWirdEingefangen` |
| Rechte | Lesen, Einordnen und Löschen gibt es **nur** in der Web-Verwaltung (`/admin/fehlerberichte/`, admin-pflichtig) und über MCP. Eine REST-Route zum Lesen existiert bewusst nicht — die Verwaltung läuft nicht mehr in den Apps. | `TestFehlerberichteNurFuerAngemeldete`, `TestFehlerberichteNurFuerAdmins` |

Kein Honigtopf und keine Mindestzeit wie bei den Ideen: Dieses Formular tippt
niemand, es füllt die App.

**Anzeige.** Der Inhalt kommt von einem fremden Gerät und ist damit Fremdtext.
Die Verwaltung rendert ausschließlich über `html/template`
(`TestFehlerberichtMitMarkupWirdEscaped`).

**Migration**: `CREATE TABLE IF NOT EXISTS error_reports` — rein additiv, an
bestehenden Tabellen ändert sich nichts, ein Rückschritt auf die vorige
Version funktioniert weiterhin.

### Was in einem Bericht steht — und was nicht

Vollständig, abgelesen an `model.ErrorReport` und `api.ErrorReportInput`:

- **Art** (`crash`, `network`, `server`, `unexpected`),
- die **Meldung**, die die Person auf dem Schirm gelesen hat — derselbe Satz,
  kein zweiter für den Bericht,
- **technische Angaben**: HTTP-Status und Pfad der Anfrage, bei einem Absturz
  die Aufrufliste (höchstens 4000 Zeichen). **Nie** ein Anfrage- oder
  Antwortrumpf,
- der **Bereich** in Alltagssprache („Mithelfen“, „Mein Profil“) — aus dem
  Pfad abgeleitet, damit „api/v1/places“ nicht in der Liste steht,
- **Plattform**, **App-Version**, **Systemversion**, **Gerätebezeichnung**
  („iPhone14,3“, „Google Pixel 6“) — der Gerätetyp, ausdrücklich **keine**
  Gerätekennung,
- der **Zeitpunkt** auf dem Gerät und der Eingangszeitpunkt,
- die **freiwillige Ergänzung** der Person, meist leer,
- **wer**, sofern angemeldet: `user_sub` und `user_name` aus dem Token.

Nicht dabei: Protokolle, Bildschirmfotos, Standort, Gerätekennungen für Push
(FCM/APNs), Werbekennungen, Kontaktdaten aus dem Profil, IP-Adressen. Die
Zugriffsgrenze hält die IP nur flüchtig im Arbeitsspeicher.

**Nichts geht von selbst hinaus.** Beide Apps zeigen den Fehler an und warten
auf einen Knopfdruck. Ein Absturzmelder, der von allein sendet, wäre die eine
Stelle in dieser App, an der still etwas erhoben wird. Das Blatt
„Dazuschreiben“ führt vor dem Absenden Zeile für Zeile auf, was hinausgeht —
und zwar aus genau den Werten, aus denen die Anfrage gebaut wird, nicht aus
einer nebenher gepflegten Liste (`esGehtNurHinausWasImBlattSteht`,
`es geht nur hinaus was im Blatt steht`).

**Was eine Regel ist, wird nicht gemeldet.** 400, 401, 403, 409 und 429 sind
das Backend bei der Arbeit — zu kurzer Text, fehlende Rolle, jemand war
schneller, zu viel auf einmal. Sie stehen dort, wo sie hingehören, und
erzeugen keinen Bericht; sonst ersäufte die echte Störung im Rauschen.

### Abstürze

- **Android**: `Thread.setDefaultUncaughtExceptionHandler` bekommt praktisch
  jeden Kotlin- und Java-Fehler samt Aufrufliste. Der bisherige Handler wird
  danach trotzdem aufgerufen — ohne ihn stürbe der Prozess anders als sonst,
  und Play sähe den Absturz nicht mehr.
- **iOS**: `NSSetUncaughtExceptionHandler` fängt Objective-C-Ausnahmen.
  Swift-Laufzeitfehler (`fatalError`, Index außerhalb) tun das **nicht** —
  dafür steht eine Marke in den Defaults, solange die App im Vordergrund ist.
  Ist sie beim nächsten Start noch da, war das Ende kein ordentliches. Wer die
  App aus dem Umschalter wegwischt, geht vorher durch den Hintergrund und löst
  deshalb keinen Fehlalarm aus. **Bewusst keine Signal-Handler**: Schreiben
  aus einem Signal-Handler ist nicht signalsicher, und ein Absturzmelder, der
  beim Melden abstürzt, ist schlimmer als keiner.
- Ein Absturz wird **einmal** angeboten und danach weggeräumt.

### Was daraus für die Datenschutzerklärung folgt

Die Datenschutzerklärung (Website `/app/datenschutz` und
`store/datenschutz.md`) braucht einen eigenen Abschnitt: gespeichert werden
**Fehlermeldung, technische Angaben, Bereich, Plattform, App- und
Systemversion, Gerätebezeichnung und Zeitpunkt**, dazu eine **freiwillige**
Ergänzung und — nur bei angemeldeter Person — Kennung und Name aus der
Rössing-ID. Zweck: die Fehlersuche während des laufenden Tests.
Rechtsgrundlage: Einwilligung (Art. 6 Abs. 1 lit. a DSGVO) durch das Drücken
des Knopfes; ohne Anmeldung ist der Bericht anonym. Empfänger: ausschließlich
die Verwaltenden des Dorfentwicklungskreises — Berichte werden **nicht**
veröffentlicht. Speicherdauer: bis der Fehler behoben oder verworfen ist.
Löschung formlos per E-Mail. **Es wird keine IP-Adresse gespeichert.**

Für die Play-Datensicherheit und Apples „App Privacy“ ändert sich etwas: Die
Zeile *Absturzberichte / Diagnostics* steht dort nicht mehr auf „nein“ —
siehe `store/data-safety.md` und `store/ios-datenschutz.md`.

## Wenn doch etwas klemmt

Alle Riegel sind über die Umgebung steuerbar, ohne neues Image:
`RATE_LIMIT=off`, `MAX_BODY_BYTES=…`, `BACKUP=off`, `VERGABE=off` (dann wird
niemand mehr von selbst gefragt). Die CSP ist bewusst *nicht*
abschaltbar — sie gehört zum Auslieferungszustand der Seiten.
