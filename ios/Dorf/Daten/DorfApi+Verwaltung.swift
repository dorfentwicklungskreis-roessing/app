import Foundation

/// Die Endpunkte der Verwaltung — Orte und Aufgaben pflegen, Hitzefaktor
/// stellen — samt ihrer Eingabe-Datensätze.
///
/// Eine eigene Datei, weil an `DorfApi.swift` gerade mehrere Bereiche
/// gleichzeitig arbeiten; die Eingaben stehen aus demselben Grund hier und
/// nicht in `Modelle.swift`. Die Feldnamen bleiben englisch — sie sind der
/// JSON-Vertrag des Backends.
///
/// **Das Backend entscheidet.** Geprüft wird dort (`adminOnly`,
/// `PlaceInput.Validate`, `TaskInput.Validate`); hier wird nur geschickt und
/// der Ablehnungsgrund im Wortlaut weitergereicht. Ein 403 ohne die Rolle
/// `admin` ist keine Panne, sondern die Regel.

// MARK: - Eingabe „Ort"

/// Eingabe von `POST /api/v1/places` und `PUT /api/v1/places/{id}`.
nonisolated struct OrtEingabe: Codable, Hashable, Sendable {
    var name: String
    var description: String = ""
    /// `blumenkasten` · `beet` · `sonstiges`
    var kind: String = OrtEingabe.blumenkasten
    var lat: Double
    var lon: Double
    var active: Bool = true

    static let blumenkasten = "blumenkasten"
    static let beet = "beet"
    static let sonstiges = "sonstiges"

    /// Die Arten in der Reihenfolge, in der die Oberfläche sie anbietet.
    static let arten = [blumenkasten, beet, sonstiges]

    static func bezeichnung(art: String) -> String {
        switch art {
        case blumenkasten: return "Blumenkasten"
        case beet: return "Beet"
        default: return "Sonstiges"
        }
    }
}

// MARK: - Eingabe „Aufgabe"

/// Eingabe von `POST /api/v1/places/{id}/tasks` und `PUT /api/v1/tasks/{id}`.
///
/// Eine Aufgabe ist **entweder** regelmäßig (Intervall und Rot-Schwelle)
/// **oder** einmalig (`oneOff` mit `dueDate`). Beides zusammen weist das
/// Backend ab („dueDate gibt es nur bei einmaligen Aufgaben"), und zwar zu
/// Recht: Sonst wäre nie klar, woraus sich die Ampel ergibt.
///
/// Deshalb wird der Fall gar nicht erst gebaut. Die beiden Bauwege
/// `regelmaessig(…)` und `einmalig(…)` schließen sich aus, und beim Kodieren
/// geht nur mit, was zur gewählten Art gehört: einmalig **ohne** Intervall,
/// regelmäßig **ohne** `dueDate`.
nonisolated struct AufgabeEingabe: Encodable, Hashable, Sendable {
    /// `giessen` · `jaeten` · `sonstiges`
    var kind: String
    var title: String = ""
    /// Nur beim Gießen; bei jeder anderen Art geht das Feld nicht mit.
    var liters: Double?
    var intervalDays: Double = 0
    var redAfterDays: Double = 0
    var oneOff: Bool = false
    /// Datum („2026-08-20") oder RFC3339 — nur bei einmaligen Aufgaben.
    var dueDate: String = ""
    var removeWhenDone: Bool = false
    var active: Bool = true

    static let giessen = "giessen"
    static let jaeten = "jaeten"
    static let sonstiges = "sonstiges"

    static let arten = [giessen, jaeten, sonstiges]

    static func bezeichnung(art: String) -> String {
        switch art {
        case giessen: return "Gießen"
        case jaeten: return "Jäten"
        default: return "Sonstiges"
        }
    }

    /// Liter gibt es nur beim Gießen — bei „Jäten, 10 Liter" wüsste niemand,
    /// was das heißen soll.
    static func literErlaubt(art: String) -> Bool { art == giessen }

    /// Eine regelmäßige Aufgabe: Intervall (→ gelb) und Rot-Schwelle.
    static func regelmaessig(
        kind: String,
        title: String = "",
        liters: Double? = nil,
        intervalDays: Double,
        redAfterDays: Double,
        removeWhenDone: Bool = false,
        active: Bool = true
    ) -> AufgabeEingabe {
        AufgabeEingabe(
            kind: kind, title: title, liters: liters,
            intervalDays: intervalDays, redAfterDays: redAfterDays,
            oneOff: false, dueDate: "",
            removeWhenDone: removeWhenDone, active: active
        )
    }

    /// Eine einmalige Aufgabe: ein Termin statt eines Intervalls.
    static func einmalig(
        kind: String,
        title: String = "",
        liters: Double? = nil,
        dueDate: String,
        removeWhenDone: Bool = false,
        active: Bool = true
    ) -> AufgabeEingabe {
        AufgabeEingabe(
            kind: kind, title: title, liters: liters,
            intervalDays: 0, redAfterDays: 0,
            oneOff: true, dueDate: dueDate,
            removeWhenDone: removeWhenDone, active: active
        )
    }

    enum CodingKeys: String, CodingKey {
        case kind, title, liters, intervalDays, redAfterDays, oneOff, dueDate
        case removeWhenDone, active
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(kind, forKey: .kind)
        try c.encode(title, forKey: .title)
        // Kein `liters` bei Jäten — und keine 0, die das Backend als
        // „liters muss eine Zahl > 0 sein" abwiese.
        if AufgabeEingabe.literErlaubt(art: kind), let menge = liters, menge > 0 {
            try c.encode(menge, forKey: .liters)
        }
        try c.encode(oneOff, forKey: .oneOff)
        if oneOff {
            try c.encode(dueDate, forKey: .dueDate)
        } else {
            try c.encode(intervalDays, forKey: .intervalDays)
            try c.encode(redAfterDays, forKey: .redAfterDays)
        }
        try c.encode(removeWhenDone, forKey: .removeWhenDone)
        try c.encode(active, forKey: .active)
    }
}

// MARK: - Einstellungen

/// Antwort von `GET/PUT /api/v1/settings`.
///
/// Das Backend schickt dort auch die Vergabe-Regeln mit; die gehören einem
/// anderen Bereich und werden hier bewusst nicht gelesen — und beim Schreiben
/// nicht mitgeschickt, damit ein Zug am Hitzefaktor sie nicht überschreibt.
nonisolated struct Einstellungen: Decodable, Hashable, Sendable {
    /// Hitzefaktor: skaliert **nur** die Gieß-Schwellen. 1 = normal,
    /// 0,5 = Hitzewelle (doppelt so schnell fällig).
    var wateringFactor: Double = 1

    enum CodingKeys: String, CodingKey { case wateringFactor }

    init(wateringFactor: Double = 1) { self.wateringFactor = wateringFactor }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        wateringFactor = c.wert(.wateringFactor, 1)
    }
}

/// Eingabe von `PUT /api/v1/settings` — nur der Hitzefaktor.
nonisolated struct HitzefaktorEingabe: Encodable, Hashable, Sendable {
    var wateringFactor: Double
}

// MARK: - Endpunkte

nonisolated extension DorfApi {
    /// Der Schreibzugang der Verwaltung.
    ///
    /// Warum ein eigener Typ und nicht einfach Methoden auf `DorfApi`:
    /// Adresse, Sitzung und Tokengeber liegen dort `private`, und ein Anhang
    /// in einer anderen Datei kommt an sie nicht heran. `DorfApi.swift`
    /// aufzubohren ist gerade keine Option — an der Datei arbeiten mehrere
    /// Bereiche gleichzeitig. Also wird der Zugang aus denselben Teilen
    /// gebaut (`Konfiguration.apiBasis`, `URLSession.dorfSitzung`, derselbe
    /// Tokengeber) und übersetzt Fehler in dieselben `DorfFehler`. Sobald
    /// `DorfApi` seine Helfer sichtbar macht, schrumpft das hier auf die
    /// nackten Aufrufe zusammen (siehe `ios/OFFEN-verwaltung.md`).
    struct Verwaltung: Sendable {
        let basis: URL
        let sitzung: URLSession
        let tokenGeber: @Sendable () async -> String?

        // MARK: Orte

        func ortAnlegen(_ eingabe: OrtEingabe) async throws -> Ort {
            try await schicke("POST", "api/v1/places", rumpf: eingabe)
        }

        func ortAendern(id: Int64, _ eingabe: OrtEingabe) async throws -> Ort {
            try await schicke("PUT", "api/v1/places/\(id)", rumpf: eingabe)
        }

        func ortLoeschen(id: Int64) async throws {
            try await schickeOhneAntwort("DELETE", "api/v1/places/\(id)")
        }

        // MARK: Aufgaben

        func aufgabeAnlegen(ort: Int64, _ eingabe: AufgabeEingabe) async throws -> Aufgabe {
            try await schicke("POST", "api/v1/places/\(ort)/tasks", rumpf: eingabe)
        }

        func aufgabeAendern(id: Int64, _ eingabe: AufgabeEingabe) async throws -> Aufgabe {
            try await schicke("PUT", "api/v1/tasks/\(id)", rumpf: eingabe)
        }

        func aufgabeLoeschen(id: Int64) async throws {
            try await schickeOhneAntwort("DELETE", "api/v1/tasks/\(id)")
        }

        // MARK: Hitzefaktor

        func einstellungen() async throws -> Einstellungen {
            var anfrage = URLRequest(url: basis.appending(path: "api/v1/settings"))
            anfrage.httpMethod = "GET"
            return try await ausfuehren(anfrage)
        }

        /// Das Backend antwortet mit dem neuen Stand — der wird angezeigt,
        /// nicht der Wert, den wir gerade geschickt haben.
        func hitzefaktorSetzen(_ faktor: Double) async throws -> Einstellungen {
            try await schicke("PUT", "api/v1/settings",
                              rumpf: HitzefaktorEingabe(wateringFactor: faktor))
        }

        // MARK: Innereien

        private func schicke<Rumpf: Encodable, T: Decodable>(
            _ methode: String, _ pfad: String, rumpf: Rumpf
        ) async throws -> T {
            var anfrage = URLRequest(url: basis.appending(path: pfad))
            anfrage.httpMethod = methode
            anfrage.setValue("application/json", forHTTPHeaderField: "Content-Type")
            anfrage.httpBody = try JSONEncoder().encode(rumpf)
            return try await ausfuehren(anfrage)
        }

        private func schickeOhneAntwort(_ methode: String, _ pfad: String) async throws {
            var anfrage = URLRequest(url: basis.appending(path: pfad))
            anfrage.httpMethod = methode
            _ = try await rohAusfuehren(anfrage)
        }

        private func ausfuehren<T: Decodable>(_ anfrage: URLRequest) async throws -> T {
            let daten = try await rohAusfuehren(anfrage)
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
                throw Verwaltung.fehler(status: http.statusCode, daten: daten)
            }
            return daten
        }

        /// Die Übersetzung der Statuscodes — als reine Funktion, damit sie
        /// sich ohne Netz prüfen lässt.
        ///
        /// Der Wortlaut kommt aus dem Rumpf des Backends: „admin-Rolle
        /// erforderlich" bei 403, der Grund der Prüfung bei 400. Die App
        /// erfindet keine eigene Begründung; nur wenn das Backend gar nichts
        /// sagt, steht hier ein Ersatzsatz.
        static func fehler(status: Int, daten: Data) -> DorfFehler {
            let antwort = try? JSONDecoder().decode(ApiFehlerAntwort.self, from: daten)
            let grund = antwort?.error.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            switch status {
            case 400:
                return .abgelehnt(grund: grund.isEmpty ? "Die Eingabe wurde abgelehnt." : grund)
            case 401:
                return .nichtAngemeldet
            case 403:
                return .keineBerechtigung(grund: grund.isEmpty
                    ? "Dafür fehlt die Berechtigung." : grund)
            case 404:
                return .nichtGefunden
            case 409:
                return .schonVergeben(grund: grund.isEmpty
                    ? "Das hat gerade jemand anderes geändert." : grund)
            case 429:
                return .zuVieleAnfragen
            default:
                return .serverfehler(status: status)
            }
        }
    }

    /// Baut den Verwaltungszugang — mit demselben Tokengeber, mit dem auch
    /// `DorfApi` gebaut wird.
    static func verwaltung(
        tokenGeber: @escaping @Sendable () async -> String?,
        basis: URL = Konfiguration.apiBasis,
        sitzung: URLSession = .dorfSitzung
    ) -> Verwaltung {
        Verwaltung(basis: basis, sitzung: sitzung, tokenGeber: tokenGeber)
    }
}

// MARK: - Verdrahtung

extension AppUmgebung {
    /// Der Schreibzugang der Verwaltung — mit demselben Token wie `api`.
    ///
    /// Als berechnete Eigenschaft in dieser Datei, damit `Umgebung.swift`
    /// unangetastet bleibt. Der Zugang hält keinen Zustand: Er ist eine
    /// Handvoll Felder um `URLSession.dorfSitzung`, die ohnehin geteilt wird.
    var verwaltung: DorfApi.Verwaltung {
        DorfApi.verwaltung(tokenGeber: { [anmeldung] in await anmeldung.frischesToken() })
    }
}
