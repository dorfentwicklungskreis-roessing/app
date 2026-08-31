import Combine
import Foundation

/// „Meine Buchungen": what I asked for, what was confirmed, what I can still
/// withdraw.
///
/// This is one of the screens that needs a token, and therefore one of the
/// places where the stumbling block of the audience scope shows up: a device
/// that was signed in before the update carries a token the rental platform
/// does not accept. It answers `token_audience`, and the screen asks for a
/// fresh sign-in instead of showing an empty list. Browsing the catalogue is
/// untouched by all of this — those routes are public.
///
/// Whether a booking may be withdrawn is not decided here: `canCancel` comes
/// from the platform, the button follows it.
final class RentalBookingsModel: ObservableObject {
    @Published private(set) var bookings: [RentalBooking] = []
    @Published private(set) var loading = false
    @Published private(set) var trouble: RentalTrouble?
    /// Which booking is being withdrawn right now — one row at a time keeps
    /// the rest of the list usable.
    @Published private(set) var cancelling: Set<String> = []

    private var fetched = false
    private let source: RentalSource
    private let now: () -> Date

    init(source: RentalSource, now: @escaping () -> Date = Date.init) {
        self.source = source
        self.now = now
    }

    var empty: Bool { bookings.isEmpty && !loading && trouble == nil }

    /// The note above the list — and, when a list is already standing, the
    /// gentler wording: it may be out of date, it is not gone.
    var hint: String? {
        guard let trouble else { return nil }
        if bookings.isEmpty { return trouble.message }
        return "Gerade keine Verbindung zum Maschinchenring — die Liste ist "
            + "möglicherweise nicht mehr aktuell."
    }

    /// Whether the screen should offer a sign-in rather than „try again".
    /// Both sign-in cases end up here; the wording differs, the button is the
    /// same one.
    var needsSignIn: Bool { trouble?.wantsSignIn == true }

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
            let raw = try await source.myBookings()
            bookings = raw.asBookings(now: now())
            fetched = true
            trouble = nil
        } catch {
            trouble = RentalTrouble(error)
        }
    }

    /// Withdraws a booking. The platform decides whether that is allowed —
    /// the list is fetched again afterwards so its answer, not our guess,
    /// ends up on screen.
    func cancel(_ booking: RentalBooking) async {
        if cancelling.contains(booking.id) { return }
        cancelling.insert(booking.id)
        defer { cancelling.remove(booking.id) }
        do {
            try await source.cancel(booking.id)
            trouble = nil
            let raw = try await source.myBookings()
            bookings = raw.asBookings(now: now())
        } catch {
            // Nothing is removed from the list on a failure: a row that
            // vanishes while the platform still holds the booking is the one
            // mistake nobody notices until the device is gone.
            trouble = RentalTrouble(error)
        }
    }

    /// After a fresh sign-in the next attempt should start clean.
    func clearTrouble() {
        trouble = nil
    }
}
