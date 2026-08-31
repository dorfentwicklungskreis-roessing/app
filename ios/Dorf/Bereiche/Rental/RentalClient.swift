import Foundation

/// The error codes of the rental platform, one for one from
/// `docs/mieten-api.md`, section „Fehler".
///
/// They exist as a type because **the app branches on the code, never on the
/// message**: the wording may be improved over there at any time, the code
/// may not. Anything unknown lands on `unknown` and is shown with the
/// platform's own sentence — a new code must not turn into a silent nothing.
nonisolated enum RentalErrorCode: String, Sendable {
    case badRequest = "bad_request"
    case invalidPeriod = "invalid_period"
    case profileIncomplete = "profile_incomplete"
    case unauthorized
    case tokenAudience = "token_audience"
    case forbidden
    case notALender = "not_a_lender"
    case notFound = "not_found"
    case occupied
    case conflict
    case rateLimited = "rate_limited"
    case serverFault = "internal"
    case unknown

    init(raw: String) {
        self = RentalErrorCode(rawValue: raw) ?? .unknown
    }

    /// What we say when the platform sent no sentence of its own. Its own
    /// wording always wins — these are the fallbacks.
    var fallbackMessage: String {
        switch self {
        case .badRequest: return "Die Anfrage war unvollständig."
        case .invalidPeriod: return "Der Zeitraum stimmt nicht: Das Ende muss nach dem Anfang liegen."
        case .profileIncomplete: return "Für diesen Schritt fehlen noch Angaben in deinem Profil."
        case .unauthorized: return "Dafür brauchst du deine Rössing-ID."
        case .tokenAudience:
            return "Deine Anmeldung kennt den Maschinchenring noch nicht. "
                + "Melde dich einmal neu an, dann geht es weiter."
        case .forbidden: return "Das darfst du hier nicht."
        case .notALender: return "Das geht nur, wenn du als Verleiher freigeschaltet bist."
        case .notFound: return "Das gibt es dort nicht (mehr)."
        case .occupied: return "Der Zeitraum ist inzwischen belegt."
        case .conflict: return "Das geht in diesem Zustand nicht mehr."
        case .rateLimited: return "Das ging zu schnell hintereinander. Versuch es später noch einmal."
        case .serverFault: return "Der Maschinchenring hat gerade einen Fehler."
        case .unknown: return "Der Maschinchenring hat die Anfrage abgelehnt."
        }
    }
}

/// A refusal of the platform: what it answered, in the shape it answers in.
nonisolated struct RentalRefusal: Equatable, Sendable {
    let status: Int
    let code: RentalErrorCode
    /// German and meant for the screen — the platform's own sentence where
    /// it sent one.
    let message: String
    /// Only with `profile_incomplete`.
    let missingFields: [String]

    init(status: Int, code: RentalErrorCode, message: String = "", missingFields: [String] = []) {
        self.status = status
        self.code = code
        self.message = message.isEmpty ? code.fallbackMessage : message
        self.missingFields = missingFields
    }
}

/// What can go wrong on the way to the rental platform.
///
/// The two sign-in cases are the reason this is not just a status code. „You
/// are not signed in" and „your sign-in does not know the Maschinchenring
/// yet" are both a 401 on the wire, but they need opposite answers from the
/// person holding the phone — and the platform tells them apart for us with
/// `unauthorized` versus `token_audience`.
nonisolated enum RentalError: Error, Sendable, Equatable {
    case network(String)
    /// Something arrived that was not the payload — an error page, a portal,
    /// a moved route. It must never pass as „nothing there".
    case unreadable
    case refused(RentalRefusal)

    /// The sentence shown to the person. German, like everything visible.
    var message: String {
        switch self {
        case .network:
            return "Der Maschinchenring ist gerade nicht erreichbar. Besteht eine Verbindung?"
        case .unreadable:
            return "Die Antwort des Maschinchenrings war nicht lesbar."
        case .refused(let refusal):
            return refusal.message
        }
    }

    var code: RentalErrorCode? {
        if case .refused(let refusal) = self { return refusal.code }
        return nil
    }

    /// The device kept its token set across the update that added the
    /// audience scope. Only a fresh sign-in fixes that — see `ANMELDE_SCOPES`.
    var needsFreshSignIn: Bool { code == .tokenAudience }

    /// Nobody is signed in, or the token has expired: the ordinary sign-in.
    var needsSignIn: Bool { code == .unauthorized }

    /// Which fields the platform is missing before it lets this through.
    var missingFields: [String] {
        if case .refused(let refusal) = self { return refusal.missingFields }
        return []
    }

    /// Nobody is signed in on this device at all — built here rather than at
    /// three call sites, so it reads like every other refusal.
    static let signInRequired = RentalError.refused(RentalRefusal(
        status: 401, code: .unauthorized,
        message: "Zum Buchen brauchst du deine Rössing-ID."
    ))
}

/// The way to the rental platform („Maschinchenring").
///
/// A client of its own, deliberately: `mieten.…` is **not** the village
/// backend, so the rule „exactly one way to the backend: `DorfApi`" does not
/// cover it (`CLAUDE.md`; `docs/mietplattform-in-den-apps.md`, AP 4). The
/// events area has the same shape for the same reason — a second server, a
/// small client of its own, `URLSession` and `Codable` and nothing else.
///
/// The one difference to the events: a token goes along, but **only where it
/// has to.** Catalogue, detail, search, sets, availability and occupancy are
/// public over there and are fetched without an `Authorization` header. That
/// is not thrift, it is the difference between a working and a broken area:
/// a device whose token predates the audience scope would otherwise get a 401
/// on the catalogue as well, and looking around would break for exactly the
/// people who are already signed in.
nonisolated final class RentalClient: Sendable {
    private let base: URL
    private let session: URLSession
    private let tokenProvider: @Sendable () async -> Tokenlage

    init(base: URL = Konfiguration.rentalBaseUrl,
         session: URLSession = .rentalSession,
         tokenProvider: @escaping @Sendable () async -> Tokenlage) {
        self.base = base
        self.session = session
        self.tokenProvider = tokenProvider
    }

    // MARK: Public routes — no token goes out here

    /// Route 1: all active devices, sorted by name over there. No paging, and
    /// none planned.
    func items() async throws -> [RentalItemDto] {
        let answer: RentalItemsDto = try await get(RentalRoutes.items)
        return answer.items
    }

    /// Route 2: one device with all its pictures.
    func item(id: String) async throws -> RentalItemDto {
        let answer: RentalItemEnvelopeDto = try await get(RentalRoutes.item(id))
        return answer.item
    }

    /// Route 3: the hybrid search of the platform — semantic and literal,
    /// weighted over there. **The order that comes back is the answer**; the
    /// app does not sort it again.
    func search(_ text: String, tags: [String] = [], limit: Int? = nil) async throws -> [RentalItemDto] {
        var query = [RentalRoutes.queryText: text]
        if !tags.isEmpty { query[RentalRoutes.queryTags] = tags.joined(separator: ",") }
        if let limit { query[RentalRoutes.queryLimit] = String(limit) }
        let answer: RentalSearchDto = try await get(RentalRoutes.search, query: query)
        return answer.results
    }

    /// Route 4: the sets.
    func sets() async throws -> [RentalSetDto] {
        let answer: RentalSetsDto = try await get(RentalRoutes.sets)
        return answer.sets
    }

    /// Route 5: „is it free from … to …?" — asked and answered over there,
    /// even though the calendar from route 6 is already on screen. Exactly
    /// one of `deviceId` and `setId`.
    func availability(deviceId: String? = nil, setId: String? = nil,
                      startDate: String, endDate: String) async throws -> RentalAvailabilityDto {
        var query = [
            RentalRoutes.queryStartDate: startDate,
            RentalRoutes.queryEndDate: endDate,
        ]
        if let deviceId { query[RentalRoutes.queryDeviceId] = deviceId }
        if let setId { query[RentalRoutes.querySetId] = setId }
        return try await get(RentalRoutes.availability, query: query)
    }

    /// Route 6: taken stretches of days, without a single personal datum.
    /// Without a device it is the whole village's calendar.
    func occupancy(deviceId: String? = nil, setId: String? = nil) async throws -> [RentalPeriodDto] {
        var query: [String: String] = [:]
        if let deviceId { query[RentalRoutes.queryDeviceId] = deviceId }
        if let setId { query[RentalRoutes.querySetId] = setId }
        let answer: RentalOccupancyDto = try await get(RentalRoutes.occupancy, query: query)
        return answer.periods
    }

    // MARK: Routes that need a token

    /// Route 7. The first call with a new Rössing-ID quietly creates the
    /// account over there; the app does nothing for that.
    func profile() async throws -> RentalProfileDto {
        let answer: RentalProfileEnvelopeDto = try await get(RentalRoutes.me, withToken: true)
        return answer.profile
    }

    /// Route 8. Only what is sent changes.
    func updateProfile(_ patch: RentalProfilePatchDto) async throws -> RentalProfileDto {
        let data = try await perform(request("PATCH", RentalRoutes.me, body: patch, withToken: true))
        let answer: RentalProfileEnvelopeDto = try decode(data)
        return answer.profile
    }

    /// Route 9. An acknowledgement, not a permission — a person decides that,
    /// in the web version.
    func requestLender() async throws -> RentalLenderRequestDto {
        let data = try await perform(request("POST", RentalRoutes.lenderRequest, withToken: true))
        return try decode(data)
    }

    /// Route 10.
    func myBookings() async throws -> [RentalBookingDto] {
        let answer: RentalBookingsDto = try await get(RentalRoutes.myBookings, withToken: true)
        return answer.bookings
    }

    /// Route 11. Creates a request, confirms nothing.
    func book(_ wish: RentalBookingRequestDto) async throws -> RentalBookingDto {
        let data = try await perform(request("POST", RentalRoutes.bookings,
                                             body: wish, withToken: true))
        let answer: RentalBookingEnvelopeDto = try decode(data)
        return answer.booking
    }

    /// Route 12.
    @discardableResult
    func cancel(bookingId: String) async throws -> String {
        let data = try await perform(request("POST", RentalRoutes.cancel(bookingId), withToken: true))
        let answer: RentalStatusDto = try decode(data)
        return answer.status
    }

    // MARK: The lender's side

    /// Route 13. Somebody without devices gets an empty list, not an error.
    func ownerBookings() async throws -> [RentalOwnerBookingDto] {
        let answer: RentalOwnerBookingsDto = try await get(RentalRoutes.ownerBookings, withToken: true)
        return answer.bookings
    }

    /// Route 14.
    @discardableResult
    func approve(bookingId: String) async throws -> String {
        let data = try await perform(request("POST", RentalRoutes.approve(bookingId), withToken: true))
        let answer: RentalStatusDto = try decode(data)
        return answer.status
    }

    /// Route 15. No reason is recorded.
    @discardableResult
    func reject(bookingId: String) async throws -> String {
        let data = try await perform(request("POST", RentalRoutes.reject(bookingId), withToken: true))
        let answer: RentalStatusDto = try decode(data)
        return answer.status
    }

    /// Route 16. The disabled ones are in here too — they are missing from
    /// route 1.
    func ownerItems() async throws -> [RentalItemDto] {
        let answer: RentalItemsDto = try await get(RentalRoutes.ownerItems, withToken: true)
        return answer.items
    }

    /// Route 17.
    func blocks() async throws -> [RentalBlockDto] {
        let answer: RentalBlocksDto = try await get(RentalRoutes.ownerBlocks, withToken: true)
        return answer.blocks
    }

    /// Route 18. An existing booking is never pushed aside — the platform
    /// answers `occupied` instead.
    func addBlock(_ wish: RentalBlockRequestDto) async throws -> RentalBlockDto {
        let data = try await perform(request("POST", RentalRoutes.ownerBlocks,
                                             body: wish, withToken: true))
        let answer: RentalBlockEnvelopeDto = try decode(data)
        return answer.block
    }

    /// Route 19.
    func removeBlock(id: String) async throws {
        _ = try await perform(request("DELETE", RentalRoutes.ownerBlock(id), withToken: true))
    }

    // MARK: Transport

    private func address(_ path: String, query: [String: String]) -> URL {
        var url = base.appending(path: path)
        if !query.isEmpty {
            url.append(queryItems: query.sorted { $0.key < $1.key }
                .map { URLQueryItem(name: $0.key, value: $0.value) })
        }
        return url
    }

    /// Builds the request — a step of its own so a test can see what would go
    /// out (path, method, body, and whether a token is attached) without a
    /// network anywhere near it.
    func request(_ method: String, _ path: String,
                 query: [String: String] = [:],
                 body: (any Encodable)? = nil,
                 withToken: Bool) async throws -> URLRequest {
        var outgoing = URLRequest(url: address(path, query: query))
        outgoing.httpMethod = method
        outgoing.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            outgoing.setValue("application/json", forHTTPHeaderField: "Content-Type")
            outgoing.httpBody = try? JSONEncoder().encode(body)
        }
        guard withToken else { return outgoing }

        switch await tokenProvider() {
        case .token(let token):
            outgoing.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        case .abgemeldet:
            throw RentalError.signInRequired
        case .nichtErreichbar:
            // The sign-in holds, it just could not be refreshed. Sending
            // nothing would come back as 401 and we would tell somebody their
            // sign-in is stale when it is the network that is stale.
            throw RentalError.network("Die Anmeldung ließ sich gerade nicht erneuern.")
        }
        return outgoing
    }

    private func get<T: Decodable>(_ path: String, query: [String: String] = [:],
                                   withToken: Bool = false) async throws -> T {
        let data = try await perform(request("GET", path, query: query, withToken: withToken))
        return try decode(data)
    }

    private func decode<T: Decodable>(_ data: Data) throws -> T {
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw RentalError.unreadable
        }
    }

    private func perform(_ outgoing: URLRequest) async throws -> Data {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: outgoing)
        } catch {
            throw RentalError.network(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else {
            throw RentalError.network("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            throw Self.error(status: http.statusCode, data: data)
        }
        return data
    }

    /// The platform's refusal, read the way the contract writes it: the
    /// **code** decides, the message is only shown. A plain function, so it
    /// can be checked without a network.
    ///
    /// When nothing readable came back — a gateway page, an empty body — the
    /// status stands in for the code. That keeps a 401 from a proxy from
    /// looking like a fresh-sign-in case that it is not.
    static func error(status: Int, data: Data) -> RentalError {
        guard let dto = try? JSONDecoder().decode(RentalErrorDto.self, from: data),
              !dto.code.isEmpty || !dto.message.isEmpty
        else {
            return RentalError.refused(RentalRefusal(
                status: status, code: codeForStatus(status)
            ))
        }
        let code = dto.code.isEmpty ? codeForStatus(status) : RentalErrorCode(raw: dto.code)
        return .refused(RentalRefusal(
            status: status, code: code, message: dto.message, missingFields: dto.missingFields
        ))
    }

    /// A stand-in for a missing code. Deliberately coarse: it only has to
    /// carry a status into something the screens can react to.
    private static func codeForStatus(_ status: Int) -> RentalErrorCode {
        switch status {
        case 400: return .badRequest
        case 401: return .unauthorized
        case 403: return .forbidden
        case 404: return .notFound
        case 409: return .conflict
        case 429: return .rateLimited
        case 500 ..< 600: return .serverFault
        default: return .unknown
        }
    }
}

nonisolated extension URLSession {
    /// A session of its own for the rental platform — separate from the
    /// backend's, so nothing that only concerns the village API ever rides
    /// along. Same deadlines as everywhere else in this app.
    static let rentalSession: URLSession = {
        let k = URLSessionConfiguration.default
        k.timeoutIntervalForRequest = 20
        k.timeoutIntervalForResource = 30
        k.waitsForConnectivity = false
        return URLSession(configuration: k)
    }()
}
