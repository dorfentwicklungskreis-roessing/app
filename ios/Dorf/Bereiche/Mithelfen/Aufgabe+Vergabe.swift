import Foundation

/// Wofür jemand mithelfen möchte: für alles am Ort oder nur für eine
/// Aufgabenart. Gießen ist eine kurze Sache, die jede Woche ansteht; Jäten
/// dauert und kommt selten — beides in einen Topf zu werfen, hielte manche
/// vom Mitmachen ab.
enum Helferwahl: Hashable, Identifiable {
    case alles
    case art(String)

    var id: String {
        switch self {
        case .alles: return "alles"
        case .art(let kind): return kind
        }
    }

    /// Der Wert, der ans Backend geht (nil = alle Aufgaben des Ortes).
    var taskKind: String? {
        switch self {
        case .alles: return nil
        case .art(let kind): return kind
        }
    }

    var titel: String {
        switch self {
        case .alles: return "Alles"
        case .art(let kind): return Helferwahl.name(kind)
        }
    }

    /// Der deutsche Name einer Aufgabenart. Unbekannte Arten kommen aus einem
    /// neueren Backend — sie bekommen ihren rohen Namen, statt zu verschwinden.
    static func name(_ kind: String) -> String {
        switch kind {
        case "giessen": return "Gießen"
        case "jaeten": return "Jäten"
        case "sonstiges": return "Sonstiges"
        default: return kind.isEmpty ? "Pflege" : kind
        }
    }
}

extension Aufgabe {
    /// Der Vergabestand in einem Satz: „Du hast zugesagt — bis …",
    /// „Anna hat zugesagt …". Liefert nil, wenn gerade kein Vorgang läuft.
    ///
    /// Alles darin kommt aus dem Vorgang des Backends; die App rechnet weder
    /// Fristen nach noch entscheidet sie, wer als Nächstes dran ist.
    func zusagetext(meinSub: String?) -> String? {
        guard let vorgang = assignment else { return nil }
        if vorgang.vonMir(meinSub) {
            guard let bis = vorgang.zusageFrist else { return "Du hast zugesagt." }
            return "Du hast zugesagt — bis \(Zeitpunkt.mitUhrzeit(bis))."
        }
        if vorgang.uebernommen {
            let wer = vorgang.claimedByName.isEmpty ? "Jemand" : vorgang.claimedByName
            guard let bis = vorgang.zusageFrist else {
                return "\(wer) hat zugesagt — es braucht keine zweite Zusage."
            }
            return "\(wer) hat zugesagt (bis \(Zeitpunkt.mitUhrzeit(bis))) — "
                + "es braucht keine zweite Zusage."
        }
        if vorgang.rundruf {
            return "Es wurden schon alle gefragt und noch hat niemand zugesagt. Wer kann?"
        }
        return "Die Angemeldeten werden gerade der Reihe nach gefragt."
    }

    /// Das Symbol zum Vergabestand — es steht nie allein, der Satz daneben
    /// sagt dasselbe.
    func zusagesymbol(meinSub: String?) -> String {
        guard let vorgang = assignment else { return "person.2" }
        if vorgang.vonMir(meinSub) { return "hand.thumbsup.fill" }
        if vorgang.uebernommen { return "person.fill.checkmark" }
        return "megaphone"
    }

    /// „4 helfen hier mit" — nil, wenn niemand angemeldet ist.
    var helfertext: String? {
        switch signupCount {
        case ..<1: return nil
        case 1: return signedUp ? "Du hilfst hier mit." : "Eine Person hilft hier mit."
        default: return signedUp
            ? "\(signupCount) helfen hier mit, du bist dabei."
            : "\(signupCount) helfen hier mit."
        }
    }
}

extension Ort {
    /// Die Aufgabenarten, die es hier überhaupt gibt — in der Reihenfolge der
    /// Aufgaben, damit die Auswahl nicht bei jedem Abruf springt.
    var helferArten: [String] {
        var gesehen: Set<String> = []
        return aktiveAufgaben.map(\.kind).filter { gesehen.insert($0).inserted }
    }

    /// Ob ich für diesen Ort (oder eine seiner Aufgaben) angemeldet bin.
    var helfeIchMit: Bool { aktiveAufgaben.contains { $0.signedUp } }

    /// Wofür ich angemeldet bin: für alles oder für genau eine Art.
    var meineHelferwahl: Helferwahl {
        let angemeldet = aktiveAufgaben.filter(\.signedUp)
        guard let erste = angemeldet.first else { return .alles }
        if angemeldet.count == aktiveAufgaben.count { return .alles }
        return .art(erste.kind)
    }

    /// Wie viele hier mithelfen. Die Zahl hängt an der Aufgabe (je Art
    /// verschieden) — für den Ort zählt die größte.
    var helferzahl: Int { aktiveAufgaben.map(\.signupCount).max() ?? 0 }
}
