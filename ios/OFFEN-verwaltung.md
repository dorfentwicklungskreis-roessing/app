# Bereich „Verwaltung" — Einhängepunkt und offene Punkte

Der Bereich ist gebaut (`Dorf/Bereiche/Verwaltung/`), aber **noch nicht in
die Navigation verdrahtet**: `Navigation/` gehört gerade einem anderen
Schritt. Hier steht, was dort einzutragen ist — und was der Bereich bewusst
noch nicht kann.

## Einhängen (drei Zeilen in `Navigation/`)

1. `Dorf/Navigation/Ziel.swift`, in `enum Ziel`:

   ```swift
   case verwaltung
   ```

2. Ebenda, in `dorfZiele()`:

   ```swift
   case .verwaltung: VerwaltungView()
   ```

3. `Dorf/Navigation/StartView.swift` — die Kachel erscheint **nur für
   Verwaltende**. Das Backend weist ohnehin jede Änderung ohne die Rolle
   `admin` mit 403 ab; die Kachel auszublenden erspart nur den Weg dorthin:

   ```swift
   if umgebung.binAdmin {
       Section("Verwaltung") {
           Bereichskachel(
               ziel: .verwaltung, symbol: "wrench.and.screwdriver.fill",
               titel: "Verwaltung",
               untertitel: "Orte und Aufgaben pflegen."
           )
       }
   }
   ```

`VerwaltungView` braucht sonst nichts: Sie zieht sich `AppUmgebung` aus der
Umgebung, baut ihr `OrteModell` (dasselbe wie „Mithelfen") selbst und prüft
`umgebung.binAdmin` zusätzlich noch einmal — wer über einen Umweg hierher
kommt, sieht einen Satz statt einer Werkzeugkiste.

## Warum der Zugang nicht direkt an `DorfApi` hängt

`DorfApi` hält `basis`, `sitzung`, `tokenGeber` und seine Helfer (`hole`,
`schicke`, `rohAusfuehren`, `fehler`) `private`. Ein Anhang in einer anderen
Datei kommt an sie nicht heran, und `DorfApi.swift` war beim Bau dieses
Bereichs für andere Schritte gesperrt. `DorfApi+Verwaltung.swift` baut den
Schreibzugang deshalb aus denselben Teilen noch einmal
(`DorfApi.Verwaltung`) — gleiche Fristen, gleiche `DorfFehler`. Verdrahtet
ist er wie der Vergabe-Zugang: `umgebung.verwaltung`, eine berechnete
Eigenschaft in derselben Datei, damit `Umgebung.swift` unangetastet bleibt.

Der Bereich „Anfragen" steht vor demselben Problem und hat es gleich gelöst
(`VergabeApi` in `DorfApi+Vergabe.swift`). Damit gibt es inzwischen **drei**
Transportschichten mit demselben Inhalt.

**Aufzuräumen, sobald `DorfApi.swift` wieder frei ist:** dort `hole`,
`schicke`, `schickeOhneAntwort` und `fehler(status:daten:pfad:)` auf
`internal` heben; dann schrumpfen `DorfApi.Verwaltung` und `VergabeApi` auf
die nackten Aufrufe zusammen, und es gibt wieder genau eine Transportschicht.

## Bewusst noch nicht gebaut

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

## Kleinere Lücken

- **Kartentipp und VoiceOver.** Auf eine bestimmte Stelle der Karte zu
  tippen, geht mit VoiceOver nicht. Der barrierefreie Weg zur Koordinate ist
  „Meinen Standort übernehmen" — das Formular sagt das auch, und die Karte
  trägt im Auswahlmodus einen entsprechenden Hinweis. Wer einen Ort anlegt,
  an dem er nicht steht, braucht bis auf Weiteres die Web-Verwaltung.
- **Kein Zurücksetzen der Kamera** im Auswahlmodus: Die Auswahlkarte startet
  auf dem Ausschnitt aller Orte, nicht auf dem eigenen Standort. Wer „Meinen
  Standort übernehmen" drückt, sieht den gesetzten Punkt erst nach dem
  Hinschieben. `KarteView` kann nach wie vor nicht auf einen Punkt gezeigt
  werden (steht auch in `OFFEN.md`).
- **Der Hitzefaktor steht in Zehntelschritten.** Ein vom Backend geliefertes
  0,75 bleibt erhalten, lässt sich mit dem Stepper aber nur in Zehnteln
  verändern.
- **Keine Suche und keine Sortierung** in der Ortsliste der Verwaltung; sie
  steht nach Dringlichkeit wie in „Mithelfen". Ab einigen Dutzend Orten wird
  das unhandlich.
