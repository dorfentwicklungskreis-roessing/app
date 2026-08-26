import Foundation

/// Fehler, die die Oberfläche kennen muss.
///
/// Der Klartext kommt, wo es einen gibt, aus dem Backend — dort sitzt die
/// Prüfung, und dort steht die Begründung, die für die Person gedacht ist.
/// Die App erfindet keine eigene.
enum DorfFehler: Error, Sendable {
    /// Spielschutz: dieselbe Aufgabe ist noch gesperrt (HTTP 409).
    case gesperrt(wiederAb: Date?)
    /// Das Backend hat die Eingabe abgewiesen und nennt den Grund (HTTP 400).
    case abgelehnt(grund: String)
    /// Die Rolle fehlt (HTTP 403).
    case keineBerechtigung(grund: String)
    /// Jemand anderes war schneller (HTTP 409 bei der Vergabe).
    case schonVergeben(grund: String)
    /// Zu viele Anfragen in kurzer Zeit (HTTP 429).
    case zuVieleAnfragen
    /// Nicht (mehr) angemeldet (HTTP 401).
    case nichtAngemeldet
    case nichtGefunden
    case serverfehler(status: Int)
    case netz(String)

    var klartext: String {
        switch self {
        case .gesperrt(let wiederAb):
            guard let wiederAb else { return "Das wurde gerade erst erledigt." }
            let f = DateFormatter()
            f.locale = Locale(identifier: "de_DE")
            f.timeZone = TimeZone(identifier: "Europe/Berlin")
            f.dateFormat = "dd.MM., HH:mm"
            return "Das wurde gerade erst erledigt — wieder ab \(f.string(from: wiederAb)) Uhr."
        case .abgelehnt(let grund): return grund
        case .keineBerechtigung(let grund): return grund
        case .schonVergeben(let grund): return grund
        case .zuVieleAnfragen: return "Das waren gerade viele auf einmal. Bitte später noch einmal."
        case .nichtAngemeldet: return "Die Anmeldung ist abgelaufen. Bitte neu anmelden."
        case .nichtGefunden: return "Das gibt es nicht (mehr)."
        case .serverfehler(let status): return "Der Server antwortet gerade nicht (\(status))."
        case .netz: return "Keine Verbindung zum Server. Es werden ggf. alte Daten angezeigt."
        }
    }
}

/// Der Zugang zum Dorf-Backend.
///
/// Bewusst nur `URLSession` und `Codable` — keine Netzwerk-Bibliothek. Die
/// API ist klein, und jede Abhängigkeit müsste über Jahre mitgepflegt werden.
/// Das Zugangstoken holt sich der Client **vor jeder Anfrage** frisch beim
/// `tokenGeber`; erneuert wird es dort, nicht hier.
nonisolated final class DorfApi: Sendable {
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

    // MARK: Lesen

    func ich() async throws -> Ich { try await hole("api/v1/me") }

    func orte() async throws -> OrteAntwort { try await hole("api/v1/places") }

    func erledigungen(aufgabe id: Int64) async throws -> [Erledigung] {
        let antwort: ErledigungenAntwort = try await hole("api/v1/tasks/\(id)/completions")
        return antwort.completions
    }

    func rangliste(zeitraum: Zeitraum) async throws -> Rangliste {
        try await hole("api/v1/stats/leaderboard", abfrage: ["period": zeitraum.rawValue])
    }

    func dorfbewohner() async throws -> DorfbewohnerAntwort { try await hole("api/v1/members") }

    // MARK: Schreiben

    /// Meldet eine Erledigung. Der Spielschutz des Backends antwortet mit 409
    /// und nennt im Rumpf den Zeitpunkt, ab dem wieder gemeldet werden darf —
    /// daraus wird hier `DorfFehler.gesperrt`, damit die Oberfläche nicht in
    /// HTTP-Codes denken muss.
    func melden(aufgabe id: Int64, liter: Double?, notiz: String = "") async throws -> Erledigung {
        try await schicke("POST", "api/v1/tasks/\(id)/completions",
                          rumpf: ErledigungEingabe(liters: liter, note: notiz))
    }

    func erledigungZuruecknehmen(id: Int64) async throws {
        try await schickeOhneAntwort("DELETE", "api/v1/completions/\(id)")
    }

    func profilSpeichern(_ eingabe: ProfilEingabe) async throws -> Profil {
        try await schicke("PUT", "api/v1/me/profile", rumpf: eingabe)
    }

    /// Reicht einen Wunsch ein. Derselbe Eingang, den auch das Formular auf
    /// der Website benutzt; aus der App geht das Token mit, damit die Idee dem
    /// Konto zugeordnet wird.
    func ideeEinreichen(_ eingabe: IdeeEingabe) async throws -> Idee {
        try await schicke("POST", "api/v1/ideen", rumpf: eingabe)
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

    private func hole<T: Decodable>(_ pfad: String, abfrage: [String: String] = [:]) async throws -> T {
        var anfrage = URLRequest(url: adresse(pfad, abfrage: abfrage))
        anfrage.httpMethod = "GET"
        return try await ausfuehren(anfrage)
    }

    private func schicke<Rumpf: Encodable, T: Decodable>(
        _ methode: String, _ pfad: String, rumpf: Rumpf
    ) async throws -> T {
        var anfrage = URLRequest(url: adresse(pfad))
        anfrage.httpMethod = methode
        anfrage.setValue("application/json", forHTTPHeaderField: "Content-Type")
        anfrage.httpBody = try JSONEncoder().encode(rumpf)
        return try await ausfuehren(anfrage)
    }

    private func schickeOhneAntwort(_ methode: String, _ pfad: String) async throws {
        var anfrage = URLRequest(url: adresse(pfad))
        anfrage.httpMethod = methode
        _ = try await rohAusfuehren(anfrage)
    }

    private func ausfuehren<T: Decodable>(_ anfrage: URLRequest) async throws -> T {
        let daten = try await rohAusfuehren(anfrage)
        // Ein leerer Rumpf ist eine gültige Antwort auf 204 — Aufrufer, die
        // trotzdem etwas erwarten, bekommen hier einen klaren Fehler.
        do {
            return try JSONDecoder().decode(T.self, from: daten)
        } catch {
            throw DorfFehler.netz("Antwort nicht lesbar: \(error.localizedDescription)")
        }
    }

    private func rohAusfuehren(_ vorlage: URLRequest) async throws -> Data {
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
            throw fehler(status: http.statusCode, daten: daten, pfad: anfrage.url?.path ?? "")
        }
        return daten
    }

    private func fehler(status: Int, daten: Data, pfad: String) -> DorfFehler {
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
            // Zwei verschiedene 409 im Backend: der Spielschutz nennt
            // retryAfter, die Vergabe nennt nur einen Grund im Klartext.
            if let retry = antwort?.retryAfter {
                return .gesperrt(wiederAb: RFC3339.datum(retry))
            }
            if pfad.contains("/completions") {
                return .gesperrt(wiederAb: nil)
            }
            return .schonVergeben(grund: grund.isEmpty
                ? "Das hat gerade jemand anderes übernommen." : grund)
        case 429:
            return .zuVieleAnfragen
        default:
            return .serverfehler(status: status)
        }
    }
}

extension URLSession {
    /// Fristen wie auf Android (10 s Verbindung, 20 s Antwort): lang genug für
    /// eine schlechte Mobilverbindung, kurz genug, dass die Oberfläche nicht
    /// minutenlang wartend dasteht.
    static let dorfSitzung: URLSession = {
        let k = URLSessionConfiguration.default
        k.timeoutIntervalForRequest = 20
        k.timeoutIntervalForResource = 30
        k.waitsForConnectivity = false
        return URLSession(configuration: k)
    }()
}
