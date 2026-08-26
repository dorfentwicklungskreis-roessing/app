import Combine
import Foundation

/// Der Stand der Rangliste für einen Zeitraum.
///
/// Ausgewertet wird komplett im Backend — Zeiträume, Reihenfolge, Liter,
/// Auszeichnungen und die Frage, was überhaupt als Erledigung zählt. Hier wird
/// nur geladen, gehalten und beim Umschalten neu geholt.
///
/// Fällt das Netz aus, bleibt der zuletzt geladene Stand stehen und bekommt
/// einen Hinweis darüber: Eine leere Seite wäre die schlechtere Auskunft.
final class RanglisteModell: ObservableObject {
    @Published private(set) var stand: Rangliste?
    @Published private(set) var laedt = false
    /// Klartext des letzten Fehlversuchs — vom Backend, nicht von der App.
    @Published private(set) var hinweis: String?
    /// Der Zeitraum, zu dem der angezeigte Stand gehört.
    @Published private(set) var geladenerZeitraum: Zeitraum?

    func laden(api: DorfApi, zeitraum: Zeitraum) async {
        laedt = true
        do {
            stand = try await api.rangliste(zeitraum: zeitraum)
            geladenerZeitraum = zeitraum
            hinweis = nil
            laedt = false
        } catch {
            // Beim Umschalten bricht die vorige Abfrage ab. Das ist kein
            // Fehler, und der neue Lauf hat „lädt" bereits gesetzt — hier
            // also nichts anfassen.
            if Task.isCancelled { return }
            hinweis = (error as? DorfFehler)?.klartext ?? "Unerwarteter Fehler."
            laedt = false
        }
    }

    /// „1. März bis 31. Oktober 2026" — aus der Antwort, nicht aus dem
    /// Umschalter: So steht dort immer der Zeitraum der gezeigten Zahlen.
    var zeitraumtext: String? {
        guard let stand else { return nil }
        return Ranglistentexte.zeitraum(von: stand.from, bis: stand.to)
    }

    var zeilen: [Ranglistenzeile] { stand?.entries ?? [] }
    var summen: Gesamtsummen { stand?.totals ?? Gesamtsummen() }

    func eigeneZeile(meinSub: String?) -> Ranglistenzeile? {
        Ranglistentexte.eigeneZeile(stand, meinSub: meinSub)
    }

    /// Erst nachdem einmal etwas ankam, lohnt sich die Liste. Davor läuft der
    /// Ladebalken, statt „noch niemand" zu behaupten.
    var nochNieGeladen: Bool { stand == nil }
}
