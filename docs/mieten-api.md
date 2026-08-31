# Die JSON-Schnittstelle der Mietplattform

Vertrag für die beiden Dorf-Apps. Stand: 31.08.2026.

Diese Datei beschreibt, was die Mietplattform den Apps anbietet. Sie ist die
**einzige** Quelle für den Bereich „Verleih" in iOS und Android — wer hier
etwas nicht findet, findet es auch nicht im Server. Die Umsetzung drüben
läuft unter `levino/mietplattform-roessing`; der Plan dahinter steht in
`docs/mietplattform-in-den-apps.md`, Arbeitspaket 3.

> **Achtung, solange der Pull Request drüben offen ist:** Die Routen unter
> `/api/v1/…` sind hier festgelegt, aber noch nicht ausgerollt. Wer gegen
> diese Datei baut, kann die Ansichten und Datentypen fertigstellen; die
> ersten echten Antworten kommen, sobald der PR in der Mietplattform
> zusammengeführt und ausgerollt ist. Die **Form** ändert sich danach nicht
> mehr ohne eine Änderung an dieser Datei.

---

## 1. Grundsätzliches

### Adresse

```
https://mieten.xn--rssing-wxa.de
```

Das ist **nicht** das Go-Backend der Dorf-App. Der Bereich Verleih redet
unmittelbar mit der Mietplattform, so wie die Veranstaltungen unmittelbar
mit ihrem Server reden (`README.md`, „Veranstaltungen"). Das Backend erfährt
davon nichts und ist kein Weiterleiter — diese Entscheidung ist gefallen
(Arbeitspaket 4).

Die Adresse gehört nach `CLAUDE.md` in die Build-Einstellungen, nicht in den
Quelltext: iOS über `ios/project.yml` → `Info.plist` → `Konfiguration.swift`,
Android über `android/app/build.gradle.kts` → `BuildConfig`.

### Anmeldung

Wo ein Token verlangt wird, ist es das **Access-Token der Rössing-ID**
(Zitadel, `https://id.xn--rssing-wxa.de`) — dasselbe, das die App schon für
das Go-Backend benutzt:

```
Authorization: Bearer <access_token>
```

Die Mietplattform prüft Signatur (JWKS des Ausstellers), Aussteller und
**Empfänger**. Damit ihr Projekt in der `aud` des Tokens steht, muss die App
beim Anmelden den zusätzlichen Scope anfordern:

```
urn:zitadel:iam:org:project:id:377276525071827047:aud
```

Das ist Arbeitspaket 2 und je App eine Zeile (`ANMELDE_SCOPES` in
`ios/Dorf/Anmeldung/Anmeldung.swift`, `LOGIN_SCOPES` in
`android/…/auth/AuthManager.kt`).

**Der Stolperstein, der dieses Projekt schon einmal getroffen hat:** Ein
bereits angemeldetes Gerät behält seinen Token-Satz über die Aktualisierung
hinweg. Wer den Scope nur einbaut, bekommt auf allen Bestandsgeräten weiter
Token **ohne** die neue Audience — und von der Mietplattform ein `401`. Die
App muss das auffangen und eine erneute Anmeldung anstoßen, statt eine leere
Liste zu zeigen. Daran erkennt man den Fall:

```json
{ "error": { "code": "token_audience", "message": "Token gilt nicht für die Mietplattform" } }
```

Der Fehlercode `token_audience` heißt: **Token neu holen, dann erneut
versuchen.** Der Code `unauthorized` heißt: gar kein oder ein abgelaufenes
Token — normale Anmeldung.

### Was ohne Anmeldung geht

Geräte auflisten, Gerät im Detail, suchen, Sets, Verfügbarkeit und belegte
Zeiträume brauchen **kein** Token. Der Bereich kann also vollständig
angezeigt werden, bevor sich jemand anmeldet. Ein Token verlangen nur:
buchen, eigene Buchungen, stornieren, das Profil und alles auf der
Vermieterseite.

### Datenformate

- Alle Antworten sind `application/json; charset=utf-8`.
- **Datumsangaben** sind `YYYY-MM-DD`, ohne Uhrzeit, ohne Zeitzone.
- **Zeitpunkte** (nur wo genannt) sind Millisekunden seit 1970 als Zahl.
- **Preise** sind Zahlen in Euro (`25`, `12.5`). Kein Cent-Integer, keine
  Währungsangabe — es ist immer Euro.
- Fehlende Werte sind `null`, nicht ausgelassen. Ein Feld, das im Beispiel
  steht, steht auch in der Antwort.
- Listen kommen in einem Hüllobjekt (`{"items": […]}`), nie als nacktes
  Array. So kann später ein Feld danebentreten, ohne den Vertrag zu brechen.

### Zeiträume: `endDate` ist der Rückgabetag

Ein Zeitraum ist **halboffen**: `startDate` gehört dazu, `endDate` nicht.
Zwei Zeiträume überschneiden sich genau dann, wenn

```
a.startDate < b.endDate  &&  a.endDate > b.startDate
```

Eine Buchung vom `2026-09-05` bis `2026-09-07` belegt also den 5. und den 6.;
am 7. kann jemand anders anfangen. **Die App rechnet das nicht selbst nach** —
sie fragt `GET /api/v1/availability` oder zeichnet, was `GET /api/v1/occupancy`
liefert. Die Regel steht hier nur, damit die Anzeige des Kalenders stimmt.

### Fehler

Jeder Fehler kommt in derselben Form:

```json
{
  "error": {
    "code": "not_found",
    "message": "Gerät nicht gefunden"
  }
}
```

`message` ist deutsch und darf dem Nutzer gezeigt werden. `code` ist stabil
und maschinenlesbar; **die App verzweigt auf `code`, nie auf `message`.**

| HTTP | `code` | Bedeutung |
| --- | --- | --- |
| 400 | `bad_request` | Anfrage unvollständig oder unverständlich |
| 400 | `invalid_period` | `endDate` liegt nicht nach `startDate`, oder Datumsformat falsch |
| 400 | `profile_incomplete` | Für diesen Schritt fehlen Angaben im Profil (siehe `missingFields`) |
| 401 | `unauthorized` | Kein oder abgelaufenes Token → anmelden |
| 401 | `token_audience` | Token gilt nicht für die Mietplattform → **neu anmelden** (siehe oben) |
| 403 | `forbidden` | Angemeldet, aber nicht berechtigt (fremde Buchung, fremdes Gerät) |
| 403 | `not_a_lender` | Handlung nur für freigeschaltete Vermieter |
| 404 | `not_found` | Gerät, Set, Buchung oder Sperre gibt es nicht |
| 409 | `occupied` | Zeitraum ist belegt |
| 409 | `conflict` | Der Zustand passt nicht (z. B. Buchung ist nicht mehr `pending`) |
| 429 | `rate_limited` | Zu schnell hintereinander (nur bei `lender-request`) |
| 500 | `internal` | Fehler auf dem Server |

Bei `profile_incomplete` steht zusätzlich, was fehlt:

```json
{
  "error": {
    "code": "profile_incomplete",
    "message": "Dein Profil ist unvollständig",
    "missingFields": ["phone", "addressStreet", "addressZip", "addressCity"]
  }
}
```

### Was die App nicht entscheidet

**Die Apps enthalten keine Regeln des Verleihs.** Ob ein Zeitraum buchbar
ist, wer stornieren darf, wer Vermieter werden kann — das entscheidet der
Server und sagt es der App. Ein Knopf darf ausgegraut werden, weil der Server
das mitgeteilt hat (`canCancel`, `canDecide`, `lenderStatus`), aber die
Bedingung dahinter gehört nicht in die App. Sonst laufen Web und App
auseinander, und zwar zuerst dort, wo es weh tut.

---

## 2. Die Routen

Übersicht. Ausführlich darunter, in derselben Reihenfolge.

| # | Methode | Pfad | Token |
| --- | --- | --- | --- |
| 1 | GET | `/api/v1/items` | nein |
| 2 | GET | `/api/v1/items/{id}` | nein |
| 3 | GET | `/api/v1/search` | nein |
| 4 | GET | `/api/v1/sets` | nein |
| 5 | GET | `/api/v1/availability` | nein |
| 6 | GET | `/api/v1/occupancy` | nein |
| 7 | GET | `/api/v1/me` | ja |
| 8 | PATCH | `/api/v1/me` | ja |
| 9 | POST | `/api/v1/me/lender-request` | ja |
| 10 | GET | `/api/v1/bookings/mine` | ja |
| 11 | POST | `/api/v1/bookings` | ja |
| 12 | POST | `/api/v1/bookings/{id}/cancel` | ja |
| 13 | GET | `/api/v1/owner/bookings` | ja |
| 14 | POST | `/api/v1/bookings/{id}/approve` | ja |
| 15 | POST | `/api/v1/bookings/{id}/reject` | ja |
| 16 | GET | `/api/v1/owner/items` | ja |
| 17 | GET | `/api/v1/owner/blocks` | ja |
| 18 | POST | `/api/v1/owner/blocks` | ja |
| 19 | DELETE | `/api/v1/owner/blocks/{id}` | ja |

---

### 1. Geräte auflisten

```
GET /api/v1/items
```

Ohne Anmeldung. Keine Parameter. Liefert alle **aktiven** Geräte, nach Namen
sortiert. Es sind heute rund 27 Stück — es gibt keine Seitenaufteilung, und
sie ist auch nicht geplant.

**Antwort 200:**

```json
{
  "items": [
    {
      "id": "as-585-km-kreiselmaeher",
      "name": "AS 585 KM Kreiselmäher",
      "description": "Kreiselmäher für hohes Gras und Böschungen.\n\n- Arbeitsbreite 85 cm\n- Benzin, Radantrieb",
      "pricePerDay": 25,
      "pricePerWeekend": 40,
      "pricePerWeek": 120,
      "deposit": 100,
      "tags": ["garten", "motorgeraet"],
      "thumbnailUrl": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/resize:fill:600:450/plain/local:///items/as-585-km-kreiselmaeher/front.jpg",
      "productUrl": "https://www.as-motor.de/as-585",
      "webUrl": "https://mieten.xn--rssing-wxa.de/geraete/as-585-km-kreiselmaeher/"
    },
    {
      "id": "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
      "name": "Rasenwalze",
      "description": null,
      "pricePerDay": 8,
      "pricePerWeekend": null,
      "pricePerWeek": null,
      "deposit": null,
      "tags": ["garten"],
      "thumbnailUrl": null,
      "productUrl": null,
      "webUrl": "https://mieten.xn--rssing-wxa.de/geraete/018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11/"
    }
  ]
}
```

Zu den Feldern:

- `id` ist eine undurchsichtige Zeichenkette. Meist eine UUID, bei älteren
  Beständen ein sprechender Name. **Nicht darauf verlassen, wie sie aussieht.**
- `description` ist **Markdown** — Absätze, `- ` als Aufzählung, `**fett**`,
  Überschriften `##` bis `####`, Links. Die App muss das darstellen können
  oder es bewusst als Klartext zeigen; roh mit Sternchen anzeigen ist keine
  Lösung. Kann `null` sein.
- `pricePerWeekend`, `pricePerWeek`, `deposit` sind oft `null`. Dann gibt es
  diesen Tarif bzw. keine Kaution — nicht auf 0 setzen und nicht rechnen.
- `thumbnailUrl` ist eine fertige, signierte Adresse (imgproxy, 600×450,
  zugeschnitten). Sie ist ohne Token abrufbar. `null` heißt: kein Bild.
- `webUrl` zeigt auf die Webfassung desselben Geräts. Nützlich für „im
  Browser öffnen"; die App braucht sie sonst nicht.
- **Eine Eigentümerangabe gibt es hier nicht und wird es nicht geben.** Wer
  ein Gerät verleiht, steht in keiner öffentlichen Antwort — das ist eine
  Datenschutzregel der Mietplattform, keine Auslassung.

**Fehler:** nur `500 internal`.

---

### 2. Ein Gerät im Detail

```
GET /api/v1/items/{id}
```

Ohne Anmeldung. Wie oben, zusätzlich mit allen Bildern.

**Antwort 200:**

```json
{
  "item": {
    "id": "as-585-km-kreiselmaeher",
    "name": "AS 585 KM Kreiselmäher",
    "description": "Kreiselmäher für hohes Gras und Böschungen.\n\n- Arbeitsbreite 85 cm\n- Benzin, Radantrieb",
    "pricePerDay": 25,
    "pricePerWeekend": 40,
    "pricePerWeek": 120,
    "deposit": 100,
    "tags": ["garten", "motorgeraet"],
    "thumbnailUrl": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/resize:fill:600:450/plain/local:///items/as-585-km-kreiselmaeher/front.jpg",
    "productUrl": "https://www.as-motor.de/as-585",
    "webUrl": "https://mieten.xn--rssing-wxa.de/geraete/as-585-km-kreiselmaeher/",
    "images": [
      {
        "id": "img-7f3a",
        "url": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/resize:fill:600:450/plain/local:///items/as-585-km-kreiselmaeher/front.jpg",
        "isThumbnail": true
      },
      {
        "id": "img-9b21",
        "url": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/resize:fill:600:450/plain/local:///items/as-585-km-kreiselmaeher/seite.jpg",
        "isThumbnail": false
      }
    ]
  }
}
```

`images` ist immer da, kann leer sein. Genau ein Eintrag kann
`isThumbnail: true` haben — oder keiner.

**Fehler:** `404 not_found`.

---

### 3. Suchen

```
GET /api/v1/search?q=<text>&tags=<a,b>&limit=<n>
```

Ohne Anmeldung.

| Parameter | Pflicht | Bedeutung |
| --- | --- | --- |
| `q` | ja | Suchbegriff, mindestens ein Zeichen |
| `tags` | nein | Kommaliste; ein Treffer muss **alle** genannten Tags haben |
| `limit` | nein | 1–20, Vorgabe 5 |

Die Suche ist hybrid: semantisch (Einbettungen) und wörtlich, zusammen
gewichtet. Was sie liefert, ist damit **keine** stabile Sortierung nach
Namen, sondern nach Passung. Die App sortiert nicht um.

**Antwort 200:**

```json
{
  "results": [
    {
      "id": "as-585-km-kreiselmaeher",
      "name": "AS 585 KM Kreiselmäher",
      "description": "Kreiselmäher für hohes Gras und Böschungen.",
      "pricePerDay": 25,
      "pricePerWeekend": 40,
      "pricePerWeek": 120,
      "deposit": 100,
      "tags": ["garten", "motorgeraet"],
      "thumbnailUrl": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/…/front.jpg",
      "productUrl": null,
      "webUrl": "https://mieten.xn--rssing-wxa.de/geraete/as-585-km-kreiselmaeher/",
      "score": 0.913
    }
  ]
}
```

Dieselben Felder wie in der Liste, zusätzlich `score` (0 bis 1, drei
Nachkommastellen). `score` ist zum Sortieren gedacht, nicht zum Anzeigen.

Kein Treffer ist kein Fehler: `{"results": []}` mit `200`.

**Fehler:** `400 bad_request`, wenn `q` fehlt oder leer ist.

---

### 4. Sets auflisten

```
GET /api/v1/sets
```

Ohne Anmeldung. Ein Set ist eine Zusammenstellung mehrerer Geräte zu einem
eigenen Tagespreis.

**Antwort 200:**

```json
{
  "sets": [
    {
      "id": "gartenset",
      "name": "Gartenset",
      "description": "Vertikutierer, Rasenwalze und Streuwagen zusammen.",
      "pricePerDay": 30,
      "deposit": 150,
      "itemIds": [
        "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
        "vertikutierer",
        "streuwagen"
      ]
    }
  ]
}
```

`description` ist bei Sets **Klartext**, kein Markdown. `itemIds` verweist
auf Geräte aus Route 1 — ein Set hat kein eigenes Bild; die App zeigt die
Bilder seiner Geräte.

Buchen lässt sich ein Set über Route 11 (`setId` statt `deviceId`).
**Achtung:** Stornieren, Bestätigen und Ablehnen von Set-Buchungen ist
serverseitig noch nicht umgesetzt und antwortet mit `409 conflict`. Wer den
Bereich zuschneidet, sollte Sets vorerst nur **anzeigen**.

**Fehler:** nur `500 internal`.

---

### 5. Verfügbarkeit prüfen

```
GET /api/v1/availability?deviceId=<id>&startDate=<d>&endDate=<d>
GET /api/v1/availability?setId=<id>&startDate=<d>&endDate=<d>
```

Ohne Anmeldung. Genau eines von `deviceId` und `setId` angeben. Das ist die
Frage, die die App vor dem Buchen stellt — **sie beantwortet sie nicht
selbst**, auch wenn sie den Kalender aus Route 6 schon gezeichnet hat.

**Antwort 200 (frei):**

```json
{ "available": true, "reason": null }
```

**Antwort 200 (belegt):**

```json
{ "available": false, "reason": "occupied" }
```

`reason` ist `null` oder `"occupied"`. Ob eine fremde Buchung, eine
angefragte Buchung oder eine Sperre des Vermieters im Weg steht, sagt die
Antwort bewusst nicht — das ginge sonst niemanden etwas an.

**Fehler:** `400 bad_request` (weder `deviceId` noch `setId`, oder beide),
`400 invalid_period` (Datumsformat falsch oder `endDate` nicht nach
`startDate`), `404 not_found`.

---

### 6. Belegte Zeiträume

```
GET /api/v1/occupancy?deviceId=<id>
GET /api/v1/occupancy?setId=<id>
GET /api/v1/occupancy
```

Ohne Anmeldung. Ohne Parameter kommt die Belegung **aller** Geräte — das ist
der Kalender für eine Übersicht. Mit `deviceId` nur die eines Geräts; dann
sind auch die Sperren des Vermieters enthalten.

**Antwort 200:**

```json
{
  "periods": [
    {
      "deviceId": "as-585-km-kreiselmaeher",
      "setId": null,
      "startDate": "2026-09-05",
      "endDate": "2026-09-07",
      "status": "approved"
    },
    {
      "deviceId": "as-585-km-kreiselmaeher",
      "setId": null,
      "startDate": "2026-09-12",
      "endDate": "2026-09-14",
      "status": "pending"
    },
    {
      "deviceId": "as-585-km-kreiselmaeher",
      "setId": null,
      "startDate": "2026-10-01",
      "endDate": "2026-10-08",
      "status": "blocked"
    }
  ]
}
```

`status` ist `"pending"`, `"approved"` oder `"blocked"`. Alle drei bedeuten
für den Kalender dasselbe: **belegt.** Die Unterscheidung ist nur für die
Darstellung da (angefragt gestrichelt, bestätigt voll, gesperrt grau) — sie
ist kein Hinweis darauf, dass „pending" noch zu haben wäre. Es ist nicht.

**Hier stehen keine Personendaten.** Nie. Wer gebucht hat, sagt diese Route
nicht, und sie wird es auch nicht lernen.

**Fehler:** nur `500 internal`.

---

### 7. Eigenes Profil lesen

```
GET /api/v1/me
```

**Token nötig.** Legt beim ersten Aufruf mit einem neuen Rössing-ID-Konto
still ein Konto in der Mietplattform an, verknüpft über die E-Mail-Adresse.
Die App muss dafür nichts tun.

**Antwort 200:**

```json
{
  "profile": {
    "name": "Erika Musterfrau",
    "email": "erika@example.de",
    "phone": "+49 5069 123456",
    "addressStreet": "Hauptstraße 1",
    "addressZip": "31171",
    "addressCity": "Nordstemmen",
    "lender": false,
    "lenderStatus": "none",
    "profileComplete": false,
    "missingFields": ["addressCity"]
  }
}
```

- `lenderStatus` ist `"none"`, `"pending"` oder `"approved"`. Nur bei
  `"approved"` (dann ist auch `lender: true`) zeigt die App die
  Vermieteransicht (Routen 13 bis 19).
- `profileComplete` ist `true`, wenn `missingFields` leer ist. Verlangt sind
  Telefon, Straße, PLZ und Ort — der Name reicht nicht.
- `admin` steht bewusst **nicht** in dieser Antwort. Die Freigabe von
  Vermietern läuft ausschließlich über die Webfassung; die App hat damit
  nichts zu tun.

**Fehler:** `401 unauthorized`, `401 token_audience`.

---

### 8. Eigenes Profil ändern

```
PATCH /api/v1/me
```

**Token nötig.** Nur die gesendeten Felder ändern sich; alle sind optional.

**Anfrage:**

```json
{
  "name": "Erika Musterfrau",
  "phone": "+49 5069 123456",
  "addressStreet": "Hauptstraße 1",
  "addressZip": "31171",
  "addressCity": "Nordstemmen"
}
```

Die E-Mail-Adresse ist **nicht** änderbar — sie kommt aus der Rössing-ID und
ist zugleich die Verknüpfung zum Konto. Ein `email`-Feld in der Anfrage wird
ignoriert.

**Antwort 200:** dasselbe Objekt wie Route 7, mit den neuen Werten.

**Fehler:** `400 bad_request` (leerer Wert in einem gesendeten Feld),
`401 unauthorized`, `401 token_audience`.

---

### 9. Vermieter werden

```
POST /api/v1/me/lender-request
```

**Token nötig.** Kein Anfragekörper. Schickt eine Anfrage an die
Verwaltenden der Mietplattform; die entscheiden von Hand und in der
Webfassung. Die Antwort ist eine Eingangsbestätigung, keine Freischaltung.

**Antwort 200:**

```json
{
  "lenderStatus": "pending",
  "message": "Deine Anfrage wurde weitergeleitet. Du bekommst eine E-Mail, sobald dein Zugang freigeschaltet ist."
}
```

`message` ist deutsch und für die Anzeige gedacht.

**Fehler:**

- `400 profile_incomplete` — Telefon und Adresse müssen stehen. `missingFields`
  sagt, was fehlt; die App führt dann nach Route 8.
- `409 conflict` — schon Vermieter.
- `429 rate_limited` — die letzte Anfrage liegt weniger als eine Stunde
  zurück.
- `401 unauthorized`, `401 token_audience`.

---

### 10. Eigene Buchungen

```
GET /api/v1/bookings/mine
```

**Token nötig.** Alle Buchungen, bei denen der Angemeldete der Mieter ist —
auch abgelehnte und stornierte, aufsteigend nach `startDate`.

**Antwort 200:**

```json
{
  "bookings": [
    {
      "id": "8f14c2b0-91ae-4c77-b1b2-0a3d5e7c9f01",
      "deviceId": "as-585-km-kreiselmaeher",
      "setId": null,
      "deviceName": "AS 585 KM Kreiselmäher",
      "startDate": "2026-09-05",
      "endDate": "2026-09-07",
      "status": "approved",
      "notes": "Hole ich Samstag früh ab.",
      "canCancel": true,
      "pickup": {
        "address": "Hauptstraße 1, 31171 Nordstemmen",
        "phone": "+49 5069 123456"
      }
    },
    {
      "id": "1c9e77a3-1234-4bb8-9a0e-77ce31d2a456",
      "deviceId": "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
      "setId": null,
      "deviceName": "Rasenwalze",
      "startDate": "2026-10-01",
      "endDate": "2026-10-03",
      "status": "pending",
      "notes": null,
      "canCancel": true,
      "pickup": null
    }
  ]
}
```

- `status`: `"pending"`, `"approved"`, `"rejected"` oder `"cancelled"`.
- `pickup` ist **nur bei `status: "approved"`** gefüllt und nur, wenn der
  Vermieter eine Adresse hinterlegt hat — sonst `null`. Das ist die einzige
  Stelle in dieser Schnittstelle, an der eine Adresse und eine Telefonnummer
  eines anderen Menschen steht, und sie steht dort erst, nachdem er die
  Buchung bestätigt hat. Nicht zwischenspeichern, nicht anderswo anzeigen.
- `canCancel` sagt, ob Route 12 jetzt Aussicht auf Erfolg hat. **Der Knopf
  richtet sich danach**, nicht nach einer selbst gebauten Statusprüfung.
- `deviceName` ist bei Set-Buchungen der Name des Sets.

**Fehler:** `401 unauthorized`, `401 token_audience`.

---

### 11. Buchen

```
POST /api/v1/bookings
```

**Token nötig.** Legt eine Buchungsanfrage an (`status: "pending"`) und
schickt dem Vermieter eine E-Mail mit einem Entscheid-Link. Bestätigt ist
damit nichts.

**Anfrage:**

```json
{
  "deviceId": "as-585-km-kreiselmaeher",
  "startDate": "2026-09-05",
  "endDate": "2026-09-07",
  "firstName": "Erika",
  "lastName": "Musterfrau",
  "phone": "+49 5069 123456",
  "notes": "Hole ich Samstag früh ab."
}
```

| Feld | Pflicht | Bemerkung |
| --- | --- | --- |
| `deviceId` | eines von beiden | Gerät |
| `setId` | eines von beiden | Set. **Nicht** beide angeben |
| `startDate` | ja | `YYYY-MM-DD` |
| `endDate` | ja | `YYYY-MM-DD`, muss nach `startDate` liegen |
| `firstName` | nein | fehlt er, nimmt der Server den Namen aus dem Profil |
| `lastName` | nein | wie oben |
| `phone` | nein | fehlt sie, nimmt der Server die Nummer aus dem Profil |
| `notes` | nein | freier Text für den Vermieter |

Die drei Personenfelder wegzulassen ist der normale Weg für die App: Sie hat
das Profil aus Route 7 und muss nichts abtippen lassen. Fehlen sie **und**
stehen sie nicht im Profil, kommt `400 profile_incomplete` — dann führt die
App nach Route 8.

**Antwort 201:**

```json
{
  "booking": {
    "id": "8f14c2b0-91ae-4c77-b1b2-0a3d5e7c9f01",
    "deviceId": "as-585-km-kreiselmaeher",
    "setId": null,
    "deviceName": "AS 585 KM Kreiselmäher",
    "startDate": "2026-09-05",
    "endDate": "2026-09-07",
    "status": "pending",
    "notes": "Hole ich Samstag früh ab.",
    "canCancel": true,
    "pickup": null
  }
}
```

Dasselbe Objekt wie in Route 10.

**Fehler:**

- `409 occupied` — der Zeitraum ist inzwischen belegt. Das ist der
  Normalfall eines Wettlaufs: Zwischen dem Zeichnen des Kalenders und dem
  Tippen kann eine Minute liegen. Die App zeigt das als Hinweis und lädt
  Route 6 neu, nicht als Absturz.
- `400 invalid_period`, `400 bad_request`, `400 profile_incomplete`
- `404 not_found` — Gerät oder Set gibt es nicht
- `401 unauthorized`, `401 token_audience`

**Ein Gerät ohne hinterlegten Eigentümer** nimmt die Buchung an, verschickt
aber keine E-Mail — sie bleibt dann auf `pending` stehen. Das ist ein Zustand
der Daten, kein Fehler der App; sie muss nichts Besonderes tun.

---

### 12. Buchung stornieren

```
POST /api/v1/bookings/{id}/cancel
```

**Token nötig.** Kein Anfragekörper. Erlaubt für den **Mieter** und für den
**Vermieter** des Geräts, solange die Buchung `pending` oder `approved` ist.
Die jeweils andere Seite bekommt eine E-Mail.

**Antwort 200:**

```json
{ "status": "cancelled" }
```

**Fehler:**

- `403 forbidden` — weder Mieter noch Vermieter
- `409 conflict` — schon storniert, abgelehnt, oder eine Set-Buchung
  (Set-Stornierung ist serverseitig noch nicht umgesetzt)
- `404 not_found`, `401 unauthorized`, `401 token_audience`

---

### 13. Buchungen auf meinen Geräten

```
GET /api/v1/owner/bookings
```

**Token nötig**, sinnvoll nur für Vermieter. Wer keine Geräte hat, bekommt
eine leere Liste — kein Fehler.

**Antwort 200:**

```json
{
  "bookings": [
    {
      "id": "8f14c2b0-91ae-4c77-b1b2-0a3d5e7c9f01",
      "deviceId": "as-585-km-kreiselmaeher",
      "deviceName": "AS 585 KM Kreiselmäher",
      "startDate": "2026-09-05",
      "endDate": "2026-09-07",
      "status": "pending",
      "renterName": "Erika Musterfrau",
      "renterPhone": "+49 5069 123456",
      "notes": "Hole ich Samstag früh ab.",
      "canDecide": true,
      "canCancel": true
    }
  ]
}
```

- `renterName` und `renterPhone` stehen hier, weil der Vermieter sie
  braucht, um die Übergabe zu verabreden. Sie gehören in **keine** andere
  Ansicht und in keinen Zwischenspeicher, der länger lebt als die Ansicht.
- `canDecide` sagt, ob die Routen 14 und 15 jetzt gehen (also: `pending`).
  Danach richten sich die Knöpfe.

**Fehler:** `401 unauthorized`, `401 token_audience`.

---

### 14. Buchung bestätigen

```
POST /api/v1/bookings/{id}/approve
```

**Token nötig.** Nur der Vermieter des Geräts. Setzt die Buchung auf
`approved` und schickt dem Mieter eine E-Mail — **darin steht die
Abholadresse**. Ab jetzt liefert Route 10 dem Mieter das Feld `pickup`.

**Antwort 200:**

```json
{ "status": "approved" }
```

**Fehler:** `403 forbidden`, `409 conflict` (nicht mehr `pending`, oder
Set-Buchung), `404 not_found`, `401 unauthorized`, `401 token_audience`.

---

### 15. Buchung ablehnen

```
POST /api/v1/bookings/{id}/reject
```

Wie Route 14, aber Status `rejected` und eine Absage-E-Mail an den Mieter.
Ein Grund wird nicht erfasst.

**Antwort 200:**

```json
{ "status": "rejected" }
```

**Fehler:** wie Route 14.

---

### 16. Meine Geräte

```
GET /api/v1/owner/items
```

**Token nötig.** Die eigenen Geräte des Vermieters — **einschließlich der
abgeschalteten**, die in Route 1 nicht auftauchen.

**Antwort 200:**

```json
{
  "items": [
    {
      "id": "as-585-km-kreiselmaeher",
      "name": "AS 585 KM Kreiselmäher",
      "description": "Kreiselmäher für hohes Gras und Böschungen.",
      "pricePerDay": 25,
      "pricePerWeekend": 40,
      "pricePerWeek": 120,
      "deposit": 100,
      "tags": ["garten", "motorgeraet"],
      "thumbnailUrl": "https://cdn.mieten.xn--rssing-wxa.de/sIgn/…/front.jpg",
      "productUrl": null,
      "webUrl": "https://mieten.xn--rssing-wxa.de/geraete/as-585-km-kreiselmaeher/",
      "active": true
    }
  ]
}
```

Ein Feld mehr als in Route 1: `active`. Steht es auf `false`, ist das Gerät
für andere unsichtbar.

**Anlegen und Ändern von Geräten gibt es in dieser Schnittstelle nicht** —
weder hier noch anderswo. Das läuft über den Chat und die Webfassung der
Mietplattform, und es soll dort bleiben. Die App verweist dafür auf
`https://mieten.xn--rssing-wxa.de`.

**Fehler:** `401 unauthorized`, `401 token_audience`.

---

### 17. Eigene Sperren auflisten

```
GET /api/v1/owner/blocks
```

**Token nötig.** Zeiträume, in denen der Vermieter seine Geräte selbst
braucht oder anderweitig verliehen hat. Für andere sehen sie in Route 6 wie
belegt aus (`status: "blocked"`), ohne Grund und ohne Namen.

**Antwort 200:**

```json
{
  "blocks": [
    {
      "id": "b1f0c9a2-33aa-4d10-8e77-5c2b1a9f0e33",
      "deviceId": "as-585-km-kreiselmaeher",
      "deviceName": "AS 585 KM Kreiselmäher",
      "startDate": "2026-10-01",
      "endDate": "2026-10-08",
      "reason": "Eigener Einsatz"
    }
  ]
}
```

`reason` kann `null` sein.

**Fehler:** `401 unauthorized`, `401 token_audience`.

---

### 18. Zeitraum sperren

```
POST /api/v1/owner/blocks
```

**Token nötig.** Nur für das eigene Gerät.

**Anfrage:**

```json
{
  "deviceId": "as-585-km-kreiselmaeher",
  "startDate": "2026-10-01",
  "endDate": "2026-10-08",
  "reason": "Eigener Einsatz"
}
```

`reason` ist optional. Sets lassen sich nicht sperren, nur einzelne Geräte.

**Antwort 201:**

```json
{
  "block": {
    "id": "b1f0c9a2-33aa-4d10-8e77-5c2b1a9f0e33",
    "deviceId": "as-585-km-kreiselmaeher",
    "deviceName": "AS 585 KM Kreiselmäher",
    "startDate": "2026-10-01",
    "endDate": "2026-10-08",
    "reason": "Eigener Einsatz"
  }
}
```

**Fehler:**

- `403 forbidden` — nicht das eigene Gerät
- `409 occupied` — im Zeitraum liegt schon eine Buchung oder eine Sperre.
  Eine bestehende Buchung wird **nicht** verdrängt; wer das will, storniert
  sie zuerst über Route 12.
- `400 invalid_period`, `404 not_found`, `401 unauthorized`,
  `401 token_audience`

---

### 19. Sperre aufheben

```
DELETE /api/v1/owner/blocks/{id}
```

**Token nötig.** Nur die eigene Sperre.

**Antwort 200:**

```json
{ "deleted": true }
```

**Fehler:** `403 forbidden`, `404 not_found`, `401 unauthorized`,
`401 token_audience`.

---

## 3. Was bewusst fehlt

Damit niemand danach sucht:

- **Geräte anlegen oder ändern.** Bleibt beim Chat und der Webfassung
  (siehe Route 16).
- **Bilder hochladen.** Es gibt `POST /images/upload` in der Mietplattform,
  aber es ist nicht Teil dieses Vertrags. Ob die App Bilder hochladen können
  soll, ist nicht entschieden.
- **Eine Preisberechnung.** Der Server rechnet keinen Gesamtpreis aus. Die
  App zeigt die Tarife (`pricePerDay`, `pricePerWeekend`, `pricePerWeek`,
  `deposit`) und darf sie **nicht** zu einer Summe verrechnen — welcher Tarif
  bei welcher Dauer gilt, ist nirgends festgelegt, und eine erfundene Regel
  in der App wäre genau der Bruch zwischen Web und App, den wir vermeiden.
  Wenn eine Summe gebraucht wird, kommt sie später vom Server.
- **Push-Nachrichten.** Buchungsanfragen und Entscheidungen laufen heute
  ausschließlich über E-Mail. Push ist ein eigenes Paket.
- **Eine Nutzerliste, eine Eigentümersuche, ein Nachrichtenaustausch.** Gibt
  es nicht und wird es nicht geben.

---

## 4. Alte Pfade

Die Mietplattform hatte vor `/api/v1/…` schon einige Routen; die Webfassung
ruft sie auf. Sie bleiben als **308-Weiterleitung** bestehen. Die Apps
benutzen sie nicht — die Liste steht hier nur, damit niemand sie beim
Aufräumen für tot hält:

| alt | neu |
| --- | --- |
| `GET /bookings` | `GET /api/v1/occupancy` |
| `GET /api/my-bookings` | `GET /api/v1/bookings/mine` |
| `GET /api/owner-bookings` | `GET /api/v1/owner/bookings` |
| `POST /api/bookings/{id}/cancel` | `POST /api/v1/bookings/{id}/cancel` |
| `POST /api/bookings/{id}/approve` | `POST /api/v1/bookings/{id}/approve` |
| `POST /api/bookings/{id}/reject` | `POST /api/v1/bookings/{id}/reject` |

`GET /auth/me` bleibt unverändert bestehen — es hängen die Webfassung und der
Chat-Container daran. Für die Apps gilt trotzdem Route 7.

---

## 5. Zum Ausprobieren

Ohne Anmeldung, sobald der PR drüben ausgerollt ist:

```sh
curl -s https://mieten.xn--rssing-wxa.de/api/v1/items | jq '.items[0]'
curl -s 'https://mieten.xn--rssing-wxa.de/api/v1/search?q=rasen&limit=3' | jq
curl -s 'https://mieten.xn--rssing-wxa.de/api/v1/availability?deviceId=rasenwalze&startDate=2026-09-05&endDate=2026-09-07' | jq
curl -s 'https://mieten.xn--rssing-wxa.de/api/v1/occupancy?deviceId=rasenwalze' | jq
```

Mit Token — das Token ist dasselbe, das die App fürs Backend benutzt, aber es
muss die Mietplattform in der `aud` haben (Abschnitt 1):

```sh
TOKEN=…
curl -s -H "Authorization: Bearer $TOKEN" https://mieten.xn--rssing-wxa.de/api/v1/me | jq
curl -s -H "Authorization: Bearer $TOKEN" https://mieten.xn--rssing-wxa.de/api/v1/bookings/mine | jq
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"deviceId":"rasenwalze","startDate":"2026-09-05","endDate":"2026-09-07"}' \
  https://mieten.xn--rssing-wxa.de/api/v1/bookings | jq
```

---

## 6. Wenn hier etwas fehlt

Diese Datei ändert sich nur zusammen mit der Mietplattform. Wer beim Bauen
merkt, dass ein Feld fehlt oder ein Fehlerfall unbeschrieben ist: **nicht
raten und nicht in der App nachbauen**, sondern einen Issue im Repo der
Mietplattform (`levino/mietplattform-roessing`) aufmachen und hier
nachziehen. Eine Regel, die in einer App steht, steht bald in zweien —
verschieden.
