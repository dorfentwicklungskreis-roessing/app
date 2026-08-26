import Foundation

/// Was der Melde-Knopf einer Aufgabe gerade anbietet.
///
/// Eigener Typ, weil die Entscheidung an drei Stellen gleich ausfallen muss
/// (Knopf, Vorlesetext, Test) und weil sie sich so prüfen lässt, ohne eine
/// Oberfläche zu bauen. Die Regeln selbst kommen vom Backend: `lockedUntil`
/// ist der Spielschutz, `oneOff` + `lastCompletion` die erledigte
/// Einmalaufgabe. Die App denkt sich hier nichts aus.
enum Meldeknopf: Equatable {
    /// Melden ist möglich.
    case bereit(titel: String)
    /// Spielschutz: erst wieder ab diesem Zeitpunkt.
    case gesperrt(bis: Date)
    /// Einmalig und erledigt — der Knopf gehört weg, nicht bloß gesperrt.
    case keiner
}

extension Aufgabe {
    /// Der Zustand des Melde-Knopfes zu einem Zeitpunkt.
    ///
    /// `jetzt` ist ein Parameter, damit der Test nicht auf die Uhr warten muss.
    func meldeknopf(jetzt: Date = Date()) -> Meldeknopf {
        if erledigtUndVorbei { return .keiner }
        if let bis = gesperrtBis, jetzt < bis { return .gesperrt(bis: bis) }
        return .bereit(titel: meldetext)
    }

    /// Die Aufschrift des Knopfes — sie sagt, was man getan hat, nicht was
    /// die App gleich tut.
    var meldetext: String {
        switch kind {
        case "giessen": return "Ich habe gegossen 💧"
        case "jaeten": return "Ich habe gejätet 🌿"
        default: return "Als erledigt melden"
        }
    }

    /// Die Frage vor dem Melden. Ein Fehltipp darf nichts eintragen.
    var nachfrageTitel: String {
        switch kind {
        case "giessen": return "Hast du wirklich gegossen?"
        case "jaeten": return "Hast du wirklich gejätet?"
        default: return "Wirklich als erledigt melden?"
        }
    }

    /// Danke-Text nach einer angenommenen Meldung.
    var dankeText: String {
        switch kind {
        case "giessen": return "Danke fürs Gießen! 💚"
        case "jaeten": return "Danke fürs Jäten! 💚"
        default: return "Danke fürs Mithelfen! 💚"
        }
    }

    /// Regelmäßig steht hier der Plan („10 Liter · alle 7 Tage"), einmalig der
    /// Termin. Beides kommt aus den Feldern des Backends, gerechnet wird nichts.
    var planText: String {
        if oneOff {
            guard let termin = faelligAm else { return "Einmalige Aufgabe" }
            return "Einmalig · Termin \(Zeitpunkt.kurz(termin))"
        }
        var teile: [String] = []
        if let menge = liters { teile.append("\(Zahl.liter(menge)) Liter") }
        teile.append("alle \(Zahl.liter(intervalDays)) Tage")
        if let faellig = frist { teile.append("fällig \(Zeitpunkt.kurz(faellig))") }
        return teile.joined(separator: " · ")
    }

    /// Kurzfassung für die Liste: „Gießen · 10 Liter".
    var kurztext: String {
        var teile = [anzeigename]
        if let menge = liters { teile.append("\(Zahl.liter(menge)) Liter") }
        return teile.joined(separator: " · ")
    }

    /// „zuletzt: Anna, vor 3 Tagen" — relativ, weil das im Dorf mehr sagt als
    /// ein Datum.
    var letzteMeldungText: String {
        guard let letzte = lastCompletion else { return "noch nie erledigt" }
        let name = letzte.userName.isEmpty ? "jemand" : letzte.userName
        guard let wann = letzte.zeitpunkt else { return "zuletzt: \(name)" }
        return "zuletzt: \(name), \(Zeitpunkt.relativ(wann))"
    }

    /// Die letzte Meldung, wenn sie von mir ist — nur die darf ich
    /// zurücknehmen. Wer sie zurücknehmen darf, entscheidet am Ende das
    /// Backend; hier wird der Knopf nur nicht angeboten, wo er sicher nichts
    /// bringt.
    func eigeneLetzteMeldung(_ meinSub: String?) -> Erledigung? {
        guard let meinSub, !meinSub.isEmpty, let letzte = lastCompletion else { return nil }
        return letzte.userSub == meinSub ? letzte : nil
    }

    /// Wann die Aufgabe fällig ist — bei einmaligen Aufgaben der Termin.
    var frist: Date? { RFC3339.datum(dueAt) ?? faelligAm }

    /// Offen heißt: aktiv und nicht als Einmalaufgabe schon erledigt.
    var offen: Bool { active && !erledigtUndVorbei }
}

extension Ort {
    /// Die Aufgaben, die dieser Ort überhaupt zeigt. Abgeschaltete sind
    /// niemandes Aufgabe.
    var aktiveAufgaben: [Aufgabe] { tasks.filter(\.active) }

    /// Die offene Aufgabe mit der kürzesten Frist — die, die als Nächstes
    /// drängt. Bei gleicher Frist entscheidet die Ampel, dann der Name, damit
    /// die Liste bei jedem Abruf gleich aussieht.
    var kuerzesteOffeneAufgabe: Aufgabe? {
        aktiveAufgaben.filter(\.offen).min { links, rechts in
            let a = links.frist ?? .distantFuture
            let b = rechts.frist ?? .distantFuture
            if a != b { return a < b }
            if links.ampel.dringlichkeit != rechts.ampel.dringlichkeit {
                return links.ampel.dringlichkeit < rechts.ampel.dringlichkeit
            }
            return links.anzeigename.localizedCaseInsensitiveCompare(rechts.anzeigename)
                == .orderedAscending
        }
    }
}
