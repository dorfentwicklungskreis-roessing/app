import Foundation

/// Turning the platform's payloads into what the screens show.
///
/// Deliberately plain preparation — no network, no SwiftUI. It runs in an
/// ordinary unit test, which is where the tricky parts belong: days that
/// arrive as bare dates in a half-open range, tariffs that must not be added
/// up, and a description written in Markdown.
///
/// What is **not** here: any decision. Whether a stretch of days is free,
/// whether a booking may be withdrawn, who may confirm — all of that arrives
/// as a flag and is only passed through.

// MARK: - Days

/// Everything that turns the platform's `2026-09-04` into a date and back.
///
/// The platform speaks in whole days, not in instants. Reading such a day as
/// UTC would move it by an hour twice a year, so it is read in the village's
/// own time zone — the same habit as in the events area.
///
/// **The half-open range** (`docs/mieten-api.md`, „Zeiträume") lives here and
/// nowhere else: `startDate` belongs to a period, `endDate` does not — it is
/// the day the thing comes back. A booking from the 5th to the 7th occupies
/// the 5th and the 6th. That is a fact about the calendar, not a rule about
/// renting: the app needs it to draw and to label, and it still asks the
/// platform whether anything may be booked.
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
        return f.date(from: raw)
    }

    /// Back into the platform's spelling — this is what goes out in a query
    /// or a booking wish.
    static func api(_ day: Date, zone: TimeZone = villageZone) -> String {
        let t = calendar(zone).dateComponents([.year, .month, .day], from: day)
        return String(format: "%04d-%02d-%02d", t.year ?? 0, t.month ?? 0, t.day ?? 0)
    }

    /// The day after — the app picks the last day somebody wants the thing,
    /// the platform wants the day it comes back.
    static func nextDay(_ day: Date, zone: TimeZone = villageZone) -> Date {
        calendar(zone).date(byAdding: .day, value: 1, to: day) ?? day
    }

    static func previousDay(_ day: Date, zone: TimeZone = villageZone) -> Date {
        calendar(zone).date(byAdding: .day, value: -1, to: day) ?? day
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

    /// The days a half-open period actually occupies, as a sentence:
    /// „Fr, 04.09.2026 – Sa, 05.09.2026", and a single day as just itself.
    ///
    /// Unreadable ends fall back to whatever the platform sent, so a person
    /// still sees something rather than a gap.
    static func occupiedText(startDate: String, endDate: String,
                             zone: TimeZone = villageZone) -> String {
        guard let start = parse(startDate, zone: zone) else {
            return [startDate, endDate].joined(separator: " – ")
        }
        guard let end = parse(endDate, zone: zone) else { return withWeekday(start, zone: zone) }
        let last = previousDay(end, zone: zone)
        if last <= start { return withWeekday(start, zone: zone) }
        return withWeekday(start, zone: zone) + " – " + withWeekday(last, zone: zone)
    }

    /// „Rückgabe: So, 06.09.2026" — the other half of the same fact, spelled
    /// out rather than left to be worked out from the line above.
    static func returnText(endDate: String, zone: TimeZone = villageZone) -> String? {
        guard let end = parse(endDate, zone: zone) else { return nil }
        return "Rückgabe: " + withWeekday(end, zone: zone)
    }
}

/// Money the way it is written in Germany: „25,00 €".
///
/// Built from a decimal number plus the sign rather than from
/// `numberStyle = .currency`: that one's spacing has moved between ICU
/// versions before, and a price is a thing two people compare across two
/// phones. The space in front of the sign is a non-breaking one, so „25,00"
/// and „€" never end up on two lines.
nonisolated enum RentalMoney {
    static let euroSign = "\u{00A0}€"

    static func euro(_ amount: Double) -> String {
        let f = NumberFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.numberStyle = .decimal
        f.minimumFractionDigits = 2
        f.maximumFractionDigits = 2
        let number = f.string(from: NSNumber(value: amount))
            ?? String(format: "%.2f", amount).replacingOccurrences(of: ".", with: ",")
        return number + euroSign
    }
}

// MARK: - Markdown

/// One piece of a device description.
///
/// The platform writes descriptions in Markdown (`docs/mieten-api.md`, route
/// 1). Showing them raw, with asterisks and dashes, is not an option, and
/// neither is a Markdown library — the app carries none but MapLibre. So the
/// blocks are cut here and the inline part (`**fett**`, links) is left to
/// `AttributedString`, which the standard library brings along.
nonisolated enum RentalTextBlock: Identifiable, Hashable, Sendable {
    case heading(level: Int, text: AttributedString)
    case paragraph(AttributedString)
    case bullet(AttributedString)

    var id: String {
        switch self {
        case .heading(let level, let text): return "h\(level)-\(String(text.characters))"
        case .paragraph(let text): return "p-\(String(text.characters))"
        case .bullet(let text): return "l-\(String(text.characters))"
        }
    }
}

nonisolated enum RentalMarkdown {
    /// Cuts a description into blocks. Deliberately small: paragraphs, `- `
    /// bullets and `##` headings are what the platform's descriptions use.
    /// Anything else stays a paragraph and is still readable.
    static func blocks(_ text: String?) -> [RentalTextBlock] {
        guard let text, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return []
        }
        var result: [RentalTextBlock] = []
        var paragraph: [String] = []

        func flush() {
            let joined = paragraph.joined(separator: " ").trimmingCharacters(in: .whitespaces)
            paragraph.removeAll()
            if joined.isEmpty { return }
            result.append(.paragraph(inline(joined)))
        }

        for rawLine in text.replacingOccurrences(of: "\r\n", with: "\n").split(
            separator: "\n", omittingEmptySubsequences: false
        ) {
            let line = String(rawLine).trimmingCharacters(in: .whitespaces)
            if line.isEmpty {
                flush()
                continue
            }
            if line.hasPrefix("#") {
                flush()
                let hashes = line.prefix { $0 == "#" }.count
                let rest = line.dropFirst(hashes).trimmingCharacters(in: .whitespaces)
                if !rest.isEmpty { result.append(.heading(level: min(hashes, 4), text: inline(rest))) }
                continue
            }
            if line.hasPrefix("- ") || line.hasPrefix("* ") {
                flush()
                let rest = String(line.dropFirst(2)).trimmingCharacters(in: .whitespaces)
                if !rest.isEmpty { result.append(.bullet(inline(rest))) }
                continue
            }
            paragraph.append(line)
        }
        flush()
        return result
    }

    /// The inline part — bold, italics, links. Whatever cannot be parsed is
    /// shown as it stands; a description must never disappear because of a
    /// stray asterisk.
    static func inline(_ text: String) -> AttributedString {
        (try? AttributedString(
            markdown: text,
            options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)
        )) ?? AttributedString(text)
    }
}

// MARK: - A device

/// One device as the list and the detail page show it.
nonisolated struct RentalDevice: Identifiable, Hashable, Sendable {
    let id: String
    let name: String
    /// The description, cut into blocks — empty when there is none.
    let blocks: [RentalTextBlock]
    /// The first paragraph as plain text, for the row in the list.
    let summary: String
    let tags: [String]
    /// „25,00 € pro Tag", „40,00 € pro Wochenende", … — **one line per tariff
    /// the platform actually has.** Never added up: which tariff applies to
    /// which duration is nowhere written down, and an invented rule in the
    /// app would be exactly the split between web and app we avoid
    /// (`docs/mieten-api.md`, „Was bewusst fehlt").
    let tariffs: [String]
    /// „Kaution 100,00 €", or nothing when there is no deposit.
    let depositText: String?
    let thumbnailURL: URL?
    let images: [URL]
    /// The manufacturer's page, where the platform knows one.
    let productURL: URL?
    /// The same device in the web version — for „im Browser öffnen".
    let webURL: URL?
    /// Route 16 only: the lender's own list marks a switched-off device.
    let active: Bool?
    /// Everything searchable about this device, folded down once so the
    /// filter does not rebuild it on every keystroke.
    let searchIndex: String

    /// Does this device match what somebody typed? Every word has to appear
    /// somewhere — „rasen walze" finds the Rasenwalze, and so does „walze".
    ///
    /// This is the fallback for when the platform's own search cannot be
    /// reached; it is a filter over text, not a ranking.
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

        var tariffs: [String] = []
        if let day = pricePerDay { tariffs.append("\(RentalMoney.euro(day)) pro Tag") }
        if let weekend = pricePerWeekend {
            tariffs.append("\(RentalMoney.euro(weekend)) pro Wochenende")
        }
        if let week = pricePerWeek { tariffs.append("\(RentalMoney.euro(week)) pro Woche") }

        let blocks = RentalMarkdown.blocks(description)
        let summary = blocks.compactMap { block -> String? in
            if case .paragraph(let text) = block { return String(text.characters) }
            return nil
        }.first ?? ""

        // The thumbnail belongs in front of the gallery, and it is often the
        // first of `images` again — showing it twice looks like a bug.
        var pictures: [String] = []
        for candidate in (thumbnailUrl.map { [$0] } ?? []) + images.map(\.url)
        where !candidate.isEmpty && !pictures.contains(candidate) {
            pictures.append(candidate)
        }

        let haystack = ([title, summary, description ?? ""] + tags)
            .joined(separator: " ")
            .lowercased()

        return RentalDevice(
            id: id,
            name: title.isEmpty ? id : title,
            blocks: blocks,
            summary: summary,
            tags: tags,
            tariffs: tariffs,
            depositText: deposit.map { "Kaution \(RentalMoney.euro($0))" },
            thumbnailURL: pictures.first.flatMap(URL.init(string:)),
            images: pictures.compactMap(URL.init(string:)),
            productURL: productUrl.flatMap(URL.init(string:)),
            webURL: webUrl.flatMap(URL.init(string:)),
            active: active,
            searchIndex: haystack
        )
    }
}

nonisolated extension Collection where Element == RentalItemDto {
    /// The catalogue as it is shown: readable entries only, in the order the
    /// platform sent them. Route 1 sorts by name over there, and route 3
    /// sorts by how well something matched — **re-sorting here would throw
    /// the search away** (`docs/mieten-api.md`, route 3).
    func asDevices() -> [RentalDevice] {
        compactMap { $0.asDevice() }
    }
}

// MARK: - Sets

/// A set: several devices at one price of its own.
nonisolated struct RentalSet: Identifiable, Hashable, Sendable {
    let id: String
    let name: String
    /// Plain text here, not Markdown.
    let description: String?
    let tariffs: [String]
    let depositText: String?
    let itemIds: [String]
}

nonisolated extension RentalSetDto {
    func asSet() -> RentalSet? {
        let id = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if id.isEmpty { return nil }
        var tariffs: [String] = []
        if let day = pricePerDay { tariffs.append("\(RentalMoney.euro(day)) pro Tag") }
        let text = description?.trimmingCharacters(in: .whitespacesAndNewlines)
        return RentalSet(
            id: id,
            name: name.isEmpty ? id : name,
            description: (text?.isEmpty ?? true) ? nil : text,
            tariffs: tariffs,
            depositText: deposit.map { "Kaution \(RentalMoney.euro($0))" },
            itemIds: itemIds
        )
    }
}

nonisolated extension Collection where Element == RentalSetDto {
    func asSets() -> [RentalSet] { compactMap { $0.asSet() } }
}

// MARK: - Occupied periods

/// Why a stretch of days is taken. For the drawing only — **all three mean
/// taken**, and „angefragt" is not something still to be had.
nonisolated enum RentalOccupancyKind: String, Hashable, Sendable {
    case pending
    case approved
    case blocked
    case unknown

    init(raw: String) {
        self = RentalOccupancyKind(rawValue: raw.lowercased()) ?? .unknown
    }

    var label: String {
        switch self {
        case .pending: return "angefragt"
        case .approved: return "vergeben"
        case .blocked: return "gesperrt"
        case .unknown: return "belegt"
        }
    }
}

/// A stretch of days that is taken already — shown under a device so one can
/// see at a glance whether the weekend is free.
nonisolated struct RentalPeriod: Identifiable, Hashable, Sendable {
    let id: String
    let text: String
    let kind: RentalOccupancyKind
    let start: Date?
    /// The day it comes back — the first day somebody else could start.
    let end: Date?
}

nonisolated extension Collection where Element == RentalPeriodDto {
    /// The taken periods of one device, upcoming ones first, past ones gone.
    ///
    /// Purely informational: whether a wish fits is answered by route 5,
    /// never by this list.
    func occupied(deviceId: String? = nil, now: Date,
                  zone: TimeZone = RentalDay.villageZone) -> [RentalPeriod] {
        let today = RentalDay.calendar(zone).startOfDay(for: now)
        return enumerated().compactMap { position, period -> RentalPeriod? in
            if let deviceId, period.deviceId != deviceId { return nil }
            let start = RentalDay.parse(period.startDate, zone: zone)
            let end = RentalDay.parse(period.endDate, zone: zone)
            // `endDate` is the return day: a period is over once that day has
            // come, not a day later.
            if let end, end <= today { return nil }
            return RentalPeriod(
                id: "\(period.deviceId ?? period.setId ?? "")-\(position)-\(period.startDate)",
                text: RentalDay.occupiedText(
                    startDate: period.startDate, endDate: period.endDate, zone: zone
                ),
                kind: RentalOccupancyKind(raw: period.status),
                start: start,
                end: end
            )
        }
        .sorted { ($0.start ?? .distantFuture) < ($1.start ?? .distantFuture) }
    }
}

// MARK: - Availability

/// The platform's answer to „ist es frei?".
nonisolated struct RentalAvailability: Hashable, Sendable {
    /// Exactly the stretch this answer was given for. An answer that no
    /// longer belongs to what is on screen is worthless — and dangerous.
    let startDate: String
    let endDate: String
    let free: Bool
    /// What is shown above the button.
    let message: String
}

nonisolated extension RentalAvailabilityDto {
    func asAvailability(startDate: String, endDate: String) -> RentalAvailability {
        RentalAvailability(
            startDate: startDate,
            endDate: endDate,
            free: available,
            // The platform says `occupied` or nothing at all — deliberately
            // not who or why. So the sentence is ours, and it stays as
            // uninformative as the answer is.
            message: available
                ? "Der Zeitraum ist frei."
                : "Der Zeitraum ist schon vergeben. Such dir einen anderen aus."
        )
    }
}
