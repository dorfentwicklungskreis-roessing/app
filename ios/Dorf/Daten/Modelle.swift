import Foundation

/// Bequemer Zugriff mit Vorgabe: Ein fehlendes oder `null`-Feld ergibt den
/// Vorgabewert, statt das ganze Objekt scheitern zu lassen.
///
/// Das ist Absicht und entspricht dem Android-Client (`ignoreUnknownKeys`,
/// `coerceInputValues`): Das Backend darf Felder ergänzen, ohne dass ältere
/// App-Versionen aufhören zu funktionieren.
nonisolated extension KeyedDecodingContainer {
    func wert<T: Decodable>(_ schluessel: Key, _ vorgabe: T) -> T {
        (try? decodeIfPresent(T.self, forKey: schluessel)) .flatMap { $0 } ?? vorgabe
    }

    func wertOptional<T: Decodable>(_ schluessel: Key) -> T? {
        (try? decodeIfPresent(T.self, forKey: schluessel)) ?? nil
    }
}

// MARK: - Ampel

/// Ampel-Status — die Werte kommen unverändert vom Backend.
nonisolated enum Ampel: String, Codable, Sendable {
    case green, yellow, red

    init(roh: String) { self = Ampel(rawValue: roh) ?? .green }
}

// MARK: - Erledigungen

nonisolated struct Erledigung: Codable, Identifiable, Hashable, Sendable {
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

nonisolated struct Aufgabe: Codable, Identifiable, Hashable, Sendable {
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

// MARK: - Vergabe

/// Ein laufender Vergabe-Vorgang zu genau einer Aufgabe. Die Regeln stehen im
/// Backend (`internal/vergabe`); hier interessiert nur, was anzuzeigen ist.
nonisolated struct Vorgang: Codable, Identifiable, Hashable, Sendable {
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

/// Ergänzungen zum Vorgang — gelesen, nicht gerechnet.
nonisolated extension Vorgang {
    /// Bis wann die Zusage hält. Die Dauer setzt das Backend (Vorgabe 24 h).
    var zusageFrist: Date? { claimedUntil.flatMap(RFC3339.datum) }

    /// Die Liste war einmal durch: Jetzt wird offen gesucht.
    var rundruf: Bool { state == "rundruf" }
}

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

// MARK: - Orte

nonisolated struct Ort: Codable, Identifiable, Hashable, Sendable {
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

nonisolated struct OrteAntwort: Codable, Sendable {
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

nonisolated struct ErledigungenAntwort: Codable, Sendable {
    var completions: [Erledigung] = []
    enum CodingKeys: String, CodingKey { case completions }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        completions = c.wert(.completions, [])
    }
}

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

// MARK: - Ich und Profil

nonisolated struct Ich: Codable, Sendable {
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
nonisolated struct Sichtbarkeit: Codable, Hashable, Sendable {
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

nonisolated struct Profil: Codable, Hashable, Sendable {
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
nonisolated struct ProfilEingabe: Codable, Sendable {
    var displayName: String = ""
    var nickname: String = ""
    var phone: String = ""
    var email: String = ""
    var note: String = ""
    var visibility: Sichtbarkeit = Sichtbarkeit()
}

/// Eine Person in der Dorfbewohner-Liste — mit genau den Feldern, die sie
/// freigegeben hat. Nicht freigegebene Felder kommen gar nicht erst mit.
nonisolated struct Dorfbewohner: Codable, Identifiable, Hashable, Sendable {
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

nonisolated struct DorfbewohnerAntwort: Codable, Sendable {
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

nonisolated struct Auszeichnung: Codable, Identifiable, Hashable, Sendable {
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
nonisolated struct Ranglistenzeile: Codable, Identifiable, Hashable, Sendable {
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

nonisolated struct Gesamtsummen: Codable, Hashable, Sendable {
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

nonisolated struct Rangliste: Codable, Sendable {
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
nonisolated enum Zeitraum: String, CaseIterable, Identifiable, Sendable {
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

nonisolated struct ErledigungEingabe: Codable, Sendable {
    var liters: Double?
    var note: String = ""
}

/// Eingabe von `POST /api/v1/ideen`.
nonisolated struct IdeeEingabe: Codable, Sendable {
    var wunsch: String
    var name: String = ""
    var email: String = ""
}

nonisolated struct Idee: Codable, Sendable {
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
nonisolated struct ApiFehlerAntwort: Codable, Sendable {
    var error: String = ""
    var retryAfter: String?

    enum CodingKeys: String, CodingKey { case error, retryAfter }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        error = c.wert(.error, "")
        retryAfter = c.wertOptional(.retryAfter)
    }
}

// MARK: - Einstellungen des Dorfes

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

// MARK: - Konto

/// Die Antwort von `DELETE /api/v1/me`.
///
/// Sie enthält bewusst zwei Sätze im Klartext, die die App **anzeigt statt
/// nachzubauen**: was mit den Erledigungen passiert und dass die Rössing-ID
/// bestehen bleibt. Was das Backend tut, sagt das Backend — sonst laufen
/// beide Fassungen über die Jahre auseinander.
nonisolated struct Kontoloeschung: Decodable, Sendable {
    var geloescht: Bool = false
    /// „Deine Meldungen bleiben anonym stehen …"
    var erledigungen: String = ""
    /// „Deine Rössing-ID bleibt bestehen …"
    var roessingId: String = ""
    var roessingIdUrl: String = ""

    enum CodingKeys: String, CodingKey { case geloescht, erledigungen, roessingId, roessingIdUrl }

    init(geloescht: Bool = false, erledigungen: String = "",
         roessingId: String = "", roessingIdUrl: String = "") {
        self.geloescht = geloescht; self.erledigungen = erledigungen
        self.roessingId = roessingId; self.roessingIdUrl = roessingIdUrl
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        geloescht = c.wert(.geloescht, false)
        erledigungen = c.wert(.erledigungen, "")
        roessingId = c.wert(.roessingId, "")
        roessingIdUrl = c.wert(.roessingIdUrl, "")
    }
}

// MARK: - Gerät

/// Eingabe von `POST` und `DELETE /api/v1/me/devices` — der Rumpf ist für
/// beide derselbe: `{"token": "<hex>", "platform": "ios"}`.
///
/// `platform` ist das Feld, an dem das Backend den Versandweg festmacht:
/// „ios" spricht direkt mit Apple (APNs), alles andere geht über Firebase
/// (`backend/internal/push/weiche.go`).
nonisolated struct GeraetEingabe: Encodable, Hashable, Sendable {
    /// Die Plattform, für die diese App Kennungen meldet.
    static let plattform = "ios"

    var token: String
    var platform: String = GeraetEingabe.plattform

    init(kennung: String) {
        self.token = kennung
        self.platform = GeraetEingabe.plattform
    }
}

// MARK: - Zeit

/// RFC3339 lesen. Das Backend schickt mal mit, mal ohne Sekundenbruchteile —
/// beides muss gehen, sonst fehlt in der Historie plötzlich ein Datum.
nonisolated enum RFC3339 {
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
