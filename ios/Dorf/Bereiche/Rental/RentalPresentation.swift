import Foundation

/// Turning the platform's payloads into what the screens show.
///
/// Deliberately plain preparation — no network, no SwiftUI. It runs in an
/// ordinary unit test, which is where the tricky parts belong: dates that
/// arrive as bare days, money that the platform may or may not have
/// formatted already, and a status whose wording belongs to the platform.
///
/// What is **not** here: any decision. Whether a device may be booked,
/// whether a booking may be withdrawn, whether a stretch of days is free —
/// all of that arrives as a flag and is only passed through. The app may grey
/// a button out because the server said so; it must not know the condition.

// MARK: - Days

/// Everything that turns the platform's `2026-09-04` into a date and back.
///
/// The platform speaks in whole days, not in instants: a booking runs from
/// one day to another, both ends included. Reading such a day as UTC would
/// move it by an hour twice a year, so it is read in the village's own time
/// zone — the same habit as in the events area.
nonisolated enum RentalDay {
    /// The village's own time zone. Spelled out rather than borrowed from
    /// `Zeitpunkt`: that one belongs to the main actor, and this preparation
    /// deliberately runs anywhere.
    static let villageZone = TimeZone(identifier: "Europe/Berlin") ?? .current

    /// Short weekdays, wired down so device and test agree no matter which
    /// language the phone is set to.
    static let weekdays = ["Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"]

    static func calendar(_ zone: TimeZone = villageZone) -> Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = zone
        c.locale = Locale(identifier: "de_DE")
        return c
    }

    /// Reads `2026-09-04` as midnight in the village. Anything unreadable is
    /// `nil` — one broken entry must not cost the whole list.
    static func parse(_ text: String, zone: TimeZone = villageZone) -> Date? {
        let raw = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if raw.isEmpty { return nil }
        let f = ISO8601DateFormatter()
        f.timeZone = zone
        f.formatOptions = [.withFullDate]
        if let day = f.date(from: raw) { return day }
        // Some routes answer with a full timestamp; the day is what we want.
        guard let instant = RFC3339.datum(raw) else { return nil }
        return calendar(zone).startOfDay(for: instant)
    }

    /// Back into the platform's spelling — this is what goes out in a query
    /// or a booking wish.
    static func api(_ day: Date, zone: TimeZone = villageZone) -> String {
        let t = calendar(zone).dateComponents([.year, .month, .day], from: day)
        return String(format: "%04d-%02d-%02d", t.year ?? 0, t.month ?? 0, t.day ?? 0)
    }

    /// „04.09.2026"
    static func short(_ day: Date, zone: TimeZone = villageZone) -> String {
        let t = calendar(zone).dateComponents([.year, .month, .day], from: day)
        return String(format: "%02d.%02d.%04d", t.day ?? 0, t.month ?? 0, t.year ?? 0)
    }

    /// „Fr, 04.09.2026"
    static func withWeekday(_ day: Date, zone: TimeZone = villageZone) -> String {
        let t = calendar(zone).dateComponents([.weekday], from: day)
        // `weekday` counts from Sunday = 1; our list starts on Monday.
        let name = weekdays[(((t.weekday ?? 1) - 2) + 7) % 7]
        return "\(name), \(short(day, zone: zone))"
    }

    /// „Fr, 04.09.2026 – So, 06.09.2026", and a single day as just itself.
    /// Unreadable ends fall back to whatever the platform sent, so a person
    /// still sees something rather than a gap.
    static func periodText(from: String, to: String, zone: TimeZone = villageZone) -> String {
        guard let start = parse(from, zone: zone) else { return [from, to].joined(separator: " – ") }
        guard let end = parse(to, zone: zone), end != start else {
            return withWeekday(start, zone: zone)
        }
        return withWeekday(start, zone: zone) + " – " + withWeekday(end, zone: zone)
    }
}

/// Money the way it is written in Germany: „5,00 €".
///
/// Only ever used when the platform sent a bare number and no text of its
/// own. Wherever it sent a text, that text wins — it knows about deposits,
/// weekend rates and whatever else it charges.
nonisolated enum RentalMoney {
    static func euro(_ amount: Double) -> String {
        let f = NumberFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.numberStyle = .currency
        f.currencyCode = "EUR"
        return f.string(from: NSNumber(value: amount)) ?? String(format: "%.2f €", amount)
    }
}

// MARK: - A device

/// One device as the list and the detail page show it.
nonisolated struct RentalDevice: Identifiable, Hashable, Sendable {
    let id: String
    let name: String
    let summary: String
    let description: String
    let category: String?
    /// „5,00 € pro Tag" — the platform's wording where it has one.
    let priceText: String?
    /// „Kaution 20,00 €"
    let depositText: String?
    let ownerName: String?
    let pickupHint: String?
    let imageURL: URL?
    let images: [URL]
    /// Whether the platform offers this device for booking. Its answer, not
    /// ours.
    let bookable: Bool
    /// Why not — in the platform's words.
    let unavailableNote: String?
    /// Everything searchable about this device, folded down once so the
    /// filter does not rebuild it on every keystroke.
    let searchIndex: String

    /// Does this device match what somebody typed? Every word has to appear
    /// somewhere — „rasen walze" finds the Rasenwalze, and so does „walze".
    func matches(_ query: String) -> Bool {
        let words = query.lowercased()
            .split(whereSeparator: { $0.isWhitespace })
            .map(String.init)
        if words.isEmpty { return true }
        return words.allSatisfy { searchIndex.contains($0) }
    }
}

nonisolated extension RentalItemDto {
    /// A device, or `nil` when the entry carries no identity at all — a row
    /// nobody could open is worse than a row that is not there.
    func asDevice() -> RentalDevice? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        let title = name.trimmingCharacters(in: .whitespacesAndNewlines)

        let price = priceText?.nonEmpty ?? pricePerDay.map { "\(RentalMoney.euro($0)) pro Tag" }
        let deposit = depositText?.nonEmpty ?? self.deposit.map { "Kaution \(RentalMoney.euro($0))" }
        let owner = ownerName?.nonEmpty
        let category = category?.nonEmpty

        let haystack = [title, summary, description, category ?? "", owner ?? ""]
            .joined(separator: " ")
            .lowercased()

        // The lead picture belongs in front of the gallery, and it is often
        // the first of `images` again — showing it twice looks like a bug.
        var pictures: [String] = []
        for candidate in (imageUrl.map { [$0] } ?? []) + images where !pictures.contains(candidate) {
            pictures.append(candidate)
        }

        return RentalDevice(
            id: id,
            name: title.isEmpty ? id : title,
            summary: summary.trimmingCharacters(in: .whitespacesAndNewlines),
            description: description.trimmingCharacters(in: .whitespacesAndNewlines),
            category: category,
            priceText: price,
            depositText: deposit,
            ownerName: owner,
            pickupHint: pickupHint?.nonEmpty,
            imageURL: pictures.first.flatMap(URL.init(string:)),
            images: pictures.compactMap(URL.init(string:)),
            bookable: bookable,
            unavailableNote: unavailableReason?.nonEmpty,
            searchIndex: haystack
        )
    }
}

nonisolated extension Collection where Element == RentalItemDto {
    /// The catalogue as it is shown: readable entries only, sorted by name so
    /// two fetches never shuffle the list under somebody's thumb.
    func asDevices() -> [RentalDevice] {
        compactMap { $0.asDevice() }
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }
}

// MARK: - Occupied periods

/// A stretch of days that is taken already — shown under a device so one can
/// see at a glance whether the weekend is free.
nonisolated struct RentalPeriod: Identifiable, Hashable, Sendable {
    let id: String
    let text: String
    let start: Date?
}

nonisolated extension Collection where Element == RentalPeriodDto {
    /// The taken periods of one device, upcoming ones first, past ones gone.
    /// Purely informational: whether a wish fits is answered by the platform,
    /// never by this list.
    func occupied(deviceId: String, now: Date, zone: TimeZone = RentalDay.villageZone) -> [RentalPeriod] {
        let today = RentalDay.calendar(zone).startOfDay(for: now)
        return enumerated().compactMap { position, period -> RentalPeriod? in
            guard period.deviceId == deviceId else { return nil }
            let start = RentalDay.parse(period.startDate, zone: zone)
            let end = RentalDay.parse(period.endDate, zone: zone) ?? start
            if let end, end < today { return nil }
            return RentalPeriod(
                id: "\(deviceId)-\(position)-\(period.startDate)",
                text: RentalDay.periodText(from: period.startDate, to: period.endDate, zone: zone),
                start: start
            )
        }
        .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
    }
}

// MARK: - Availability

/// The platform's answer to „is it free from … to …?".
nonisolated struct RentalAvailability: Hashable, Sendable {
    let itemId: String
    let from: String
    let to: String
    /// Nothing else is booked in that stretch.
    let free: Bool
    /// May this person book it. Separate from `free` on purpose — free is not
    /// the same as allowed, and only the platform knows the difference.
    let bookable: Bool
    /// What is shown above the button. The platform's sentence where it sent
    /// one; otherwise nothing more than its own yes or no, restated.
    let message: String
    let priceText: String?
}

nonisolated extension RentalAvailabilityDto {
    func asAvailability() -> RentalAvailability {
        let price = priceText?.nonEmpty ?? totalPrice.map { RentalMoney.euro($0) }
        let text: String
        if let reason = reason?.nonEmpty {
            // The platform said why. Its words, unchanged — the rule lives
            // over there and so does the explanation.
            text = reason
        } else if available {
            text = "Der Zeitraum ist frei."
        } else {
            text = "Der Zeitraum ist nicht verfügbar."
        }
        return RentalAvailability(
            itemId: itemId,
            from: from,
            to: to,
            free: available,
            bookable: bookable,
            message: text,
            priceText: price
        )
    }
}

// MARK: - Bookings

/// How far along a booking is. Unknown values keep their own wording rather
/// than being squeezed into one of ours.
nonisolated enum RentalBookingState: Hashable, Sendable {
    case pending
    case approved
    case rejected
    case cancelled
    case other(String)

    init(raw: String) {
        switch raw.lowercased() {
        case "pending", "requested", "open": self = .pending
        case "approved", "confirmed": self = .approved
        case "rejected", "declined": self = .rejected
        case "cancelled", "canceled", "withdrawn": self = .cancelled
        default: self = .other(raw)
        }
    }

    /// The fallback wording, used only when the platform sent no text of its
    /// own.
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

/// One booking of mine, ready for the list.
nonisolated struct RentalBooking: Identifiable, Hashable, Sendable {
    let id: String
    let deviceId: String
    let deviceName: String
    let periodText: String
    let state: RentalBookingState
    /// What the row says about the state — the platform's wording first.
    let statusLabel: String
    /// Whether the platform still allows withdrawing this booking.
    let canCancel: Bool
    /// Only present once the platform hands it over, which it does only for
    /// confirmed bookings of one's own.
    let pickupAddress: String?
    let ownerName: String?
    let ownerPhone: String?
    let priceText: String?
    let note: String?
    let start: Date?
    let end: Date?

    /// Over and done with — the last day has passed.
    func isPast(_ now: Date, zone: TimeZone = RentalDay.villageZone) -> Bool {
        guard let end else { return false }
        return end < RentalDay.calendar(zone).startOfDay(for: now)
    }
}

nonisolated extension RentalBookingDto {
    func asBooking(zone: TimeZone = RentalDay.villageZone) -> RentalBooking? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        let state = RentalBookingState(raw: status)
        return RentalBooking(
            id: id,
            deviceId: itemId,
            deviceName: itemName.nonEmpty ?? itemId.nonEmpty ?? "Gerät",
            periodText: RentalDay.periodText(from: from, to: to, zone: zone),
            state: state,
            statusLabel: statusText?.nonEmpty ?? state.label,
            canCancel: canCancel,
            pickupAddress: pickupAddress?.nonEmpty,
            ownerName: ownerName?.nonEmpty,
            ownerPhone: ownerPhone?.nonEmpty,
            priceText: priceText?.nonEmpty,
            note: note?.nonEmpty,
            start: RentalDay.parse(from, zone: zone),
            end: RentalDay.parse(to, zone: zone) ?? RentalDay.parse(from, zone: zone)
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

// MARK: - Small helpers

/// File-scoped on purpose: this is a convenience of this area, not a new
/// habit for the whole app.
nonisolated private extension String {
    /// The text, or nothing at all — „a name made of spaces" appears a dozen
    /// times in this file and is never worth showing.
    var nonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
