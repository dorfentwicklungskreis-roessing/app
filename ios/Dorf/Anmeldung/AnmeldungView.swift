import SwiftUI

/// Der Anmeldebildschirm.
///
/// Impressum und Datenschutz stehen auch hier — wer noch kein Konto hat, muss
/// trotzdem nachlesen können, was mit seinen Daten geschieht (§ 5 DDG).
struct AnmeldungView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var laeuft = false
    @State private var fehler: String?

    var body: some View {
        // Ein Stapel, damit der Blick in den Maschinchenring von hier aus
        // möglich ist: Die Geräteliste ist öffentlich, und wer noch keine
        // Rössing-ID hat, soll sich erst umsehen dürfen.
        NavigationStack {
            inhalt
        }
    }

    private var inhalt: some View {
        VStack(spacing: 24) {
            Spacer()

            Text("🌻")
                .font(.system(size: 72))
                .accessibilityHidden(true)

            VStack(spacing: 8) {
                Text("Willkommen in Rössing")
                    .font(.largeTitle.bold())
                    .multilineTextAlignment(.center)
                Text("Die App fürs Dorf. Den Anfang macht das Mithelfen: Was steht gerade an?")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal)

            if let fehler {
                Text(fehler)
                    .font(.footnote)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)
                    .accessibilityIdentifier("anmeldung-fehler")
            }

            Button {
                Task { await anmelden() }
            } label: {
                HStack {
                    if laeuft { ProgressView().controlSize(.small) }
                    Text(laeuft ? "Anmeldung läuft …" : "Mit Rössing-ID anmelden")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .disabled(laeuft)
            .padding(.horizontal, 32)
            .accessibilityIdentifier("anmeldung-knopf")

            // Umsehen geht ohne Konto: Die Geräteliste des Maschinchenrings
            // ist öffentlich. Zum Buchen fragt der Bereich selbst nach der
            // Rössing-ID — an der Stelle, an der sie wirklich gebraucht wird.
            NavigationLink {
                RentalCatalogView()
            } label: {
                Label("Erst einmal umsehen: Maschinchenring", systemImage: "wrench.and.screwdriver")
                    .font(.subheadline)
            }
            .accessibilityIdentifier("anmeldung-maschinchenring")

            if Konfiguration.entwicklerLoginErlaubt {
                VStack(spacing: 4) {
                    Button("Entwickler-Login (nur Test)") {
                        umgebung.anmeldung.entwicklerAnmeldung(alsAdmin: true)
                    }
                    .accessibilityIdentifier("anmeldung-entwickler")
                    Text("Nur im Debug-Build gegen ein Backend im Dev-Modus.")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer()
            RechtlichesLeiste()
                .padding(.bottom, 8)
        }
        .padding()
    }

    private func anmelden() async {
        laeuft = true
        fehler = nil
        let ergebnis = await umgebung.anmeldung.anmelden()
        laeuft = false
        switch ergebnis {
        case .erfolg:
            await umgebung.ichLaden()
        case .abgebrochen:
            // Abbruch ist kein Fehler: kommentarlos zurück auf diesen Schirm.
            break
        case .fehlgeschlagen(let kuerzel):
            fehler = "Anmeldung fehlgeschlagen (\(kuerzel)). Bitte erneut versuchen."
        }
    }
}
