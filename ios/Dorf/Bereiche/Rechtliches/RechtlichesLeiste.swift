import SwiftUI

/// Impressum und Datenschutzerklärung — leicht erkennbar und unmittelbar
/// erreichbar, wie es § 5 DDG verlangt. Steht deshalb auf der Startseite
/// *und* auf dem Anmeldebildschirm.
///
/// Die Seiten stehen auf der Website und werden nur dort gepflegt; eine
/// zweite Fassung in der App würde über kurz oder lang abweichen, und dann
/// wäre keine mehr verbindlich.
struct RechtlichesLeiste: View {
    var body: some View {
        VStack(spacing: 2) {
            Text("Rechtliches")
                .font(.caption)
                .foregroundStyle(.secondary)
            HStack {
                Link("Impressum", destination: URL(string: "https://xn--rssing-wxa.de/impressum/")!)
                Link("Datenschutz", destination: URL(string: "https://xn--rssing-wxa.de/app/datenschutz/")!)
            }
            .font(.footnote)
        }
        .accessibilityIdentifier("rechtliches")
    }
}
