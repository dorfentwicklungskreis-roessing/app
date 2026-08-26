import SwiftUI

/// Platzhalter — wird in einem eigenen Schritt gebaut.
struct ProfilView: View {
    var body: some View {
        ContentUnavailableView(
            "Mein Profil",
            systemImage: "hammer",
            description: Text("Dieser Bereich wird gerade gebaut.")
        )
        .navigationTitle("Mein Profil")
    }
}
