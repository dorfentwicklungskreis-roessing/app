import Foundation

/// The contract with the rental platform („Maschinchenring", `mieten.…`) —
/// routes and payloads, and nothing else.
///
/// **The single source for this file is `docs/mieten-api.md`.** Every type
/// below mirrors one of its nineteen routes; every field name is spelled the
/// way the platform spells it. Nothing here is invented: where the document
/// is silent, this file is silent too, and the answer is fetched from the
/// platform rather than guessed (`docs/mieten-api.md`, section 6).
///
/// Two habits keep this survivable, both taken from the events area:
///  - every field falls back to a default, so the platform may add fields
///    without older app versions going blank;
///  - the app reads decisions, it does not compute them. `available`,
///    `canCancel`, `canDecide`, `lenderStatus` and every message come from
///    the server. The app may grey a button out because the server said so;
///    it must not know the condition (`docs/mieten-api.md`, „Was die App
///    nicht entscheidet").

// MARK: - Routes

/// Where the things live. Paths only — the host comes from
/// `Konfiguration.rentalBaseUrl`, so CI and tests can point elsewhere.
nonisolated enum RentalRoutes {
    /// Public: the catalogue (1).
    static let items = "api/v1/items"
    /// Public: one device with its pictures (2).
    static func item(_ id: String) -> String { "api/v1/items/\(id)" }
    /// Public: hybrid search, ranked by the platform (3).
    static let search = "api/v1/search"
    /// Public: the sets (4).
    static let sets = "api/v1/sets"
    /// Public: „is it free from … to …?" — the platform's answer, not ours (5).
    static let availability = "api/v1/availability"
    /// Public: occupied periods, without any personal data (6).
    static let occupancy = "api/v1/occupancy"

    /// Needs a token: own profile (7, 8).
    static let me = "api/v1/me"
    /// Needs a token: ask to become a lender (9).
    static let lenderRequest = "api/v1/me/lender-request"
    /// Needs a token: my own bookings (10).
    static let myBookings = "api/v1/bookings/mine"
    /// Needs a token: book a device or a set (11).
    static let bookings = "api/v1/bookings"
    /// Needs a token: withdraw a booking (12).
    static func cancel(_ id: String) -> String { "api/v1/bookings/\(id)/cancel" }

    /// Needs a token: bookings on my own devices (13).
    static let ownerBookings = "api/v1/owner/bookings"
    /// Needs a token: confirm (14) and turn down (15) a booking.
    static func approve(_ id: String) -> String { "api/v1/bookings/\(id)/approve" }
    static func reject(_ id: String) -> String { "api/v1/bookings/\(id)/reject" }
    /// Needs a token: my own devices, disabled ones included (16).
    static let ownerItems = "api/v1/owner/items"
    /// Needs a token: my own blocked periods (17, 18).
    static let ownerBlocks = "api/v1/owner/blocks"
    /// Needs a token: lift one of them (19).
    static func ownerBlock(_ id: String) -> String { "api/v1/owner/blocks/\(id)" }

    // Query parameters, spelled once.
    static let queryText = "q"
    static let queryTags = "tags"
    static let queryLimit = "limit"
    static let queryDeviceId = "deviceId"
    static let querySetId = "setId"
    static let queryStartDate = "startDate"
    static let queryEndDate = "endDate"
}

// MARK: - Catalogue

/// One picture of a device (route 2).
nonisolated struct RentalImageDto: Codable, Hashable, Sendable {
    var id: String = ""
    var url: String = ""
    var isThumbnail: Bool = false

    enum CodingKeys: String, CodingKey { case id, url, isThumbnail }

    init(id: String = "", url: String = "", isThumbnail: Bool = false) {
        self.id = id; self.url = url; self.isThumbnail = isThumbnail
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        url = c.wert(.url, "")
        isThumbnail = c.wert(.isThumbnail, false)
    }
}

/// One device, as routes 1, 2, 3 and 16 all spell it.
///
/// The fields that only some of those routes carry are optional and default
/// to nothing: `images` (2), `score` (3), `active` (16). One type for four
/// routes is honest here — the platform really does send the same object.
///
/// **There is no owner in this payload and there will not be one.** Who lends
/// a device out appears in no public answer; that is a rule of the platform,
/// not an omission (`docs/mieten-api.md`, route 1).
nonisolated struct RentalItemDto: Codable, Hashable, Sendable {
    var id: String = ""
    var name: String = ""
    /// Markdown. May be `null`.
    var description: String?
    /// Euro, as a plain number. `null` means: this tariff does not exist —
    /// **not** zero, and nothing to calculate with.
    var pricePerDay: Double?
    var pricePerWeekend: Double?
    var pricePerWeek: Double?
    var deposit: Double?
    var tags: [String] = []
    /// A finished, signed address (imgproxy, 600×450). `null` means no picture.
    var thumbnailUrl: String?
    /// The manufacturer's page, if there is one.
    var productUrl: String?
    /// The same device in the web version.
    var webUrl: String?
    /// Route 2 only.
    var images: [RentalImageDto] = []
    /// Route 3 only: how well it matched. For sorting, never for showing.
    var score: Double?
    /// Route 16 only: `false` means the device is invisible to everybody else.
    var active: Bool?

    enum CodingKeys: String, CodingKey {
        case id, name, description
        case pricePerDay, pricePerWeekend, pricePerWeek, deposit
        case tags, thumbnailUrl, productUrl, webUrl, images, score, active
    }

    init(id: String = "", name: String = "", description: String? = nil,
         pricePerDay: Double? = nil, pricePerWeekend: Double? = nil,
         pricePerWeek: Double? = nil, deposit: Double? = nil, tags: [String] = [],
         thumbnailUrl: String? = nil, productUrl: String? = nil, webUrl: String? = nil,
         images: [RentalImageDto] = [], score: Double? = nil, active: Bool? = nil) {
        self.id = id; self.name = name; self.description = description
        self.pricePerDay = pricePerDay; self.pricePerWeekend = pricePerWeekend
        self.pricePerWeek = pricePerWeek; self.deposit = deposit; self.tags = tags
        self.thumbnailUrl = thumbnailUrl; self.productUrl = productUrl; self.webUrl = webUrl
        self.images = images; self.score = score; self.active = active
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        name = c.wert(.name, "")
        description = c.wertOptional(.description)
        pricePerDay = c.wertOptional(.pricePerDay)
        pricePerWeekend = c.wertOptional(.pricePerWeekend)
        pricePerWeek = c.wertOptional(.pricePerWeek)
        deposit = c.wertOptional(.deposit)
        tags = c.wert(.tags, [])
        thumbnailUrl = c.wertOptional(.thumbnailUrl)
        productUrl = c.wertOptional(.productUrl)
        webUrl = c.wertOptional(.webUrl)
        images = c.wert(.images, [])
        score = c.wertOptional(.score)
        active = c.wertOptional(.active)
    }
}

/// Routes 1 and 16: `{"items": […]}`. A hull object, never a bare array — so
/// a field can grow next to it without breaking the contract.
nonisolated struct RentalItemsDto: Codable, Sendable {
    var items: [RentalItemDto] = []

    enum CodingKeys: String, CodingKey { case items }

    init(items: [RentalItemDto] = []) { self.items = items }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        items = c.wert(.items, [])
    }
}

/// Route 2: `{"item": {…}}`.
nonisolated struct RentalItemEnvelopeDto: Codable, Sendable {
    var item: RentalItemDto = RentalItemDto()

    enum CodingKeys: String, CodingKey { case item }

    init(item: RentalItemDto = RentalItemDto()) { self.item = item }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        item = c.wert(.item, RentalItemDto())
    }
}

/// Route 3: `{"results": […]}`. No hit is not an error.
nonisolated struct RentalSearchDto: Codable, Sendable {
    var results: [RentalItemDto] = []

    enum CodingKeys: String, CodingKey { case results }

    init(results: [RentalItemDto] = []) { self.results = results }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        results = c.wert(.results, [])
    }
}

/// Route 4: several devices at one price of their own.
nonisolated struct RentalSetDto: Codable, Hashable, Sendable {
    var id: String = ""
    var name: String = ""
    /// Plain text here, **not** Markdown — unlike a device's description.
    var description: String?
    var pricePerDay: Double?
    var deposit: Double?
    var itemIds: [String] = []

    enum CodingKeys: String, CodingKey { case id, name, description, pricePerDay, deposit, itemIds }

    init(id: String = "", name: String = "", description: String? = nil,
         pricePerDay: Double? = nil, deposit: Double? = nil, itemIds: [String] = []) {
        self.id = id; self.name = name; self.description = description
        self.pricePerDay = pricePerDay; self.deposit = deposit; self.itemIds = itemIds
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        name = c.wert(.name, "")
        description = c.wertOptional(.description)
        pricePerDay = c.wertOptional(.pricePerDay)
        deposit = c.wertOptional(.deposit)
        itemIds = c.wert(.itemIds, [])
    }
}

nonisolated struct RentalSetsDto: Codable, Sendable {
    var sets: [RentalSetDto] = []

    enum CodingKeys: String, CodingKey { case sets }

    init(sets: [RentalSetDto] = []) { self.sets = sets }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sets = c.wert(.sets, [])
    }
}

// MARK: - Availability and occupancy

/// Route 5. Two fields, and the app owns neither of them.
///
/// `available` defaults to `false`: an answer we could not read must never
/// read as „go ahead".
nonisolated struct RentalAvailabilityDto: Codable, Sendable {
    var available: Bool = false
    /// `null` or `"occupied"`. Why exactly it is taken is deliberately not
    /// said — that would be somebody else's business.
    var reason: String?

    enum CodingKeys: String, CodingKey { case available, reason }

    init(available: Bool = false, reason: String? = nil) {
        self.available = available; self.reason = reason
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        available = c.wert(.available, false)
        reason = c.wertOptional(.reason)
    }
}

/// Route 6: one taken stretch of days. **Half open** — `startDate` belongs to
/// it, `endDate` does not; `endDate` is the day it comes back.
nonisolated struct RentalPeriodDto: Codable, Hashable, Sendable {
    var deviceId: String?
    var setId: String?
    var startDate: String = ""
    var endDate: String = ""
    /// `"pending"`, `"approved"` or `"blocked"` — all three mean taken. The
    /// difference is for the drawing, not for the decision.
    var status: String = ""

    enum CodingKeys: String, CodingKey { case deviceId, setId, startDate, endDate, status }

    init(deviceId: String? = nil, setId: String? = nil, startDate: String = "",
         endDate: String = "", status: String = "") {
        self.deviceId = deviceId; self.setId = setId
        self.startDate = startDate; self.endDate = endDate; self.status = status
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        deviceId = c.wertOptional(.deviceId)
        setId = c.wertOptional(.setId)
        startDate = c.wert(.startDate, "")
        endDate = c.wert(.endDate, "")
        status = c.wert(.status, "")
    }
}

nonisolated struct RentalOccupancyDto: Codable, Sendable {
    var periods: [RentalPeriodDto] = []

    enum CodingKeys: String, CodingKey { case periods }

    init(periods: [RentalPeriodDto] = []) { self.periods = periods }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        periods = c.wert(.periods, [])
    }
}

// MARK: - Profile

/// Route 7 and 8. `admin` is deliberately absent from this payload: letting
/// somebody lend is decided in the web version, and the app has nothing to do
/// with it.
nonisolated struct RentalProfileDto: Codable, Hashable, Sendable {
    var name: String?
    /// Comes from the Rössing-ID and cannot be changed here.
    var email: String?
    var phone: String?
    var addressStreet: String?
    var addressZip: String?
    var addressCity: String?
    var lender: Bool = false
    /// `"none"`, `"pending"` or `"approved"`. Only `"approved"` opens the
    /// lender's side — and the platform says so, the app does not work it out.
    var lenderStatus: String = "none"
    var profileComplete: Bool = false
    var missingFields: [String] = []

    enum CodingKeys: String, CodingKey {
        case name, email, phone, addressStreet, addressZip, addressCity
        case lender, lenderStatus, profileComplete, missingFields
    }

    init(name: String? = nil, email: String? = nil, phone: String? = nil,
         addressStreet: String? = nil, addressZip: String? = nil, addressCity: String? = nil,
         lender: Bool = false, lenderStatus: String = "none",
         profileComplete: Bool = false, missingFields: [String] = []) {
        self.name = name; self.email = email; self.phone = phone
        self.addressStreet = addressStreet; self.addressZip = addressZip
        self.addressCity = addressCity; self.lender = lender
        self.lenderStatus = lenderStatus; self.profileComplete = profileComplete
        self.missingFields = missingFields
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = c.wertOptional(.name)
        email = c.wertOptional(.email)
        phone = c.wertOptional(.phone)
        addressStreet = c.wertOptional(.addressStreet)
        addressZip = c.wertOptional(.addressZip)
        addressCity = c.wertOptional(.addressCity)
        lender = c.wert(.lender, false)
        lenderStatus = c.wert(.lenderStatus, "none")
        profileComplete = c.wert(.profileComplete, false)
        missingFields = c.wert(.missingFields, [])
    }
}

nonisolated struct RentalProfileEnvelopeDto: Codable, Sendable {
    var profile: RentalProfileDto = RentalProfileDto()

    enum CodingKeys: String, CodingKey { case profile }

    init(profile: RentalProfileDto = RentalProfileDto()) { self.profile = profile }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        profile = c.wert(.profile, RentalProfileDto())
    }
}

/// Route 8. Only the fields that are sent change, so every one of them is
/// optional and `nil` ones are left out of the body entirely.
nonisolated struct RentalProfilePatchDto: Encodable, Sendable {
    var name: String?
    var phone: String?
    var addressStreet: String?
    var addressZip: String?
    var addressCity: String?

    // No `email`: it comes from the Rössing-ID and is the link to the
    // account. The platform ignores it; we do not even send it.
    enum CodingKeys: String, CodingKey { case name, phone, addressStreet, addressZip, addressCity }
}

/// Route 9: the receipt for „please let me lend things out", not the
/// permission itself.
nonisolated struct RentalLenderRequestDto: Codable, Sendable {
    var lenderStatus: String = "pending"
    /// German and meant for the screen.
    var message: String = ""

    enum CodingKeys: String, CodingKey { case lenderStatus, message }

    init(lenderStatus: String = "pending", message: String = "") {
        self.lenderStatus = lenderStatus; self.message = message
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        lenderStatus = c.wert(.lenderStatus, "pending")
        message = c.wert(.message, "")
    }
}

// MARK: - Bookings

/// Route 10: where to pick the thing up. **The only place in this interface
/// carrying another person's address and telephone number**, and only after
/// they confirmed the booking. Not to be cached, not to be shown elsewhere.
nonisolated struct RentalPickupDto: Codable, Hashable, Sendable {
    var address: String?
    var phone: String?

    enum CodingKeys: String, CodingKey { case address, phone }

    init(address: String? = nil, phone: String? = nil) {
        self.address = address; self.phone = phone
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        address = c.wertOptional(.address)
        phone = c.wertOptional(.phone)
    }
}

/// Routes 10 and 11: a booking seen by the person who made it.
nonisolated struct RentalBookingDto: Codable, Hashable, Sendable {
    var id: String = ""
    var deviceId: String?
    var setId: String?
    /// With a set booking this is the set's name.
    var deviceName: String = ""
    var startDate: String = ""
    var endDate: String = ""
    /// `"pending"`, `"approved"`, `"rejected"` or `"cancelled"`.
    var status: String = ""
    var notes: String?
    /// Whether route 12 has any prospect right now. **The button follows
    /// this**, not a status check of our own.
    var canCancel: Bool = false
    var pickup: RentalPickupDto?

    enum CodingKeys: String, CodingKey {
        case id, deviceId, setId, deviceName, startDate, endDate, status, notes, canCancel, pickup
    }

    init(id: String = "", deviceId: String? = nil, setId: String? = nil,
         deviceName: String = "", startDate: String = "", endDate: String = "",
         status: String = "", notes: String? = nil, canCancel: Bool = false,
         pickup: RentalPickupDto? = nil) {
        self.id = id; self.deviceId = deviceId; self.setId = setId
        self.deviceName = deviceName; self.startDate = startDate; self.endDate = endDate
        self.status = status; self.notes = notes; self.canCancel = canCancel
        self.pickup = pickup
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        deviceId = c.wertOptional(.deviceId)
        setId = c.wertOptional(.setId)
        deviceName = c.wert(.deviceName, "")
        startDate = c.wert(.startDate, "")
        endDate = c.wert(.endDate, "")
        status = c.wert(.status, "")
        notes = c.wertOptional(.notes)
        canCancel = c.wert(.canCancel, false)
        pickup = c.wertOptional(.pickup)
    }
}

nonisolated struct RentalBookingsDto: Codable, Sendable {
    var bookings: [RentalBookingDto] = []

    enum CodingKeys: String, CodingKey { case bookings }

    init(bookings: [RentalBookingDto] = []) { self.bookings = bookings }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        bookings = c.wert(.bookings, [])
    }
}

nonisolated struct RentalBookingEnvelopeDto: Codable, Sendable {
    var booking: RentalBookingDto = RentalBookingDto()

    enum CodingKeys: String, CodingKey { case booking }

    init(booking: RentalBookingDto = RentalBookingDto()) { self.booking = booking }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        booking = c.wert(.booking, RentalBookingDto())
    }
}

/// Route 11. Leaving the three personal fields out is the ordinary way for
/// the app: it has the profile from route 7, so nobody has to type their own
/// name again. If they are missing here **and** in the profile, the platform
/// answers `profile_incomplete` and the app leads to route 8.
nonisolated struct RentalBookingRequestDto: Encodable, Sendable {
    var deviceId: String?
    var setId: String?
    var startDate: String
    var endDate: String
    var firstName: String?
    var lastName: String?
    var phone: String?
    var notes: String?

    enum CodingKeys: String, CodingKey {
        case deviceId, setId, startDate, endDate, firstName, lastName, phone, notes
    }
}

/// Routes 12, 14 and 15: `{"status": "cancelled"}` and friends.
nonisolated struct RentalStatusDto: Codable, Sendable {
    var status: String = ""

    enum CodingKeys: String, CodingKey { case status }

    init(status: String = "") { self.status = status }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        status = c.wert(.status, "")
    }
}

// MARK: - The lender's side

/// Route 13: a booking seen by the person the device belongs to.
///
/// `renterName` and `renterPhone` are here because the handover has to be
/// arranged. They belong in **no** other view and in no store that outlives
/// the screen.
nonisolated struct RentalOwnerBookingDto: Codable, Hashable, Sendable {
    var id: String = ""
    var deviceId: String?
    var deviceName: String = ""
    var startDate: String = ""
    var endDate: String = ""
    var status: String = ""
    var renterName: String?
    var renterPhone: String?
    var notes: String?
    /// Whether routes 14 and 15 work right now. The buttons follow this.
    var canDecide: Bool = false
    var canCancel: Bool = false

    enum CodingKeys: String, CodingKey {
        case id, deviceId, deviceName, startDate, endDate, status
        case renterName, renterPhone, notes, canDecide, canCancel
    }

    init(id: String = "", deviceId: String? = nil, deviceName: String = "",
         startDate: String = "", endDate: String = "", status: String = "",
         renterName: String? = nil, renterPhone: String? = nil, notes: String? = nil,
         canDecide: Bool = false, canCancel: Bool = false) {
        self.id = id; self.deviceId = deviceId; self.deviceName = deviceName
        self.startDate = startDate; self.endDate = endDate; self.status = status
        self.renterName = renterName; self.renterPhone = renterPhone; self.notes = notes
        self.canDecide = canDecide; self.canCancel = canCancel
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        deviceId = c.wertOptional(.deviceId)
        deviceName = c.wert(.deviceName, "")
        startDate = c.wert(.startDate, "")
        endDate = c.wert(.endDate, "")
        status = c.wert(.status, "")
        renterName = c.wertOptional(.renterName)
        renterPhone = c.wertOptional(.renterPhone)
        notes = c.wertOptional(.notes)
        canDecide = c.wert(.canDecide, false)
        canCancel = c.wert(.canCancel, false)
    }
}

nonisolated struct RentalOwnerBookingsDto: Codable, Sendable {
    var bookings: [RentalOwnerBookingDto] = []

    enum CodingKeys: String, CodingKey { case bookings }

    init(bookings: [RentalOwnerBookingDto] = []) { self.bookings = bookings }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        bookings = c.wert(.bookings, [])
    }
}

/// Routes 17 and 18: a stretch the lender keeps for themselves. To everybody
/// else it looks like any other taken period — no reason, no name.
nonisolated struct RentalBlockDto: Codable, Hashable, Sendable {
    var id: String = ""
    var deviceId: String = ""
    var deviceName: String = ""
    var startDate: String = ""
    var endDate: String = ""
    var reason: String?

    enum CodingKeys: String, CodingKey { case id, deviceId, deviceName, startDate, endDate, reason }

    init(id: String = "", deviceId: String = "", deviceName: String = "",
         startDate: String = "", endDate: String = "", reason: String? = nil) {
        self.id = id; self.deviceId = deviceId; self.deviceName = deviceName
        self.startDate = startDate; self.endDate = endDate; self.reason = reason
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        deviceId = c.wert(.deviceId, "")
        deviceName = c.wert(.deviceName, "")
        startDate = c.wert(.startDate, "")
        endDate = c.wert(.endDate, "")
        reason = c.wertOptional(.reason)
    }
}

nonisolated struct RentalBlocksDto: Codable, Sendable {
    var blocks: [RentalBlockDto] = []

    enum CodingKeys: String, CodingKey { case blocks }

    init(blocks: [RentalBlockDto] = []) { self.blocks = blocks }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        blocks = c.wert(.blocks, [])
    }
}

nonisolated struct RentalBlockEnvelopeDto: Codable, Sendable {
    var block: RentalBlockDto = RentalBlockDto()

    enum CodingKeys: String, CodingKey { case block }

    init(block: RentalBlockDto = RentalBlockDto()) { self.block = block }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        block = c.wert(.block, RentalBlockDto())
    }
}

/// Route 18. Sets cannot be blocked, only single devices.
nonisolated struct RentalBlockRequestDto: Encodable, Sendable {
    var deviceId: String
    var startDate: String
    var endDate: String
    var reason: String?

    enum CodingKeys: String, CodingKey { case deviceId, startDate, endDate, reason }
}

// MARK: - Errors

/// Every refusal of the platform has this shape.
///
/// `code` is stable and machine-readable, `message` is German and may be
/// shown. **The app branches on `code`, never on `message`**
/// (`docs/mieten-api.md`, „Fehler").
nonisolated struct RentalErrorDto: Decodable, Sendable {
    var code: String = ""
    var message: String = ""
    /// Only with `profile_incomplete`: which fields the platform is missing.
    var missingFields: [String] = []

    enum Hull: String, CodingKey { case error }
    enum CodingKeys: String, CodingKey { case code, message, missingFields }

    init(code: String = "", message: String = "", missingFields: [String] = []) {
        self.code = code; self.message = message; self.missingFields = missingFields
    }

    init(from decoder: Decoder) throws {
        let hull = try decoder.container(keyedBy: Hull.self)
        let c = try hull.nestedContainer(keyedBy: CodingKeys.self, forKey: .error)
        code = c.wert(.code, "")
        message = c.wert(.message, "")
        // The document puts `missingFields` next to `code` and `message`
        // inside `error`; reading it defensively costs nothing.
        missingFields = c.wert(.missingFields, [])
    }
}
