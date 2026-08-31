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
/// Searching happens here, over what is already loaded: the catalogue is
/// small, it answers on the first keystroke, and it keeps working while the
/// platform is away. That is a filter over text, not a rule about renting.
final class RentalCatalogModel: ObservableObject {
    @Published private(set) var devices: [RentalDevice] = []
    @Published private(set) var loading = false
    @Published private(set) var trouble: RentalTrouble?
    @Published var query = ""

    private var fetched = false
    private let source: RentalSource

    init(source: RentalSource) {
        self.source = source
    }

    /// What the list shows right now — everything, or what matches the query.
    var visible: [RentalDevice] {
        let wanted = query.trimmingCharacters(in: .whitespacesAndNewlines)
        if wanted.isEmpty { return devices }
        return devices.filter { $0.matches(wanted) }
    }

    /// Nothing there, nothing on the way, nothing broken — then the platform
    /// really has nothing to lend.
    var empty: Bool { devices.isEmpty && !loading && trouble == nil }

    /// Somebody searched and there is nothing. Different from `empty`, and it
    /// deserves a different sentence.
    var withoutMatch: Bool { !devices.isEmpty && visible.isEmpty }

    /// The note above the list. If an older list is still standing it says so
    /// instead of claiming there is nothing.
    var hint: String? {
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
    }
}
