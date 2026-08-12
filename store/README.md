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

## versionCode und Änderungshinweise (automatisch)

`versionCode` und `versionName` kommen aus dem Git-Tag, nicht mehr aus der
CI-Laufnummer oder aus einer von Hand gepflegten Zahl:

```
v0.1.3  ->  versionName 0.1.3   versionCode 1000103
```

Die Formel steht in `scripts/naechste_version.py` (`version_code`) und noch
einmal in `android/app/build.gradle.kts`:
`1 000 000 + major*10000 + minor*100 + patch`. Der Release-Workflow lässt sich
den Wert vom Gradle-Build ausgeben (`./gradlew -q :app:versionInfo`) und
vergleicht ihn mit der Berechnung des Skripts — laufen die beiden Stellen
auseinander, bricht das Release ab.

**Startpunkt:** Der Sockel von 1.000.000 liegt weit über allem, was vorher
vergeben wurde (`2` beim Handbuild, `100 + Laufnummer` in der Laufnummern-Ära),
und wächst streng mit der Version, solange `minor` und `patch` unter 100
bleiben. Play nimmt jeden Code nur einmal an; ein Rückschritt ist damit
ausgeschlossen. Für lokale Builds ohne Tag gilt `0.0.0-dev` / `1000000`.

**Die Änderungshinweise entstehen automatisch.** `scripts/aenderungsnotiz.py`
schreibt beim Release `metadata/android/<locale>/changelogs/<versionCode>.txt`
aus den `feat:`- und `fix:`-Commit-Betreffen seit dem letzten Tag:

```sh
python3 scripts/aenderungsnotiz.py --version 0.1.3            # nur anzeigen
python3 scripts/aenderungsnotiz.py --version 0.1.3 --schreiben
```

`chore:`, `ci:`, `test:`, `docs:`, `refactor:`, Merge-Commits und alles im
Bereich `(ci)`/`(e2e)` fallen raus; gekürzt wird auf Googles 500 Zeichen.
`de-DE` bekommt die Aufzählung, `en-US` eine kurze generische Fassung — die
Commits sind auf Deutsch, und eine maschinelle Übersetzung wäre schlechter als
ein ehrlicher Einzeiler. Wer einen schöneren Text will, überschreibt die Datei
einfach von Hand; erzeugt wird sie nur, wenn das Release läuft.

Warum die Datei zweimal entsteht (im Autorelease-Lauf für `main` und noch
einmal im Release-Lauf): Der Tag zeigt bewusst auf genau den Commit, der grün
getestet wurde — die Notiz liegt darin also nicht. Der Release-Lauf baut sie
aus derselben Commit-Spanne neu auf (gleiches Ergebnis) und benutzt sie für
Play, GitHub-Release und Firebase; parallel landet sie auf `main`, damit
`check_metadata.py` sie dauerhaft vorfindet.

Codes unter 1.000.000 (`2.txt` von früher) gibt es nicht mehr — der Text steckt
jetzt in `1000100.txt`, dem versionCode von `v0.1.0`.

## Was noch fehlt

- [ ] Telefon-Screenshots (`metadata/android/de-DE/images/phoneScreenshots/`)
- [ ] Öffentliche Datenschutzerklärung auf roessing.de + URL in der Play Console
- [ ] Öffentliche Seite zur Konto-/Datenlöschung auf roessing.de
- [ ] Play-Console-Konto, Service-Account, Secret `PLAY_SERVICE_ACCOUNT_JSON`
      (Anleitung: `veroeffentlichung.md`)
