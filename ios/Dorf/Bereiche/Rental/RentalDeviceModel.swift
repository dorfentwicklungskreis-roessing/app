import Combine
import Foundation

/// One device, its taken periods, and the way to book it.
///
/// The important part of this model is what it does **not** do: it never
/// decides whether a stretch of days can be booked. It asks the platform
/// (route 5) and shows the answer; the button follows it. Whoever puts that
/// condition into the app has it twice, and web and app start disagreeing
/// exactly where it hurts (`docs/mieten-api.md`, „Was die App nicht
/// entscheidet").
///
/// The second thing it watches: an answer belongs to the stretch of days it
/// was given for. Changing a date drops it. Otherwise somebody asks for the
/// free weekend, moves the dates onto a taken one and books it on the
/// strength of a „yes" that was about something else.
///
/// The dates on screen are the days somebody **has** the thing — first and
/// last. The platform wants the day it comes back, which is one day later
/// (`docs/mieten-api.md`, „Zeiträume"). That conversion happens here, in one
/// place, and nowhere else.
final class RentalDeviceModel: ObservableObject {
    /// What the list already knew — shown immediately, so the page is never
    /// blank while the detail is being fetched.
    @Published private(set) var device: RentalDevice
    @Published private(set) var occupied: [RentalPeriod] = []
    @Published private(set) var loading = false

    /// First and last day of the loan, as a person picks them.
    @Published private(set) var firstDay: Date
    @Published private(set) var lastDay: Date
    @Published var notes = ""

    /// The platform's answer for exactly the stretch above — `nil` as soon as
    /// a date moves.
    @Published private(set) var availability: RentalAvailability?
    @Published private(set) var checking = false
    @Published private(set) var booking = false
    @Published private(set) var trouble: RentalTrouble?
    /// The booking that just went through. Stays on screen until the page is
    /// left — it carries what happens next.
    @Published private(set) var confirmed: RentalBooking?
    /// The platform is missing something in the profile before it takes a
    /// booking. Its list, in German — the screen leads on to the profile.
    @Published private(set) var missingProfileFields: [String] = []

    private let source: RentalSource
    private let now: () -> Date

    init(device: RentalDevice, source: RentalSource, now: @escaping () -> Date = Date.init) {
        self.device = device
        self.source = source
        self.now = now
        let today = RentalDay.calendar().startOfDay(for: now())
        firstDay = today
        lastDay = today
    }

    /// What goes out as `startDate`.
    var startDate: String { RentalDay.api(firstDay) }
    /// What goes out as `endDate` — the return day, one after the last day of
    /// the loan.
    var endDate: String { RentalDay.api(RentalDay.nextDay(lastDay)) }

    /// Whether the booking button is live. Everything in here comes from the
    /// platform except „a request is already on its way".
    var canBook: Bool {
        guard let answer = availability, !booking, confirmed == nil else { return false }
        // An answer only counts for the stretch it was given for.
        return answer.free && answer.startDate == startDate && answer.endDate == endDate
    }

    /// Whether the answer on screen still belongs to the dates on screen.
    var answer: RentalAvailability? {
        guard let availability,
              availability.startDate == startDate, availability.endDate == endDate
        else { return nil }
        return availability
    }

    func setFirstDay(_ day: Date) {
        firstDay = RentalDay.calendar().startOfDay(for: day)
        if lastDay < firstDay { lastDay = firstDay }
        availability = nil
    }

    func setLastDay(_ day: Date) {
        lastDay = RentalDay.calendar().startOfDay(for: day)
        if lastDay < firstDay { firstDay = lastDay }
        availability = nil
    }

    /// Details and taken periods. Called once when the page opens.
    func load() async {
        if loading { return }
        loading = true
        defer { loading = false }
        do {
            let detail = try await source.item(device.id)
            if let refreshed = detail.asDevice() { device = refreshed }
            trouble = nil
        } catch {
            // The device from the list is still on screen — a note above it
            // beats an empty page.
            trouble = RentalTrouble(error)
        }
        do {
            // Route 6 with a device also carries the lender's own blocks, and
            // it needs no sign-in.
            occupied = try await source.occupancy(device.id).occupied(
                deviceId: device.id, now: now()
            )
        } catch {
            if trouble == nil { trouble = RentalTrouble(error) }
        }
    }

    /// „Ist es frei?" — asked of the platform, always.
    func check() async {
        if checking { return }
        checking = true
        defer { checking = false }
        let wantedStart = startDate
        let wantedEnd = endDate
        do {
            let answer = try await source.availability(device.id, wantedStart, wantedEnd)
            availability = answer.asAvailability(startDate: wantedStart, endDate: wantedEnd)
            trouble = nil
        } catch {
            availability = nil
            trouble = RentalTrouble(error)
        }
    }

    /// Books the stretch the platform just said yes to.
    ///
    /// Name and telephone are deliberately **not** sent: the platform takes
    /// them from the profile, so nobody types their own name again. Only if
    /// they are missing there too does it refuse — and then the screen leads
    /// to the profile instead of guessing.
    func book() async {
        guard canBook else { return }
        booking = true
        defer { booking = false }
        missingProfileFields = []
        do {
            let created = try await source.book(RentalBookingRequestDto(
                deviceId: device.id,
                startDate: startDate,
                endDate: endDate,
                notes: notes.trimmingCharacters(in: .whitespacesAndNewlines).rentalNonEmpty
            ))
            confirmed = created.asBooking()
            availability = nil
            trouble = nil
        } catch {
            let bad = RentalTrouble(error)
            trouble = bad
            if bad.code == .profileIncomplete {
                missingProfileFields = RentalFieldNames.labels(bad.missingFields)
            }
            if bad.code == .occupied {
                // Between drawing the calendar and tapping, a minute can pass
                // and somebody else can be quicker. That is a normal race, not
                // a crash: the answer is dropped and the calendar refetched.
                availability = nil
                await reloadOccupancy()
            }
        }
    }

    /// After a fresh sign-in, or after the profile was completed: forget the
    /// refusal so the page is usable again.
    func clearTrouble() {
        trouble = nil
        missingProfileFields = []
    }

    private func reloadOccupancy() async {
        if let periods = try? await source.occupancy(device.id) {
            occupied = periods.occupied(deviceId: device.id, now: now())
        }
    }
}
