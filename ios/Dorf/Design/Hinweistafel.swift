import SwiftUI

/// „Hier ist gerade nichts" — die ganzseitige Auskunft, wenn eine Liste oder
/// eine Seite leer bleibt.
///
/// Sie ersetzt Apples `ContentUnavailableView`, die es erst ab iOS 17 gibt.
/// Die App läuft ab iOS 16, damit auch ein iPhone 8 oder X mitkommt — und
/// eine leere Fläche ohne Erklärung wäre die schlechteste Auskunft, gerade
/// dort. Aufbau und Größenverhältnisse sind dieselben wie im Original: großes
/// Symbol in Sekundärfarbe, darunter der Titel fett, darunter die
/// Beschreibung — alles mittig.
///
/// Für VoiceOver ist die Tafel **ein** Element: Symbol, Titel und
/// Beschreibung gehören zusammen und werden am Stück vorgelesen. Das Symbol
/// selbst sagt nichts, was der Titel nicht schon sagt, und bleibt deshalb
/// stumm.
struct Hinweistafel: View {
    /// Die Überschrift — kurz und in ganzen Worten („Noch niemand dabei").
    let titel: String
    /// Ein SF-Symbol, das zum Titel passt.
    let symbol: String
    /// Der erklärende Satz darunter. Ohne ihn steht nur der Titel da.
    var beschreibung: String?

    init(_ titel: String, symbol: String, beschreibung: String? = nil) {
        self.titel = titel
        self.symbol = symbol
        self.beschreibung = beschreibung
    }

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: symbol)
                .font(.system(size: 52, weight: .regular))
                .foregroundStyle(.secondary)
                .padding(.bottom, 4)
                // Das Symbol wiederholt nur den Titel — vorgelesen wird es
                // nicht.
                .accessibilityHidden(true)

            Text(titel)
                .font(.title2.weight(.bold))
                .multilineTextAlignment(.center)

            if let beschreibung {
                Text(beschreibung)
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.horizontal, 32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .combine)
    }

    /// Der Sonderfall „Suche ohne Treffer" — entspricht
    /// `ContentUnavailableView.search(text:)`.
    static func suche(text: String) -> Hinweistafel {
        Hinweistafel(
            "Keine Ergebnisse für \u{201E}\(text)\u{201C}",
            symbol: "magnifyingglass",
            beschreibung: "Überprüfe die Schreibweise oder starte eine neue Suche."
        )
    }
}

#Preview("Leer") {
    Hinweistafel(
        "Noch niemand dabei",
        symbol: "person.2",
        beschreibung: "Bisher hat niemand Angaben freigegeben."
    )
}

#Preview("Suche") {
    Hinweistafel.suche(text: "Meier")
}
