import SwiftUI

/// Die Dorfkarte mit den Orten als Ampel-Nadeln.
///
/// Feste Schnittstelle, damit „Mithelfen" und die Karte unabhängig gebaut
/// werden können: Die Karte bekommt die Orte gereicht und meldet den Tipp
/// zurück — sie lädt selbst nichts und weiß nichts vom Backend.
struct KarteView: View {
    let orte: [Ort]
    var auswahl: (Ort) -> Void

    init(orte: [Ort], auswahl: @escaping (Ort) -> Void = { _ in }) {
        self.orte = orte
        self.auswahl = auswahl
    }

    var body: some View {
        ContentUnavailableView(
            "Karte",
            systemImage: "map",
            description: Text("Die Karte wird gerade gebaut.")
        )
    }
}
