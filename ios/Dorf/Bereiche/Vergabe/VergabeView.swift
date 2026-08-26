import SwiftUI

/// Platzhalter — „Anfragen und Hinweise" wird in einem eigenen Schritt gebaut.
struct VergabeView: View {
    var body: some View {
        ContentUnavailableView("Anfragen", systemImage: "bell",
                               description: Text("Dieser Bereich wird gerade gebaut."))
            .navigationTitle("Anfragen")
    }
}
