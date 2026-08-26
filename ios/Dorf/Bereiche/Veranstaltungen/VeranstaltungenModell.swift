import Foundation
import Observation

/// Zustand der Ansicht „Was ist los in Rössing".
///
/// Ein Fehler heißt nicht „nichts da", sondern „gerade nicht erreichbar":
/// Steht noch eine ältere Liste, bleibt sie stehen und bekommt einen Hinweis
/// darüber. Eine leere Seite ohne Erklärung wäre das schlechteste Ergebnis.
///
/// Es wird nichts geschrieben und nichts gemeldet — die App zeigt hier nur
/// an. Kein Push für Termine: Eine Erinnerung wäre ein eigenes Thema mit
/// eigener Einwilligung.
@Observable
final class VeranstaltungenModell {
    private(set) var termine: [Termin] = []
    private(set) var laedt = false
    private(set) var fehlertext: String?

    private var geholt = false
    private let holen: () async throws -> [VeranstaltungDto]
    /// Die Uhr als Parameter — sonst altern die Tests mit dem Kalender.
    private let uhr: () -> Date

    init(
        holen: @escaping () async throws -> [VeranstaltungDto] = {
            try await WebseiteVeranstaltungen().kommende()
        },
        uhr: @escaping () -> Date = Date.init
    ) {
        self.holen = holen
        self.uhr = uhr
    }

    /// Nichts da, nichts unterwegs, nichts kaputt — dann ist wirklich nichts los.
    var leer: Bool { termine.isEmpty && !laedt && fehlertext == nil }

    /// Der Hinweis über der Liste. Steht noch eine ältere Liste, sagt er, dass
    /// sie alt sein könnte — statt zu behaupten, es gäbe nichts.
    var hinweis: String? {
        guard let fehlertext else { return nil }
        if termine.isEmpty { return fehlertext }
        return "Gerade keine Verbindung — die Liste ist möglicherweise nicht mehr aktuell."
    }

    /// Beim Öffnen des Bereichs. Was schon da ist, wird nicht noch einmal
    /// geholt — nur neu gesiebt, damit ein Termin, der während der laufenden
    /// Sitzung vorbeigegangen ist, verschwindet.
    func laden() async {
        if geholt {
            let jetzt = uhr()
            termine = termine.filter { !$0.istVorbei(jetzt) }
            return
        }
        await holenUndSetzen()
    }

    /// Bewusstes Aktualisieren (Herunterziehen oder „Erneut versuchen").
    func aktualisieren() async {
        await holenUndSetzen()
    }

    private func holenUndSetzen() async {
        if laedt { return }
        laedt = true
        defer { laedt = false }
        do {
            let roh = try await holen()
            geholt = true
            termine = roh.alsTermine(jetzt: uhr())
            fehlertext = nil
        } catch let fehler as VeranstaltungenFehler {
            // Die alte Liste bleibt stehen; der Hinweis kommt darüber.
            fehlertext = fehler.klartext
        } catch {
            fehlertext = VeranstaltungenFehler.netz(error.localizedDescription).klartext
        }
    }
}
