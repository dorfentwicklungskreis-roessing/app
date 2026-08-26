import SwiftUI
import UIKit

/// Die Dorfkarte mit den Orten als Ampel-Nadeln.
///
/// Feste Schnittstelle, damit „Mithelfen" und die Karte unabhängig gebaut
/// werden können: Die Karte bekommt die Orte gereicht und meldet den Tipp
/// zurück — sie lädt selbst nichts und weiß nichts vom Backend.
///
/// **Auswahlmodus** (für die Verwaltung): Wird `flaecheGetippt` gesetzt,
/// liefert ein Tipp auf die freie Fläche die Koordinate zurück, und
/// `gewaehlterPunkt` zeigt die getroffene Wahl. Beide Angaben haben
/// Vorgabewerte — der Aufruf aus „Mithelfen" (`KarteView(orte:auswahl:)`)
/// bleibt unverändert gültig.
struct KarteView: View {
    let orte: [Ort]
    var auswahl: (Ort) -> Void
    /// Der bereits gewählte Punkt. Er wird eigens hervorgehoben und ist
    /// bewusst kein Ort: Angelegt ist noch nichts.
    var gewaehlterPunkt: Kartenpunkt?
    /// Tipp auf die freie Fläche. `nil` heißt: kein Auswahlmodus — dann
    /// verändert ein Tipp neben eine Nadel gar nichts.
    var flaecheGetippt: ((Kartenpunkt) -> Void)?

    init(orte: [Ort],
         auswahl: @escaping (Ort) -> Void = { _ in },
         gewaehlterPunkt: Kartenpunkt? = nil,
         flaecheGetippt: ((Kartenpunkt) -> Void)? = nil) {
        self.orte = orte
        self.auswahl = auswahl
        self.gewaehlterPunkt = gewaehlterPunkt
        self.flaecheGetippt = flaecheGetippt
    }

    @State private var standort = Standortgeber()
    /// Eigener Punkt auf der Karte — erst nach erteilter Freigabe.
    @State private var standortZeigen = false
    /// Jeder Druck auf „Mein Standort" erhöht den Zähler; die Karte fährt
    /// daraufhin einmal hin.
    @State private var hinfahren = 0
    @State private var stilVersuch = 0
    @State private var stilFehler: String?
    @State private var standortHinweis: String?
    /// Der Systemdialog läuft; die Antwort kommt über `standort.freigabe`.
    @State private var wartetAufFreigabe = false

    var body: some View {
        // Der GeometryReader ist kein Zierrat: Er meldet die Größe der Karte
        // an `MapLibreKarte` weiter, und erst damit kann der Startausschnitt
        // gerechnet werden — auch nach dem Drehen des Geräts.
        GeometryReader { platz in
            MapLibreKarte(
                orte: orte,
                groesse: platz.size,
                eigenenStandortZeigen: standortZeigen,
                hinfahren: hinfahren,
                stilVersuch: stilVersuch,
                auswahl: auswahl,
                gewaehlterPunkt: gewaehlterPunkt,
                flaecheGetippt: flaecheGetippt,
                stilzustand: { meldung in stilFehler = meldung }
            )
        }
        .overlay(alignment: .top) { hinweise }
        .overlay(alignment: .bottomTrailing) { standortknopf }
        .onChange(of: standort.freigabe) { _, neue in freigabeGeaendert(neue) }
    }

    // MARK: - Knopf „Mein Standort"

    private var standortknopf: some View {
        Button(action: meinStandort) {
            Image(systemName: standortZeigen ? "location.fill" : "location")
                .font(.title3)
                .padding(14)
                .background(.regularMaterial, in: Circle())
                .overlay(Circle().strokeBorder(.separator))
        }
        .buttonStyle(.plain)
        .padding(16)
        .accessibilityLabel("Mein Standort")
        .accessibilityHint("Zeigt deinen Standort auf der Karte und fährt hin.")
        .accessibilityIdentifier("karte-mein-standort")
    }

    private func meinStandort() {
        switch standort.freigabe {
        case .erlaubt:
            standortHinweis = nil
            standortZeigen = true
            hinfahren += 1
        case .ungefragt:
            // Erst jetzt fragen — nicht schon beim Öffnen der Karte.
            wartetAufFreigabe = true
            standort.anfragen()
        case .verweigert:
            standortHinweis = verweigertText
        }
    }

    private func freigabeGeaendert(_ neue: Standortgeber.Freigabe) {
        guard wartetAufFreigabe else { return }
        switch neue {
        case .ungefragt:
            return
        case .erlaubt:
            wartetAufFreigabe = false
            standortHinweis = nil
            standortZeigen = true
            hinfahren += 1
        case .verweigert:
            wartetAufFreigabe = false
            standortHinweis = verweigertText
        }
    }

    private var verweigertText: String {
        "Ohne Freigabe zur Ortung kann die Karte deinen Standort nicht zeigen. "
            + "Du kannst sie in den Einstellungen nachtragen — die Karte "
            + "funktioniert auch ohne."
    }

    // MARK: - Hinweise

    /// Lädt der Stil nicht, steht hier ein Satz statt einer weißen Fläche.
    /// Die Karte bleibt bedienbar: verschieben, zoomen und Nadeln antippen
    /// gehen auch ohne Hintergrundkarte.
    @ViewBuilder private var hinweise: some View {
        VStack(spacing: 8) {
            if let stilFehler {
                hinweiskachel(text: stilFehler, symbol: "wifi.slash") {
                    Button("Erneut versuchen") { stilVersuch += 1 }
                        .accessibilityIdentifier("karte-stil-erneut")
                }
            }
            if let standortHinweis {
                hinweiskachel(text: standortHinweis, symbol: "location.slash") {
                    HStack(spacing: 16) {
                        if let einstellungen = URL(string: UIApplication.openSettingsURLString) {
                            Link("Einstellungen öffnen", destination: einstellungen)
                        }
                        Button("Verstanden") { self.standortHinweis = nil }
                            .accessibilityIdentifier("karte-standort-verstanden")
                    }
                }
            }
        }
        .padding(12)
    }

    private func hinweiskachel(
        text: String,
        symbol: String,
        @ViewBuilder knoepfe: () -> some View
    ) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(text, systemImage: symbol)
                .font(.subheadline)
                .fixedSize(horizontal: false, vertical: true)
            knoepfe()
                .font(.subheadline.weight(.semibold))
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(.separator))
    }
}

#Preview("Karte") {
    KarteView(orte: [
        Ort(id: 1, name: "Unter den Eichen", lat: 52.1832, lon: 9.8168, status: "red"),
        Ort(id: 2, name: "Am Bahnhof", lat: 52.1961, lon: 9.8151, status: "yellow"),
        Ort(id: 3, name: "Kirchplatz", lat: 52.1902, lon: 9.8102),
    ])
}

#Preview("Auswahlmodus") {
    KarteView(
        orte: [Ort(id: 1, name: "Kirchplatz", lat: 52.1902, lon: 9.8102)],
        gewaehlterPunkt: Kartenpunkt(breite: 52.1912, laenge: 9.8122),
        flaecheGetippt: { _ in }
    )
}
