# Die Mietplattform in den Apps

Entscheidungsvorlage. Stand: 26.08.2026.

Untersucht wurde `levino/mietplattform-roessing` (Klon vom 26.08.2026, letzter
Commit `d56fab7`) gegen den Bestand der Dorf-App (`docs/mieten`, auf
`origin/main`, `4f15336`). Alle Pfadangaben mit dem Präfix `mietplattform:`
beziehen sich auf das fremde Repo; alle anderen auf dieses hier. Am fremden
Repo wurde nichts geändert.

Der Betreiber hat währenddessen entschieden: **volle Integration in beide
Apps, die Webfassung bleibt eigenständig, Anmeldung überall mit der
Rössing-ID.** Diese Vorlage ist deshalb kein Vergleich mehr, sondern ein Plan.
Die verworfenen Wege stehen trotzdem drin — kurz, mit Begründung, damit sie
nicht in einem halben Jahr neu diskutiert werden.

---

## 1. Der Befund: Das meiste ist schon da

Die Ausgangsfrage lautete, ob und wie sich zwei getrennte Anmeldungen
zusammenführen lassen. Sie hat sich am Quelltext erledigt.

### 1.1 Die Mietplattform meldet bereits über die Rössing-ID an

Der eigene OIDC-Provider und die Wege über GitHub und Magic-Link sind **nicht
mehr die Anmeldung**. Seit `5dc0602` („ZITADEL als einzige Login-Methode")
gilt:

- `mietplattform:website/src/pages/login.astro` ist fünf Zeilen lang und
  leitet **jede** Anmeldung samt aller OAuth-Parameter nach `/auth/zitadel`
  weiter — mit dem Kommentar „it's the only login method".
- `mietplattform:server/src/http/auth.ts` kennt nur noch `/auth/zitadel`,
  `/auth/zitadel/callback` und `/auth/me`. Kein GitHub-, kein
  Magic-Link-Endpunkt.
- `mietplattform:server/src/index.ts:107-124` verdrahtet ausschließlich den
  Zitadel-Anbieter. `server/src/auth/github.ts` liegt noch im Repo, wird aber
  von nichts mehr importiert; `auth/magic-link.ts` ist nur noch der
  SMTP-Versender für Buchungsmails.
- Nachgeprüft am laufenden System: `GET https://mieten.xn--rssing-wxa.de/auth/zitadel`
  antwortet mit `302` nach
  `https://id.xn--rssing-wxa.de/oauth/v2/authorize?client_id=377276539064090727&…&code_challenge_method=S256`.

Das Zitadel-Projekt „Mietplattform" (`377276525071827047`) wird also benutzt,
und zwar mit einem Web-Client `377276539064090727` für den Login der Webseite.
Der Betreiber nennt die Mietplattform in diesem Repo bereits als eines der
Nachbarprojekte auf derselben Rössing-ID
(`backend/cmd/server/main_test.go:92`).

**Der `CLAUDE.md` der Mietplattform ist an dieser Stelle veraltet** — sie
beschreibt weiter „GitHub + Magic Link" und führt Dateien (`k8s/rbac.yaml`)
und verbotene Werkzeuge (`admin_list_users`) auf, die es nicht mehr gibt. Wer
sich nur auf sie verlässt, plant an der Wirklichkeit vorbei. Das sollte bei
Gelegenheit im fremden Repo nachgezogen werden.

### 1.2 Was die Mietplattform *nicht* hat: eine Schnittstelle für Apps

Die vollständige Liste der Nicht-Astro-Routen steht in
`mietplattform:server/src/index.ts:210-224`. Daraus, sortiert nach dem, was
eine App davon hätte:

**Öffentlich, ohne Anmeldung, JSON:**

| Route | Inhalt |
| --- | --- |
| `GET /bookings` | belegte Zeiträume je Gerät, ohne Personendaten |
| `GET /health` | Lebenszeichen |
| `/.well-known/*` | OAuth-Metadaten, JWKS |

Nachgeprüft: `GET /bookings` liefert heute vier bestätigte Buchungen als JSON,
mit `startDate`, `endDate`, `deviceId`, `status` — mehr nicht. Das ist
Absicht (`mietplattform:server/src/http/bookings.ts`, Abschnitt „Datenschutz"
in deren `CLAUDE.md`).

**Mit Bearer-Token, JSON:**

| Route | Inhalt |
| --- | --- |
| `GET /auth/me` | eigenes Profil |
| `GET /api/my-bookings` | eigene Buchungen, nach Bestätigung mit Abholadresse |
| `GET /api/owner-bookings` | Buchungen auf den eigenen Geräten |
| `POST /api/bookings/:id/{cancel,approve,reject}` | Entscheidungen |
| `POST /images/upload` | Bild-Upload |
| `GET/PUT/DELETE /sessions/:key` | Schlüssel-Wert-Ablage |

**Was fehlt** — und zwar genau das, womit ein Bereich in der App anfangen
müsste: **es gibt keine JSON-Liste der Geräte, kein Gerätedetail, keine
Suche, keine Verfügbarkeitsprüfung und kein Anlegen einer Buchung.**
Nachgeprüft: `/api/items`, `/items.json`, `/geraete.json` und `/api/geraete`
antworten alle mit `404`.

Diese Fähigkeiten existieren doppelt, aber beide Male für einen anderen
Abnehmer:

1. **Als MCP-Werkzeuge** (`mietplattform:server/src/mcp/tools/`), 22 Stück:
   `list_items`, `search_items`, `list_sets`, `get_item`, `create_item`,
   `update_item`, `list_my_items`, `attach_image_to_item`, `delete_image`,
   `check_availability`, `create_booking`, `get_my_bookings`,
   `get_owner_bookings`, `cancel_booking`, `approve_booking`,
   `reject_booking`, `block_period`, `unblock_period`, `list_my_blocks`,
   `get_profile`, `update_profile`, `request_lender_status`. Der Endpunkt
   `/mcp` verlangt **immer** ein Token
   (`mietplattform:server/src/mcp/server.ts`); nachgeprüft: ein Aufruf ohne
   Token liefert `401` mit `WWW-Authenticate: Bearer realm="mieten"`.
2. **Als In-Prozess-Zugriff für die Webseite**
   (`mietplattform:server/src/website-api.ts`): Astro liest über
   `Astro.locals.websiteApi` direkt aus der SQLite-Datei, ohne HTTP und ohne
   Token.

MCP ist für einen mobilen Client der falsche Weg: Es ist ein Protokoll für
Sprachmodelle mit Sitzungsverwaltung, SSE und Werkzeugbeschreibungen, deren
Wortlaut für die Kostenrechnung eines LLM optimiert ist. Eine App will `GET`
und JSON.

Der gute Teil: **das Muster gibt es schon.** Die Webseite ruft
`/api/my-bookings` und `/api/owner-bookings` bereits über HTTP mit
Bearer-Token auf (`mietplattform:website/src/pages/buchungen.astro:33-41`).
Die JSON-Schnittstelle wird also nicht erfunden, sie wird vervollständigt.

### 1.3 Wie die Mietplattform heute Token prüft

`mietplattform:server/src/auth/jwt.ts:78` — `makeTokenVerifier` nimmt **einen
statischen RSA-Schlüssel** aus der Umgebung und prüft damit RS256-Signaturen.
Kein JWKS, keine Prüfung von Aussteller oder Empfänger. Diese Token stellt die
Mietplattform selbst aus, über einen **eigenen** OAuth-Server
(`mietplattform:server/src/oauth/server.ts`): dynamische Client-Registrierung
nach RFC 7591, PKCE mit S256, öffentliche Clients erlaubt, Gültigkeit sieben
Tage (`:303`), **kein Refresh-Token**.

Das ist die eigentliche Bruchstelle. Der Login geht schon über die Rössing-ID,
aber danach tauscht die Mietplattform die fremde Identität gegen eine eigene
ein. Eine App, die ein Rössing-ID-Token hat, kommt damit heute an `/mcp` und
`/api/*` **nicht** vorbei.

### 1.4 Die Konten hängen an der E-Mail-Adresse

`mietplattform:server/src/db/queries/users.ts:68-98` — `findOrCreateUser`
sucht erst nach `id`, dann nach `email`, und legt nur an, wenn beides nichts
findet. Die lokale `users.id` ist eine zufällige UUID und hat mit dem Zitadel-
`sub` nichts zu tun; **die E-Mail-Adresse ist der Identitäts-Join**. Die
Migration `011-randomize-magic-link-ids.sql` sagt das wörtlich: „lassen die
Email als stabilen Identitäts-Join intakt".

Das ist für die Bestandskonten die entscheidende Eigenschaft, siehe
Arbeitspaket 5.

### 1.5 Betrieb: es ist nichts umzuziehen

Im k3s-Cluster läuft der Namensraum `mieten-roessing-de`
(`mietplattform:k8s/namespace.yaml`) mit:

- Deployment `mieten` + Init-Container für die Migrationen, Service, Ingress
  auf `mieten.xn--rssing-wxa.de`, TLS über cert-manager
  (`mietplattform:k8s/app.yaml`)
- Deployment `mieten-chat` (`mietplattform:k8s/chat.yaml`), an das der Ingress
  `/api/chat`, `/api/conversations` und `/api/usage` weiterreicht
- Deployment `mieten-imgproxy` (`mietplattform:k8s/imgproxy.yaml`), erreichbar
  unter `https://cdn.mieten.xn--rssing-wxa.de` (nur als Wert von
  `IMGPROXY_BASE_URL` in `k8s/app.yaml:73` belegt — **einen Ingress für diesen
  Namen enthält das Repo nicht**)
- SQLite auf einem PVC, Bilder in MinIO (`http://mieten-minio:9000`) — **auch
  für MinIO liegt kein Manifest im Repo**; der Dienst wird nur referenziert.
- Ausrollen läuft wie hier über Flux: die CI baut, schiebt nach GHCR und
  schreibt den Bild-Tag in `mietplattform:deploy/overlays/production/kustomization.yaml`
  (`mietplattform:.github/workflows/deploy.yml`).

**Durch die Integration ändert sich daran nichts.** Es kommen Umgebungswerte
hinzu (Aussteller und erlaubte Empfänger, siehe Arbeitspaket 1) — die gehören
ins bestehende `mieten-secrets` bzw. als Klartext in `k8s/app.yaml`, so wie
`AUTH_ISSUER`/`AUTH_AUDIENCE` hier in `deploy/overlays/production/deployment.yaml`
stehen.

**Nebenbefund, unabhängig von dieser Vorlage:** In der Produktion steht
`NODE_ENV=staging` (`mietplattform:k8s/app.yaml:80`), und
`mietplattform:server/src/index.ts:70-73` startet bei genau diesem Wert den
Seed-Lauf — gegen die persistente Datei. Dazu passt, dass die Live-Startseite
neben 18 Geräten mit UUID-Kennung neun mit Seed-Slugs zeigt
(`as-585-km-kreiselmaeher`, `rasenwalze`, `fallschutzmatten` …). Das ist kein
Hindernis für die Integration, sollte aber jemand ansehen, bevor an der
Datenbank gearbeitet wird.

---

## 2. Verworfene Wege

Der Vollständigkeit halber und damit sie nicht wiederkehren.

### a) Nur ein Verweis

Eine Kachel in beiden Apps, die `mieten.rössing.de` im In-App-Browser öffnet.
Aufwand: auf iOS eine Datei und zwei Zeilen Verdrahtung
(`ios/Dorf/Navigation/Ziel.swift`, `StartView.swift`), auf Android ein
Enum-Eintrag und vier Berührungspunkte in `ui/`. Ein Tag Arbeit, in beiden
Apps zusammen.

Das entspricht der Regel, nach der die Veranstaltungen gebaut sind
(`README.md:248-260`): Was woanders gepflegt wird, wird in der App nicht
nacherzählt. Und es hätte den Vorzug, dass die Webfassung die einzige
Oberfläche bleibt.

**Verworfen**, weil der Betreiber die volle Integration will. Der sachliche
Einwand dahinter: Der Verweis führt in eine Weboberfläche, die eine eigene
Anmeldung im Browser verlangt. Zwar dieselbe Rössing-ID, aber eine zweite
Anmeldehandlung — die Sitzung der App liegt im Schlüsselbund, nicht im
Browser. Genau der Bruch, den man nicht will.

### b) Nur lesend einbauen

Geräte und Verfügbarkeit in der App zeigen, gebucht wird im Web. Bräuchte von
Arbeitspaket 3 nur die lesenden Endpunkte und keine Identitätsarbeit — die
Geräteliste kann öffentlich sein, wie `/bookings` es heute ist.

**Verworfen**, weil es die Arbeit nicht spart, sondern verschiebt: Wer die
Liste sieht und tippen will, landet doch wieder im Browser, und die
Buchungsstrecke müsste später trotzdem gebaut werden. Als **Zwischenstand auf
dem Weg zu (c)** ist der Schnitt allerdings brauchbar, siehe Reihenfolge in
Abschnitt 4.

---

## 3. Der Plan

Sechs Arbeitspakete. „MP" = Repo `mietplattform-roessing`, „DA" = dieses Repo,
„Z" = Einstellungen in Zitadel (kein Quelltext).

### AP 1 — Die Mietplattform akzeptiert Rössing-ID-Token (MP + Z)

Ziel: `Authorization: Bearer <Zitadel-Access-Token>` wird an `/api/*` und
`/mcp` genauso akzeptiert wie ein selbst ausgestelltes Token.

Vorbild ist `backend/internal/auth/auth.go` dieses Repos. Es macht genau
dreierlei, und alle drei Schritte fehlen drüben:

1. **Aussteller und Schlüssel über Discovery**: `NewOIDCVerifier`
   (`auth.go:68-78`) holt sich `jwks_uri` aus
   `https://id.xn--rssing-wxa.de/.well-known/openid-configuration`. In der
   Mietplattform ist `jose` schon Abhängigkeit; `createRemoteJWKSet` genügt.
2. **Empfängerprüfung selbst gemacht** (`auth.go:75` mit
   `SkipClientIDCheck: true`, dann `audienceOK`, `:110-122`), gegen eine
   kommaseparierte Liste aus der Umgebung. Der Kommentar in
   `deploy/overlays/production/deployment.yaml:45-72` erklärt, warum die
   Prüfung Pflicht ist: ohne sie käme ein Token für ein beliebiges anderes
   Projekt derselben Rössing-ID durch.
3. **Claims lesen**: `sub`, `email`, `name`/`preferred_username`
   (`auth.go:80-84`, `:145-149`). Dass `email` im Access-Token steht, ist
   durch dieses Backend belegt — es liest sie dort seit Monaten.

Konkret in der Mietplattform:

- `server/src/auth/jwt.ts` behält `makeTokenVerifier` für die eigenen Token
  (die MCP-Clients in Claude Desktop hängen daran und sollen weiterlaufen) und
  bekommt einen zweiten Verifizierer daneben. Der Aufrufer probiert erst den
  Aussteller `id.xn--rssing-wxa.de`, dann den eigenen — beide liefern
  denselben `JwtPayload`, damit sich für die Werkzeuge und Routen nichts
  ändert.
- Der Zitadel-`sub` muss auf die lokale `users.id` abgebildet werden. Der
  bestehende `findOrCreateUser` tut das bereits über die E-Mail-Adresse
  (siehe 1.4) — es genügt, ihn mit den Claims des Token statt mit denen des
  Userinfo-Aufrufs zu füttern.
- Neue Umgebungswerte, benannt wie hier: `AUTH_ISSUER`, `AUTH_AUDIENCE`.

In Zitadel (Betreiberarbeit, kein Code):

- Im Projekt „Mietplattform" (`377276525071827047`) müssen die beiden
  App-Clients der Dorf-App als Empfänger zugelassen werden — Android
  `385941807986376899`, iOS `387943892076527811`.
- Die Clients müssen **JWT**-Access-Token ausstellen, keine undurchsichtigen;
  sonst gibt es nichts zu prüfen. Für die Dorf-App ist das offenbar der Fall,
  sonst könnte `backend/internal/auth` nicht arbeiten — für den Weg in die
  Mietplattform ist es dieselbe Einstellung an denselben Clients, also
  erledigt.
- Ob die Mietplattform eigene Rollen braucht (`lender`, `admin` liegen heute
  als Spalten in ihrer Datenbank, `mietplattform:server/migrations/010-user-cleanup.sql`),
  ist eine getrennte Frage. Empfehlung: **nicht** nach Zitadel verlagern. Der
  Vermieter-Status hängt an einem Profil-Check und einem Freigabe-Flow, der
  bewusst in der Mietplattform lebt; ihn in Rollen zu übersetzen, verteilt
  eine Regel auf zwei Systeme.

Umfang: überschaubar — eine Datei anfassen, eine dazu, Tests. Der Aufwand
steckt in den Bestandstests: `security.regression.test.ts`, `oauth/e2e.test.ts`
und `mcp/e2e.test.ts` prüfen die Token-Prüfung scharf und müssen den zweiten
Aussteller mit abdecken.

### AP 2 — Der zusätzliche Empfänger im Token der Apps (DA)

Damit ein Token der Dorf-App überhaupt für die Mietplattform gilt, muss ihr
Projekt in der `aud` stehen. Zitadel macht das über einen angeforderten Scope;
das Verfahren ist in diesem Repo schon beschrieben (`README.md:75-78`):
`urn:zitadel:iam:org:project:id:<id>:aud`.

Es ist **je App eine Zeile**:

- iOS: `ios/Dorf/Anmeldung/Anmeldung.swift:17`, die Liste `ANMELDE_SCOPES`
- Android: `android/app/src/main/java/de/roessing/app/auth/AuthManager.kt:50`,
  die Liste `LOGIN_SCOPES`

Dazu die Kennung selbst — nach der Regel in `CLAUDE.md:54-57` gehört sie **in
die Build-Einstellungen**, nicht in den Quelltext: iOS über `ios/project.yml`
→ `Info.plist` → `Konfiguration.swift`, Android über
`android/app/build.gradle.kts` → `BuildConfig`.

Ein Stolperstein, den dieses Repo schon einmal getreten hat und der in
`README.md:38-43` steht: **Ein bereits angemeldetes Gerät behält seinen
Token-Satz über die Aktualisierung hinweg.** Wer den neuen Scope nur einbaut,
bekommt auf allen Bestandsgeräten weiterhin Token ohne die neue Audience — und
damit `401` von der Mietplattform. Das muss die App auffangen: erkennt sie an
`/api/*` einen `401` wegen fehlender Audience, muss sie eine erneute Anmeldung
anstoßen, statt eine leere Liste zu zeigen.

### AP 3 — Die JSON-Schnittstelle der Mietplattform (MP)

Was eine App braucht, und was es dafür schon gibt:

| Zweck | fehlt / vorhanden | Quelle für die Umsetzung |
| --- | --- | --- |
| Geräte auflisten | fehlt | `websiteApi.fetchItems()` |
| Gerät im Detail (mit Bildern) | fehlt | `websiteApi.fetchItem()` |
| Suchen | fehlt | `toolSearchItems` (Hybrid-Suche) |
| Sets auflisten | fehlt | `websiteApi.fetchSets()` |
| Verfügbarkeit prüfen | fehlt | `checkOverlap` (berücksichtigt auch Vermieter-Sperren) |
| Belegte Zeiträume | **vorhanden** | `GET /bookings` |
| Buchen | fehlt | `toolCreateBooking` |
| Eigene Buchungen | **vorhanden** | `GET /api/my-bookings` |
| Buchungen auf eigenen Geräten | **vorhanden** | `GET /api/owner-bookings` |
| Stornieren / bestätigen / ablehnen | **vorhanden** | `POST /api/bookings/:id/…` |
| Profil lesen | **vorhanden** | `GET /auth/me` |
| Profil ändern | fehlt | `toolUpdateProfile` |
| Vermieter werden | fehlt | `toolRequestLenderStatus` |

**Neben den MCP-Werkzeugen führen oder aus denselben Funktionen speisen?**
Aus denselben. Die Werkzeuge sind heute schon dünne Hüllen um Funktionen, die
anderswo liegen — `toolCheckAvailability` ruft `checkOverlap`,
`toolCreateBooking` ruft `checkOverlap` + `createBooking` + den Mailversand.
Die HTTP-Route ruft dieselben Funktionen. Wer die Regel („belegt ist belegt",
„der Eigentümer bekommt eine Mail mit Entscheid-Link") in einer zweiten
Fassung in einen Handler schreibt, hat sie zweimal — und beim nächsten
Sonderfall einmal falsch. Das ist dieselbe Haltung wie hier bei
`model.Zugriff` und `gewertetSQL`: **eine Regel, eine Stelle.**

Praktisch heißt das: Die Geschäftslogik wandert aus
`mcp/tools/{items,bookings,blocks,profile}.ts` in `services/` (dort liegt mit
`services/booking-decision.ts` schon eines), und Werkzeug wie Route werden zu
Hüllen. Das ist die eigentliche Arbeit dieses Pakets — nicht das Schreiben der
Routen.

Zuschnitt der Routen: `/api/v1/…`, wie hier. Die Dinge, die heute schon unter
`/api/` liegen, sollten mit umziehen und ihre alten Pfade als Weiterleitung
behalten — die Webseite ruft sie auf (`buchungen.astro`).

Was **öffentlich** bleiben kann und sollte: Geräteliste, Detail, Suche, Sets,
Verfügbarkeit, belegte Zeiträume. Das steht heute schon ohne Anmeldung auf der
Webseite; es hinter ein Token zu legen, wäre eine neue Hürde ohne Gewinn — und
es erlaubt der App, den Bereich **vor** der Anmeldung zu zeigen. Ein Token
verlangen nur Buchen, eigene Buchungen, Stornieren und alles auf der
Vermieterseite. Die Datenschutz-Regeln der Mietplattform bleiben unangetastet:
keine Eigentümeradresse in einer öffentlichen Antwort, keine Personendaten in
`/bookings` (deren `CLAUDE.md`, Abschnitt „Datenschutz").

### AP 4 — Direkt oder über `app.rössing.de`? (Entscheidung: direkt)

**Empfehlung: Die Apps reden unmittelbar mit `mieten.xn--rssing-wxa.de`. Das
Go-Backend bekommt nichts.**

Dafür:

- Es gibt den Präzedenzfall im Haus. Die Veranstaltungen holen ihre Daten von
  einem **anderen** Server, mit einem **eigenen** kleinen Client, und das
  Backend „weiß nichts davon" (`README.md:248-251`,
  `ios/Dorf/Bereiche/Veranstaltungen/Veranstaltungen.swift:3-13`). Der
  Unterschied hier ist nur, dass ein Token mitgeht.
- Ein Weiterleiter im Go-Backend wäre eine dritte Stelle, an der das
  Domänenmodell des Verleihs auftaucht — mit eigenen DTOs, eigener
  Fehlerübersetzung und der Pflicht, jede Änderung drüben nachzuziehen. Genau
  das vermeidet dieses Projekt sonst überall.
- Er brächte auch keine Vereinfachung bei der Identität: Das Backend müsste
  das Token entweder durchreichen (dann ist die Audience-Frage aus AP 2
  unverändert) oder ein eigenes ausstellen (dann hätten wir die zwei
  Identitäten zurück, die wir gerade loswerden).

Dagegen, ehrlich benannt:

- Eine zweite Adresse in den Apps. Nach `CLAUDE.md:44-52` führt auf iOS „genau
  ein Weg zum Backend: `DorfApi`" — dieser Weg ist damit **nicht** gemeint,
  denn `mieten.…` ist nicht das Backend. Der Bereich bekommt einen eigenen
  Client, wie `Veranstaltungen.swift` einen hat. Die Adresse gehört nach
  `CLAUDE.md:54-57` in die Build-Einstellungen, nicht in den Quelltext.
- Zwei Dienste, die unabhängig ausfallen können. Der Bereich muss das
  aushalten, ohne den Rest der App mitzureißen — die Veranstaltungen machen
  das vor (Hinweis über der alten Liste statt leerer Seite).
- Ein zweiter Empfänger im Token (AP 2).

### AP 5 — Die Bestandskonten (MP + Z)

Der Betreiber hat das Verfahren vorgegeben: neue Zitadel-Konten im Zustand
*initial* anlegen (`POST /management/v1/users/human` ohne Passwort,
`USER_STATE_INITIAL`), die Leute setzen beim ersten Anmelden selbst ein
Passwort.

**Der gute Teil: eine Abbildung alter auf neue Kennungen ist nicht nötig.**
Weil `findOrCreateUser` über die **E-Mail-Adresse** joint (1.4) und die lokale
`users.id` beim Wiedererkennen unverändert bleibt, hängen Buchungen
(`bookings.user_id`), Geräte (`items.user_id`), Bilder, Freigabe-Token und
Vermieter-Status weiter am selben Datensatz. Der Vorgang ist schon einmal
gelaufen: Wer sich früher per GitHub oder Magic-Link angemeldet hat und heute
über die Rössing-ID kommt, findet mit gleicher E-Mail sein altes Konto vor.

**Der Knackpunkt ist deshalb nicht die Datenbank, sondern die Adresse.**
Meldet sich jemand bei der Rössing-ID mit einer *anderen* E-Mail an als der,
unter der er in der Mietplattform steht, legt `findOrCreateUser` einen
**zweiten, leeren** Datensatz an. Seine Geräte und Buchungen bleiben am
verwaisten alten hängen — er sieht sie nicht mehr, und die Eigentümer-Mails
zu seinen Geräten gehen weiter an die alte Adresse. Deshalb:

1. Bestand auslesen: `users` hat `id`, `email` (UNIQUE), `name`, `phone`,
   `address_street/zip/city`, `lender`, `admin`, `approved_at/by`,
   `lender_requested_at` (`mietplattform:server/src/db/queries/users.ts:19-33`).
2. Für **jede** dieser E-Mail-Adressen, die noch keine Rössing-ID hat, ein
   Zitadel-Konto im Zustand *initial* anlegen — mit **genau dieser** Adresse.
   Dann trifft der Join beim ersten Login automatisch.
3. Wer sein Konto nie aktiviert, verliert nichts: Der Datensatz bleibt stehen,
   seine Geräte bleiben in der Liste, Buchungsanfragen gehen weiter per E-Mail
   an ihn. Er kommt nur nicht mehr selbst hinein. **Für Eigentümer ist das
   ein echtes Problem** — Buchungen auf ihren Geräten laufen über
   Mail-Entscheid-Links, das funktioniert weiter, aber Geräte pflegen können
   sie nicht mehr. Diese Leute muss man einzeln ansprechen; es sind
   voraussichtlich wenige.
4. Laufende Buchungen sind unkritisch: Die Entscheid-Links in den Mails hängen
   an `booking_tokens`, nicht an einer Sitzung
   (`mietplattform:server/src/http/bookings.ts`). Ein Umstellungsfenster
   braucht es nicht.

**Wie viele Konten es sind, konnte ich nicht feststellen.** Der Bestand liegt
in der SQLite-Datei auf dem PVC im Cluster; das Repo enthält keine Zahl, und
die Mietplattform gibt bewusst nirgends eine Nutzerliste heraus (deren
`CLAUDE.md`, Abschnitt „Was Admins dürfen und was nicht"). Was sich von außen
sagen lässt: Die Startseite zeigt heute 27 Geräte, und jedes hat einen
Eigentümer (`items.userId`) — die Untergrenze ist also die Zahl der
verschiedenen Eigentümer, die Obergrenze unbekannt. Ob das eine Stunde oder
eine Woche Arbeit ist, hängt an dieser Zahl; sie sollte vor der Zusage einer
Frist ermittelt werden, im Cluster, mit einem Blick in `users`.

### AP 6 — Der Bereich in beiden Apps (DA)

Funktionsgleich, wie überall im Projekt.

**iOS**: ein Ordner `ios/Dorf/Bereiche/Verleih/` (Bezeichner englisch,
sichtbare Texte deutsch — `CLAUDE.md:15-22`), verdrahtet an zwei Stellen:
ein `case` in `ios/Dorf/Navigation/Ziel.swift` und eine `Bereichskachel` in
`ios/Dorf/Navigation/StartView.swift`. Zum Umfang: Der Bereich
„Veranstaltungen" umfasst 5 Dateien und 808 Zeilen und kann nur lesen. Ein
Bereich mit Kalender, Buchungsstrecke und Vermieteransicht wird deutlich
größer — eine belastbare Schätzung gebe ich hier nicht ab, dafür müsste der
Zuschnitt der Ansichten stehen.

**Android**: kein Bereichsordner, sondern ein Eintrag in
`enum class Bereich` (`ui/StartScreen.kt:49`) plus Kachel, Titel in der
TopAppBar (`ui/HomeScreen.kt:324-332`), Zweig im `when (bereich)`
(`:497-501`), ViewModel in der Factory (`MainActivity.kt:141-164`) und
Repository im `AppContainer` (`DorfApp.kt:56-57`). Mehr Berührungspunkte als
auf iOS, aber kein Neuland.

Für den Zuschnitt gilt, was in AP 3 steht: **Die Apps enthalten keine
Regeln.** Ob ein Zeitraum buchbar ist, wer bestätigen darf, wer Vermieter
werden kann — das entscheidet der Server und sagt es der App. Die App darf
einen Knopf ausgrauen, weil der Server das mitteilt, aber sie darf die
Bedingung nicht selbst kennen. Sonst laufen Web und App auseinander, und zwar
zuerst dort, wo es weh tut.

Zwei Dinge, die zum Zuschnitt gehören und noch niemand entschieden hat:

- **Push.** Für Buchungsanfragen und -entscheidungen wäre eine Benachrichtigung
  naheliegend; heute läuft alles über E-Mail. Die Dorf-App hat FCM und APNs.
  Das ist ein eigenes Paket, kein Nebenbei — und es setzt voraus, dass die
  Mietplattform weiß, an welches Gerät sie senden soll.
- **Bilder.** `attach_image_to_item` und `POST /images/upload` gibt es; ob die
  App Bilder hochladen können soll, ist offen.

---

## 4. Reihenfolge

```
Z: Clients der Dorf-App im Projekt „Mietplattform" zulassen
        │
        ├──► AP 1  MP: Rössing-ID-Token annehmen (JWKS + Audience)
        │            │
        │            └──► AP 2  DA: Scope in beiden Apps + erneute Anmeldung
        │                          │
AP 3  MP: JSON-Schnittstelle ──────┤
   (lesender Teil zuerst)          │
        │                          │
        └──────────────────────────┴──► AP 6  DA: Bereich in iOS und Android
                                            (erst lesend, dann buchend)

AP 5  MP/Z: Bestandskonten — parallel, blockiert nichts,
            muss aber vor der Umstellung der Webfassung fertig sein
```

Vier Bemerkungen dazu:

1. **Die Zitadel-Freigabe ist die erste Handlung.** Ohne sie lässt sich AP 1
   nicht einmal prüfen — es gibt kein Token, das durchkommen dürfte.
2. **AP 3 kann sofort beginnen** und braucht AP 1 nicht: Der lesende Teil ist
   öffentlich. Damit sind die beiden großen Pakete parallel bearbeitbar.
3. **Der Schnitt „erst lesend, dann buchend"** in AP 6 ist der frühe
   Nutzen: Sobald die öffentlichen Endpunkte stehen, kann der Bereich in die
   App, ohne dass die Identitätsarbeit fertig ist. Das ist Weg (b) — nicht als
   Ziel, sondern als Zwischenstand, den man ausliefern kann.
4. **AP 5 vor der Umstellung der Webfassung.** Solange die Weboberfläche noch
   den alten Weg anbietet, merkt niemand, dass sein Konto nicht aktiviert ist.

---

## 5. Ein erledigter Einwand: das Nachbardorf

Der naheliegende Einwand gegen die Rössing-ID lautet: Wer aus Barnten oder
Adensen ein Gerät mieten will, hat keine — die Umstellung schlösse den Verleih
aufs Dorf ein.

**Der Einwand trägt nicht.** Der Betreiber dazu: „Auf id.rössing.de können
sich alle anmelden. Auch aus Barnten. Wie soll ich das kontrollieren?" Die
Rössing-ID ist ein offener Identitätsanbieter, keine Einwohnerliste. Wer von
außerhalb mieten will, legt sich eine an — dieselbe Handlung wie bisher das
Anlegen eines Kontos in der Mietplattform, nur an einer anderen Stelle.

Damit ist die Umstellung **keine** Einschränkung des Nutzerkreises. Sie
tauscht eine Anmeldung gegen eine andere.

---

## 6. Ein Punkt, der eine Entscheidung braucht

Nicht technisch, sondern grundsätzlich, und er gehört vor die erste Zeile
Quelltext.

`CLAUDE.md:82-85` und `README.md:60-62` sagen über die Dorf-App:

> Die Dorf-App verwaltet die **Allmende**, sie vermittelt nicht zwischen
> Privatleuten. […] **Keine Parallelstrukturen zu bestehenden Vereinen
> aufbauen.**

Der Maschinchenring ist genau das: Privatleute verleihen ihre Maschinen an
Privatleute, gegen Geld, mit Kaution. Er ist ein gutes Ding und er läuft — aber
er ist nicht die Allmende, und als Bereich in der Dorf-App steht er neben einem
Satz, der ihn ausschließt.

Es gibt drei ehrliche Auflösungen, und der Betreiber sollte eine davon wählen:

1. **Den Satz ändern.** Die Dorf-App ist dann das Dach für alles Dörfliche,
   auch für Vermittlung zwischen Privatleuten. Dann gehört die Begründung
   dafür in `README.md` und `CLAUDE.md`, sonst wird die Regel beim nächsten
   Bereich wieder gegen ihn zitiert.
2. **Den Maschinchenring als Träger führen.** Das Modell dafür steht schon:
   „Ein Träger = ein Zitadel-Projekt" mit `admin` und `mitglied`
   (`CLAUDE.md:87-99`), und das Zitadel-Projekt „Mietplattform" gibt es
   bereits. Dann ist der Verleih nicht Privatvermittlung, sondern das Angebot
   eines Trägers — und die Regel bleibt, wie sie ist. Das wäre die sauberste
   Auflösung, verlangt aber, dass jemand den Maschinchenring als Träger
   verantwortet.
3. **Die Ausnahme benennen.** Explizit hinschreiben, dass der Verleih die eine
   Ausnahme ist und warum. Ehrlicher als sie zu übersehen, aber die schwächste
   der drei.

Meine Empfehlung ist (2): Es kostet am wenigsten, es hält die bestehende Regel
intakt, und es ordnet den Verleih dort ein, wo die Dorf-App ohnehin alles
einordnet.

---

## 7. Der Umzug des Repos

Der Umzug von `levino/mietplattform-roessing` in die Organisation
`dorfentwicklungskreis-roessing` ist von dieser Vorlage unabhängig und kann
davor oder danach passieren. **Er wurde nicht ausgeführt.**

Der Befehl (nicht ausgeführt):

```sh
gh api -X POST repos/levino/mietplattform-roessing/transfer \
  -f new_owner=dorfentwicklungskreis-roessing
```

Was mitgeht, und was nicht:

- **Verweise auf das Repo** leitet GitHub dauerhaft um. `git remote`, Links,
  offene Pull Requests und Issues überleben.
- **Pakete in der GitHub Container Registry ziehen nicht mit.** Die Bilder
  heißen `ghcr.io/levino/mieten` und `ghcr.io/levino/mieten-chat`; sie bleiben
  im Namensraum `levino`. Nach dem Umzug kann der `GITHUB_TOKEN` der Actions
  — er gehört dann der Organisation — **nicht mehr dorthin schieben**. Der
  Bau bricht. Zu ändern sind:
  `mietplattform:.github/workflows/deploy.yml`,
  `mietplattform:.github/workflows/preview.yml`,
  `mietplattform:k8s/app.yaml`, `mietplattform:k8s/chat.yaml` und
  `mietplattform:deploy/overlays/production/kustomization.yaml` — plus das
  `ghcr-pull-secret` im Namensraum `mieten-roessing-de`, das dann auf den
  neuen Namensraum lesen können muss.
- **Actions-Secrets und -Variablen des Repos ziehen nicht mit.** Was heute
  gesetzt ist, muss in der Organisation neu gesetzt werden. Betroffen ist
  mindestens `ANTHROPIC_API_KEY` (`claude.yml`); die
  `.env.example` nennt weitere.
- **Der Zugriff auf den k3s-Cluster bricht.** `preview.yml:41-53` und
  `preview-cleanup.yml:15-27` melden sich per GitHub-Actions-OIDC an
  (Audience `levinkeller-de`). Im Token steckt der Repo-Pfad; die
  RoleBindings im Cluster binden auf diesen Pfad. Sie müssen nachgezogen
  werden — **das Manifest dafür liegt nicht im Repo** (deren `CLAUDE.md` nennt
  ein `k8s/rbac.yaml`, das es nicht gibt), also nur direkt im Cluster.
- **Unberührt bleiben**: die SealedSecrets (sie sind an den Cluster gebunden,
  nicht ans Repo), die Ingresse und Domänennamen, die Zitadel-Einstellungen
  und die Rücksprungadressen.
- **Zugriffsrechte**: Wer heute über das persönliche Konto Zugang hat, braucht
  danach eine Team-Mitgliedschaft in der Organisation.

Empfehlung zur Reihenfolge: **Umzug vor AP 1.** Wer erst umbaut und dann
umzieht, hat zwei Fehlerquellen gleichzeitig offen, wenn die CI stehenbleibt.

---

## 8. Was ich nicht klären konnte

- **Wie viele Konten es in der Mietplattform gibt** und wie viele davon
  heute schon eine Rössing-ID haben. Das steht nicht im Quelltext, nicht in
  einer Migration, und die Plattform gibt bewusst keine Nutzerliste heraus.
  Es entscheidet über den Umfang von AP 5 und muss im Cluster nachgesehen
  werden, in `users` auf dem PVC.
- **Wie das Zitadel-Projekt „Mietplattform" (`377276525071827047`) im Inneren
  konfiguriert ist** — Rollen, weitere Clients, Token-Art. Belegt ist nur, was
  von außen sichtbar ist: Es existiert, es hat einen Web-Client
  `377276539064090727`, und dieser meldet die Webfassung an. In beiden Repos
  gibt es keine Beschreibung dieser Einstellungen (grep nach der Projekt-ID
  in diesem Repo: kein Treffer), und Zitadel wird nirgends als Infrastruktur
  im Quelltext beschrieben. Vor AP 1 muss das jemand in der Oberfläche
  ansehen.
- **Ob die Access-Token der beiden App-Clients tatsächlich als JWT ausgestellt
  werden.** Sehr wahrscheinlich ja, weil `backend/internal/auth` sie sonst
  nicht prüfen könnte — aber nachgesehen habe ich es in Zitadel nicht.
- **Wo MinIO und der Ingress für `cdn.mieten.xn--rssing-wxa.de` verwaltet
  werden.** Beide werden von den Manifesten benutzt, aber von keinem im Repo
  angelegt. Für die Integration ist das ohne Belang; für den Betrieb sollte es
  jemand wissen.
