import Foundation

/// Bequemer Zugriff mit Vorgabe: Ein fehlendes oder `null`-Feld ergibt den
/// Vorgabewert, statt das ganze Objekt scheitern zu lassen.
///
/// Das ist Absicht und entspricht dem Android-Client (`ignoreUnknownKeys`,
/// `coerceInputValues`): Das Backend darf Felder ergänzen, ohne dass ältere
/// App-Versionen aufhören zu funktionieren.
extension KeyedDecodingContainer {
    func wert<T: Decodable>(_ schluessel: Key, _ vorgabe: T) -> T {
        (try? decodeIfPresent(T.self, forKey: schluessel)) .flatMap { $0 } ?? vorgabe
    }

    func wertOptional<T: Decodable>(_ schluessel: Key) -> T? {
        (try? decodeIfPresent(T.self, forKey: schluessel)) ?? nil
    }
}

// MARK: - Ampel

/// Ampel-Status — die Werte kommen unverändert vom Backend.
enum Ampel: String, Codable, Sendable {
    case green, yellow, red

    init(roh: String) { self = Ampel(rawValue: roh) ?? .green }
}

// MARK: - Erledigungen

struct Erledigung: Codable, Identifiable, Hashable, Sendable {
    var id: Int64 = 0
    var taskId: Int64 = 0
    var userSub: String = ""
    var userName: String = ""
    var liters: Double?
    var note: String = ""
    var doneAt: String = ""

    enum CodingKeys: String, CodingKey { case id, taskId, userSub, userName, liters, note, doneAt }

    init(id: Int64 = 0, taskId: Int64 = 0, userSub: String = "", userName: String = "",
         liters: Double? = nil, note: String = "", doneAt: String = "") {
        self.id = id; self.taskId = taskId; self.userSub = userSub
        self.userName = userName; self.liters = liters; self.note = note; self.doneAt = doneAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, 0)
        taskId = c.wert(.taskId, 0)
        userSub = c.wert(.userSub, "")
        userName = c.wert(.userName, "")
        liters = c.wertOptional(.liters)
        note = c.wert(.note, "")
        doneAt = c.wert(.doneAt, "")
    }

    var zeitpunkt: Date? { RFC3339.datum(doneAt) }
}

// MARK: - Aufgabe

struct Aufgabe: Codable, Identifiable, Hashable, Sendable {
    var id: Int64
    var placeId: Int64 = 0
    var kind: String = "giessen"
    var title: String = ""
    var liters: Double?
    var intervalDays: Double = 7
    var redAfterDays: Double = 14
    /// Einmalige Aufgabe („einmal zum Bahnhof fahren"): An die Stelle des
    /// Intervalls tritt das Fälligkeitsdatum.
    var oneOff: Bool = false
    var dueDate: String?
    /// Nach dem Erledigen von Karte und Liste nehmen.
    var removeWhenDone: Bool = false
    var active: Bool = true
    var status: String = "green"
    var lastCompletion: Erledigung?
    var dueAt: String = ""
    var redAt: String = ""
    /// Spielschutz: bis dahin darf nicht erneut gemeldet werden.
    var lockedUntil: String?
    var assignment: Vorgang?
    var signupCount: Int = 0
    var signedUp: Bool = false

    enum CodingKeys: String, CodingKey {
        case id, placeId, kind, title, liters, intervalDays, redAfterDays, oneOff, dueDate
        case removeWhenDone, active, status, lastCompletion, dueAt, redAt, lockedUntil
        case assignment, signupCount, signedUp
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int64.self, forKey: .id)
        placeId = c.wert(.placeId, 0)
        kind = c.wert(.kind, "giessen")
        title = c.wert(.title, "")
        liters = c.wertOptional(.liters)
        intervalDays = c.wert(.intervalDays, 7)
        redAfterDays = c.wert(.redAfterDays, 14)
        oneOff = c.wert(.oneOff, false)
        dueDate = c.wertOptional(.dueDate)
        removeWhenDone = c.wert(.removeWhenDone, false)
        active = c.wert(.active, true)
        status = c.wert(.status, "green")
        lastCompletion = c.wertOptional(.lastCompletion)
        dueAt = c.wert(.dueAt, "")
        redAt = c.wert(.redAt, "")
        lockedUntil = c.wertOptional(.lockedUntil)
        assignment = c.wertOptional(.assignment)
        signupCount = c.wert(.signupCount, 0)
        signedUp = c.wert(.signedUp, false)
    }

    init(id: Int64, placeId: Int64 = 0, kind: String = "giessen", title: String = "",
         liters: Double? = nil, intervalDays: Double = 7, redAfterDays: Double = 14,
         oneOff: Bool = false, dueDate: String? = nil, removeWhenDone: Bool = false,
         active: Bool = true, status: String = "green", lastCompletion: Erledigung? = nil,
         dueAt: String = "", redAt: String = "", lockedUntil: String? = nil,
         assignment: Vorgang? = nil, signupCount: Int = 0, signedUp: Bool = false) {
        self.id = id; self.placeId = placeId; self.kind = kind; self.title = title
        self.liters = liters; self.intervalDays = intervalDays; self.redAfterDays = redAfterDays
        self.oneOff = oneOff; self.dueDate = dueDate; self.removeWhenDone = removeWhenDone
        self.active = active; self.status = status; self.lastCompletion = lastCompletion
        self.dueAt = dueAt; self.redAt = redAt; self.lockedUntil = lockedUntil
        self.assignment = assignment; self.signupCount = signupCount; self.signedUp = signedUp
    }

    var ampel: Ampel { Ampel(roh: status) }
    var gesperrtBis: Date? { lockedUntil.flatMap(RFC3339.datum) }
    var faelligAm: Date? { dueDate.flatMap(RFC3339.datum) }

    /// Eine einmalige Aufgabe, die schon erledigt ist. Sie wird nicht wieder
    /// fällig, und das Backend weist eine zweite Meldung mit 409 ab — der
    /// Knopf gehört also weg, nicht bloß gesperrt.
    var erledigtUndVorbei: Bool { oneOff && lastCompletion != nil }

    /// Ob gerade gemeldet werden darf (Spielschutz beachtet).
    func meldenMoeglich(jetzt: Date = Date()) -> Bool {
        if erledigtUndVorbei { return false }
        guard let bis = gesperrtBis else { return true }
        return jetzt >= bis
    }

    var anzeigename: String {
        if !title.isEmpty { return title }
        switch kind {
        case "giessen": return "Gießen"
        case "jaeten": return "Jäten"
        default: return "Pflege"
        }
    }
}

// MARK: - Vergabe

/// Ein laufender Vergabe-Vorgang zu genau einer Aufgabe. Die Regeln stehen im
/// Backend (`internal/vergabe`); hier interessiert nur, was anzuzeigen ist.
struct Vorgang: Codable, Identifiable, Hashable, Sendable {
    var id: Int64 = 0
    var taskId: Int64 = 0
    /// offen · uebernommen · rundruf · beendet
    var state: String = "offen"
    var claimedBy: String = ""
    var claimedByName: String = ""
    var claimedUntil: String?
    var nextOfferAt: String?
    var askedCount: Int = 0

    enum CodingKeys: String, CodingKey {
        case id, taskId, state, claimedBy, claimedByName, claimedUntil, nextOfferAt, askedCount
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, 0)
        taskId = c.wert(.taskId, 0)
        state = c.wert(.state, "offen")
        claimedBy = c.wert(.claimedBy, "")
        claimedByName = c.wert(.claimedByName, "")
        claimedUntil = c.wertOptional(.claimedUntil)
        nextOfferAt = c.wertOptional(.nextOfferAt)
        askedCount = c.wert(.askedCount, 0)
    }

    var uebernommen: Bool { !claimedBy.isEmpty }
    func vonMir(_ meinSub: String?) -> Bool { !claimedBy.isEmpty && claimedBy == meinSub }
}

// MARK: - Orte

struct Ort: Codable, Identifiable, Hashable, Sendable {
    var id: Int64
    var name: String
    var description: String = ""
    var kind: String = "blumenkasten"
    var lat: Double
    var lon: Double
    var active: Bool = true
    var status: String = "green"
    var tasks: [Aufgabe] = []

    enum CodingKeys: String, CodingKey {
        case id, name, description, kind, lat, lon, active, status, tasks
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int64.self, forKey: .id)
        name = c.wert(.name, "")
        description = c.wert(.description, "")
        kind = c.wert(.kind, "blumenkasten")
        lat = c.wert(.lat, 0)
        lon = c.wert(.lon, 0)
        active = c.wert(.active, true)
        status = c.wert(.status, "green")
        tasks = c.wert(.tasks, [])
    }

    init(id: Int64, name: String, description: String = "", kind: String = "blumenkasten",
         lat: Double, lon: Double, active: Bool = true, status: String = "green",
         tasks: [Aufgabe] = []) {
        self.id = id; self.name = name; self.description = description; self.kind = kind
        self.lat = lat; self.lon = lon; self.active = active; self.status = status
        self.tasks = tasks
    }

    var ampel: Ampel { Ampel(roh: status) }
}

struct OrteAntwort: Codable, Sendable {
    var places: [Ort] = []
    var wateringFactor: Double = 1

    enum CodingKeys: String, CodingKey { case places, wateringFactor }

    init(places: [Ort] = [], wateringFactor: Double = 1) {
        self.places = places; self.wateringFactor = wateringFactor
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        places = c.wert(.places, [])
        wateringFactor = c.wert(.wateringFactor, 1)
    }
}

struct ErledigungenAntwort: Codable, Sendable {
    var completions: [Erledigung] = []
    enum CodingKeys: String, CodingKey { case completions }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        completions = c.wert(.completions, [])
    }
}

// MARK: - Ich und Profil

struct Ich: Codable, Sendable {
    var sub: String = ""
    var name: String = ""
    var email: String = ""
    var roles: [String] = []
    var isAdmin: Bool = false
    var profile: Profil?

    enum CodingKeys: String, CodingKey { case sub, name, email, roles, isAdmin, profile }

    init(sub: String = "", name: String = "", email: String = "", roles: [String] = [],
         isAdmin: Bool = false, profile: Profil? = nil) {
        self.sub = sub; self.name = name; self.email = email
        self.roles = roles; self.isAdmin = isAdmin; self.profile = profile
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sub = c.wert(.sub, "")
        name = c.wert(.name, "")
        email = c.wert(.email, "")
        roles = c.wert(.roles, [])
        isAdmin = c.wert(.isAdmin, false)
        profile = c.wertOptional(.profile)
    }
}

/// Sichtbarkeit je Profilfeld. Werte des Backends: `dorf` (alle angemeldeten
/// Dorfbewohner) oder `verwaltung` (nur Verwaltende).
///
/// Die Vorbelegung entspricht der des Backends: Kontaktdaten bleiben bei der
/// Verwaltung, bis jemand sie bewusst freigibt. Ein Wert, den diese
/// App-Version nicht kennt, gilt vorsichtshalber als nicht öffentlich.
struct Sichtbarkeit: Codable, Hashable, Sendable {
    static let dorf = "dorf"
    static let verwaltung = "verwaltung"

    var displayName: String = Sichtbarkeit.dorf
    var nickname: String = Sichtbarkeit.dorf
    var phone: String = Sichtbarkeit.verwaltung
    var email: String = Sichtbarkeit.verwaltung
    var note: String = Sichtbarkeit.verwaltung

    enum CodingKeys: String, CodingKey { case displayName, nickname, phone, email, note }

    init(displayName: String = Sichtbarkeit.dorf, nickname: String = Sichtbarkeit.dorf,
         phone: String = Sichtbarkeit.verwaltung, email: String = Sichtbarkeit.verwaltung,
         note: String = Sichtbarkeit.verwaltung) {
        self.displayName = displayName; self.nickname = nickname
        self.phone = phone; self.email = email; self.note = note
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        displayName = c.wert(.displayName, Sichtbarkeit.dorf)
        nickname = c.wert(.nickname, Sichtbarkeit.dorf)
        phone = c.wert(.phone, Sichtbarkeit.verwaltung)
        email = c.wert(.email, Sichtbarkeit.verwaltung)
        note = c.wert(.note, Sichtbarkeit.verwaltung)
    }

    var displayNameOeffentlich: Bool { displayName == Sichtbarkeit.dorf }
    var nicknameOeffentlich: Bool { nickname == Sichtbarkeit.dorf }
    var phoneOeffentlich: Bool { phone == Sichtbarkeit.dorf }
    var emailOeffentlich: Bool { email == Sichtbarkeit.dorf }
    var noteOeffentlich: Bool { note == Sichtbarkeit.dorf }

    static func wert(_ oeffentlich: Bool) -> String { oeffentlich ? dorf : verwaltung }
}

struct Profil: Codable, Hashable, Sendable {
    var userSub: String = ""
    var displayName: String = ""
    var nickname: String = ""
    var phone: String = ""
    var email: String = ""
    var note: String = ""
    var visibility: Sichtbarkeit = Sichtbarkeit()
    var updatedAt: String = ""

    enum CodingKeys: String, CodingKey {
        case userSub, displayName, nickname, phone, email, note, visibility, updatedAt
    }

    init(userSub: String = "", displayName: String = "", nickname: String = "",
         phone: String = "", email: String = "", note: String = "",
         visibility: Sichtbarkeit = Sichtbarkeit(), updatedAt: String = "") {
        self.userSub = userSub; self.displayName = displayName; self.nickname = nickname
        self.phone = phone; self.email = email; self.note = note
        self.visibility = visibility; self.updatedAt = updatedAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        userSub = c.wert(.userSub, "")
        displayName = c.wert(.displayName, "")
        nickname = c.wert(.nickname, "")
        phone = c.wert(.phone, "")
        email = c.wert(.email, "")
        note = c.wert(.note, "")
        visibility = c.wert(.visibility, Sichtbarkeit())
        updatedAt = c.wert(.updatedAt, "")
    }
}

/// Eingabe von `PUT /api/v1/me/profile`.
struct ProfilEingabe: Codable, Sendable {
    var displayName: String = ""
    var nickname: String = ""
    var phone: String = ""
    var email: String = ""
    var note: String = ""
    var visibility: Sichtbarkeit = Sichtbarkeit()
}

/// Eine Person in der Dorfbewohner-Liste — mit genau den Feldern, die sie
/// freigegeben hat. Nicht freigegebene Felder kommen gar nicht erst mit.
struct Dorfbewohner: Codable, Identifiable, Hashable, Sendable {
    var userSub: String = ""
    /// Name in Rangliste und Erledigungen (Nickname, sonst Anzeigename).
    var name: String = ""
    var displayName: String = ""
    var nickname: String = ""
    var phone: String = ""
    var email: String = ""
    var note: String = ""
    /// Felder, die nur Verwaltende sehen, weil die Person sie nicht
    /// freigegeben hat. Für gewöhnliche Mitglieder immer leer.
    var restricted: [String] = []

    var id: String { userSub }

    enum CodingKeys: String, CodingKey {
        case userSub, name, displayName, nickname, phone, email, note, restricted
    }

    init(userSub: String = "", name: String = "", displayName: String = "",
         nickname: String = "", phone: String = "", email: String = "",
         note: String = "", restricted: [String] = []) {
        self.userSub = userSub; self.name = name; self.displayName = displayName
        self.nickname = nickname; self.phone = phone; self.email = email
        self.note = note; self.restricted = restricted
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        userSub = c.wert(.userSub, "")
        name = c.wert(.name, "")
        displayName = c.wert(.displayName, "")
        nickname = c.wert(.nickname, "")
        phone = c.wert(.phone, "")
        email = c.wert(.email, "")
        note = c.wert(.note, "")
        restricted = c.wert(.restricted, [])
    }

    func nurFuerVerwaltung(_ feld: String) -> Bool { restricted.contains(feld) }
}

struct DorfbewohnerAntwort: Codable, Sendable {
    var members: [Dorfbewohner] = []
    /// true, wenn die Liste alles zeigt, weil der Abruf von Verwaltenden kam.
    var adminView: Bool = false

    enum CodingKeys: String, CodingKey { case members, adminView }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        members = c.wert(.members, [])
        adminView = c.wert(.adminView, false)
    }
}

// MARK: - Rangliste

struct Auszeichnung: Codable, Identifiable, Hashable, Sendable {
    var key: String = ""
    var label: String = ""
    var description: String = ""

    var id: String { key }

    enum CodingKeys: String, CodingKey { case key, label, description }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        key = c.wert(.key, "")
        label = c.wert(.label, "")
        description = c.wert(.description, "")
    }

    init(key: String = "", label: String = "", description: String = "") {
        self.key = key; self.label = label; self.description = description
    }
}

/// Eine Zeile der Rangliste. `rank == 0` heißt: im Zeitraum noch nichts gemeldet.
struct Ranglistenzeile: Codable, Identifiable, Hashable, Sendable {
    var rank: Int = 0
    var userSub: String = ""
    var userName: String = ""
    var completions: Int = 0
    var byKind: [String: Int] = [:]
    var liters: Double = 0
    var lastCompletion: String?
    var badges: [Auszeichnung] = []

    var id: String { userSub.isEmpty ? "\(rank)-\(userName)" : userSub }

    enum CodingKeys: String, CodingKey {
        case rank, userSub, userName, completions, byKind, liters, lastCompletion, badges
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        rank = c.wert(.rank, 0)
        userSub = c.wert(.userSub, "")
        userName = c.wert(.userName, "")
        completions = c.wert(.completions, 0)
        byKind = c.wert(.byKind, [:])
        liters = c.wert(.liters, 0)
        lastCompletion = c.wertOptional(.lastCompletion)
        badges = c.wert(.badges, [])
    }

    init(rank: Int = 0, userSub: String = "", userName: String = "", completions: Int = 0,
         byKind: [String: Int] = [:], liters: Double = 0, lastCompletion: String? = nil,
         badges: [Auszeichnung] = []) {
        self.rank = rank; self.userSub = userSub; self.userName = userName
        self.completions = completions; self.byKind = byKind; self.liters = liters
        self.lastCompletion = lastCompletion; self.badges = badges
    }
}

struct Gesamtsummen: Codable, Hashable, Sendable {
    var completions: Int = 0
    var byKind: [String: Int] = [:]
    var liters: Double = 0
    var participants: Int = 0

    enum CodingKeys: String, CodingKey { case completions, byKind, liters, participants }

    init(completions: Int = 0, byKind: [String: Int] = [:], liters: Double = 0, participants: Int = 0) {
        self.completions = completions; self.byKind = byKind
        self.liters = liters; self.participants = participants
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        completions = c.wert(.completions, 0)
        byKind = c.wert(.byKind, [:])
        liters = c.wert(.liters, 0)
        participants = c.wert(.participants, 0)
    }
}

struct Rangliste: Codable, Sendable {
    var period: String = "saison"
    var from: String = ""
    var to: String = ""
    var entries: [Ranglistenzeile] = []
    var totals: Gesamtsummen = Gesamtsummen()
    /// Der eigene Eintrag — auch, wenn er nicht in `entries` steht.
    var me: Ranglistenzeile?

    enum CodingKeys: String, CodingKey { case period, from, to, entries, totals, me }

    init(period: String = "saison", from: String = "", to: String = "",
         entries: [Ranglistenzeile] = [], totals: Gesamtsummen = Gesamtsummen(),
         me: Ranglistenzeile? = nil) {
        self.period = period; self.from = from; self.to = to
        self.entries = entries; self.totals = totals; self.me = me
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        period = c.wert(.period, "saison")
        from = c.wert(.from, "")
        to = c.wert(.to, "")
        entries = c.wert(.entries, [])
        totals = c.wert(.totals, Gesamtsummen())
        me = c.wertOptional(.me)
    }
}

/// Zeitraum der Rangliste. Die Werte gehen als `?period=` an das Backend.
enum Zeitraum: String, CaseIterable, Identifiable, Sendable {
    case woche, monat, saison, jahr, gesamt

    var id: String { rawValue }

    var titel: String {
        switch self {
        case .woche: return "Woche"
        case .monat: return "Monat"
        case .saison: return "Saison"
        case .jahr: return "Jahr"
        case .gesamt: return "Gesamt"
        }
    }
}

// MARK: - Eingaben

struct ErledigungEingabe: Codable, Sendable {
    var liters: Double?
    var note: String = ""
}

/// Eingabe von `POST /api/v1/ideen`.
struct IdeeEingabe: Codable, Sendable {
    var wunsch: String
    var name: String = ""
    var email: String = ""
}

struct Idee: Codable, Sendable {
    var id: Int64 = 0
    var wunsch: String = ""
    var name: String = ""
    var email: String = ""
    /// website · app
    var quelle: String = "app"
    /// neu · gelesen · umgesetzt · abgelehnt
    var status: String = "neu"
    var createdAt: String = ""

    enum CodingKeys: String, CodingKey { case id, wunsch, name, email, quelle, status, createdAt }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, 0)
        wunsch = c.wert(.wunsch, "")
        name = c.wert(.name, "")
        email = c.wert(.email, "")
        quelle = c.wert(.quelle, "app")
        status = c.wert(.status, "neu")
        createdAt = c.wert(.createdAt, "")
    }
}

/// Fehlerantwort des Backends (z.B. bei HTTP 409 mit Sperrfrist).
struct ApiFehlerAntwort: Codable, Sendable {
    var error: String = ""
    var retryAfter: String?

    enum CodingKeys: String, CodingKey { case error, retryAfter }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        error = c.wert(.error, "")
        retryAfter = c.wertOptional(.retryAfter)
    }
}

// MARK: - Zeit

/// RFC3339 lesen. Das Backend schickt mal mit, mal ohne Sekundenbruchteile —
/// beides muss gehen, sonst fehlt in der Historie plötzlich ein Datum.
enum RFC3339 {
    nonisolated(unsafe) private static let mitBruchteilen: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    nonisolated(unsafe) private static let ohneBruchteile: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    static func datum(_ text: String) -> Date? {
        if text.isEmpty { return nil }
        return mitBruchteilen.date(from: text) ?? ohneBruchteile.date(from: text)
    }
}
