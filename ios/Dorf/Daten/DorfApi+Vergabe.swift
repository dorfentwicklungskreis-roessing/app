import Foundation

// Die Vergabe der Pflegeaufgaben: anmelden, gefragt werden, zusagen.
//
// Die Regeln — Reihenfolge, Staffelung, Ruhezeiten, Verfall — stehen
// vollständig im Backend (`backend/internal/vergabe`). Hier wird nichts
// nachgerechnet: Die App fragt, was für mich offen ist, und schickt zurück,
// was ich angetippt habe. Ein 409 heißt nicht „Panne", sondern „jemand war
// schneller" — der Satz dazu kommt vom Backend und wird im Wortlaut gezeigt.
//
// Warum die Endpunkte nicht in `DorfApi.swift` stehen: `DorfApi` hält
// Adresse, Sitzung und Token-Geber `private`, und an dieser Datei arbeiten
// gerade mehrere Zweige gleichzeitig. Der Zugang hier folgt demselben Muster
// (`hole`, `schicke`, `DorfFehler`) und gehört beim Zusammenführen in
// `DorfApi` hinein — es soll auf Dauer genau einen Weg zum Backend geben.

// MARK: - DTOs

/// Eine Zustellung an genau mich: „du bist dran" (Anfrage) oder ein Hinweis.
///
/// Die Feldnamen sind der JSON-Vertrag des Backends
/// (`model.Notification`) und bleiben deshalb englisch. Fehlende Felder
/// ergeben Vorgabewerte — das Backend darf ergänzen, ohne dass eine ältere
/// App-Version aufhört zu funktionieren.
nonisolated struct Benachrichtigung: Codable, Identifiable, Hashable, Sendable {
    /// Persönliche Anfrage der Warteschlange: „Du bist als Nächste(r) dran."
    static let anfrage = "anfrage"
    /// Rundruf an alle Angemeldeten, nachdem die Liste durch war.
    static let rundruf = "rundruf"

    var id: Int64 = 0
    var assignmentId: Int64 = 0
    var taskId: Int64 = 0
    /// giessen · jaeten · sonstiges
    var taskKind: String = ""
    var taskName: String = ""
    var placeId: Int64 = 0
    var placeName: String = ""
    /// anfrage · rundruf · zusage_abgelaufen · zusage_aufgehoben ·
    /// vorgang_beendet · vorgang_entfallen
    var kind: String = ""
    var title: String = ""
    var text: String = ""
    var createdAt: String = ""
    /// Bei einer Anfrage der Vortritt, bei einer gehaltenen Zusage deren Ende.
    var expiresAt: String?
    var acknowledgedAt: String?

    enum CodingKeys: String, CodingKey {
        case id, assignmentId, taskId, taskKind, taskName, placeId, placeName
        case kind, title, text, createdAt, expiresAt, acknowledgedAt
    }

    init(id: Int64 = 0, assignmentId: Int64 = 0, taskId: Int64 = 0, taskKind: String = "",
         taskName: String = "", placeId: Int64 = 0, placeName: String = "", kind: String = "",
         title: String = "", text: String = "", createdAt: String = "",
         expiresAt: String? = nil, acknowledgedAt: String? = nil) {
        self.id = id; self.assignmentId = assignmentId; self.taskId = taskId
        self.taskKind = taskKind; self.taskName = taskName; self.placeId = placeId
        self.placeName = placeName; self.kind = kind; self.title = title; self.text = text
        self.createdAt = createdAt; self.expiresAt = expiresAt
        self.acknowledgedAt = acknowledgedAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, 0)
        assignmentId = c.wert(.assignmentId, 0)
        taskId = c.wert(.taskId, 0)
        taskKind = c.wert(.taskKind, "")
        taskName = c.wert(.taskName, "")
        placeId = c.wert(.placeId, 0)
        placeName = c.wert(.placeName, "")
        kind = c.wert(.kind, "")
        title = c.wert(.title, "")
        text = c.wert(.text, "")
        createdAt = c.wert(.createdAt, "")
        expiresAt = c.wertOptional(.expiresAt)
        acknowledgedAt = c.wertOptional(.acknowledgedAt)
    }

    /// Anfrage und Rundruf wollen eine Antwort; alles andere ist ein Hinweis,
    /// der mit dem Lesen erledigt ist. Dieselbe Regel wie im Backend
    /// (`NotificationKind.IsRequest`).
    var istAnfrage: Bool { kind == Self.anfrage || kind == Self.rundruf }

    var frist: Date? { expiresAt.flatMap(RFC3339.datum) }
    var erstellt: Date? { RFC3339.datum(createdAt) }
    var gelesen: Bool { !(acknowledgedAt ?? "").isEmpty }

    /// Ob die Frist verstrichen ist. Bei einer Anfrage heißt das nur: Der
    /// Vortritt ist weg, gefragt wird jetzt der Nächste. **Zusagen darf man
    /// weiterhin** — das entscheidet ohnehin das Backend.
    func abgelaufen(jetzt: Date = Date()) -> Bool {
        guard let frist else { return false }
        return jetzt >= frist
    }
}

/// Ergänzungen zum Vorgang aus `Modelle.swift` — gelesen, nicht gerechnet.
nonisolated extension Vorgang {
    /// Bis wann die Zusage hält. Die Dauer setzt das Backend (Vorgabe 24 h).
    var zusageFrist: Date? { claimedUntil.flatMap(RFC3339.datum) }

    /// Die Liste war einmal durch: Jetzt wird offen gesucht.
    var rundruf: Bool { state == "rundruf" }
}

/// Antwort von `GET /api/v1/me/notifications`.
nonisolated struct BenachrichtigungenAntwort: Codable, Sendable {
    var notifications: [Benachrichtigung] = []

    enum CodingKeys: String, CodingKey { case notifications }

    init(notifications: [Benachrichtigung] = []) { self.notifications = notifications }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        notifications = c.wert(.notifications, [])
    }
}

/// Eingabe von `POST /api/v1/places/{id}/signup`. Leere Aufgabenart heißt:
/// alles, was an diesem Ort anfällt.
nonisolated struct AnmeldeEingabe: Codable, Sendable {
    var taskKind: String = ""
}

// MARK: - Zugang

/// Der Zugang zu den Vergabe-Endpunkten — gebaut wie `DorfApi`: nur
/// `URLSession` und `Codable`, das Token vor jeder Anfrage frisch vom
/// `tokenGeber`, Fehler als `DorfFehler` mit dem Klartext des Backends.
nonisolated final class VergabeApi: Sendable {
    private let basis: URL
    private let sitzung: URLSession
    private let tokenGeber: @Sendable () async -> String?

    init(basis: URL = Konfiguration.apiBasis,
         sitzung: URLSession = .dorfSitzung,
         tokenGeber: @escaping @Sendable () async -> String?) {
        self.basis = basis
        self.sitzung = sitzung
        self.tokenGeber = tokenGeber
    }

    // MARK: Anmeldung zum Mithelfen

    /// „Ich helfe hier mit." `art` = nil meldet für alle Aufgaben des Ortes an.
    func anmelden(ort: Int64, art: String? = nil) async throws {
        _ = try await roh("POST", "api/v1/places/\(ort)/signup",
                          rumpf: AnmeldeEingabe(taskKind: art ?? ""))
    }

    /// „Ich mag nicht mehr." Ohne Aufgabenart wird der ganze Ort abgemeldet.
    func abmelden(ort: Int64, art: String? = nil) async throws {
        var abfrage: [String: String] = [:]
        if let art, !art.isEmpty { abfrage["taskKind"] = art }
        _ = try await roh("DELETE", "api/v1/places/\(ort)/signup", abfrage: abfrage)
    }

    // MARK: Benachrichtigungen

    func benachrichtigungen() async throws -> [Benachrichtigung] {
        let antwort: BenachrichtigungenAntwort = try await hole("api/v1/me/notifications")
        return antwort.notifications
    }

    /// Gelesen. Hinweise sind damit erledigt; Anfragen bleiben stehen, bis
    /// der Vorgang sie schließt — sonst wäre die Aufgabe aus der App
    /// verschwunden, bevor jemand zugesagt hat.
    func gelesen(benachrichtigung id: Int64) async throws {
        _ = try await roh("POST", "api/v1/me/notifications/\(id)/ack")
    }

    // MARK: Zusagen

    /// Zusagen. **409 heißt: jemand anderes war schneller** — daraus wird
    /// `DorfFehler.schonVergeben`, und der Grund nennt Name und Frist im
    /// Wortlaut des Backends.
    func zusagen(vorgang id: Int64) async throws -> Vorgang {
        try await antwort("POST", "api/v1/assignments/\(id)/claim")
    }

    /// Zusage zurückgeben. Der Vorgang läuft danach weiter — die Nächsten
    /// werden gefragt.
    func zurueckgeben(vorgang id: Int64) async throws -> Vorgang {
        try await antwort("POST", "api/v1/assignments/\(id)/release")
    }

    // MARK: Innereien

    private func adresse(_ pfad: String, abfrage: [String: String] = [:]) -> URL {
        var url = basis.appending(path: pfad)
        if !abfrage.isEmpty {
            url.append(queryItems: abfrage.sorted { $0.key < $1.key }
                .map { URLQueryItem(name: $0.key, value: $0.value) })
        }
        return url
    }

    private func hole<T: Decodable>(_ pfad: String) async throws -> T {
        try await lies(roh("GET", pfad))
    }

    private func antwort<T: Decodable>(_ methode: String, _ pfad: String) async throws -> T {
        try await lies(roh(methode, pfad))
    }

    private func lies<T: Decodable>(_ daten: Data) throws -> T {
        do {
            return try JSONDecoder().decode(T.self, from: daten)
        } catch {
            throw DorfFehler.netz("Antwort nicht lesbar: \(error.localizedDescription)")
        }
    }

    private func roh(_ methode: String, _ pfad: String,
                     abfrage: [String: String] = [:]) async throws -> Data {
        var anfrage = URLRequest(url: adresse(pfad, abfrage: abfrage))
        anfrage.httpMethod = methode
        return try await ausfuehren(anfrage)
    }

    private func roh<Rumpf: Encodable>(_ methode: String, _ pfad: String,
                                       rumpf: Rumpf) async throws -> Data {
        var anfrage = URLRequest(url: adresse(pfad))
        anfrage.httpMethod = methode
        anfrage.setValue("application/json", forHTTPHeaderField: "Content-Type")
        anfrage.httpBody = try JSONEncoder().encode(rumpf)
        return try await ausfuehren(anfrage)
    }

    private func ausfuehren(_ vorlage: URLRequest) async throws -> Data {
        var anfrage = vorlage
        if let token = await tokenGeber() {
            anfrage.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        anfrage.setValue("application/json", forHTTPHeaderField: "Accept")

        let daten: Data
        let antwort: URLResponse
        do {
            (daten, antwort) = try await sitzung.data(for: anfrage)
        } catch {
            throw DorfFehler.netz(error.localizedDescription)
        }
        guard let http = antwort as? HTTPURLResponse else {
            throw DorfFehler.netz("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            throw Self.fehler(status: http.statusCode, daten: daten)
        }
        return daten
    }

    /// Statuscode zu Fehler — mit dem Satz des Backends, wo es einen gibt.
    /// In der Vergabe bedeutet 409 immer „zu spät": schon vergeben, schon
    /// erledigt, schon zurückgegeben. Der Grund steht im Rumpf.
    static func fehler(status: Int, daten: Data) -> DorfFehler {
        let antwort = try? JSONDecoder().decode(ApiFehlerAntwort.self, from: daten)
        let grund = antwort?.error.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        switch status {
        case 400:
            return .abgelehnt(grund: grund.isEmpty ? "Die Eingabe wurde abgelehnt." : grund)
        case 401:
            return .nichtAngemeldet
        case 403:
            return .keineBerechtigung(grund: grund.isEmpty ? "Dafür fehlt die Berechtigung." : grund)
        case 404:
            return .nichtGefunden
        case 409:
            return .schonVergeben(grund: grund.isEmpty
                ? "Das hat gerade jemand anderes übernommen." : grund)
        case 429:
            return .zuVieleAnfragen
        default:
            return .serverfehler(status: status)
        }
    }
}

// MARK: - Verdrahtung

extension AppUmgebung {
    /// Der Vergabe-Zugang der App — mit demselben Token wie `api`.
    ///
    /// Als berechnete Eigenschaft in dieser Datei, damit `Umgebung.swift`
    /// unangetastet bleibt. Der Zugang hält keinen Zustand: Er ist eine
    /// Handvoll Felder um `URLSession.dorfSitzung`, die ohnehin geteilt wird.
    var vergabe: VergabeApi {
        VergabeApi(tokenGeber: { [anmeldung] in await anmeldung.frischesToken() })
    }
}
