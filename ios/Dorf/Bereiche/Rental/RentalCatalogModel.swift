import Combine
import Foundation

/// The state of the catalogue („Maschinchenring": which devices are there?).
///
/// The rule of this area is the same as in the events: **an error is not
/// „nothing there".** If a list is already standing it stays standing and
/// gets a note above it. An empty page without an explanation would be the
/// worst possible answer — and it is the one somebody gets whenever a second
/// service is down and nobody thought about it.
///
/// Searching is the platform's job: it weighs meaning and wording together
/// and hands back an order (`docs/mieten-api.md`, route 3), which this model
/// does not touch. Only when that route cannot be reached does the model fall
/// back to filtering the list it already holds — a filter over text, so that
/// looking around keeps working while the platform is away.
final class RentalCatalogModel: ObservableObject {
    @Published private(set) var devices: [RentalDevice] = []
    @Published private(set) var sets: [RentalSet] = []
    @Published private(set) var loading = false
    @Published private(set) var trouble: RentalTrouble?
    @Published var query = ""

    /// What the platform answered for the current query — `nil` while nobody
    /// is searching.
    @Published private(set) var results: [RentalDevice]?
    @Published private(set) var searching = false
    /// Set when the search route could not be reached and the list on the
    /// device was filtered instead. Said out loud, because the result is a
    /// different one.
    @Published private(set) var searchedLocally = false

    private var fetched = false
    private let source: RentalSource

    init(source: RentalSource) {
        self.source = source
    }

    /// What the list shows right now.
    var visible: [RentalDevice] { results ?? devices }

    /// Nothing there, nothing on the way, nothing broken — then the platform
    /// really has nothing to lend.
    var empty: Bool { devices.isEmpty && !loading && trouble == nil }

    /// Somebody searched and there is nothing. Different from `empty`, and it
    /// deserves a different sentence.
    var withoutMatch: Bool { results?.isEmpty == true && !searching }

    /// Whether the sets are worth a section of their own.
    var showsSets: Bool { !sets.isEmpty && results == nil }

    /// The note above the list. If an older list is still standing it says so
    /// instead of claiming there is nothing.
    var hint: String? {
        if searchedLocally {
            return "Die Suche des Maschinchenrings ist gerade nicht erreichbar. "
                + "Durchsucht wird die Liste, die schon geladen war."
        }
        guard let trouble else { return nil }
        if devices.isEmpty { return trouble.message }
        return "Gerade keine Verbindung zum Maschinchenring — die Liste ist "
            + "möglicherweise nicht mehr aktuell."
    }

    /// When the area is opened. What is already here is not fetched again.
    func load() async {
        if fetched { return }
        await fetch()
    }

    /// Pulling down, or „Erneut versuchen".
    func refresh() async {
        await fetch()
        if !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            await runSearch()
        }
    }

    private func fetch() async {
        if loading { return }
        loading = true
        defer { loading = false }
        do {
            let raw = try await source.items()
            devices = raw.asDevices()
            fetched = true
            trouble = nil
        } catch {
            // The old list stays; the note goes above it.
            trouble = RentalTrouble(error)
        }
        // The sets are an addition, not the point of the screen: if they do
        // not arrive, the catalogue is still a catalogue.
        if let raw = try? await source.sets() {
            sets = raw.asSets()
        }
    }

    /// Asks the platform. Called by the screen once typing has settled — the
    /// waiting happens there so this stays a plain, testable step.
    func runSearch() async {
        let wanted = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !wanted.isEmpty else {
            results = nil
            searchedLocally = false
            return
        }
        searching = true
        defer { searching = false }
        do {
            let raw = try await source.search(wanted)
            // The platform's order is the answer; sorting it again here would
            // throw the ranking away.
            results = raw.asDevices()
            searchedLocally = false
        } catch {
            // Away, but not helpless: what is already on the device can still
            // be filtered, and the screen says that this is what happened.
            results = devices.filter { $0.matches(wanted) }
            searchedLocally = true
        }
    }
}
