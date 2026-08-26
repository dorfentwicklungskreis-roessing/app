import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct RanglisteView: View {
    var body: some View {
        ContentUnavailableView(
            "Rangliste",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Rangliste")
    }
}
