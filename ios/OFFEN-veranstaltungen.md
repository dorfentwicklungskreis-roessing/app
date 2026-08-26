# Offen aus dem Bereich „Veranstaltungen"

Notiert aus `ios/Dorf/Bereiche/Veranstaltungen/`. Ein Bereich fasst keinen
fremden an — was außerhalb geändert werden muss, steht deshalb hier.

## 1. Das Fundament baut mit Xcode 26.5 / Swift 6.3 nicht mehr

Schon **ohne** die Veranstaltungen ist `make bauen` rot (geprüft auf dem
Stand `9924c05`, Xcode 26.5, Swift 6.3.2). Ursache ist die
`MainActor`-Vorbelegung (`SWIFT_DEFAULT_ACTOR_ISOLATION`): Sie erfasst auch
`Konfiguration`, die `Codable`-Konformanzen in `Modelle.swift`, `RFC3339` und
`URLSession.dorfSitzung` — `DorfApi` ist aber `nonisolated` und darf sie
deshalb nicht anfassen.

```
Dorf/Daten/DorfApi.swift:57: main actor-isolated default value in a nonisolated context
Dorf/Daten/DorfApi.swift:67: main actor-isolated conformance of 'Ich' to 'Decodable'
                             cannot be used in caller isolation inheriting-isolated context
… (dasselbe für OrteAntwort, ErledigungenAntwort, Rangliste, DorfbewohnerAntwort,
   ErledigungEingabe, ProfilEingabe, IdeeEingabe, ApiFehlerAntwort, RFC3339.datum)
```

Die saubere Lösung gehört ins Fundament, nicht in einen Bereich: `Konfiguration`,
die DTOs in `Modelle.swift`, `RFC3339` und `URLSession.dorfSitzung` sind
unveränderlich und gehören ausdrücklich `nonisolated` — dann bleibt `DorfApi`
`nonisolated` und Sendable, wie gedacht.

Zum Nachstellen genügt hier eine Zeile in `Dorf/Daten/DorfApi.swift`:

```diff
-nonisolated final class DorfApi: Sendable {
+final class DorfApi {
```

Damit ist der Build grün. Das ist aber nur die Notlösung: `DorfApi` wäre dann
an den Hauptfaden gebunden, obwohl er dort nichts zu suchen hat.

Der Bereich „Veranstaltungen" selbst braucht nichts davon — sein Client zur
Website liegt bewusst in der Vorbelegung des Projekts und ist unabhängig
von `DorfApi`.

## 2. `make testen` lief noch nie durch

Zwei Dinge in `ios/project.yml` fehlen, beide unabhängig von diesem Bereich:

```
xcodebuild: error: Could not find test host for DorfTests:
  TEST_HOST evaluates to ".../Dorf.app/Dorf"
DorfTests/ModelleTests.swift:4: unable to resolve module dependency: 'Dorf'
```

Das Programm heißt `Rössing` (`PRODUCT_NAME`), also heißt auch das Modul so —
`@testable import Dorf` findet nichts, und der Testträger sucht eine
`Dorf.app`, die es nicht gibt. Vorschlag:

```diff
 targets:
   Dorf:
     settings:
       base:
         PRODUCT_NAME: Rössing
+        # Angezeigt wird „Rössing", importiert wird „Dorf" — sonst müsste
+        # jeder Test `@testable import Rössing` schreiben.
+        PRODUCT_MODULE_NAME: Dorf
 …
   DorfTests:
     settings:
       base:
         GENERATE_INFOPLIST_FILE: YES
+        TEST_HOST: "$(BUILT_PRODUCTS_DIR)/Rössing.app/Rössing"
+        BUNDLE_LOADER: "$(TEST_HOST)"
```

Mit diesen beiden Punkten laufen die Tests durch (29 Tests, 2 Suiten, grün —
davon 22 aus `DorfTests/VeranstaltungenTests.swift`).

## 3. Die CI-Wache kennt `ios/` noch nicht

`.github/scripts/pruefe_lokale_tests.py` prüft in `GEPRUEFTE_PFADE` nur
`android/…` und `backend/…`. `ios/DorfTests` und `ios/project.yml` gehören
dort ergänzt, sonst kann sich in den iOS-Tests unbemerkt eine
Produktionsadresse einnisten. `DorfTests/VeranstaltungenTests.swift` hält
sich schon an die Regel: Es geht nichts ins Netz (eigene `URLProtocol`-Ablage),
und die Adressen der Website kommen nur als Datenstrings vor.

## 4. Termine auf der Dorfkarte

`Termin.koordinate` ist gefüllt, sobald die Website Koordinaten am Ort pflegt
(`geo` in `src/data/locations/*.yaml`). Die Karte (`Bereiche/Karte/`) könnte
sie anzeigen — abgestimmt werden müsste, wem die Kartenpunkte gehören.
Bewusst nicht von hier aus gemacht.
