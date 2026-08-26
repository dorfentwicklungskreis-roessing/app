import SwiftUI
import UIKit

/// The "Verwalten" section of the start page.
///
/// Administration no longer happens in the app: places, tasks, completions,
/// the heat factor and the ideas are handled by the MCP server and the web
/// admin, which share the same backend endpoints. Simply dropping the old
/// tile would leave a gap exactly where someone with the `admin` role looks
/// for it — so this section says where the work moved and how to get there.
///
/// The two ways look alike — two https addresses — but they are used in
/// entirely different ways, and the section shows that: the connector
/// address is copied, the web admin is opened.
///
/// Shown to admins only. For everyone else the two addresses answer a
/// question they never asked. That is politeness, not a safeguard: the rule
/// is enforced by the backend, which rejects every change without the role.
struct AdminHintSection: View {
    /// How long "Adresse kopiert" stays before the row goes back to
    /// offering the copy. Long enough to be read, short enough that nobody
    /// takes it for the permanent state of the row.
    private static let confirmationDuration: Duration = .seconds(3)

    @State private var addressCopied = false
    @State private var confirmationReset: Task<Void, Never>?

    var body: some View {
        Section {
            Text("Orte und Aufgaben anlegen, ändern, pausieren und löschen, "
                + "Erledigungen nachtragen und zurücknehmen, Rangliste abfragen, "
                + "Hitzefaktor setzen, Ideen sichten: Das läuft über den Dorfserver. "
                + "Angemeldet wird mit der Rössing-ID, die Rolle „admin“ ist Voraussetzung.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("admin-hint-intro")

            // Deliberately a button and not a `Link`: `/mcp` is a protocol
            // endpoint (MCP over Streamable HTTP), not a web page — whoever
            // taps it in a browser sees nothing but an error message. The
            // address is needed somewhere else entirely: it is entered in
            // claude.ai under Einstellungen → Connectors, often from another
            // device. So it belongs on the clipboard — and stays spelled out
            // underneath, so it can be typed off as well.
            Button(action: copyMcpAddress) {
                AdminHintRow(
                    symbol: "text.bubble",
                    title: "Mit Claude",
                    text: "In claude.ai unter Einstellungen → Connectors einen eigenen "
                        + "Connector anlegen und diese Adresse eintragen:",
                    address: Self.mcpAddress.absoluteString,
                    actionSymbol: addressCopied ? "checkmark.circle.fill" : "doc.on.doc",
                    actionText: addressCopied ? "Adresse kopiert" : "Adresse kopieren",
                    actionColor: addressCopied ? .green : .accentColor,
                    actionIdentifier: addressCopied ? "admin-hint-copied" : "admin-hint-copy"
                )
            }
            // Without `.plain` the button tints its whole label, and the
            // explanation would read as if every word were tappable.
            .buttonStyle(.plain)
            .accessibilityIdentifier("admin-hint-mcp")
            .accessibilityHint("Legt die Adresse in die Zwischenablage")

            // This one is a real web page, so tapping it is the right thing.
            Link(destination: Self.adminAddress) {
                AdminHintRow(
                    symbol: "macwindow",
                    title: "Im Browser",
                    text: "Dieselben Dinge zum Klicken, mit einer Karte zum Setzen des Punktes.",
                    address: Self.adminAddress.absoluteString,
                    // The arrow says what the chevron would not: this leaves
                    // the app and opens a browser.
                    actionSymbol: "arrow.up.right",
                    actionText: "Im Browser öffnen",
                    actionColor: .accentColor,
                    actionIdentifier: "admin-hint-open"
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

    /// Copies and confirms. Without the confirmation a tap looks exactly
    /// like a dead button — nothing on screen would change.
    private func copyMcpAddress() {
        Self.copyMcpAddress()
        confirmationReset?.cancel()
        withAnimation { addressCopied = true }
        confirmationReset = Task {
            try? await Task.sleep(for: Self.confirmationDuration)
            guard !Task.isCancelled else { return }
            withAnimation { addressCopied = false }
        }
    }

    /// Puts the connector address on the pasteboard and returns what was
    /// written there.
    ///
    /// Split off from the button so a test can check what lands there: the
    /// closure of a view is out of reach without a UI test target, and this
    /// project has none. The pasteboard is a parameter for the same reason —
    /// a test uses one of its own instead of the system's.
    @discardableResult
    static func copyMcpAddress(to pasteboard: UIPasteboard = .general) -> String {
        let address = mcpAddress.absoluteString
        pasteboard.string = address
        return address
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

/// One way in: what it is, what to do there, the address in plain text — and
/// the gesture that belongs to it. Symbol and wording of that gesture differ
/// per row, because the gestures differ.
private struct AdminHintRow: View {
    let symbol: String
    let title: String
    let text: String
    let address: String
    let actionSymbol: String
    let actionText: String
    let actionColor: Color
    let actionIdentifier: String

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
                // In plain text on purpose: whoever sits at a second screen
                // has to be able to type the address off.
                Text(address)
                    .font(.footnote.monospaced())
                    .foregroundStyle(.tint)
                    .fixedSize(horizontal: false, vertical: true)
                Label(actionText, systemImage: actionSymbol)
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(actionColor)
                    .accessibilityIdentifier(actionIdentifier)
                    .padding(.top, 2)
            }
            Spacer(minLength: 8)
        }
        .padding(.vertical, 4)
    }
}
