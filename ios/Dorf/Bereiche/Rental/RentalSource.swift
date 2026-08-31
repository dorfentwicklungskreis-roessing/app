import Foundation

/// Where the rental area gets its data from.
///
/// A bundle of closures instead of a protocol — the same shape as
/// `VergabeQuelle`: the area needs a handful of calls, and a test fills them
/// in itself. That is what keeps every test in this area away from the
/// network without a single mock framework.
nonisolated struct RentalSource {
    // Public, no token (routes 1 to 6).
    var items: @MainActor () async throws -> [RentalItemDto]
    var item: @MainActor (String) async throws -> RentalItemDto
    var search: @MainActor (String) async throws -> [RentalItemDto]
    var sets: @MainActor () async throws -> [RentalSetDto]
    var availability: @MainActor (_ deviceId: String, _ startDate: String, _ endDate: String)
        async throws -> RentalAvailabilityDto
    var occupancy: @MainActor (_ deviceId: String?) async throws -> [RentalPeriodDto]

    // With a token (routes 7 to 12).
    var profile: @MainActor () async throws -> RentalProfileDto
    var updateProfile: @MainActor (RentalProfilePatchDto) async throws -> RentalProfileDto
    var requestLender: @MainActor () async throws -> RentalLenderRequestDto
    var myBookings: @MainActor () async throws -> [RentalBookingDto]
    var book: @MainActor (RentalBookingRequestDto) async throws -> RentalBookingDto
    var cancel: @MainActor (String) async throws -> Void

    // The lender's side (routes 13 to 19).
    var ownerBookings: @MainActor () async throws -> [RentalOwnerBookingDto]
    var approve: @MainActor (String) async throws -> Void
    var reject: @MainActor (String) async throws -> Void
    var ownerItems: @MainActor () async throws -> [RentalItemDto]
    var blocks: @MainActor () async throws -> [RentalBlockDto]
    var addBlock: @MainActor (RentalBlockRequestDto) async throws -> RentalBlockDto
    var removeBlock: @MainActor (String) async throws -> Void

    /// The one place that ties the models to the actual platform.
    static func from(_ client: RentalClient) -> RentalSource {
        RentalSource(
            items: { try await client.items() },
            item: { try await client.item(id: $0) },
            search: { try await client.search($0, limit: 20) },
            sets: { try await client.sets() },
            availability: {
                try await client.availability(deviceId: $0, startDate: $1, endDate: $2)
            },
            occupancy: { try await client.occupancy(deviceId: $0) },
            profile: { try await client.profile() },
            updateProfile: { try await client.updateProfile($0) },
            requestLender: { try await client.requestLender() },
            myBookings: { try await client.myBookings() },
            book: { try await client.book($0) },
            cancel: { try await client.cancel(bookingId: $0) },
            ownerBookings: { try await client.ownerBookings() },
            approve: { try await client.approve(bookingId: $0) },
            reject: { try await client.reject(bookingId: $0) },
            ownerItems: { try await client.ownerItems() },
            blocks: { try await client.blocks() },
            addBlock: { try await client.addBlock($0) },
            removeBlock: { try await client.removeBlock(id: $0) }
        )
    }
}

/// How the screens of this area report trouble.
///
/// One type for every model, because they all answer the same questions: what
/// does the person read, does the list stay standing, and is this one of the
/// two cases where a sign-in — a fresh one or an ordinary one — is the way
/// out rather than „try again".
nonisolated struct RentalTrouble: Equatable, Sendable {
    let message: String
    /// The token was refused although it was sent: the audience from
    /// `ANMELDE_SCOPES` is missing on this device, because it was already
    /// signed in when that scope arrived. Only a fresh sign-in helps, and the
    /// screen offers exactly that (`docs/mieten-api.md`, `token_audience`).
    let needsFreshSignIn: Bool
    /// Nobody is signed in; the screen offers the ordinary sign-in.
    let needsSignIn: Bool
    /// The platform's error code, where it sent one — for the screens that
    /// react to a particular one (`profile_incomplete`, `occupied`).
    let code: RentalErrorCode?
    /// Only with `profile_incomplete`: what the platform is still missing.
    let missingFields: [String]

    init(_ error: Error) {
        let rental = error as? RentalError ?? .network(error.localizedDescription)
        message = rental.message
        needsFreshSignIn = rental.needsFreshSignIn
        needsSignIn = rental.needsSignIn
        code = rental.code
        missingFields = rental.missingFields
    }

    /// Whether a sign-in of some kind is what this needs.
    var wantsSignIn: Bool { needsSignIn || needsFreshSignIn }
}
