import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct MithelfenView: View {
    var body: some View {
        ContentUnavailableView(
            "Mithelfen",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Mithelfen")
    }
}
