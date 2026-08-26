# „App Privacy" — Antworten für App Store Connect (iOS)

Ausgefüllt auf Basis des tatsächlichen iOS-Codes, Stand `MARKETING_VERSION
0.1.0` (`ios/project.yml`). Zu jeder Antwort steht die Fundstelle, damit sich
nachprüfen lässt, worauf sie beruht — genau wie in `store/data-safety.md` für
den Play Store.

Maßgeblich für die Formulierungen ist die veröffentlichte Erklärung unter
<https://xn--rssing-wxa.de/app/datenschutz/>; dieses Dokument übersetzt sie in
die Formularfelder von App Store Connect.

**Ort in App Store Connect:** *My Apps → Rössing → App Privacy → Edit*
(deutsch: *Meine Apps → Rössing → App-Datenschutz → Bearbeiten*).

> **Der große Unterschied zur Android-Fassung:** Die iOS-App hat **kein Push**
> und damit **keine Gerätekennung**, die an Google ginge. Es gibt in
> `ios/project.yml` genau ein Paket — MapLibre für die Karte — und im ganzen
> Verzeichnis `ios/` keinen Aufruf von `UNUserNotificationCenter`,
> `ASIdentifierManager`, `AppTrackingTransparency` oder irgendeiner
> Analyse-Bibliothek (nachgeprüft mit `grep -rn` über `ios/`). Die Zeilen
> „Geräte-ID" und „Weitergabe an Dritte" aus `data-safety.md` entfallen hier
> vollständig.

---

## 1. Die drei Fragen, die Apple je Datenart stellt

1. **Wird die Datenart erhoben?** Apples Definition: Daten verlassen das Gerät
   und sind für uns länger zugänglich, als die Beantwortung der Anfrage in
   Echtzeit dauert. Was nur auf dem Gerät bleibt, ist **nicht** erhoben.
2. **Ist sie mit der Identität verknüpft** („Linked to You")?
3. **Wird sie zum Tracking benutzt** („Used to Track You")?

**Antwort auf Frage 3 ist überall: Nein.** Tracking im Sinne von Apple heißt,
Daten dieser App mit Daten anderer Firmen zusammenzuführen, für Werbung oder
Werbemessung, oder sie an einen Datenmakler zu geben. Nichts davon passiert:
Es gibt kein Werbe-SDK, keine Analyse, keinen Datenmakler und keine
Werbekennung. Daraus folgt zugleich: **Kein App-Tracking-Transparency-Dialog
und kein `NSUserTrackingUsageDescription`** — der Schlüssel steht bewusst
nicht in `ios/Dorf/Info.plist`.

---

## 2. Erhobene Datenarten

### Contact Info → Name

- **Erhoben:** ja · **Mit Identität verknüpft:** ja · **Tracking:** nein
- **Zwecke (Purposes):** *App Functionality* — sonst nichts. Kein Analytics,
  keine Personalisierung, keine Werbung.
- **Was genau:** Der Name aus der Rössing-ID; dazu der frei setzbare
  Anzeigename und Spitzname aus dem Profil. Der Name steht an jeder
  Erledigungsmeldung und damit in Historie und Rangliste.
- **Beleg:** `ios/Dorf/Anmeldung/Anmeldung.swift` fordert die Scopes
  `openid profile email offline_access` an (`ANMELDE_SCOPES`);
  `ios/Dorf/Daten/Modelle.swift` — `Ich.name`, `Profil.displayName`,
  `Profil.nickname`, `Erledigung.userName`; serverseitig
  `backend/internal/auth/auth.go` (Claim `name`) und die Spalte
  `completions.user_name` in `backend/internal/db/db.go`.

### Contact Info → Email Address

- **Erhoben:** ja · **Mit Identität verknüpft:** ja · **Tracking:** nein
- **Zwecke:** *App Functionality*
- **Was genau:** Die Adresse kommt aus der Rössing-ID und ist im Profil
  vorbelegt; wer das Profil speichert, legt sie dauerhaft im Dorfserver ab.
  Sie ist außerdem ein freiwilliges Feld beim Einreichen einer Idee.
- **Sichtbarkeit:** **von Haus aus nur für Verwaltende.** Erst ein bewusst
  umgelegter Schalter gibt sie für andere angemeldete Dorfbewohner frei;
  nicht freigegebene Felder verlassen den Server gar nicht.
- **Beleg:** `Modelle.swift` — `Ich.email`, `Profil.email`,
  `Sichtbarkeit.email = Sichtbarkeit.verwaltung` (Vorbelegung),
  `IdeeEingabe.email`; `ios/Dorf/Daten/DorfApi.swift` —
  `profilSpeichern` (`PUT /api/v1/me/profile`), `ideeEinreichen`
  (`POST /api/v1/ideen`); serverseitig `backend/SICHERHEIT.md`, Abschnitt
  „Profile: freiwillige Kontaktdaten" und `TestMitgliederSehenNurFreigegebenes`.

### Contact Info → Phone Number

- **Erhoben:** ja · **Mit Identität verknüpft:** ja · **Tracking:** nein
- **Zwecke:** *App Functionality* (nachbarschaftliche Erreichbarkeit)
- **Freiwillig:** ja — die App funktioniert vollständig ohne die Nummer. Apple
  kennt kein Feld „optional"; das gehört deshalb in die Store-Beschreibung
  („ganz freiwillig") und in die Datenschutzerklärung, nicht ins Formular.
- **Sichtbarkeit:** wie E-Mail — Vorbelegung „nur Verwaltung".
- **Beleg:** `Modelle.swift` — `Profil.phone`, `ProfilEingabe.phone`,
  `Sichtbarkeit.phone = Sichtbarkeit.verwaltung`.

### Identifiers → User ID

- **Erhoben:** ja · **Mit Identität verknüpft:** ja · **Tracking:** nein
- **Zwecke:** *App Functionality*
- **Was genau:** die Subject-Kennung (`sub`) der Rössing-ID. Sie hängt an
  jeder Erledigung und ist der Schlüssel des Profils.
- **Beleg:** `Modelle.swift` — `Ich.sub`, `Erledigung.userSub`,
  `Profil.userSub`; Spalte `completions.user_sub` in `backend/internal/db/db.go`.

### User Content → Other User Content

- **Erhoben:** ja · **Mit Identität verknüpft:** ja · **Tracking:** nein
- **Zwecke:** *App Functionality*
- **Was genau:** zweierlei.
  1. **Erledigungsmeldungen:** welche Pflegeaufgabe, Zeitpunkt, gegebenenfalls
     die Litermenge. Für alle angemeldeten Dorfbewohner sichtbar — das ist der
     Zweck der gemeinsamen Übersicht.
  2. **Ideen:** ein frei getippter Wunsch (5–2000 Zeichen), freiwillig mit
     Name und E-Mail. Der Text ist **nicht öffentlich**: nur die Verwaltung
     sieht ihn, in der App erscheint er für niemanden sonst.
- **Beleg:** `DorfApi.swift` — `melden(aufgabe:liter:notiz:)`
  (`POST /api/v1/tasks/{id}/completions`) mit `ErledigungEingabe(liters, note)`
  und `ideeEinreichen` (`POST /api/v1/ideen`) mit
  `IdeeEingabe(wunsch, name, email)`; Tabellen `completions` und `ideen` in
  `backend/internal/db/`, dazu `backend/SICHERHEIT.md`, Abschnitt
  „Ideen-Sammlung".
- **Randnotiz:** `ErledigungEingabe.note` ist ein Freitextfeld des Backends.
  Die iOS-App **füllt es nicht** — `melden(...)` hat den Vorgabewert `""` und
  in der Oberfläche gibt es kein Eingabefeld dafür. Bekommt die App eines,
  ändert sich an dieser Zeile nichts (es bleibt *Other User Content*), wohl
  aber an der Altersfreigabe-Frage nach Nutzer-zu-Nutzer-Text.

---

## 3. Ausdrücklich NICHT erhoben („Data Not Collected")

| Apple-Datenart | Antwort | Begründung mit Fundstelle |
|---|---|---|
| **Location** (Precise / Coarse) | **nein** | Siehe Sonderfall unten. |
| Contact Info → Physical Address, Other | nein | Kein Feld dafür — `Profil` in `Modelle.swift` kennt nur `displayName`, `nickname`, `phone`, `email`, `note`. |
| Health & Fitness | nein | — |
| Financial Info, Purchases | nein | Keine Zahlungen, kein StoreKit im Projekt (`ios/project.yml` hat als einziges Paket MapLibre). |
| Sensitive Info | nein | Keine Angaben zu Herkunft, Gesundheit, Religion, Orientierung o.ä. |
| Contacts | nein | Kein `Contacts`-Framework, kein `NSContactsUsageDescription` in `ios/Dorf/Info.plist`. |
| User Content → Photos or Videos, Audio | nein | Keine Medienauswahl; `Info.plist` hat weder `NSPhotoLibraryUsageDescription` noch `NSCameraUsageDescription` noch `NSMicrophoneUsageDescription`. |
| Browsing History, Search History | nein | Die App hat weder einen Browser noch eine Suche, die irgendwohin gemeldet würde. |
| Identifiers → Device ID | **nein** | Kein Push, keine `identifierForVendor`-Verwendung, keine Werbekennung — `grep -rn "ASIdentifier\|UNUserNotification"` über `ios/` findet nichts. **Das ist der Unterschied zur Android-Fassung**, wo seit `0.1.7` eine FCM-Kennung entsteht. |
| Usage Data (Product Interaction, Advertising Data, Other) | nein | Kein Analyse-SDK, keine eigene Nutzungsmessung. Einziges Paket: MapLibre (`ios/project.yml`). |
| Diagnostics (Crash Data, Performance Data, Other) | nein | Kein Crashlytics, kein Sentry, kein `MetricKit`. Absturzberichte gibt es nur, soweit ein Nutzer sie **Apple gegenüber** freigibt — das ist Apples Erhebung, nicht unsere, und wird im Formular nicht angegeben. |
| Other Data | nein | — |

### Sonderfall Standort — warum „nicht erhoben" stimmt

`ios/Dorf/Info.plist` enthält `NSLocationWhenInUseUsageDescription`
(„Zeigt deinen Standort auf der Dorfkarte, damit du siehst, welcher
Blumenkasten in der Nähe ist"). Der Schlüssel steht dort für die Dorfkarte
(`ios/Dorf/Bereiche/Karte/KarteView.swift`), die den eigenen Punkt zeigen
soll.

**Stand heute (0.1.0) gibt es dazu noch gar keinen Code:** `grep -rn
"CoreLocation\|CLLocation"` über `ios/` findet nichts, und `KarteView` ist
bislang eine `ContentUnavailableView` („Die Karte wird gerade gebaut").

Für das Formular gilt trotzdem dieselbe Antwort wie auf Android — **Location:
nicht erhoben** —, und zwar aus einem Grund, der auch nach Fertigstellung der
Karte trägt:

- **Es gibt keinen Weg, auf dem eine Position das Gerät verlässt.** Die
  gesamte API der App steht in `ios/Dorf/Daten/DorfApi.swift`; kein einziger
  Endpunkt nimmt Koordinaten entgegen. Die geschriebenen Rümpfe sind
  `ErledigungEingabe(liters, note)`, `ProfilEingabe` und
  `IdeeEingabe(wunsch, name, email)` — nachzulesen in `Modelle.swift`.
- **Kein Hintergrundstandort.** `Info.plist` hat weder
  `NSLocationAlwaysAndWhenInUseUsageDescription` noch `UIBackgroundModes`.
- Apples Definition von „erhoben" ist die Übertragung vom Gerät weg. Findet
  nicht statt.

**Prüfpunkt vor dem Hochladen:** Sobald die Karte den Standort wirklich
benutzt, hier noch einmal nachsehen — solange keine Koordinate in einem
Anfragerumpf landet, bleibt die Antwort „nicht erhoben". Landet doch einmal
eine dort (etwa beim Anlegen eines Ortes durch die Verwaltung, wie es der
Kommentar in `Info.plist` andeutet), wird daraus **Location → Precise
Location, erhoben, mit Identität verknüpft, Zweck App Functionality** — und
dieses Dokument, die Store-Beschreibung und die Datenschutzerklärung müssen
nachgezogen werden.

### Randnotiz IP-Adresse

Beim Laden der Kartenkacheln von `tiles.openfreemap.org`
(`MAP_STYLE_URL` in `ios/project.yml`) und bei jedem Aufruf des Dorfservers
wird zwangsläufig die IP-Adresse übertragen. Apple kennt dafür keine eigene
Datenart und verlangt keine Angabe; in der Datenschutzerklärung ist es
erwähnt. Der Dorfserver protokolliert Methode, Pfad und Dauer, **keine IP**
(`backend/internal/api/api.go`, `slog.Info("request", …)`). Offen wie auf
Android: ob der Reverse-Proxy davor IP-Adressen protokolliert.

---

## 4. Wo die Daten liegen und wie man sie loswird

- **Auf dem Gerät:** nur der Tokensatz der Anmeldung, im Schlüsselbund mit
  `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` — er wandert also nicht
  in ein iCloud-Backup und nicht auf ein anderes Gerät
  (`ios/Dorf/Anmeldung/Schluesselbund.swift`). **Abmelden löscht ihn**
  (`Anmeldung.abmelden()` ruft `Schluesselbund.loeschen()`).
- **Auf dem Server:** Erledigungen, Profil, Ideen — siehe oben.
- **Löschung:** Eine einzelne Meldung nimmt die meldende Person selbst zurück
  (`DorfApi.erledigungZuruecknehmen`, `DELETE /api/v1/completions/{id}`).
  Konto und alle Meldungen löscht der Dorfentwicklungskreis auf Anfrage; der
  Weg steht öffentlich unter
  <https://xn--rssing-wxa.de/app/daten-loeschen/>.
- Apple verlangt seit 2022 für Apps mit Konto einen **Weg zur Kontolöschung
  in der App selbst** (App Review Guideline 5.1.1(v)). Siehe dazu den
  offenen Punkt unten — das ist vor der Freigabe für die **Produktion** zu
  klären, für TestFlight noch nicht.

---

## 5. Datenschutz-URL

`https://xn--rssing-wxa.de/app/datenschutz/` — steht in
`store/metadata/ios/de-DE/privacy_url.txt` und `…/en-US/privacy_url.txt` und
wird von `store/check_ios_metadata.py` gegen genau diesen Wert geprüft.
Dieselbe Adresse ist in der App selbst verlinkt
(`ios/Dorf/Bereiche/Rechtliches/RechtlichesLeiste.swift`) — auf der
Startseite **und** auf dem Anmeldebildschirm, wie es § 5 DDG verlangt.

---

## 6. Offene Punkte

- [ ] **Kontolöschung in der App** (Guideline 5.1.1(v)): Heute gibt es nur
      „Abmelden" (`StartView.swift`). Für die Produktion braucht es entweder
      einen Knopf „Konto löschen" oder — was Apple ebenfalls akzeptiert —
      einen unmittelbar erreichbaren Link auf
      <https://xn--rssing-wxa.de/app/daten-loeschen/>. Der Link ist der
      kleinere Eingriff und sollte in `RechtlichesLeiste` daneben.
- [ ] Prüfen, sobald die Karte den Standort benutzt (siehe Sonderfall oben).
- [ ] Loggt der Reverse-Proxy IP-Adressen? Wenn ja: in der Erklärung nennen.
- [ ] **Sobald Push nachgerüstet wird**, kommt hier *Identifiers → Device ID*
      hinzu (APNs-Token, mit Identität verknüpft, Zweck App Functionality) —
      und die Antwort auf „Weitergabe" ändert sich, weil das Token bei Apple
      liegt. Dann auch `store/data-safety.md` gegenlesen, damit beide Stores
      dasselbe sagen.
- [ ] Bei jeder neuen Version prüfen, ob ein neues Feld die Antworten oben
      ändert. Der schnellste Test: Gibt es einen neuen Anfragerumpf in
      `ios/Dorf/Daten/Modelle.swift`?
