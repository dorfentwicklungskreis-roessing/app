# Push auf iOS — was noch verdrahtet werden muss

Diese Datei gehört zu `ios/OFFEN.md` und wird dort eingearbeitet. Sie steht
getrennt, weil an der iOS-App gerade mehrere Leute gleichzeitig arbeiten und
`OFFEN.md`, `DorfApp.swift`, `Umgebung.swift`, `Navigation/` und
`Bereiche/` anderen gehören.

**Fertig ist**: `ios/Dorf/Push/` (Erlaubnis, Kanäle, Gerätekennung, die
beiden Endpunkte), `ios/Dorf/Dorf.entitlements`, die Build-Einstellungen in
`ios/project.yml` und der gesamte Weg im Backend
(`backend/internal/push/apns.go`). Was fehlt, sind **drei Zeilen in Dateien,
die mir nicht gehören.**

## 1. Den Systemdelegaten anhängen (`DorfApp.swift`)

Die Gerätekennung von Apple kommt ausschließlich über
`UIApplicationDelegate`; SwiftUI hat dafür keinen eigenen Weg. `PushDelegat`
bringt alles mit, angehängt wird er mit einer Zeile:

```swift
@main
struct DorfApp: App {
    @UIApplicationDelegateAdaptor(PushDelegat.self) private var pushDelegat   // neu
    @State private var umgebung = AppUmgebung()
    …
}
```

## 2. Den Backend-Zugang durchreichen (`Umgebung.swift` oder `DorfApp.swift`)

`Benachrichtigungen.gemeinsam` kennt das Backend noch nicht. Einmal beim
Start:

```swift
Benachrichtigungen.gemeinsam.verdrahten(
    api: umgebung.api,
    zugangstoken: { [anmeldung = umgebung.anmeldung] in await anmeldung.frischesToken() }
)
```

Ohne diese Zeile läuft die App vollständig weiter — es wird nur nie eine
Kennung angemeldet, und es kommt kein Push an.

Dazu gehört der Abgleich bei jeder Rückkehr in den Vordergrund
(`.onChange(of: scenePhase)`, `Task { await Benachrichtigungen.gemeinsam.abgleichen() }`).
Der räumt die Kennung weg, wenn jemand die Mitteilungen in den
iOS-Einstellungen wieder abdreht.

## 3. Die Erlaubnis an der richtigen Stelle erfragen — **Vergabe-Agent**

Der Systemdialog erscheint **genau einmal** im Leben einer Installation. Wer
ihn beim Start sieht, ohne zu wissen wofür, sagt Nein — und danach hilft nur
noch der Weg über die iOS-Einstellungen. Deshalb, genau wie auf Android:

> **Gefragt wird erst, wenn sich jemand als Helfer:in für eine Aufgabe
> eingetragen hat.** Dort ist die Frage selbsterklärend: „Wir sagen dir
> Bescheid, wenn du dran bist."

Der Aufruf ist:

```swift
await Benachrichtigungen.gemeinsam.erlaubnisErfragen()
```

Er gehört unmittelbar hinter den erfolgreichen `signup`-Aufruf — nicht
davor, nicht in `onAppear` eines Bildschirms.

## 4. Beim Abmelden aufräumen (`StartView` / wo „Abmelden" sitzt)

**Vor** `anmeldung.abmelden()`:

```swift
await Benachrichtigungen.gemeinsam.abmelden()
```

Danach gibt es kein gültiges Token mehr, und die Kennung bliebe für immer im
Dorfserver stehen — der schickte dann Anfragen an ein Gerät, an dem niemand
mehr angemeldet ist.

## 5. Der Fingertipp führt zur Aufgabe

`Benachrichtigungen.gemeinsam.beiTipp` ist ein Rückruf mit einem `PushZiel`
(Ort, Aufgabe, Vorgang, Art). Wer die Navigation baut, hängt sich dort ein:

```swift
Benachrichtigungen.gemeinsam.beiTipp = { ziel in
    // ziel.ortId / ziel.aufgabeId → Ziel.ortDetail(…)
}
```

Solange niemand den Rückruf setzt, wird die Meldung angezeigt und der Tipp
öffnet schlicht die App — kein Fehler, nur ein verschenkter Weg.

## Was bewusst offen geblieben ist

- **Keine Knöpfe an der Meldung.** Die `UNNotificationCategory`n sind ohne
  `UNNotificationAction` angelegt. „Übernehmen" direkt aus der Meldung
  heraus wäre möglich, braucht aber die Vergabe-Endpunkte (`claim`) in
  `DorfApi` — die gibt es noch nicht.
- **Keine `interruption-level: time-sensitive`.** Das würde Anfragen auch
  durch „Nicht stören" hindurchlassen, verlangt aber die zusätzliche
  Berechtigung *Time Sensitive Notifications* im Entwicklerportal. Solange
  die nicht eingerichtet ist, bleibt es bei `apns-priority: 10` — das
  reicht für sofortige Zustellung.
- **Kein Zähler am App-Symbol** (Badge). Die Erlaubnis dafür wird
  mitgefragt, benutzt wird sie nicht: Eine ehrliche Zahl bräuchte die
  Benachrichtigungsliste des Backends, die die App noch nicht abruft.
- **Im Simulator kommt keine echte Gerätekennung an.** Das ist normal und
  kein Fehler. Prüfbar ist im Simulator nur die Anzeige
  (`xcrun simctl push <geräte-id> de.roessing.app <datei.apns>`) — der Weg
  über Apple braucht ein echtes iPhone.

## Was der Mensch bei Apple und im Cluster noch tun muss

Der Menüpfad zum Erzeugen des `.p8`-Schlüssels und die Schritte im Cluster
stehen ausführlich in `deploy/overlays/production/deployment.yaml` beim
auskommentierten Block `APNS_KEY_FILE`. Kurz: Schlüssel als SealedSecret
ablegen, unter `/secrets/apns` einhängen, die `APNS_*`-Zeilen
einkommentieren.

Noch nachzutragen (gehört anderen Dateien): `README.md`, Abschnitt
„Push-Benachrichtigungen", nennt bislang nur Firebase — dort gehört der
iOS-Weg dazu; und `ios/README.md` führt Push unter „Was noch fehlt".
