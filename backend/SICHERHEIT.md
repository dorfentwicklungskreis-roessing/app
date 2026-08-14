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
| 1 | `AUTH_AUDIENCE` wurde stillschweigend ignoriert (`SkipClientIDCheck`, Parameter verworfen): Jedes Token derselben Rössing-ID galt hier, auch das einer fremden Anwendung. | mittel | behoben — Audience wird geprüft, wenn konfiguriert; ohne Konfiguration unverändert offen (so läuft Produktion heute). | `internal/auth/auth_test.go: TestOIDCVerifierPrueftAudience` |
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
   und die Zusagen ab; als Zweck weiterhin „App functionality", keine
   Weitergabe an Dritte.
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
   - **Freiwillig bleibt es trotzdem.** Ohne Erlaubnis für Benachrichtigungen
     (ab Android 13 eine eigene Frage mit Begründung) und ohne
     `FCM_CREDENTIALS_FILE` im Cluster wird schlicht nicht gepusht; die App
     holt ihre Anfragen dann wie bisher selbst ab. Diese Rückfallebene läuft
     immer mit — sie ist kein Notbehelf, sondern der verlässliche Weg.

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

## Wenn doch etwas klemmt

Alle Riegel sind über die Umgebung steuerbar, ohne neues Image:
`RATE_LIMIT=off`, `MAX_BODY_BYTES=…`, `BACKUP=off`, `VERGABE=off` (dann wird
niemand mehr von selbst gefragt). Die CSP ist bewusst *nicht*
abschaltbar — sie gehört zum Auslieferungszustand der Seiten.
