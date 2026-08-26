import SwiftUI

/// Platzhalter — Einstellungen und Konto werden in einem eigenen Schritt gebaut.
struct EinstellungenView: View {
    var body: some View {
        ContentUnavailableView("Einstellungen", systemImage: "gearshape",
                               description: Text("Dieser Bereich wird gerade gebaut."))
            .navigationTitle("Einstellungen")
    }
}
