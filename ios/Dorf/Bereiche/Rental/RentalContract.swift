import Foundation

/// The contract with the rental platform („Maschinchenring",
/// `mieten.…`) — routes and payloads, and nothing else.
///
/// **This file is the one place that changes when the platform's interface
/// lands.** Everything above it (client, presentation, models, views) talks
/// in terms of these types, so a renamed field or a moved route costs one
/// edit here plus its example file, not a walk through the whole area.
///
/// Why it is written down as a guess in the first place: the platform's
/// JSON interface is being built in parallel (`docs/mietplattform-in-den-apps.md`,
/// AP 3) and its description (`docs/mieten-api.md`) had not arrived when this
/// area was written. Only one route below is verified against the running
/// system — `GET /bookings`, the public list of occupied periods, which the
/// platform serves today. The rest follows the shape the platform's own MCP
/// tools and website already use, and is driven by the example files under
/// `ios/DorfTests/Fixtures/`.
///
/// Two habits keep this survivable, both taken from the events area:
///  - every field falls back to a default, so the platform may add fields
///    without older app versions going blank;
///  - the app reads decisions, it does not compute them. `bookable`,
///    `available`, `canCancel` and every reason text come from the server.
///    The app may grey a button out because the server said so; it must not
///    know the condition (`docs/mietplattform-in-den-apps.md`, AP 6).

// MARK: - Routes

/// Where the things live. Paths only — the host comes from
/// `Konfiguration.rentalBaseUrl`, so CI and tests can point elsewhere.
nonisolated enum RentalRoutes {
    /// Public: the catalogue.
    static let items = "api/v1/items"
    /// Public: one device with its pictures.
    static func item(_ id: String) -> String { "api/v1/items/\(id)" }
    /// Public: „is it free from … to …?" — the platform's answer, not ours.
    static func availability(_ id: String) -> String { "api/v1/items/\(id)/availability" }
    /// Public and **verified against the running system**: occupied periods
    /// for every device, without any personal data.
    static let occupied = "bookings"

    /// Needs a token: my own bookings.
    static let myBookings = "api/v1/my-bookings"
    /// Needs a token: book a device.
    static let bookings = "api/v1/bookings"
    /// Needs a token: withdraw one of my bookings.
    static func cancel(_ id: String) -> String { "api/v1/bookings/\(id)/cancel" }

    static let searchParameter = "q"
    static let fromParameter = "from"
    static let toParameter = "to"
}

// MARK: - Catalogue

/// One device in the catalogue.
///
/// Dates and money arrive as the platform formats them wherever it has an
/// opinion (`priceText`); the plain numbers are there so the app can still
/// say something if it has not.
nonisolated struct RentalItemDto: Codable, Hashable, Sendable {
    var id: String = ""
    var name: String = ""
    /// One line for the list.
    var summary: String = ""
    /// The long text for the detail page.
    var description: String = ""
    var category: String?
    var pricePerDay: Double?
    /// Already formatted by the platform („5,00 € pro Tag") — wins over
    /// `pricePerDay` when both are there.
    var priceText: String?
    var deposit: Double?
    var depositText: String?
    var imageUrl: String?
    var images: [String] = []
    /// Who lends it out — a display name, never an address. Addresses are
    /// personal data and only appear on a confirmed booking of my own.
    var ownerName: String?
    var pickupHint: String?
    /// Whether the platform offers this device for booking at all.
    var bookable: Bool = true
    /// Why not, in the platform's own words. Shown verbatim.
    var unavailableReason: String?

    enum CodingKeys: String, CodingKey {
        case id, name, summary, description, category
        case pricePerDay, priceText, deposit, depositText
        case imageUrl, images, ownerName, pickupHint, bookable, unavailableReason
    }

    init(id: String = "", name: String = "", summary: String = "", description: String = "",
         category: String? = nil, pricePerDay: Double? = nil, priceText: String? = nil,
         deposit: Double? = nil, depositText: String? = nil, imageUrl: String? = nil,
         images: [String] = [], ownerName: String? = nil, pickupHint: String? = nil,
         bookable: Bool = true, unavailableReason: String? = nil) {
        self.id = id; self.name = name; self.summary = summary; self.description = description
        self.category = category; self.pricePerDay = pricePerDay; self.priceText = priceText
        self.deposit = deposit; self.depositText = depositText; self.imageUrl = imageUrl
        self.images = images; self.ownerName = ownerName; self.pickupHint = pickupHint
        self.bookable = bookable; self.unavailableReason = unavailableReason
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        name = c.wert(.name, "")
        summary = c.wert(.summary, "")
        description = c.wert(.description, "")
        category = c.wertOptional(.category)
        pricePerDay = c.wertOptional(.pricePerDay)
        priceText = c.wertOptional(.priceText)
        deposit = c.wertOptional(.deposit)
        depositText = c.wertOptional(.depositText)
        imageUrl = c.wertOptional(.imageUrl)
        images = c.wert(.images, [])
        ownerName = c.wertOptional(.ownerName)
        pickupHint = c.wertOptional(.pickupHint)
        bookable = c.wert(.bookable, true)
        unavailableReason = c.wertOptional(.unavailableReason)
    }
}

/// The catalogue as the platform hands it over. Wrapped in an object rather
/// than a bare array so it can grow a cursor later without breaking us.
nonisolated struct RentalItemListDto: Codable, Sendable {
    var items: [RentalItemDto] = []
    var generatedAt: String = ""

    enum CodingKeys: String, CodingKey { case items, generatedAt }

    init(items: [RentalItemDto] = [], generatedAt: String = "") {
        self.items = items; self.generatedAt = generatedAt
    }

    init(from decoder: Decoder) throws {
        // A bare array is just as plausible a shape as an object with an
        // `items` key, and which one it will be is not settled yet. Reading
        // both costs four lines and saves a broken release.
        if let list = try? decoder.singleValueContainer().decode([RentalItemDto].self) {
            items = list
            return
        }
        let c = try decoder.container(keyedBy: CodingKeys.self)
        items = c.wert(.items, [])
        generatedAt = c.wert(.generatedAt, "")
    }
}

/// A device with everything the detail page shows.
nonisolated struct RentalItemDetailDto: Codable, Sendable {
    var item: RentalItemDto = RentalItemDto()
    /// Periods that are taken already — for the little calendar under the
    /// device. Purely informational; whether a wish fits is answered by
    /// `RentalAvailabilityDto`, never by this list.
    var blockedPeriods: [RentalPeriodDto] = []

    enum CodingKeys: String, CodingKey { case item, blockedPeriods }

    init(item: RentalItemDto = RentalItemDto(), blockedPeriods: [RentalPeriodDto] = []) {
        self.item = item; self.blockedPeriods = blockedPeriods
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        blockedPeriods = c.wert(.blockedPeriods, [])
        if let nested: RentalItemDto = c.wertOptional(.item) {
            item = nested
            return
        }
        // The device may just as well be the top-level object itself.
        item = (try? RentalItemDto(from: decoder)) ?? RentalItemDto()
    }
}

/// A stretch of days, both ends inclusive („2026-09-04" … „2026-09-06").
nonisolated struct RentalPeriodDto: Codable, Hashable, Sendable {
    var startDate: String = ""
    var endDate: String = ""
    var deviceId: String?
    var setId: String?
    var status: String?

    enum CodingKeys: String, CodingKey { case startDate, endDate, deviceId, setId, status }

    init(startDate: String = "", endDate: String = "", deviceId: String? = nil,
         setId: String? = nil, status: String? = nil) {
        self.startDate = startDate; self.endDate = endDate
        self.deviceId = deviceId; self.setId = setId; self.status = status
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        startDate = c.wert(.startDate, "")
        endDate = c.wert(.endDate, "")
        deviceId = c.wertOptional(.deviceId)
        setId = c.wertOptional(.setId)
        status = c.wertOptional(.status)
    }
}

// MARK: - Availability

/// „Is this device free from … to …?" — asked and answered by the platform.
///
/// The app does not own a single line of that decision. It shows `reason`
/// word for word and enables the booking button only if `bookable` is true.
nonisolated struct RentalAvailabilityDto: Codable, Sendable {
    var itemId: String = ""
    var from: String = ""
    var to: String = ""
    /// Nothing else is booked in that stretch.
    var available: Bool = false
    /// May *this* person book it, right now, for that stretch. Separate from
    /// `available` on purpose: free is not the same as allowed.
    var bookable: Bool = false
    /// The platform's sentence when it says no.
    var reason: String?
    var days: Int?
    var totalPrice: Double?
    var priceText: String?

    enum CodingKeys: String, CodingKey {
        case itemId, from, to, available, bookable, reason, days, totalPrice, priceText
    }

    init(itemId: String = "", from: String = "", to: String = "", available: Bool = false,
         bookable: Bool = false, reason: String? = nil, days: Int? = nil,
         totalPrice: Double? = nil, priceText: String? = nil) {
        self.itemId = itemId; self.from = from; self.to = to
        self.available = available; self.bookable = bookable; self.reason = reason
        self.days = days; self.totalPrice = totalPrice; self.priceText = priceText
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        itemId = c.wert(.itemId, "")
        self.from = c.wert(.from, "")
        to = c.wert(.to, "")
        available = c.wert(.available, false)
        // If the platform only says „available", that is the answer we have.
        // Guessing „then it is bookable too" would be a rule of our own.
        bookable = c.wert(.bookable, available)
        reason = c.wertOptional(.reason)
        days = c.wertOptional(.days)
        totalPrice = c.wertOptional(.totalPrice)
        priceText = c.wertOptional(.priceText)
    }
}

// MARK: - Bookings

/// What we send to book a device.
nonisolated struct RentalBookingRequestDto: Codable, Sendable {
    var itemId: String
    var from: String
    var to: String
    var note: String

    enum CodingKeys: String, CodingKey { case itemId, from, to, note }
}

/// One booking of mine.
nonisolated struct RentalBookingDto: Codable, Hashable, Sendable {
    var id: String = ""
    var itemId: String = ""
    var itemName: String = ""
    var from: String = ""
    var to: String = ""
    /// `pending`, `approved`, `rejected`, `cancelled` — and whatever else the
    /// platform invents. Unknown values are shown, not swallowed.
    var status: String = ""
    /// The platform's own wording for that status, if it has one.
    var statusText: String?
    /// Whether this booking may still be withdrawn. The platform decides.
    var canCancel: Bool = false
    /// Only ever filled once a booking is confirmed — the platform holds the
    /// personal data back until then, and so does this field.
    var pickupAddress: String?
    var ownerName: String?
    var ownerPhone: String?
    var priceText: String?
    var note: String?
    var createdAt: String?

    enum CodingKeys: String, CodingKey {
        case id, itemId, itemName, from, to, status, statusText, canCancel
        case pickupAddress, ownerName, ownerPhone, priceText, note, createdAt
    }

    init(id: String = "", itemId: String = "", itemName: String = "", from: String = "",
         to: String = "", status: String = "", statusText: String? = nil,
         canCancel: Bool = false, pickupAddress: String? = nil, ownerName: String? = nil,
         ownerPhone: String? = nil, priceText: String? = nil, note: String? = nil,
         createdAt: String? = nil) {
        self.id = id; self.itemId = itemId; self.itemName = itemName
        self.from = from; self.to = to; self.status = status; self.statusText = statusText
        self.canCancel = canCancel; self.pickupAddress = pickupAddress
        self.ownerName = ownerName; self.ownerPhone = ownerPhone
        self.priceText = priceText; self.note = note; self.createdAt = createdAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.wert(.id, "")
        itemId = c.wert(.itemId, "")
        itemName = c.wert(.itemName, "")
        self.from = c.wert(.from, "")
        to = c.wert(.to, "")
        status = c.wert(.status, "")
        statusText = c.wertOptional(.statusText)
        canCancel = c.wert(.canCancel, false)
        pickupAddress = c.wertOptional(.pickupAddress)
        ownerName = c.wertOptional(.ownerName)
        ownerPhone = c.wertOptional(.ownerPhone)
        priceText = c.wertOptional(.priceText)
        note = c.wertOptional(.note)
        createdAt = c.wertOptional(.createdAt)
    }
}

nonisolated struct RentalBookingListDto: Codable, Sendable {
    var bookings: [RentalBookingDto] = []

    enum CodingKeys: String, CodingKey { case bookings }

    init(bookings: [RentalBookingDto] = []) { self.bookings = bookings }

    init(from decoder: Decoder) throws {
        if let list = try? decoder.singleValueContainer().decode([RentalBookingDto].self) {
            bookings = list
            return
        }
        let c = try decoder.container(keyedBy: CodingKeys.self)
        bookings = c.wert(.bookings, [])
    }
}

// MARK: - Errors

/// What the platform says when it refuses. Two spellings are accepted
/// because two are in use over there — the website's routes answer with
/// `error`, the newer services with `message`.
/// Read-only: the app never sends this shape, it only reads it.
nonisolated struct RentalErrorDto: Decodable, Sendable {
    var text: String = ""

    enum CodingKeys: String, CodingKey { case error, message }

    init(text: String = "") { self.text = text }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let fromError: String = c.wert(.error, "")
        let fromMessage: String = c.wert(.message, "")
        text = (fromError.isEmpty ? fromMessage : fromError)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
