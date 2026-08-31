import Combine
import Foundation

/// Where the Traeger area gets its data from.
///
/// A bundle of closures instead of a protocol — the same shape as
/// `OrteQuelle` and `VergabeQuelle`: the area needs a handful of calls, and
/// a test fills them itself. No test goes to the network, and `DorfApi`
/// stays the only way to the backend.
struct TraegerSource {
    var list: @MainActor () async throws -> [Traeger]
    var detail: @MainActor (Int64) async throws -> Traeger
    /// Traeger id and the reason the person wrote.
    var join: @MainActor (Int64, String) async throws -> Beitritt
    var requests: @MainActor (Int64) async throws -> [Beitritt]
    /// Request id, "erteilt" or "abgelehnt".
    var decide: @MainActor (Int64, String) async throws -> Beitritt
    /// Traeger id and the person's identifier.
    var addMember: @MainActor (Int64, String) async throws -> Beitritt
    var myRequests: @MainActor () async throws -> [Beitritt]
    /// The village directory — needed to take somebody in without a request
    /// of their own; it is the only place an identifier can come from.
    var villagers: @MainActor () async throws -> [Dorfbewohner] = { [] }

    static func of(_ api: DorfApi) -> TraegerSource {
        TraegerSource(
            list: { try await api.traegerListe() },
            detail: { try await api.traeger(id: $0) },
            join: { try await api.beitrittBeantragen(traeger: $0, begruendung: $1) },
            requests: { try await api.beitritte(traeger: $0, status: "") },
            decide: { try await api.beitrittEntscheiden(id: $0, status: $1) },
            addMember: { try await api.mitgliedAufnehmen(traeger: $0, userSub: $1) },
            myRequests: { try await api.meineBeitritte() },
            villagers: { try await api.dorfbewohner().members }
        )
    }
}

/// The state of the Traeger area: which associations and working groups
/// there are, whether I belong to them, and what is currently running.
///
/// As everywhere in this app: **the last state stays put.** If the network
/// drops, the list is not emptied but gets a notice above it — "there are no
/// associations" would be a false statement in a dead spot.
///
/// And: nothing here is re-decided. Whether joining is possible, whether the
/// buttons for deciding appear at all, whether a Traeger shows up — all of
/// that arrives answered from the server. What this class does is remember
/// the answer and say what is currently being written.
final class TraegerModel: ObservableObject {
    private let source: TraegerSource

    @Published private(set) var all: [Traeger] = []
    /// My own requests across all Traeger — the answer to "did I ask
    /// somewhere and is it still pending?".
    @Published private(set) var mine: [Beitritt] = []
    /// Requests per Traeger, for those who administer it.
    @Published private(set) var requests: [Int64: [Beitritt]] = [:]
    /// The village directory, loaded only when somebody wants to take a
    /// person in directly.
    @Published private(set) var villagers: [Dorfbewohner] = []
    @Published private(set) var loading = false
    /// Whether a fetch ever completed — before that the list shows a spinner
    /// instead of "no associations yet".
    @Published private(set) var everLoaded = false
    /// The last fetch failed, in the backend's own words.
    @Published private(set) var notice: String?
    /// A genuinely rejected write — shown as an alert with an OK button.
    /// This is where a 502/503 from the Rössing-ID lands, and the sentence
    /// is the server's: an app that reports "taken in" while the door stays
    /// shut would be worse than no app.
    @Published private(set) var error: String?
    /// Short feedback after a successful write.
    @Published private(set) var confirmation: String?
    /// Traeger currently being written to (join, take somebody in).
    @Published private(set) var busy: Set<Int64> = []
    /// Requests currently being decided.
    @Published private(set) var busyRequests: Set<Int64> = []

    private var fading: Task<Void, Never>?

    init(source: TraegerSource) { self.source = source }

    convenience init(api: DorfApi) { self.init(source: .of(api)) }

    // MARK: Views on the state

    func traeger(id: Int64) -> Traeger? { all.first { $0.id == id } }

    /// Whether the server offered this Traeger to this person at all. The
    /// place detail asks before it offers a way there — not as a second
    /// visibility rule, but because the server's own directory is the only
    /// honest answer to "is there anything to see behind this?".
    func inDirectory(_ id: Int64) -> Bool { id != 0 && traeger(id: id) != nil }

    /// Associations: everything without a roof — plus anything whose roof
    /// this person cannot see, because otherwise it would fall out of the
    /// directory entirely.
    var roots: [Traeger] {
        let sichtbar = Set(all.map(\.id))
        return all
            .filter { $0.parentId == 0 || !sichtbar.contains($0.parentId) }
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    /// The working groups under an association. Exactly one level — deeper
    /// nesting does not exist (see `model.Traeger.ParentID`).
    func children(of id: Int64) -> [Traeger] {
        all.filter { $0.parentId == id }
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    /// How many undecided requests are waiting for me, across everything I
    /// administer. The start page shows this — somebody is waiting.
    var openRequestsForMe: Int { all.reduce(0) { $0 + $1.offeneBeitritte } }

    func isBusy(traeger id: Int64) -> Bool { busy.contains(id) }

    func isBusy(request id: Int64) -> Bool { busyRequests.contains(id) }

    // MARK: Loading

    func load() async {
        loading = true
        defer {
            loading = false
            everLoaded = true
        }
        do {
            all = try await source.list()
            notice = nil
        } catch let fehler as DorfFehler {
            notice = fehler.klartext
        } catch {
            notice = Self.networkNotice
        }
        // The own requests are a second call and must not cost the list: a
        // Traeger directory without them is still useful.
        if let eigene = try? await source.myRequests() { mine = eigene }
    }

    /// Fetches one Traeger anew and puts it into the list.
    ///
    /// The detail page can be reached from a place, and then the directory
    /// may be older than what is shown. A failure is deliberately silent
    /// here: the entry that is already there stays, and it is not wrong —
    /// only possibly a minute old.
    func refresh(traeger id: Int64) async {
        guard let frisch = try? await source.detail(id) else { return }
        if let stelle = all.firstIndex(where: { $0.id == id }) {
            all[stelle] = frisch
        } else {
            all.append(frisch)
        }
    }

    /// Fetches the requests of one Traeger. Only makes sense for those who
    /// administer it — the server answers everyone else with 403, and that
    /// then stands as a notice.
    func loadRequests(traeger id: Int64) async {
        do {
            requests[id] = try await source.requests(id)
            notice = nil
        } catch let fehler as DorfFehler {
            notice = fehler.klartext
        } catch {
            notice = Self.networkNotice
        }
    }

    /// The village directory — needed to take somebody in directly.
    func loadVillagers() async {
        guard villagers.isEmpty else { return }
        if let liste = try? await source.villagers() { villagers = liste }
    }

    // MARK: Joining

    /// "I want to join." A rejection keeps the typed reason: it is returned
    /// as false, and the view leaves the sheet open.
    @discardableResult
    func join(traeger id: Int64, reason: String) async -> Bool {
        guard !busy.contains(id) else { return false }
        busy.insert(id)
        defer { busy.remove(id) }
        do {
            _ = try await source.join(id, reason.trimmingCharacters(in: .whitespacesAndNewlines))
            await load()
            show(confirmation: "Deine Anfrage ist raus. Der Vorstand entscheidet sie.")
            return true
        } catch let abgewiesen as DorfFehler {
            // A 409 is not a mishap: the situation does not fit, and the
            // server says in which way. Either way the list is stale now.
            await load()
            error = abgewiesen.klartext
            return false
        } catch {
            self.error = Self.networkNotice
            return false
        }
    }

    // MARK: Deciding (for the board)

    func decide(request: Beitritt, status: String) async {
        guard !busyRequests.contains(request.id) else { return }
        busyRequests.insert(request.id)
        defer { busyRequests.remove(request.id) }
        do {
            _ = try await source.decide(request.id, status)
            await loadRequests(traeger: request.traegerId)
            await load()
            show(confirmation: status == "erteilt"
                ? "\(request.anzeigename) gehört jetzt dazu."
                : "Abgelehnt. \(request.anzeigename) bleibt außen vor.")
        } catch let abgewiesen as DorfFehler {
            // Granting writes to the Rössing-ID first. Fails that, the
            // request stays open — and saying so is the whole point.
            await loadRequests(traeger: request.traegerId)
            error = abgewiesen.klartext
        } catch {
            self.error = Self.networkNotice
        }
    }

    /// Takes somebody in without a request of their own — the only way into
    /// a closed group.
    func addMember(traeger id: Int64, person: Dorfbewohner) async {
        guard !busy.contains(id) else { return }
        busy.insert(id)
        defer { busy.remove(id) }
        do {
            _ = try await source.addMember(id, person.userSub)
            await loadRequests(traeger: id)
            await load()
            show(confirmation: "\(Self.name(of: person)) gehört jetzt dazu.")
        } catch let abgewiesen as DorfFehler {
            error = abgewiesen.klartext
        } catch {
            self.error = Self.networkNotice
        }
    }

    /// The name a villager goes by — nickname before display name before the
    /// bare identifier.
    static func name(of person: Dorfbewohner) -> String {
        let kandidaten = [person.name, person.nickname, person.displayName]
        return kandidaten.first { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
            ?? person.userSub
    }

    // MARK: Clearing messages

    func dismissError() { error = nil }

    func dismissNotice() { notice = nil }

    func dismissConfirmation() {
        fading?.cancel()
        confirmation = nil
    }

    /// The same wording as `DorfFehler.netz` — so that a dead spot sounds
    /// the same everywhere in the app.
    static let networkNotice = DorfFehler.netz("").klartext

    private func show(confirmation text: String) {
        fading?.cancel()
        confirmation = text
        fading = Task { [weak self] in
            try? await Task.sleep(for: .seconds(6))
            guard !Task.isCancelled else { return }
            self?.confirmation = nil
        }
    }
}

/// Wie ein Zulassungsstand und eine Sichtbarkeit auf Deutsch heißen.
///
/// Reine Anzeige: Was daraus folgt — sichtbar, beitretbar, verwaltbar —
/// steht schon in den Feldern, die der Server mitschickt.
enum TraegerTexte {
    static func status(_ roh: String) -> String {
        switch roh {
        case "zugelassen": return "Zugelassen"
        case "beantragt": return "Noch nicht zugelassen"
        case "gesperrt": return "Gesperrt"
        default: return roh
        }
    }

    static func sichtbarkeit(_ roh: String) -> String {
        switch roh {
        case "offen": return "Offen für alle im Dorf"
        case "geschlossen": return "Geschlossene Gruppe"
        default: return roh
        }
    }

    /// Was der eigene Antrag gerade macht.
    static func beitrittStatus(_ roh: String) -> String? {
        switch roh {
        case "beantragt": return "Deine Anfrage liegt beim Vorstand."
        case "erteilt": return "Du wurdest aufgenommen."
        case "abgelehnt": return "Deine Anfrage wurde abgelehnt."
        default: return nil
        }
    }
}
