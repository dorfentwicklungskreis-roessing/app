import Foundation

/// Fehler, die die Oberfläche kennen muss.
///
/// Der Klartext kommt, wo es einen gibt, aus dem Backend — dort sitzt die
/// Prüfung, und dort steht die Begründung, die für die Person gedacht ist.
/// Die App erfindet keine eigene.
nonisolated enum DorfFehler: Error, Sendable {
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

    // MARK: Innereien — der eine Weg zum Backend
    //
    // Diese Helfer sind bewusst **nicht** `private`, sondern `internal`.
    //
    // Die Endpunkte der App wachsen bereichsweise und wohnen deshalb in
    // Anhängen: `DorfApi+Vergabe.swift`, `+Verwaltung`, `+Konto` und
    // `Push/DorfApi+Geraete.swift`. Ein Anhang in einer *anderen* Datei kommt
    // an `private` nicht heran — und weil er nicht herankam, hat sich jeder
    // Bereich einmal seinen eigenen Transport gebaut. Am Ende gab es vier
    // Fassungen derselben Sache, mit auseinanderlaufenden Fristen,
    // Kopfzeilen und Fehlertexten; welche davon die richtige war, wusste
    // niemand mehr.
    //
    // Genau so weit geht die Öffnung und **keinen Schritt weiter**: `basis`,
    // `sitzung` und `tokenGeber` bleiben `private`. Wer einen Endpunkt
    // ergänzt, benutzt `hole`, `schicke` oder `schickeOhneAntwort` — an die
    // Sitzung selbst kommt er nicht heran und kann sich daher auch keine
    // zweite mit anderen Fristen bauen, ohne dass es auffällt.

    /// Die Adresse eines Pfades unter der Basis. Die Abfrage steht sortiert,
    /// damit dieselbe Anfrage immer gleich aussieht.
    func adresse(_ pfad: String, abfrage: [String: String] = [:]) -> URL {
        var url = basis.appending(path: pfad)
        if !abfrage.isEmpty {
            url.append(queryItems: abfrage.sorted { $0.key < $1.key }
                .map { URLQueryItem(name: $0.key, value: $0.value) })
        }
        return url
    }

    /// Baut die fertige Anfrage: Adresse, Methode, Rumpf, Kopfzeilen und das
    /// **vor jedem Versand frisch geholte** Token.
    ///
    /// Ein eigener Schritt, damit sich ohne Netz prüfen lässt, was
    /// tatsächlich hinausginge — Pfad, Methode, Rumpf und Autorisierung.
    func anfrage(_ methode: String, _ pfad: String,
                 abfrage: [String: String] = [:],
                 rumpf: (any Encodable)? = nil) async throws -> URLRequest {
        var versand = URLRequest(url: adresse(pfad, abfrage: abfrage))
        versand.httpMethod = methode
        versand.setValue("application/json", forHTTPHeaderField: "Accept")
        if let rumpf {
            versand.setValue("application/json", forHTTPHeaderField: "Content-Type")
            versand.httpBody = try JSONEncoder().encode(rumpf)
        }
        if let token = await tokenGeber() {
            versand.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return versand
    }

    func hole<T: Decodable>(_ pfad: String, abfrage: [String: String] = [:]) async throws -> T {
        try await ausfuehren(anfrage("GET", pfad, abfrage: abfrage))
    }

    /// Schreiben mit Antwort. `rumpf` darf fehlen — manche Endpunkte
    /// (Zusagen, Rückgabe) brauchen keinen.
    func schicke<T: Decodable>(_ methode: String, _ pfad: String,
                               rumpf: (any Encodable)? = nil) async throws -> T {
        try await ausfuehren(anfrage(methode, pfad, rumpf: rumpf))
    }

    /// Schreiben ohne Antwort. Der Rumpf des Servers wird verworfen, der
    /// Statuscode aber geprüft.
    func schickeOhneAntwort(_ methode: String, _ pfad: String,
                            abfrage: [String: String] = [:],
                            rumpf: (any Encodable)? = nil) async throws {
        _ = try await rohAusfuehren(anfrage(methode, pfad, abfrage: abfrage, rumpf: rumpf))
    }

    func ausfuehren<T: Decodable>(_ versand: URLRequest) async throws -> T {
        let daten = try await rohAusfuehren(versand)
        // Ein leerer Rumpf ist eine gültige Antwort auf 204 — Aufrufer, die
        // trotzdem etwas erwarten, bekommen hier einen klaren Fehler.
        do {
            return try JSONDecoder().decode(T.self, from: daten)
        } catch {
            throw DorfFehler.netz("Antwort nicht lesbar: \(error.localizedDescription)")
        }
    }

    func rohAusfuehren(_ versand: URLRequest) async throws -> Data {
        let daten: Data
        let antwort: URLResponse
        do {
            (daten, antwort) = try await sitzung.data(for: versand)
        } catch {
            throw DorfFehler.netz(error.localizedDescription)
        }
        guard let http = antwort as? HTTPURLResponse else {
            throw DorfFehler.netz("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            throw Self.fehler(status: http.statusCode, daten: daten,
                              pfad: versand.url?.path ?? "")
        }
        return daten
    }

    /// Statuscode zu Fehler — mit dem Satz des Backends, wo es einen gibt.
    ///
    /// Die **einzige** Übersetzung der App: Dort sitzt die Prüfung, dort
    /// steht die Begründung, und sie wird im Wortlaut weitergereicht. Nur
    /// wenn das Backend gar nichts sagt, steht hier ein Ersatzsatz.
    ///
    /// Als reine Funktion, damit sie sich ohne Netz prüfen lässt.
    static func fehler(status: Int, daten: Data, pfad: String = "") -> DorfFehler {
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

nonisolated extension URLSession {
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
