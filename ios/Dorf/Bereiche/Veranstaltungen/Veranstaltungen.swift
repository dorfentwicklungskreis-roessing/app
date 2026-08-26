import Foundation

/// „Was ist los in Rössing" — die Veranstaltungen kommen von der Website.
///
/// Gepflegt werden sie dort (`src/content/events/` auf rössing.de) und nur
/// dort; die Website legt sie beim Bauen zusätzlich als `/events.json` ab.
/// Damit gibt es keine zweite Pflegestelle, keine neue Verwaltungsoberfläche
/// und im Dorf-Backend nichts, was veralten könnte.
///
/// Wichtig: Diese Abfrage geht an einen **anderen** Server als die Dorf-API
/// und läuft deshalb über einen eigenen, schlichten Client — **ohne** das
/// Zugangstoken. Die Website ist öffentlich und hat mit unserer Anmeldung
/// nichts zu tun. Deshalb steht hier bewusst kein `DorfApi`.

// MARK: - Was die Datei liefert

/// Ort einer Veranstaltung, wie ihn `/events.json` liefert.
struct VeranstaltungsortDto: Codable, Hashable, Sendable {
    var name: String = ""
    var address: String = ""
    var lat: Double?
    var lon: Double?

    enum CodingKeys: String, CodingKey { case name, address, lat, lon }

    init(name: String = "", address: String = "", lat: Double? = nil, lon: Double? = nil) {
        self.name = name; self.address = address; self.lat = lat; self.lon = lon
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = c.wert(.name, "")
        address = c.wert(.address, "")
        lat = c.wertOptional(.lat)
        lon = c.wertOptional(.lon)
    }
}

/// Veranstalter einer Veranstaltung.
struct VeranstalterDto: Codable, Hashable, Sendable {
    var name: String = ""

    enum CodingKeys: String, CodingKey { case name }

    init(name: String = "") { self.name = name }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = c.wert(.name, "")
    }
}

/// Eine Veranstaltung aus `/events.json`.
///
/// Die Feldnamen bleiben englisch — sie sind der Vertrag mit der Website.
/// Fehlende Felder fallen auf ihre Vorgabe zurück: Die Website darf etwas
/// ergänzen, ohne dass ältere App-Versionen eine leere Liste zeigen.
struct VeranstaltungDto: Codable, Hashable, Sendable {
    var id: String = ""
    var name: String = ""
    var description: String = ""
    /// Ganztägig nur das Datum (`2026-08-17`), sonst die Ortszeit mit Offset
    /// (`2026-08-20T18:00:00+02:00`).
    var start: String = ""
    var end: String?
    var allDay: Bool = false
    /// Externe Primärquelle, falls `external`, sonst die Seite auf rössing.de.
    var url: String = ""
    var external: Bool = false
    var location: VeranstaltungsortDto?
    var organizer: VeranstalterDto?
    var image: String?

    enum CodingKeys: String, CodingKey {
        case id, name, description, start, end, allDay, url, external
        case location, organizer, image
    }

    init(id: String = "", name: String = "", description: String = "", start: String = "",
         end: String? = nil, allDay: Bool = false, url: String = "", external: Bool = false,
         location: VeranstaltungsortDto? = nil, organizer: VeranstalterDto? = nil,
         image: String? = nil) {
        self.id = id; self.name = name; self.description = description; self.start = start
        self.end = end; self.allDay = allDay; self.url = url; self.external = external
        self.location = location; self.organizer = organizer; self.image = image
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        name = c.wert(.name, "")
        description = c.wert(.description, "")
        start = c.wert(.start, "")
        end = c.wertOptional(.end)
        allDay = c.wert(.allDay, false)
        url = c.wert(.url, "")
        external = c.wert(.external, false)
        location = c.wertOptional(.location)
        organizer = c.wertOptional(.organizer)
        image = c.wertOptional(.image)
    }
}

struct VeranstaltungenFeedDto: Codable, Sendable {
    var version: Int = 1
    var generatedAt: String = ""
    var events: [VeranstaltungDto] = []

    enum CodingKeys: String, CodingKey { case version, generatedAt, events }

    init(version: Int = 1, generatedAt: String = "", events: [VeranstaltungDto] = []) {
        self.version = version; self.generatedAt = generatedAt; self.events = events
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = c.wert(.version, 1)
        generatedAt = c.wert(.generatedAt, "")
        events = c.wert(.events, [])
    }
}

// MARK: - Fehler

/// Was beim Holen schiefgehen kann. Die Ansicht macht daraus einen Hinweis
/// über der Liste — still verschlucken wäre das Schlimmste.
enum VeranstaltungenFehler: Error, Sendable {
    case netz(String)
    case serverfehler(status: Int)
    case nichtLesbar

    var klartext: String {
        switch self {
        case .netz:
            return "Die Termine konnten nicht geladen werden. Besteht eine Verbindung?"
        case .serverfehler(let status):
            return "Die Website antwortet gerade nicht (\(status))."
        case .nichtLesbar:
            return "Die Terminliste der Website war nicht lesbar."
        }
    }
}

// MARK: - Der Weg zur Website

/// Holt `events.json` von der Website. Bewusst nur `URLSession` und
/// `Codable`, und bewusst **kein** `Authorization`-Kopf: Ein Token an einen
/// fremden Server wäre eine unnötige Preisgabe.
///
/// Die Adresse kommt ausschließlich aus `Konfiguration.webseiteBasis`, damit
/// CI und E2E sie lokal übersteuern können.
///
/// Die Klasse bleibt in der Vorbelegung des Projekts (`MainActor`): Sie hält
/// nichts fest, und `URLSession` gibt den Faden beim Warten ohnehin frei.
final class WebseiteVeranstaltungen {
    private let basis: URL
    private let sitzung: URLSession

    init(basis: URL = Konfiguration.webseiteBasis, sitzung: URLSession = .webseiteSitzung) {
        self.basis = basis
        self.sitzung = sitzung
    }

    /// Holt die Liste, wie sie beim letzten Bauen der Website entstanden ist.
    func kommende() async throws -> [VeranstaltungDto] {
        try await feed().events
    }

    func feed() async throws -> VeranstaltungenFeedDto {
        var anfrage = URLRequest(url: basis.appending(path: "events.json"))
        anfrage.httpMethod = "GET"
        anfrage.setValue("application/json", forHTTPHeaderField: "Accept")

        let daten: Data
        let antwort: URLResponse
        do {
            (daten, antwort) = try await sitzung.data(for: anfrage)
        } catch {
            throw VeranstaltungenFehler.netz(error.localizedDescription)
        }
        guard let http = antwort as? HTTPURLResponse else {
            throw VeranstaltungenFehler.netz("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            throw VeranstaltungenFehler.serverfehler(status: http.statusCode)
        }
        // Kommt statt der Datei eine Fehlerseite (Zwischenspeicher, Portal,
        // umgezogene Adresse), darf das nicht als „nichts los" durchgehen.
        do {
            return try JSONDecoder().decode(VeranstaltungenFeedDto.self, from: daten)
        } catch {
            throw VeranstaltungenFehler.nichtLesbar
        }
    }
}

extension URLSession {
    /// Eigene Sitzung für die Website — getrennt von der des Backends, damit
    /// hier nie versehentlich etwas mitfährt, das nur die Dorf-API angeht.
    /// Fristen wie auf Android: 10 s Verbindung, 20 s Antwort.
    static let webseiteSitzung: URLSession = {
        let k = URLSessionConfiguration.default
        k.timeoutIntervalForRequest = 20
        k.timeoutIntervalForResource = 30
        k.waitsForConnectivity = false
        return URLSession(configuration: k)
    }()
}
