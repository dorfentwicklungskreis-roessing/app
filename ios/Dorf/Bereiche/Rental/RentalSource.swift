import Foundation

/// Where the rental area gets its data from.
///
/// A bundle of closures instead of a protocol — the same shape as
/// `VergabeQuelle`: the area needs a handful of calls, and a test fills them
/// in itself. That is what keeps every test in this area away from the
/// network without a single mock framework.
nonisolated struct RentalSource {
    var items: @MainActor () async throws -> [RentalItemDto]
    var item: @MainActor (String) async throws -> RentalItemDetailDto
    var occupied: @MainActor () async throws -> [RentalPeriodDto]
    var availability: @MainActor (_ itemId: String, _ from: String, _ to: String) async throws
        -> RentalAvailabilityDto
    var book: @MainActor (RentalBookingRequestDto) async throws -> RentalBookingDto
    var myBookings: @MainActor () async throws -> [RentalBookingDto]
    var cancel: @MainActor (String) async throws -> Void

    /// The one place that ties the models to the actual platform.
    static func from(_ client: RentalClient) -> RentalSource {
        RentalSource(
            items: { try await client.items() },
            item: { try await client.item(id: $0) },
            occupied: { try await client.occupiedPeriods() },
            availability: { try await client.availability(itemId: $0, from: $1, to: $2) },
            book: { try await client.book($0) },
            myBookings: { try await client.myBookings() },
            cancel: { try await client.cancel(bookingId: $0) }
        )
    }
}

/// How the screens of this area report trouble.
///
/// One type for all three models, because they all answer the same three
/// questions: what does the person read, does the list stay standing, and is
/// this the one case where a fresh sign-in is the way out.
nonisolated struct RentalTrouble: Equatable, Sendable {
    let message: String
    /// The token was refused although it was sent — the audience from
    /// `ANMELDE_SCOPES` is missing on this device (see `RentalError`). Only a
    /// fresh sign-in helps, and the screen offers exactly that.
    let needsFreshSignIn: Bool
    /// Nobody is signed in; the screen offers the ordinary sign-in.
    let needsSignIn: Bool

    init(_ error: Error) {
        let rental = error as? RentalError ?? .network(error.localizedDescription)
        message = rental.message
        needsFreshSignIn = rental == .signInOutdated
        needsSignIn = rental == .signInRequired
    }
}
