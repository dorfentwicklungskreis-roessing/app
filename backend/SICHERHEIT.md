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
| 7 | Kein Rate-Limiting: `/mcp`, `/oauth/register` und alle Schreibpfade waren beliebig oft aufrufbar. | mittel | behoben — Token-Bucket je Nutzer/IP, `429` mit `Retry-After`. | `internal/httpx/ratelimit_test.go` |
| 8 | Keine Sicherheits-Kopfzeilen: keine CSP, kein `nosniff`, kein Klickjacking-Schutz. | mittel | behoben — strenge CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, HSTS hinter TLS. | `internal/httpx/security_test.go` |
| 9 | Kein Panic-Schutz: Ein Programmierfehler in einem Handler riss den ganzen Server mit — inklusive der einen SQLite-Schreibverbindung. | mittel | behoben — Recovery-Middleware, Antwort ohne Panic-Text. | `internal/httpx/security_test.go: TestRecoverFaengtPanic` |
| 10 | `http.ListenAndServe` ohne jede Zeitschranke, kein Signal-Handling: langsame Verbindungen banden Ressourcen unbegrenzt, ein Rollout brach laufende Schreibvorgänge hart ab. | mittel | behoben — `http.Server` mit Lese-/Schreib-/Idle-Timeouts und geordnetem Herunterfahren. | — (Betriebsverhalten, im E2E abgedeckt) |
| 11 | MCP-Discovery: `http.Get` ohne Timeout, und ein einmaliger Fehler wurde per `sync.Once` bis zum Neustart festgeschrieben (der MCP-Login blieb dauerhaft tot). | niedrig | behoben — 15-s-Timeout, Fehler werden nicht mehr zwischengespeichert. | — |
| 12 | Cross-Site-Request-Forgery war allein durch `SameSite=Lax` abgedeckt. | niedrig | gehärtet — zusätzlich Herkunftsprüfung (`Origin`) für alle schreibenden Zugriffe unter `/admin`. | `internal/httpx/security_test.go: TestHerkunftsPruefung` |
| 13 | Keine Sicherung der Datenbank. Kein Angriff, aber das größte reale Verlustrisiko. | mittel | behoben — täglicher `VACUUM INTO` ins PVC, 14 Kopien. | `internal/backup/backup_test.go` |
| 14 | `AUTH_MODE=insecure-dev` war auch mit öffentlicher https-URL startbar. | niedrig | behoben — der Server verweigert diese Kombination. | — (Startprüfung) |

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

## Wenn doch etwas klemmt

Alle Riegel sind über die Umgebung steuerbar, ohne neues Image:
`RATE_LIMIT=off`, `MAX_BODY_BYTES=…`, `BACKUP=off`. Die CSP ist bewusst *nicht*
abschaltbar — sie gehört zum Auslieferungszustand der Seiten.
