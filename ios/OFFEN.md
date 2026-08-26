# Offene Punkte der iOS-App

Was die App bewusst **nicht** kann, und was beim Bauen aufgefallen ist.
Nichts davon ist ein Fehler — es ist der Zuschnitt: Die App sollte zuerst
das können, wofür es die Dorf-App gibt (mithelfen, sehen, melden), und nicht
alles auf einmal halb.

Die Blocker, die während des Baus in der Grundlage steckten (MainActor-
Isolation der Datenschicht, Modulname, Signierung im Simulator), sind
behoben — siehe `CLAUDE.md`, Abschnitt „Entwicklungsumgebung (iOS)".

Diese Datei sammelt, was vorher in `OFFEN-konto.md`, `OFFEN-verwaltung.md`
und `OFFEN-push.md` stand. Die drei Dateien gab es, weil mehrere Zweige
gleichzeitig an der App gebaut haben; sie sind eingearbeitet und gelöscht.

## Bewusst noch nicht gebaut

### Verwaltung

- **Träger-Auswahl.** `PlaceInput.traegerId` geht nicht mit. Das Backend
  nimmt dann den einzigen Träger, den der Aufrufer verwaltet — im Alltag der
  Normalfall. Wer **mehrere** Träger verwaltet, kann in der App nicht
  auswählen, für wen er anlegt (die Web-Verwaltung kann es). Ein Ort lässt
  sich aus der App auch nicht umhängen.
- **Sichtbarkeit `nur_mitglieder`.** `TaskInput.sichtbarkeit` wird nicht
  mitgeschickt; leer heißt im Backend „unverändert lassen". Eine interne
  Aufgabe bleibt also intern, wenn man ihr Intervall ändert — aber intern
  **machen** kann man sie aus der App nicht.
- **Befähigungen** (`TaskInput.befaehigungId`) ebenso: nicht geschickt, also
  unverändert. Die verlangte Einweisung wird in der App weder gezeigt noch
  gesetzt.
- **Vergabe-Einstellungen.** `PUT /api/v1/settings` schickt **nur**
  `wateringFactor`. Die Vergabe-Regeln stehen in derselben Antwort, gehören
  aber dem Bereich „Anfragen" — ein Zug am Hitzefaktor darf sie nicht
  überschreiben.
- **Erledigungen nachtragen und zurücknehmen** (der `forced`-Nachtrag der
  Web-Verwaltung) gibt es in der App nicht.
- **Ideen verwalten** (`GET/PATCH/DELETE /api/v1/ideen`) ist ebenfalls
  `adminOnly`, gehört aber zum Bereich „Ideen".

### Push

- **Der Fingertipp führt noch nicht zur Aufgabe.**
  `Benachrichtigungen.gemeinsam.beiTipp` ist ein Rückruf mit einem `PushZiel`
  (Ort, Aufgabe, Vorgang, Art); gesetzt wird er von niemandem. Solange das so
  ist, wird die Meldung angezeigt und der Tipp öffnet schlicht die App — kein
  Fehler, nur ein verschenkter Weg. Wer ihn einhängt, braucht dafür einen
  Navigationspfad bis zur Ortsseite (`Ziel` kennt bislang nur den Bereich,
  nicht den einzelnen Ort).
- **Keine Knöpfe an der Meldung.** Die `UNNotificationCategory`n sind ohne
  `UNNotificationAction` angelegt. „Übernehmen" direkt aus der Meldung heraus
  wäre jetzt möglich (`DorfApi.zusagen`), ist aber nicht gebaut.
- **Keine `interruption-level: time-sensitive`.** Das würde Anfragen auch
  durch „Nicht stören" hindurchlassen, verlangt aber die zusätzliche
  Berechtigung *Time Sensitive Notifications* im Entwicklerportal. Solange
  die nicht eingerichtet ist, bleibt es bei `apns-priority: 10` — das reicht
  für sofortige Zustellung.
- **Kein Zähler am App-Symbol** (Badge). Die Erlaubnis dafür wird mitgefragt,
  benutzt wird sie nicht: Eine ehrliche Zahl bräuchte einen eigenen Abruf.
- **Im Simulator kommt keine echte Gerätekennung an.** Das ist normal und
  kein Fehler. Prüfbar ist im Simulator nur die Anzeige
  (`xcrun simctl push <geräte-id> de.roessing.app <datei.apns>`) — der Weg
  über Apple braucht ein echtes iPhone.

## Kleinere Lücken

- **Termine auf der Dorfkarte.** `Termin.koordinate` ist gefüllt, sobald die
  Website Koordinaten am Ort pflegt. Auf der Karte wären sie eine **zweite**
  GeoJSON-Quelle mit eigener Ebene — andere Form und Farbe als die
  Ampel-Nadeln, sonst verwechselt man beides. `Kartendaten` arbeitet bereits
  auf `[Kartenmerkmal]` statt auf `[Ort]` und ist dafür vorbereitet; die
  Signatur von `KarteView` müsste um `termine:` wachsen.
- **Karte kann nicht auf einen Ort zeigen.** Sie meldet einen Tipp zurück,
  nimmt aber keine Anweisung entgegen — beim Umschalten von der Liste zur
  Karte lässt sich der gewählte Ort deshalb nicht anfahren. Dasselbe fehlt im
  Auswahlmodus der Verwaltung: Die Auswahlkarte startet auf dem Ausschnitt
  aller Orte, nicht auf dem eigenen Standort. Wer „Meinen Standort
  übernehmen" drückt, sieht den gesetzten Punkt erst nach dem Hinschieben.
- **Kartentipp und VoiceOver.** Auf eine bestimmte Stelle der Karte zu
  tippen, geht mit VoiceOver nicht. Der barrierefreie Weg zur Koordinate ist
  „Meinen Standort übernehmen" — das Formular sagt das auch, und die Karte
  trägt im Auswahlmodus einen entsprechenden Hinweis. Wer einen Ort anlegt,
  an dem er nicht steht, braucht bis auf Weiteres die Web-Verwaltung.
- **Der Hitzefaktor steht in Zehntelschritten.** Ein vom Backend geliefertes
  0,75 bleibt erhalten, lässt sich mit dem Stepper aber nur in Zehnteln
  verändern.
- **Keine Suche und keine Sortierung** in der Ortsliste der Verwaltung; sie
  steht nach Dringlichkeit wie in „Mithelfen". Ab einigen Dutzend Orten wird
  das unhandlich.
- **Kein Zwischenspeicher je Zeitraum** in der Rangliste: Jedes Umschalten
  fragt neu. Bei fünf Zeiträumen und kleinen Antworten verschmerzbar.

## Was iOS 16 als Mindestfassung bedeutet

Die App läuft ab **iOS 16** (vorher iOS 17), damit iPhone 8, 8 Plus und X
(2017) mitkommen. Tiefer geht es bewusst nicht: Ein iPhone 6s tut sich mit
der Vektorkarte schwer. Was dafür anders ist als in einer iOS-17-App:

- **Kein Observation-Framework.** Die dreizehn Modelle sind
  `ObservableObject` mit `@Published`; die Ansichten halten sie als
  `@StateObject`, `@ObservedObject` oder `@EnvironmentObject`. Das beobachtet
  **gröber** als `@Observable`: Jede Meldung zeichnet die ganze Ansicht neu,
  nicht nur das gelesene Feld. Für eine App dieser Größe ist das nicht zu
  merken — wer aber eine Eigenschaft **ohne** `@Published` hinzufügt,
  bekommt eine Oberfläche, die sich nicht mehr aktualisiert, und **kein Test
  merkt das**: Die Tests prüfen die Modelle direkt.
- **`ObservableObject` beobachtet nicht durch verschachtelte Objekte
  hindurch.** `AppUmgebung` reicht die Meldungen von `Anmeldung` und
  `OrteModell` deshalb ausdrücklich weiter (siehe `Umgebung.swift`). Kommt
  ein drittes eigenes Modell in die Umgebung, gehört es dort dazu — sonst
  bleibt die Startseite stehen.
- **Kein `ContentUnavailableView`.** An seiner Stelle steht `Hinweistafel`
  (`Dorf/Design/Hinweistafel.swift`), gleiches Aussehen, gleiche
  Zugänglichkeit. Ihre Texte sind jetzt unsere, nicht mehr Apples — eine
  neue Systemsprache übersetzt sie also nicht mit.
- **Ältere Fassungen einiger SwiftUI-Aufrufe**: `onChange` ohne `initial:`
  und ohne alten Wert, `navigationDestination(isPresented:)` statt
  `(item:)`, `Color.trennlinie` statt `ShapeStyle.separator`. Alle vier sind
  ab iOS 17 als veraltet markiert; solange die Mindestfassung 16 ist, warnt
  der Übersetzer nicht.
- **Ein `if #available` gibt es nirgends.** Für jede benutzte iOS-17-API gab
  es eine ältere Fassung mit demselben Verhalten; keine Funktion wurde
  weggelassen.
- **Geprüft wurde bisher nur auf einem iOS-26-Simulator.** Das Mindest-Ziel
  ist gesenkt, ein Lauf auf einem echten iOS-16-Gerät (oder wenigstens einer
  iOS-16-Simulator-Laufzeit) steht aus — dort verhalten sich `List`,
  `NavigationStack` und `.searchable` in Kleinigkeiten anders.

## Aus der Zusammenführung der Zugänge

Seit `refactor(ios): genau ein Weg zum Backend` gibt es wieder genau **eine**
Transportschicht: `DorfApi`. Die Helfer (`adresse`, `anfrage`, `hole`,
`schicke`, `schickeOhneAntwort`, `ausfuehren`, `rohAusfuehren`, `fehler`)
sind `internal`, damit die Anhänge in `DorfApi+*.swift` und
`Push/DorfApi+Geraete.swift` sie benutzen können; `basis`, `sitzung` und
`tokenGeber` bleiben `private`. Wer einen Endpunkt ergänzt, benutzt die
Helfer — eine zweite Sitzung mit anderen Fristen kann er von außen gar nicht
mehr bauen.

Was dabei offen geblieben ist:

- **Der Ersatzsatz bei 409 ist jetzt überall derselbe** („Das hat gerade
  jemand anderes übernommen."). Die Verwaltung hatte einen eigenen („…
  geändert."). Zu sehen ist der Unterschied nur, wenn das Backend einen 409
  **ohne** Begründung im Rumpf schickt — dann sagt ohnehin niemand etwas
  Genaues. Wenn ein Endpunkt einen eigenen Wortlaut braucht, gehört er ins
  Backend, nicht in die App.
- **Der Konto-Endpunkt deutet seine Antwort selbst.** `DELETE /api/v1/me`
  darf mit leerem Rumpf antworten — gelöscht ist gelöscht. Er ist deshalb der
  einzige Aufruf, der `rohAusfuehren` direkt benutzt statt `hole`/`schicke`.
- **Kein Zwischenspeicher, keine Wiederholung, keine Warteschlange.** Der
  Transport schickt und wartet; wer im Funkloch meldet, bekommt den Hinweis
  und versucht es selbst noch einmal. Für ein Dorf mit ein paar Dutzend
  Meldungen am Tag ist das die ehrlichere Bauweise als eine Ablage, die
  irgendwann irgendetwas nachschickt.

## Gehört anderen Dateien im Repo

Diese Punkte liegen außerhalb von `ios/` und wurden hier nur aufgeschrieben.

**Erledigt** (im selben Zweig nachgezogen):

- **`backend/README.md`** führt `DELETE /api/v1/me` inzwischen als eigenen
  Abschnitt.
- **`backend/SICHERHEIT.md`** hat den Abschnitt „Kontolöschung": was gelöscht
  wird (Profil, Gerätekennungen, Helfer-Eintragungen, Benachrichtigungen,
  Befähigungsanträge), was anonymisiert bleibt (Erledigungen, beendete
  Zusagen, eingereichte Ideen) und dass das Konto in der **Rössing-ID**
  ausdrücklich **nicht** angetastet wird. Dazu den Abschnitt „Gerätekennung
  für iOS: APNs statt Firebase".
- **`README.md`** im Wurzelverzeichnis nennt unter „Push-Benachrichtigungen"
  jetzt beide Wege — Firebase für Android, APNs für iOS.
- **`store/datenschutz.md`** nennt den Weg in der App (Einstellungen → Konto
  löschen) und erklärt die Anonymisierung.
- **`store/ios-datenschutz.md`** führt die APNs-Gerätekennung mit Apple als
  Empfänger.

**Noch offen:**

- **`backend/SICHERHEIT.md`**, Punkt 3 der Liste („Push ist da — und damit
  ist Google beteiligt") beschreibt weiterhin nur den Firebase-Weg. Der
  iOS-Weg steht inzwischen weiter unten in einem eigenen Abschnitt, aber
  Punkt 3 verweist nicht darauf — wer nur die Liste liest, hält Google für
  den einzigen Beteiligten.
- **Der offene Punkt in Punkt 3 selbst** (`ui/HomeScreen.kt` meldet die
  Kennung an, **bevor** die Erlaubnis geklärt ist) betrifft **Android**. Auf
  iOS ist es umgekehrt gebaut: gefragt wird erst, wenn sich jemand als
  Helfer:in einträgt, und angemeldet wird erst danach.

## Was der Mensch bei Apple und im Cluster noch tun muss

Der Menüpfad zum Erzeugen des `.p8`-Schlüssels und die Schritte im Cluster
stehen ausführlich in `deploy/overlays/production/deployment.yaml` beim
auskommentierten Block `APNS_KEY_FILE`. Kurz: Schlüssel als SealedSecret
ablegen, unter `/secrets/apns` einhängen, die `APNS_*`-Zeilen
einkommentieren.

Die Apple-Signierung steht dagegen: `DEVELOPMENT_TEAM` in `project.yml`
trägt die Team-ID, Zertifikat und Provisioning-Profil legt `xcodebuild`
über `-allowProvisioningUpdates` selbst an (`make archiv`,
`.github/workflows/ios-release.yml`). Für den Simulator bleibt es bei der
Ad-hoc-Signatur; die beiden Wege stören sich nicht.

Solange `APNS_KEY_FILE` im Cluster fehlt, wird für iOS schlicht nicht
gepusht. Das ist kein Fehlerzustand: Die App holt ihre Benachrichtigungen
über `GET /api/v1/me/notifications` ohnehin selbst ab, und Android bleibt
unberührt.

## Für die iOS-E2E-Läufe merken

Es gibt noch keine. Wenn sie kommen, gilt dieselbe Regel wie überall
(`CLAUDE.md`): **kein Test fasst einen entfernten Server an.** Alle vier
Adressen sind über Build-Einstellungen übersteuerbar, und
`.github/workflows/ios.yml` setzt sie bereits lokal;
`.github/scripts/pruefe_lokale_tests.py` wacht darüber und kennt `ios/`
inzwischen.

Für die Karte taugt `android/e2e/fixtures/map-style.json` unverändert als
Ersatzstil — sonst zieht ein Kartentest Kacheln von openfreemap.org.
