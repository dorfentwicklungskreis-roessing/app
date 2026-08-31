import Foundation

/// Bookings, blocks and the profile, prepared for the screens.
///
/// Same rule as everywhere in this area: nothing here decides anything.
/// `canCancel`, `canDecide` and `lenderStatus` arrive from the platform and
/// are carried through; the buttons follow them.

// MARK: - How far along a booking is

/// The four states of `docs/mieten-api.md` — and everything else the platform
/// might invent, which keeps its own wording rather than being squeezed into
/// one of ours.
nonisolated enum RentalBookingState: Hashable, Sendable {
    case pending
    case approved
    case rejected
    case cancelled
    case other(String)

    init(raw: String) {
        switch raw.lowercased() {
        case "pending": self = .pending
        case "approved": self = .approved
        case "rejected": self = .rejected
        case "cancelled": self = .cancelled
        default: self = .other(raw)
        }
    }

    var label: String {
        switch self {
        case .pending: return "Angefragt"
        case .approved: return "Bestätigt"
        case .rejected: return "Abgelehnt"
        case .cancelled: return "Storniert"
        case .other(let raw): return raw.isEmpty ? "Unbekannt" : raw
        }
    }

    var symbol: String {
        switch self {
        case .pending: return "hourglass"
        case .approved: return "checkmark.seal.fill"
        case .rejected: return "xmark.circle"
        case .cancelled: return "slash.circle"
        case .other: return "questionmark.circle"
        }
    }
}

// MARK: - My bookings

/// One booking of mine, ready for the list.
nonisolated struct RentalBooking: Identifiable, Hashable, Sendable {
    let id: String
    let deviceId: String?
    let setId: String?
    let deviceName: String
    /// The days the thing is mine.
    let periodText: String
    /// „Rückgabe: So, 07.09.2026" — the day it goes back.
    let returnText: String?
    let state: RentalBookingState
    let statusLabel: String
    /// Whether route 12 has any prospect. The platform decides, not the app.
    let canCancel: Bool
    /// Only ever filled once the booking is confirmed. It is the one place in
    /// this whole interface carrying somebody else's address, and it is not
    /// stored anywhere beyond this screen.
    let pickupAddress: String?
    let pickupPhone: String?
    let notes: String?
    let start: Date?
    /// The return day.
    let end: Date?

    /// Over and done with — the return day has come.
    func isPast(_ now: Date, zone: TimeZone = RentalDay.villageZone) -> Bool {
        guard let end else { return false }
        return end <= RentalDay.calendar(zone).startOfDay(for: now)
    }
}

nonisolated extension RentalBookingDto {
    func asBooking(zone: TimeZone = RentalDay.villageZone) -> RentalBooking? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        let state = RentalBookingState(raw: status)
        let name = deviceName.trimmingCharacters(in: .whitespacesAndNewlines)
        return RentalBooking(
            id: id,
            deviceId: deviceId,
            setId: setId,
            deviceName: name.isEmpty ? (deviceId ?? setId ?? "Gerät") : name,
            periodText: RentalDay.occupiedText(
                startDate: startDate, endDate: endDate, zone: zone
            ),
            returnText: RentalDay.returnText(endDate: endDate, zone: zone),
            state: state,
            statusLabel: state.label,
            canCancel: canCancel,
            pickupAddress: pickup?.address?.rentalNonEmpty,
            pickupPhone: pickup?.phone?.rentalNonEmpty,
            notes: notes?.rentalNonEmpty,
            start: RentalDay.parse(startDate, zone: zone),
            end: RentalDay.parse(endDate, zone: zone)
        )
    }
}

nonisolated extension Collection where Element == RentalBookingDto {
    /// My bookings in the order they matter: what is still ahead first, in
    /// the order it happens, and the finished ones behind it, the most recent
    /// of them on top.
    func asBookings(now: Date, zone: TimeZone = RentalDay.villageZone) -> [RentalBooking] {
        let all = compactMap { $0.asBooking(zone: zone) }
        let ahead = all.filter { !$0.isPast(now, zone: zone) }
            .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
        let over = all.filter { $0.isPast(now, zone: zone) }
            .sorted { ($0.start ?? .distantPast) > ($1.start ?? .distantPast) }
        return ahead + over
    }
}

// MARK: - Bookings on my own devices

/// One booking seen by the person the device belongs to.
nonisolated struct RentalOwnerBooking: Identifiable, Hashable, Sendable {
    let id: String
    let deviceId: String?
    let deviceName: String
    let periodText: String
    let returnText: String?
    let state: RentalBookingState
    let statusLabel: String
    /// Here so the handover can be arranged — and nowhere else.
    let renterName: String?
    let renterPhone: String?
    let notes: String?
    /// Whether routes 14 and 15 work right now.
    let canDecide: Bool
    let canCancel: Bool
    let start: Date?
    let end: Date?

    func isPast(_ now: Date, zone: TimeZone = RentalDay.villageZone) -> Bool {
        guard let end else { return false }
        return end <= RentalDay.calendar(zone).startOfDay(for: now)
    }
}

nonisolated extension RentalOwnerBookingDto {
    func asOwnerBooking(zone: TimeZone = RentalDay.villageZone) -> RentalOwnerBooking? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        let state = RentalBookingState(raw: status)
        let name = deviceName.trimmingCharacters(in: .whitespacesAndNewlines)
        return RentalOwnerBooking(
            id: id,
            deviceId: deviceId,
            deviceName: name.isEmpty ? (deviceId ?? "Gerät") : name,
            periodText: RentalDay.occupiedText(
                startDate: startDate, endDate: endDate, zone: zone
            ),
            returnText: RentalDay.returnText(endDate: endDate, zone: zone),
            state: state,
            statusLabel: state.label,
            renterName: renterName?.rentalNonEmpty,
            renterPhone: renterPhone?.rentalNonEmpty,
            notes: notes?.rentalNonEmpty,
            canDecide: canDecide,
            canCancel: canCancel,
            start: RentalDay.parse(startDate, zone: zone),
            end: RentalDay.parse(endDate, zone: zone)
        )
    }
}

nonisolated extension Collection where Element == RentalOwnerBookingDto {
    /// What waits for a decision comes first — that is the whole reason
    /// somebody opens this screen. After that the same order as everywhere:
    /// upcoming by date, finished ones behind.
    func asOwnerBookings(now: Date, zone: TimeZone = RentalDay.villageZone) -> [RentalOwnerBooking] {
        let all = compactMap { $0.asOwnerBooking(zone: zone) }
        let waiting = all.filter { $0.canDecide }
            .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
        let ahead = all.filter { !$0.canDecide && !$0.isPast(now, zone: zone) }
            .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
        let over = all.filter { !$0.canDecide && $0.isPast(now, zone: zone) }
            .sorted { ($0.start ?? .distantPast) > ($1.start ?? .distantPast) }
        return waiting + ahead + over
    }
}

// MARK: - Blocks

/// A stretch the lender keeps for themselves.
nonisolated struct RentalBlock: Identifiable, Hashable, Sendable {
    let id: String
    let deviceId: String
    let deviceName: String
    let periodText: String
    let returnText: String?
    let reason: String?
    let start: Date?
    let end: Date?
}

nonisolated extension RentalBlockDto {
    func asBlock(zone: TimeZone = RentalDay.villageZone) -> RentalBlock? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        let name = deviceName.trimmingCharacters(in: .whitespacesAndNewlines)
        return RentalBlock(
            id: id,
            deviceId: deviceId,
            deviceName: name.isEmpty ? deviceId : name,
            periodText: RentalDay.occupiedText(
                startDate: startDate, endDate: endDate, zone: zone
            ),
            returnText: RentalDay.returnText(endDate: endDate, zone: zone),
            reason: reason?.rentalNonEmpty,
            start: RentalDay.parse(startDate, zone: zone),
            end: RentalDay.parse(endDate, zone: zone)
        )
    }
}

nonisolated extension Collection where Element == RentalBlockDto {
    /// Blocks that are over are of no use to anybody; the rest by date.
    func asBlocks(now: Date, zone: TimeZone = RentalDay.villageZone) -> [RentalBlock] {
        let today = RentalDay.calendar(zone).startOfDay(for: now)
        return compactMap { $0.asBlock(zone: zone) }
            .filter { block in
                guard let end = block.end else { return true }
                return end > today
            }
            .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
    }
}

// MARK: - Profile

/// Whether somebody may lend things out. The platform says so; nobody works
/// it out here.
nonisolated enum RentalLenderStatus: String, Hashable, Sendable {
    case none
    case pending
    case approved
    case unknown

    init(raw: String) {
        self = RentalLenderStatus(rawValue: raw.lowercased()) ?? .unknown
    }

    var label: String {
        switch self {
        case .none: return "Du verleihst noch nichts."
        case .pending: return "Deine Anfrage als Verleiher liegt vor."
        case .approved: return "Du bist als Verleiher freigeschaltet."
        case .unknown: return "Unbekannter Stand."
        }
    }
}

/// The profile as the screen shows it.
nonisolated struct RentalProfile: Hashable, Sendable {
    let name: String
    /// Comes from the Rössing-ID and cannot be changed here.
    let email: String
    let phone: String
    let addressStreet: String
    let addressZip: String
    let addressCity: String
    let lenderStatus: RentalLenderStatus
    let complete: Bool
    /// What the platform is still missing, in German and readable.
    let missingLabels: [String]

    /// Whether the lender's side is shown at all. **The platform's answer** —
    /// `approved`, nothing else.
    var showsLenderArea: Bool { lenderStatus == .approved }

    /// Whether asking to become a lender is worth offering.
    var canAskToLend: Bool { lenderStatus == .none }
}

nonisolated extension RentalProfileDto {
    func asProfile() -> RentalProfile {
        RentalProfile(
            name: name?.rentalNonEmpty ?? "",
            email: email?.rentalNonEmpty ?? "",
            phone: phone?.rentalNonEmpty ?? "",
            addressStreet: addressStreet?.rentalNonEmpty ?? "",
            addressZip: addressZip?.rentalNonEmpty ?? "",
            addressCity: addressCity?.rentalNonEmpty ?? "",
            lenderStatus: RentalLenderStatus(raw: lenderStatus),
            complete: profileComplete,
            missingLabels: RentalFieldNames.labels(missingFields)
        )
    }
}

/// The platform names its fields in English (`addressZip`); a person reads
/// German. One table, used by the profile screen and by every
/// `profile_incomplete` refusal.
nonisolated enum RentalFieldNames {
    static let german: [String: String] = [
        "name": "Name",
        "phone": "Telefonnummer",
        "addressStreet": "Straße und Hausnummer",
        "addressZip": "Postleitzahl",
        "addressCity": "Ort",
    ]

    /// Unknown field names are passed through rather than dropped: a person
    /// reading „addressCountry" at least knows something is missing.
    static func labels(_ fields: [String]) -> [String] {
        fields.compactMap { field in
            let key = field.trimmingCharacters(in: .whitespacesAndNewlines)
            if key.isEmpty { return nil }
            return german[key] ?? key
        }
    }

    /// „Telefonnummer, Ort" — for a single line above a form.
    static func sentence(_ fields: [String]) -> String? {
        let list = labels(fields)
        if list.isEmpty { return nil }
        return list.joined(separator: ", ")
    }
}

// MARK: - Small helpers

/// Prefixed rather than plain `nonEmpty`: this is a convenience of this area,
/// not a new habit for the whole app.
nonisolated extension String {
    /// The text, or nothing at all — „a name made of spaces" appears a dozen
    /// times in this area and is never worth showing.
    var rentalNonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
