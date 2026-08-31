import SwiftUI

/// The pieces every screen of this area uses: the note above a list, the way
/// back into a sign-in, a thumbnail, a line of facts, and a Markdown text.

/// The note above a list — with the way back, not just the bad news.
struct RentalNotice: View {
    let text: String
    var actionTitle: String = "Erneut versuchen"
    let action: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(text, systemImage: "exclamationmark.triangle.fill")
                .font(.subheadline)
                .fixedSize(horizontal: false, vertical: true)
            Button(actionTitle, action: action)
                .buttonStyle(.borderless)
                .accessibilityIdentifier("rental-notice-action")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("rental-notice")
    }
}

/// The note when a sign-in is what is missing.
///
/// Two cases, one box. „Nobody is signed in" leads to the ordinary sign-in;
/// „this device's token predates the Maschinchenring" leads to the same
/// screen but for a different reason, and saying which one it is beats an
/// empty list and a shrug (`docs/mieten-api.md`, `token_audience`).
struct RentalSignInNotice: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    let trouble: RentalTrouble
    /// Run after a successful sign-in — usually „fetch it again".
    let afterwards: () async -> Void
    @State private var running = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(trouble.message, systemImage: trouble.needsFreshSignIn
                ? "person.badge.key" : "person.crop.circle.badge.questionmark")
                .font(.subheadline)
                .fixedSize(horizontal: false, vertical: true)
            Button(trouble.needsFreshSignIn ? "Neu anmelden" : "Mit Rössing-ID anmelden") {
                Task { await signIn() }
            }
            .buttonStyle(.borderless)
            .disabled(running)
            .accessibilityIdentifier("rental-sign-in")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier(trouble.needsFreshSignIn
            ? "rental-sign-in-outdated" : "rental-sign-in-required")
    }

    private func signIn() async {
        running = true
        defer { running = false }
        // A full sign-in, not a refresh: Zitadel only puts the rental
        // platform into the token's audience when the authorization request
        // asks for it, and a refreshed token keeps the audiences it was
        // issued with. That is the whole reason this button exists.
        if await umgebung.anmeldung.anmelden() == .erfolg {
            await umgebung.ichLaden()
            await afterwards()
        }
    }
}

/// The little picture in front of a row. A device without one keeps its place
/// in the column, so the names stay aligned.
struct RentalThumbnail: View {
    let url: URL?

    var body: some View {
        Group {
            if let url {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .success(let bild):
                        bild.resizable().scaledToFill()
                    default:
                        placeholder
                    }
                }
            } else {
                placeholder
            }
        }
        .frame(width: 56, height: 56)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .accessibilityHidden(true)
    }

    private var placeholder: some View {
        ZStack {
            Color.accentColor.opacity(0.10)
            Image(systemName: "wrench.and.screwdriver")
                .font(.title3)
                .foregroundStyle(.tint)
        }
    }
}

/// One line of facts — the same arrangement for tariff, deposit and address,
/// so a block does not look like four different things.
struct RentalFact: View {
    let symbol: String
    let text: String

    var body: some View {
        Label {
            Text(text).font(.subheadline)
        } icon: {
            Image(systemName: symbol).foregroundStyle(.tint)
        }
    }
}

/// A device description, as the platform writes it: Markdown.
///
/// Paragraphs, bullets and headings are laid out here; bold, italics and
/// links come from `AttributedString`. Showing the raw text with its
/// asterisks would be the one thing that is not an option.
struct RentalMarkdownText: View {
    let blocks: [RentalTextBlock]

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(blocks) { block in
                switch block {
                case .heading(let level, let text):
                    Text(text)
                        .font(level <= 2 ? .headline : .subheadline.weight(.semibold))
                        .padding(.top, 2)
                        .fixedSize(horizontal: false, vertical: true)
                case .paragraph(let text):
                    Text(text)
                        .font(.body)
                        .fixedSize(horizontal: false, vertical: true)
                case .bullet(let text):
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Text("•").font(.body).foregroundStyle(.secondary)
                        Text(text)
                            .font(.body)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
