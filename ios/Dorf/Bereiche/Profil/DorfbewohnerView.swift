import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct DorfbewohnerView: View {
    var body: some View {
        ContentUnavailableView(
            "Dorfbewohner",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Dorfbewohner")
    }
}
