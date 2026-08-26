# Die Mietplattform in den Apps

Entscheidungsvorlage. Stand: 26.08.2026.

Untersucht wurde `levino/mietplattform-roessing` (Klon vom 26.08.2026, letzter
Commit `d56fab7`) gegen den Bestand der Dorf-App (`docs/mieten`, auf
`origin/main`, `4f15336`). Alle Pfadangaben mit dem Präfix `mietplattform:`
beziehen sich auf das fremde Repo; alle anderen auf dieses hier. Am fremden
Repo wurde nichts geändert.

Der Betreiber hat währenddessen entschieden: **volle Integration in beide
Apps, die Webfassung bleibt eigenständig, Anmeldung überall mit der
Rössing-ID.** Die Übernahme der Bestandskonten macht jemand **von Hand**; ein
Migrationswerkzeug wird nicht gebaut (AP 5 ist deshalb eine Handanweisung, keine
Spezifikation). Diese Vorlage ist damit kein Vergleich mehr, sondern ein Plan.
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

### AP 5 — Die Bestandskonten: eine Handanweisung (MP + Z)

Der Betreiber hat entschieden: **kein Werkzeug, kein Skript.** Das ist bei
einer überschaubaren Zahl Konten richtig — ein Migrationsprogramm zu
schreiben, zu testen und einmal laufen zu lassen ist teurer als der Vorgang
selbst. Dieser Abschnitt ist deshalb eine Anweisung für einen Abend, keine
Spezifikation.

#### Die gute Nachricht zuerst

**Im Normalfall wird an der Datenbank nichts angefasst.** Der Grund steht in
1.4: `findOrCreateUser` sucht einen Nutzer über die **E-Mail-Adresse** und
behält dabei dessen bestehende `users.id`. Wer sich mit der Rössing-ID
anmeldet und dieselbe Adresse hat wie sein altes Konto, findet seine Geräte,
seine Buchungen und seinen Vermieter-Status vor, ohne dass jemand etwas
umgeschrieben hätte.

Die Handarbeit besteht deshalb aus **einem** Vorgang: für jede E-Mail-Adresse,
die in der Mietplattform steht, muss es in der Rössing-ID ein Konto mit
**genau derselben** Adresse geben. Nur wenn das nicht herstellbar ist, wird in
der Datenbank gearbeitet — und dafür steht unten das Rezept.

#### Wo genau steht, wem etwas gehört

Das ist die Stelle, an der ein Mensch eingreift, deshalb genau. Das Schema
unten stammt aus den Migrationen `001`–`015`, in Reihenfolge angewandt.

Die Person selbst steht in **`users`**. Der Schlüssel ist `users.id` (eine
zufällige UUID), und `users.email` ist **UNIQUE NOT NULL** — zwei Zeilen mit
derselben Adresse kann es nicht geben. `github_id` und `google_id` stehen noch
in der Tabelle, sind aber laut `010-user-cleanup.sql` „ohnehin nie befüllt
worden".

Auf `users.id` zeigen genau **acht** Stellen:

| Tabelle · Spalte | bedeutet | Fremdschlüssel? |
| --- | --- | --- |
| `items.user_id` | wem das Gerät gehört | ja, `REFERENCES users(id)` — aber **NULL erlaubt** |
| `bookings.user_id` | wer gemietet hat | **nein** — reines `TEXT`, ungeprüft |
| `booking_tokens.user_id` | wer über diese Buchung entscheiden darf | ja |
| `images.user_id` | wer das Bild hochgeladen hat | ja |
| `blocked_periods.user_id` | wer den Zeitraum gesperrt hat | ja, `NOT NULL` |
| `oauth_grants.user_id` | welchem Client jemand Zugriff gab | ja, `NOT NULL` |
| `oauth_codes.user_id` | laufender Anmeldevorgang | **nein**, und ohnehin flüchtig |
| `users.approved_by` | welcher Admin jemanden freigeschaltet hat | **nein** |

Zwei Dinge daran sind für die Handarbeit wichtig:

- **`bookings.user_id` ist nicht abgesichert.** Ein Tippfehler dort wird von
  SQLite nicht bemerkt und erzeugt eine Buchung, die zu niemandem mehr gehört.
  Dasselbe gilt für `oauth_codes.user_id` und `users.approved_by`.
- **`items.user_id` darf NULL sein** (seit `007-retire-owners.sql`). Ein Gerät
  ohne Eigentümer steht weiter in der Liste, aber niemand bekommt die
  Buchungsanfrage per Mail — `create_booking` verschickt sie an
  `getUserById(item.userId).email` und tut ohne Eigentümer schlicht nichts.

Es gibt in der Mietplattform **keine** Kopie der Identität an anderer Stelle:
kein Zwischenspeicher, keine zweite Datenbank. Der Chat-Container hat eine
eigene SQLite-Datei, speichert dort aber nur Gesprächsverläufe und rechnet mit
demselben `sub` aus dem Token — er hält keine Nutzerliste.

#### Wie viele Konten es sind

**Konnte ich nicht feststellen, und es muss vor dem Termin nachgesehen
werden** — daran hängt, ob „von Hand" trägt. Aus dem Repo ist die Zahl nicht
ablesbar: Es gibt keine Seed-Liste mit echten Personen, keine Migration, die
Konten zählt, und die Plattform gibt bewusst nirgends eine Nutzerliste heraus
(deren `CLAUDE.md`, „Was Admins dürfen und was nicht"). Von außen sichtbar ist
nur: 27 Geräte auf der Startseite, jedes mit einem Eigentümer — die Zahl der
verschiedenen Eigentümer ist also die Untergrenze, die Zahl aller Konten
liegt darüber.

Nachsehen (ungeprüft — der Befehl ist aus `Dockerfile` und `k8s/app.yaml`
abgeleitet, nicht ausgeführt):

```sh
kubectl -n mieten-roessing-de exec deploy/mieten -- node -e "
const db = require('better-sqlite3')('/data/db/mieten.sqlite', { readonly: true });
console.log(db.prepare('SELECT COUNT(*) AS konten FROM users').get());
console.log(db.prepare('SELECT COUNT(DISTINCT user_id) AS eigentuemer FROM items WHERE user_id IS NOT NULL').get());
"
```

Faustregel für den Abend: Bis etwa 30 Konten ist Handarbeit klar billiger.
Wird es dreistellig, gehört die Entscheidung noch einmal auf den Tisch.

#### Die Anweisung

**Vorher, einmal:**

1. **Sicherung ziehen.** Die Datei liegt auf dem PVC unter
   `/data/db/mieten.sqlite`. Sie läuft **nicht** im WAL-Modus — der Server
   setzt kein `journal_mode`. Wer daneben schreibt, während der Server läuft,
   riskiert eine gesperrte oder halb geschriebene Datei. Also: erst sichern,
   und für jede Änderung an der Datei den Server anhalten
   (`kubectl -n mieten-roessing-de scale deploy/mieten --replicas=0`) und
   danach wieder starten. Für reines **Lesen** genügt `readonly: true` wie
   oben, ohne den Server anzuhalten.
2. **Liste ziehen** — das ist die Arbeitsgrundlage für den Abend:

   ```sql
   SELECT id, email, name, phone, lender, admin FROM users ORDER BY created_at;
   ```

   `lender = 1` markiert die Vermieter. Das sind die Leute, bei denen es
   wehtut, wenn sie nicht hineinkommen (Schritt 6).

**Für jede Zeile der Liste:**

3. **Gibt es in der Rössing-ID schon ein Konto mit dieser E-Mail-Adresse?**
   Wenn ja: fertig, nichts zu tun. Die Person meldet sich beim nächsten Mal an
   und findet alles vor.
4. **Wenn nein: Konto anlegen**, mit **genau dieser** Adresse, im Zustand
   *initial* (`USER_STATE_INITIAL`, ohne Passwort — die Person vergibt es beim
   ersten Anmelden selbst). Adresse abtippen ist die häufigste Fehlerquelle des
   Abends: ein Zeichen daneben, und der Join greift nicht, sondern legt beim
   ersten Login ein zweites, leeres Konto an. Lieber kopieren als tippen.
5. **Sonderfall — die Person hat schon eine Rössing-ID unter einer anderen
   Adresse.** Dann gibt es zwei Wege, und der erste ist fast immer der
   richtige:
   - **a)** Die Rössing-ID auf die Adresse aus der Mietplattform umstellen.
     Keine Datenbankänderung, kein Risiko.
   - **b)** Die Adresse in der Mietplattform ändern — **ein** `UPDATE`, das
     nichts anderes berührt, weil alles an der `id` hängt und nicht an der
     Adresse:

     ```sql
     UPDATE users SET email = 'neu@example.de' WHERE id = '<alte-uuid>';
     ```

     Vorsicht: `users.email` ist UNIQUE. Steht die neue Adresse schon in einer
     anderen Zeile, schlägt das `UPDATE` fehl — dann liegen zwei Konten
     derselben Person vor, und es geht bei Schritt 7 weiter.
6. **Vermieter gesondert ansprechen.** Wer `lender = 1` hat und sein Konto
   nicht aktiviert, verliert nichts, kommt aber nicht mehr an seine Geräte.
   Diese Leute sind einzeln anzurufen, nicht anzumailen — es sind wenige, und
   sie sind es, an denen der Verleih hängt.

**Nur im Ausnahmefall — zwei Konten derselben Person zusammenführen:**

7. Das passiert, wenn jemand sich mit einer anderen Adresse angemeldet hat und
   dabei eine zweite, leere Zeile in `users` entstanden ist. Zusammengeführt
   wird auf die **alte** Zeile (dort hängen die Daten); die neue wird geleert
   und gelöscht. Bei angehaltenem Server, in einer Transaktion, mit
   eingeschalteter Fremdschlüsselprüfung — die `sqlite3`-Kommandozeile hat sie
   **standardmäßig aus**, anders als der Server:

   ```sql
   PRAGMA foreign_keys = ON;
   BEGIN;
   -- 1. Alle acht Stellen aus der Tabelle oben umbiegen:
   UPDATE items           SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE bookings        SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE booking_tokens  SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE images          SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE blocked_periods SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE oauth_grants    SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE oauth_codes     SET user_id     = '<alt>' WHERE user_id     = '<neu>';
   UPDATE users           SET approved_by = '<alt>' WHERE approved_by = '<neu>';
   -- 2. Erst danach die alte Zeile auf die neue Adresse setzen …
   DELETE FROM users WHERE id = '<neu>';
   UPDATE users SET email = '<neue-adresse>' WHERE id = '<alt>';
   COMMIT;
   ```

   Die Reihenfolge ist nicht beliebig: Erst müssen alle Verweise weg von
   `<neu>`, dann darf die Zeile gelöscht werden, und erst dann ist die Adresse
   frei für den UNIQUE-Index. Dasselbe Vorgehen — Verweise zuerst, Schlüssel
   zuletzt — steht als Vorlage in `011-randomize-magic-link-ids.sql`, wo genau
   diese Umschlüsselung schon einmal gemacht wurde.

8. **Zum Schluss prüfen** (lesend, Server darf wieder laufen):

   ```sql
   -- Buchungen, deren Mieter es nicht mehr gibt (bookings.user_id ist ungeprüft!)
   SELECT b.id, b.user_id, b.first_name, b.last_name, b.status
     FROM bookings b LEFT JOIN users u ON u.id = b.user_id
    WHERE u.id IS NULL;

   -- Geräte ohne Eigentümer: dorthin geht keine Buchungsanfrage mehr
   SELECT id, name FROM items WHERE user_id IS NULL;

   -- frisch entstandene Karteileichen: Konto ohne Gerät und ohne Buchung
   SELECT u.id, u.email, u.name FROM users u
    WHERE NOT EXISTS (SELECT 1 FROM items    i WHERE i.user_id = u.id)
      AND NOT EXISTS (SELECT 1 FROM bookings b WHERE b.user_id = u.id);
   ```

   Die dritte Abfrage ist die wichtigste und gehört **auch in den Wochen nach
   der Umstellung** wiederholt: Ein leeres Konto neben einem vollen mit
   ähnlichem Namen ist das Zeichen dafür, dass jemand mit der falschen Adresse
   hereingekommen ist.

#### Laufende Buchungen während der Umstellung

**Es braucht kein Umstellungsfenster und keine Sperrfrist.** Der Grund:

- **Entscheidungen laufen über Mail-Links, nicht über Sitzungen.** Ein
  Approve- oder Reject-Link hängt an einer Zeile in `booking_tokens`
  (`server/src/http/bookings.ts`), nicht an einem Login. Ein Eigentümer kann
  also mitten in der Umstellung eine Anfrage bestätigen, auch ohne
  Rössing-ID.
- **Der öffentliche Kalender kennt keine Nutzer.** `GET /bookings` liefert nur
  Zeitraum, Gerät und Status. Er ist von allem hier unberührt.
- **Eigene Buchungen bleiben sichtbar**, solange `users.id` unverändert
  bleibt — `/api/my-bookings` filtert auf `payload.sub`, und der ist die
  lokale `id`, nicht der Zitadel-`sub`.

Die einzige Reihenfolge, die wirklich gilt, ist deshalb kurz:

1. Server anhalten, Sicherung ziehen — **nur** wenn Schritt 5b oder 7 ansteht.
2. Ändern, prüfen, Server starten.
3. Bestätigte Buchungen im laufenden Zeitraum vorher durchsehen: Wer gerade
   ein Gerät bei sich stehen hat, sollte an dem Abend nicht plötzlich seine
   Abholadresse nicht mehr sehen. Die Abfrage dafür:
   `SELECT id, device_id, start_date, end_date FROM bookings WHERE status = 'approved' AND end_date >= date('now');`

#### Wenn jemand sein Konto nie aktiviert

Kurz: **Es geht nichts verloren, aber es wird auch nicht von selbst besser.**

- Die Zeile in `users` bleibt stehen. **Niemals löschen** — an ihr hängen
  Buchungen und Geräte, und `bookings.user_id` hat keinen Fremdschlüssel, der
  einen davor bewahren würde.
- **Seine Geräte bleiben buchbar.** Anfragen gehen weiter per E-Mail an
  `users.email`, und der Entscheid-Link in der Mail funktioniert ohne Login.
  Der Verleih läuft also weiter — solange er sein Postfach liest.
- **Was er nicht mehr kann**: Geräte anlegen oder ändern, Zeiträume sperren,
  die Übersicht seiner Buchungen sehen.
- **Herrenlos wird eine Buchung dadurch nicht.** Sie zeigt weiter auf ein
  existierendes Konto. Wirklich herrenlos wird sie nur auf zwei Wegen: wenn
  jemand die `users`-Zeile löscht, oder wenn sich die Person unter einer
  **anderen** Adresse neu anmeldet — dann steht sie auf einem zweiten,
  leeren Konto, sieht ihre alten Buchungen nicht mehr und hält das für einen
  Fehler der App.
- **Gesehen wird beides** mit den drei Abfragen aus Schritt 8. Die erste
  findet echte Waisen, die dritte findet den zweiten Fall — und der ist der
  wahrscheinlichere.

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

AP 5  MP/Z: Bestandskonten — von Hand, blockiert nichts
            und wird von nichts blockiert (siehe Bemerkung 4)
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
4. **AP 5 hängt an nichts — und ist vermutlich überfällig.** Die Webfassung
   ist bereits auf die Rössing-ID umgestellt (1.1). Wer ein altes Konto in der
   Mietplattform hat und keine Rössing-ID, kommt **heute schon** nicht mehr
   hinein — nicht erst, wenn die Apps kommen. Die Handanweisung aus AP 5 kann
   also am nächsten Abend abgearbeitet werden und sollte es auch. Ob das viele
   trifft oder niemanden, weiß bis dahin niemand; die Abfrage dafür steht in
   AP 5.

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
  Es entscheidet darüber, ob die Handanweisung aus AP 5 einen Abend oder eine
  Woche kostet, und muss im Cluster nachgesehen werden — der Befehl dafür
  steht in AP 5, „Wie viele Konten es sind".
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
