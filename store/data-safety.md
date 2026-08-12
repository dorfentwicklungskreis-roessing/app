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
- **Beleg:** `AuthManager.kt` fordert die Scopes `openid profile email
  offline_access` an; `backend/internal/auth/auth.go` liest den Claim `name`;
  `backend/internal/db/db.go` speichert `completions.user_name`

### Personenbezogene Daten → E-Mail-Adresse

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht
- **Zwecke:** Kontoverwaltung
- **Nur kurzzeitig verarbeitet:** ja — die Adresse steht im Token und wird von
  `GET /api/v1/me` zurückgegeben, aber **nirgends in der Datenbank abgelegt**
  (das Schema in `db.go` enthält keine E-Mail-Spalte)
- **Beleg:** `zitadelClaims.Email` in `backend/internal/auth/auth.go`,
  `MeDto.email` in `android/.../data/Model.kt`

### Personenbezogene Daten → Nutzer-IDs

- **Erhoben:** ja · **Geteilt:** nein
- **Pflicht oder optional:** Pflicht
- **Zwecke:** App-Funktionalität, Kontoverwaltung
- **Nur kurzzeitig verarbeitet:** nein
- **Was genau:** die Subject-Kennung (`sub`) der Rössing-ID, gespeichert als
  `completions.user_sub`

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
| Standort (genau/ungefähr) | **nein** | Das Manifest enthält keine der `ACCESS_*_LOCATION`-Berechtigungen. `MapScreen.kt` zeigt nur die festen Koordinaten der Pflege-Orte, es gibt keine `LocationComponent`, keine Ortungsabfrage und keine Übertragung einer Position ans Backend. |
| Finanzdaten | nein | keine Zahlungen, keine In-App-Käufe |
| Gesundheits-/Fitnessdaten | nein | — |
| Kontakte, Kalender, SMS, Anrufliste | nein | keine entsprechenden Berechtigungen |
| Fotos, Videos, Audio, Dateien | nein | keine Medienauswahl in der App |
| Absturzberichte, Diagnosen, Leistungsdaten | nein | kein Crashlytics, kein Analytics-SDK; das Firebase-SDK ist **nicht** eingebunden — Firebase wird nur außerhalb der App zur Verteilung von Testversionen benutzt |
| Werbe-ID / Geräte-IDs | nein | keine `play-services-ads-identifier`-Abhängigkeit |
| Kaufhistorie, Suchverlauf, installierte Apps | nein | — |

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
