import Foundation

/// Die Sätze der Startseite, die aus Daten entstehen — als reine Funktionen,
/// damit sie prüfbar sind, ohne eine Oberfläche zu bauen.
///
/// Warum das hier steht und nicht im Bereich „Mithelfen": Beides sind
/// Aussagen der **Startseite** über die Orte („was steht an?", „ist es
/// heiß?"). Der Bereich selbst zeigt jeden Ort einzeln und braucht keine
/// Zusammenfassung.
enum Startseitentexte {
    /// Wie viele Orte auf gelb oder rot stehen — also darauf warten, dass
    /// jemand hingeht. Grüne zählen nicht: „Alles gut" ist kein Auftrag.
    static func wartendeOrte(_ orte: [Ort]) -> Int {
        orte.filter { $0.active && $0.ampel != .green }.count
    }

    /// Der Hinweis auf der Kachel „Mithelfen".
    ///
    /// Null Orte ergeben **keinen** Hinweis statt „0 Orte warten auf dich":
    /// Eine Null ist keine Nachricht, sondern Rauschen. Und Singular und
    /// Plural stimmen — „1 Orte" liest sich wie eine Maschine.
    static func mithelfenHinweis(orte: [Ort]) -> String? {
        hinweisFuer(wartendeOrte(orte))
    }

    static func hinweisFuer(_ anzahl: Int) -> String? {
        switch anzahl {
        case ..<1: return nil
        case 1: return "Ein Ort wartet auf dich"
        default: return "\(anzahl) Orte warten auf dich"
        }
    }

    /// Der Hitzehinweis. Der Gießfaktor des Backends skaliert die
    /// Gieß-Schwellen: 1 ist normal, **kleiner als 1 heißt heiß** (alles wird
    /// schneller fällig). Größer als 1 wäre eine nasse Woche — dafür braucht
    /// niemand einen Hinweis auf der Startseite.
    ///
    /// Die Zahl selbst steht bewusst nicht da: „0,5" sagt niemandem etwas,
    /// „bitte großzügig gießen" schon.
    static func hitzehinweis(giessfaktor: Double) -> String? {
        guard giessfaktor.isFinite, giessfaktor < 1 else { return nil }
        return "Heiß — bitte großzügig gießen."
    }

    /// Der Hinweis auf der Kachel „Vereine und Gruppen".
    ///
    /// Gezählt werden die Anfragen, die auf **meine** Entscheidung warten —
    /// die Zahl schickt der Server nur denen mit, die den Träger verwalten.
    /// Für alle anderen bleibt die Kachel still: Wer nichts zu entscheiden
    /// hat, braucht keine Null.
    static func traegerHinweis(offeneAnfragen: Int) -> String? {
        switch offeneAnfragen {
        case ..<1: return nil
        case 1: return "Eine Anfrage wartet auf dich"
        default: return "\(offeneAnfragen) Anfragen warten auf dich"
        }
    }
}
