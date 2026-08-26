import SwiftUI

/// The "Verwalten" section of the start page.
///
/// Administration no longer happens in the app: places, tasks, completions,
/// the heat factor and the ideas are handled by the MCP server and the web
/// admin, which share the same backend endpoints. Simply dropping the old
/// tile would leave a gap exactly where someone with the `admin` role looks
/// for it — so this section says where the work moved and how to get there.
///
/// Shown to admins only. For everyone else the two addresses answer a
/// question they never asked. That is politeness, not a safeguard: the rule
/// is enforced by the backend, which rejects every change without the role.
struct AdminHintSection: View {
    var body: some View {
        Section {
            Text("Orte und Aufgaben anlegen, ändern, pausieren und löschen, "
                + "Erledigungen nachtragen und zurücknehmen, Rangliste abfragen, "
                + "Hitzefaktor setzen, Ideen sichten: Das läuft über den Dorfserver. "
                + "Angemeldet wird mit der Rössing-ID, die Rolle „admin“ ist Voraussetzung.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("admin-hint-intro")

            Link(destination: Self.mcpAddress) {
                AdminHintRow(
                    symbol: "text.bubble",
                    title: "Mit Claude",
                    text: "In claude.ai unter Einstellungen → Connectors einen eigenen "
                        + "Connector anlegen und diese Adresse eintragen:",
                    address: Self.mcpAddress.absoluteString
                )
            }
            // Without `.plain` the link tints its whole label, and the
            // explanation would read as if every word were tappable.
            .buttonStyle(.plain)
            .accessibilityIdentifier("admin-hint-mcp")

            Link(destination: Self.adminAddress) {
                AdminHintRow(
                    symbol: "macwindow",
                    title: "Im Browser",
                    text: "Dieselben Dinge zum Klicken, mit einer Karte zum Setzen des Punktes.",
                    address: Self.adminAddress.absoluteString
                )
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("admin-hint-web")
        } header: {
            Text("Verwalten")
        } footer: {
            Text("Claude kennt auf dem Telefon deinen Standort: Du kannst vor dem "
                + "Blumenkasten stehen und sagen „leg hier einen Kasten an“ — die "
                + "Koordinaten übernimmt Claude. Damit kann die App nichts mehr, "
                + "was diese beiden Wege nicht besser können.")
                .accessibilityIdentifier("admin-hint-reason")
        }
    }

    // The addresses are derived from `Konfiguration.apiBasis`, not written
    // out here: CI and E2E point the app at a local backend, and an address
    // hard-coded in the source would send them to production.
    static var mcpAddress: URL { address("/mcp") }
    static var adminAddress: URL { address("/admin/") }

    private static func address(_ path: String) -> URL {
        URL(string: path, relativeTo: Konfiguration.apiBasis)?.absoluteURL
            ?? Konfiguration.apiBasis
    }
}

/// One way in: what it is, what to do there, and the address to copy.
private struct AdminHintRow: View {
    let symbol: String
    let title: String
    let text: String
    let address: String

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: symbol)
                .font(.title2)
                .frame(width: 32)
                .foregroundStyle(.tint)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text(title).font(.headline)
                Text(text)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text(address)
                    .font(.footnote.monospaced())
                    .foregroundStyle(.tint)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 8)
            // The arrow says what the chevron would not: this leaves the app
            // and opens a browser.
            Image(systemName: "arrow.up.right")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)
        }
        .padding(.vertical, 4)
    }
}
