# Screenshots (Telefon)

Hier liegen die sechs Telefon-Screenshots der Store-Eintragung. Sie werden am
Emulator erzeugt; diese Anleitung beschreibt, wie man sie reproduziert.

| Datei | Bildschirm |
|---|---|
| `01-karte.png` | Karte mit den Ampel-Markern aller Pflege-Orte |
| `02-liste.png` | Liste der Orte, nach Dringlichkeit sortiert, mit Entfernung |
| `03-ort-mit-aufgaben.png` | Ort mit Gießplan, Historie und Melden-Knopf |
| `04-melden-bestaetigen.png` | Rückfrage „Hast du wirklich gegossen?" |
| `05-rangliste.png` | Rangliste (Saison) mit Podest und Dorf-Gesamtsummen |
| `06-anmeldung.png` | Anmeldung mit der Rössing-ID |

## Vorgaben von Google Play

- Mindestens **2**, höchstens 8 Telefon-Screenshots (Play lässt eine App ohne
  Screenshots nicht veröffentlichen).
- PNG oder JPEG, **kein Alphakanal**.
- Seitenverhältnis 16:9 oder 9:16, kürzere Kante 320–3840 px.
  Praktisch: **1080 × 1920** (Hochformat).
- Dateinamen bestimmen die Reihenfolge in der Store-Eintragung, deshalb
  durchnummerieren: `01-karte.png`, `02-liste.png`, …

Das AVD `testgeraet` hat 1080 × 2340 — das ist **9:19,5 und damit zu hoch für
Play**. Deshalb wird die Anzeige vor der Aufnahme auf 1080 × 1920 gesetzt
(`adb shell wm size 1080x1920`), statt hinterher zuzuschneiden: so bleibt das
Layout vollständig und nichts wird abgeschnitten.

## Beispieldaten

In der Produktion gibt es bislang nur die beiden Kästen „Unter den Eichen".
Eine Karte mit zwei Punkten und eine leere Rangliste sind kein guter
Store-Eintritt, deshalb entstehen die Bilder gegen ein **lokales Backend mit
Beispieldaten**: dieselbe App, plausible Rössinger Orte, gemischte Ampel und
Erledigungen mehrerer Personen. Erfundene Funktionen zeigen die Bilder nicht.

```sh
# Backend lokal mit Beispieldaten
cd backend
DB_PATH=/tmp/shots.sqlite AUTH_MODE=insecure-dev SEED=1 LISTEN_ADDR=:8099 \
  PUBLIC_URL=http://localhost:8099 RATE_LIMIT=off BACKUP=off go run ./cmd/server

# Orte, Aufgaben und Erledigungen anlegen: über die REST-API
# (POST /api/v1/places, .../tasks, .../completions). Zurückdatierte Meldungen
# brauchen "force": true und ein Admin-Token; der Anzeigename gehört in den
# JSON-Körper ("name"), nicht ins Token — HTTP-Header vertragen keine Umlaute.
```

## Erzeugen

```sh
# Emulator starten
"$ANDROID_HOME/emulator/emulator" -avd testgeraet -no-window -no-snapshot &
adb wait-for-device

# App gegen das lokale Backend bauen und installieren
cd android && ./gradlew installDebug -PdevAuth=true -PapiBaseUrl=http://10.0.2.2:8099

# Anzeige auf das Play-Format bringen
adb shell wm size 1080x1920 && adb shell wm density 420

# Standort setzen, damit die Liste Entfernungen zeigt
adb emu geo fix 9.8162 52.1843
adb shell pm grant de.roessing.app android.permission.ACCESS_FINE_LOCATION

# Statusleiste aufräumen, damit im Bild keine Debug-Icons/Uhrzeiten stören
adb shell settings put global sysui_demo_allowed 1
adb shell am broadcast -a com.android.systemui.demo -e command enter
adb shell am broadcast -a com.android.systemui.demo -e command clock -e hhmm 0930
adb shell am broadcast -a com.android.systemui.demo -e command battery -e level 100 -e plugged false
adb shell am broadcast -a com.android.systemui.demo -e command network -e wifi show -e level 4 -e fully true
adb shell am broadcast -a com.android.systemui.demo -e command network -e mobile hide
adb shell am broadcast -a com.android.systemui.demo -e command notifications -e visible false

# Je Bildschirm: in der App hinnavigieren, dann
adb exec-out screencap -p > roh.png

# Demo-Modus wieder aus
adb shell am broadcast -a com.android.systemui.demo -e command exit
```

`screencap` liefert RGBA. Play will **keinen Alphakanal**, deshalb vor dem
Ablegen umwandeln:

```sh
convert roh.png -alpha remove -alpha off -depth 8 -define png:color-type=2 01-karte.png
```

Den **Anmeldebildschirm** aus einem Build *ohne* `-PdevAuth=true` aufnehmen,
sonst steht der Knopf „Entwickler-Login (nur Test)" im Bild.

Danach prüfen — `store/check_metadata.py` kontrolliert Anzahl und Kantenlängen:

```sh
identify store/metadata/android/de-DE/images/phoneScreenshots/*.png
python3 store/check_metadata.py
```

## Nicht vergessen

Die Screenshots zeigen die deutsche Oberfläche und werden **nur unter `de-DE`**
gepflegt. Für `en-US` verwendet Play automatisch die Bilder der Standardsprache,
solange dort keine eigenen liegen — Kopien wären nur doppelte Pflegelast.
