import Foundation

/// Die Texte der Rangliste, die aus Rohdaten des Backends entstehen: der
/// Zeitraum im Klartext, der eigene Rang, die Aufschlüsselung nach Art und
/// das, was VoiceOver vorliest.
///
/// Bewusst ohne Oberfläche — so lässt sich prüfen, was dort steht, ohne eine
/// View zu bauen. Gerechnet wird hier nichts: Wer auf welchem Platz steht und
/// was überhaupt gewertet wird, entscheidet das Backend.
enum Ranglistentexte {

    // MARK: - Zeitraum

    /// Grenzen jenseits dieser Jahre meint das Backend als „offen": Für
    /// `gesamt` schickt es das Jahr 1 und das Jahr 9999.
    private static let offeneVergangenheitVorJahr = 1900
    private static let offeneZukunftAbJahr = 2900

    /// „1. März bis 31. Oktober 2026" aus `from`/`to` der Antwort.
    ///
    /// Das Backend liefert ein halboffenes Intervall `[from, to)` — der
    /// 1. November gehört also nicht mehr dazu. Angezeigt wird deshalb der
    /// letzte Tag, an dem noch etwas zählt, nicht die Obergrenze selbst.
    static func zeitraum(von: String, bis: String) -> String? {
        guard !von.isEmpty || !bis.isEmpty else { return nil }

        // Was sich nicht lesen lässt, gilt als offene Grenze — genau wie das
        // Jahr 1 und das Jahr 9999, mit denen „Gesamt" ankommt.
        let anfang = RFC3339.datum(von)
            .flatMap { jahr($0) < offeneVergangenheitVorJahr ? nil : $0 }
        let ende = RFC3339.datum(bis)
            .map { $0.addingTimeInterval(-1) }
            .flatMap { jahr($0) >= offeneZukunftAbJahr ? nil : $0 }

        switch (anfang, ende) {
        case (nil, nil):
            return "Alles, was je gemeldet wurde"
        case (nil, .some(let e)):
            return "Bis " + tagMonatJahr(e)
        case (.some(let a), nil):
            return "Seit " + tagMonatJahr(a)
        case (.some(let a), .some(let e)):
            if kalender.isDate(a, inSameDayAs: e) { return tagMonatJahr(e) }
            if gleichesJahr(a, e) {
                if gleicherMonat(a, e) { return tag(a) + " bis " + tagMonatJahr(e) }
                return tagMonat(a) + " bis " + tagMonatJahr(e)
            }
            return tagMonatJahr(a) + " bis " + tagMonatJahr(e)
        }
    }

    // MARK: - Eigener Rang

    /// Der eigene Platz — oder, bei `rank == 0`, der freundliche Hinweis
    /// darauf, dass im Zeitraum noch nichts angekommen ist. „Platz 0" wäre
    /// keine Auskunft, sondern eine Kränkung.
    static func eigenerRang(_ zeile: Ranglistenzeile?) -> String {
        guard let zeile, zeile.rank > 0 else {
            return "Im gewählten Zeitraum hast du noch nichts gemeldet — jede Kanne zählt."
        }
        return "Du stehst auf Platz \(zeile.rank)."
    }

    /// Die eigene Zeile: bevorzugt `me` aus der Antwort, sonst die passende
    /// Zeile der Liste. Fehlt `me`, ist das kein Fehler — die Seite steht
    /// trotzdem.
    static func eigeneZeile(_ stand: Rangliste?, meinSub: String?) -> Ranglistenzeile? {
        guard let stand else { return nil }
        if let me = stand.me { return me }
        return stand.entries.first { $0.istMeine(meinSub) }
    }

    // MARK: - Arten

    /// „12× Gießen · 3× Jäten". Arten, die diese App-Version nicht kennt,
    /// laufen unter „Pflege" und werden dort zusammengezählt.
    static func arten(_ byKind: [String: Int]) -> String {
        var summen: [String: Int] = [:]
        for (art, anzahl) in byKind where anzahl > 0 {
            summen[artName(art), default: 0] += anzahl
        }
        // Feste Reihenfolge: sonst tanzen die Angaben von Zeile zu Zeile.
        return ["Gießen", "Jäten", "Pflege"]
            .compactMap { name in summen[name].map { "\($0)× \(name)" } }
            .joined(separator: " · ")
    }

    static func artName(_ art: String) -> String {
        switch art {
        case "giessen": return "Gießen"
        case "jaeten": return "Jäten"
        default: return "Pflege"
        }
    }

    // MARK: - Kleinkram der Anzeige

    /// Medaille statt Zahl für die ersten drei — mehr Podest braucht es nicht.
    static func medaille(_ rang: Int) -> String? {
        switch rang {
        case 1: return "🥇"
        case 2: return "🥈"
        case 3: return "🥉"
        default: return nil
        }
    }

    /// „vor 3 Tagen" — oder nichts, wenn nie etwas gemeldet wurde.
    static func letzteErledigung(_ roh: String?, jetzt: Date = Date()) -> String? {
        guard let roh, let datum = RFC3339.datum(roh) else { return nil }
        return Zeitpunkt.relativ(datum, jetzt: jetzt)
    }

    /// Für VoiceOver: Die Rangzahl steht ausgeschrieben da, weil „3." allein
    /// je nach Stimme als „drei Punkt" ankommt.
    static func vorlesen(_ zeile: Ranglistenzeile, eigen: Bool, jetzt: Date = Date()) -> String {
        var teile: [String] = []
        teile.append(zeile.rank > 0 ? "Platz \(zeile.rank)" : "Noch kein Platz")
        teile.append(zeile.userName.isEmpty ? "ohne Namen" : zeile.userName)
        if eigen { teile.append("das bist du") }
        teile.append("\(zeile.completions) Erledigungen")
        if zeile.liters > 0 { teile.append("\(Zahl.liter(zeile.liters)) Liter") }
        let nachArt = arten(zeile.byKind)
        if !nachArt.isEmpty { teile.append(nachArt) }
        if let zuletzt = letzteErledigung(zeile.lastCompletion, jetzt: jetzt) {
            teile.append("zuletzt \(zuletzt)")
        }
        return teile.joined(separator: ", ") + "."
    }

    // MARK: - Innereien

    private static var kalender: Calendar {
        var k = Calendar(identifier: .gregorian)
        k.locale = Locale(identifier: "de_DE")
        k.timeZone = Zeitpunkt.dorfZone
        return k
    }

    private static func formatierer(_ muster: String) -> DateFormatter {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = Zeitpunkt.dorfZone
        f.dateFormat = muster
        return f
    }

    private static func jahr(_ datum: Date) -> Int { kalender.component(.year, from: datum) }
    private static func tag(_ datum: Date) -> String { formatierer("d.").string(from: datum) }
    private static func tagMonat(_ datum: Date) -> String { formatierer("d. MMMM").string(from: datum) }
    private static func tagMonatJahr(_ datum: Date) -> String {
        formatierer("d. MMMM yyyy").string(from: datum)
    }

    private static func gleichesJahr(_ a: Date, _ b: Date) -> Bool {
        kalender.isDate(a, equalTo: b, toGranularity: .year)
    }

    private static func gleicherMonat(_ a: Date, _ b: Date) -> Bool {
        gleichesJahr(a, b) && kalender.isDate(a, equalTo: b, toGranularity: .month)
    }
}

extension Ranglistenzeile {
    /// Ob diese Zeile die eigene ist. Ohne bekanntes Sub ist sie es nie —
    /// zwei leere Zeichenketten sind keine Übereinstimmung.
    func istMeine(_ meinSub: String?) -> Bool {
        guard let meinSub, !meinSub.isEmpty, !userSub.isEmpty else { return false }
        return userSub == meinSub
    }

    /// „12× Gießen · 3× Jäten"
    var artenText: String { Ranglistentexte.arten(byKind) }
}
