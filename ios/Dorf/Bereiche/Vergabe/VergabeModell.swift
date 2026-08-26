import Combine
import Foundation

/// Woher der Bereich „Anfragen" seine Daten holt.
///
/// Ein Bündel Verschlüsse statt eines Protokolls — genau wie `OrteQuelle`:
/// Der Bereich braucht vier Aufrufe, und ein Test füllt sie selbst. Damit
/// geht kein Test ins Netz.
struct VergabeQuelle {
    var benachrichtigungen: @MainActor () async throws -> [Benachrichtigung]
    var gelesen: @MainActor (Int64) async throws -> Void
    var zusagen: @MainActor (Int64) async throws -> Vorgang
    var zurueckgeben: @MainActor (Int64) async throws -> Vorgang

    static func vom(_ api: DorfApi) -> VergabeQuelle {
        VergabeQuelle(
            benachrichtigungen: { try await api.benachrichtigungen() },
            gelesen: { try await api.gelesen(benachrichtigung: $0) },
            zusagen: { try await api.zusagen(vorgang: $0) },
            zurueckgeben: { try await api.zurueckgeben(vorgang: $0) }
        )
    }
}

/// Eine Zusage, die ich gerade gegeben habe.
///
/// Das Backend schließt mit der Zusage alle offenen Anfragen zu diesem
/// Vorgang — die Karte verschwände sonst im selben Moment aus der Liste, in
/// dem man zugesagt hat. Deshalb bleibt sie hier stehen, bis sie
/// zurückgegeben wird. Der verbindliche Stand steht danach unter
/// „Mithelfen" am Ort selbst.
struct Zusagestand: Identifiable, Hashable {
    var vorgang: Vorgang
    /// „Gießen an „Am Anger““ — wofür die Zusage gilt.
    var was: String

    var id: Int64 { vorgang.id }

    var fristtext: String {
        guard let bis = vorgang.zusageFrist else { return "Du hast zugesagt." }
        return "Du hast zugesagt — bis \(Zeitpunkt.mitUhrzeit(bis))."
    }
}

/// Der Stand des Bereichs „Anfragen": was offen ist, was gerade läuft, was
/// zuletzt schiefging.
///
/// Wie in „Mithelfen" gilt: **Der letzte Stand bleibt stehen.** Fällt das
/// Netz aus, wird die Liste nicht geleert, sondern bekommt einen Hinweis
/// darüber — „gerade ist nichts offen" wäre im Funkloch eine Falschaussage.
final class VergabeModell: ObservableObject {
    private let quelle: VergabeQuelle
    /// Die eigene Kennung — nur, um eine Zusage als die eigene zu erkennen.
    let meinSub: String?

    @Published private(set) var eintraege: [Benachrichtigung] = []
    @Published private(set) var zusagen: [Zusagestand] = []
    @Published private(set) var laeuft = false
    /// Ob überhaupt schon einmal ein Abruf durch war — davor zeigt die Liste
    /// einen Ladekreis statt „nichts offen".
    @Published private(set) var jeGeladen = false
    /// Der letzte Abruf ist gescheitert, oder jemand war schneller — im
    /// Wortlaut des Backends.
    @Published private(set) var hinweis: String?
    /// Ein wirklich abgelehnter Schreibvorgang; als Meldung mit Ok-Knopf.
    @Published private(set) var fehler: String?
    /// Kurze Rückmeldung nach einer Zusage.
    @Published private(set) var bestaetigung: String?
    @Published private(set) var laufendeVorgaenge: Set<Int64> = []
    @Published private(set) var laufendeHinweise: Set<Int64> = []

    private var verblassen: Task<Void, Never>?

    init(quelle: VergabeQuelle, meinSub: String? = nil) {
        self.quelle = quelle
        self.meinSub = meinSub
    }

    convenience init(api: DorfApi, meinSub: String? = nil) {
        self.init(quelle: .vom(api), meinSub: meinSub)
    }

    // MARK: Ansichten auf den Stand

    /// Anfragen zuoberst.
    var geordnet: [Benachrichtigung] { Benachrichtigung.geordnet(eintraege) }

    var offeneAnfragen: Int { eintraege.filter(\.istAnfrage).count }

    /// Nichts offen — weder eine Anfrage noch eine eigene Zusage.
    var leer: Bool { eintraege.isEmpty && zusagen.isEmpty }

    func laeuftGerade(vorgang id: Int64) -> Bool { laufendeVorgaenge.contains(id) }

    func laeuftGerade(hinweis id: Int64) -> Bool { laufendeHinweise.contains(id) }

    // MARK: Laden

    func laden() async {
        laeuft = true
        defer {
            laeuft = false
            jeGeladen = true
        }
        do {
            eintraege = try await quelle.benachrichtigungen()
            hinweis = nil
        } catch let fehler as DorfFehler {
            hinweis = fehler.klartext
        } catch {
            hinweis = Self.netzhinweis
        }
    }

    // MARK: Zusagen

    /// Sagt zu. Kommt ein 409, war **jemand anderes schneller** — das ist
    /// keine Panne: Der Satz des Backends nennt Namen und Frist, und die
    /// Liste wird neu geholt, weil sie ohnehin überholt ist.
    func zusagen(_ n: Benachrichtigung) async {
        guard !laufendeVorgaenge.contains(n.assignmentId) else { return }
        laufendeVorgaenge.insert(n.assignmentId)
        defer { laufendeVorgaenge.remove(n.assignmentId) }
        do {
            let vorgang = try await quelle.zusagen(n.assignmentId)
            let stand = Zusagestand(vorgang: vorgang, was: n.ort)
            merke(stand)
            await laden()
            zeige(bestaetigung: stand.fristtext)
        } catch let abgewiesen as DorfFehler {
            await laden()
            if case .schonVergeben(let grund) = abgewiesen {
                hinweis = grund
            } else {
                fehler = abgewiesen.klartext
            }
        } catch {
            hinweis = Self.netzhinweis
        }
    }

    /// Gibt eine Zusage zurück. Die Rückfrage davor stellt die Oberfläche.
    func zurueckgeben(vorgang id: Int64) async {
        guard !laufendeVorgaenge.contains(id) else { return }
        laufendeVorgaenge.insert(id)
        defer { laufendeVorgaenge.remove(id) }
        do {
            _ = try await quelle.zurueckgeben(id)
            zusagen.removeAll { $0.id == id }
            await laden()
            zeige(bestaetigung: "Zurückgegeben. Jetzt werden wieder die anderen gefragt.")
        } catch let abgewiesen as DorfFehler {
            // Auch hier ist ein 409 kein Fehler: Der Vorgang war schon vorbei.
            zusagen.removeAll { $0.id == id }
            await laden()
            if case .schonVergeben(let grund) = abgewiesen {
                hinweis = grund
            } else {
                fehler = abgewiesen.klartext
            }
        } catch {
            hinweis = Self.netzhinweis
        }
    }

    /// Ein Hinweis ist mit dem Lesen erledigt.
    func gelesen(_ n: Benachrichtigung) async {
        guard !laufendeHinweise.contains(n.id) else { return }
        laufendeHinweise.insert(n.id)
        defer { laufendeHinweise.remove(n.id) }
        do {
            try await quelle.gelesen(n.id)
            eintraege.removeAll { $0.id == n.id }
            await laden()
        } catch let abgewiesen as DorfFehler {
            fehler = abgewiesen.klartext
            await laden()
        } catch {
            hinweis = Self.netzhinweis
        }
    }

    /// Merkt sich die eigene Zusage. Wem sie gehört, sagt das Backend im
    /// Vorgang; ist die eigene Kennung noch nicht geladen, zählt, dass die
    /// Zusage gerade eben von hier kam.
    private func merke(_ stand: Zusagestand) {
        guard meinSub == nil || stand.vorgang.vonMir(meinSub) else { return }
        zusagen.removeAll { $0.id == stand.id }
        zusagen.append(stand)
    }

    // MARK: Meldungen wegräumen

    func fehlerVerwerfen() { fehler = nil }

    func hinweisVerwerfen() { hinweis = nil }

    func bestaetigungVerwerfen() {
        verblassen?.cancel()
        bestaetigung = nil
    }

    /// Derselbe Wortlaut wie `DorfFehler.netz` — damit Netzausfall überall
    /// in der App gleich klingt.
    static let netzhinweis = DorfFehler.netz("").klartext

    private func zeige(bestaetigung text: String) {
        verblassen?.cancel()
        bestaetigung = text
        verblassen = Task { [weak self] in
            try? await Task.sleep(for: .seconds(6))
            guard !Task.isCancelled else { return }
            self?.bestaetigung = nil
        }
    }
}
