import Foundation

/// What kind of malfunction it was. The same four values the backend knows
/// (`model.ErrorReportKind`) — the app does not invent a fifth.
nonisolated enum ErrorReportKind: String, Sendable {
    case crash
    case network
    case server
    case unexpected
}

/// One thing that went wrong, as the person experienced it.
///
/// `message` is the sentence that stood on screen, in plain German. It comes
/// from `DorfFehler.klartext`, and that in turn comes from the backend
/// wherever the backend said something — the app does not make up its own
/// wording, neither for the person nor for the report.
nonisolated struct ErrorIncident: Identifiable, Equatable, Sendable {
    let id: UUID
    let kind: ErrorReportKind
    let message: String
    let detail: String
    let area: String
    let occurredAt: Date

    init(id: UUID = UUID(), kind: ErrorReportKind, message: String,
         detail: String = "", area: String = "", occurredAt: Date = Date()) {
        self.id = id
        self.kind = kind
        self.message = message
        self.detail = detail
        self.area = area
        self.occurredAt = occurredAt
    }
}

extension ErrorIncident {
    /// Builds an incident out of a failed request — or nothing.
    ///
    /// Not every refusal is a malfunction. „Das wurde gerade erst erledigt",
    /// „Dafür fehlt die Berechtigung", „Das hat jemand anderes übernommen":
    /// those are rules doing their job, they are already shown where they
    /// belong, and a report about them would bury the real breakage under
    /// noise. Reported is what nobody wanted: no connection, a server that
    /// answers with an error, an answer nobody can read.
    static func aus(_ fehler: DorfFehler, pfad: String, methode: String = "") -> ErrorIncident? {
        let kind: ErrorReportKind
        var technik = ""
        switch fehler {
        case .netz(let grund):
            kind = .network
            technik = grund
        case .serverfehler(let status):
            kind = .server
            technik = "HTTP \(status)"
        case .nichtGefunden:
            kind = .unexpected
            technik = "HTTP 404"
        case .abgelehnt, .keineBerechtigung, .schonVergeben,
             .gesperrt, .zuVieleAnfragen, .nichtAngemeldet:
            // Eine Regel, die greift, ist kein Fehler.
            return nil
        }
        let anfrage = [methode, pfad].filter { !$0.isEmpty }.joined(separator: " ")
        return ErrorIncident(
            kind: kind,
            message: fehler.klartext,
            detail: [technik, anfrage].filter { !$0.isEmpty }.joined(separator: " · "),
            area: Bereichsnamen.zu(pfad: pfad)
        )
    }
}

/// Translates a request path into the part of the app somebody was actually
/// looking at. „api/v1/places" says nothing to the Dorfentwicklungskreis;
/// „Mithelfen" says where to look.
nonisolated enum Bereichsnamen {
    /// The longest matching prefix wins, so `me/devices` does not get caught
    /// by `me`.
    private static let zuordnung: [(pfad: String, name: String)] = [
        ("api/v1/me/notifications", "Anfragen und Hinweise"),
        ("api/v1/me/devices", "Benachrichtigungen"),
        ("api/v1/me/profile", "Mein Profil"),
        ("api/v1/me", "Konto"),
        ("api/v1/members", "Dorfbewohner"),
        ("api/v1/places", "Mithelfen"),
        ("api/v1/tasks", "Mithelfen"),
        ("api/v1/completions", "Mithelfen"),
        ("api/v1/assignments", "Anfragen und Hinweise"),
        ("api/v1/stats/leaderboard", "Rangliste"),
        ("api/v1/ideen", "Idee vorschlagen"),
        ("api/v1/traeger", "Träger"),
        ("api/v1/settings", "Einstellungen"),
    ]

    static func zu(pfad: String) -> String {
        let sauber = pfad.hasPrefix("/") ? String(pfad.dropFirst()) : pfad
        let treffer = zuordnung
            .filter { sauber.hasPrefix($0.pfad) }
            .max { $0.pfad.count < $1.pfad.count }
        return treffer?.name ?? "App"
    }
}

/// What the app knows about itself: version, system and device type. Read
/// once — none of it changes while the app runs.
nonisolated struct Geraeteangaben: Sendable, Equatable {
    let appVersion: String
    let osVersion: String
    let deviceModel: String

    /// The device type as Apple names it („iPhone14,3"). Deliberately the
    /// model and not `identifierForVendor`: the model helps to reproduce
    /// something, an identifier would only track a person.
    static func aktuell(bundle: Bundle = .main,
                        systemVersion: String,
                        maschine: String = maschinenkennung()) -> Geraeteangaben
    {
        let version = bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        let build = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""
        let beides = build.isEmpty ? version : "\(version) (\(build))"
        return Geraeteangaben(
            appVersion: beides.trimmingCharacters(in: .whitespaces),
            osVersion: systemVersion.isEmpty ? "" : "iOS \(systemVersion)",
            deviceModel: maschine
        )
    }

    static func maschinenkennung() -> String {
        var werte = utsname()
        guard uname(&werte) == 0 else { return "" }
        // Die Kennung steht als C-Zeichenkette in einem Tupel fester Länge.
        // Erst kopieren, dann lesen: Ein Zeiger auf `werte.machine` und
        // `werte` selbst im selben Ausdruck wäre ein überlappender Zugriff.
        let maschine = werte.machine
        return withUnsafeBytes(of: maschine) { rohbytes in
            let bytes = rohbytes.prefix { $0 != 0 }
            return String(decoding: bytes, as: UTF8.self)
        }
    }
}
