import Combine
import Foundation

/// Der Verlauf eines Gesprächs — in der App, nicht auf dem Server.
///
/// Das Backend hält bewusst keine Sitzung: Es bekommt bei jeder Frage den
/// Verlauf mitgeschickt und vergisst ihn danach. Was hier steht, ist also
/// alles, was es von dem Gespräch gibt; wer die App schließt, fängt neu an.
///
/// Was hier NICHT steht, sind Rechte: Der Chat sieht und darf genau das,
/// was diese Person auch sonst sieht und darf. Entschieden wird das im
/// Backend (`model.Zugriff`) — eine App, die es selbst entschiede, wäre eine
/// zweite Wahrheit und beim nächsten Sonderfall die falsche.
final class ChatModell: ObservableObject {
    /// Die bisherigen Züge, ältester zuerst.
    @Published private(set) var verlauf: [Gespraechszug] = []
    /// Der getippte Text. Er bleibt bei einem Fehler stehen — niemand tippt
    /// gern zweimal.
    @Published var entwurf = ""
    @Published private(set) var laeuft = false
    /// Klartext der letzten Störung, vom Backend.
    @Published private(set) var hinweis: String?
    /// Nil, solange ungeprüft; danach der Stand des Bereichs.
    @Published private(set) var stand: Chatstand?

    /// So viele Züge gehen mit. Mehr braucht ein Dorfgespräch nicht, und
    /// jeder Zug wird bei jeder Frage erneut bezahlt — dieselbe Grenze wie
    /// im Backend (`MaxVerlauf`).
    static let maxVerlauf = 20
    /// So lang darf eine Frage sein (`MaxFrage` im Backend).
    static let maxFrage = 2000

    var eingerichtet: Bool { stand?.verfuegbar ?? false }

    var absendbar: Bool {
        !laeuft && eingerichtet
            && !entwurf.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && entwurf.count <= Self.maxFrage
    }

    /// Fragt den Bereichsstand ab.
    ///
    /// Scheitert schon diese Frage — kein Netz —, gilt der Chat als
    /// verfügbar: Dann sagt der erste Versuch die Wahrheit, statt dass ein
    /// Aussetzer der Leitung wie eine dauerhafte Abschaltung aussieht. „Noch
    /// nicht eingerichtet" sagt der Bereich nur, wenn das Backend es sagt.
    func standLaden(api: DorfApi) async {
        do {
            stand = try await api.chatstand()
            hinweis = nil
        } catch {
            if Task.isCancelled { return }
            stand = Chatstand(verfuegbar: true)
            hinweis = (error as? DorfFehler)?.klartext ?? "Unerwarteter Fehler."
        }
    }

    /// Schickt den Entwurf ab.
    func fragen(api: DorfApi) async {
        let frage = entwurf.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !frage.isEmpty, !laeuft else { return }
        let mitgeschickt = Array(verlauf.suffix(Self.maxVerlauf))
        laeuft = true
        hinweis = nil
        entwurf = ""
        verlauf.append(Gespraechszug(rolle: Gespraechszug.rolleIch, text: frage))
        do {
            let antwort = try await api.chatFragen(frage, verlauf: mitgeschickt)
            var text = antwort.antwort
            if text.isEmpty {
                text = "Darauf habe ich keine Antwort gefunden. Frag es gern anders."
            }
            verlauf.append(Gespraechszug(rolle: Gespraechszug.rolleApp, text: text,
                                         werkzeuge: antwort.werkzeuge,
                                         abgebrochen: antwort.abgebrochen))
        } catch {
            if Task.isCancelled {
                laeuft = false
                return
            }
            // Der Satz kommt aus dem Backend, wo die Prüfung sitzt. Die
            // Frage wandert zurück ins Eingabefeld und verschwindet wieder
            // aus dem Verlauf: Ein zweiter Versuch ist dann ein Tipp und
            // kein Abtippen, und im Gespräch bleibt keine Frage stehen, auf
            // die nie eine Antwort kam.
            hinweis = (error as? DorfFehler)?.klartext ?? "Unerwarteter Fehler."
            entwurf = frage
            verlauf.removeLast()
        }
        laeuft = false
    }

    /// Fängt von vorn an. Der Verlauf lebt nur hier — mehr ist nicht zu tun.
    func neuAnfangen() {
        verlauf.removeAll()
        hinweis = nil
        entwurf = ""
    }
}
