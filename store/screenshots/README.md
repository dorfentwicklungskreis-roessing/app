# Store-Bilder der iOS-App

Hier liegen die Bildschirmfotos, die im App Store zu sehen sind, und alles,
was nötig ist, um sie **von Grund auf neu** zu erzeugen — Beispieldaten,
Kartenstil, Terminfeed und der Prüfstand, der sich durch die App tippt.

```
store/screenshots/
├── aufnehmen.sh            ein Aufruf je Gerät: Bühne aufbauen, knipsen, ablegen
├── bilder_ablegen.py       Bilder aus dem xcresult-Bündel holen und einsortieren
├── beispieldaten/
│   ├── fuellen.py          Orte, Aufgaben, Erledigungen, Profile über die API
│   ├── events.json         der Terminfeed („Was ist los in Rössing")
│   ├── kartenstil_bauen.py holt einmalig OpenStreetMap-Daten und baut daraus …
│   └── map-style.json      … den lokalen Kartenstil (Geometrie fest eingebettet)
├── pruefstand/             eigenes Xcode-Projekt, nur ein UI-Test-Bündel
└── ios/<sprache>/<gerät>/NN-name.png
```

## Die wichtigste Regel

**Kein Bild entsteht gegen die Produktion.** Weder gegen
`app.xn--rssing-wxa.de`, noch gegen die echte Rössing-ID, noch gegen
`tiles.openfreemap.org`. Auf den Bildern steht kein Name und keine
Kontaktangabe eines echten Dorfbewohners — sie sind öffentlich, sobald die App
im Store ist.

Deshalb:

* **Backend**: lokal, `AUTH_MODE=insecure-dev`, eigene Datenbank unter
  `/tmp/dorf-shots.sqlite`.
* **Anmeldung**: der Entwickler-Login der App (`DEV_AUTH=1`, nur im
  Debug-Build vorhanden — `Konfiguration.entwicklerLoginErlaubt`). Zitadel
  wird nicht angefasst.
* **Terminfeed und Kartenstil**: aus `beispieldaten/`, ausgeliefert von einem
  `python3 -m http.server` auf `127.0.0.1:8097` — dasselbe Verfahren wie in
  `.github/workflows/ios.yml`.
* **Namen**: erfunden (Anna B., Bernd K., Clara W. …). Rufnummern aus dem von
  der Bundesnetzagentur für Film und Funk reservierten Bereich
  (`0171 39x xxxx`), Adressen unter `example.org` (RFC 2606).

## Aufnehmen

Voraussetzungen: Xcode 26, XcodeGen (`brew install xcodegen`), Go im PATH
(`~/.local/share/mise/installs/go/1.23.12/bin`), ein freier Port 8080 und 8097.

```sh
# 6,9 Zoll (iPhone 17 Pro Max)
store/screenshots/aufnehmen.sh 59AC6B50-FB37-4DF2-9E3B-0B9DD67A8D67 iphone-6_9

# 13 Zoll (iPad Pro 13-inch, M5)
store/screenshots/aufnehmen.sh 82470F74-87D2-45EB-969F-66604BFB123D ipad-13
```

Die UDIDs stehen in `xcrun simctl list devices available`. Ein Lauf dauert
rund fünf Minuten und macht der Reihe nach:

1. Terminfeed und Kartenstil auf `127.0.0.1:8097` ausliefern.
2. Backend übersetzen und auf `127.0.0.1:8080` starten
   (`DB_PATH=/tmp/dorf-shots.sqlite AUTH_MODE=insecure-dev SEED=1`).
3. `beispieldaten/fuellen.py` laufen lassen. `SEED=1` allein legt nur zwei
   Blumenkästen an — zu wenig für eine Karte und viel zu wenig für eine
   Rangliste. Das Skript räumt die zwei weg und legt zehn Orte, dreizehn
   Aufgaben und vierzig Erledigungen von acht erfundenen Personen an.
4. Simulator neu starten (sonst klebt ein „◀ Safari" in der Statusleiste),
   Systemsprache auf Deutsch stellen, Statusleiste auf 9:15 und volle
   Batterie setzen. An den Netz-Symbolen wird **nichts** gedreht.
5. Die App aus `ios/` bauen — mit allen vier Adressen lokal übersteuert und
   `DEV_AUTH=1` — und auf dem Simulator installieren.
6. Den Prüfstand als UI-Test laufen lassen. Er tippt sich durch die App und
   hängt jedes Bild als `XCTAttachment` an das Testergebnis.
7. `bilder_ablegen.py` holt die Anhänge mit
   `xcrun xcresulttool export attachments` heraus, prüft ihre Maße und legt
   sie unter `ios/<sprache>/<gerät>/` ab.

Läuft etwas schief, stehen die Protokolle unter `/tmp/dorf-shots-arbeit/`
(`backend.log`, `web.log`, `bau.log`, `test.log`).

### Warum das Skript in einem Stück laufen muss

Backend, Webserver und Simulator müssen **gleichzeitig** leben. In gekapselten
Ausführungsumgebungen (etwa dem Sandkasten eines Agenten) bekommt jeder
Shell-Aufruf sein eigenes Netz; ein in Aufruf A gestarteter Server ist in
Aufruf B nicht erreichbar. Der Simulator dagegen hängt am echten Netz des
Rechners und erreicht beide Server. Deshalb: ein Aufruf, ein Skript.

### Warum ein zweites Xcode-Projekt

Der Simulator lässt sich über `xcrun simctl` starten und knipsen, aber nicht
**tippen** — und einen Bildschirm gibt es auf der Bau-VM nicht. Die
Navigation muss also ein UI-Test übernehmen.

`ios/project.yml` bleibt dabei unangetastet (dort arbeiten andere): Der
Prüfstand unter `pruefstand/` ist ein eigenes Projekt mit **nur** einem
UI-Test-Bündel und ohne App. Er steuert die schon installierte Dorf-App über
ihre Bundle-ID (`XCUIApplication(bundleIdentifier: "de.roessing.app")`).

### Warum ein eigener Kartenstil

`android/e2e/fixtures/map-style.json` ist eine einzige Hintergrundfarbe —
richtig für einen Test, der nur prüfen will, ob Nadeln erscheinen. Als erstes
Bild im App Store ist eine leere beige Fläche unbrauchbar: Sie sieht nach
einer kaputten Karte aus.

`beispieldaten/kartenstil_bauen.py` holt deshalb **einmal** die Geometrie um
Rössing von der Overpass-API und bettet sie fest in
`beispieldaten/map-style.json` ein. Beim Aufnehmen wird nur noch diese Datei
ausgeliefert — kein Kachelserver, keine Netzverbindung, jedes Mal dasselbe
Bild. Datenquelle: OpenStreetMap, ODbL, © OpenStreetMap-Mitwirkende; der
Hinweis steht als `attribution` in jeder Quelle des Stils und erscheint in der
App hinter dem (i)-Zeichen.

Neu holen (nur nötig, wenn sich die Karte geändert haben soll):

```sh
python3 store/screenshots/beispieldaten/kartenstil_bauen.py
```

## Die sieben Bilder

In dieser Reihenfolge — Apple zeigt die ersten drei in den Suchergebnissen:

| Datei | Was darauf ist |
|---|---|
| `01-mithelfen-karte.png` | Dorfkarte mit Ampel-Nadeln (3 rot, 2 gelb, 5 grün) |
| `02-mithelfen-liste.png` | dieselben Orte als Liste, dringendste zuerst |
| `03-ortsdetail.png` | „Blumenkasten am Dorfplatz": zwei Aufgaben, Historie, Melden-Knopf |
| `04-rangliste.png` | Gießsaison: 40 Erledigungen, 393 Liter, 8 Beteiligte |
| `05-veranstaltungen.png` | „Was ist los in Rössing" mit fünf Terminen |
| `06-profil.png` | „Mein Profil" mit den Sichtbarkeits-Schaltern (gemischt) |
| `07-startseite.png` | die Bereiche, Hitzehinweis, „5 Orte warten auf dich" |

## Sprachen: de-DE und en-US bekommen dieselben Bilder

Die App ist **durchgehend deutsch**; eine englische Oberfläche gibt es nicht.
Für `en-US` liegen deshalb dieselben Dateien. Das ist zulässig und ehrlicher,
als eine Übersetzung vorzutäuschen, die die App nicht hat — die englische
Store-Beschreibung sagt es ausdrücklich („The interface is in German only").

Sobald es eine englische Oberfläche gibt, wird `bilder_ablegen.py` je Sprache
einmal aufgerufen, statt zu kopieren.

## Hochladen

```sh
export APP_STORE_CONNECT_KEY_ID=75C8P9JB9F
export APP_STORE_CONNECT_ISSUER_ID=cbe512b7-1520-46f6-a7cf-6db8d1d7ac41

python3 store/check_ios_metadata.py                  # Maße und Vollzähligkeit
python3 store/asc.py screenshots-hochladen --probe   # erst zeigen …
python3 store/asc.py screenshots-hochladen           # … dann schicken
```

Der Unterbefehl sucht je Sprache und Gerätegröße den Bildsatz (oder legt ihn
an), **leert ihn** und lädt neu hoch — sonst wächst er mit jedem Lauf, und
Apple nimmt höchstens zehn Bilder je Satz. Anschließend setzt er die
Reihenfolge.

Anzeigetypen (nachgeschlagen, nicht geraten — Apple zählt die gültigen Werte
in der Fehlermeldung zu `POST /v1/appScreenshotSets` auf):

| Ordner | Anzeigetyp | Maße |
|---|---|---|
| `iphone-6_9` | `APP_IPHONE_67` | 1320×2868 (auch 1290×2796) |
| `ipad-13` | `APP_IPAD_PRO_3GEN_129` | 2064×2752 (auch 2048×2732) |

Für 6,9″ gibt es keinen eigenen Typ; die Größe gehört zu `APP_IPHONE_67`.

**Eingereicht wird nichts.** Das Hochladen füllt nur den Store-Eintrag; der
Klick auf „Zur Prüfung einreichen" bleibt ein Mensch.

## Aufräumen

`aufnehmen.sh` beendet seine Server selbst. Was liegen bleibt:

```sh
rm -f /tmp/dorf-shots.sqlite /tmp/dorf-shots.sqlite-shm /tmp/dorf-shots.sqlite-wal
rm -rf /tmp/dorf-shots-arbeit
rm -rf store/screenshots/pruefstand/DorfAufnahmen.xcodeproj   # erzeugt XcodeGen neu
xcrun simctl status_bar <udid> clear
```
