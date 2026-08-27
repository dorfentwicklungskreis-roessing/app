# Datensicherheit („Data safety") — Antworten für die Play Console

Ausgefüllt auf Basis des tatsächlichen Codes, Stand `versionCode 1000107` /
`0.1.7`. Belegstellen sind angegeben, damit sich jede Antwort nachprüfen lässt.

Maßgeblich für die Formulierungen ist die veröffentlichte Erklärung unter
<https://xn--rssing-wxa.de/app/datenschutz/>; dieses Dokument übersetzt sie in
die Formularfelder der Play Console.

Ort in der Play Console: **App-Inhalte → Datensicherheit**.

---

## 1. Übersicht

| Frage | Antwort |
|---|---|
| Erhebt oder teilt die App Nutzerdaten? | **Ja, erheben. Ja, teilen** — seit `0.1.7` geht die Gerätekennung samt Meldungstext an Google (Firebase Cloud Messaging), siehe Abschnitt 2. Seit `0.1.11` kommen **Fehlerberichte** dazu: erhoben, aber **nicht** geteilt — sie gehen nur an den Dorfserver. |
| Werden Daten bei der Übertragung verschlüsselt? | **Ja** — ausschließlich HTTPS (`https://app.xn--rssing-wxa.de`, `https://id.xn--rssing-wxa.de`, `https://tiles.openfreemap.org`) |
| Können Nutzer die Löschung ihrer Daten beantragen? | **Ja** — siehe Abschnitt 4 |
| Unabhängige Sicherheitsprüfung | **Nein** |
| Play-Richtlinie „Familien" | **Nein**, App richtet sich nicht an Kinder |

„Teilen" im Sinne von Play heißt: Weitergabe an ein *anderes Unternehmen*.
Dorfserver und Rössing-ID betreibt der Dorfentwicklungskreis selbst — das ist
keine Weitergabe. Nichts wird verkauft, es gibt keine Werbepartner. **Eine
Ausnahme gibt es seit `0.1.7`:** Wer Benachrichtigungen erlaubt, dessen
Gerätekennung und der Text der Anfrage laufen über Google (Firebase Cloud
Messaging) — das ist eine Weitergabe und unten als solche deklariert.

---

## 2. Erhobene Datentypen

### Personenbezogene Daten → Name

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht (die App ist ohne Anmeldung nicht nutzbar)
- **Zwecke:** App-Funktionalität, Kontoverwaltung
- **Nur kurzzeitig verarbeitet:** nein — der Name wird zusammen mit jeder
  Erledigung dauerhaft gespeichert
- **Zusätzlich seit der Profilverwaltung:** Anzeigename und Nickname lassen
  sich im Profil frei setzen (Nickname erscheint in Rangliste und Meldungen).
  Beide sind standardmäßig für angemeldete Dorfbewohner sichtbar — ohne sie
  funktionieren Rangliste und Erledigungshistorie nicht.
- **Beleg:** `AuthManager.kt` fordert die Scopes `openid profile email
  offline_access` an; `backend/internal/auth/auth.go` liest den Claim `name`;
  `backend/internal/db/db.go` speichert `completions.user_name`;
  Profiltabelle in `backend/internal/db/`

### Personenbezogene Daten → E-Mail-Adresse

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht (aus der Anmeldung) bzw. optional (im Profil)
- **Zwecke:** Kontoverwaltung, App-Funktionalität
- **Nur kurzzeitig verarbeitet:** nein — seit der Profilverwaltung kann die
  Adresse **dauerhaft gespeichert** werden: Sie ist im Profil mit dem Wert aus
  der Rössing-ID vorbelegt und überschreibbar. Ohne Profil steht sie nur im
  Token und wird nicht abgelegt.
- **Sichtbarkeit:** standardmäßig **nur für Verwaltende**; erst ein bewusst
  umgelegter Schalter gibt sie für andere angemeldete Dorfbewohner frei
- **Beleg:** `zitadelClaims.Email` in `backend/internal/auth/auth.go`,
  Profiltabelle in `backend/internal/db/`, `PUT /api/v1/me/profile`

### Personenbezogene Daten → Nutzer-IDs

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht
- **Zwecke:** App-Funktionalität, Kontoverwaltung
- **Nur kurzzeitig verarbeitet:** nein
- **Was genau:** die Subject-Kennung (`sub`) der Rössing-ID, gespeichert als
  `completions.user_sub`

### Personenbezogene Daten → Telefonnummer

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** **optional** — freiwillige Angabe im Profil, die
  App funktioniert vollständig ohne sie
- **Zwecke:** App-Funktionalität (nachbarschaftliche Erreichbarkeit)
- **Nur kurzzeitig verarbeitet:** nein
- **Sichtbarkeit:** standardmäßig **nur für Verwaltende**; die Freigabe für
  andere angemeldete Dorfbewohner erfordert einen bewusst umgelegten Schalter.
  Nicht freigegebene Felder verlassen den Server nicht.
- **Rechtsgrundlage:** Einwilligung, jederzeit durch Zurückstellen widerrufbar
- **Beleg:** Profiltabelle in `backend/internal/db/`, `PUT /api/v1/me/profile`,
  `GET /api/v1/members` (liefert je Eintrag nur die freigegebenen Felder)

### App-Aktivitäten → Sonstige Aktionen

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht
- **Zwecke:** App-Funktionalität
- **Nur kurzzeitig verarbeitet:** nein
- **Was genau:** Erledigungsmeldungen — welche Pflegeaufgabe, Zeitpunkt
  (`done_at`), gegebenenfalls Litermenge. Für alle angemeldeten Dorfbewohner
  sichtbar (Historie eines Ortes, Rangliste).
  Seit `0.1.7` fällt hierunter zusätzlich die **Vergabe**: der freiwillige
  Eintrag als Helfer:in an einem Ort (Kennung, Ort, ggf. Aufgabenart,
  Zeitpunkt), die daraus entstehenden Anfragen (Kennung, Anlass, Zeitpunkt, ob
  gelesen) und die Zusagen. Eine Zusage ist mit Namen und Frist für die
  übrigen Eingetragenen sichtbar — sonst gießen zwei denselben Kasten. Keine
  neue Play-Kategorie: Es kommen nur Kennung, Ort und Zeitpunkte hinzu.
- **Beleg:** `POST /api/v1/tasks/{id}/completions` in
  `backend/internal/api/api.go`, Tabelle `completions` in `db.go`;
  `care_signups`, `care_assignments`, `care_notifications` und
  `backend/internal/vergabe/vergabe.go`

### App-Aktivitäten → Sonstige nutzergenerierte Inhalte (seit 0.1.7)

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** **optional** — die Kachel „Idee vorschlagen" ist
  ein Angebot; die App funktioniert vollständig, ohne sie je zu öffnen
- **Zwecke:** App-Funktionalität (Weiterentwicklung nach Wünschen aus dem Dorf)
- **Nur kurzzeitig verarbeitet:** nein — die Einreichung bleibt in der
  Ideen-Tabelle stehen, bis die Verwaltung sie löscht
- **Was genau:** ein frei getippter Wunsch (5–2000 Zeichen), dazu freiwillig
  Name und E-Mail-Adresse (aus dem Profil vorbelegt, überschreib- und
  leerbar). Der Text ist **nicht öffentlich**: Er ist nur für die Verwaltung
  sichtbar und erscheint an keiner Stelle der App für andere Nutzer.
- **Beleg:** `android/app/src/main/java/de/roessing/app/ui/IdeenScreen.kt`,
  `data/Ideen.kt`, `POST /api/v1/ideen` in `backend/internal/api/`

### Geräte- oder andere Kennungen → Gerätekennung (seit 0.1.7)

- **Erhoben:** ja · **Geteilt:** **ja** (an Google/Firebase Cloud Messaging)
- **Pflicht oder optional:** **optional** — die Kennung entsteht erst, wenn
  Benachrichtigungen wirksam erlaubt sind; wer sie ablehnt, sieht die Anfragen
  wie bisher beim Öffnen der App. Seit `0.1.8` deckt sich das mit dem Code
  (siehe „So hängt das im Code zusammen" unten).
- **Zwecke:** App-Funktionalität (Benachrichtigung, dass jemand an der Reihe
  ist)
- **Nur kurzzeitig verarbeitet:** nein — die Kennung steht bis zum Abmelden
  in der Tabelle `push_devices`
- **Was genau:** die von Firebase vergebene Kennung der App-Installation
  (kein Werbe-Identifikator, keine Hardware-Kennung, kein IMEI). Sie wird in
  keiner Antwort der API ausgeliefert und lässt sich nur von der eigenen
  Person abmelden. Meldet Google sie als ungültig (`UNREGISTERED`,
  `INVALID_ARGUMENT`), löscht der Server sie von sich aus.
- **Was an Google geht** — vollständige Liste, abgelesen an `Nutzlast()` in
  `backend/internal/push/fcm.go`:
  - im `notification`-Teil: **Titel und Text** der Meldung, z.B. „Gießen an
    ‚Kirchplatz' ist dran" / „Du bist als Nächste(r) an der Reihe: Gießen an
    ‚Kirchplatz'. Wenn du zusagst, hast du 24 Stunden Zeit."
  - im `data`-Teil zusätzlich: **Ortsname** (`placeName`), **Aufgabenname**
    (`taskName`), die internen Nummern von **Ort, Aufgabe, Vorgang und
    Benachrichtigung** (`placeId`, `taskId`, `assignmentId`,
    `notificationId`), die **Art** der Nachricht (`kind`, `taskKind`), der
    Ablaufzeitpunkt einer Anfrage (`expiresAt`) sowie Titel und Text noch
    einmal als Zeichenkette. Der `data`-Teil sagt der App, wohin ein
    Fingertipp führen soll.
  - im `android`-Teil: der Kanal (`anfragen`/`hinweise`) und eine Kennzeichnung
    je Vorgang, damit sich Meldungen zum selben Vorgang gegenseitig ersetzen.
  - **Namen anderer Personen stehen nie in einer Push-Nachricht.** Die Texte
    entstehen in `vergabe.texte()` aus Ortsname, Aufgabenname und Frist; ein
    Personenname kommt dort nicht vor — auch nicht in der Meldung „schon
    erledigt".
- **Wann etwas hinausgeht:** wenn jemand an der Reihe ist, beim **Rundruf** am
  Ende der Warteschlange, bei abgelaufener oder von der Verwaltung
  aufgehobener Zusage und wenn eine Aufgabe schon erledigt oder nicht mehr
  nötig ist. Zwischen **21 und 7 Uhr** wird nichts zugestellt
  (`model.AssignmentRules.NextDelivery`).
- **Beleg:** `backend/internal/push/fcm.go` (Nutzlast), `POST/DELETE
  /api/v1/me/devices` in `backend/internal/api/geraete.go`, Tabelle
  `push_devices` in `backend/internal/db/db.go`,
  `android/app/src/main/java/de/roessing/app/push/`

#### So hängt das im Code zusammen

Die Angabe „optional" setzt voraus, dass ohne Erlaubnis keine Kennung
entsteht. **Bis einschließlich `0.1.7` stimmte das nicht:**
`ui/HomeScreen.kt` rief `Geraeteanmeldung.anmelden(context)` in einem
`LaunchedEffect(Unit)` beim Betreten der Startseite auf — ohne vorher die
Erlaubnis zu prüfen. Die Frage nach `POST_NOTIFICATIONS` kommt erst später
und nur, wenn sich jemand als Helfer:in eingetragen hat. Die Folge: Firebase
vergab die Kennung und der Server merkte sie sich auch bei jemandem, der die
Benachrichtigungen abgelehnt hatte.

**Seit `0.1.8` folgt die Kennung der Erlaubnis** — nachzulesen in
`android/app/src/main/java/de/roessing/app/push/Geraeteabgleich.kt`:

- Maßgeblich ist der *wirksame* Zustand: vor Android 13
  `areNotificationsEnabled()`, ab Android 13 zusätzlich die erteilte
  Berechtigung `POST_NOTIFICATIONS`. In den Einstellungen abgedreht heißt
  abgedreht.
- Ohne Erlaubnis wird Firebase **gar nicht erst nach einer Kennung gefragt** —
  das Fragen selbst legt sie an.
- **Auch Firebase selbst bleibt still.** Ohne Zutun meldet das FCM-SDK das
  Gerät beim ersten Start von sich aus bei Google an (Auto-Init) und legt
  dabei eine Installations-Kennung an. `firebase_messaging_auto_init_enabled`
  und `firebase_data_collection_default_enabled` stehen im Manifest deshalb
  auf `false`; scharf gestellt wird ausdrücklich erst bei erteilter Erlaubnis,
  beim Entzug geht es wieder aus.
- Wird die Erlaubnis später entzogen, löscht die App die Kennung beim nächsten
  Start bzw. bei der Rückkehr in den Vordergrund (`DELETE /api/v1/me/devices`)
  und wirft sie danach auch bei Firebase weg. Dasselbe beim Abmelden aus der
  App.
- Auch die Erneuerung der Kennung (`onNewToken`) läuft nur bei erteilter
  Erlaubnis.

**Bestandsgeräte:** Wer von 0.1.7 aktualisiert, hat eine Kennung im Backend
liegen — die Merkung, ob angemeldet wurde, gibt es ja erst seit 0.1.8. Ein
aktualisiertes Gerät gilt deshalb zunächst als angemeldet und wird beim ersten
Start ohne Erlaubnis aufgeräumt (`Anmeldevermutung`). Bei einer
Neuinstallation gilt das ausdrücklich nicht — sonst fragte die App Firebase
nach einer Kennung, nur um sie zu löschen.

Belegt durch `android/app/src/test/java/de/roessing/app/GeraeteabgleichTest.kt`
sowie den Instrumentierungstest
`androidTest/.../GeraetekennungE2eTest.kt`, der auf den Emulatoren API 28 und
35 gegen ein echtes Backend nachweist, dass bei abgeschalteten
Benachrichtigungen keine Kennung dort ankommt.

In der Play Console darf *Geräte-ID* deshalb als **optional** angekreuzt
werden.

### App-Info und Leistung → Absturzprotokolle (seit 0.1.11)

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** **optional** — nichts geht von selbst hinaus. Die
  App zeigt den Fehler an, und erst ein Fingertipp auf „Bericht schicken“
  schickt ihn ab. Wer den Hinweis wegtippt, hat nichts gemeldet.
- **Zwecke:** App-Funktionalität, Fehlerbehebung („Diagnostics“)
- **Nur kurzzeitig verarbeitet:** nein — der Bericht bleibt in der Tabelle
  `error_reports` stehen, bis die Verwaltung ihn löscht
- **Was genau:** die Art der Störung (`crash`/`network`/`server`/
  `unexpected`), die **Meldung, die auf dem Bildschirm stand**, technische
  Angaben (HTTP-Status und Pfad; bei einem Absturz die Aufrufliste, höchstens
  4000 Zeichen), der Bereich der App in Alltagssprache, Plattform, App- und
  Systemversion, die **Gerätebezeichnung** („Google Pixel 6“) und der
  Zeitpunkt. **Kein** Anfrage- oder Antwortrumpf, kein Protokoll, kein
  Bildschirmfoto, kein Standort, **keine Gerätekennung** (weder die
  FCM-Kennung noch eine Hardware-Kennung).
- **Kein Fremddienst:** kein Crashlytics, kein Sentry, kein Analytics-SDK. Der
  Bericht geht ausschließlich an den Dorfserver, den der
  Dorfentwicklungskreis selbst betreibt — deshalb *geteilt: nein*.
- **Beleg:** `android/app/src/main/java/de/roessing/app/errors/`,
  `data/ErrorReports.kt`, `ui/ErrorReportBanner.kt`,
  `POST /api/v1/error-reports` in `backend/internal/api/error_reports.go`,
  Tabelle `error_reports` in `backend/internal/db/error_reports.go`,
  `backend/SICHERHEIT.md`, Abschnitt „Fehlerberichte aus den Apps“

### App-Aktivitäten → Sonstige nutzergenerierte Inhalte (Ergänzung 0.1.11)

Zu den Ideen (oben) kommt die **freiwillige Ergänzung** an einem
Fehlerbericht: ein frei getippter Satz („Was hast du gerade gemacht?“, bis
2000 Zeichen). Er ist freiwillig — ein Fingertipp ohne Text hilft genauso —,
nicht öffentlich und nur für die Verwaltung sichtbar.

### Personenbezogene Daten → Name und Nutzer-IDs (Ergänzung 0.1.11)

Ist beim Absenden eines Fehlerberichts jemand angemeldet, werden **Kennung
(`sub`) und Name aus der Rössing-ID** am Bericht gespeichert — damit der
Dorfentwicklungskreis nachfragen kann. Beides kommt aus dem Token, nicht aus
der App. Ohne Anmeldung ist der Bericht anonym; genau das ist der Fall, auf
den es ankommt, wenn das Anmelden selbst klemmt.

### Nachrichten (Chat, E-Mail, SMS)

- **Erhoben:** **nein**
- **Begründung:** Es gibt keinen Chat, keine Kommentare und keinen Weg, einer
  anderen Person Text zu schicken. Anfragen und Hinweise erzeugt der Server
  aus festen Bausteinen. Der freie Ideen-Text ist eine Rückmeldung an den
  Betreiber und oben unter *sonstige nutzergenerierte Inhalte* deklariert.
- **Randnotiz `note`:** Das Datenmodell kennt ein Freitextfeld an einer
  Erledigung; die **Android-App füllt es weiterhin nicht** — es gibt in der
  Oberfläche kein Eingabefeld dafür (nur Web-Verwaltung und MCP setzen es).
- **Achtung bei Änderungen:** Sobald die App ein Notizfeld an der Erledigung
  bekommt, gehört es ebenfalls unter *sonstige nutzergenerierte Inhalte*.

---

## 3. Ausdrücklich NICHT erhoben

| Datentyp | Status | Begründung |
|---|---|---|
| Standort (genau/ungefähr) | **nein — aber erklärungsbedürftig, siehe unten** | Die App fragt `ACCESS_FINE_LOCATION`/`ACCESS_COARSE_LOCATION` ab, benutzt die Position aber ausschließlich auf dem Gerät. |
| Finanzdaten | nein | keine Zahlungen, keine In-App-Käufe |
| Gesundheits-/Fitnessdaten | nein | — |
| Kontakte, Kalender, SMS, Anrufliste | nein | keine entsprechenden Berechtigungen |
| Fotos, Videos, Audio, Dateien | nein | keine Medienauswahl in der App |
| Absturzberichte, Diagnosen, Leistungsdaten | **ja, seit 0.1.11 — optional** | Weiterhin kein Crashlytics und kein Analytics-SDK: `firebase-messaging` ist **nur** für Benachrichtigungen eingebunden, die Analyse-Bibliothek fehlt bewusst. Neu ist der **von Hand abgeschickte Fehlerbericht** — siehe Abschnitt 2. Nichts davon geht ohne Knopfdruck hinaus, und nichts geht an einen Dritten. Leistungsdaten werden weiterhin nicht erhoben. |
| Werbe-ID | nein | keine `play-services-ads-identifier`-Abhängigkeit |
| Geräte-IDs | **ja, seit 0.1.7** | Kennung der App-Installation für Benachrichtigungen — siehe Abschnitt 2 |
| Kaufhistorie, Suchverlauf, installierte Apps | nein | — |

### Sonderfall Standort — warum „nicht erhoben" trotzdem stimmt

Die App hat seit `de.roessing.app` Stand August 2026 eine Standortfunktion:
Kartenausschnitt auf die eigene Position, Entfernung zu jedem Ort, Sortierung
der Liste nach Nähe.

Play definiert „erhoben" als **Übertragung vom Gerät weg**. Das passiert hier
nicht:

- `android/app/src/main/java/de/roessing/app/data/DeviceLocation.kt` holt die
  Position über den `LocationManager` der Plattform — zuerst die zuletzt
  bekannte, sonst **ein einzelner Fix** mit 8 s Zeitlimit. Kein Dauer-Tracking.
- Die Position landet in `PlacesViewModel.userLocation` und wird dort nur für
  Sortierung und Entfernungsanzeige benutzt.
- `DorfApi` (`data/Api.kt`) hat **keinen Endpunkt mit Koordinaten**;
  `CompletionInput` besteht aus `liters` und `note`. Es gibt keinen Pfad, auf
  dem die Position das Gerät verlässt.
- Kein Hintergrundstandort: `ACCESS_BACKGROUND_LOCATION` steht nicht im
  Manifest, es gibt keinen Vordergrunddienst.

Daraus folgt für die Play Console:

- **Datensicherheit:** Standort **nicht** als erhobenen Datentyp eintragen.
- **Keine Erklärung für sensible Berechtigungen nötig.** Ein Formular verlangt
  Google nur für **Hintergrund**standort — den nutzt die App nicht.
- In der Store-Beschreibung ist der Zweck genannt („Position bleibt auf dem
  Gerät"). Das sollte so bleiben: Google prüft die Angaben gegen das
  beobachtete Verhalten der App, und eine erklärte, im Vordergrund abgefragte
  Ortung ist unproblematisch — eine unerklärte fällt auf.

**Randnotiz IP-Adresse:** Beim Laden der Kartenkacheln von
`tiles.openfreemap.org` (MapLibre) und beim Aufruf des Dorfservers wird
zwangsläufig die IP-Adresse übertragen. Play kennt für IP-Adressen keinen
eigenen Datentyp und verlangt dafür keine Angabe; in der Datenschutzerklärung
ist es trotzdem erwähnt. Der Dorfserver protokolliert nur Methode, Pfad und
Dauer eines Requests, **keine IP** (`internal/api/api.go`, `slog.Info("request",
…)`). — **offen:** Loggt der Ingress/Reverse-Proxy vor dem Backend
IP-Adressen? Falls ja, gehört das in die Datenschutzerklärung.

---

## 4. Löschung von Daten

Play fragt zweistufig:

1. **„Können Nutzer die Löschung ihrer Daten beantragen?" → Ja.**
2. Zusätzlich verlangt Play für Apps mit Konto eine **öffentlich erreichbare
   Seite zur Löschung von Konto und Daten** (Play Console → *App-Inhalte →
   Datenlöschung*). Diese Seite steht:
   <https://xn--rssing-wxa.de/app/daten-loeschen/>

Was heute schon geht:

- Eine einzelne Meldung nimmt die meldende Person selbst zurück
  (`DELETE /api/v1/completions/{id}`, erlaubt für Melder und Admins).
- Vollständige Löschung von Konto und Meldungen auf Anfrage beim
  Dorfentwicklungskreis (Konto in Zitadel, Meldungen in der Datenbank).
- Abmelden in der App löscht alle lokal gespeicherten Token
  (`AuthManager.logout()` leert den DataStore).

---

## 5. Offene Punkte

- [x] URL der Datenschutzerklärung auf roessing.de —
      <https://xn--rssing-wxa.de/app/datenschutz/>
- [x] URL der Seite zur Konto-/Datenlöschung auf roessing.de —
      <https://xn--rssing-wxa.de/app/daten-loeschen/>
- [ ] **Gerätekennung ohne Erlaubnis** (siehe „Abweichung im Code"): entweder
      die Anmeldung hinter die Erlaubnis legen oder in der Console „Pflicht"
      ankreuzen
- [ ] Loggt der Reverse-Proxy IP-Adressen? Wenn ja: in der Erklärung nennen
- [ ] **Fehlerberichte in der Play Console eintragen** (0.1.11): *App info and
      performance → Crash logs* und *Diagnostics* als **optional erhoben, nicht
      geteilt**, Zweck *App functionality*; *Analytics* bleibt ausdrücklich aus
- [ ] Bei jeder neuen Version prüfen, ob ein neues Feld (z.B. Notiz, Foto,
      Standort) die Antworten oben ändert
