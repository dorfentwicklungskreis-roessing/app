import Foundation

/// Die beiden Kanäle der Meldungen — auf iOS heißen sie „Category".
///
/// Getrennt, weil sie Verschiedenes wollen: Eine **Anfrage** möchte eine
/// Antwort und darf deshalb auffallen; ein **Hinweis** berichtet nur („schon
/// erledigt", „Zusage abgelaufen"). Wer die Hinweise leiser stellt, soll
/// trotzdem gefragt werden können.
///
/// Die Bezeichner sind dieselben wie die Kanal-IDs auf Android
/// (`PushKanal.ANFRAGEN`/`HINWEISE`) und wie das Feld `category`, das das
/// Backend in die APNs-Nutzlast schreibt
/// (`backend/internal/push/apns.go`, `kanal()`). Wer einen ändert, ändert ihn
/// an drei Stellen.
nonisolated enum PushKanal: String, CaseIterable, Sendable {
    case anfragen
    case hinweise

    /// Was in den iOS-Einstellungen unter „Mitteilungen" steht.
    var bezeichnung: String {
        switch self {
        case .anfragen: "Anfragen"
        case .hinweise: "Hinweise"
        }
    }

    var beschreibung: String {
        switch self {
        case .anfragen: "Wenn du an der Reihe bist, eine Aufgabe zu übernehmen."
        case .hinweise: "Kurze Rückmeldungen zu Aufgaben, für die du zugesagt hast."
        }
    }
}

/// Wohin ein Fingertipp auf eine Meldung führt.
///
/// Der Inhalt kommt aus dem Backend und ist für Android und iOS **derselbe**
/// (`backend/internal/push/fcm.go`, `daten()`): FCM trägt ihn im `data`-Feld,
/// APNs neben dem `aps`-Objekt. Alle Werte sind Zeichenketten — FCM lässt
/// nichts anderes zu, und damit bleibt der Vertrag für beide Apps gleich.
/// Gegenstück auf Android: `de.roessing.app.push.PushZiel`.
///
/// Reine Rechnerei ohne System — genau deshalb hier und nicht im Delegaten:
/// Im Simulator kommt keine echte Meldung an, prüfbar muss das trotzdem sein.
nonisolated struct PushZiel: Equatable, Sendable {
    let ortId: Int64
    let aufgabeId: Int64
    let vorgangId: Int64
    let meldungId: Int64
    /// Die Art der Benachrichtigung, wie das Backend sie nennt: „anfrage",
    /// „rundruf", „zusage_abgelaufen" …
    let art: String
    let aufgabenart: String
    let ortsname: String
    let aufgabe: String
    let titel: String
    let text: String

    /// Anfragen wollen eine Antwort; alles andere ist ein Hinweis.
    var istAnfrage: Bool { art == "anfrage" || art == "rundruf" }

    var kanal: PushKanal { istAnfrage ? .anfragen : .hinweise }

    /// Liest das Ziel aus der Nutzlast einer Meldung
    /// (`UNNotificationContent.userInfo`).
    ///
    /// Ohne brauchbare Orts-Kennung gibt es nichts anzuspringen — dann bleibt
    /// es bei der bloßen Anzeige, und `nil` sagt der App genau das.
    static func ausDaten(_ daten: [AnyHashable: Any]) -> PushZiel? {
        guard let ortId = zahl(daten["placeId"]), ortId > 0 else { return nil }
        return PushZiel(
            ortId: ortId,
            aufgabeId: zahl(daten["taskId"]) ?? 0,
            vorgangId: zahl(daten["assignmentId"]) ?? 0,
            meldungId: zahl(daten["notificationId"]) ?? 0,
            art: text(daten["kind"]),
            aufgabenart: text(daten["taskKind"]),
            ortsname: text(daten["placeName"]),
            aufgabe: text(daten["taskName"]),
            titel: text(daten["title"]),
            text: text(daten["body"])
        )
    }

    /// Zahlen kommen als Zeichenkette (so schreibt es das Backend). Eine
    /// echte JSON-Zahl wird trotzdem angenommen: Eine Meldung, die an einer
    /// Kleinigkeit im Datentyp scheitert, wäre ein ärgerlicher Verlust.
    private static func zahl(_ wert: Any?) -> Int64? {
        switch wert {
        case let s as String: Int64(s)
        case let z as Int64: z
        case let z as Int: Int64(z)
        case let z as NSNumber: z.int64Value
        default: nil
        }
    }

    private static func text(_ wert: Any?) -> String {
        switch wert {
        case let s as String: s
        case let z as NSNumber: z.stringValue
        default: ""
        }
    }
}
