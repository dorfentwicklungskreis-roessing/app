import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct IdeenView: View {
    var body: some View {
        ContentUnavailableView(
            "Idee vorschlagen",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Idee vorschlagen")
    }
}
