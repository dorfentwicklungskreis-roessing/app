# Play-Store-Eintragung

Alles, was für die Veröffentlichung der Dorf-App im Google Play Store ohne
Play-Console-Konto vorbereitbar ist.

```
store/
├── metadata/android/          Fastlane-Format (de-DE, en-US)
│   └── de-DE/
│       ├── title.txt              Titel, max. 30 Zeichen
│       ├── short_description.txt  max. 80 Zeichen
│       ├── full_description.txt   max. 4000 Zeichen
│       ├── changelogs/<vc>.txt    Änderungshinweise je versionCode, max. 500
│       └── images/
│           ├── icon.png           512×512
│           ├── featureGraphic.png 1024×500
│           └── phoneScreenshots/  fehlen noch, siehe README dort
├── assets/                    SVG-Quellen + render.sh für die Grafiken
├── check_metadata.py          prüft Limits, Vollständigkeit und Bildmaße
├── datenschutz.md             Kurzfassung, Vorlage für die Seite auf roessing.de
├── data-safety.md             Antworten für das Formular „Datensicherheit"
├── content-rating.md          Antworten für den IARC-Fragebogen
└── veroeffentlichung.md       Schritt für Schritt in der Play Console
```

## Metadaten prüfen

```sh
python3 store/check_metadata.py
```

Läuft ohne Abhängigkeiten und wird von `.github/workflows/store.yml` bei jeder
Änderung an `store/` oder `android/app/build.gradle.kts` ausgeführt, außerdem im
Release-Workflow vor dem Play-Upload.

## Grafiken neu erzeugen

```sh
bash store/assets/render.sh      # braucht ImageMagick + DejaVu-Schriften
```

Icon und Feature-Grafik entstehen aus den SVG-Quellen in `store/assets/` und
sind als PNG committet. Das Icon ist der sichtbare 72dp-Ausschnitt des Adaptive
Icons der App, in denselben Farben (`#3B6939`, `#FFF3C4`, `#F9A825`).

## Store-Tauglichkeit (Stand der Prüfung)

| Punkt | Wert | Bewertung |
|---|---|---|
| Paketname | `de.roessing.app` | passt zur Domain, nach dem ersten Upload unveränderlich |
| `targetSdk` / `compileSdk` | 35 (Android 15) | erfüllt Googles Vorgabe für neue Apps |
| `minSdk` | 26 (Android 8) | erreicht praktisch jedes Gerät im Dorf |
| AAB-Größe | ca. 18,7 MB (Release `v0.1.1`) | weit unter dem Play-Limit; der universelle APK ist mit 44,8 MB größer, weil er alle ABIs für MapLibre enthält — über das AAB bekommt jedes Gerät nur seine |
| Signierung | Release-Keystore aus den Repo-Secrets | wird bei Play App Signing zum Upload-Schlüssel |
| Minifizierung | `isMinifyEnabled`, `isShrinkResources` im Release | in Ordnung |

### Berechtigungen im Manifest

| Berechtigung | Begründung |
|---|---|
| `INTERNET` | Ohne Netz geht nichts: REST-API des Dorfservers, OIDC-Anmeldung gegen die Rössing-ID, Kartenkacheln von OpenFreeMap. |
| `ACCESS_NETWORK_STATE` | Von OkHttp/AppAuth erwartet; die App unterscheidet „kein Netz" von „Serverfehler" und zeigt sonst weiter den letzten Stand. |
| `ACCESS_FINE_LOCATION` | Entfernung zum jeweiligen Blumenkasten und Sortierung „was ist in meiner Nähe". Einzelabfrage im Vordergrund, Position bleibt auf dem Gerät. |
| `ACCESS_COARSE_LOCATION` | Fällt bei einer Standortfreigabe ohnehin mit an und reicht für die Entfernungsanzeige; wer nur den ungefähren Ort freigibt, kann die Funktion trotzdem nutzen. |

Kein `ACCESS_BACKGROUND_LOCATION`, keine Kamera, kein Speicherzugriff, keine
Kontakte, keine Benachrichtigungen. `INTERNET` und `ACCESS_NETWORK_STATE`
allein reichen **nicht** mehr, seit die Karte die eigene Position zeigt — die
beiden Standortberechtigungen sind für diese Funktion nötig und in der
Store-Beschreibung sowie in `data-safety.md` begründet.

## Offene Änderung am Android-Build: versionCode

**Der `versionCode` steht in `android/app/build.gradle.kts` fest verdrahtet
(aktuell `2`).** Play nimmt jeden Code nur ein einziges Mal an: Wer zweimal
hintereinander taggt, ohne die Zahl von Hand zu erhöhen, bekommt beim zweiten
Upload `403 … version code that has already been used`.

Vorschlag (gehört in `defaultConfig`, muss von der Person gemacht werden, die
`android/` betreut):

```kotlin
// versionCode aus der CI-Laufnummer, damit er monoton steigt und nie doppelt
// bei Play ankommt. Lokal (ohne CI) bleibt 1, das reicht für Debug-Builds.
versionCode = (System.getenv("GITHUB_RUN_NUMBER") ?: "1").toInt()
versionName = "0.1.1"
```

Im Workflow ist dafür nichts zu tun — `GITHUB_RUN_NUMBER` setzt GitHub Actions
selbst. Wichtig: die Laufnummer ist pro Workflow eindeutig und steigt monoton,
sie darf aber nie kleiner werden als ein bereits hochgeladener Code. Falls der
erste manuelle Upload mit `versionCode 2` erfolgt und der Release-Workflow bei
Lauf 3 steht, passt das; sonst muss einmalig ein Offset addiert werden
(`+ 100`).

Alternative, wenn der Code aus dem Tag kommen soll:

```kotlin
// v1.2.3 -> 10203
versionCode = System.getenv("GITHUB_REF_NAME")
    ?.removePrefix("v")?.split(".")?.takeIf { it.size == 3 }
    ?.let { (a, b, c) -> a.toInt() * 10000 + b.toInt() * 100 + c.toInt() }
    ?: 1
```

Das ist nachvollziehbarer, verlangt aber Disziplin beim Taggen (jeder Tag muss
größer sein als der vorige) und einen Fallback für Builds ohne Tag.

**Solange nichts davon umgesetzt ist:** vor jedem Release den `versionCode` von
Hand erhöhen und den passenden Änderungshinweis unter
`metadata/android/*/changelogs/<versionCode>.txt` anlegen — `check_metadata.py`
schlägt sonst fehl.

## Was noch fehlt

- [ ] Telefon-Screenshots (`metadata/android/de-DE/images/phoneScreenshots/`)
- [ ] Öffentliche Datenschutzerklärung auf roessing.de + URL in der Play Console
- [ ] Öffentliche Seite zur Konto-/Datenlöschung auf roessing.de
- [ ] `versionCode`-Automatik (siehe oben)
- [ ] Play-Console-Konto, Service-Account, Secret `PLAY_SERVICE_ACCOUNT_JSON`
      (Anleitung: `veroeffentlichung.md`)
