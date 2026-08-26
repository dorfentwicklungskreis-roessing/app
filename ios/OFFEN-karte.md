# Offen aus dem Bereich „Karte"

Notiert aus `ios/Dorf/Bereiche/Karte/`. Ein Bereich fasst keinen fremden an —
was außerhalb geändert werden muss, steht deshalb hier.

## 1. Die Karte hängt noch an keiner Stelle der App

`KarteView(orte:auswahl:)` ist fertig, wird aber von niemandem aufgerufen.
Einbinden gehört nach `Dorf/Bereiche/Mithelfen/` (Reiter „Karte" neben der
Liste) beziehungsweise `Dorf/Navigation/Ziel.swift`. Die Karte lädt bewusst
selbst nichts: Sie bekommt die Orte gereicht und meldet den Tipp zurück.

```swift
KarteView(orte: orte) { ort in /* zur Detailseite */ }
```

## 2. Termine mit Koordinate gehören auf die Karte

`Termin.koordinate` (`Dorf/Bereiche/Veranstaltungen/Termine.swift`) liefert
inzwischen Punkte. Auf der Karte wären sie eine **zweite** GeoJSON-Quelle mit
eigener Ebene (andere Form/Farbe als die Ampel-Nadeln, sonst verwechselt man
beides) — technisch bereits vorbereitet: `Kartendaten.merkmale`/`geoJson`
arbeiten auf `[Kartenmerkmal]`, nicht auf `[Ort]`.

Dafür müsste die vereinbarte Signatur von `KarteView` um einen Parameter
wachsen (etwa `termine: [Termin] = []`). Das ist eine Absprache, kein
Alleingang — deshalb hier notiert und nicht gebaut.

## 3. „Ort anlegen" braucht einen Auswahlmodus

Die Android-Karte kann zusätzlich, was die Verwaltung braucht: Ein Tipp auf
die freie Fläche liefert die Koordinate (`onMapTap`), ein blauer Punkt zeigt
die getroffene Wahl (`auswahl`). Für den Bereich „Verwaltung" fehlt das hier
noch; auch dafür müsste die Signatur wachsen.

## 4. E2E: Der Kartenstil ist schon lokal übersteuerbar

`MAP_STYLE_URL` aus `project.yml` landet über `Info.plist` in
`Konfiguration.kartenstil`; im Quelltext steht keine Adresse. Die CI setzt
bereits `MAP_STYLE_URL=http://127.0.0.1:8097/map-style.json`
(`.github/workflows/ios.yml`), `android/e2e/fixtures/map-style.json` taugt
dafür unverändert. Beim Bau der iOS-E2E-Läufe nicht vergessen — sonst zieht
ein Kartentest Kacheln von openfreemap.org.

## 5. Kein Test darf die Karte laden

Deshalb liegt die ganze Rechnerei in `Kartenrechnung.swift` (reine
Funktionen, kein MapLibre, kein UIKit) und wird in `DorfTests/KarteTests.swift`
geprüft. Wer die Karte später doch im Test öffnet, braucht dafür den lokalen
Ersatzstil aus Punkt 4 — nicht den echten.
