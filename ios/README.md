# Dorf-App für iOS

Native iOS-App zur Dorf-App Rössing — dieselbe Rössing-ID, dasselbe Backend,
dieselben Regeln wie in `android/`. Was das Backend entscheidet, entscheidet
die App nicht noch einmal.

Swift 6 und SwiftUI, **Mindestfassung iOS 16**, iPhone und iPad
(`TARGETED_DEVICE_FAMILY: "1,2"`). Einzige Fremdbibliothek ist MapLibre.

iOS 16 statt 17 ist eine Entscheidung fürs Dorf: iOS 17 läuft erst ab
iPhone XS/XR (2018), iOS 16 nimmt iPhone 8, 8 Plus und X (2017) dazu. Wer die
Blumenkästen gießt, hat nicht zwangsläufig das neueste Telefon. Tiefer geht
es bewusst nicht — ein iPhone 6s tut sich mit der Vektorkarte schwer, und
eine App zu versprechen, die dann ruckelt, wäre keine Wohltat.

Was das im Quelltext heißt: **kein Observation-Framework** (`@Observable`
gibt es erst ab iOS 17) — die Modelle sind `ObservableObject` mit
`@Published`, die Ansichten halten sie als `@StateObject`,
`@ObservedObject` oder `@EnvironmentObject`. Und **kein
`ContentUnavailableView`**: An seiner Stelle steht `Hinweistafel` unter
`Dorf/Design/`.

## Bauen und testen

```sh
cd ios
make projekt        # erzeugt Dorf.xcodeproj aus project.yml (XcodeGen)
make bauen          # Simulator-Build
make testen         # Unit-Tests, mit lokalen Adressen
make pruefen        # die Wache „Tests nur lokal" über den iOS-Teil
make oeffnen        # in Xcode öffnen
```

Gebraucht werden XcodeGen (`brew install xcodegen`) und Xcode 26 — Swift 6 mit
MainActor-Vorbelegung kann keine ältere Fassung.

`Dorf.xcodeproj` wird **erzeugt und nicht committet**: Eine `.pbxproj` ist
nicht lesbar zu prüfen und erzeugt bei jedem zweiten Merge einen Konflikt.
Wer eine Datei hinzufügt, ruft danach `make projekt` auf.

`make testen` setzt alle vier Adressen auf den eigenen Rechner um — dieselben
Werte wie `.github/workflows/ios.yml`. Der letzte grüne CI-Lauf meldet
**181 Tests in 11 Suiten**.

### Zwei Fallstricke, die Zeit gekostet haben

- **Ohne Signatur startet im Simulator nichts.** Mit
  `CODE_SIGNING_ALLOWED=NO` weist dyld das eingebettete `MapLibre.framework`
  ab, und der Testträger stirbt mit SIGKILL, ohne den Grund zu nennen. Es
  genügt eine **Ad-hoc-Signatur** (`CODE_SIGN_IDENTITY: "-"`) — ein
  Zertifikat braucht der Simulator nicht. So steht es in `project.yml`, im
  `Makefile` und in der CI.
- **Der Simulator muss vor `xcodebuild test` wirklich gebootet sein.** Steht
  er auf `Shutdown`, stirbt der Testträger reproduzierbar mit „Early
  unexpected exit / signal kill", ohne dass am Code etwas falsch wäre.
  `xcrun simctl bootstatus <id> -b` vorweg räumt das aus.

Beides steht auch in `CLAUDE.md`, Abschnitt „Entwicklungsumgebung (iOS)" —
zusammen mit dem Umstand, dass hier auf einer headless macOS-VM gearbeitet
wird, auf der kein Fenster jemandem etwas zeigt.

## Ausliefern

```sh
make store-pruefen   # Store-Metadaten, Icon, Asset-Kataloge
make archiv          # signiertes Archiv (Team-ID + App-Store-Connect-Schlüssel)
make ipa             # Export als app-store-connect
make hochladen       # erst --validate-app, dann --upload-app
```

Denselben Weg geht `.github/workflows/ios-release.yml`, ausgelöst von einem
Tag nach dem Muster `ios-v*`. Signiert wird **automatisch**
(`-allowProvisioningUpdates`); Zertifikat und Provisioning-Profil legt
`xcodebuild` bei Apple selbst an, der Ausweis dafür ist derselbe
App-Store-Connect-API-Schlüssel, mit dem später hochgeladen wird. Kein
fastlane, keine Marketplace-Action.

`project.yml` steht auf Ad-hoc-Signatur — richtig für den Simulator,
untauglich für den Store. Beide Wege übersteuern das für den einen Aufruf,
statt die Datei zu ändern.

Der Weg im Ganzen, mit allem, was ein Mensch bei Apple klicken muss:
`../README.md`, Abschnitt „Releases (iOS)", und `../store/ios-veroeffentlichung.md`.

## Aufbau

| Ort | Inhalt |
|---|---|
| `Dorf/Konfiguration.swift` | Adressen und Kennungen, aus Build-Einstellungen (`project.yml`) über die `Info.plist` |
| `Dorf/Umgebung.swift` | `AppUmgebung` — die Handverdrahtung, Gegenstück zu `AppContainer` auf Android; reicht die Meldungen von `Anmeldung` und `OrteModell` weiter, weil `ObservableObject` nicht durch verschachtelte Objekte hindurch beobachtet |
| `Dorf/Daten/Modelle.swift` | Die DTOs des Backends. Feldnamen 1:1 wie im JSON |
| `Dorf/Daten/DorfApi.swift` | Der einzige Weg zum Backend (`URLSession`, kein Framework) |
| `Dorf/Daten/DorfApi+*.swift` | Weitere Endpunkte als Anhänge — derselbe Transport |
| `Dorf/Push/` | Erlaubnis, Kanäle, Gerätekennung, Sprungziel (APNs, ohne Firebase) |
| `Dorf/Anmeldung/` | OIDC Authorization Code + PKCE (`ASWebAuthenticationSession`), Schlüsselbund, Anmeldebildschirm |
| `Dorf/Navigation/` | Startseite und Verdrahtung der Bereiche (`Ziel`) |
| `Dorf/Bereiche/<Bereich>/` | Je Bereich ein Ordner. Ein Bereich fasst keinen fremden an |
| `Dorf/Design/` | Ampel-Farben, Zahlen- und Datumsformate, `Hinweistafel` (leere Seiten), `Color.trennlinie` |
| `Dorf/Assets.xcassets/` | App-Icon (hell, dunkel, eingefärbt) und Akzentfarbe |
| `DorfTests/` | Unit-Tests (swift-testing) |

## Was die App kann

Nach der Anmeldung eine Startseite mit Bereichen — eine Liste, keine
Registerkarten, damit weitere Bereiche danebenpassen. Sie zeigt außerdem, wie
viele Orte gerade warten, und bei Hitze einen Hinweis.

| Bereich | Was er tut |
|---|---|
| **Mithelfen** | Orte als Karte oder Liste, Ampel je Aufgabe, Ortsdetail mit Plan, letzter Erledigung und Historie; melden nach Rückfrage, zurücknehmen; „Ich helfe hier mit" trägt als Helfer:in ein |
| **Dorfkarte** (keine eigene Seite, sondern Teil von „Mithelfen" und der Verwaltung) | MapLibre mit den Orten als Ampel-Nadeln, eigener Standort; im **Auswahlmodus** liefert ein Tipp auf die Fläche die Koordinate für die Verwaltung |
| **Was ist los in Rössing** | Termine aus `events.json` der Website — der einzige Abruf **ohne** Zugangstoken |
| **Rangliste** | Was das Dorf geschafft hat, wer wie viel, wo man selbst steht; fünf Zeiträume. Ohne Punkte und ohne Abzeichen für Versäumtes |
| **Anfragen und Hinweise** | Die Vergabe: wer gerade gefragt ist, zusagen und wieder abgeben, Hinweise bestätigen |
| **Mein Profil** | Eigene Angaben mit Sichtbarkeitsschaltern, die in Worten sagen, wer ein Feld sieht. Telefon, E-Mail und Notiz sind vorbelegt **nicht** öffentlich |
| **Dorfbewohner** | Wer mitmacht, mit antippbarer Rufnummer und E-Mail nach Freigabe |
| **Ideen** | „Sag uns, was die App können soll" — derselbe Eingang wie das Formular auf der Website, hier am Konto hängend |
| **Verwaltung** | Orte und Aufgaben pflegen, am Blumenkasten stehend; Hitzefaktor setzen. Sichtbar nur für Verwaltende — durchgesetzt wird das im Backend |
| **Einstellungen** | Abmelden, Konto löschen (`DELETE /api/v1/me`), was die App über sich selbst zu sagen hat |

Impressum und Datenschutz stehen auf der Startseite **und** auf dem
Anmeldebildschirm und verweisen auf die Website — eine zweite Fassung in der
App würde über kurz oder lang abweichen.

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
- **Zugänglichkeit:** Farbe allein ist nie die Information — die Ampel trägt
  immer auch einen Text (`Ampelpunkt`), Knöpfe haben
  `accessibilityIdentifier`.
- **Swift 6** mit `MainActor`-Vorbelegung. Netzarbeit läuft über `async/await`
  auf `URLSession`, nicht über eigene Threads. Die Datenschicht ist deshalb
  ausdrücklich `nonisolated`: Der API-Zugang wird außerhalb des Hauptthreads
  gebaut und dekodiert dort.

## Zugang zur Rössing-ID

Eigener nativer PKCE-Client im Zitadel-Projekt `dorf-app`:
`387943892076527811` („Dorf-App iOS"), Rücksprung
`de.roessing.app:/oauth2redirect`. Die Client-ID muss in `AUTH_AUDIENCE` des
Backends stehen (`deploy/overlays/production/deployment.yaml`), sonst weist
das Backend jedes Token dieser App ab.

Angefordert wird beim Login zusätzlich der Scope
`urn:zitadel:iam:org:projects:roles` („projects", Plural). Ohne ihn stellt
Zitadel ein Token **ganz ohne Rollen** aus — dann ist niemand Verwaltung.

`offline_access` hält die Sitzung über den Neustart hinweg; die Tokens liegen
im Schlüsselbund. Der Entwickler-Login ohne Zitadel gibt es nur im
Debug-Build und nur mit `DEV_AUTH=1` — in einem Release-Build ist er hart
aus (`Konfiguration.entwicklerLoginErlaubt`).

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

Einzige Ausnahme ist der Terminfeed: `events.json` der Website ist öffentlich
und wird ohne Zugangstoken gelesen.

## Push-Benachrichtigungen

Ohne Firebase: Die App meldet ihre **rohe APNs-Kennung** beim Dorfserver an,
der spricht direkt mit Apple (`backend/internal/push/apns.go`). Nach der
Erlaubnis gefragt wird **erst, wenn sich jemand als Helfer:in einträgt** —
der Systemdialog kommt einmal im Leben einer Installation, und dort ist
selbsterklärend, wofür. Push ist dabei die Abkürzung, nicht der Weg: Jede
Anfrage steht auch in der Abrufliste und erscheint beim nächsten Öffnen.

Die Berechtigung `aps-environment` kommt aus der Build-Einstellung
`APS_UMGEBUNG` (Debug → `development`, Release → `production`) und **muss**
zu `APNS_UMGEBUNG` im Backend passen. Eine Kennung an der falschen Adresse
ergibt `BadDeviceToken`, und das Backend wirft das Gerät weg, obwohl daran
nichts falsch ist.

## Was noch fehlt

Die vollständige Liste steht in `OFFEN.md`. Das Wichtigste daraus:

- **Im Cluster fehlt der APNs-Schlüssel.** Solange `APNS_KEY_FILE` in
  `deploy/overlays/production/deployment.yaml` auskommentiert ist, wird für
  iOS nicht gepusht — die App holt ihre Benachrichtigungen ohnehin selbst ab.
  Der Menüpfad zum `.p8` steht dort im Kommentar.
- **Der Tipp auf eine Push-Meldung öffnet die App, führt aber nicht zur
  Aufgabe.** `Benachrichtigungen.gemeinsam.beiTipp` ist vorgesehen und wird
  von niemandem gesetzt; `Ziel` kennt bislang nur den Bereich, nicht den
  einzelnen Ort. Auf Android ist derselbe Weg verdrahtet.
- **Die Verwaltung kann weniger als die Web-Verwaltung**: keine
  Träger-Auswahl, keine Sichtbarkeit `nur_mitglieder`, keine Befähigungen,
  keine nachgetragenen oder zurückgenommenen Erledigungen.
- **Die Karte lässt sich nicht auf einen Ort lenken**, und ein Kartentipp
  geht mit VoiceOver nicht — der barrierefreie Weg zur Koordinate ist
  „Meinen Standort übernehmen".
- **Keine E2E-Tests für iOS.** Wenn sie kommen, gilt dieselbe Regel wie
  überall: kein Test fasst einen entfernten Server an.
