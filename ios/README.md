# Dorf-App für iOS

Native iOS-App zur Dorf-App Rössing — dieselbe Rössing-ID, dasselbe Backend,
dieselben Regeln wie in `android/`. Was das Backend entscheidet, entscheidet
die App nicht noch einmal.

## Bauen

```sh
cd ios
make projekt        # erzeugt Dorf.xcodeproj aus project.yml
make bauen          # Simulator-Build
make testen         # Unit-Tests
make oeffnen        # in Xcode öffnen
```

`Dorf.xcodeproj` wird **erzeugt und nicht committet**: Eine `.pbxproj` ist
nicht lesbar zu prüfen und erzeugt bei jedem zweiten Merge einen Konflikt.
Wer eine Datei hinzufügt, ruft danach `make projekt` auf.

## Aufbau

| Ort | Inhalt |
|---|---|
| `Dorf/Konfiguration.swift` | Adressen und Kennungen, aus Build-Einstellungen (`project.yml`) |
| `Dorf/Daten/Modelle.swift` | Die DTOs des Backends. Feldnamen 1:1 wie im JSON |
| `Dorf/Daten/DorfApi.swift` | Der einzige Weg zum Backend (`URLSession`, kein Framework) |
| `Dorf/Daten/DorfApi+*.swift` | Weitere Endpunkte als Anhänge — derselbe Transport |
| `Dorf/Push/` | Erlaubnis, Kanäle, Gerätekennung (APNs, ohne Firebase) |
| `Dorf/Anmeldung/` | OIDC Authorization Code + PKCE, Schlüsselbund, Anmeldebildschirm |
| `Dorf/Navigation/` | Startseite und Verdrahtung der Bereiche (`Ziel`) |
| `Dorf/Bereiche/<Bereich>/` | Je Bereich ein Ordner. Ein Bereich fasst keinen fremden an |
| `Dorf/Design/` | Ampel-Farben, Zahlen- und Datumsformate |
| `DorfTests/` | Unit-Tests (swift-testing) |

## Regeln

- **Deutsch.** Bezeichner, Kommentare und alle Texte der Oberfläche. Die
  DTO-Feldnamen bleiben englisch — sie sind der JSON-Vertrag des Backends.
- **Keine Fremdbibliotheken** außer MapLibre (Karte). Netz, JSON, Anmeldung
  und Ablage macht die Standardbibliothek. Jede Abhängigkeit müsste über
  Jahre mitgepflegt werden.
- **Das Backend entscheidet.** Sichtbarkeit, Sperrfristen, Rollen und
  Ablehnungsgründe kommen von dort; die App zeigt sie an, statt sie
  nachzubauen. Fehlertexte des Backends werden im Wortlaut angezeigt.
- **Keine Adresse im Quelltext.** Alles, was auf einen Server zeigt, steht in
  `project.yml` und ist über `xcodebuild API_BASE_URL=…` übersteuerbar. Kein
  Test darf gegen die Produktion laufen
  (`.github/scripts/pruefe_lokale_tests.py`).
- **Zugriffbarkeit:** Farbe allein ist nie die Information — die Ampel trägt
  immer auch einen Text (`Ampelpunkt`), Knöpfe haben
  `accessibilityIdentifier`.
- **Swift 6** mit `MainActor`-Vorbelegung. Netzarbeit läuft über `async/await`
  auf `URLSession`, nicht über eigene Threads.

## Zugang zur Rössing-ID

Eigener nativer PKCE-Client im Zitadel-Projekt `dorf-app`:
`387943892076527811` („Dorf-App iOS"), Rücksprung
`de.roessing.app:/oauth2redirect`. Die Client-ID muss in `AUTH_AUDIENCE` des
Backends stehen (`deploy/overlays/production/deployment.yaml`), sonst weist
das Backend jedes Token dieser App ab.

Angefordert wird beim Login zusätzlich der Scope
`urn:zitadel:iam:org:projects:roles` („projects", Plural). Ohne ihn stellt
Zitadel ein Token **ganz ohne Rollen** aus — dann ist niemand Verwaltung.

## Genau ein Weg zum Backend

Alles, was die App vom Server holt oder dorthin schickt, geht durch
`DorfApi` — auch das, was in Anhängen steht (`DorfApi+Vergabe.swift`,
`+Verwaltung`, `+Konto`, `Push/DorfApi+Geraete.swift`). Die Transport-Helfer
(`hole`, `schicke`, `schickeOhneAntwort`, `fehler`) sind deshalb `internal`;
`basis`, `sitzung` und `tokenGeber` bleiben `private`.

Das ist keine Förmlichkeit: Als die Helfer noch `private` waren, hat sich
jeder Bereich seinen eigenen Transport gebaut, und Fristen, Kopfzeilen und
Fehlerübersetzung liefen auseinander. Wer einen Endpunkt ergänzt, benutzt
die Helfer und schreibt das DTO zu den übrigen in `Modelle.swift`.

## Push-Benachrichtigungen

Ohne Firebase: Die App meldet ihre **rohe APNs-Kennung** beim Dorfserver an,
der spricht direkt mit Apple (`backend/internal/push/apns.go`). Nach der
Erlaubnis gefragt wird **erst, wenn sich jemand als Helfer:in einträgt** —
der Systemdialog kommt einmal im Leben einer Installation, und dort ist
selbsterklärend, wofür. Push ist dabei die Abkürzung, nicht der Weg: Jede
Anfrage steht auch in der Abrufliste und erscheint beim nächsten Öffnen.

## Was noch fehlt

- Apple-Signierung: `DEVELOPMENT_TEAM` in `project.yml` ist leer, gebaut wird
  bislang nur für den Simulator.
- Der Tipp auf eine Push-Meldung öffnet die App, führt aber noch nicht zur
  Aufgabe.
- Weiteres in `OFFEN.md`.
