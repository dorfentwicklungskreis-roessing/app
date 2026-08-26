# Offen aus dem Bereich „Mithelfen"

Was der Bereich außerhalb von `Dorf/Bereiche/Mithelfen/` bräuchte. Hier steht
es, statt dass ein Bereich eine fremde Datei anfasst.

## 1. Blocker: Der Trunk baut und testet mit Xcode 26.5 nicht

`ios-app` (9924c05) lässt sich **vor jeder Bereichsarbeit** weder übersetzen
noch testen. Drei Sachen sind es, alle im Fundament — kein Bereich kann sie
beheben, ohne genau die Dateien anzufassen, die er nicht anfassen darf. Alle
drei sind lokal geprüft; danach: `** BUILD SUCCEEDED **` und
`Test run with 19 tests in 2 suites passed`.

### 1a. Die Datenschicht gehört nicht auf den Hauptthread

```
Dorf/Daten/DorfApi.swift:57:10: error: main actor-isolated default value in a nonisolated context
… error: main actor-isolated conformance of 'Ort' to 'Decodable' cannot be used …
```

`SWIFT_DEFAULT_ACTOR_ISOLATION: MainActor` macht auch die DTOs und
`Konfiguration` zu `@MainActor`. `DorfApi` ist aber ausdrücklich
`nonisolated` — damit sind von dort aus weder `Konfiguration.apiBasis` und
`URLSession.dorfSitzung` noch irgendeine `Codable`-Konformanz erreichbar.

Die Datenschicht kennt keine Oberfläche und gehört nicht auf den Hauptthread.
Sie wird `nonisolated`:

```sh
cd ios
perl -0pi -e 's/^(struct |enum |extension )/nonisolated $1/mg'                Dorf/Daten/Modelle.swift
perl -0pi -e 's/^enum DorfFehler/nonisolated enum DorfFehler/m'               Dorf/Daten/DorfApi.swift
perl -0pi -e 's/^extension URLSession \{/nonisolated extension URLSession {/m' Dorf/Daten/DorfApi.swift
perl -0pi -e 's/^enum Konfiguration \{/nonisolated enum Konfiguration {/m'    Dorf/Konfiguration.swift
```

Die Vorbelegung `MainActor` bleibt damit für Views und Modelle bestehen —
genau so, wie `README.md` es beschreibt.

### 1b. `xcodebuild test` findet den Test-Host nicht und kennt kein Modul `Dorf`

```
Could not find test host for DorfTests: TEST_HOST evaluates to ".../Dorf.app/Dorf"
DorfTests/…: error: unable to resolve module dependency: 'Dorf'
```

`PRODUCT_NAME: Rössing` benennt sowohl das Produkt als auch das Swift-Modul
um. XcodeGen setzt `TEST_HOST` aber nach dem **Target**-Namen (`Dorf.app`),
und `@testable import Dorf` sucht ein Modul, das nicht mehr so heißt. Die
Unit-Tests sind also noch nie gelaufen — auch `ModelleTests.swift` nicht.

In `project.yml`, Target `Dorf`:

```yaml
        PRODUCT_NAME: Rössing
        PRODUCT_MODULE_NAME: Dorf     # neu
```

in Target `DorfTests`:

```yaml
        TEST_HOST: $(BUILT_PRODUCTS_DIR)/Rössing.app/Rössing   # neu
        BUNDLE_LOADER: $(TEST_HOST)                            # neu
```

Sauberer wäre `PRODUCT_NAME: Dorf` plus `CFBundleDisplayName = Rössing` in
`Dorf/Info.plist` — dann stimmt alles von allein. Das ist aber die größere
Änderung; die Entscheidung gehört dem Fundament.

### 1c. Eine unsignierte App startet im Simulator nicht

```
Rössing encountered an error (Early unexpected exit … Test crashed with signal kill …)
```

`CODE_SIGNING_ALLOWED: NO` im Target lässt die App unsigniert; der Simulator
tötet sie beim Start, und der Testläufer bekommt nie eine Verbindung. Gebaut
wird sie so noch, getestet nicht mehr.

Ad-hoc-Signierung genügt und braucht kein Apple-Konto — im Target `Dorf`
statt `CODE_SIGNING_REQUIRED: NO` / `CODE_SIGNING_ALLOWED: NO`:

```yaml
        CODE_SIGN_IDENTITY: "-"
```

Dazu muss `CODE_SIGNING_ALLOWED=NO` aus dem **Test**-Aufruf verschwinden
(`ios/Makefile`, Ziel `testen`, und die Zeile in `README.md`) — der Schalter
auf der Kommandozeile übersteuert sonst alles. Bis dahin laufen die Tests mit:

```sh
xcodebuild test -project Dorf.xcodeproj -scheme Dorf \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=-
```

**Der Bereich „Mithelfen" ist gegen diesen reparierten Stand gebaut und
getestet.** Die Reparatur selbst ist hier bewusst **nicht** mitcommittet: Sie
betrifft `Modelle.swift`, `DorfApi.swift`, `Konfiguration.swift`,
`project.yml`, `Makefile` und `README.md` — Dateien, die jeder Bereich sonst
gleichzeitig und verschieden anfassen würde. Sie gehört einmal auf `ios-app`.

## 2. Kleinigkeiten

- `Navigation/StartView.swift`: `Bereichskachel` hat schon ein Feld `hinweis`
  („3 Orte warten auf dich"), benutzt es aber nirgends. Für „Mithelfen" wäre
  die Zahl der gelben und roten Orte dort der beste Platz — dafür müsste die
  Startseite die Orte kennen (etwa ein `OrteModell` in `AppUmgebung`, das
  Startseite und Bereich teilen).
- `Bereiche/Karte/KarteView.swift`: Die Karte meldet nur einen Tipp zurück.
  Ein „zeige auf Ort X" (etwa beim Umschalten von der Liste zur Karte) ginge
  nur mit einer Erweiterung der Schnittstelle — bewusst nicht angefasst.
- Der Wetterfaktor (`OrteAntwort.wateringFactor`) wird geladen und im Modell
  gehalten, aber noch nirgends gezeigt. Android macht daraus einen Hinweis auf
  der Startseite („Heiß — bitte großzügig gießen"); das ist ein eigener
  Schritt und gehört eher auf die Startseite als in die Ortsliste.
