# Offene Punkte der iOS-App

Was die erste Fassung bewusst **nicht** kann, und was beim Bauen aufgefallen
ist. Nichts davon ist ein Fehler — es ist der Zuschnitt: Die App sollte
zuerst das können, wofür es die Dorf-App gibt (mithelfen, sehen, melden),
und nicht alles auf einmal halb.

Die Blocker, die während des Baus in der Grundlage steckten (MainActor-
Isolation der Datenschicht, Modulname, Signierung im Simulator), sind
behoben — siehe `CLAUDE.md`, Abschnitt „Entwicklungsumgebung (iOS)".

## Bewusst noch nicht gebaut

- **Vergabe der Pflegeaufgaben.** `Aufgabe.assignment`, `signupCount` und
  `signedUp` kommen in den DTOs bereits an, aber `DorfApi` kennt weder
  `signup`/`signoff` noch `claim`/`release` noch die Benachrichtigungsliste.
  Solange das fehlt, zeigt „Mithelfen" auch keine Helfer-Anfragen.
- **Bereich „Verwaltung".** Orte und Aufgaben pflegen, während man davor
  steht — auf Android der Bereich, der die `admin`-Rolle braucht. Dazu
  gehörte auch ein Auswahlmodus der Karte („Tipp auf die Fläche liefert die
  Koordinate"), den `KarteView` noch nicht hat.
- **Push (APNs).** Es gibt keine Gerätekennung, keine Kanäle, keine
  Erlaubnisfrage. Deshalb erhebt die App auch keine Geräte-ID — anders als
  die Android-Fassung; siehe `store/ios-datenschutz.md`.
- **Kontolöschung in der App.** Heute gibt es nur „Abmelden". Für TestFlight
  genügt das, für die Veröffentlichung verlangt Apple (Richtlinie 5.1.1 v)
  einen Weg zum Löschen des Kontos.

## Kleinere Lücken

- **Startseite zeigt keine Zahl.** `Bereichskachel` hat ein Feld `hinweis`
  („3 Orte warten auf dich"), das niemand füllt. Dafür müsste die Startseite
  die Orte kennen — am ehesten ein `OrteModell` in `AppUmgebung`, das
  Startseite und Bereich teilen.
- **Der Hitzefaktor wird geladen, aber nicht gezeigt.**
  `OrteAntwort.wateringFactor` liegt im Modell; auf Android wird daraus ein
  Hinweis auf der Startseite („Heiß — bitte großzügig gießen"). Gehört
  dorthin, nicht in die Ortsliste.
- **Termine auf der Dorfkarte.** `Termin.koordinate` ist gefüllt, sobald die
  Website Koordinaten am Ort pflegt. Auf der Karte wären sie eine **zweite**
  GeoJSON-Quelle mit eigener Ebene — andere Form und Farbe als die
  Ampel-Nadeln, sonst verwechselt man beides. `Kartendaten` arbeitet bereits
  auf `[Kartenmerkmal]` statt auf `[Ort]` und ist dafür vorbereitet; die
  Signatur von `KarteView` müsste um `termine:` wachsen.
- **Karte kann nicht auf einen Ort zeigen.** Sie meldet einen Tipp zurück,
  nimmt aber keine Anweisung entgegen — beim Umschalten von der Liste zur
  Karte lässt sich der gewählte Ort deshalb nicht anfahren.
- **Kein Zwischenspeicher je Zeitraum** in der Rangliste: Jedes Umschalten
  fragt neu. Bei fünf Zeiträumen und kleinen Antworten verschmerzbar.

## Für die iOS-E2E-Läufe merken

Es gibt noch keine. Wenn sie kommen, gilt dieselbe Regel wie überall
(`CLAUDE.md`): **kein Test fasst einen entfernten Server an.** Alle vier
Adressen sind über Build-Einstellungen übersteuerbar, und
`.github/workflows/ios.yml` setzt sie bereits lokal;
`.github/scripts/pruefe_lokale_tests.py` wacht darüber und kennt `ios/`
inzwischen.

Für die Karte taugt `android/e2e/fixtures/map-style.json` unverändert als
Ersatzstil — sonst zieht ein Kartentest Kacheln von openfreemap.org.
