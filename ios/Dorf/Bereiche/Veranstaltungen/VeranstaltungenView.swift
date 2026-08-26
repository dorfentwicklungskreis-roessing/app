import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct VeranstaltungenView: View {
    var body: some View {
        ContentUnavailableView(
            "Was ist los in Rössing",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Was ist los in Rössing")
    }
}
