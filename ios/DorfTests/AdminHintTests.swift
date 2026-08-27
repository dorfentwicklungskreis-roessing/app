import Foundation
import Testing
import UIKit

@testable import Dorf

/// The "Verwalten" section: two addresses that look alike and are used in
/// entirely different ways. The connector address goes on the clipboard —
/// `/mcp` speaks a protocol, a browser only shows an error there — while the
/// web administration stays a link that a browser can open.
///
/// Nothing here reaches a server: the addresses are read from the build
/// settings of the running build, and all that happens to them is a copy and
/// a shape check. Whichever backend the build points at, both shapes have to
/// hold — otherwise nobody finds the way.
@MainActor
struct AdminHintTests {
    // MARK: The connector address is copied

    @Test func copyingPutsTheConnectorAddressOnThePasteboard() {
        // A pasteboard of its own instead of the system's: a test must not
        // overwrite what the person at the machine has copied.
        let pasteboard = UIPasteboard.withUniqueName()
        defer { UIPasteboard.remove(withName: pasteboard.name) }

        let copied = AdminHintSection.copyMcpAddress(to: pasteboard)

        #expect(copied == AdminHintSection.mcpAddress.absoluteString)
        #expect(pasteboard.string == AdminHintSection.mcpAddress.absoluteString)
    }

    @Test func theCopiedAddressIsWholeAndNotAFragment() {
        // Whoever pastes it into claude.ai pastes it into an empty field:
        // scheme and host have to travel with it.
        let pasteboard = UIPasteboard.withUniqueName()
        defer { UIPasteboard.remove(withName: pasteboard.name) }

        let copied = AdminHintSection.copyMcpAddress(to: pasteboard)

        #expect(copied.hasPrefix("http"))
        #expect(URL(string: copied)?.host?.isEmpty == false)
    }

    @Test func theConnectorAddressPointsAtTheMcpEndpoint() {
        // No trailing slash: claude.ai passes the address on unchanged, and
        // the MCP endpoint is served without one. Checked on the whole
        // address, not on `path` — that one drops a trailing slash and would
        // pass either way.
        #expect(AdminHintSection.mcpAddress.absoluteString.hasSuffix("/mcp"))
    }

    // MARK: The web administration stays a link

    @Test func theWebAddressCanBeOpenedByABrowser() {
        // This row is still a `Link`, and `Link` needs a URL a browser can
        // follow: a scheme it knows and a host to ask.
        let address = AdminHintSection.adminAddress
        #expect(address.scheme?.hasPrefix("http") == true)
        #expect(address.host?.isEmpty == false)
    }

    @Test func theWebAddressKeepsItsTrailingSlash() {
        // The redirect URI registered in Zitadel ends in one, and a redirect
        // in the in-app browser looks like a failure. `path` is no witness
        // here: it drops the very slash this is about.
        #expect(AdminHintSection.adminAddress.absoluteString.hasSuffix("/admin/"))
    }

    // MARK: Both together

    @Test func bothWaysLeadToTheSameServer() {
        // One deployment serves REST, MCP and the web administration.
        // Whoever moves the app to another host moves both addresses.
        #expect(AdminHintSection.mcpAddress.host == AdminHintSection.adminAddress.host)
    }

    @Test func theTwoAddressesAreNotTheSame() {
        // They differ, and so does what one does with them — copying the web
        // address or opening the connector address would both be wrong.
        #expect(AdminHintSection.mcpAddress != AdminHintSection.adminAddress)
    }
}
