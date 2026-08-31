import Combine
import Foundation

/// One device, its taken periods, and the way to book it.
///
/// The important part of this model is what it does **not** do: it never
/// decides whether a stretch of days can be booked. It asks the platform and
/// shows the answer. `bookable` comes back over the wire; the button follows
/// it. Whoever puts that condition into the app has it twice, and the web and
/// the app start disagreeing exactly where it hurts
/// (`docs/mietplattform-in-den-apps.md`, AP 6).
///
/// The second thing it watches: an answer belongs to the stretch of days it
/// was given for. Changing a date drops it. Otherwise somebody asks for the
/// free weekend, moves the dates onto a taken one and books it on the
/// strength of a „yes" that was about something else.
final class RentalDeviceModel: ObservableObject {
    /// What the list already knew — shown immediately, so the page is never
    /// blank while the detail is being fetched.
    @Published private(set) var device: RentalDevice
    @Published private(set) var occupied: [RentalPeriod] = []
    @Published private(set) var loading = false

    @Published private(set) var from: Date
    @Published private(set) var to: Date
    @Published var note = ""

    /// The platform's answer for exactly the stretch above — `nil` as soon as
    /// a date moves.
    @Published private(set) var availability: RentalAvailability?
    @Published private(set) var checking = false
    @Published private(set) var booking = false
    @Published private(set) var trouble: RentalTrouble?
    /// The booking that just went through. Stays on screen until the page is
    /// left — it carries what happens next.
    @Published private(set) var confirmed: RentalBooking?

    private let source: RentalSource
    private let now: () -> Date

    init(device: RentalDevice, source: RentalSource, now: @escaping () -> Date = Date.init) {
        self.device = device
        self.source = source
        self.now = now
        let today = RentalDay.calendar().startOfDay(for: now())
        from = today
        to = today
    }

    /// Whether the booking button is live. Every part of this comes from the
    /// platform except „a request is already on its way".
    var canBook: Bool {
        device.bookable && availability?.bookable == true && !booking && confirmed == nil
    }

    /// What the day above the button says once the platform has answered.
    var answer: RentalAvailability? { availability }

    func setFrom(_ day: Date) {
        from = RentalDay.calendar().startOfDay(for: day)
        if to < from { to = from }
        availability = nil
    }

    func setTo(_ day: Date) {
        to = RentalDay.calendar().startOfDay(for: day)
        if to < from { from = to }
        availability = nil
    }

    /// Details and taken periods. Called once when the page opens.
    func load() async {
        if loading { return }
        loading = true
        defer { loading = false }
        do {
            let detail = try await source.item(device.id)
            if let refreshed = detail.item.asDevice() { device = refreshed }
            if !detail.blockedPeriods.isEmpty {
                occupied = detail.blockedPeriods.occupied(deviceId: device.id, now: now())
            } else {
                // The platform serves the taken periods of every device
                // publicly; that route exists today and needs no sign-in.
                occupied = try await source.occupied().occupied(deviceId: device.id, now: now())
            }
            trouble = nil
        } catch {
            // The device from the list is still on screen — a note above it
            // beats an empty page.
            trouble = RentalTrouble(error)
        }
    }

    /// „Ist es frei?" — asked of the platform, always.
    func check() async {
        if checking { return }
        checking = true
        defer { checking = false }
        do {
            let answer = try await source.availability(
                device.id, RentalDay.api(from), RentalDay.api(to)
            )
            availability = answer.asAvailability()
            trouble = nil
        } catch {
            availability = nil
            trouble = RentalTrouble(error)
        }
    }

    /// Books the stretch the platform just said yes to.
    func book() async {
        guard canBook else { return }
        booking = true
        defer { booking = false }
        do {
            let created = try await source.book(RentalBookingRequestDto(
                itemId: device.id,
                from: RentalDay.api(from),
                to: RentalDay.api(to),
                note: note.trimmingCharacters(in: .whitespacesAndNewlines)
            ))
            confirmed = created.asBooking()
            availability = nil
            trouble = nil
        } catch {
            // What was typed stays where it is; nobody fills in a booking
            // twice because the network hiccuped.
            trouble = RentalTrouble(error)
        }
    }

    /// After a fresh sign-in: forget the refusal so the page is usable again.
    func clearTrouble() {
        trouble = nil
    }
}
