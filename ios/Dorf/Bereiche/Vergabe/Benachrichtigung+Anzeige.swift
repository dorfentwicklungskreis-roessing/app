import Foundation

/// Wie eine Benachrichtigung in der Liste aussieht.
///
/// Titel und Text formuliert das Backend — sie werden im Wortlaut gezeigt
/// und hier nur ergänzt, wenn sie fehlen (alte Zustellung, gelöschte
/// Aufgabe). Über die Frist wird nichts gerechnet: Sie kommt fertig mit und
/// wird bloß hingeschrieben.
extension Benachrichtigung {
    /// Anfragen zuoberst — sie warten auf eine Antwort, Hinweise nicht.
    /// Danach das Neueste zuerst; bei gleicher Zeit entscheidet die Kennung,
    /// damit die Liste zwischen zwei Abrufen nicht springt.
    static func geordnet(_ liste: [Benachrichtigung]) -> [Benachrichtigung] {
        liste.sorted { links, rechts in
            if links.istAnfrage != rechts.istAnfrage { return links.istAnfrage }
            let a = links.erstellt ?? .distantPast
            let b = rechts.erstellt ?? .distantPast
            if a != b { return a > b }
            return links.id > rechts.id
        }
    }

    /// Das Symbol sagt dasselbe wie der Text — es steht nie allein.
    var symbol: String {
        switch kind {
        case Self.anfrage: return "hand.raised.fill"
        case Self.rundruf: return "megaphone.fill"
        case "zusage_abgelaufen", "zusage_aufgehoben": return "clock.badge.exclamationmark"
        default: return "checkmark.circle"
        }
    }

    /// Die Überschrift. Kommt keine mit, wird aus Aufgabe und Ort eine
    /// gebaut — eine Karte ohne Überschrift wäre unlesbar.
    var anzeigetitel: String {
        if !title.isEmpty { return title }
        if !ort.isEmpty { return ort }
        return istAnfrage ? "Du bist dran" : "Hinweis"
    }

    /// „Gießen an „Am Anger"" — leer, wenn beides fehlt.
    var ort: String {
        switch (taskName.isEmpty, placeName.isEmpty) {
        case (false, false): return "\(taskName) an „\(placeName)“"
        case (false, true): return taskName
        case (true, false): return placeName
        case (true, true): return ""
        }
    }

    /// Der Satz darunter. Auch er kommt vom Backend; fehlt er, steht dort
    /// wenigstens, worum es geht.
    var anzeigetext: String {
        if !text.isEmpty { return text }
        if !ort.isEmpty {
            return istAnfrage ? "\(ort) ist offen." : "Es geht um \(ort)."
        }
        return istAnfrage ? "Diese Aufgabe wartet auf jemanden." : ""
    }

    /// Was die Frist bedeutet, in einem Satz.
    ///
    /// Bei einer Anfrage ist sie der **Vortritt**: Danach wird der Nächste
    /// gefragt — zusagen darf man trotzdem weiter. Das entscheidet das
    /// Backend, die App nimmt der Person nur den Knopf nicht weg.
    func fristtext(jetzt: Date = Date()) -> String? {
        guard let frist else { return nil }
        if istAnfrage {
            if abgelaufen(jetzt: jetzt) {
                return "Dein Vortritt ist abgelaufen — jetzt wird der Nächste gefragt. "
                    + "Zusagen kannst du weiterhin."
            }
            return "Du hast Vortritt bis \(Zeitpunkt.mitUhrzeit(frist))."
        }
        return "Frist: \(Zeitpunkt.mitUhrzeit(frist))."
    }
}
