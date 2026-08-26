# Offen aus dem Bereich „Idee vorschlagen"

Was beim Bau des Ideen-Formulars aufgefallen ist und **außerhalb** von
`Dorf/Bereiche/Ideen/` liegt. Der Bereich fasst keine fremde Datei an —
deshalb steht es hier und nicht im Quelltext.

Umgebung des Befunds: Xcode 26.5 (Build 17F42), Apple Swift 6.3.2,
Simulator iPhone 17 (iOS 26.5).

## 1. Der Build bricht schon ohne den Ideen-Bereich ab (Swift 6.3)

`xcodebuild build` scheitert auf dem Stand `9924c05` **auch dann**, wenn
`Dorf/Bereiche/Ideen/` und `DorfTests/IdeenTests.swift` gar nicht vorhanden
sind:

```
Dorf/Daten/DorfApi.swift:57:10: error: main actor-isolated default value in a nonisolated context
Dorf/Daten/DorfApi.swift:58:10: error: main actor-isolated default value in a nonisolated context
```

Grund: `SWIFT_DEFAULT_ACTOR_ISOLATION: MainActor` (project.yml) macht auch
`Konfiguration.apiBasis` und `URLSession.dorfSitzung` MainActor-isoliert.
`DorfApi` ist aber `nonisolated` — seit Swift 6.3 ist das ein Fehler, kein
Hinweis mehr. Dasselbe gilt für die DTOs: Ihre `Codable`-Konformanzen sind
MainActor-isoliert und lassen sich in `DorfApi` nicht mehr benutzen
(„main actor-isolated conformance of 'Ich' to 'Decodable' …").

Behoben ist das, wenn `Dorf/Daten/Modelle.swift` und `Dorf/Daten/DorfApi.swift`
ihre Typen als `nonisolated` deklarieren (DTOs, `RFC3339`, `DorfFehler`,
`DorfApi`, die `URLSession`-Erweiterung) und `Konfiguration` ebenfalls
`nonisolated` ist. Das gehört in den Bereich, dem diese Dateien gehören.

**Der Ideen-Bereich wurde mit genau diesem Behelf geprüft** (lokal angewandt,
gebaut, getestet, wieder zurückgenommen — nichts davon ist committet):
Build und alle 20 Tests grün.

## 2. `TEST_HOST` passt nicht zu `PRODUCT_NAME`

`xcodebuild test` findet den Testträger nicht:

```
Could not find test host for DorfTests:
TEST_HOST evaluates to ".../Dorf.app/Dorf"
```

`project.yml` setzt `PRODUCT_NAME: Rössing`, gebaut wird also `Rössing.app`;
XcodeGen leitet `TEST_HOST` aber aus dem Zielnamen `Dorf` ab. Entweder trägt
`DorfTests` ein passendes `TEST_HOST`/`BUNDLE_LOADER` nach, oder der App-Name
und der Zielname werden zusammengeführt.

## 3. Ohne Signatur wird die Testträger-App abgeschossen

Mit `CODE_SIGNING_ALLOWED=NO` (so steht es in `ios/README.md` und im
Makefile) startet die App im Simulator nicht:

```
Test crashed with signal kill before establishing connection.
```

Das eingebettete `MapLibre.framework` kommt ohne Signatur nicht durch dyld.
Grün wird der Testlauf mit einer Ad-hoc-Signatur:

```sh
xcodebuild test -project Dorf.xcodeproj -scheme Dorf \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  CODE_SIGNING_ALLOWED=YES CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY=-
```

Das sollte in `project.yml`, `Makefile` und `README.md` nachgezogen werden —
sonst steht die CI vor demselben Rätsel.

## 4. Nichts davon liegt am Ideen-Bereich

`Dorf/Bereiche/Ideen/IdeenModell.swift` kennt kein Netz: Der Weg zum Backend
wird `absenden(ueber:)` übergeben. Die Tests in `DorfTests/IdeenTests.swift`
laufen deshalb ohne Server und ohne Adresse.
