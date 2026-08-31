import Foundation

/// What can go wrong on the way to the rental platform.
///
/// The two sign-in cases are the whole point of this enum. „You are not
/// signed in" and „your sign-in does not know the Maschinchenring yet" look
/// identical on the wire — both are a 401 — but they need opposite answers
/// from the person holding the phone. Telling them apart is possible because
/// the client knows whether it actually sent a token.
nonisolated enum RentalError: Error, Sendable, Equatable {
    case network(String)
    case serverError(status: Int)
    case unreadable
    case notFound
    /// The platform refused and said why. Its wording is passed on verbatim —
    /// the rule sits over there, and so does the sentence explaining it.
    case rejected(reason: String)
    /// Nobody is signed in.
    case signInRequired
    /// Somebody is signed in, but the token does not carry the rental
    /// platform as an audience: the device kept its token set across the
    /// update that added the scope (see `ANMELDE_SCOPES`). Only a fresh
    /// sign-in fixes that — and saying so is the whole job here.
    case signInOutdated

    /// The sentence shown to the person. German, like everything visible.
    var message: String {
        switch self {
        case .network:
            return "Der Maschinchenring ist gerade nicht erreichbar. Besteht eine Verbindung?"
        case .serverError(let status):
            return "Der Maschinchenring antwortet gerade nicht (\(status))."
        case .unreadable:
            return "Die Antwort des Maschinchenrings war nicht lesbar."
        case .notFound:
            return "Das gibt es dort nicht (mehr)."
        case .rejected(let reason):
            return reason
        case .signInRequired:
            return "Zum Buchen brauchst du deine Rössing-ID."
        case .signInOutdated:
            return "Deine Anmeldung kennt den Maschinchenring noch nicht. "
                + "Melde dich einmal neu an, dann geht es weiter."
        }
    }
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
/// has to.** Catalogue, detail, availability and the occupied periods are
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

    /// The catalogue. `search` is handed to the platform when it is set; the
    /// area also filters what it already holds, so looking around keeps
    /// working while the platform is away.
    func items(search: String? = nil) async throws -> [RentalItemDto] {
        var query: [String: String] = [:]
        if let search, !search.trimmingCharacters(in: .whitespaces).isEmpty {
            query[RentalRoutes.searchParameter] = search
        }
        let answer: RentalItemListDto = try await get(RentalRoutes.items, query: query)
        return answer.items
    }

    func item(id: String) async throws -> RentalItemDetailDto {
        try await get(RentalRoutes.item(id))
    }

    /// Asks the platform whether a stretch of days is free — and whether it
    /// may be booked. Both answers come from over there.
    func availability(itemId: String, from: String, to: String) async throws -> RentalAvailabilityDto {
        try await get(RentalRoutes.availability(itemId), query: [
            RentalRoutes.fromParameter: from,
            RentalRoutes.toParameter: to,
        ])
    }

    /// Occupied periods of every device, without any personal data. The only
    /// route in this area that is verified against the running platform.
    func occupiedPeriods() async throws -> [RentalPeriodDto] {
        try await get(RentalRoutes.occupied)
    }

    // MARK: Routes that need a token

    func myBookings() async throws -> [RentalBookingDto] {
        let answer: RentalBookingListDto = try await get(RentalRoutes.myBookings, withToken: true)
        return answer.bookings
    }

    func book(_ wish: RentalBookingRequestDto) async throws -> RentalBookingDto {
        let data = try await perform(request("POST", RentalRoutes.bookings,
                                             body: wish, withToken: true))
        return try decode(data)
    }

    func cancel(bookingId: String) async throws {
        _ = try await perform(request("POST", RentalRoutes.cancel(bookingId), withToken: true))
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
            // An error page instead of the payload (cache, portal, moved
            // route) must not pass as „nothing there".
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
            throw Self.error(
                status: http.statusCode,
                data: data,
                tokenSent: outgoing.value(forHTTPHeaderField: "Authorization") != nil
            )
        }
        return data
    }

    /// Status code to error — with the platform's own sentence wherever it
    /// sends one. A plain function, so it can be checked without a network.
    ///
    /// `tokenSent` is what makes the 401 readable: with a token, a 401 means
    /// the token was not accepted — on this platform that is almost always
    /// the missing audience from `ANMELDE_SCOPES`, and the way out is a fresh
    /// sign-in, not a sign-out.
    static func error(status: Int, data: Data, tokenSent: Bool) -> RentalError {
        let reason = (try? JSONDecoder().decode(RentalErrorDto.self, from: data))?.text ?? ""
        switch status {
        case 400, 403, 409, 422:
            return .rejected(reason: reason.isEmpty
                ? "Der Maschinchenring hat die Anfrage abgelehnt." : reason)
        case 401:
            return tokenSent ? .signInOutdated : .signInRequired
        case 404:
            return .notFound
        default:
            return .serverError(status: status)
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
