import Foundation
import Observation

/// Woher „Mithelfen" seine Daten holt.
///
/// Ein kleines Bündel Funktionen statt eines Protokolls: Der Bereich braucht
/// genau vier Aufrufe, und ein Test reicht dafür vier Verschlüsse herein,
/// ohne eine Attrappe des ganzen `DorfApi` bauen zu müssen. Damit geht kein
/// Test ins Netz — und `DorfApi` bleibt der einzige Weg zum Backend.
struct OrteQuelle {
    var orte: @MainActor () async throws -> OrteAntwort
    var erledigungen: @MainActor (Int64) async throws -> [Erledigung]
    var melden: @MainActor (Int64, Double?, String) async throws -> Erledigung
    var zuruecknehmen: @MainActor (Int64) async throws -> Void

    static func vom(_ api: DorfApi) -> OrteQuelle {
        OrteQuelle(
            orte: { try await api.orte() },
            erledigungen: { try await api.erledigungen(aufgabe: $0) },
            melden: { try await api.melden(aufgabe: $0, liter: $1, notiz: $2) },
            zuruecknehmen: { try await api.erledigungZuruecknehmen(id: $0) }
        )
    }
}

/// Der Stand des Bereichs „Mithelfen": welche Orte es gibt, was gerade läuft,
/// was zuletzt schiefging.
///
/// Wichtigste Eigenschaft: **Der letzte Stand bleibt stehen.** Fällt das Netz
/// aus, wird die Liste nicht geleert — sie bekommt einen Hinweis darüber.
/// Eine leere Seite im Funkloch wäre eine Falschaussage („es steht nichts an"),
/// und im Dorf ist die Verbindung nun einmal nicht überall gut.
@Observable
final class OrteModell {
    private let quelle: OrteQuelle

    private(set) var orte: [Ort] = []
    /// Wetterfaktor des Backends (1 = normal). Wird angezeigt, nicht gerechnet.
    private(set) var giessfaktor: Double = 1
    private(set) var laeuft = false
    /// Ob überhaupt schon einmal ein Abruf durch war — davor zeigt die Liste
    /// einen Ladekreis statt „nichts zu tun".
    private(set) var jeGeladen = false
    /// Der letzte Abruf ist gescheitert; im Wortlaut des Backends.
    private(set) var hinweis: String?
    /// Kurze Rückmeldung nach einer Meldung („Danke fürs Gießen! 💚").
    private(set) var bestaetigung: String?
    /// Ein abgelehnter Schreibvorgang — im Wortlaut des Backends.
    private(set) var fehler: String?
    /// Aufgaben, für die gerade geschrieben wird (Knopf sperren).
    private(set) var laufendeAufgaben: Set<Int64> = []
    /// Historie je Aufgabe. Wird nachgeladen und blockiert das Öffnen nicht.
    private(set) var historie: [Int64: [Erledigung]] = [:]

    @ObservationIgnored private var verblassen: Task<Void, Never>?

    init(quelle: OrteQuelle) { self.quelle = quelle }

    convenience init(api: DorfApi) { self.init(quelle: .vom(api)) }

    // MARK: Ansichten auf den Stand

    /// Dringendste zuerst: rot, dann gelb, dann grün. Bei gleicher Ampel
    /// alphabetisch, damit die Liste zwischen zwei Abrufen nicht springt.
    var nachDringlichkeit: [Ort] {
        orte.sorted { links, rechts in
            if links.ampel.dringlichkeit != rechts.ampel.dringlichkeit {
                return links.ampel.dringlichkeit < rechts.ampel.dringlichkeit
            }
            return links.name.localizedCaseInsensitiveCompare(rechts.name) == .orderedAscending
        }
    }

    func ort(id: Int64) -> Ort? { orte.first { $0.id == id } }

    func laeuftGerade(_ aufgabe: Int64) -> Bool { laufendeAufgaben.contains(aufgabe) }

    // MARK: Laden

    func laden() async {
        laeuft = true
        defer {
            laeuft = false
            jeGeladen = true
        }
        do {
            let antwort = try await quelle.orte()
            orte = antwort.places
            giessfaktor = antwort.wateringFactor
            hinweis = nil
        } catch let fehler as DorfFehler {
            // Der alte Stand bleibt stehen — nur der Hinweis kommt dazu.
            hinweis = fehler.klartext
        } catch {
            hinweis = OrteModell.netzhinweis
        }
    }

    /// Historie einer Aufgabe nachladen. Scheitert das, bleibt die Seite ohne
    /// Historie — dafür macht niemand ein Fenster auf.
    func historieLaden(aufgabe id: Int64) async {
        guard let liste = try? await quelle.erledigungen(id) else { return }
        historie[id] = liste
    }

    // MARK: Schreiben

    /// Meldet eine Aufgabe als erledigt und lädt danach neu.
    ///
    /// Kommt trotz gesperrtem Knopf ein 409 (zwei Geräte, zwei Personen), wird
    /// der Satz des Backends gezeigt und der Stand neu geholt — er ist dann
    /// ohnehin überholt.
    func melden(_ aufgabe: Aufgabe) async {
        guard !laufendeAufgaben.contains(aufgabe.id) else { return }
        laufendeAufgaben.insert(aufgabe.id)
        defer { laufendeAufgaben.remove(aufgabe.id) }
        do {
            _ = try await quelle.melden(aufgabe.id, aufgabe.liters, "")
            zeige(bestaetigung: aufgabe.dankeText)
            await historieLaden(aufgabe: aufgabe.id)
            await laden()
        } catch let abgewiesen as DorfFehler {
            fehler = abgewiesen.klartext
            await laden()
        } catch {
            fehler = OrteModell.netzhinweis
        }
    }

    /// Nimmt eine irrtümliche eigene Meldung zurück.
    func zuruecknehmen(_ erledigung: Erledigung) async {
        guard !laufendeAufgaben.contains(erledigung.taskId) else { return }
        laufendeAufgaben.insert(erledigung.taskId)
        defer { laufendeAufgaben.remove(erledigung.taskId) }
        do {
            try await quelle.zuruecknehmen(erledigung.id)
            zeige(bestaetigung: "Meldung zurückgenommen.")
            await historieLaden(aufgabe: erledigung.taskId)
            await laden()
        } catch let abgewiesen as DorfFehler {
            fehler = abgewiesen.klartext
            await laden()
        } catch {
            fehler = OrteModell.netzhinweis
        }
    }

    // MARK: Meldungen wegräumen

    func fehlerVerwerfen() { fehler = nil }

    func bestaetigungVerwerfen() {
        verblassen?.cancel()
        bestaetigung = nil
    }

    /// Der Dank steht kurz da und geht dann von allein — er ist eine
    /// Rückmeldung, keine Aufgabe.
    private func zeige(bestaetigung text: String) {
        verblassen?.cancel()
        bestaetigung = text
        verblassen = Task { [weak self] in
            try? await Task.sleep(for: .seconds(5))
            guard !Task.isCancelled else { return }
            self?.bestaetigung = nil
        }
    }

    /// Derselbe Wortlaut wie `DorfFehler.netz` — an einer Stelle, damit
    /// Netzausfall überall gleich klingt.
    static let netzhinweis = DorfFehler.netz("").klartext
}
