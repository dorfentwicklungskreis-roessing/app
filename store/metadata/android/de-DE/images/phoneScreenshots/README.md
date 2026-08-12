# Screenshots (Telefon)

Dieses Verzeichnis ist **absichtlich leer**. Die Screenshots werden von Hand am
Emulator erzeugt und danach hier abgelegt.

## Vorgaben von Google Play

- Mindestens **2**, höchstens 8 Telefon-Screenshots (Play lässt eine App ohne
  Screenshots nicht veröffentlichen).
- PNG oder JPEG, **kein Alphakanal**.
- Seitenverhältnis 16:9 oder 9:16, kürzere Kante 320–3840 px.
  Praktisch: **1080 × 1920** (Hochformat) — genau das liefert ein Pixel-ähnlicher
  Emulator mit 1080p.
- Dateinamen bestimmen die Reihenfolge in der Store-Eintragung, deshalb
  durchnummerieren: `01-karte.png`, `02-ort.png`, …

## Erzeugen

Vorbereitung: Emulator **`testgeraet`** starten und das Release-nahe Build
(oder ein Debug-Build gegen das echte Backend) installieren. Anmelden mit dem
Testkonto **`test-dorf`** der Rössing-ID — nicht mit dem Entwickler-Login, sonst
stehen im Bild Dev-Bezeichnungen.

```sh
# Emulator starten (falls nicht schon offen)
"$ANDROID_HOME/emulator/emulator" -avd testgeraet -no-snapshot-load &
adb wait-for-device

# App bauen und installieren (gegen das Produktiv-Backend)
cd android && ./gradlew installDebug

# Statusleiste aufräumen, damit im Bild keine Debug-Icons/Uhrzeiten stören
adb shell settings put global sysui_demo_allowed 1
adb shell am broadcast -a com.android.systemui.demo -e command enter
adb shell am broadcast -a com.android.systemui.demo -e command clock -e hhmm 0930
adb shell am broadcast -a com.android.systemui.demo -e command battery -e level 100 -e plugged false
adb shell am broadcast -a com.android.systemui.demo -e command network -e wifi show -e level 4
adb shell am broadcast -a com.android.systemui.demo -e command notifications -e visible false

# Je Bildschirm: in der App hinnavigieren, dann
adb exec-out screencap -p > 01-karte.png

# Demo-Modus wieder aus
adb shell am broadcast -a com.android.systemui.demo -e command exit
```

Danach die Dateien in dieses Verzeichnis legen und prüfen:

```sh
identify store/metadata/android/de-DE/images/phoneScreenshots/*.png
```

## Welche Bildschirme

In dieser Reihenfolge, sie erzählt die Geschichte der App:

1. **Karte** mit mehreren Orten in Grün/Gelb/Rot
2. **Ort mit Aufgaben** (Gießen/Jäten, Fälligkeit, Verlauf)
3. **Bestätigungsdialog** „Hast du wirklich gegossen?"
4. **Rangliste** (Zeitraum Saison, mit Gesamtsummen des Dorfes)
5. **Liste** aller Orte (optional)

Auf echte Daten achten: keine Testeinträge mit Fantasienamen, keine leeren
Bildschirme, keine Fehlermeldungen im Bild.

## Nicht vergessen

Die Screenshots zeigen die deutsche Oberfläche und werden nur unter `de-DE`
gepflegt. Für `en-US` verwendet Play automatisch die Bilder der Standardsprache,
solange dort keine eigenen liegen.
