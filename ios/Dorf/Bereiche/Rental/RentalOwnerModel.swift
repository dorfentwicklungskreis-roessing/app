import Combine
import Foundation

/// The lender's side: who asked for my devices, which devices are mine, and
/// which stretches I keep for myself.
///
/// It only exists for somebody the platform has approved (`lenderStatus ==
/// "approved"`); the app does not work that out, it reads it from route 7.
/// Everything else is the same habit as in the rest of the area: `canDecide`
/// and `canCancel` come over the wire, the buttons follow them, and a failed
/// call never quietly removes a row.
///
/// **Adding or changing devices is not here, and not anywhere in the app.**
/// That happens in the chat and the web version of the platform, and it stays
/// there (`docs/mieten-api.md`, route 16). The screen says so and offers the
/// way over.
final class RentalOwnerModel: ObservableObject {
    @Published private(set) var bookings: [RentalOwnerBooking] = []
    @Published private(set) var devices: [RentalDevice] = []
    @Published private(set) var blocks: [RentalBlock] = []
    @Published private(set) var loading = false
    @Published private(set) var trouble: RentalTrouble?
    /// Which booking is being decided right now.
    @Published private(set) var deciding: Set<String> = []
    @Published private(set) var blocking = false

    private var fetched = false
    private let source: RentalSource
    private let now: () -> Date

    init(source: RentalSource, now: @escaping () -> Date = Date.init) {
        self.source = source
        self.now = now
    }

    var needsSignIn: Bool { trouble?.wantsSignIn == true }

    var hint: String? {
        guard let trouble else { return nil }
        if bookings.isEmpty && devices.isEmpty { return trouble.message }
        return "Gerade keine Verbindung zum Maschinchenring — die Listen sind "
            + "möglicherweise nicht mehr aktuell."
    }

    /// Nothing to decide, nothing lent out — an ordinary state, not an error.
    var empty: Bool { bookings.isEmpty && devices.isEmpty && !loading && trouble == nil }

    /// How many requests wait for a yes or a no. The number the screen is
    /// opened for.
    var waiting: Int { bookings.filter(\.canDecide).count }

    func load() async {
        if fetched { return }
        await fetch()
    }

    func refresh() async {
        await fetch()
    }

    private func fetch() async {
        if loading { return }
        loading = true
        defer { loading = false }
        do {
            bookings = try await source.ownerBookings().asOwnerBookings(now: now())
            devices = try await source.ownerItems().asDevices()
            blocks = try await source.blocks().asBlocks(now: now())
            fetched = true
            trouble = nil
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    /// Confirms a booking. From then on the person who asked sees the pickup
    /// address — that is why this is a decision and not a toggle.
    func approve(_ booking: RentalOwnerBooking) async {
        await decide(booking) { try await self.source.approve(booking.id) }
    }

    /// Turns a booking down. No reason is recorded; the platform sends the
    /// cancellation e-mail.
    func reject(_ booking: RentalOwnerBooking) async {
        await decide(booking) { try await self.source.reject(booking.id) }
    }

    /// Withdraws a booking on one's own device — allowed for both sides while
    /// it is `pending` or `approved`.
    func cancel(_ booking: RentalOwnerBooking) async {
        await decide(booking) { try await self.source.cancel(booking.id) }
    }

    private func decide(_ booking: RentalOwnerBooking,
                        _ step: @MainActor () async throws -> Void) async {
        if deciding.contains(booking.id) { return }
        deciding.insert(booking.id)
        defer { deciding.remove(booking.id) }
        do {
            try await step()
            trouble = nil
            // The platform's list is the truth about what just happened.
            bookings = try await source.ownerBookings().asOwnerBookings(now: now())
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    /// Blocks a stretch on one's own device. An existing booking is never
    /// pushed aside — the platform answers `occupied`, and whoever wants the
    /// days anyway cancels the booking first.
    ///
    /// `lastDay` is the last day the device is needed; the platform wants the
    /// day after, like everywhere in this area.
    @discardableResult
    func block(deviceId: String, firstDay: Date, lastDay: Date, reason: String) async -> Bool {
        if blocking { return false }
        blocking = true
        defer { blocking = false }
        do {
            _ = try await source.addBlock(RentalBlockRequestDto(
                deviceId: deviceId,
                startDate: RentalDay.api(firstDay),
                endDate: RentalDay.api(RentalDay.nextDay(lastDay)),
                reason: reason.rentalNonEmpty
            ))
            blocks = try await source.blocks().asBlocks(now: now())
            trouble = nil
            return true
        } catch {
            trouble = RentalTrouble(error)
            return false
        }
    }

    /// Lifts one of one's own blocks.
    func removeBlock(_ block: RentalBlock) async {
        do {
            try await source.removeBlock(block.id)
            blocks = try await source.blocks().asBlocks(now: now())
            trouble = nil
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    func clearTrouble() {
        trouble = nil
    }
}
