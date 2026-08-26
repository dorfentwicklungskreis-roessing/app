# Offene Punkte aus dem Zweig `ios/konto`

Was beim Bau von Einstellungen, Kontolöschung und dem Feinschliff der
Startseite liegen geblieben ist — nicht aus Versehen, sondern weil es
anderen gehört. Dieser Zweig übersetzt für sich; alles hier Genannte ist
eine **Zeile bei jemand anderem**, nicht eine Baustelle hier.

## Gehört anderen Zweigen

- **`Ziel.verwaltung` fehlt bewusst.** Der Bereich „Verwaltung" kommt von
  anderer Seite, und ein `case .verwaltung: VerwaltungView()` würde diesen
  Zweig sofort unübersetzbar machen. Sobald `VerwaltungView` existiert, sind
  es genau drei Stellen:
  1. `Navigation/Ziel.swift`: `case verwaltung` in die Aufzählung,
  2. `Navigation/Ziel.swift`: `case .verwaltung: VerwaltungView()` in
     `dorfZiele()`,
  3. `Navigation/StartView.swift`: eine `Bereichskachel` — und zwar nur, wenn
     `umgebung.binAdmin` gilt. Ein Bereich, den man nicht betreten darf,
     gehört nicht auf die Startseite.

- **„Mithelfen" lädt noch ein zweites Mal.** `AppUmgebung.orte` ist jetzt das
  gemeinsame `OrteModell` — die Startseite zählt daraus die wartenden Orte
  und liest den Hitzefaktor. `Bereiche/Mithelfen/MithelfenView.swift` baut
  sich in seinem `.task` aber weiterhin ein **eigenes** Modell
  (`OrteModell(api: umgebung.api)`). Solange das so ist, wird zweimal
  abgerufen, und Kachel und Liste können kurz Verschiedenes sagen. Die
  Umstellung ist eine Zeile:

  ```swift
  // statt: let vorhanden = modell ?? OrteModell(api: umgebung.api)
  MithelfenInhalt(modell: umgebung.orte, meinSub: umgebung.meinSub)
  ```

  Der Ordner `Bereiche/Mithelfen/` gehört in dieser Runde einem anderen
  Agenten und wurde deshalb nicht angefasst.

- **„Anfragen und Hinweise"** ist als Kachel (`Ziel.anfragen`) auf der
  Startseite verdrahtet und zeigt auf `VergabeView()`. Die View selbst ist
  noch der Platzhalter aus einem anderen Zweig.

## Gehört ins Repo, aber nicht in diese Dateien

- **`ios/OFFEN.md`** nennt unter „Bewusst noch nicht gebaut" weiterhin die
  fehlende **Kontolöschung** und unter „Kleinere Lücken" die **fehlende Zahl
  auf der Startseite** und den **ungenutzten Hitzefaktor**. Alle drei sind
  mit diesem Zweig erledigt; die Einträge gehören gestrichen, sobald
  zusammengeführt wird.

- **`backend/README.md`**, Abschnitt zum Profil: Die Endpunktliste kennt
  `GET /api/v1/me` und `PUT /api/v1/me/profile`, aber noch nicht
  **`DELETE /api/v1/me`**. Eine Zeile.

- **`backend/SICHERHEIT.md`** braucht einen Abschnitt „Kontolöschung": was
  gelöscht wird (Profil, Gerätekennungen, Helfer-Eintragungen,
  Benachrichtigungen, Befähigungsanträge), was anonymisiert bleibt
  (Erledigungen, beendete Zusagen, eingereichte Ideen) und dass das Konto in
  der **Rössing-ID** ausdrücklich **nicht** angetastet wird — es gehört
  Zitadel und dient auch anderen Anwendungen. Die Begründung steht im
  Kopfkommentar von `backend/internal/db/konto.go`.

- **`store/datenschutz.md`** und die Datenschutzerklärung auf der Website:
  Unter „Löschung" steht bislang „formlos per E-Mail". Ab jetzt gibt es den
  Weg **in der App** (Einstellungen → Konto löschen), und die Erklärung
  sollte auch sagen, dass Erledigungen anonymisiert bestehen bleiben und
  warum.

- **`store/ios-datenschutz.md`**: Die iOS-App erhebt weiterhin keine
  Geräte-ID; das Löschen räumt sie trotzdem mit weg, sobald es sie gibt.

## Abweichung von der Dateiliste dieses Auftrags

Der Lösch-Endpunkt braucht Schreibzugriff auf die Datenbank, und
`db.DB.sql` ist `private`; im Paket `api` gibt es keinen Weg, eine Zeile zu
ändern. Deshalb liegt die eigentliche Löschung in der **neuen** Datei
`backend/internal/db/konto.go` (eine einzige Methode, `KontoLoeschen`).
Neue Datei heißt: kein Konflikt mit den anderen Zweigen — an
`internal/db/db.go` und den übrigen Dateien des Pakets wurde nichts
geändert.
