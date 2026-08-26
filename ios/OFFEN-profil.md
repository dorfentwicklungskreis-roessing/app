# Offen aus dem Bereich „Profil"

Notizen aus der Arbeit an `Dorf/Bereiche/Profil/`. Alles hier liegt
**außerhalb** dieses Bereichs und wurde deshalb nicht angefasst.

## Der Bau ist schon ohne diesen Bereich rot (Xcode 26.5, Swift 6.3.2)

`xcodebuild build` scheitert auf dem Stand von `ios-app` — der erste Baulauf
lief noch auf dem unveränderten Stand, also ohne die neuen Dateien. Die Fehler
stehen allesamt in `Dorf/Daten/`:

1. **`Dorf/Daten/DorfApi.swift:57–58`** — *main actor-isolated default value in
   a nonisolated context*. `DorfApi` ist `nonisolated`, die Vorgabewerte
   `Konfiguration.apiBasis` und `URLSession.dorfSitzung` sind es (durch
   `SWIFT_DEFAULT_ACTOR_ISOLATION: MainActor`) nicht.
   Behoben durch `nonisolated` an `enum Konfiguration` und an
   `URLSession.dorfSitzung`.
2. **`Dorf/Daten/Modelle.swift`** — mit `SWIFT_APPROACHABLE_CONCURRENCY: YES`
   (also `InferIsolatedConformances`) sind die `Codable`-Konformanzen der DTOs
   MainActor-isoliert; das nonisolierte `DorfApi` kann sie nicht benutzen
   („main actor-isolated conformance of 'Ich' to 'Decodable' cannot be used …",
   dieselbe Meldung für `OrteAntwort`, `ErledigungenAntwort`, `Rangliste`,
   `DorfbewohnerAntwort`, `ErledigungEingabe`, `ProfilEingabe`, `IdeeEingabe`,
   `ApiFehlerAntwort`, dazu `RFC3339.datum`).
   Behoben durch `nonisolated` an den Typen in `Modelle.swift` — die DTOs sind
   reine Werte und gehören ohnehin keinem Aktor.

## Die Unit-Tests laufen so gar nicht an

Beides steckt in `project.yml`:

3. **Modulname.** Der Ziel-Name ist `Dorf`, `PRODUCT_NAME` aber `Rössing` —
   damit heißt das Modul `Rössing`, und `@testable import Dorf` in
   `DorfTests/` scheitert mit „unable to resolve module dependency: 'Dorf'".
   Behoben durch `PRODUCT_MODULE_NAME: Dorf` beim Ziel `Dorf`.
4. **Test-Host.** XcodeGen leitet `TEST_HOST` aus dem Zielnamen ab
   (`…/Dorf.app/Dorf`), die App liegt aber als `Rössing.app/Rössing` vor:
   „Could not find test host for DorfTests".
   Behoben durch `TEST_HOST: $(BUILT_PRODUCTS_DIR)/Rössing.app/Rössing` und
   `BUNDLE_LOADER: $(TEST_HOST)` beim Ziel `DorfTests`.

Mit diesen vier Änderungen — nur zur Prüfung lokal gesetzt und **nicht**
committet — bauen App und Tests durch, und alle 24 Tests
(`ModelleTests` + `ProfilTests`) laufen grün.

## Kleinigkeiten

- `Navigation/Ziel.swift` und `Navigation/StartView.swift` verdrahten
  „Mein Profil" und „Dorfbewohner" bereits richtig; dort war nichts zu tun.
- Ein `Ziel.verwaltung` gibt es noch nicht. Die Kennzeichnung „nur für die
  Verwaltung sichtbar" in der Dorfbewohner-Liste hängt deshalb allein an
  `adminView` aus der Antwort — das ist auch die richtige Quelle.
