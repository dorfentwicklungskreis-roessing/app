import SwiftUI

/// Entscheidet nur eins: angemeldet oder nicht. Alles Weitere hängt an
/// [StartView].
struct WurzelView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @ObservedObject private var melder = ErrorReporter.gemeinsam

    var body: some View {
        inhalt
            // Der Hinweis auf eine Störung sitzt hier und nicht in einem
            // Bereich: Schiefgehen kann etwas überall — auch schon auf dem
            // Anmeldeschirm —, und niemand soll erst den richtigen Schirm
            // suchen müssen, um davon zu erfahren.
            //
            // „Erneut versuchen" gibt es nur, wenn ein neuer Abruf etwas
            // bringen kann; genau das beantwortet `stoerungshinweis`.
            .fehlerbanner(melder, erneutVersuchen: erneutVersuchen)
    }

    @ViewBuilder private var inhalt: some View {
        switch umgebung.anmeldung.sitzung {
        case .laedt:
            ProgressView().controlSize(.large)
        case .abgemeldet:
            AnmeldungView()
        case .angemeldet:
            StartView()
                .task { await umgebung.ichLaden() }
        }
    }

    private var erneutVersuchen: (() -> Void)? {
        guard umgebung.stoerungshinweis != nil else { return nil }
        return { Task { await umgebung.erneutVersuchen() } }
    }
}
