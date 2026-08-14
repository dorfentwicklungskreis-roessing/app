# Datensicherheit („Data safety") — Antworten für die Play Console

Ausgefüllt auf Basis des tatsächlichen Codes, Stand `versionCode 2` / `0.1.1`.
Belegstellen sind angegeben, damit sich jede Antwort nachprüfen lässt.

Ort in der Play Console: **App-Inhalte → Datensicherheit**.

---

## 1. Übersicht

| Frage | Antwort |
|---|---|
| Erhebt oder teilt die App Nutzerdaten? | **Ja, erheben. Nein, nicht teilen.** |
| Werden Daten bei der Übertragung verschlüsselt? | **Ja** — ausschließlich HTTPS (`https://app.xn--rssing-wxa.de`, `https://id.xn--rssing-wxa.de`, `https://tiles.openfreemap.org`) |
| Können Nutzer die Löschung ihrer Daten beantragen? | **Ja** — siehe Abschnitt 4 |
| Unabhängige Sicherheitsprüfung | **Nein** |
| Play-Richtlinie „Familien" | **Nein**, App richtet sich nicht an Kinder |

„Teilen" im Sinne von Play heißt: Weitergabe an ein *anderes Unternehmen*.
Dorfserver und Rössing-ID betreibt der Dorfentwicklungskreis selbst — das ist
keine Weitergabe. Nichts wird verkauft, es gibt keine Werbepartner.

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
- **Beleg:** `POST /api/v1/tasks/{id}/completions` in
  `backend/internal/api/api.go`, Tabelle `completions` in `db.go`

### Nachrichten / sonstige nutzergenerierte Inhalte

- **Erhoben:** derzeit **nein**
- **Begründung:** Das Datenmodell kennt ein Freitextfeld `note` an einer
  Erledigung, die **Android-App füllt es nicht** — es gibt in der Oberfläche
  kein Eingabefeld dafür (nur Web-Verwaltung und MCP setzen es).
- **Achtung bei Änderungen:** Sobald die App ein Notizfeld bekommt, muss hier
  *App-Aktivitäten → sonstige nutzergenerierte Inhalte* nachgetragen werden.

---

## 3. Ausdrücklich NICHT erhoben

| Datentyp | Status | Begründung |
|---|---|---|
| Standort (genau/ungefähr) | **nein — aber erklärungsbedürftig, siehe unten** | Die App fragt `ACCESS_FINE_LOCATION`/`ACCESS_COARSE_LOCATION` ab, benutzt die Position aber ausschließlich auf dem Gerät. |
| Finanzdaten | nein | keine Zahlungen, keine In-App-Käufe |
| Gesundheits-/Fitnessdaten | nein | — |
| Kontakte, Kalender, SMS, Anrufliste | nein | keine entsprechenden Berechtigungen |
| Fotos, Videos, Audio, Dateien | nein | keine Medienauswahl in der App |
| Absturzberichte, Diagnosen, Leistungsdaten | nein | kein Crashlytics, kein Analytics-SDK; das Firebase-SDK ist **nicht** eingebunden — Firebase wird nur außerhalb der App zur Verteilung von Testversionen benutzt |
| Werbe-ID / Geräte-IDs | nein | keine `play-services-ads-identifier`-Abhängigkeit |
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
   Datenlöschung*). Diese Seite gibt es noch nicht.
   **offen: URL auf roessing.de anlegen und eintragen.**

Was heute schon geht:

- Eine einzelne Meldung nimmt die meldende Person selbst zurück
  (`DELETE /api/v1/completions/{id}`, erlaubt für Melder und Admins).
- Vollständige Löschung von Konto und Meldungen auf Anfrage beim
  Dorfentwicklungskreis (Konto in Zitadel, Meldungen in der Datenbank).
- Abmelden in der App löscht alle lokal gespeicherten Token
  (`AuthManager.logout()` leert den DataStore).

---

## 5. Offene Punkte

- [ ] URL der Datenschutzerklärung auf roessing.de
- [ ] URL der Seite zur Konto-/Datenlöschung auf roessing.de
- [ ] Loggt der Reverse-Proxy IP-Adressen? Wenn ja: in der Erklärung nennen
- [ ] Bei jeder neuen Version prüfen, ob ein neues Feld (z.B. Notiz, Foto,
      Standort) die Antworten oben ändert
